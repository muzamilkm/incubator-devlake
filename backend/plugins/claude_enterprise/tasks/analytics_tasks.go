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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/plugin"
	helper "github.com/apache/incubator-devlake/helpers/pluginhelper/api"
	"github.com/apache/incubator-devlake/plugins/claude_enterprise/models"
)

const (
	RawSummariesTable       = "claude_enterprise_api_summaries"
	RawUserActivitiesTable  = "claude_enterprise_api_users"
	RawUserUsageReportTable = "claude_enterprise_api_user_usage_report"
	RawUserCostReportTable  = "claude_enterprise_api_user_cost_report"

	analyticsPageSize                  = 1000
	analyticsReconciliationOverlapDays = 3
	usageCostReconciliationWindowDays  = 30
)

type dateParamStyle int

const (
	dateParamStartingDate dateParamStyle = iota
	dateParamStartingAt
	dateParamDate
)

type analyticsEndpoint struct {
	Name        string
	RawTable    string
	Path        string
	Description string
	DateStyle   dateParamStyle
	Paginated   bool

	// DailyIterated selects one starting/ending-date request per UTC day
	// instead of a single request covering the whole configured range.
	// Section 5.1 prefers daily collection over range-rollup collection for
	// engagement/adoption endpoints because DevLake needs stable daily rows.
	DailyIterated bool
	// ExclusiveEndingDate treats the per-day request's ending date as
	// exclusive (day, day+1), matching the /summaries endpoint's documented
	// semantics. Other daily-iterated endpoints keep an inclusive same-day
	// ending date.
	ExclusiveEndingDate bool
	// ExtraToolTables lists additive typed tool tables (beyond the shared
	// raw-preserving _tool_claude_enterprise_analytics_records table) that
	// this endpoint's extractor writes to.
	ExtraToolTables []string
}

var (
	summariesEndpoint = analyticsEndpoint{
		Name:                "summaries",
		RawTable:            RawSummariesTable,
		Path:                "organizations/analytics/summaries",
		Description:         "Claude Enterprise activity summaries",
		DateStyle:           dateParamStartingDate,
		DailyIterated:       true,
		ExclusiveEndingDate: true,
		ExtraToolTables:     []string{"_tool_claude_enterprise_summaries"},
	}
	userActivitiesEndpoint = analyticsEndpoint{
		Name:          "users",
		RawTable:      RawUserActivitiesTable,
		Path:          "organizations/analytics/users",
		Description:   "Claude Enterprise per-user activity",
		DateStyle:     dateParamDate,
		Paginated:     true,
		DailyIterated: true,
	}
	userUsageReportEndpoint = analyticsEndpoint{
		Name:            "user_usage_report",
		RawTable:        RawUserUsageReportTable,
		Path:            "organizations/analytics/user_usage_report",
		Description:     "Claude Enterprise per-user token usage",
		DateStyle:       dateParamStartingAt,
		Paginated:       true,
		ExtraToolTables: []string{"_tool_claude_enterprise_usage_reports"},
	}
	userCostReportEndpoint = analyticsEndpoint{
		Name:            "user_cost_report",
		RawTable:        RawUserCostReportTable,
		Path:            "organizations/analytics/user_cost_report",
		Description:     "Claude Enterprise per-user cost",
		DateStyle:       dateParamStartingAt,
		Paginated:       true,
		ExtraToolTables: []string{"_tool_claude_enterprise_cost_reports"},
	}
)

var CollectSummariesMeta = newCollectMeta(summariesEndpoint, CollectSummaries)
var ExtractSummariesMeta = newExtractMeta(summariesEndpoint, ExtractSummaries)
var CollectUserActivitiesMeta = newCollectMeta(userActivitiesEndpoint, CollectUserActivities)
var ExtractUserActivitiesMeta = newExtractMeta(userActivitiesEndpoint, ExtractUserActivities)
var CollectUserUsageReportMeta = newCollectMeta(userUsageReportEndpoint, CollectUserUsageReport)
var ExtractUserUsageReportMeta = newExtractMeta(userUsageReportEndpoint, ExtractUserUsageReport)
var CollectUserCostReportMeta = newCollectMeta(userCostReportEndpoint, CollectUserCostReport)
var ExtractUserCostReportMeta = newExtractMeta(userCostReportEndpoint, ExtractUserCostReport)

func newCollectMeta(endpoint analyticsEndpoint, entryPoint plugin.SubTaskEntryPoint) plugin.SubTaskMeta {
	return plugin.SubTaskMeta{
		Name:             "collect" + endpointTaskSuffix(endpoint.Name),
		EntryPoint:       entryPoint,
		EnabledByDefault: true,
		Description:      "collect " + endpoint.Description,
		DomainTypes:      []string{plugin.DOMAIN_TYPE_CROSS},
		ProductTables:    []string{"_raw_" + endpoint.RawTable},
	}
}

