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
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/models/domainlayer/didgen"
	"github.com/apache/incubator-devlake/core/plugin"
	helper "github.com/apache/incubator-devlake/helpers/pluginhelper/api"
	"github.com/apache/incubator-devlake/plugins/claude_enterprise/models"
	"github.com/stretchr/testify/require"
)

type mockClaudeEnterprisePlugin struct{}

func (m mockClaudeEnterprisePlugin) Description() string { return "" }
func (m mockClaudeEnterprisePlugin) Name() string        { return "claude_enterprise" }
func (m mockClaudeEnterprisePlugin) RootPkgPath() string {
	return "github.com/apache/incubator-devlake/plugins/claude_enterprise"
}

func init() {
	plugin.RegisterPlugin("claude_enterprise", mockClaudeEnterprisePlugin{})
}

func TestPhase3ArchitectureContract(t *testing.T) {
	require.Equal(t, "claude_enterprise_api_summaries", RawSummariesTable)
	require.Equal(t, "claude_enterprise_api_users", RawUserActivitiesTable)
	require.Equal(t, "claude_enterprise_api_user_usage_report", RawUserUsageReportTable)
	require.Equal(t, "claude_enterprise_api_user_cost_report", RawUserCostReportTable)

	endpoints := []analyticsEndpoint{
		summariesEndpoint,
		userActivitiesEndpoint,
		userUsageReportEndpoint,
		userCostReportEndpoint,
	}
	for _, endpoint := range endpoints {
		t.Run(endpoint.Name, func(t *testing.T) {
			collectMeta := newCollectMeta(endpoint, func(plugin.SubTaskContext) errors.Error { return nil })
			extractMeta := newExtractMeta(endpoint, func(plugin.SubTaskContext) errors.Error { return nil })

			require.Equal(t, []string{plugin.DOMAIN_TYPE_CROSS}, collectMeta.DomainTypes)
			require.Equal(t, []string{plugin.DOMAIN_TYPE_CROSS}, extractMeta.DomainTypes)
			require.Equal(t, []string{"_raw_" + endpoint.RawTable}, collectMeta.ProductTables)
			require.Equal(t, []string{"_raw_" + endpoint.RawTable}, extractMeta.DependencyTables)
			expectedProductTables := []string{"_tool_claude_enterprise_analytics_records"}
			if endpoint.Name == summariesEndpoint.Name {
				expectedProductTables = append(expectedProductTables, "_tool_claude_enterprise_summaries")
			}
			if endpoint.Name == userUsageReportEndpoint.Name {
				expectedProductTables = append(expectedProductTables, "_tool_claude_enterprise_usage_reports")
			}
			if endpoint.Name == userCostReportEndpoint.Name {
				expectedProductTables = append(expectedProductTables, "_tool_claude_enterprise_cost_reports")
			}
			require.Equal(t, expectedProductTables, extractMeta.ProductTables)
		})
	}

	require.Equal(t, dateParamStartingDate, summariesEndpoint.DateStyle)
	require.Equal(t, dateParamDate, userActivitiesEndpoint.DateStyle)
	require.Equal(t, dateParamStartingAt, userUsageReportEndpoint.DateStyle)
	require.Equal(t, dateParamStartingAt, userCostReportEndpoint.DateStyle)
	require.False(t, summariesEndpoint.Paginated)
	require.True(t, userActivitiesEndpoint.Paginated)
	require.True(t, userUsageReportEndpoint.Paginated)
	require.True(t, userCostReportEndpoint.Paginated)
}

func TestBuildAnalyticsRecord(t *testing.T) {
	raw := []byte(`{"date":"2026-01-05","grain":"daily","user":{"id":"user_synthetic_001","email_address":"dev@example.invalid"},"product":"claude_code","model":"claude-sonnet-4"}`)
	params := analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "org_1",
		OrganizationId: "org_1",
		Endpoint:       userActivitiesEndpoint.Name,
	}

	record, err := BuildAnalyticsRecord(raw, params)
	require.NoError(t, err)
	require.Equal(t, uint64(1), record.ConnectionId)
	require.Equal(t, "org_1", record.ScopeId)
	require.Equal(t, "users", record.Endpoint)
	require.Equal(t, "2026-01-05", record.Date)
	require.Equal(t, "daily", record.Grain)
	require.Equal(t, "user_synthetic_001", record.UserId)
	require.Equal(t, "dev@example.invalid", record.UserEmail)
	require.Equal(t, "claude_code", record.Product)
	require.Equal(t, "claude-sonnet-4", record.Model)
	require.NotEmpty(t, record.RecordId)
	require.JSONEq(t, string(raw), record.RawJson)
}

