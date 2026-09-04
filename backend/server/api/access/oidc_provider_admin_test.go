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
	"testing"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
	mockdal "github.com/apache/incubator-devlake/mocks/core/dal"
	"github.com/stretchr/testify/mock"
)

func TestNormalizeOIDCProviderInput(t *testing.T) {
	testCases := []struct {
		name      string
		input     OIDCProviderInput
		wantKey   string
		wantScope string
		wantError bool
	}{
		{
			name: "normalizes provider settings",
			input: OIDCProviderInput{
				ProviderKey: "  Google-Workspace ", DisplayName: " Google ", IssuerURL: "https://accounts.example.com/ ",
				ClientID: " client ", ClientSecret: " secret ", Scopes: "openid, profile openid email",
				GrafanaTarget: GrafanaProviderGenericOAuth,
			},
			wantKey: "google-workspace", wantScope: "openid profile email",
		},
		{
			name: "normalizes mixed comma and space delimiters in scopes",
			input: OIDCProviderInput{
				ProviderKey: "mixed-delim", DisplayName: "Mixed Delim", IssuerURL: "https://accounts.example.com",
				ClientID: "client", ClientSecret: "secret", Scopes: "openid, profile email",
				GrafanaTarget: GrafanaProviderGenericOAuth,
			},
			wantKey: "mixed-delim", wantScope: "openid profile email",
		},
		{
			name:      "rejects omitted Grafana target",
			input:     OIDCProviderInput{ProviderKey: "google", DisplayName: "Google", IssuerURL: "https://accounts.example.com", ClientID: "client", Scopes: "openid"},
			wantError: true,
		},
		{
			name:      "rejects missing openid scope",
			input:     OIDCProviderInput{ProviderKey: "google", DisplayName: "Google", IssuerURL: "https://accounts.example.com", ClientID: "client", Scopes: "profile email", GrafanaTarget: GrafanaProviderGenericOAuth},
			wantError: true,
		},
		{
			name:      "rejects unsafe provider key",
			input:     OIDCProviderInput{ProviderKey: "google/oidc", DisplayName: "Google", IssuerURL: "https://accounts.example.com", ClientID: "client", Scopes: "openid", GrafanaTarget: GrafanaProviderGenericOAuth},
			wantError: true,
		},
		{
			name:      "requires confirmation for DevLake-only provider",
			input:     OIDCProviderInput{ProviderKey: "custom", DisplayName: "Custom", IssuerURL: "https://accounts.example.com", ClientID: "client", Scopes: "openid", GrafanaTarget: GrafanaProviderNone},
			wantError: true,
		},
		{
			name:    "accepts confirmed DevLake-only provider",
			input:   OIDCProviderInput{ProviderKey: "custom", DisplayName: "Custom", IssuerURL: "https://accounts.example.com", ClientID: "client", Scopes: "openid", GrafanaTarget: GrafanaProviderNone, ConfirmDevLakeOnly: true},
			wantKey: "custom", wantScope: "openid",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			provider, _, err := normalizeOIDCProviderInput(testCase.input)
			if (err != nil) != testCase.wantError {
				t.Fatalf("normalizeOIDCProviderInput() error = %v, wantError %t", err, testCase.wantError)
			}
			if err == nil && (provider.ProviderKey != testCase.wantKey || provider.Scopes != testCase.wantScope) {
				t.Fatalf("provider = %#v, want key=%q scopes=%q", provider, testCase.wantKey, testCase.wantScope)
			}
		})
	}
}

func TestOIDCProviderResponseDoesNotExposeSecret(t *testing.T) {
	provider := &OIDCProvider{
		ProviderKey: "google", EncryptedClientSecret: []byte("ciphertext"), ClientSecretNonce: []byte("nonce"), ClientSecretKeyID: "key-1",
	}
	response := oidcProviderResponse(provider, &OIDCProviderConfiguration{})
	if !response.SecretConfigured {
		t.Fatal("response should report configured secret")
	}
}

