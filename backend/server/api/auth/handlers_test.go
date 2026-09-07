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
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/mock"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/helpers/oidchelper"
	"github.com/apache/incubator-devlake/impls/logruslog"
	mockdal "github.com/apache/incubator-devlake/mocks/core/dal"
)

// fakeIdP is a minimal OIDC provider sufficient for go-oidc discovery,
// token exchange, and JWKS verification.
type fakeIdP struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	keyID  string
	issuer string

	mu            sync.Mutex
	lastCode      string
	subject       string
	email         string
	name          string
	emailVerified bool
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	idp := &fakeIdP{
		key:           key,
		keyID:         "test-key",
		subject:       "user-123",
		email:         "alice@example.com",
		name:          "Alice",
		emailVerified: true,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", idp.handleDiscovery)
	mux.HandleFunc("/jwks", idp.handleJWKS)
	mux.HandleFunc("/token", idp.handleToken)
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "authorize endpoint not exercised by these tests", http.StatusNotImplemented)
	})
	mux.HandleFunc("/logout", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	idp.server = httptest.NewServer(mux)
	idp.issuer = idp.server.URL
	t.Cleanup(idp.server.Close)
	return idp
}

func (f *fakeIdP) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	doc := map[string]any{
		"issuer":                                f.issuer,
		"authorization_endpoint":                f.issuer + "/authorize",
		"token_endpoint":                        f.issuer + "/token",
		"jwks_uri":                              f.issuer + "/jwks",
		"end_session_endpoint":                  f.issuer + "/logout",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

func (f *fakeIdP) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	pub := f.key.PublicKey
	doc := map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"use": "sig",
			"kid": f.keyID,
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

func (f *fakeIdP) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	code := r.FormValue("code")
	f.mu.Lock()
	f.lastCode = code
	f.mu.Unlock()

	// oauth2.Config.Exchange sends client credentials via HTTP Basic by
	// default. Fall back to the form for clients that send them there.
	clientID := r.FormValue("client_id")
	if u, _, ok := r.BasicAuth(); ok {
		clientID = u
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":            f.issuer,
		"aud":            clientID,
		"sub":            f.subject,
		"email":          f.email,
		"email_verified": f.emailVerified,
		"name":           f.name,
		"iat":            time.Now().Unix(),
		"exp":            time.Now().Add(5 * time.Minute).Unix(),
	})
	tok.Header["kid"] = f.keyID
	signed, err := tok.SignedString(f.key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": "dummy-access",
		"token_type":   "Bearer",
		"expires_in":   300,
		"id_token":     signed,
	})
}

// newTestService builds a Service backed by the fake IdP and a permissive
// dal mock that accepts the auth-store writes the handlers issue.
func newTestService(t *testing.T, idp *fakeIdP) (*Service, *mockdal.Dal) {
	t.Helper()
	pc := &oidchelper.ProviderConfig{
		Name:         "test",
		IssuerURL:    idp.issuer,
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURL:  "http://localhost/auth/callback",
		Scopes:       []string{"openid", "profile", "email"},
		DisplayName:  "Test IdP",
	}
	cfg := &oidchelper.Config{
		AuthEnabled:    true,
		OIDCEnabled:    true,
		Providers:      map[string]*oidchelper.ProviderConfig{"test": pc},
		LogoutRedirect: true,
		SessionSecret:  []byte("test-secret-with-at-least-32-bytes!"),
		SessionTTL:     time.Hour,
		CookieSecure:   false,
	}
	db := &mockdal.Dal{}
	db.On("Create", mock.Anything, mock.Anything).Return(nil)
	db.On("UpdateColumn", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	db.On("All", mock.Anything, mock.Anything).Return(nil)
	db.On("Delete", mock.Anything, mock.Anything).Return(nil)

	s := &Service{
		bootstrapCfg: cfg,
		runtimeCfg:   cfg,
		providers:    map[string]*oidchelper.Provider{"test": oidchelper.NewProvider(pc)},
		logger:       logruslog.Global,
		db:           db,
		revoked:      newRevocationCache(),
		lastSeen:     map[string]time.Time{},
	}
	return s, db
}

func newTestRouter(s *Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(s.OIDCAuthentication())
	r.Use(s.RequireAuth())
	r.Use(s.CSRFProtect())
	r.GET(PathLogin, s.LoginInit)
	r.GET(PathLinkIdentity, s.LinkIdentityInit)
	r.GET(PathCallback, s.Callback)
	r.GET(PathUserInfo, s.UserInfo)
	r.POST(PathLogout, s.Logout)
	return r
}

// extractCookie pulls a Set-Cookie value by name from a recorded response.
func extractCookie(t *testing.T, resp *http.Response, name string) *http.Cookie {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("cookie %q not found in response", name)
	return nil
}

// stateNonceFromCookie decodes the encrypted state cookie back into the
// nonce, which the test then echoes as the OIDC `state` query parameter.
func stateNonceFromCookie(t *testing.T, secret []byte, cookieValue string) string {
	t.Helper()
	state, err := oidchelper.DecodeState(secret, cookieValue)
	if err != nil {
		t.Fatalf("decode state cookie: %v", err)
	}
	return state.Nonce
}

func TestLoginInitSetsStateCookieAndRedirects(t *testing.T) {
	idp := newFakeIdP(t)
	s, _ := newTestService(t, idp)
	r := newTestRouter(s)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, PathLogin+"?provider=test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "code_challenge=") || !strings.Contains(loc, "code_challenge_method=S256") {
		t.Fatalf("expected PKCE params on auth URL, got %q", loc)
	}
	resp := w.Result()
	defer resp.Body.Close()
	if c := extractCookie(t, resp, oidchelper.StateCookieName); c.Value == "" {
		t.Fatal("state cookie was empty")
	}
}