func TestPhase4AnalyticsRecordIdentitySeparatesEndpointAndScope(t *testing.T) {
	raw := []byte(`{"date":"2026-01-05","grain":"daily","user":{"id":"user_synthetic_001","email_address":"dev@example.invalid"},"product":"claude_code","model":"claude-sonnet-4"}`)

	summary, err := BuildAnalyticsRecord(raw, analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "scope_org_synthetic_001",
		OrganizationId: "org_synthetic_001",
		Endpoint:       summariesEndpoint.Name,
	})
	require.NoError(t, err)
	usage, err := BuildAnalyticsRecord(raw, analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "scope_org_synthetic_002",
		OrganizationId: "org_synthetic_002",
		Endpoint:       userUsageReportEndpoint.Name,
	})
	require.NoError(t, err)

	require.NotEqual(t, summary.Endpoint, usage.Endpoint)
	require.NotEqual(t, summary.ScopeId, usage.ScopeId)
	require.NotEqual(t, summary.OrganizationId, usage.OrganizationId)
	require.NotEqual(t, summary.RecordId, usage.RecordId)
	require.JSONEq(t, string(raw), summary.RawJson)
	require.JSONEq(t, string(raw), usage.RawJson)
}

func TestSyntheticSuccessFixturesParseIntoAnalyticsRecords(t *testing.T) {
	tests := []struct {
		name          string
		fixture       string
		endpoint      analyticsEndpoint
		expectedCount int
	}{
		{
			name:          "summaries",
			fixture:       "summaries_success.json",
			endpoint:      summariesEndpoint,
			expectedCount: 1,
		},
		{
			name:          "users",
			fixture:       "users_success.json",
			endpoint:      userActivitiesEndpoint,
			expectedCount: 2,
		},
		{
			name:          "user usage report",
			fixture:       "user_usage_report_success.json",
			endpoint:      userUsageReportEndpoint,
			expectedCount: 1,
		},
		{
			name:          "user cost report",
			fixture:       "user_cost_report_success.json",
			endpoint:      userCostReportEndpoint,
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := parseSyntheticFixture(t, tt.fixture)
			require.Len(t, rows, tt.expectedCount)

			params := analyticsRawParams{
				ConnectionId:   1,
				ScopeId:        "org_synthetic_001",
				OrganizationId: "org_synthetic_001",
				Endpoint:       tt.endpoint.Name,
			}
			for _, row := range rows {
				record, err := BuildAnalyticsRecord(row, params)
				require.NoError(t, err)
				require.Equal(t, tt.endpoint.Name, record.Endpoint)
				require.NotEmpty(t, record.RecordId)
				require.JSONEq(t, string(row), record.RawJson)
			}
		})
	}
}

func TestSyntheticEmptyFixturesParseToNoRows(t *testing.T) {
	for _, name := range []string{
		"summaries_empty.json",
		"users_empty.json",
		"user_usage_report_empty.json",
		"user_cost_report_empty.json",
	} {
		t.Run(name, func(t *testing.T) {
			rows := parseSyntheticFixture(t, name)
			require.Empty(t, rows)
		})
	}
}

