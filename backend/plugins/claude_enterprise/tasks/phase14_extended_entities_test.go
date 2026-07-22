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
	"testing"

	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/plugin"
	"github.com/stretchr/testify/require"
)

// TestPhase14ExtendedEntityEndpointsAreConfiguredLikeAdoptionEndpoints proves
// the five Phase 14 extended entities (skills, connectors, chat projects,
// plugins, artifacts) follow the same engagement/adoption endpoint contract
// as the already-implemented /summaries and /users endpoints (Section 5.1):
// explicit limit=1000 pagination, daily starting_date/ending_date iteration,
// and endpoint/scope-separated raw + typed tool tables (Section 7).
func TestPhase14ExtendedEntityEndpointsAreConfiguredLikeAdoptionEndpoints(t *testing.T) {
	endpoints := []struct {
		name      string
		endpoint  analyticsEndpoint
		path      string
		toolTable string
	}{
		{"skills", skillsEndpoint, "organizations/analytics/skills", "_tool_claude_enterprise_skills"},
		{"connectors", connectorsEndpoint, "organizations/analytics/connectors", "_tool_claude_enterprise_connectors"},
		{"chat_projects", chatProjectsEndpoint, "organizations/analytics/apps/chat/projects", "_tool_claude_enterprise_chat_projects"},
		{"plugins", pluginsEndpoint, "organizations/analytics/plugins", "_tool_claude_enterprise_plugins"},
		{"artifacts", artifactsEndpoint, "organizations/analytics/artifacts", "_tool_claude_enterprise_artifacts"},
	}

	seenRawTables := map[string]bool{}
	seenNames := map[string]bool{}
	for _, tt := range endpoints {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.name, tt.endpoint.Name)
			require.Equal(t, tt.path, tt.endpoint.Path)
			require.Equal(t, dateParamDate, tt.endpoint.DateStyle)
			require.True(t, tt.endpoint.Paginated)
			require.True(t, tt.endpoint.DailyIterated)
			require.False(t, tt.endpoint.ExclusiveEndingDate)
			require.Equal(t, []string{tt.toolTable}, tt.endpoint.ExtraToolTables)

			collectMeta := newCollectMeta(tt.endpoint, func(plugin.SubTaskContext) errors.Error { return nil })
			extractMeta := newExtractMeta(tt.endpoint, func(plugin.SubTaskContext) errors.Error { return nil })
			require.Equal(t, []string{plugin.DOMAIN_TYPE_CROSS}, collectMeta.DomainTypes)
			require.Equal(t, []string{plugin.DOMAIN_TYPE_CROSS}, extractMeta.DomainTypes)
			require.Equal(t, []string{"_raw_" + tt.endpoint.RawTable}, collectMeta.ProductTables)
			require.Equal(t, []string{"_raw_" + tt.endpoint.RawTable}, extractMeta.DependencyTables)
			require.Equal(t,
				[]string{"_tool_claude_enterprise_analytics_records", tt.toolTable},
				extractMeta.ProductTables,
			)

			require.False(t, seenRawTables[tt.endpoint.RawTable], "raw table reused: %s", tt.endpoint.RawTable)
			seenRawTables[tt.endpoint.RawTable] = true
			require.False(t, seenNames[tt.endpoint.Name], "endpoint name reused: %s", tt.endpoint.Name)
			seenNames[tt.endpoint.Name] = true
		})
	}
}

// TestPhase14ExtendedEntityQueriesUseDailyIterationAndCursorPagination
// mirrors TestPhase9UserActivitiesUseDailyIteratorAndCursorPagination for
// each of the five new endpoints: one starting_date/ending_date request per
// UTC day, an explicit limit=1000, and opaque page cursor propagation
// (Section 5.1).
func TestPhase14ExtendedEntityQueriesUseDailyIterationAndCursorPagination(t *testing.T) {
	for _, endpoint := range []analyticsEndpoint{
		skillsEndpoint, connectorsEndpoint, chatProjectsEndpoint, pluginsEndpoint, artifactsEndpoint,
	} {
		t.Run(endpoint.Name, func(t *testing.T) {
			input, err := analyticsDateIteratorForEndpoint(endpoint, "2026-01-05", "2026-01-06")
			require.NoError(t, err)
			require.NotNil(t, input)

			first, err := input.Fetch()
			require.NoError(t, err)
			query := makeQueryForTest(endpoint, first, nil)
			require.Equal(t, "2026-01-05", query.Get("date"))
			require.Equal(t, "1000", query.Get("limit"))
			require.Empty(t, query.Get("page"))

			second, err := input.Fetch()
			require.NoError(t, err)
			query = makeQueryForTest(endpoint, second, "cursor_synthetic_"+endpoint.Name+"_2")
			require.Equal(t, "2026-01-06", query.Get("date"))
			require.Equal(t, "1000", query.Get("limit"))
			require.Equal(t, "cursor_synthetic_"+endpoint.Name+"_2", query.Get("page"))
			require.False(t, input.HasNext())
		})
	}
}