func newExtractMeta(endpoint analyticsEndpoint, entryPoint plugin.SubTaskEntryPoint) plugin.SubTaskMeta {
	productTables := []string{"_tool_claude_enterprise_analytics_records"}
	productTables = append(productTables, endpoint.ExtraToolTables...)
	return plugin.SubTaskMeta{
		Name:             "extract" + endpointTaskSuffix(endpoint.Name),
		EntryPoint:       entryPoint,
		EnabledByDefault: true,
		Description:      "extract " + endpoint.Description + " into tool records",
		DomainTypes:      []string{plugin.DOMAIN_TYPE_CROSS},
		DependencyTables: []string{"_raw_" + endpoint.RawTable},
		ProductTables:    productTables,
	}
}

func CollectSummaries(taskCtx plugin.SubTaskContext) errors.Error {
	return collectAnalyticsEndpoint(taskCtx, summariesEndpoint)
}

func ExtractSummaries(taskCtx plugin.SubTaskContext) errors.Error {
	return extractSummariesEndpoint(taskCtx)
}

func CollectUserActivities(taskCtx plugin.SubTaskContext) errors.Error {
	return collectAnalyticsEndpoint(taskCtx, userActivitiesEndpoint)
}

func ExtractUserActivities(taskCtx plugin.SubTaskContext) errors.Error {
	return extractAnalyticsEndpoint(taskCtx, userActivitiesEndpoint)
}

func CollectUserUsageReport(taskCtx plugin.SubTaskContext) errors.Error {
	return collectAnalyticsEndpoint(taskCtx, userUsageReportEndpoint)
}

func ExtractUserUsageReport(taskCtx plugin.SubTaskContext) errors.Error {
	return extractUsageReportEndpoint(taskCtx)
}

func CollectUserCostReport(taskCtx plugin.SubTaskContext) errors.Error {
	return collectAnalyticsEndpoint(taskCtx, userCostReportEndpoint)
}

func ExtractUserCostReport(taskCtx plugin.SubTaskContext) errors.Error {
	return extractCostReportEndpoint(taskCtx)
}

func collectAnalyticsEndpoint(taskCtx plugin.SubTaskContext, endpoint analyticsEndpoint) errors.Error {
	data, ok := taskCtx.TaskContext().GetData().(*ClaudeEnterpriseTaskData)
	if !ok {
		return errors.Default.New("task data is not ClaudeEnterpriseTaskData")
	}
	connection := data.Connection
	connection.Normalize()

	apiClient, err := CreateApiClient(taskCtx.TaskContext(), connection)
	if err != nil {
		return err
	}

	params := analyticsRawParams{
		ConnectionId:   data.Options.ConnectionId,
		ScopeId:        data.Options.ScopeId,
		OrganizationId: effectiveOrganizationId(data),
		Endpoint:       endpoint.Name,
	}
	rawArgs := helper.RawDataSubTaskArgs{
		Ctx:     taskCtx,
		Table:   endpoint.RawTable,
		Options: params,
		Params:  params,
	}

	collector, err := helper.NewStatefulApiCollector(rawArgs)
	if err != nil {
		return err
	}

	startingDate, endingDate := resolveDateRangeForEndpoint(endpoint, data.Options)
	input, inputErr := analyticsDateIteratorForEndpoint(endpoint, startingDate, endingDate)
	if inputErr != nil {
		return inputErr
	}
	err = collector.InitCollector(helper.ApiCollectorArgs{
		ApiClient:   apiClient,
		Input:       input,
		PageSize:    analyticsPageSize,
		UrlTemplate: endpoint.Path,
		Query: func(reqData *helper.RequestData) (url.Values, errors.Error) {
			query := url.Values{}
			setDateQueryFromRequest(query, endpoint.DateStyle, startingDate, endingDate, reqData)
			setPaginationQuery(query, endpoint, reqData)
			return query, nil
		},
		GetNextPageCustomData: getAnalyticsNextPageFunc(endpoint),
		ResponseParser:        parseAnalyticsResponse,
		Incremental:           true,
		Concurrency:           1,
	})
	if err != nil {
		return err
	}
	return collector.Execute()
}

