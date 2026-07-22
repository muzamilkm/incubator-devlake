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

	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/plugin"
	"github.com/apache/incubator-devlake/plugins/claude_enterprise/models"
)

// ExtractSkillsMeta extracts skills into both the raw-preserving
// _tool_claude_enterprise_analytics_records table and the typed
// _tool_claude_enterprise_skills table. No ai_activities converter is added
// (Section 7): DevLake's domain model has no skill concept.
var ExtractSkillsMeta = newExtractMeta(skillsEndpoint, ExtractSkills)

func ExtractSkills(taskCtx plugin.SubTaskContext) errors.Error {
	return extractTypedAnalyticsEndpoint(taskCtx, skillsEndpoint, func(raw []byte, params analyticsRawParams) (interface{}, errors.Error) {
		return BuildSkillRecord(raw, params)
	})
}

// BuildSkillRecord extracts a dashboard-ready daily skill adoption row while
// keeping the full raw JSON available in the generic analytics record. See
// models.ClaudeEnterpriseSkill for provisional-field documentation.
func BuildSkillRecord(raw []byte, params analyticsRawParams) (*models.ClaudeEnterpriseSkill, errors.Error) {
	var item map[string]interface{}
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, errors.Default.Wrap(err, "failed to parse Claude Enterprise skill item")
	}

	date := firstNonEmpty(params.RequestDate, firstString(item, "date", "starting_date", "day"))
	if date == "" {
		return nil, errors.BadInput.New("Claude Enterprise skill item is missing date")
	}
	skillName := firstString(item, "skill_name")
	if skillName == "" {
		return nil, errors.BadInput.New("Claude Enterprise skill item is missing skill name")
	}

	return &models.ClaudeEnterpriseSkill{
		ConnectionId:                            params.ConnectionId,
		ScopeId:                                 params.ScopeId,
		OrganizationId:                          params.OrganizationId,
		Date:                                    date,
		SkillName:                               skillName,
		SkillDisplayName:                        firstString(item, "skill_display_name"),
		DistinctUserCount:                       firstInt64(item, "distinct_user_count"),
		InvocationCount:                         firstInt64(item, "invocation_count"),
		EnableCount:                             firstInt64(item, "enable_count"),
		AttributedListPrice:                     firstDecimalString(item, "attributed_list_price"),
		EstimatedOverageSpend:                   firstDecimalString(item, "estimated_overage_spend"),
		Currency:                                firstString(item, "currency"),
		ShareStatus:                             firstString(item, "share_status"),
		ChatDistinctConversationSkillUsedCount:  firstInt64(item, "chat_metrics.distinct_conversation_skill_used_count"),
		ClaudeCodeDistinctSessionSkillUsedCount: firstInt64(item, "claude_code_metrics.distinct_session_skill_used_count"),
		CoworkDistinctSessionSkillUsedCount:     firstInt64(item, "cowork_metrics.distinct_session_skill_used_count"),
		RawJson:                                 string(raw),
	}, nil
}
