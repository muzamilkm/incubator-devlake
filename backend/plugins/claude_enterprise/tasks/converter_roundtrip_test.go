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

	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/helpers/unithelper"
	mockdal "github.com/apache/incubator-devlake/mocks/core/dal"
	mockplugin "github.com/apache/incubator-devlake/mocks/core/plugin"
	"github.com/apache/incubator-devlake/plugins/claude_enterprise/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newSingleRowConvertSubTaskContext wires a Cursor/Fetch pair that hands
// helper.DataConverter exactly one *models.ClaudeEnterpriseAnalyticsRecord,
// so ConvertUserActivities's real Convert closure (account resolution +
// BuildUserActivity + batch save) runs end to end instead of stopping at the
// empty-cursor no-op newNoRowsConvertSubTaskContext exercises.
func newSingleRowConvertSubTaskContext(data *ClaudeEnterpriseTaskData, record models.ClaudeEnterpriseAnalyticsRecord) (*mockplugin.SubTaskContext, *mockdal.Dal) {
	mockRows := new(mockdal.Rows)
	mockRows.On("Next").Return(true).Once()
	mockRows.On("Next").Return(false)
	mockRows.On("Close").Return(nil)

	mockDal := new(mockdal.Dal)
	mockDal.On("Cursor", mock.Anything).Return(mockRows, nil)
	mockDal.On("Fetch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(1).(*models.ClaudeEnterpriseAnalyticsRecord)
		*dst = record
	}).Return(nil)
	// resolveClaudeEnterpriseAccountId's account lookup: db.All succeeds with
	// no matching rows, so AccountId stays empty -- a realistic "unresolved
	// user" outcome, not an error.
	mockDal.On("All", mock.Anything, mock.Anything).Return(nil)
	mockDal.On("GetPrimaryKeyFields", mock.Anything).Return([]reflect.StructField{{Name: "Id"}})
	mockDal.On("CreateOrUpdate", mock.Anything, mock.Anything).Return(nil)
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

// TestConvertUserActivitiesEntryPointConvertsSupportedProduct drives
// ConvertUserActivities's Convert closure through a real, supported
// (claude_code) analytics record, asserting the resulting ai_activities row
// is actually saved.
func TestConvertUserActivitiesEntryPointConvertsSupportedProduct(t *testing.T) {
	record := models.ClaudeEnterpriseAnalyticsRecord{
		ConnectionId:   1,
		ScopeId:        "org_synthetic_001",
		OrganizationId: "org_synthetic_001",
		Endpoint:       userActivitiesEndpoint.Name,
		RecordId:       "record_synthetic_001",
		Date:           "2026-01-05",
		UserEmail:      "dev@example.invalid",
		RawJson:        `{"date":"2026-01-05","user":{"id":"user_synthetic_001","email_address":"dev@example.invalid"},"claude_code_metrics":{"core_metrics":{"distinct_session_count":4}}}`,
	}
	ctx, mockDal := newSingleRowConvertSubTaskContext(validClaudeEnterpriseTaskData(), record)

	err := ConvertUserActivities(ctx)
	require.NoError(t, err)
	mockDal.AssertCalled(t, "CreateOrUpdate", mock.Anything, mock.Anything)
}

// TestConvertUserActivitiesEntryPointPropagatesAccountResolutionError asserts
// that a real account-lookup failure (e.g. a lost DB connection while
// resolving record.UserEmail to a crossdomain.Account) fails the subtask
// visibly instead of being swallowed into an empty AccountId -- the second
// half of defect #7 (implementation-plan.md Section 11): errors must not be
// suppressed identically to a legitimate "no account found" outcome.
func TestConvertUserActivitiesEntryPointPropagatesAccountResolutionError(t *testing.T) {
	record := models.ClaudeEnterpriseAnalyticsRecord{
		ConnectionId:   1,
		ScopeId:        "org_synthetic_001",
		OrganizationId: "org_synthetic_001",
		Endpoint:       userActivitiesEndpoint.Name,
		RecordId:       "record_synthetic_003",
		Date:           "2026-01-05",
		UserEmail:      "dev@example.invalid",
		RawJson:        `{"date":"2026-01-05","user":{"id":"user_synthetic_001","email_address":"dev@example.invalid"},"claude_code_metrics":{"core_metrics":{"distinct_session_count":4}}}`,
	}
	mockRows := new(mockdal.Rows)
	mockRows.On("Next").Return(true).Once()
	mockRows.On("Next").Return(false)
	mockRows.On("Close").Return(nil)

	mockDal := new(mockdal.Dal)
	mockDal.On("Cursor", mock.Anything).Return(mockRows, nil)
	mockDal.On("Fetch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(1).(*models.ClaudeEnterpriseAnalyticsRecord)
		*dst = record
	}).Return(nil)
	// resolveClaudeEnterpriseAccountId's account lookup fails for real (e.g.
	// a lost connection) rather than legitimately finding zero rows.
	mockDal.On("All", mock.Anything, mock.Anything).Return(errors.Default.New("connection lost"))

	mockTaskContext := new(mockplugin.TaskContext)
	mockTaskContext.On("GetData").Return(validClaudeEnterpriseTaskData())

	mockCtx := new(mockplugin.SubTaskContext)
	mockCtx.On("TaskContext").Return(mockTaskContext)
	mockCtx.On("GetDal").Return(mockDal)
	mockCtx.On("GetLogger").Return(unithelper.DummyLogger())
	mockCtx.On("GetContext").Return(context.Background())
	mockCtx.On("SetProgress", mock.Anything, mock.Anything)
	mockCtx.On("IncProgress", mock.Anything)

	err := ConvertUserActivities(mockCtx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "connection lost")
	mockDal.AssertNotCalled(t, "CreateOrUpdate", mock.Anything, mock.Anything)
}

// TestConvertUserActivitiesEntryPointSkipsUnsupportedProduct asserts an
// unsupported product (e.g. Cowork) produces zero ai_activities rows without
// erroring -- BuildUserActivity intentionally returns nil for it (Section 7).
func TestConvertUserActivitiesEntryPointSkipsUnsupportedProduct(t *testing.T) {
	record := models.ClaudeEnterpriseAnalyticsRecord{
		ConnectionId:   1,
		ScopeId:        "org_synthetic_001",
		OrganizationId: "org_synthetic_001",
		Endpoint:       userActivitiesEndpoint.Name,
		RecordId:       "record_synthetic_002",
		Date:           "2026-01-05",
		UserEmail:      "reviewer@example.invalid",
		RawJson:        `{"date":"2026-01-05","user":{"id":"user_synthetic_003","email_address":"reviewer@example.invalid"},"cowork_metrics":{"distinct_session_count":2,"message_count":12}}`,
	}
	ctx, mockDal := newSingleRowConvertSubTaskContext(validClaudeEnterpriseTaskData(), record)

	err := ConvertUserActivities(ctx)
	require.NoError(t, err)
	mockDal.AssertNotCalled(t, "CreateOrUpdate", mock.Anything, mock.Anything)
}