func extractAnalyticsEndpoint(taskCtx plugin.SubTaskContext, endpoint analyticsEndpoint) errors.Error {
	data, ok := taskCtx.TaskContext().GetData().(*ClaudeEnterpriseTaskData)
	if !ok {
		return errors.Default.New("task data is not ClaudeEnterpriseTaskData")
	}

	params := analyticsRawParams{
		ConnectionId:   data.Options.ConnectionId,
		ScopeId:        data.Options.ScopeId,
		OrganizationId: effectiveOrganizationId(data),
		Endpoint:       endpoint.Name,
	}
	extractor, err := helper.NewApiExtractor(helper.ApiExtractorArgs{
		RawDataSubTaskArgs: helper.RawDataSubTaskArgs{
			Ctx:     taskCtx,
			Table:   endpoint.RawTable,
			Options: params,
		},
		Extract: func(row *helper.RawData) ([]interface{}, errors.Error) {
			rowParams := analyticsParamsForRawData(params, row)
			record, buildErr := BuildAnalyticsRecord(row.Data, rowParams)
			if buildErr != nil {
				return nil, buildErr
			}
			return []interface{}{record}, nil
		},
	})
	if err != nil {
		return err
	}
	return extractor.Execute()
}

// extractTypedAnalyticsEndpoint extracts both the raw-preserving generic
// analytics record and one entity-specific typed tool row per raw item. It is
// shared by the Phase 14 extended entities (skills, connectors, chat
// projects, plugins, artifacts), which all follow the same
// raw-preservation-plus-typed-row shape as the Phase 10/11 summary, usage,
// and cost extractors.
func extractTypedAnalyticsEndpoint(
	taskCtx plugin.SubTaskContext,
	endpoint analyticsEndpoint,
	buildTyped func(raw []byte, params analyticsRawParams) (interface{}, errors.Error),
) errors.Error {
	data, ok := taskCtx.TaskContext().GetData().(*ClaudeEnterpriseTaskData)
	if !ok {
		return errors.Default.New("task data is not ClaudeEnterpriseTaskData")
	}

	params := analyticsRawParams{
		ConnectionId:   data.Options.ConnectionId,
		ScopeId:        data.Options.ScopeId,
		OrganizationId: effectiveOrganizationId(data),
		Endpoint:       endpoint.Name,
	}
	extractor, err := helper.NewApiExtractor(helper.ApiExtractorArgs{
		RawDataSubTaskArgs: helper.RawDataSubTaskArgs{
			Ctx:     taskCtx,
			Table:   endpoint.RawTable,
			Options: params,
		},
		Extract: func(row *helper.RawData) ([]interface{}, errors.Error) {
			rowParams := analyticsParamsForRawData(params, row)
			record, buildErr := BuildAnalyticsRecord(row.Data, rowParams)
			if buildErr != nil {
				return nil, buildErr
			}
			typed, typedErr := buildTyped(row.Data, rowParams)
			if typedErr != nil {
				return nil, typedErr
			}
			return []interface{}{record, typed}, nil
		},
	})
	if err != nil {
		return err
	}
	return extractor.Execute()
}

func extractSummariesEndpoint(taskCtx plugin.SubTaskContext) errors.Error {
	data, ok := taskCtx.TaskContext().GetData().(*ClaudeEnterpriseTaskData)
	if !ok {
		return errors.Default.New("task data is not ClaudeEnterpriseTaskData")
	}

	params := analyticsRawParams{
		ConnectionId:   data.Options.ConnectionId,
		ScopeId:        data.Options.ScopeId,
		OrganizationId: effectiveOrganizationId(data),
		Endpoint:       summariesEndpoint.Name,
	}
	extractor, err := helper.NewApiExtractor(helper.ApiExtractorArgs{
		RawDataSubTaskArgs: helper.RawDataSubTaskArgs{
			Ctx:     taskCtx,
			Table:   summariesEndpoint.RawTable,
			Options: params,
		},
		Extract: func(row *helper.RawData) ([]interface{}, errors.Error) {
			rowParams := analyticsParamsForRawData(params, row)
			record, buildErr := BuildAnalyticsRecord(row.Data, rowParams)
			if buildErr != nil {
				return nil, buildErr
			}
			summary, summaryErr := BuildSummaryRecord(row.Data, rowParams)
			if summaryErr != nil {
				return nil, summaryErr
			}
			return []interface{}{record, summary}, nil
		},
	})
	if err != nil {
		return err
	}
	return extractor.Execute()
}

