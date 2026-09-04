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

package access

import (
	"context"
	"net/http"
	"testing"

	"github.com/apache/incubator-devlake/core/dal"
	dalmocks "github.com/apache/incubator-devlake/mocks/core/dal"
	"github.com/stretchr/testify/mock"
)

func TestRecordGrafanaCompensatedPersistsDistinctRecoveryState(t *testing.T) {
	db := dalmocks.NewDal(t)
	provider := &OIDCProvider{Revision: 3, GrafanaSyncedRevision: 2}
	db.EXPECT().Update(provider).Run(func(actual interface{}, _ ...dal.Clause) {
		persisted := actual.(*OIDCProvider)
		if persisted.GrafanaSyncStatus != OIDCProviderStatusCompensated {
			t.Fatalf("Grafana sync status = %q, want %q", persisted.GrafanaSyncStatus, OIDCProviderStatusCompensated)
		}
		if persisted.GrafanaSyncedRevision != 2 || persisted.GrafanaLastSyncedAt == nil {
			t.Fatalf("compensated configuration = %#v", persisted)
		}
		if persisted.GrafanaLastErrorCode != ErrCodeProviderBlocked {
			t.Fatalf("Grafana last error code = %q, want %q", persisted.GrafanaLastErrorCode, ErrCodeProviderBlocked)
		}
	}).Return(nil)

	if err := (&Service{db: db}).recordGrafanaCompensated(provider, 2); err != nil {
		t.Fatalf("recordGrafanaCompensated() error = %v", err)
	}
}

func TestRetryGrafanaOIDCProviderSyncRejectsStagedCandidate(t *testing.T) {
	db := dalmocks.NewDal(t)

	provider := &OIDCProvider{
		ProviderKey:   "google",
		Revision:      2,
		GrafanaTarget: GrafanaProviderGenericOAuth,
		Enabled:       true,
	}
	provider.ID = 10

	candidate := &OIDCProviderCandidate{
		ProviderID:  10,
		ProviderKey: "google",
		Revision:    3,
	}

	db.EXPECT().First(mock.AnythingOfType("*access.OIDCProvider"), mock.Anything).Run(func(dest interface{}, clauses ...dal.Clause) {
		p := dest.(*OIDCProvider)
		*p = *provider
	}).Return(nil).Once()

	db.EXPECT().All(mock.AnythingOfType("*[]access.OIDCProviderCandidate"), mock.Anything).Run(func(dest interface{}, clauses ...dal.Clause) {
		c := dest.(*[]OIDCProviderCandidate)
		*c = []OIDCProviderCandidate{*candidate}
	}).Return(nil).Once()

	var putCalls int
	grafanaClient, err := NewGrafanaSSOClient("http://grafana.internal", "admin", "admin", &http.Client{
		Transport: grafanaRoundTripper(func(req *http.Request) (*http.Response, error) {
			putCalls++
			return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Header: make(http.Header)}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	service := &Service{
		db:         db,
		grafanaSSO: grafanaClient,
	}

	resp, retryErr := service.RetryGrafanaOIDCProviderSync(context.Background(), "admin@example.com", "google")
	if retryErr == nil {
		t.Fatal("expected error when retrying sync with staged candidate")
	}
	if resp != nil {
		t.Fatal("expected nil response on error")
	}
	if retryErr.GetData() != ErrCodeProviderBlocked {
		t.Fatalf("error data = %v, want %s", retryErr.GetData(), ErrCodeProviderBlocked)
	}
	if putCalls != 0 {
		t.Fatalf("PutProvider called %d times, want 0", putCalls)
	}
}
