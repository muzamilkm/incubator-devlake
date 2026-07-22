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

// These tests are the Phase 14 raw-to-tool E2E snapshots required by the
// implementation plan for each extended adoption entity (skills, connectors,
// chat projects, plugins, artifacts). Each one takes one synthetic success
// fixture row through the same two steps a real pipeline run performs:
//  1. BuildAnalyticsRecord: the raw-preserving generic tool row written to
//     _tool_claude_enterprise_analytics_records.
//  2. The entity's typed Build*Record: the dashboard-ready row written to
//     its dedicated _tool_claude_enterprise_<entity> table.
//
// No ai_activities conversion step exists for these entities (Section 7),
// so raw-to-tool is the complete pipeline snapshot for this phase.

func TestPhase14SkillsRawToToolSnapshot(t *testing.T) {
	rows := readSyntheticFixtureRows(t, "skills_success.json")
	require.Len(t, rows, 2)

	params := tasks.NewAnalyticsRawParams(1, "scope_org_synthetic_001", "org_synthetic_001", "skills")

	record, err := tasks.BuildAnalyticsRecord(rows[0], params)
	require.NoError(t, err)
	require.Equal(t, "skills", record.Endpoint)
	require.Equal(t, "scope_org_synthetic_001", record.ScopeId)
	require.Equal(t, "org_synthetic_001", record.OrganizationId)
	require.JSONEq(t, string(rows[0]), record.RawJson)

	skill, err := tasks.BuildSkillRecord(rows[0], params)
	require.NoError(t, err)
	require.Equal(t, uint64(1), skill.ConnectionId)
	require.Equal(t, "org_synthetic_001", skill.OrganizationId)
	require.Equal(t, "Code Review Assistant", skill.SkillName)
	require.Equal(t, int64(6), skill.DistinctUserCount)
	require.Equal(t, int64(42), skill.InvocationCount)
	require.JSONEq(t, string(rows[0]), skill.RawJson)
}

func TestPhase14ConnectorsRawToToolSnapshot(t *testing.T) {
	rows := readSyntheticFixtureRows(t, "connectors_success.json")
	require.Len(t, rows, 2)

	params := tasks.NewAnalyticsRawParams(1, "scope_org_synthetic_001", "org_synthetic_001", "connectors")

	record, err := tasks.BuildAnalyticsRecord(rows[0], params)
	require.NoError(t, err)
	require.Equal(t, "connectors", record.Endpoint)
	require.JSONEq(t, string(rows[0]), record.RawJson)

	connector, err := tasks.BuildConnectorRecord(rows[0], params)
	require.NoError(t, err)
	require.Equal(t, uint64(1), connector.ConnectionId)
	require.Equal(t, "org_synthetic_001", connector.OrganizationId)
	require.Equal(t, "Google Drive", connector.ConnectorName)
	require.Equal(t, int64(9), connector.DistinctUserCount)
	require.Equal(t, int64(44), connector.ReadCallCount)
	require.JSONEq(t, string(rows[0]), connector.RawJson)
}

func TestPhase14ChatProjectsRawToToolSnapshot(t *testing.T) {
	rows := readSyntheticFixtureRows(t, "chat_projects_success.json")
	require.Len(t, rows, 2)

	params := tasks.NewAnalyticsRawParams(1, "scope_org_synthetic_001", "org_synthetic_001", "chat_projects")

	record, err := tasks.BuildAnalyticsRecord(rows[0], params)
	require.NoError(t, err)
	require.Equal(t, "chat_projects", record.Endpoint)
	require.JSONEq(t, string(rows[0]), record.RawJson)

	project, err := tasks.BuildChatProjectRecord(rows[0], params)
	require.NoError(t, err)
	require.Equal(t, uint64(1), project.ConnectionId)
	require.Equal(t, "org_synthetic_001", project.OrganizationId)
	require.Equal(t, "project_synthetic_001", project.ProjectId)
	require.Equal(t, "Q1 Launch Planning", project.ProjectName)
	require.Equal(t, int64(5), project.DistinctUserCount)
	require.Equal(t, int64(27), project.DistinctConversationCount)
	require.JSONEq(t, string(rows[0]), project.RawJson)
}

func TestPhase14PluginsRawToToolSnapshot(t *testing.T) {
	rows := readSyntheticFixtureRows(t, "plugins_success.json")
	require.Len(t, rows, 2)

	params := tasks.NewAnalyticsRawParams(1, "scope_org_synthetic_001", "org_synthetic_001", "plugins")

	record, err := tasks.BuildAnalyticsRecord(rows[0], params)
	require.NoError(t, err)
	require.Equal(t, "plugins", record.Endpoint)
	require.JSONEq(t, string(rows[0]), record.RawJson)

	pluginRow, err := tasks.BuildPluginAdoptionRecord(rows[0], params)
	require.NoError(t, err)
	require.Equal(t, uint64(1), pluginRow.ConnectionId)
	require.Equal(t, "org_synthetic_001", pluginRow.OrganizationId)
	require.Equal(t, "plugin_synthetic_001", pluginRow.PluginId)
	require.Equal(t, "Linear Sync", pluginRow.PluginName)
	require.Equal(t, int64(8), pluginRow.DistinctUserCount)
	require.Equal(t, int64(30), pluginRow.InstallCount)
	require.JSONEq(t, string(rows[0]), pluginRow.RawJson)
}

func TestPhase14ArtifactsRawToToolSnapshot(t *testing.T) {
	rows := readSyntheticFixtureRows(t, "artifacts_success.json")
	require.Len(t, rows, 2)

	params := tasks.NewAnalyticsRawParams(1, "scope_org_synthetic_001", "org_synthetic_001", "artifacts")

	record, err := tasks.BuildAnalyticsRecord(rows[0], params)
	require.NoError(t, err)
	require.Equal(t, "artifacts", record.Endpoint)
	require.JSONEq(t, string(rows[0]), record.RawJson)

	artifact, err := tasks.BuildArtifactRecord(rows[0], params)
	require.NoError(t, err)
	require.Equal(t, uint64(1), artifact.ConnectionId)
	require.Equal(t, "org_synthetic_001", artifact.OrganizationId)
	require.Equal(t, "text/markdown", artifact.ArtifactType)
	require.True(t, artifact.IsShared)
	require.Equal(t, int64(40), artifact.ArtifactsCreatedCount)
	require.Equal(t, int64(3), artifact.PublishedArtifactsCreatedCount)
	require.JSONEq(t, string(rows[0]), artifact.RawJson)
}

// TestPhase14ExtendedEntityEmptyFixturesYieldNoRows confirms the empty-case
// fixtures for all five entities decode to zero rows, matching the
// zero-data dashboard/reconciliation case called out in the Phase 13
// handoff notes and reused here for Phase 14.
func TestPhase14ExtendedEntityEmptyFixturesYieldNoRows(t *testing.T) {
	for _, name := range []string{
		"skills_empty.json",
		"connectors_empty.json",
		"chat_projects_empty.json",
		"plugins_empty.json",
		"artifacts_empty.json",
	} {
		t.Run(name, func(t *testing.T) {
			rows := readSyntheticFixtureRows(t, name)
			require.Empty(t, rows)
		})
	}
}
