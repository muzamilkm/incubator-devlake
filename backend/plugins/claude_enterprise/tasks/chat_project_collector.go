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
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/plugin"
)

// RawChatProjectsTable is the raw table storing
// GET /v1/organizations/analytics/apps/chat/projects items. Section 4.2
// documents this endpoint under the nested "apps/chat" path, unlike the
// other extended endpoints which sit directly under "analytics/".
const RawChatProjectsTable = "claude_enterprise_api_chat_projects"

var chatProjectsEndpoint = analyticsEndpoint{
	Name:            "chat_projects",
	RawTable:        RawChatProjectsTable,
	Path:            "organizations/analytics/apps/chat/projects",
	Description:     "Claude Enterprise chat project adoption",
	DateStyle:       dateParamDate,
	Paginated:       true,
	DailyIterated:   true,
	ExtraToolTables: []string{"_tool_claude_enterprise_chat_projects"},
}

// CollectChatProjectsMeta collects the Claude Enterprise chat projects
// endpoint. It is independently selectable/testable per the Phase 14 exit
// criterion.
var CollectChatProjectsMeta = newCollectMeta(chatProjectsEndpoint, CollectChatProjects)

// CollectChatProjects mirrors the /summaries and /users collector pattern:
// explicit limit=1000, daily starting_date/ending_date iteration, and cursor
// pagination via the shared collectAnalyticsEndpoint helper.
func CollectChatProjects(taskCtx plugin.SubTaskContext) errors.Error {
	return collectAnalyticsEndpoint(taskCtx, chatProjectsEndpoint)
}
