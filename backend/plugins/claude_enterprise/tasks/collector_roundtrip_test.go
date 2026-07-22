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
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/apache/incubator-devlake/core/errors"
	helper "github.com/apache/incubator-devlake/helpers/pluginhelper/api"
	mockdal "github.com/apache/incubator-devlake/mocks/core/dal"
	"github.com/apache/incubator-devlake/plugins/claude_enterprise/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestCollectSummariesEntryPointFullRoundTrip drives the CollectSummaries
// entry point (and, through it, the shared collectAnalyticsEndpoint and
// CreateApiClient helpers -- both 0% in Phase 16, and shared by all eight
// collector entry points in this package) all the way through a real HTTP
// round trip against an httptest server, using DevLake's own
// impls/context.DefaultBasicRes rather than a hand-mocked BasicRes chain, the
// same real-implementation pattern plugins/plane/api/remote_api_test.go uses
// for its own ApiClient test. Only the Dal is mocked, since exercising a real
// SQL backend is outside what a subtask entry point unit test needs to prove.
func TestCollectSummariesEntryPointFullRoundTrip(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		require.Equal(t, "/organizations/analytics/summaries", r.URL.Path)
		require.Equal(t, "2026-01-05", r.URL.Query().Get("starting_date"))
		require.Equal(t, "sk-ant-api01-synthetic", r.Header.Get("x-api-key"))
		require.Equal(t, models.AnthropicVersion, r.Header.Get("anthropic-version"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"date":"2026-01-05","assigned_seats":10,"dau":5}]`))
	}))
	defer server.Close()

	var capturedRawRows []*helper.RawData
	mockDal := new(mockdal.Dal)
	mockDal.On("First", mock.Anything, mock.Anything).Return(errors.NotFound.New("no prior collector state"))
	mockDal.On("IsErrorNotFound", mock.Anything).Return(true)
	mockDal.On("Update", mock.Anything, mock.Anything).Return(nil)
	mockDal.On("AutoMigrate", mock.Anything, mock.Anything).Return(nil)
	mockDal.On("Delete", mock.Anything, mock.Anything).Return(nil)
	mockDal.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		if rows, ok := args.Get(0).([]*helper.RawData); ok {
			capturedRawRows = rows
		}
	}).Return(nil)

	data := &ClaudeEnterpriseTaskData{
		Options: &ClaudeEnterpriseOptions{
			ConnectionId:   1,
			ScopeId:        "org_synthetic_001",
			OrganizationId: "org_synthetic_001",
			StartingDate:   "2026-01-05",
			EndingDate:     "2026-01-05",
		},
		Connection: &models.ClaudeEnterpriseConnection{
			ClaudeEnterpriseConn: models.ClaudeEnterpriseConn{
				RestConnection:   helper.RestConnection{Endpoint: server.URL},
				AnalyticsApiKey:  "sk-ant-api01-synthetic",
				OrganizationId:   "org_synthetic_001",
				RateLimitPerHour: 2400,
			},
		},
	}

	taskCtx := newFakeTaskContext(mockDal, data)
	subTaskCtx := newFakeSubTaskContext(taskCtx)

	err := CollectSummaries(subTaskCtx)
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&requestCount), "a single finalized day should issue exactly one request")
	require.Len(t, capturedRawRows, 1)
	require.JSONEq(t, `{"date":"2026-01-05","assigned_seats":10,"dau":5}`, string(capturedRawRows[0].Data))
	mockDal.AssertCalled(t, "Update", mock.Anything, mock.Anything)
}

// TestCollectUserActivitiesEntryPointStopsPaginationOnShortPage exercises the
// paginated branch of collectAnalyticsEndpoint (getAnalyticsNextPageFunc /
// parseAnalyticsNextPage wiring) end to end: a single, short (< page size)
// response page must not trigger a second request.
func TestCollectUserActivitiesEntryPointStopsPaginationOnShortPage(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		require.Equal(t, "/organizations/analytics/users", r.URL.Path)
		require.Equal(t, "1000", r.URL.Query().Get("limit"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"date":"2026-01-05","user":{"id":"user_synthetic_001","email_address":"dev@example.invalid"},"claude_code_metrics":{"core_metrics":{"distinct_session_count":1}}}],"next_page":""}`))
	}))
	defer server.Close()

	mockDal := new(mockdal.Dal)
	mockDal.On("First", mock.Anything, mock.Anything).Return(errors.NotFound.New("no prior collector state"))
	mockDal.On("IsErrorNotFound", mock.Anything).Return(true)
	mockDal.On("Update", mock.Anything, mock.Anything).Return(nil)
	mockDal.On("AutoMigrate", mock.Anything, mock.Anything).Return(nil)
	mockDal.On("Delete", mock.Anything, mock.Anything).Return(nil)
	mockDal.On("Create", mock.Anything, mock.Anything).Return(nil)

	data := &ClaudeEnterpriseTaskData{
		Options: &ClaudeEnterpriseOptions{
			ConnectionId:   1,
			ScopeId:        "org_synthetic_001",
			OrganizationId: "org_synthetic_001",
			StartingDate:   "2026-01-05",
			EndingDate:     "2026-01-05",
		},
		Connection: &models.ClaudeEnterpriseConnection{
			ClaudeEnterpriseConn: models.ClaudeEnterpriseConn{
				RestConnection:   helper.RestConnection{Endpoint: server.URL},
				AnalyticsApiKey:  "sk-ant-api01-synthetic",
				OrganizationId:   "org_synthetic_001",
				RateLimitPerHour: 2400,
			},
		},
	}

	taskCtx := newFakeTaskContext(mockDal, data)
	subTaskCtx := newFakeSubTaskContext(taskCtx)

	err := CollectUserActivities(subTaskCtx)
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&requestCount), "a page shorter than the page size must stop pagination")
}
