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

// ExtractPluginsMeta extracts plugins into both the raw-preserving
// _tool_claude_enterprise_analytics_records table and the typed
// _tool_claude_enterprise_plugins table. No ai_activities converter is added
// (Section 7): DevLake's domain model has no plugin concept.
var ExtractPluginsMeta = newExtractMeta(pluginsEndpoint, ExtractPlugins)

func ExtractPlugins(taskCtx plugin.SubTaskContext) errors.Error {
	return extractTypedAnalyticsEndpoint(taskCtx, pluginsEndpoint, func(raw []byte, params analyticsRawParams) (interface{}, errors.Error) {
		return BuildPluginAdoptionRecord(raw, params)
	})
}

// BuildPluginAdoptionRecord extracts a dashboard-ready daily plugin adoption
// row while keeping the full raw JSON available in the generic analytics
// record. See models.ClaudeEnterprisePluginAdoption for provisional-field
// documentation.
func BuildPluginAdoptionRecord(raw []byte, params analyticsRawParams) (*models.ClaudeEnterprisePluginAdoption, errors.Error) {
	var item map[string]interface{}
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, errors.Default.Wrap(err, "failed to parse Claude Enterprise plugin item")
	}

	date := firstNonEmpty(params.RequestDate, firstString(item, "date", "starting_date", "day"))
	if date == "" {
		return nil, errors.BadInput.New("Claude Enterprise plugin item is missing date")
	}
	pluginName := firstString(item, "plugin_name")
	if pluginName == "" {
		return nil, errors.BadInput.New("Claude Enterprise plugin item is missing plugin name")
	}

	return &models.ClaudeEnterprisePluginAdoption{
		ConnectionId:                             params.ConnectionId,
		ScopeId:                                  params.ScopeId,
		OrganizationId:                           params.OrganizationId,
		Date:                                     date,
		PluginId:                                 firstString(item, "plugin_id"),
		PluginName:                               pluginName,
		DistinctUserCount:                        firstInt64(item, "distinct_user_count"),
		InstallCount:                             firstInt64(item, "install_count", "installCount", "installs"),
		InvocationCount:                          firstInt64(item, "invocation_count"),
		ClaudeCodeDistinctSessionPluginUsedCount: firstInt64(item, "claude_code_metrics.distinct_session_plugin_used_count"),
		CoworkDistinctSessionPluginUsedCount:     firstInt64(item, "cowork_metrics.distinct_session_plugin_used_count"),
		PluginType:                               firstString(item, "plugin_type"),
		Publisher:                                firstString(item, "publisher"),
		ActiveUsers:                              firstInt(item, "active_users"),
		RawJson:                                  string(raw),
	}, nil
}
