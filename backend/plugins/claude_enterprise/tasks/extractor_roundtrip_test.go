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
	"context"
	"reflect"
	"testing"

	helper "github.com/apache/incubator-devlake/helpers/pluginhelper/api"
	"github.com/apache/incubator-devlake/helpers/unithelper"
	mockdal "github.com/apache/incubator-devlake/mocks/core/dal"
	mockplugin "github.com/apache/incubator-devlake/mocks/core/plugin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newSingleRowExtractSubTaskContext returns a SubTaskContext whose raw table
// reports exactly one row (rawItem), driving helper.ApiExtractor's real
// fetch -> Extract callback -> batch-save path to completion. This closes
// the gap the no-raw-table tests in entrypoint_test.go intentionally leave
// open: those prove the "nothing collected yet" no-op, while this proves the
// entry point's Extract closure (which builds and saves real tool rows) also
// runs end to end. The returned *mockdal.Dal is exposed so callers can
// assert which rows were actually saved.
func newSingleRowExtractSubTaskContext(data *ClaudeEnterpriseTaskData, rawItem []byte) (*mockplugin.SubTaskContext, *mockdal.Dal) {
	mockRows := new(mockdal.Rows)
	mockRows.On("Next").Return(true).Once()
	mockRows.On("Next").Return(false)
	mockRows.On("Close").Return(nil)

	mockDal := new(mockdal.Dal)
	mockDal.On("HasTable", mock.Anything).Return(true)
	mockDal.On("Count", mock.Anything).Return(int64(1), nil)
	mockDal.On("Cursor", mock.Anything).Return(mockRows, nil)
	mockDal.On("Fetch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(1).(*helper.RawData)
		dst.ID = 1
		dst.Data = rawItem
	}).Return(nil)
	// A single, real field name shared by every typed model in this package
	// (Section 9's grain always starts with connection+organization), so one
	// stub covers every result type an Extract closure below might save.
	mockDal.On("GetPrimaryKeyFields", mock.Anything).Return([]reflect.StructField{{Name: "ConnectionId"}})
	mockDal.On("CreateOrUpdate", mock.Anything, mock.Anything).Return(nil)
	// helper.BatchSaveDivider deletes previously-extracted rows for this raw
	// origin (non-incremental extraction) before the first insert of a type.
	mockDal.On("Delete", mock.Anything, mock.Anything).Return(nil)

	mockTaskContext := new(mockplugin.TaskContext)
	mockTaskContext.On("GetData").Return(data)

	mockCtx := new(mockplugin.SubTaskContext)
	mockCtx.On("TaskContext").Return(mockTaskContext)
	mockCtx.On("GetDal").Return(mockDal)
	mockCtx.On("GetLogger").Return(unithelper.DummyLogger())
	mockCtx.On("GetContext").Return(context.Background())
	mockCtx.On("SetProgress", mock.Anything, mock.Anything)
	mockCtx.On("IncProgress", mock.Anything)
	return mockCtx, mockDal
}

// TestExtractSummariesEntryPointSavesRawAndTypedRows drives extractSummariesEndpoint
// (46.7% after the no-raw-table tests alone) through a real single-row fetch,
// asserting both the raw-preserving analytics record and the typed summary
// row get saved -- the real dual-write behavior Phase 10 documents.
func TestExtractSummariesEntryPointSavesRawAndTypedRows(t *testing.T) {
	ctx, mockDal := newSingleRowExtractSubTaskContext(
		validClaudeEnterpriseTaskData(),
		[]byte(`{"date":"2026-01-05","assigned_seats":10,"dau":5}`),
	)

	err := ExtractSummaries(ctx)
	require.NoError(t, err)
	mockDal.AssertCalled(t, "CreateOrUpdate", mock.Anything, mock.Anything)
	mockDal.AssertNumberOfCalls(t, "CreateOrUpdate", 2) // one for the raw record, one for the typed summary
}