func extractUsageReportEndpoint(taskCtx plugin.SubTaskContext) errors.Error {
	data, ok := taskCtx.TaskContext().GetData().(*ClaudeEnterpriseTaskData)
	if !ok {
		return errors.Default.New("task data is not ClaudeEnterpriseTaskData")
	}

	params := analyticsRawParams{
		ConnectionId:   data.Options.ConnectionId,
		ScopeId:        data.Options.ScopeId,
		OrganizationId: effectiveOrganizationId(data),
		Endpoint:       userUsageReportEndpoint.Name,
	}
	extractor, err := helper.NewApiExtractor(helper.ApiExtractorArgs{
		RawDataSubTaskArgs: helper.RawDataSubTaskArgs{
			Ctx:     taskCtx,
			Table:   userUsageReportEndpoint.RawTable,
			Options: params,
		},
		Extract: func(row *helper.RawData) ([]interface{}, errors.Error) {
			rowParams := analyticsParamsForRawData(params, row)
			record, buildErr := BuildAnalyticsRecord(row.Data, rowParams)
			if buildErr != nil {
				return nil, buildErr
			}
			usage, usageErr := BuildUsageReport(row.Data, rowParams)
			if usageErr != nil {
				return nil, usageErr
			}
			return []interface{}{record, usage}, nil
		},
	})
	if err != nil {
		return err
	}
	return extractor.Execute()
}

func extractCostReportEndpoint(taskCtx plugin.SubTaskContext) errors.Error {
	data, ok := taskCtx.TaskContext().GetData().(*ClaudeEnterpriseTaskData)
	if !ok {
		return errors.Default.New("task data is not ClaudeEnterpriseTaskData")
	}

	params := analyticsRawParams{
		ConnectionId:   data.Options.ConnectionId,
		ScopeId:        data.Options.ScopeId,
		OrganizationId: effectiveOrganizationId(data),
		Endpoint:       userCostReportEndpoint.Name,
	}
	extractor, err := helper.NewApiExtractor(helper.ApiExtractorArgs{
		RawDataSubTaskArgs: helper.RawDataSubTaskArgs{
			Ctx:     taskCtx,
			Table:   userCostReportEndpoint.RawTable,
			Options: params,
		},
		Extract: func(row *helper.RawData) ([]interface{}, errors.Error) {
			rowParams := analyticsParamsForRawData(params, row)
			record, buildErr := BuildAnalyticsRecord(row.Data, rowParams)
			if buildErr != nil {
				return nil, buildErr
			}
			cost, costErr := BuildCostReport(row.Data, rowParams)
			if costErr != nil {
				return nil, costErr
			}
			return []interface{}{record, cost}, nil
		},
	})
	if err != nil {
		return err
	}
	return extractor.Execute()
}

// BuildAnalyticsRecord extracts stable indexing fields from a raw analytics item.
func BuildAnalyticsRecord(raw []byte, params analyticsRawParams) (*models.ClaudeEnterpriseAnalyticsRecord, errors.Error) {
	var item map[string]interface{}
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, errors.Default.Wrap(err, "failed to parse Claude Enterprise analytics item")
	}

	record := &models.ClaudeEnterpriseAnalyticsRecord{
		ConnectionId:   params.ConnectionId,
		ScopeId:        params.ScopeId,
		OrganizationId: params.OrganizationId,
		Endpoint:       params.Endpoint,
		Date:           firstNonEmpty(params.RequestDate, firstString(item, "date", "starting_date", "starting_at", "start_date", "day")),
		Grain:          firstString(item, "grain", "period", "granularity"),
		UserId:         firstString(item, "user.id", "actor.user_id"),
		UserEmail:      firstString(item, "user.email_address", "actor.email"),
		Product:        firstString(item, "product"),
		Model:          firstString(item, "model"),
		RawJson:        string(raw),
	}
	record.RecordId = analyticsRecordId(record)
	return record, nil
}