func TestReuseOIDCProviderCredential(t *testing.T) {
	stored := oidcProviderFromCandidate(&OIDCProviderCandidate{
		ClientID: "client-a", EncryptedClientSecret: []byte("ciphertext"), ClientSecretNonce: []byte("nonce"), ClientSecretKeyID: "key-1",
	})
	testCases := []struct {
		name          string
		provider      *OIDCProvider
		stored        *OIDCProvider
		clientSecret  string
		wantErrorCode string
		wantReuse     bool
	}{
		{
			name: "reuses configured credential for unchanged client ID", provider: &OIDCProvider{ClientID: "client-a"}, stored: stored,
			wantReuse: true,
		},
		{
			name: "requires replacement credential for changed client ID", provider: &OIDCProvider{ClientID: "client-b"}, stored: stored,
			wantErrorCode: ErrCodeInvalidProvider,
		},
		{
			name: "requires credential for first provider", provider: &OIDCProvider{ClientID: "client-a"},
			wantErrorCode: ErrCodeInvalidProvider,
		},
		{
			name: "uses supplied replacement credential", provider: &OIDCProvider{ClientID: "client-b"}, stored: stored, clientSecret: "replacement",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := reuseOIDCProviderCredential(testCase.provider, testCase.stored, testCase.clientSecret)
			if testCase.wantErrorCode == "" {
				if err != nil {
					t.Fatalf("reuseOIDCProviderCredential() error = %v", err)
				}
			} else if err == nil || err.GetData() != testCase.wantErrorCode {
				t.Fatalf("reuseOIDCProviderCredential() error = %v, want code %q", err, testCase.wantErrorCode)
			}
			if testCase.wantReuse && !hasOIDCProviderSecret(testCase.provider) {
				t.Fatal("expected stored credential to remain available internally")
			}
		})
	}
}

func TestValidateGrafanaProviderCompatibility(t *testing.T) {
	testCases := []struct {
		name      string
		provider  *OIDCProvider
		wantError bool
	}{
		{name: "allows Google issuer", provider: &OIDCProvider{IssuerURL: "https://accounts.google.com", GrafanaTarget: GrafanaProviderGoogle}},
		{name: "allows Entra tenant issuer", provider: &OIDCProvider{IssuerURL: "https://login.microsoftonline.com/tenant/v2.0", GrafanaTarget: GrafanaProviderAzureAD}},
		{name: "allows Okta issuer", provider: &OIDCProvider{IssuerURL: "https://customer.okta.com/oauth2/default", GrafanaTarget: GrafanaProviderOkta}},
		{name: "rejects mismatched native target", provider: &OIDCProvider{IssuerURL: "https://accounts.google.com", GrafanaTarget: GrafanaProviderOkta}, wantError: true},
		{name: "allows an arbitrary issuer for generic OAuth", provider: &OIDCProvider{IssuerURL: "https://issuer.example.com", GrafanaTarget: GrafanaProviderGenericOAuth}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateGrafanaProviderCompatibility(testCase.provider)
			if (err != nil) != testCase.wantError {
				t.Fatalf("validateGrafanaProviderCompatibility() error = %v, wantError %t", err, testCase.wantError)
			}
		})
	}
}

func TestOIDCProviderResponseIncludesDeploymentDerivedCallbacks(t *testing.T) {
	service := &Service{cfg: Config{
		AuthPublicURL:    "https://devlake.example.com",
		GrafanaPublicURL: "https://grafana.example.com",
	}}
	response := service.decorateOIDCProviderResponse(&OIDCProviderResponse{GrafanaTarget: GrafanaProviderGenericOAuth})
	if response.DevLakeCallbackURL != "https://devlake.example.com/api/auth/callback" {
		t.Fatalf("DevLake callback = %q", response.DevLakeCallbackURL)
	}
	if response.GrafanaCallbackURL != "https://grafana.example.com/login/generic_oauth" {
		t.Fatalf("Grafana callback = %q", response.GrafanaCallbackURL)
	}
	if response.AllowLocalOIDC {
		t.Fatal("public deployment must not allow local OIDC HTTP")
	}
	response = service.decorateOIDCProviderResponse(&OIDCProviderResponse{GrafanaTarget: GrafanaProviderGoogle})
	if response.GrafanaCallbackURL != "https://grafana.example.com/login/google" {
		t.Fatalf("Google Grafana callback = %q", response.GrafanaCallbackURL)
	}
}