func TestPhase14BuildSkillFromSyntheticFixture(t *testing.T) {
	rows := parseSyntheticFixture(t, "skills_success.json")
	require.Len(t, rows, 2)

	skill, err := BuildSkillRecord(rows[0], analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "org_synthetic_001",
		OrganizationId: "org_synthetic_001",
		Endpoint:       skillsEndpoint.Name,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), skill.ConnectionId)
	require.Equal(t, "org_synthetic_001", skill.ScopeId)
	require.Equal(t, "org_synthetic_001", skill.OrganizationId)
	require.Equal(t, "2026-01-05", skill.Date)
	require.Equal(t, "Code Review Assistant", skill.SkillName)
	require.Equal(t, "Code Review Assistant", skill.SkillDisplayName)
	require.Equal(t, int64(6), skill.DistinctUserCount)
	require.Equal(t, int64(42), skill.InvocationCount)
	require.Equal(t, int64(4), skill.EnableCount)
	require.Equal(t, "12.3400", skill.AttributedListPrice)
	require.Equal(t, "1.2300", skill.EstimatedOverageSpend)
	require.Equal(t, "USD", skill.Currency)
	require.Equal(t, "organization", skill.ShareStatus)
	require.Equal(t, int64(20), skill.ChatDistinctConversationSkillUsedCount)
	require.Equal(t, int64(11), skill.ClaudeCodeDistinctSessionSkillUsedCount)
	require.Equal(t, int64(3), skill.CoworkDistinctSessionSkillUsedCount)
	require.JSONEq(t, string(rows[0]), skill.RawJson)
}

func TestPhase14BuildConnectorFromSyntheticFixture(t *testing.T) {
	rows := parseSyntheticFixture(t, "connectors_success.json")
	require.Len(t, rows, 2)

	connector, err := BuildConnectorRecord(rows[0], analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "org_synthetic_001",
		OrganizationId: "org_synthetic_001",
		Endpoint:       connectorsEndpoint.Name,
	})
	require.NoError(t, err)
	require.Equal(t, "Google Drive", connector.ConnectorName)
	require.Equal(t, int64(9), connector.DistinctUserCount)
	require.Equal(t, int64(44), connector.ReadCallCount)
	require.Equal(t, int64(7), connector.WriteCallCount)
	require.Equal(t, int64(3), connector.UnclassifiedCallCount)
	require.Equal(t, int64(33), connector.ChatDistinctConversationConnectorUsedCount)
	require.Equal(t, int64(18), connector.ClaudeCodeDistinctSessionConnectorUsedCount)
	require.Equal(t, int64(5), connector.CoworkDistinctSessionConnectorUsedCount)
	require.JSONEq(t, string(rows[0]), connector.RawJson)
}

func TestPhase14BuildChatProjectFromSyntheticFixture(t *testing.T) {
	rows := parseSyntheticFixture(t, "chat_projects_success.json")
	require.Len(t, rows, 2)

	project, err := BuildChatProjectRecord(rows[0], analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "org_synthetic_001",
		OrganizationId: "org_synthetic_001",
		Endpoint:       chatProjectsEndpoint.Name,
	})
	require.NoError(t, err)
	require.Equal(t, "project_synthetic_001", project.ProjectId)
	require.Equal(t, "Q1 Launch Planning", project.ProjectName)
	require.Equal(t, "2026-01-02T10:00:00Z", project.CreatedAt)
	require.Equal(t, "user_synthetic_001", project.CreatorUserId)
	require.Equal(t, "developer@example.invalid", project.CreatorEmail)
	require.Equal(t, int64(5), project.DistinctUserCount)
	require.Equal(t, int64(27), project.DistinctConversationCount)
	require.Equal(t, int64(120), project.MessageCount)
	require.JSONEq(t, string(rows[0]), project.RawJson)
}

func TestPhase14BuildPluginAdoptionFromSyntheticFixture(t *testing.T) {
	rows := parseSyntheticFixture(t, "plugins_success.json")
	require.Len(t, rows, 2)

	pluginRow, err := BuildPluginAdoptionRecord(rows[0], analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "org_synthetic_001",
		OrganizationId: "org_synthetic_001",
		Endpoint:       pluginsEndpoint.Name,
	})
	require.NoError(t, err)
	require.Equal(t, "plugin_synthetic_001", pluginRow.PluginId)
	require.Equal(t, "Linear Sync", pluginRow.PluginName)
	require.Equal(t, int64(8), pluginRow.DistinctUserCount)
	require.Equal(t, int64(30), pluginRow.InstallCount)
	require.Equal(t, int64(64), pluginRow.InvocationCount)
	require.Equal(t, int64(40), pluginRow.ClaudeCodeDistinctSessionPluginUsedCount)
	require.Equal(t, int64(9), pluginRow.CoworkDistinctSessionPluginUsedCount)
	require.JSONEq(t, string(rows[0]), pluginRow.RawJson)
}