// BuildSummaryRecord extracts dashboard-ready adoption metrics from one
// summaries endpoint item while retaining the original raw JSON.
func BuildSummaryRecord(raw []byte, params analyticsRawParams) (*models.ClaudeEnterpriseSummary, errors.Error) {
	var item map[string]interface{}
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, errors.Default.Wrap(err, "failed to parse Claude Enterprise summary item")
	}

	date := firstNonEmpty(params.RequestDate, firstString(item, "date", "starting_date", "day"), datePart(firstString(item, "starting_at")))
	if date == "" {
		return nil, errors.BadInput.New("Claude Enterprise summary item is missing date")
	}

	return &models.ClaudeEnterpriseSummary{
		ConnectionId:                     params.ConnectionId,
		ScopeId:                          params.ScopeId,
		OrganizationId:                   params.OrganizationId,
		Date:                             date,
		Grain:                            firstString(item, "grain", "period", "granularity"),
		StartingAt:                       firstString(item, "starting_at"),
		EndingAt:                         firstString(item, "ending_at"),
		AssignedSeatCount:                firstInt(item, "assigned_seat_count"),
		PendingInviteCount:               firstInt(item, "pending_invite_count"),
		DailyActiveUserCount:             firstInt(item, "daily_active_user_count"),
		WeeklyActiveUserCount:            firstInt(item, "weekly_active_user_count"),
		MonthlyActiveUserCount:           firstInt(item, "monthly_active_user_count"),
		DailyAdoptionRate:                firstFloat64(item, "daily_adoption_rate"),
		WeeklyAdoptionRate:               firstFloat64(item, "weekly_adoption_rate"),
		MonthlyAdoptionRate:              firstFloat64(item, "monthly_adoption_rate"),
		ChatDailyActiveUserCount:         firstInt(item, "chat_daily_active_user_count"),
		ChatWeeklyActiveUserCount:        firstInt(item, "chat_weekly_active_user_count"),
		ChatMonthlyActiveUserCount:       firstInt(item, "chat_monthly_active_user_count"),
		ClaudeCodeDailyActiveUserCount:   firstInt(item, "claude_code_daily_active_user_count"),
		ClaudeCodeWeeklyActiveUserCount:  firstInt(item, "claude_code_weekly_active_user_count"),
		ClaudeCodeMonthlyActiveUserCount: firstInt(item, "claude_code_monthly_active_user_count"),
		CoworkDailyActiveUserCount:       firstInt(item, "cowork_daily_active_user_count"),
		CoworkWeeklyActiveUserCount:      firstInt(item, "cowork_weekly_active_user_count"),
		CoworkMonthlyActiveUserCount:     firstInt(item, "cowork_monthly_active_user_count"),
		RawJson:                          string(raw),
	}, nil
}

// BuildUsageReport extracts a dashboard-ready per-user usage row while keeping
// its raw JSON available in the generic analytics record.
func BuildUsageReport(raw []byte, params analyticsRawParams) (*models.ClaudeEnterpriseUsageReport, errors.Error) {
	var item map[string]interface{}
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, errors.Default.Wrap(err, "failed to parse Claude Enterprise usage report item")
	}

	startingAt := firstString(item, "starting_at")
	if startingAt == "" {
		return nil, errors.BadInput.New("Claude Enterprise usage report item is missing starting_at")
	}

	report := &models.ClaudeEnterpriseUsageReport{
		ConnectionId:               params.ConnectionId,
		ScopeId:                    params.ScopeId,
		OrganizationId:             effectiveItemOrganizationId(item, params),
		StartingAt:                 startingAt,
		EndingAt:                   firstString(item, "ending_at"),
		UserId:                     firstString(item, "actor.user_id"),
		UserEmail:                  firstString(item, "actor.email"),
		DeletedActor:               firstBool(item, "actor.deleted"),
		Product:                    firstString(item, "product"),
		Model:                      firstString(item, "model"),
		DataRefreshedAt:            firstString(item, "data_refreshed_at"),
		ContextWindow:              firstString(item, "context_window"),
		InferenceGeo:               firstString(item, "inference_geo"),
		Speed:                      firstString(item, "speed"),
		UncachedInputTokens:        firstInt64(item, "uncached_input_tokens"),
		OutputTokens:               firstInt64(item, "output_tokens", "outputTokens"),
		CacheReadInputTokens:       firstInt64(item, "cache_read_input_tokens"),
		CacheCreation1hInputTokens: firstInt64(item, "cache_creation.ephemeral_1h_input_tokens"),
		CacheCreation5mInputTokens: firstInt64(item, "cache_creation.ephemeral_5m_input_tokens"),
		TotalTokens:                firstInt64(item, "total_tokens"),
		RequestCount:               firstInt64(item, "requests"),
		WebSearchRequests:          firstInt64(item, "server_tool_use.web_search_requests"),
		RawJson:                    string(raw),
	}
	report.ReportId = usageReportId(report)
	return report, nil
}

// BuildCostReport extracts a dashboard-ready per-user cost row. Monetary
// amounts are represented as strings so decimal precision is not lost.
func BuildCostReport(raw []byte, params analyticsRawParams) (*models.ClaudeEnterpriseCostReport, errors.Error) {
	var item map[string]interface{}
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, errors.Default.Wrap(err, "failed to parse Claude Enterprise cost report item")
	}

	startingAt := firstString(item, "starting_at")
	if startingAt == "" {
		return nil, errors.BadInput.New("Claude Enterprise cost report item is missing starting_at")
	}

	report := &models.ClaudeEnterpriseCostReport{
		ConnectionId:    params.ConnectionId,
		ScopeId:         params.ScopeId,
		OrganizationId:  effectiveItemOrganizationId(item, params),
		StartingAt:      startingAt,
		EndingAt:        firstString(item, "ending_at"),
		UserId:          firstString(item, "actor.user_id"),
		UserEmail:       firstString(item, "actor.email"),
		DeletedActor:    firstBool(item, "actor.deleted"),
		Product:         firstString(item, "product"),
		Model:           firstString(item, "model"),
		ContextWindow:   firstString(item, "context_window"),
		InferenceGeo:    firstString(item, "inference_geo"),
		Speed:           firstString(item, "speed"),
		CostType:        firstString(item, "cost_type"),
		TokenType:       firstString(item, "token_type"),
		Currency:        firstString(item, "currency"),
		DataRefreshedAt: firstString(item, "data_refreshed_at"),
		Amount:          firstDecimalString(item, "amount"),
		ListAmount:      firstDecimalString(item, "list_amount"),
		RequestCount:    firstInt64(item, "requests"),
		RawJson:         string(raw),
	}
	report.ReportId = costReportId(report)
	return report, nil
}

