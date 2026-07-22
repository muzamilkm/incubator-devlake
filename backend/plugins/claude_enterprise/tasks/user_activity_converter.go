/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tasks

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/models/domainlayer"
	"github.com/apache/incubator-devlake/core/models/domainlayer/ai"
	"github.com/apache/incubator-devlake/core/models/domainlayer/crossdomain"
	"github.com/apache/incubator-devlake/core/models/domainlayer/didgen"
	"github.com/apache/incubator-devlake/core/plugin"
	helper "github.com/apache/incubator-devlake/helpers/pluginhelper/api"
	"github.com/apache/incubator-devlake/plugins/claude_enterprise/models"
)

var ConvertUserActivitiesMeta = plugin.SubTaskMeta{
	Name:             "convertUserActivities",
	EntryPoint:       ConvertUserActivities,
	EnabledByDefault: true,
	Description:      "convert compatible Claude Enterprise user activity records into ai_activities",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_CROSS},
	DependencyTables: []string{"_tool_claude_enterprise_analytics_records"},
	ProductTables:    []string{"ai_activities"},
}

// BuildUserActivity converts one generic analytics tool row into a domain row.
// Unsupported Enterprise products return nil so their full payload remains
// available in the tool layer without overloading unrelated domain fields.
func BuildUserActivity(
	idGen *didgen.DomainIdGenerator,
	accountId string,
	record *models.ClaudeEnterpriseAnalyticsRecord,
) (*ai.AiActivity, errors.Error) {
	activities, err := BuildUserActivities(idGen, accountId, record)
	if err != nil || len(activities) == 0 {
		return nil, err
	}
	return activities[0], nil
}

// BuildUserActivities fans out the documented Chat and Claude Code metric
// blocks into independently queryable activities.
func BuildUserActivities(
	idGen *didgen.DomainIdGenerator,
	accountId string,
	record *models.ClaudeEnterpriseAnalyticsRecord,
) ([]*ai.AiActivity, errors.Error) {
	if record == nil || record.Endpoint != userActivitiesEndpoint.Name {
		return nil, nil
	}

	var item map[string]interface{}
	if err := json.Unmarshal([]byte(record.RawJson), &item); err != nil {
		return nil, errors.Default.Wrap(err, "failed to parse Claude Enterprise user activity payload")
	}

	dateText := firstNonEmpty(record.Date, firstString(item, "date", "starting_date", "day"))
	date, err := time.Parse("2006-01-02", dateText)
	if err != nil {
		return nil, errors.BadInput.Wrap(err, "invalid Claude Enterprise user activity date")
	}

	userEmail := firstNonEmpty(record.UserEmail, firstString(item, "user.email_address"))
	activities := make([]*ai.AiActivity, 0, 2)
	if userActivityHasChatMetrics(item) {
		activities = append(activities, buildUserActivity(idGen, accountId, record, date, userEmail, "chat", item))
	}
	if userActivityHasClaudeCodeMetrics(item) {
		activities = append(activities, buildUserActivity(idGen, accountId, record, date, userEmail, "claude_code", item))
	}
	return activities, nil
}

func buildUserActivity(idGen *didgen.DomainIdGenerator, accountId string, record *models.ClaudeEnterpriseAnalyticsRecord, date time.Time, userEmail string, product string, item map[string]interface{}) *ai.AiActivity {
	activityType, interfaceType := userActivitySemantics(product)
	activity := &ai.AiActivity{
		DomainEntity: domainlayer.DomainEntity{
			Id: idGen.Generate(record.ConnectionId, record.ScopeId, record.OrganizationId, record.Endpoint, record.RecordId, product),
		},
		Provider:      "claude_enterprise",
		AccountId:     accountId,
		UserEmail:     userEmail,
		Date:          date,
		Type:          activityType,
		InterfaceType: interfaceType,
		Model:         record.Model,
		NumSessions:   userActivitySessions(product, item),
	}
	if product == "claude_code" {
		activity.SuggestionsCount = userActivityToolActionCount(item, "rejected_count")
		activity.AcceptanceCount = userActivityToolActionCount(item, "accepted_count")
		activity.LinesAdded = intValue(item, "claude_code_metrics.core_metrics.lines_of_code.added_count")
		activity.LinesRemoved = intValue(item, "claude_code_metrics.core_metrics.lines_of_code.removed_count")
		activity.CommitsCreated = intValue(item, "claude_code_metrics.core_metrics.commit_count")
		activity.PrsCreated = intValue(item, "claude_code_metrics.core_metrics.pull_request_count")
	}
	return activity
}

