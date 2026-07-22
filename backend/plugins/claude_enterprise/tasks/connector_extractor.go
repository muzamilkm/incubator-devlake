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

// ExtractConnectorsMeta extracts connectors into both the raw-preserving
// _tool_claude_enterprise_analytics_records table and the typed
// _tool_claude_enterprise_connectors table. No ai_activities converter is
// added (Section 7): DevLake's domain model has no connector concept.
var ExtractConnectorsMeta = newExtractMeta(connectorsEndpoint, ExtractConnectors)

func ExtractConnectors(taskCtx plugin.SubTaskContext) errors.Error {
	return extractTypedAnalyticsEndpoint(taskCtx, connectorsEndpoint, func(raw []byte, params analyticsRawParams) (interface{}, errors.Error) {
		return BuildConnectorRecord(raw, params)
	})
}

// BuildConnectorRecord extracts a dashboard-ready daily connector adoption
// row while keeping the full raw JSON available in the generic analytics
// record. See models.ClaudeEnterpriseConnector for provisional-field
// documentation.
func BuildConnectorRecord(raw []byte, params analyticsRawParams) (*models.ClaudeEnterpriseConnector, errors.Error) {
	var item map[string]interface{}
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, errors.Default.Wrap(err, "failed to parse Claude Enterprise connector item")
	}

	date := firstNonEmpty(params.RequestDate, firstString(item, "date", "starting_date", "day"))
	if date == "" {
		return nil, errors.BadInput.New("Claude Enterprise connector item is missing date")
	}
	connectorName := firstString(item, "connector_name")
	if connectorName == "" {
		return nil, errors.BadInput.New("Claude Enterprise connector item is missing connector name")
	}

	return &models.ClaudeEnterpriseConnector{
		ConnectionId:          params.ConnectionId,
		ScopeId:               params.ScopeId,
		OrganizationId:        params.OrganizationId,
		Date:                  date,
		ConnectorName:         connectorName,
		DistinctUserCount:     firstInt64(item, "distinct_user_count"),
		ReadCallCount:         firstInt64(item, "read_call_count"),
		WriteCallCount:        firstInt64(item, "write_call_count"),
		UnclassifiedCallCount: firstInt64(item, "unclassified_call_count"),
		ChatDistinctConversationConnectorUsedCount:  firstInt64(item, "chat_metrics.distinct_conversation_connector_used_count"),
		ClaudeCodeDistinctSessionConnectorUsedCount: firstInt64(item, "claude_code_metrics.distinct_session_connector_used_count"),
		CoworkDistinctSessionConnectorUsedCount:     firstInt64(item, "cowork_metrics.distinct_session_connector_used_count"),
		RawJson:                                     string(raw),
	}, nil
}