func TestSyntheticPaginatedFixturesParseRows(t *testing.T) {
	tests := []struct {
		name        string
		expectedRow string
	}{
		{
			name:        "users_paginated.json",
			expectedRow: `{"date":"2026-01-06","grain":"daily","user":{"id":"user_synthetic_003","email_address":"reviewer@example.invalid"},"cowork_metrics":{"distinct_session_count":2,"message_count":12}}`,
		},
		{
			name:        "user_usage_report_paginated.json",
			expectedRow: `{"starting_at":"2026-01-06T00:00:00Z","ending_at":"2026-01-07T00:00:00Z","data_refreshed_at":"2026-01-07T04:00:00Z","organization_id":"org_synthetic_001","actor":{"user_id":"user_synthetic_002","email":"analyst@example.invalid","deleted":false},"product":"chat","model":"claude-opus-4","context_window":"0-200k","inference_geo":"global","speed":"standard","uncached_input_tokens":23456,"output_tokens":7890,"cache_read_input_tokens":333,"cache_creation":{"ephemeral_1h_input_tokens":444,"ephemeral_5m_input_tokens":555},"total_tokens":31789,"requests":11,"server_tool_use":{"web_search_requests":1}}`,
		},
		{
			name:        "user_cost_report_paginated.json",
			expectedRow: `{"starting_at":"2026-01-06T00:00:00Z","ending_at":"2026-01-07T00:00:00Z","data_refreshed_at":"2026-01-07T04:00:00Z","organization_id":"org_synthetic_001","actor":{"user_id":"user_synthetic_002","email":"analyst@example.invalid","deleted":false},"product":"chat","model":"claude-opus-4","context_window":"0-200k","inference_geo":"global","speed":"standard","cost_type":"tokens","token_type":"input","amount":"45.6789","list_amount":"60.0000","currency":"USD","requests":11}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := parseSyntheticFixture(t, tt.name)
			require.Len(t, rows, 1)
			require.JSONEq(t, tt.expectedRow, string(rows[0]))
		})
	}
}

func TestSyntheticPaginationCursorAssumptions(t *testing.T) {
	tests := []struct {
		name           string
		expectedCursor interface{}
	}{
		{name: "users_success.json", expectedCursor: "cursor_synthetic_users_2"},
		{name: "user_usage_report_success.json", expectedCursor: "cursor_synthetic_usage_2"},
		{name: "user_cost_report_success.json", expectedCursor: helperFinishCollect{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := readSyntheticFixture(t, tt.name)
			cursor, err := parseAnalyticsNextPage(nil, &http.Response{Body: io.NopCloser(bytesReader(raw))})
			if _, done := tt.expectedCursor.(helperFinishCollect); done {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedCursor, cursor)
		})
	}
}

func TestAnalyticsResponseParsersCloseResponseBodies(t *testing.T) {
	t.Run("response parser", func(t *testing.T) {
		body := &trackingReadCloser{Reader: bytesReader([]byte(`[{"date":"2026-01-05"}]`))}

		rows, err := parseAnalyticsResponse(&http.Response{Body: body})

		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.True(t, body.closed)
	})

	t.Run("pagination parser", func(t *testing.T) {
		body := &trackingReadCloser{Reader: bytesReader([]byte(`{"next_page":"cursor_synthetic_next"}`))}

		cursor, err := parseAnalyticsNextPage(nil, &http.Response{Body: body})

		require.NoError(t, err)
		require.Equal(t, "cursor_synthetic_next", cursor)
		require.True(t, body.closed)
	})
}

func TestSyntheticNullableAndDateAssumptions(t *testing.T) {
	rows := parseSyntheticFixture(t, "users_success.json")
	require.Len(t, rows, 2)

	params := analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "org_synthetic_001",
		OrganizationId: "org_synthetic_001",
		Endpoint:       userActivitiesEndpoint.Name,
	}
	record, err := BuildAnalyticsRecord(rows[1], params)
	require.NoError(t, err)
	require.Equal(t, "2026-01-05", record.Date)
	require.Equal(t, "daily", record.Grain)
	require.Equal(t, "user_synthetic_002", record.UserId)
	require.Equal(t, "analyst@example.invalid", record.UserEmail)
	require.Equal(t, "chat", record.Product)
	require.Empty(t, record.Model)
}

func TestSyntheticFixtureIdentifiersArePlaceholders(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "synthetic", "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			raw, err := os.ReadFile(file)
			require.NoError(t, err)

			var decoded interface{}
			require.NoError(t, json.Unmarshal(raw, &decoded))
			assertSyntheticPlaceholders(t, decoded)
		})
	}
}

func TestPhase9UserActivitiesUseDailyIteratorAndCursorPagination(t *testing.T) {
	input, err := analyticsDateIteratorForEndpoint(userActivitiesEndpoint, "2026-01-05", "2026-01-06")
	require.NoError(t, err)
	require.NotNil(t, input)

	first, err := input.Fetch()
	require.NoError(t, err)
	query := makeQueryForTest(userActivitiesEndpoint, first, nil)
	require.Equal(t, "2026-01-05", query.Get("starting_date"))
	require.Equal(t, "2026-01-05", query.Get("ending_date"))
	require.Equal(t, "1000", query.Get("limit"))
	require.Empty(t, query.Get("page"))

	second, err := input.Fetch()
	require.NoError(t, err)
	query = makeQueryForTest(userActivitiesEndpoint, second, "cursor_synthetic_users_2")
	require.Equal(t, "2026-01-06", query.Get("starting_date"))
	require.Equal(t, "2026-01-06", query.Get("ending_date"))
	require.Equal(t, "1000", query.Get("limit"))
	require.Equal(t, "cursor_synthetic_users_2", query.Get("page"))
	require.False(t, input.HasNext())

	nonDaily, err := analyticsDateIteratorForEndpoint(userUsageReportEndpoint, "2026-01-05", "2026-01-06")
	require.NoError(t, err)
	require.Nil(t, nonDaily)
}

func TestPhase10SummariesUseDailyExclusiveEndingDate(t *testing.T) {
	input, err := analyticsDateIteratorForEndpoint(summariesEndpoint, "2026-01-05", "2026-01-06")
	require.NoError(t, err)
	require.NotNil(t, input)

	first, err := input.Fetch()
	require.NoError(t, err)
	query := makeQueryForTest(summariesEndpoint, first, nil)
	require.Equal(t, "2026-01-05", query.Get("starting_date"))
	require.Equal(t, "2026-01-06", query.Get("ending_date"))
	require.Empty(t, query.Get("limit"))
	require.Empty(t, query.Get("page"))

	second, err := input.Fetch()
	require.NoError(t, err)
	query = makeQueryForTest(summariesEndpoint, second, nil)
	require.Equal(t, "2026-01-06", query.Get("starting_date"))
	require.Equal(t, "2026-01-07", query.Get("ending_date"))
	require.False(t, input.HasNext())
}

func TestPhase10BuildSummaryFromSyntheticFixture(t *testing.T) {
	rows := parseSyntheticFixture(t, "summaries_success.json")
	require.Len(t, rows, 1)

	summary, err := BuildSummaryRecord(rows[0], analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "org_synthetic_001",
		OrganizationId: "org_synthetic_001",
		Endpoint:       summariesEndpoint.Name,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), summary.ConnectionId)
	require.Equal(t, "org_synthetic_001", summary.ScopeId)
	require.Equal(t, "org_synthetic_001", summary.OrganizationId)
	require.Equal(t, "2026-01-05", summary.Date)
	require.Empty(t, summary.Grain)
	require.Equal(t, 42, summary.AssignedSeatCount)
	require.Equal(t, 3, summary.PendingInviteCount)
	require.Equal(t, 18, summary.DailyActiveUserCount)
	require.Equal(t, 31, summary.WeeklyActiveUserCount)
	require.Equal(t, 39, summary.MonthlyActiveUserCount)
	require.Equal(t, 42.86, summary.DailyAdoptionRate)
	require.Equal(t, 12, summary.ChatDailyActiveUserCount)
	require.Equal(t, 8, summary.ClaudeCodeDailyActiveUserCount)
	require.Equal(t, 2, summary.CoworkDailyActiveUserCount)
	require.JSONEq(t, string(rows[0]), summary.RawJson)
}

func TestPhase10SummaryIdentityIsDailyAndScopeScoped(t *testing.T) {
	raw := []byte(`{"date":"2026-01-05","assigned_seats":42,"dau":18}`)
	first, err := BuildSummaryRecord(raw, analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "org_synthetic_001",
		OrganizationId: "org_synthetic_001",
		Endpoint:       summariesEndpoint.Name,
	})
	require.NoError(t, err)
	second, err := BuildSummaryRecord(raw, analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "org_synthetic_002",
		OrganizationId: "org_synthetic_002",
		Endpoint:       summariesEndpoint.Name,
	})
	require.NoError(t, err)

	require.Equal(t, first.Date, second.Date)
	require.NotEqual(t, first.ScopeId, second.ScopeId)
	require.NotEqual(t, first.OrganizationId, second.OrganizationId)
}

func TestPhase11UsageAndCostQueriesUseRFC3339RangeAndCursor(t *testing.T) {
	usageQuery := makeQueryForTest(userUsageReportEndpoint, nil, "cursor_synthetic_usage_2")
	require.Equal(t, "2026-01-01T00:00:00Z", usageQuery.Get("starting_at"))
	require.Equal(t, "2026-01-31T00:00:00Z", usageQuery.Get("ending_at"))
	require.Empty(t, usageQuery.Get("starting_date"))
	require.Empty(t, usageQuery.Get("ending_date"))
	require.Equal(t, "1000", usageQuery.Get("limit"))
	require.Equal(t, "cursor_synthetic_usage_2", usageQuery.Get("page"))

	costQuery := makeQueryForTest(userCostReportEndpoint, nil, "cursor_synthetic_cost_2")
	require.Equal(t, "2026-01-01T00:00:00Z", costQuery.Get("starting_at"))
	require.Equal(t, "2026-01-31T00:00:00Z", costQuery.Get("ending_at"))
	require.Empty(t, costQuery.Get("starting_date"))
	require.Empty(t, costQuery.Get("ending_date"))
	require.Equal(t, "1000", costQuery.Get("limit"))
	require.Equal(t, "cursor_synthetic_cost_2", costQuery.Get("page"))
}

func TestPhase11DefaultUsageCostWindowIsThirtyDays(t *testing.T) {
	startingAt, endingAt := resolveDateRangeForEndpoint(userCostReportEndpoint, nil)
	start, err := time.Parse(time.RFC3339, startingAt)
	require.NoError(t, err)
	end, err := time.Parse(time.RFC3339, endingAt)
	require.NoError(t, err)
	require.InDelta(t, float64(30*24*time.Hour), float64(end.Sub(start)), float64(time.Second))
}

func TestPhase11BuildUsageReportFromSyntheticFixture(t *testing.T) {
	rows := parseSyntheticFixture(t, "user_usage_report_success.json")
	require.Len(t, rows, 1)

	usage, err := BuildUsageReport(rows[0], analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "scope_org_synthetic_001",
		OrganizationId: "org_synthetic_fallback",
		Endpoint:       userUsageReportEndpoint.Name,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), usage.ConnectionId)
	require.Equal(t, "scope_org_synthetic_001", usage.ScopeId)
	require.Equal(t, "org_synthetic_001", usage.OrganizationId)
	require.Equal(t, "2026-01-05T00:00:00Z", usage.StartingAt)
	require.Equal(t, "2026-01-06T00:00:00Z", usage.EndingAt)
	require.Equal(t, "2026-01-06T04:00:00Z", usage.DataRefreshedAt)
	require.Equal(t, "user_synthetic_001", usage.UserId)
	require.Equal(t, "developer@example.invalid", usage.UserEmail)
	require.Equal(t, "claude_code", usage.Product)
	require.Equal(t, "claude-sonnet-4", usage.Model)
	require.NotEmpty(t, usage.ReportId)
	require.Equal(t, false, usage.DeletedActor)
	require.Equal(t, "0-200k", usage.ContextWindow)
	require.Equal(t, "global", usage.InferenceGeo)
	require.Equal(t, "standard", usage.Speed)
	require.Equal(t, int64(12345), usage.UncachedInputTokens)
	require.Equal(t, int64(6789), usage.OutputTokens)
	require.Equal(t, int64(111), usage.CacheReadInputTokens)
	require.Equal(t, int64(222), usage.CacheCreation1hInputTokens)
	require.Equal(t, int64(333), usage.CacheCreation5mInputTokens)
	require.Equal(t, int64(13456), usage.TotalTokens)
	require.Equal(t, int64(9), usage.RequestCount)
	require.Equal(t, int64(2), usage.WebSearchRequests)
	require.JSONEq(t, string(rows[0]), usage.RawJson)
}

func TestPhase11BuildCostReportKeepsDecimalAmountsAsStrings(t *testing.T) {
	rows := parseSyntheticFixture(t, "user_cost_report_success.json")
	require.Len(t, rows, 1)

	cost, err := BuildCostReport(rows[0], analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "scope_org_synthetic_001",
		OrganizationId: "org_synthetic_fallback",
		Endpoint:       userCostReportEndpoint.Name,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), cost.ConnectionId)
	require.Equal(t, "scope_org_synthetic_001", cost.ScopeId)
	require.Equal(t, "org_synthetic_001", cost.OrganizationId)
	require.Equal(t, "2026-01-05T00:00:00Z", cost.StartingAt)
	require.Equal(t, "2026-01-06T00:00:00Z", cost.EndingAt)
	require.Equal(t, "2026-01-06T04:00:00Z", cost.DataRefreshedAt)
	require.Equal(t, "user_synthetic_001", cost.UserId)
	require.Equal(t, "developer@example.invalid", cost.UserEmail)
	require.Equal(t, "claude_code", cost.Product)
	require.Equal(t, "claude-sonnet-4", cost.Model)
	require.NotEmpty(t, cost.ReportId)
	require.Equal(t, false, cost.DeletedActor)
	require.Equal(t, "0-200k", cost.ContextWindow)
	require.Equal(t, "global", cost.InferenceGeo)
	require.Equal(t, "standard", cost.Speed)
	require.Equal(t, "tokens", cost.CostType)
	require.Equal(t, "input", cost.TokenType)
	require.Equal(t, "USD", cost.Currency)
	require.Equal(t, "123.4567", cost.Amount)
	require.Equal(t, "150.0000", cost.ListAmount)
	require.Equal(t, int64(9), cost.RequestCount)
	require.JSONEq(t, string(rows[0]), cost.RawJson)
}

func TestPhase11UsageAndCostRemainRawPreserving(t *testing.T) {
	for _, tt := range []struct {
		fixture  string
		endpoint analyticsEndpoint
	}{
		{fixture: "user_usage_report_success.json", endpoint: userUsageReportEndpoint},
		{fixture: "user_cost_report_success.json", endpoint: userCostReportEndpoint},
	} {
		t.Run(tt.endpoint.Name, func(t *testing.T) {
			rows := parseSyntheticFixture(t, tt.fixture)
			require.Len(t, rows, 1)

			record, err := BuildAnalyticsRecord(rows[0], analyticsRawParams{
				ConnectionId:   1,
				ScopeId:        "scope_org_synthetic_001",
				OrganizationId: "org_synthetic_001",
				Endpoint:       tt.endpoint.Name,
			})
			require.NoError(t, err)
			require.Equal(t, tt.endpoint.Name, record.Endpoint)
			require.Equal(t, "2026-01-05T00:00:00Z", record.Date)
			require.NotEmpty(t, record.RecordId)
			require.JSONEq(t, string(rows[0]), record.RawJson)
		})
	}
}

func TestPhase9BuildUserActivitiesFromSyntheticFixture(t *testing.T) {
	rows := parseSyntheticFixture(t, "users_success.json")
	require.Len(t, rows, 2)

	idGen := didgen.NewDomainIdGenerator(&models.ClaudeEnterpriseAnalyticsRecord{})
	params := analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "org_synthetic_001",
		OrganizationId: "org_synthetic_001",
		Endpoint:       userActivitiesEndpoint.Name,
	}

	codeRecord, err := BuildAnalyticsRecord(rows[0], params)
	require.NoError(t, err)
	codeActivity, err := BuildUserActivity(idGen, "", codeRecord)
	require.NoError(t, err)
	require.NotNil(t, codeActivity)
	require.Equal(t, "claude_enterprise", codeActivity.Provider)
	require.Equal(t, "developer@example.invalid", codeActivity.UserEmail)
	require.Equal(t, "2026-01-05", codeActivity.Date.Format("2006-01-02"))
	require.Equal(t, "CODE_EDIT", codeActivity.Type)
	require.Equal(t, "cli", codeActivity.InterfaceType)
	require.Equal(t, 4, codeActivity.NumSessions)
	require.Equal(t, 25, codeActivity.SuggestionsCount)
	require.Equal(t, 20, codeActivity.AcceptanceCount)
	require.Equal(t, 120, codeActivity.LinesAdded)
	require.Equal(t, 40, codeActivity.LinesRemoved)
	require.Equal(t, 2, codeActivity.CommitsCreated)
	require.Equal(t, 1, codeActivity.PrsCreated)
	require.NotEmpty(t, codeActivity.Id)

	chatRecord, err := BuildAnalyticsRecord(rows[1], params)
	require.NoError(t, err)
	chatActivity, err := BuildUserActivity(idGen, "account_synthetic_001", chatRecord)
	require.NoError(t, err)
	require.NotNil(t, chatActivity)
	require.Equal(t, "account_synthetic_001", chatActivity.AccountId)
	require.Equal(t, "analyst@example.invalid", chatActivity.UserEmail)
	require.Equal(t, "CHAT", chatActivity.Type)
	require.Equal(t, "web_ui", chatActivity.InterfaceType)
	require.Equal(t, 7, chatActivity.NumSessions)
	require.Zero(t, chatActivity.SuggestionsCount)
	require.Zero(t, chatActivity.AcceptanceCount)
}

func TestPhase9UnsupportedUserActivityProductsRemainToolOnly(t *testing.T) {
	rows := parseSyntheticFixture(t, "users_paginated.json")
	require.Len(t, rows, 1)

	record, err := BuildAnalyticsRecord(rows[0], analyticsRawParams{
		ConnectionId:   1,
		ScopeId:        "org_synthetic_001",
		OrganizationId: "org_synthetic_001",
		Endpoint:       userActivitiesEndpoint.Name,
	})
	require.NoError(t, err)

	activity, err := BuildUserActivity(didgen.NewDomainIdGenerator(&models.ClaudeEnterpriseAnalyticsRecord{}), "", record)
	require.NoError(t, err)
	require.Nil(t, activity)
	require.Empty(t, record.Product)
	require.JSONEq(t, string(rows[0]), record.RawJson)
}

func TestSyntheticErrorFixturesAreValidErrorEnvelopes(t *testing.T) {
	for _, name := range []string{"error_401.json", "error_403.json", "error_429.json", "error_invalid_date.json"} {
		t.Run(name, func(t *testing.T) {
			var envelope struct {
				Type      string `json:"type"`
				RequestId string `json:"request_id"`
				Error     struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			raw := readSyntheticFixture(t, name)
			require.NoError(t, json.Unmarshal(raw, &envelope))
			require.Equal(t, "error", envelope.Type)
			require.Contains(t, envelope.RequestId, "req_synthetic_")
			require.NotEmpty(t, envelope.Error.Type)
			require.NotEmpty(t, envelope.Error.Message)
		})
	}
}

func makeQueryForTest(endpoint analyticsEndpoint, input interface{}, cursor interface{}) url.Values {
	query := url.Values{}
	reqData := &helper.RequestData{Input: input, CustomData: cursor}
	startingDate := "2026-01-01"
	endingDate := "2026-01-31"
	if endpoint.DateStyle == dateParamStartingAt {
		startingDate = normalizeAnalyticsTimestamp(startingDate)
		endingDate = normalizeAnalyticsTimestamp(endingDate)
	}
	setDateQueryFromRequest(query, endpoint.DateStyle, startingDate, endingDate, reqData)
	setPaginationQuery(query, endpoint, reqData)
	return query
}

func parseSyntheticFixture(t *testing.T, name string) []json.RawMessage {
	t.Helper()
	raw := readSyntheticFixture(t, name)
	res := &http.Response{Body: io.NopCloser(bytesReader(raw))}
	rows, err := parseAnalyticsResponse(res)
	require.NoError(t, err)
	return rows
}

func readSyntheticFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "synthetic", name))
	require.NoError(t, err)
	return raw
}

func bytesReader(raw []byte) io.Reader {
	return bytes.NewReader(raw)
}

type helperFinishCollect struct{}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (rc *trackingReadCloser) Close() error {
	rc.closed = true
	return nil
}

func assertSyntheticPlaceholders(t *testing.T, value interface{}) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, nested := range typed {
			assertSyntheticPlaceholderValue(t, key, nested)
			assertSyntheticPlaceholders(t, nested)
		}
	case []interface{}:
		for _, nested := range typed {
			assertSyntheticPlaceholders(t, nested)
		}
	}
}

func assertSyntheticPlaceholderValue(t *testing.T, key string, value interface{}) {
	t.Helper()
	text, ok := value.(string)
	if !ok || text == "" {
		return
	}
	lowerKey := strings.ToLower(key)
	switch {
	case strings.Contains(lowerKey, "email"):
		require.Truef(t, strings.HasSuffix(text, "@example.invalid"), "%s must use example.invalid placeholder: %s", key, text)
	case lowerKey == "organization_id" || lowerKey == "organizationid":
		require.Truef(t, strings.HasPrefix(text, "org_synthetic_"), "%s must use synthetic org placeholder: %s", key, text)
	case lowerKey == "id" || strings.HasSuffix(lowerKey, "_id") || strings.HasSuffix(lowerKey, "id"):
		require.Truef(t, strings.Contains(text, "_synthetic_"), "%s must use synthetic id placeholder: %s", key, text)
	case strings.Contains(lowerKey, "page") || strings.Contains(lowerKey, "cursor"):
		require.Truef(t, strings.Contains(text, "cursor_synthetic_"), "%s must use synthetic cursor placeholder: %s", key, text)
	case lowerKey == "request_id":
		require.Truef(t, strings.HasPrefix(text, "req_synthetic_"), "%s must use synthetic request placeholder: %s", key, text)
	}
}
