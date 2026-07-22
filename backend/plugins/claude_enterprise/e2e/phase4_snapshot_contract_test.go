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

package e2e

import (
	"testing"

	"github.com/apache/incubator-devlake/plugins/claude_enterprise/tasks"
	"github.com/stretchr/testify/require"
)

// TestPhase4E2ESnapshotContract began as a Phase 4 compile-safe placeholder
// ("add raw-to-tool snapshot fixtures once converter/domain behavior is
// implemented"). Summary raw-to-tool behavior has existed since Phase 10, so
// this now asserts the real raw-to-tool contract for one MVP entity
// end-to-end from a synthetic fixture: the collector's raw JSON parses into
// both the raw-preserving generic analytics record and the typed dashboard
// tool row, scoped to one connection/organization. The five Phase 14
// extended entities have their own dedicated snapshot tests in
// phase14_extended_entities_snapshot_test.go.
func TestPhase4E2ESnapshotContract(t *testing.T) {
	rows := readSyntheticFixtureRows(t, "summaries_success.json")
	require.Len(t, rows, 1)

	params := tasks.NewAnalyticsRawParams(1, "scope_org_synthetic_001", "org_synthetic_001", "summaries")

	record, err := tasks.BuildAnalyticsRecord(rows[0], params)
	require.NoError(t, err)
	require.Equal(t, "summaries", record.Endpoint)
	require.Equal(t, "scope_org_synthetic_001", record.ScopeId)
	require.Equal(t, "org_synthetic_001", record.OrganizationId)
	require.JSONEq(t, string(rows[0]), record.RawJson)

	summary, err := tasks.BuildSummaryRecord(rows[0], params)
	require.NoError(t, err)
	require.Equal(t, uint64(1), summary.ConnectionId)
	require.Equal(t, "org_synthetic_001", summary.OrganizationId)
	require.Equal(t, "2026-01-05", summary.Date)
	require.Equal(t, 42, summary.AssignedSeatCount)
	require.Equal(t, 18, summary.DailyActiveUserCount)
	require.JSONEq(t, string(rows[0]), summary.RawJson)
}