type analyticsResponseEnvelope struct {
	Data      []json.RawMessage `json:"data"`
	Items     []json.RawMessage `json:"items"`
	Results   []json.RawMessage `json:"results"`
	Summaries []json.RawMessage `json:"summaries"`
}

type analyticsPaginationEnvelope struct {
	NextPage string `json:"next_page"`
}

func parseAnalyticsResponse(res *http.Response) ([]json.RawMessage, errors.Error) {
	body, readErr := io.ReadAll(res.Body)
	res.Body.Close()
	if readErr != nil {
		return nil, errors.Default.Wrap(readErr, "failed to read Claude Enterprise analytics response")
	}
	if len(body) == 0 {
		return nil, nil
	}

	var rows []json.RawMessage
	if jsonErr := json.Unmarshal(body, &rows); jsonErr == nil {
		return rows, nil
	}

	var envelope analyticsResponseEnvelope
	if jsonErr := json.Unmarshal(body, &envelope); jsonErr != nil {
		return nil, errors.Default.Wrap(jsonErr, "failed to parse Claude Enterprise analytics response")
	}
	switch {
	case envelope.Data != nil:
		return envelope.Data, nil
	case envelope.Items != nil:
		return envelope.Items, nil
	case envelope.Results != nil:
		return envelope.Results, nil
	case envelope.Summaries != nil:
		return envelope.Summaries, nil
	default:
		return []json.RawMessage{body}, nil
	}
}

func getAnalyticsNextPageFunc(endpoint analyticsEndpoint) func(*helper.RequestData, *http.Response) (interface{}, errors.Error) {
	if !endpoint.Paginated {
		return nil
	}
	return parseAnalyticsNextPage
}

func parseAnalyticsNextPage(_ *helper.RequestData, res *http.Response) (interface{}, errors.Error) {
	defer res.Body.Close()
	body, readErr := io.ReadAll(res.Body)
	if readErr != nil {
		return nil, errors.Default.Wrap(readErr, "failed to read Claude Enterprise pagination response")
	}
	var envelope analyticsPaginationEnvelope
	if jsonErr := json.Unmarshal(body, &envelope); jsonErr != nil {
		return nil, errors.Default.Wrap(jsonErr, "failed to parse Claude Enterprise pagination response")
	}
	if envelope.NextPage == "" {
		return nil, helper.ErrFinishCollect
	}
	return envelope.NextPage, nil
}

func firstString(item map[string]interface{}, paths ...string) string {
	for _, path := range paths {
		if value, ok := lookupPath(item, strings.Split(path, ".")); ok {
			switch typed := value.(type) {
			case string:
				if typed != "" {
					return typed
				}
			case fmt.Stringer:
				return typed.String()
			case float64, bool:
				return fmt.Sprint(typed)
			}
		}
	}
	return ""
}

