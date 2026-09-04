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
	"github.com/apache/incubator-devlake/core/errors"
	dalmocks "github.com/apache/incubator-devlake/mocks/core/dal"
	"github.com/stretchr/testify/mock"
)

type oidcProviderRuntimeStub struct {
	revocationCalls int
}

func (*oidcProviderRuntimeStub) PrepareOIDCProvider(context.Context, *OIDCProvider, string) (*PreparedOIDCProvider, errors.Error) {
	return &PreparedOIDCProvider{
		GrafanaSettings: GrafanaSSOSettings{Enabled: true},
	}, nil
}

func (*oidcProviderRuntimeStub) RefreshOIDCProvider(context.Context) errors.Error { return nil }

func (stub *oidcProviderRuntimeStub) RevokeProviderSessions(dal.Transaction, string) ([]string, errors.Error) {
	stub.revocationCalls++
	return nil, nil
}

func (*oidcProviderRuntimeStub) CacheRevokedSessions([]string) {}

func TestSetOIDCProviderEnabledDoesNotRevokeSessionsAfterStaleTransition(t *testing.T) {
	db := dalmocks.NewDal(t)
	tx := dalmocks.NewTransaction(t)
	runtime := &oidcProviderRuntimeStub{}
	notFound := errors.NotFound.New("OIDC provider is not active")
	db.EXPECT().Begin().Return(tx)
	tx.EXPECT().First(mock.Anything, mock.Anything).Return(notFound)
	tx.EXPECT().IsErrorNotFound(notFound).Return(true)
	tx.EXPECT().Rollback().Return(nil)

	provider := &OIDCProvider{ProviderKey: "google", Enabled: true}
	provider.ID = 1
	_, err := (&Service{db: db, oidcRuntime: runtime}).setOIDCProviderEnabled(
		context.Background(),
		"admin@example.com",
		provider,
		false,
	)
	if err == nil || err.GetData() != ErrCodeProviderBlocked {
		t.Fatalf("setOIDCProviderEnabled() error = %v, want stale-state rejection", err)
	}
	if runtime.revocationCalls != 0 {
		t.Fatalf("session revocation calls = %d, want 0", runtime.revocationCalls)
	}
}

func TestActivationFailureCompensatesAndPreservesGrafanaSyncedRevision(t *testing.T) {
	db := dalmocks.NewDal(t)
	txActivate := dalmocks.NewTransaction(t)

	provider := &OIDCProvider{
		ProviderKey:           "google",
		DisplayName:           "Google",
		IssuerURL:             "https://accounts.google.com",
		ClientID:              "client-id",
		Revision:              5,
		GrafanaTarget:         GrafanaProviderGenericOAuth,
		Enabled:               true,
		GrafanaSyncStatus:     OIDCProviderStatusPending,
		GrafanaSyncedRevision: 4,
	}
	provider.ID = 10

	candidate := &OIDCProviderCandidate{
		ProviderID:    10,
		ProviderKey:   "google",
		DisplayName:   "Google New",
		IssuerURL:     "https://accounts.google.com",
		ClientID:      "client-id",
		Revision:      5,
		GrafanaTarget: GrafanaProviderGenericOAuth,
	}
	candidate.ID = 20

	// 1. currentOIDCProvider queries provider and candidate
	db.EXPECT().First(mock.AnythingOfType("*access.OIDCProvider"), mock.Anything).Run(func(dest interface{}, clauses ...dal.Clause) {
		p := dest.(*OIDCProvider)
		*p = *provider
	}).Return(nil).Once()

	db.EXPECT().All(mock.AnythingOfType("*[]access.OIDCProviderCandidate"), mock.Anything).Run(func(dest interface{}, clauses ...dal.Clause) {
		c := dest.(*[]OIDCProviderCandidate)
		*c = []OIDCProviderCandidate{*candidate}
	}).Return(nil).Once()

	// 2. ensureGrafanaTargetAvailable
	db.EXPECT().All(mock.AnythingOfType("*[]access.OIDCProvider"), mock.Anything).Return(nil).Once()

	// 3. syncGrafana updates DB on successful Grafana sync for candidate
	db.EXPECT().UpdateColumns(mock.AnythingOfType("*access.OIDCProvider"), mock.Anything, mock.Anything).Return(nil).Once()

	// 4. activateOIDCProvider transaction fails
	dbErr := errors.Default.New("activation DB failure")
	db.EXPECT().Begin().Return(txActivate).Once()
	txActivate.EXPECT().UpdateColumns(mock.AnythingOfType("*access.OIDCProvider"), mock.Anything, mock.Anything).Return(dbErr).Once()
	txActivate.EXPECT().Rollback().Return(nil).Once()

	// 5. recordGrafanaCompensated calls s.db.Update(provider)
	var compensatedProvider *OIDCProvider
	db.EXPECT().Update(mock.AnythingOfType("*access.OIDCProvider")).Run(func(entity interface{}, clauses ...dal.Clause) {
		compensatedProvider = entity.(*OIDCProvider)
	}).Return(nil).Once()

	var putRequests []string
	grafanaClient, err := NewGrafanaSSOClient("http://grafana.internal", "admin", "admin", &http.Client{
		Transport: grafanaRoundTripper(func(req *http.Request) (*http.Response, error) {
			putRequests = append(putRequests, req.URL.Path)
			return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Header: make(http.Header)}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	runtime := &oidcProviderRuntimeStub{}
	service := &Service{
		db:          db,
		grafanaSSO:  grafanaClient,
		oidcRuntime: runtime,
	}

	_, activateErr := service.ActivateOIDCProvider(context.Background(), "admin@example.com", "google")
	if activateErr == nil {
		t.Fatal("expected activate error from DB failure")
	}

	if len(putRequests) != 2 {
		t.Fatalf("expected 2 Grafana PUT requests (sync candidate + restore provider), got %d: %v", len(putRequests), putRequests)
	}

	if compensatedProvider == nil {
		t.Fatal("expected recordGrafanaCompensated to call s.db.Update")
	}
	if compensatedProvider.GrafanaSyncStatus != OIDCProviderStatusCompensated {
		t.Fatalf("compensated grafana_sync_status = %v, want %s", compensatedProvider.GrafanaSyncStatus, OIDCProviderStatusCompensated)
	}
	if compensatedProvider.GrafanaSyncedRevision != uint64(4) {
		t.Fatalf("compensated grafana_synced_revision = %v, want 4", compensatedProvider.GrafanaSyncedRevision)
	}
}