func TestOIDCProviderCallbacksIncludeEachGrafanaSignInOption(t *testing.T) {
	service := &Service{cfg: Config{
		AuthPublicURL:    "https://devlake.example.com",
		GrafanaPublicURL: "https://grafana.example.com",
	}}
	callbacks, err := service.OIDCProviderCallbacks()
	if err != nil {
		t.Fatalf("OIDCProviderCallbacks() error = %v", err)
	}
	if callbacks.DevLakeCallbackURL != "https://devlake.example.com/api/auth/callback" {
		t.Fatalf("DevLake callback = %q", callbacks.DevLakeCallbackURL)
	}
	if callbacks.AllowLocalOIDC {
		t.Fatal("public deployment must not allow local OIDC HTTP")
	}
	for target, want := range map[GrafanaProviderKind]string{
		GrafanaProviderNone:         "/login",
		GrafanaProviderGoogle:       "/login/google",
		GrafanaProviderAzureAD:      "/login/azuread",
		GrafanaProviderOkta:         "/login/okta",
		GrafanaProviderGitLab:       "/login/gitlab",
		GrafanaProviderGenericOAuth: "/login/generic_oauth",
	} {
		if got := callbacks.GrafanaCallbackURLs[target]; got != "https://grafana.example.com"+want {
			t.Errorf("Grafana callback for %q = %q, want %q", target, got, "https://grafana.example.com"+want)
		}
	}
}

func TestOIDCProviderCallbacksExposeLocalDevelopmentCapability(t *testing.T) {
	service := &Service{cfg: Config{
		AuthPublicURL:    "http://localhost:4000",
		GrafanaPublicURL: "http://localhost:3002",
	}}
	callbacks, err := service.OIDCProviderCallbacks()
	if err != nil {
		t.Fatalf("OIDCProviderCallbacks() error = %v", err)
	}
	if !callbacks.AllowLocalOIDC {
		t.Fatal("local deployment must allow local OIDC HTTP")
	}
	response := service.decorateOIDCProviderResponse(&OIDCProviderResponse{})
	if !response.AllowLocalOIDC {
		t.Fatal("local provider response must allow local OIDC HTTP")
	}
}

func TestOIDCProviderCallbacksRequireDeploymentOrigins(t *testing.T) {
	service := &Service{cfg: Config{AuthPublicURL: "https://devlake.example.com"}}
	if _, _, err := service.oidcProviderCallbacks(); err == nil {
		t.Fatal("expected missing Grafana public URL to block provider administration")
	}
}

func TestGrafanaLoginPath(t *testing.T) {
	testCases := []struct {
		target GrafanaProviderKind
		want   string
	}{
		{target: GrafanaProviderGoogle, want: "/login/google"},
		{target: GrafanaProviderAzureAD, want: "/login/azuread"},
		{target: GrafanaProviderGenericOAuth, want: "/login/generic_oauth"},
		{target: GrafanaProviderNone, want: "/login"},
	}
	for _, testCase := range testCases {
		if actual := grafanaLoginPath(testCase.target); actual != testCase.want {
			t.Fatalf("grafanaLoginPath(%q) = %q, want %q", testCase.target, actual, testCase.want)
		}
	}
}