func TestLinkIdentityFlowBindsStateToAuthenticatedUserAndProvider(t *testing.T) {
	idp := newFakeIdP(t)
	s, _ := newTestService(t, idp)
	authorizer := &testAccessAuthorizer{linkStateID: "link-state-123"}
	s.access = authorizer
	r := newTestRouter(s)

	cfg, _ := s.providerState()
	session, _, err := oidchelper.IssueSession(cfg, "link-session", "test", idp.subject, idp.email, idp.name)
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	startRequest := httptest.NewRequest(http.MethodGet, PathLinkIdentity+"?provider=test&return_url=/access", nil)
	startRequest.AddCookie(&http.Cookie{Name: oidchelper.SessionCookieName, Value: session})
	startResponse := httptest.NewRecorder()
	r.ServeHTTP(startResponse, startRequest)
	if startResponse.Code != http.StatusSeeOther {
		t.Fatalf("link start: expected 303, got %d: %s", startResponse.Code, startResponse.Body.String())
	}
	stateCookie := extractCookie(t, startResponse.Result(), oidchelper.StateCookieName)
	state, err := oidchelper.DecodeState(cfg.SessionSecret, stateCookie.Value)
	if err != nil {
		t.Fatalf("decode link state: %v", err)
	}
	if state.Provider != "test" || state.IdentityLinkStateID != authorizer.linkStateID {
		t.Fatalf("link state = %+v, want provider=%q state=%q", state, "test", authorizer.linkStateID)
	}

	callbackRequest := httptest.NewRequest(http.MethodGet,
		PathCallback+"?code=fake-code&state="+url.QueryEscape(state.Nonce), nil)
	callbackRequest.AddCookie(stateCookie)
	callbackResponse := httptest.NewRecorder()
	r.ServeHTTP(callbackResponse, callbackRequest)
	if callbackResponse.Code != http.StatusSeeOther {
		t.Fatalf("link callback: expected 303, got %d: %s", callbackResponse.Code, callbackResponse.Body.String())
	}
	if callbackResponse.Header().Get("Location") != "/access?identity_link=linked" {
		t.Fatalf("link callback location = %q, want successful link return", callbackResponse.Header().Get("Location"))
	}
	if authorizer.linkProvider != "test" || authorizer.linkedProvider != "test" || authorizer.linkedStateID != authorizer.linkStateID {
		t.Fatalf("link binding = start provider=%q callback provider=%q state=%q", authorizer.linkProvider, authorizer.linkedProvider, authorizer.linkedStateID)
	}
	if authorizer.linkedIdentity.Issuer != idp.issuer || authorizer.linkedIdentity.Subject != idp.subject {
		t.Fatalf("linked identity = %+v, want IdP identity", authorizer.linkedIdentity)
	}
}