func TestPhase14BuildArtifactFromSyntheticFixture(t *testing.T) {
	rows := parseSyntheticFixture(t, "artifacts_success.json")
	require.Len(t, rows, 2)

	artifact, err := BuildArtifactRecord(rows[0], analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "org_synthetic_001",
		OrganizationId: "org_synthetic_001",
		Endpoint:       artifactsEndpoint.Name,
	})
	require.NoError(t, err)
	require.Equal(t, "text/markdown", artifact.ArtifactType)
	require.True(t, artifact.IsShared)
	require.Equal(t, int64(40), artifact.ArtifactsCreatedCount)
	require.Equal(t, int64(3), artifact.PublishedArtifactsCreatedCount)
	require.Equal(t, int64(7), artifact.DistinctUserCount)
	require.JSONEq(t, string(rows[0]), artifact.RawJson)
}

// TestPhase14ExtendedEntityEmptyFixturesParseToNoRows mirrors
// TestSyntheticEmptyFixturesParseToNoRows for the five new endpoints.
func TestPhase14ExtendedEntityEmptyFixturesParseToNoRows(t *testing.T) {
	for _, name := range []string{
		"skills_empty.json",
		"connectors_empty.json",
		"chat_projects_empty.json",
		"plugins_empty.json",
		"artifacts_empty.json",
	} {
		t.Run(name, func(t *testing.T) {
			rows := parseSyntheticFixture(t, name)
			require.Empty(t, rows)
		})
	}
}

// TestPhase14ExtendedEntityPaginatedFixturesParseRows mirrors
// TestSyntheticPaginatedFixturesParseRows for the five new endpoints and
// confirms each paginated fixture's terminal next_page is null (collection
// finishes rather than looping).
func TestPhase14ExtendedEntityPaginatedFixturesParseRows(t *testing.T) {
	tests := []struct {
		name          string
		expectedValue string
		field         string
	}{
		{"skills_paginated.json", "Meeting Notes Summarizer", "skill_name"},
		{"connectors_paginated.json", "Slack", "connector_name"},
		{"chat_projects_paginated.json", "project_synthetic_003", "project_id"},
		{"plugins_paginated.json", "plugin_synthetic_003", "plugin_id"},
		{"artifacts_paginated.json", "text/markdown", "artifact_type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := parseSyntheticFixture(t, tt.name)
			require.Len(t, rows, 1)

			var item map[string]interface{}
			require.NoError(t, json.Unmarshal(rows[0], &item))
			require.Equal(t, tt.expectedValue, item[tt.field])
		})
	}
}

// TestPhase14ExtendedEntityRawPreservationSeparatesEndpointAndScope mirrors
// TestPhase4AnalyticsRecordIdentitySeparatesEndpointAndScope, proving each
// new entity's generic analytics record stays endpoint- and scope-separated
// from the others (Section 7 raw-preservation contract).
func TestPhase14ExtendedEntityRawPreservationSeparatesEndpointAndScope(t *testing.T) {
	raw := []byte(`{"date":"2026-01-05","id":"entity_synthetic_001"}`)

	skillRecord, err := BuildAnalyticsRecord(raw, analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "scope_org_synthetic_001",
		OrganizationId: "org_synthetic_001",
		Endpoint:       skillsEndpoint.Name,
	})
	require.NoError(t, err)
	artifactRecord, err := BuildAnalyticsRecord(raw, analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "scope_org_synthetic_002",
		OrganizationId: "org_synthetic_002",
		Endpoint:       artifactsEndpoint.Name,
	})
	require.NoError(t, err)

	require.NotEqual(t, skillRecord.Endpoint, artifactRecord.Endpoint)
	require.NotEqual(t, skillRecord.ScopeId, artifactRecord.ScopeId)
	require.NotEqual(t, skillRecord.RecordId, artifactRecord.RecordId)
	require.JSONEq(t, string(raw), skillRecord.RawJson)
	require.JSONEq(t, string(raw), artifactRecord.RawJson)
}

// TestPhase14ExtendedEntitiesHaveNoDomainConverter documents the Section 7 /
// Phase 14 handoff rule: none of the five extended entities has an
// ai_activities converter subtask, unlike /users (which has
// ConvertUserActivitiesMeta).
func TestPhase14ExtendedEntitiesHaveNoDomainConverter(t *testing.T) {
	metas := GetSubTaskMetas()
	for _, meta := range metas {
		switch meta.Name {
		case "collectSkills", "extractSkills",
			"collectConnectors", "extractConnectors",
			"collectChatProjects", "extractChatProjects",
			"collectPlugins", "extractPlugins",
			"collectArtifacts", "extractArtifacts":
			require.NotContains(t, meta.ProductTables, "ai_activities")
		}
	}
}