func TestEnsureIssuerAvailable(t *testing.T) {
	t.Run("rejects active duplicate issuer", func(t *testing.T) {
		db := &mockdal.Dal{}
		db.On("First", mock.Anything, mock.Anything).Return(nil)
		service := &Service{db: db}

		err := service.ensureIssuerAvailable("https://accounts.google.com", 1)
		if err == nil {
			t.Fatal("expected error for active duplicate issuer")
		}
		if err.GetData() != ErrCodeInvalidProvider {
			t.Fatalf("err code = %v, want %s", err.GetData(), ErrCodeInvalidProvider)
		}
	})

	t.Run("allows self-update", func(t *testing.T) {
		db := &mockdal.Dal{}
		notFoundErr := errors.NotFound.New("not found")
		db.On("First", mock.Anything, mock.Anything).Return(notFoundErr)
		db.On("IsErrorNotFound", notFoundErr).Return(true)
		service := &Service{db: db}

		err := service.ensureIssuerAvailable("https://accounts.google.com", 1)
		if err != nil {
			t.Fatalf("expected nil error for self-update, got %v", err)
		}
	})

	t.Run("allows recreation when previous provider is retired", func(t *testing.T) {
		db := &mockdal.Dal{}
		notFoundErr := errors.NotFound.New("not found")
		db.On("First", mock.Anything, mock.Anything).Return(notFoundErr)
		db.On("IsErrorNotFound", notFoundErr).Return(true)
		service := &Service{db: db}

		err := service.ensureIssuerAvailable("https://accounts.google.com", 0)
		if err != nil {
			t.Fatalf("expected nil error when previous provider is retired, got %v", err)
		}
	})
}

func TestPersistGenericSelectionResetsDemotedProviderState(t *testing.T) {
	db := &mockdal.Dal{}
	tx := &mockdal.Transaction{}

	db.On("Begin").Return(tx)
	tx.On("Rollback").Return(nil)
	tx.On("Commit").Return(nil)

	var demoteSets []dal.DalSet
	tx.On("UpdateColumns", mock.AnythingOfType("*access.OIDCProvider"), mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		sets := args.Get(1).([]dal.DalSet)
		// Capture the first update which demotes previous generic providers
		if len(demoteSets) == 0 {
			demoteSets = sets
		}
	}).Return(nil)

	service := &Service{db: db}
	newProvider := &OIDCProvider{
		ProviderKey: "okta",
		Revision:    3,
	}
	newProvider.ID = 2

	err := service.persistGenericSelection(newProvider)
	if err != nil {
		t.Fatalf("persistGenericSelection() error = %v", err)
	}

	// Verify demoted provider sets contain target: none, sync_status: not_applicable, synced_revision: 0
	setMap := make(map[string]interface{})
	for _, s := range demoteSets {
		setMap[s.ColumnName] = s.Value
	}

	if setMap["grafana_target"] != GrafanaProviderNone {
		t.Fatalf("demoted grafana_target = %v, want %s", setMap["grafana_target"], GrafanaProviderNone)
	}
	if setMap["grafana_sync_status"] != OIDCProviderStatusNotApplicable {
		t.Fatalf("demoted grafana_sync_status = %v, want %s", setMap["grafana_sync_status"], OIDCProviderStatusNotApplicable)
	}
	if setMap["grafana_synced_revision"] != uint64(0) {
		t.Fatalf("demoted grafana_synced_revision = %v, want 0", setMap["grafana_synced_revision"])
	}
	if setMap["grafana_last_error_code"] != "" {
		t.Fatalf("demoted grafana_last_error_code = %v, want empty string", setMap["grafana_last_error_code"])
	}

	// Verify newly selected provider fields
	if newProvider.GrafanaTarget != GrafanaProviderGenericOAuth {
		t.Fatalf("newProvider.GrafanaTarget = %v, want %s", newProvider.GrafanaTarget, GrafanaProviderGenericOAuth)
	}
	if newProvider.GrafanaSyncStatus != OIDCProviderStatusSynchronized {
		t.Fatalf("newProvider.GrafanaSyncStatus = %v, want %s", newProvider.GrafanaSyncStatus, OIDCProviderStatusSynchronized)
	}
	if newProvider.GrafanaSyncedRevision != 3 {
		t.Fatalf("newProvider.GrafanaSyncedRevision = %v, want 3", newProvider.GrafanaSyncedRevision)
	}
}