// TestExtractUserActivitiesEntryPointSavesAnalyticsRecord drives
// extractAnalyticsEndpoint (used only by /users, which has no dedicated typed
// table) through a real single-row fetch.
func TestExtractUserActivitiesEntryPointSavesAnalyticsRecord(t *testing.T) {
	ctx, mockDal := newSingleRowExtractSubTaskContext(
		validClaudeEnterpriseTaskData(),
		[]byte(`{"date":"2026-01-05","user":{"id":"user_synthetic_001","email_address":"dev@example.invalid"},"claude_code_metrics":{"core_metrics":{"distinct_session_count":4}}}`),
	)

	err := ExtractUserActivities(ctx)
	require.NoError(t, err)
	mockDal.AssertNumberOfCalls(t, "CreateOrUpdate", 1)
}

// TestExtractUserUsageReportEntryPointSavesRawAndTypedRows drives
// extractUsageReportEndpoint through a real single-row fetch with a valid
// starting_at, the field BuildUsageReport requires.
func TestExtractUserUsageReportEntryPointSavesRawAndTypedRows(t *testing.T) {
	ctx, mockDal := newSingleRowExtractSubTaskContext(
		validClaudeEnterpriseTaskData(),
		[]byte(`{"starting_at":"2026-01-05T00:00:00Z","ending_at":"2026-01-06T00:00:00Z","actor":{"user_id":"user_synthetic_001","email":"dev@example.invalid","deleted":false},"product":"claude_code","model":"claude-sonnet-4","uncached_input_tokens":100,"output_tokens":50}`),
	)

	err := ExtractUserUsageReport(ctx)
	require.NoError(t, err)
	mockDal.AssertNumberOfCalls(t, "CreateOrUpdate", 2)
}

// TestExtractUserCostReportEntryPointSavesRawAndTypedRows drives
// extractCostReportEndpoint through a real single-row fetch with a valid
// starting_at, the field BuildCostReport requires.
func TestExtractUserCostReportEntryPointSavesRawAndTypedRows(t *testing.T) {
	ctx, mockDal := newSingleRowExtractSubTaskContext(
		validClaudeEnterpriseTaskData(),
		[]byte(`{"starting_at":"2026-01-05T00:00:00Z","ending_at":"2026-01-06T00:00:00Z","actor":{"user_id":"user_synthetic_001","email":"dev@example.invalid","deleted":false},"product":"claude_code","model":"claude-sonnet-4","cost_type":"tokens","amount":"1.2345","currency":"USD"}`),
	)

	err := ExtractUserCostReport(ctx)
	require.NoError(t, err)
	mockDal.AssertNumberOfCalls(t, "CreateOrUpdate", 2)
}

// TestExtractSkillsEntryPointSavesRawAndTypedRows drives the shared
// extractTypedAnalyticsEndpoint helper (used by all five Phase 14 extended
// entities) through a real single-row fetch for the skills endpoint.
func TestExtractSkillsEntryPointSavesRawAndTypedRows(t *testing.T) {
	ctx, mockDal := newSingleRowExtractSubTaskContext(
		validClaudeEnterpriseTaskData(),
		[]byte(`{"date":"2026-01-05","skill_name":"Doc Search","distinct_user_count":2,"invocation_count":5}`),
	)

	err := ExtractSkills(ctx)
	require.NoError(t, err)
	mockDal.AssertNumberOfCalls(t, "CreateOrUpdate", 2)
}

// TestExtractEntryPointsSurfaceBuildErrors asserts that a raw item missing a
// required field (e.g. a summary item without a date) causes the entry point
// to return a real, descriptive error instead of silently dropping the row.
func TestExtractEntryPointsSurfaceBuildErrors(t *testing.T) {
	ctx, _ := newSingleRowExtractSubTaskContext(
		validClaudeEnterpriseTaskData(),
		[]byte(`{"assigned_seats":10}`), // missing "date"
	)

	err := ExtractSummaries(ctx)
	require.Error(t, err)
}
