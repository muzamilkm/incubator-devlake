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
	return nil, nil
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