func TestFullLoginCallbackFlow(t *testing.T) {
	idp := newFakeIdP(t)
	s, db := newTestService(t, idp)
	r := newTestRouter(s)

	// 1. /auth/login: capture the state cookie.
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, httptest.NewRequest(http.MethodGet, PathLogin+"?provider=test", nil))
	if loginW.Code != http.StatusSeeOther {
		t.Fatalf("login: expected 303, got %d", loginW.Code)
	}
	stateCookie := extractCookie(t, loginW.Result(), oidchelper.StateCookieName)
	nonce := stateNonceFromCookie(t, s.runtimeCfg.SessionSecret, stateCookie.Value)

	// 2. /auth/callback: must succeed and set session + csrf cookies.
	cbReq := httptest.NewRequest(http.MethodGet,
		PathCallback+"?code=fake-code&state="+url.QueryEscape(nonce), nil)
	cbReq.AddCookie(stateCookie)
	cbW := httptest.NewRecorder()
	r.ServeHTTP(cbW, cbReq)
	if cbW.Code != http.StatusSeeOther {
		t.Fatalf("callback: expected 303, got %d body=%s", cbW.Code, cbW.Body.String())
	}
	sessionCookie := extractCookie(t, cbW.Result(), oidchelper.SessionCookieName)
	csrfCookie := extractCookie(t, cbW.Result(), oidchelper.CSRFCookieName)
	if sessionCookie.Value == "" || csrfCookie.Value == "" {
		t.Fatal("expected session and csrf cookies to be set")
	}
	db.AssertCalled(t, "Create", mock.Anything, mock.Anything)

	// 3. /auth/userinfo with the session cookie should report authenticated.
	uiReq := httptest.NewRequest(http.MethodGet, PathUserInfo, nil)
	uiReq.AddCookie(sessionCookie)
	uiW := httptest.NewRecorder()
	r.ServeHTTP(uiW, uiReq)
	if uiW.Code != http.StatusOK {
		t.Fatalf("userinfo: expected 200, got %d", uiW.Code)
	}
	var userResp userInfoResponse
	if err := json.Unmarshal(uiW.Body.Bytes(), &userResp); err != nil {
		t.Fatalf("decode userinfo: %v: %s", err, uiW.Body.String())
	}
	if !userResp.Authenticated || userResp.Email != idp.email {
		t.Fatalf("unexpected userinfo: %+v body=%s", userResp, uiW.Body.String())
	}

	// 4. /auth/logout with session + csrf header should succeed and revoke.
	logoutReq := httptest.NewRequest(http.MethodPost, PathLogout, nil)
	logoutReq.AddCookie(sessionCookie)
	logoutReq.AddCookie(csrfCookie)
	logoutReq.Header.Set(oidchelper.CSRFHeaderName, csrfCookie.Value)
	logoutW := httptest.NewRecorder()
	r.ServeHTTP(logoutW, logoutReq)
	if logoutW.Code != http.StatusOK {
		t.Fatalf("logout: expected 200, got %d body=%s", logoutW.Code, logoutW.Body.String())
	}
	db.AssertCalled(t, "UpdateColumn", mock.Anything, "revoked_at", mock.Anything, mock.Anything)

	// 5. The same session cookie now belongs to a revoked jti; userinfo
	// should report not authenticated.
	uiReq2 := httptest.NewRequest(http.MethodGet, PathUserInfo, nil)
	uiReq2.AddCookie(sessionCookie)
	uiW2 := httptest.NewRecorder()
	r.ServeHTTP(uiW2, uiReq2)
	var userResp2 userInfoResponse
	if err := json.Unmarshal(uiW2.Body.Bytes(), &userResp2); err != nil {
		t.Fatalf("decode userinfo (revoked): %v", err)
	}
	if userResp2.Authenticated {
		t.Fatal("expected revoked session to no longer be authenticated")
	}
}

func TestCallbackRejectsUnverifiedDirectoryEmail(t *testing.T) {
	idp := newFakeIdP(t)
	idp.emailVerified = false
	s, _ := newTestService(t, idp)
	authorizer := &testAccessAuthorizer{}
	s.access = authorizer
	r := newTestRouter(s)

	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, httptest.NewRequest(http.MethodGet, PathLogin+"?provider=test", nil))
	stateCookie := extractCookie(t, loginW.Result(), oidchelper.StateCookieName)
	nonce := stateNonceFromCookie(t, s.runtimeCfg.SessionSecret, stateCookie.Value)

	callbackW := httptest.NewRecorder()
	callbackReq := httptest.NewRequest(http.MethodGet, PathCallback+"?code=fake-code&state="+url.QueryEscape(nonce), nil)
	callbackReq.AddCookie(stateCookie)
	r.ServeHTTP(callbackW, callbackReq)

	if callbackW.Code != http.StatusForbidden {
		t.Fatalf("callback: expected 403 for an unverified email, got %d: %s", callbackW.Code, callbackW.Body.String())
	}
	if len(authorizer.identities) != 0 {
		t.Fatalf("expected unverified email to bypass directory admission, got %d lookups", len(authorizer.identities))
	}
}

