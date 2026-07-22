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

// RawConnectorsTable is the raw table storing
// GET /v1/organizations/analytics/connectors items (Section 4.2).
const RawConnectorsTable = "claude_enterprise_api_connectors"

var connectorsEndpoint = analyticsEndpoint{
	Name:            "connectors",
	RawTable:        RawConnectorsTable,
	Path:            "organizations/analytics/connectors",
	Description:     "Claude Enterprise connector adoption",
	DateStyle:       dateParamDate,
	Paginated:       true,
	DailyIterated:   true,
	ExtraToolTables: []string{"_tool_claude_enterprise_connectors"},
}

// CollectConnectorsMeta collects the Claude Enterprise connectors endpoint.
// It is independently selectable/testable per the Phase 14 exit criterion.
var CollectConnectorsMeta = newCollectMeta(connectorsEndpoint, CollectConnectors)

// CollectConnectors mirrors the /summaries and /users collector pattern:
// explicit limit=1000, daily starting_date/ending_date iteration, and cursor
// pagination via the shared collectAnalyticsEndpoint helper.
func CollectConnectors(taskCtx plugin.SubTaskContext) errors.Error {
	return collectAnalyticsEndpoint(taskCtx, connectorsEndpoint)
}
