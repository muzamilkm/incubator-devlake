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

package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/helpers/oidchelper"
	"github.com/apache/incubator-devlake/server/api/access"
)

type testAccessAuthorizer struct {
	err            errors.Error
	identities     []access.Identity
	linkStateID    string
	linkProvider   string
	linkedStateID  string
	linkedProvider string
	linkedIdentity access.Identity
}

func (a *testAccessAuthorizer) Enabled() bool { return true }

func (a *testAccessAuthorizer) Authorize(identity access.Identity) (*access.Principal, errors.Error) {
	return a.AuthorizeSession(identity)
}

func (a *testAccessAuthorizer) AuthorizeSession(identity access.Identity) (*access.Principal, errors.Error) {
	a.identities = append(a.identities, identity)
	if a.err != nil {
		return nil, a.err
	}
	return &access.Principal{UserID: 1, Role: access.RoleCustomerAdmin}, nil
}

func (a *testAccessAuthorizer) BeginIdentityLink(_ uint64, providerKey string) (string, errors.Error) {
	a.linkProvider = providerKey
	if a.linkStateID == "" {
		a.linkStateID = "test-link-state"
	}
	return a.linkStateID, a.err
}

func (a *testAccessAuthorizer) CompleteIdentityLink(stateID, providerKey string, identity access.Identity) errors.Error {
	a.linkedStateID = stateID
	a.linkedProvider = providerKey
	a.linkedIdentity = identity
	return a.err
}

func TestOIDCAuthenticationRejectsUnauthorizedAccessSession(t *testing.T) {
	idp := newFakeIdP(t)
	service, _ := newTestService(t, idp)
	authorizer := &testAccessAuthorizer{err: errors.Unauthorized.New("this account is disabled")}
	service.access = authorizer
	router := newTestRouter(service)

	session, _, err := oidchelper.IssueSession(service.runtimeCfg, "disabled-session", "test", idp.subject, idp.email, idp.name)
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, PathUserInfo, nil)
	req.AddCookie(&http.Cookie{Name: oidchelper.SessionCookieName, Value: session})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("userinfo: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var body userInfoResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode userinfo: %v", err)
	}
	if body.Authenticated {
		t.Fatal("expected a disabled directory identity to be unauthenticated")
	}
	if len(authorizer.identities) != 1 {
		t.Fatalf("expected one access-directory lookup, got %d", len(authorizer.identities))
	}
	if got := response.Header().Get("Set-Cookie"); got == "" {
		t.Fatal("expected denied session cookie to be cleared")
	}
}

func TestOIDCAuthenticationRejectsUnknownProviderSession(t *testing.T) {
	idp := newFakeIdP(t)
	service, _ := newTestService(t, idp)
	authorizer := &testAccessAuthorizer{}
	service.access = authorizer
	router := newTestRouter(service)

	session, _, err := oidchelper.IssueSession(service.runtimeCfg, "unknown-provider-session", "retired", idp.subject, idp.email, idp.name)
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, PathUserInfo, nil)
	req.AddCookie(&http.Cookie{Name: oidchelper.SessionCookieName, Value: session})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	var body userInfoResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode userinfo: %v", err)
	}
	if body.Authenticated {
		t.Fatal("expected a session from a removed provider to be unauthenticated")
	}
	if len(authorizer.identities) != 0 {
		t.Fatalf("expected no directory lookup for an unknown provider, got %d", len(authorizer.identities))
	}
}