func ConvertUserActivities(taskCtx plugin.SubTaskContext) errors.Error {
	data, ok := taskCtx.TaskContext().GetData().(*ClaudeEnterpriseTaskData)
	if !ok {
		return errors.Default.New("task data is not ClaudeEnterpriseTaskData")
	}

	db := taskCtx.GetDal()
	params := analyticsRawParams{
		ConnectionId:   data.Options.ConnectionId,
		ScopeId:        data.Options.ScopeId,
		OrganizationId: effectiveOrganizationId(data),
		Endpoint:       userActivitiesEndpoint.Name,
	}
	cursor, err := db.Cursor(
		dal.From(&models.ClaudeEnterpriseAnalyticsRecord{}),
		dal.Where("connection_id = ? AND scope_id = ? AND endpoint = ?", params.ConnectionId, params.ScopeId, userActivitiesEndpoint.Name),
	)
	if err != nil {
		return err
	}
	defer cursor.Close()

	idGen := didgen.NewDomainIdGenerator(&models.ClaudeEnterpriseAnalyticsRecord{})
	converter, err := helper.NewDataConverter(helper.DataConverterArgs{
		RawDataSubTaskArgs: helper.RawDataSubTaskArgs{
			Ctx:     taskCtx,
			Table:   RawUserActivitiesTable,
			Options: params,
		},
		InputRowType: reflect.TypeOf(models.ClaudeEnterpriseAnalyticsRecord{}),
		Input:        cursor,
		Convert: func(inputRow interface{}) ([]interface{}, errors.Error) {
			record := inputRow.(*models.ClaudeEnterpriseAnalyticsRecord)
			accountId, resolveErr := resolveClaudeEnterpriseAccountId(db, record.UserEmail)
			if resolveErr != nil {
				return nil, resolveErr
			}
			activities, buildErr := BuildUserActivities(idGen, accountId, record)
			if buildErr != nil || len(activities) == 0 {
				return nil, buildErr
			}
			rows := make([]interface{}, len(activities))
			for i, activity := range activities {
				rows[i] = activity
			}
			return rows, nil
		},
	})
	if err != nil {
		return err
	}
	return converter.Execute()
}

func userActivitySemantics(product string) (string, string) {
	switch product {
	case "claude_code", "claude-code", "code":
		return "CODE_EDIT", "cli"
	case "chat", "claude_chat", "claude-chat":
		return "CHAT", "web_ui"
	default:
		return "", ""
	}
}

func userActivitySessions(product string, item map[string]interface{}) int {
	if product == "chat" || product == "claude_chat" || product == "claude-chat" {
		return intValue(item, "chat_metrics.distinct_conversation_count")
	}
	return intValue(item, "claude_code_metrics.core_metrics.distinct_session_count")
}

func userActivityHasChatMetrics(item map[string]interface{}) bool {
	return intValue(item, "chat_metrics.distinct_conversation_count", "chat_metrics.message_count", "chat_metrics.connectors_used_count") > 0
}

func userActivityHasClaudeCodeMetrics(item map[string]interface{}) bool {
	return intValue(item,
		"claude_code_metrics.core_metrics.distinct_session_count",
		"claude_code_metrics.core_metrics.commit_count",
		"claude_code_metrics.core_metrics.pull_request_count",
		"claude_code_metrics.core_metrics.lines_of_code.added_count",
		"claude_code_metrics.core_metrics.lines_of_code.removed_count",
	) > 0 || userActivityToolActionCount(item, "accepted_count") > 0 || userActivityToolActionCount(item, "rejected_count") > 0
}

func userActivityToolActionCount(item map[string]interface{}, metric string) int {
	total := 0
	for _, tool := range []string{"edit_tool", "multi_edit_tool", "notebook_edit_tool", "write_tool"} {
		total += intValue(item, "claude_code_metrics.tool_actions."+tool+"."+metric)
	}
	return total
}

// resolveClaudeEnterpriseAccountId resolves a Claude Enterprise activity's
// email to a crossdomain.Account ID. It loads *every* matching account
// (never just the first, unordered row a DB happens to return) and branches
// explicitly: zero matches or an empty email is a legitimate "unresolved
// user" outcome, exactly one match resolves normally, and a real query
// failure is propagated instead of being treated identically to "no rows".
func resolveClaudeEnterpriseAccountId(db dal.Dal, email string) (string, errors.Error) {
	if email == "" {
		return "", nil
	}
	var accounts []crossdomain.Account
	if err := db.All(&accounts, dal.Where("email = ?", email)); err != nil {
		return "", err
	}
	switch len(accounts) {
	case 0:
		return "", nil
	case 1:
		return accounts[0].Id, nil
	default:
		// Multiple accounts share this email -- a data integrity anomaly,
		// not a legitimate state to guess through. Picking one arbitrarily
		// (the defect this function used to replicate, see
		// implementation-plan.md Section 11, defect #7) risks silently
		// misattributing activity to the wrong person, so it's safer to
		// leave it unresolved than to pick a "first" row unordered SQL
		// happens to hand back.
		return "", nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func intValue(item map[string]interface{}, paths ...string) int {
	for _, path := range paths {
		if value, ok := lookupPath(item, strings.Split(path, ".")); ok {
			switch typed := value.(type) {
			case float64:
				return int(typed)
			case int:
				return typed
			case int64:
				return int(typed)
			case json.Number:
				parsed, err := typed.Int64()
				if err == nil {
					return int(parsed)
				}
			case string:
				parsed, err := strconv.Atoi(typed)
				if err == nil {
					return parsed
				}
			}
		}
	}
	return 0
}