func TestCallbackRedirectsUnexpectedDirectoryFailure(t *testing.T) {
	idp := newFakeIdP(t)
	s, _ := newTestService(t, idp)
	s.access = &testAccessAuthorizer{err: errors.Default.New("database unavailable")}
	r := newTestRouter(s)

	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, httptest.NewRequest(http.MethodGet, PathLogin+"?provider=test", nil))
	stateCookie := extractCookie(t, loginW.Result(), oidchelper.StateCookieName)
	nonce := stateNonceFromCookie(t, s.runtimeCfg.SessionSecret, stateCookie.Value)

	callbackW := httptest.NewRecorder()
	callbackReq := httptest.NewRequest(http.MethodGet, PathCallback+"?code=fake-code&state="+url.QueryEscape(nonce), nil)
	callbackReq.AddCookie(stateCookie)
	r.ServeHTTP(callbackW, callbackReq)

	if callbackW.Code != http.StatusSeeOther {
		t.Fatalf("callback: expected browser-safe redirect, got %d: %s", callbackW.Code, callbackW.Body.String())
	}
	if location := callbackW.Header().Get("Location"); location != "/login?error=access_denied" {
		t.Fatalf("callback redirect = %q, want access-denied login", location)
	}
}

func TestCallbackRejectsStateMismatch(t *testing.T) {
	idp := newFakeIdP(t)
	s, _ := newTestService(t, idp)
	r := newTestRouter(s)

	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, httptest.NewRequest(http.MethodGet, PathLogin+"?provider=test", nil))
	stateCookie := extractCookie(t, loginW.Result(), oidchelper.StateCookieName)

	cbReq := httptest.NewRequest(http.MethodGet, PathCallback+"?code=x&state=not-the-real-nonce", nil)
	cbReq.AddCookie(stateCookie)
	cbW := httptest.NewRecorder()
	r.ServeHTTP(cbW, cbReq)
	if cbW.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on state mismatch, got %d", cbW.Code)
	}
	if !strings.Contains(cbW.Body.String(), "state mismatch") {
		t.Fatalf("expected state mismatch in body, got %s", cbW.Body.String())
	}
}

func TestCallbackRejectsMissingStateCookie(t *testing.T) {
	idp := newFakeIdP(t)
	s, _ := newTestService(t, idp)
	r := newTestRouter(s)

	cbReq := httptest.NewRequest(http.MethodGet, PathCallback+"?code=x&state=anything", nil)
	cbW := httptest.NewRecorder()
	r.ServeHTTP(cbW, cbReq)
	if cbW.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 with missing state cookie, got %d", cbW.Code)
	}
	if !strings.Contains(cbW.Body.String(), "missing state cookie") {
		t.Fatalf("expected missing state cookie in body, got %s", cbW.Body.String())
	}
}

func TestCSRFRequiredOnUnsafeMethod(t *testing.T) {
	idp := newFakeIdP(t)
	s, _ := newTestService(t, idp)
	r := newTestRouter(s)
	r.POST("/some-mutation", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	// Issue a session via the fake-callback so we have valid cookies.
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, httptest.NewRequest(http.MethodGet, PathLogin+"?provider=test", nil))
	stateCookie := extractCookie(t, loginW.Result(), oidchelper.StateCookieName)
	nonce := stateNonceFromCookie(t, s.runtimeCfg.SessionSecret, stateCookie.Value)

	cbReq := httptest.NewRequest(http.MethodGet,
		PathCallback+"?code=c&state="+url.QueryEscape(nonce), nil)
	cbReq.AddCookie(stateCookie)
	cbW := httptest.NewRecorder()
	r.ServeHTTP(cbW, cbReq)
	sessionCookie := extractCookie(t, cbW.Result(), oidchelper.SessionCookieName)
	csrfCookie := extractCookie(t, cbW.Result(), oidchelper.CSRFCookieName)

	// POST without the CSRF header is rejected.
	bad := httptest.NewRequest(http.MethodPost, "/some-mutation", nil)
	bad.AddCookie(sessionCookie)
	bad.AddCookie(csrfCookie)
	badW := httptest.NewRecorder()
	r.ServeHTTP(badW, bad)
	if badW.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without csrf header, got %d", badW.Code)
	}

	// POST with a matching header passes.
	good := httptest.NewRequest(http.MethodPost, "/some-mutation", nil)
	good.AddCookie(sessionCookie)
	good.AddCookie(csrfCookie)
	good.Header.Set(oidchelper.CSRFHeaderName, csrfCookie.Value)
	goodW := httptest.NewRecorder()
	r.ServeHTTP(goodW, good)
	if goodW.Code != http.StatusNoContent {
		t.Fatalf("expected 204 with csrf header, got %d body=%s", goodW.Code, goodW.Body.String())
	}
}

func TestLogoutPublicWhenNoSession(t *testing.T) {
	idp := newFakeIdP(t)
	s, _ := newTestService(t, idp)
	r := newTestRouter(s)

	// No session cookie, no csrf header. publicPaths should let it through
	// and the handler should respond 200.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, PathLogout, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected logout-without-session to return 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// Compile-time assertion that the testify mock satisfies the dal.Dal
// interface so the tests catch any new method that gets added.
var _ dal.Dal = (*mockdal.Dal)(nil)