func firstInt(item map[string]interface{}, paths ...string) int {
	for _, path := range paths {
		if value, ok := lookupPath(item, strings.Split(path, ".")); ok {
			switch typed := value.(type) {
			case float64:
				return int(typed)
			case int:
				return typed
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

func firstInt64(item map[string]interface{}, paths ...string) int64 {
	for _, path := range paths {
		if value, ok := lookupPath(item, strings.Split(path, ".")); ok {
			switch typed := value.(type) {
			case float64:
				return int64(typed)
			case int:
				return int64(typed)
			case int64:
				return typed
			case json.Number:
				parsed, err := typed.Int64()
				if err == nil {
					return parsed
				}
			case string:
				parsed, err := strconv.ParseInt(typed, 10, 64)
				if err == nil {
					return parsed
				}
			}
		}
	}
	return 0
}

func firstFloat64(item map[string]interface{}, paths ...string) float64 {
	for _, path := range paths {
		if value, ok := lookupPath(item, strings.Split(path, ".")); ok {
			switch typed := value.(type) {
			case float64:
				return typed
			case int:
				return float64(typed)
			case int64:
				return float64(typed)
			case json.Number:
				parsed, err := typed.Float64()
				if err == nil {
					return parsed
				}
			case string:
				parsed, err := strconv.ParseFloat(typed, 64)
				if err == nil {
					return parsed
				}
			}
		}
	}
	return 0
}

func firstBool(item map[string]interface{}, paths ...string) bool {
	for _, path := range paths {
		if value, ok := lookupPath(item, strings.Split(path, ".")); ok {
			if typed, ok := value.(bool); ok {
				return typed
			}
		}
	}
	return false
}

func firstDecimalString(item map[string]interface{}, paths ...string) string {
	for _, path := range paths {
		if value, ok := lookupPath(item, strings.Split(path, ".")); ok {
			switch typed := value.(type) {
			case string:
				return typed
			case json.Number:
				return typed.String()
			case float64:
				return strconv.FormatFloat(typed, 'f', -1, 64)
			case int:
				return strconv.Itoa(typed)
			case int64:
				return strconv.FormatInt(typed, 10)
			}
		}
	}
	return ""
}

func effectiveItemOrganizationId(item map[string]interface{}, params analyticsRawParams) string {
	if organizationId := firstString(item, "organization_id", "organizationId"); organizationId != "" {
		return organizationId
	}
	return params.OrganizationId
}

func analyticsParamsForRawData(params analyticsRawParams, row *helper.RawData) analyticsRawParams {
	if row == nil || len(row.Input) == 0 {
		return params
	}
	var input analyticsDayInput
	if err := json.Unmarshal(row.Input, &input); err == nil {
		params.RequestDate = input.StartingDate
	}
	return params
}

func lookupPath(item map[string]interface{}, path []string) (interface{}, bool) {
	var current interface{} = item
	for _, key := range path {
		object, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current, ok = object[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func analyticsRecordId(record *models.ClaudeEnterpriseAnalyticsRecord) string {
	stableParts := []string{
		record.Endpoint,
		record.Date,
		record.Grain,
		record.UserId,
		record.UserEmail,
		record.Product,
		record.Model,
		record.RawJson,
	}
	sum := sha256.Sum256([]byte(strings.Join(stableParts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func usageReportId(report *models.ClaudeEnterpriseUsageReport) string {
	stableParts := []string{
		report.OrganizationId,
		report.StartingAt,
		report.EndingAt,
		report.UserId,
		report.Product,
		report.Model,
		report.ContextWindow,
		report.InferenceGeo,
		report.Speed,
	}
	sum := sha256.Sum256([]byte(strings.Join(stableParts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func costReportId(report *models.ClaudeEnterpriseCostReport) string {
	stableParts := []string{
		report.OrganizationId,
		report.StartingAt,
		report.EndingAt,
		report.UserId,
		report.Product,
		report.Model,
		report.ContextWindow,
		report.InferenceGeo,
		report.Speed,
		report.CostType,
		report.TokenType,
		report.Currency,
	}
	sum := sha256.Sum256([]byte(strings.Join(stableParts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func datePart(value string) string {
	if len(value) >= len("2006-01-02") {
		return value[:len("2006-01-02")]
	}
	return value
}

func resolveDateRange(options *ClaudeEnterpriseOptions) (string, string) {
	if options != nil && options.StartingDate != "" {
		return options.StartingDate, options.EndingDate
	}
	ending := time.Now().UTC().AddDate(0, 0, -analyticsReconciliationOverlapDays)
	starting := ending.AddDate(0, 0, -6)
	return starting.Format("2006-01-02"), ending.Format("2006-01-02")
}

func resolveDateRangeForEndpoint(endpoint analyticsEndpoint, options *ClaudeEnterpriseOptions) (string, string) {
	if endpoint.DateStyle != dateParamStartingAt {
		return resolveDateRange(options)
	}
	if options != nil && options.StartingDate != "" {
		return normalizeAnalyticsTimestamp(options.StartingDate), normalizeAnalyticsTimestamp(options.EndingDate)
	}
	ending := time.Now().UTC()
	starting := ending.AddDate(0, 0, -usageCostReconciliationWindowDays)
	return starting.Format(time.RFC3339), ending.Format(time.RFC3339)
}

func normalizeAnalyticsTimestamp(value string) string {
	if value == "" {
		return ""
	}
	if _, err := time.Parse(time.RFC3339, value); err == nil {
		return value
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed.UTC().Format(time.RFC3339)
	}
	return value
}

type analyticsDayInput struct {
	StartingDate string `json:"startingDate"`
	EndingDate   string `json:"endingDate"`
}

type analyticsDayIterator struct {
	days []analyticsDayInput
	idx  int
}

func newAnalyticsDayIterator(startingDate string, endingDate string, exclusiveEndingDate bool) (*analyticsDayIterator, errors.Error) {
	if endingDate == "" {
		endingDate = startingDate
	}
	start, err := time.Parse("2006-01-02", startingDate)
	if err != nil {
		return nil, errors.BadInput.Wrap(err, "invalid starting date")
	}
	end, err := time.Parse("2006-01-02", endingDate)
	if err != nil {
		return nil, errors.BadInput.Wrap(err, "invalid ending date")
	}
	if end.Before(start) {
		return nil, errors.BadInput.New("ending date must be on or after starting date")
	}

	var days []analyticsDayInput
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		formatted := day.Format("2006-01-02")
		inputEndingDate := formatted
		if exclusiveEndingDate {
			inputEndingDate = day.AddDate(0, 0, 1).Format("2006-01-02")
		}
		days = append(days, analyticsDayInput{StartingDate: formatted, EndingDate: inputEndingDate})
	}
	return &analyticsDayIterator{days: days}, nil
}

func (it *analyticsDayIterator) HasNext() bool {
	return it.idx < len(it.days)
}

func (it *analyticsDayIterator) Fetch() (interface{}, errors.Error) {
	if !it.HasNext() {
		return nil, nil
	}
	day := it.days[it.idx]
	it.idx++
	return &day, nil
}

func (it *analyticsDayIterator) Close() errors.Error {
	return nil
}

func analyticsDateIteratorForEndpoint(endpoint analyticsEndpoint, startingDate string, endingDate string) (helper.Iterator, errors.Error) {
	if !endpoint.DailyIterated {
		return nil, nil
	}
	return newAnalyticsDayIterator(startingDate, endingDate, endpoint.ExclusiveEndingDate)
}

func setDateQueryFromRequest(query url.Values, style dateParamStyle, startingDate string, endingDate string, reqData *helper.RequestData) {
	if reqData != nil {
		if day, ok := reqData.Input.(*analyticsDayInput); ok && day.StartingDate != "" {
			setDateQuery(query, style, day.StartingDate, day.EndingDate)
			return
		}
	}
	setDateQuery(query, style, startingDate, endingDate)
}

func setDateQuery(query url.Values, style dateParamStyle, startingDate string, endingDate string) {
	if style == dateParamStartingAt {
		query.Set("starting_at", startingDate)
		if endingDate != "" {
			query.Set("ending_at", endingDate)
		}
		return
	}
	if style == dateParamDate {
		query.Set("date", startingDate)
		return
	}
	query.Set("starting_date", startingDate)
	if endingDate != "" {
		query.Set("ending_date", endingDate)
	}
}

func setPaginationQuery(query url.Values, endpoint analyticsEndpoint, reqData *helper.RequestData) {
	if !endpoint.Paginated {
		return
	}
	query.Set("limit", strconv.Itoa(analyticsPageSize))
	if reqData == nil || reqData.CustomData == nil {
		return
	}
	if cursor, ok := reqData.CustomData.(string); ok && cursor != "" {
		query.Set("page", cursor)
	}
}

func effectiveOrganizationId(data *ClaudeEnterpriseTaskData) string {
	if data == nil {
		return ""
	}
	if data.Connection != nil && data.Connection.OrganizationId != "" {
		return data.Connection.OrganizationId
	}
	if data.Options != nil {
		return data.Options.OrganizationId
	}
	return ""
}

type analyticsRawParams struct {
	ConnectionId   uint64 `json:"connectionId"`
	ScopeId        string `json:"scopeId"`
	OrganizationId string `json:"organizationId"`
	Endpoint       string `json:"endpoint"`
	RequestDate    string `json:"-"`
}

func (p analyticsRawParams) GetParams() any {
	return p
}

// NewAnalyticsRawParams builds the identity parameters shared by every
// analytics collector/extractor (connection, scope, organization, endpoint).
// It is exported so raw-to-tool E2E snapshot tests in the sibling e2e package
// can drive the same BuildAnalyticsRecord/Build*Record functions production
// extractors use, without exposing the unexported analyticsRawParams type
// itself.
func NewAnalyticsRawParams(connectionId uint64, scopeId string, organizationId string, endpoint string) analyticsRawParams {
	return analyticsRawParams{
		ConnectionId:   connectionId,
		ScopeId:        scopeId,
		OrganizationId: organizationId,
		Endpoint:       endpoint,
	}
}

func endpointTaskSuffix(endpointName string) string {
	parts := strings.Split(endpointName, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}
