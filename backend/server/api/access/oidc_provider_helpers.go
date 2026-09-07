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
	"fmt"
	"net/url"
	"strings"

	"github.com/apache/incubator-devlake/core/errors"
)

func normalizeOIDCProviderInput(input OIDCProviderInput) (*OIDCProvider, string, errors.Error) {
	providerKey := strings.ToLower(strings.TrimSpace(input.ProviderKey))
	issuerURL := strings.TrimSpace(input.IssuerURL)
	clientID := strings.TrimSpace(input.ClientID)
	displayName := strings.TrimSpace(input.DisplayName)
	scopes := normalizeOIDCScopes(input.Scopes)
	if !validOIDCProviderKey(providerKey) || issuerURL == "" || clientID == "" || displayName == "" || scopes == "" {
		return nil, "", errors.BadInput.New("provide valid OIDC provider settings", errors.WithData(ErrCodeInvalidProvider))
	}
	target, err := normalizeGrafanaProviderKind(input.GrafanaTarget, input.ConfirmDevLakeOnly)
	if err != nil {
		return nil, "", err
	}
	provider := &OIDCProvider{
		ProviderKey: providerKey, DisplayName: displayName, IssuerURL: issuerURL,
		ClientID: clientID, Scopes: scopes, GrafanaTarget: target,
	}
	if err := validateGrafanaProviderCompatibility(provider); err != nil {
		return nil, "", err
	}
	return provider, strings.TrimSpace(input.ClientSecret), nil
}

func normalizeGrafanaProviderKind(kind GrafanaProviderKind, confirmDevLakeOnly bool) (GrafanaProviderKind, errors.Error) {
	switch kind {
	case GrafanaProviderNone:
		if !confirmDevLakeOnly {
			return "", errors.BadInput.New("confirm DevLake-only access before saving an unmapped OIDC provider", errors.WithData(ErrCodeInvalidProvider))
		}
	case GrafanaProviderGoogle, GrafanaProviderAzureAD, GrafanaProviderOkta, GrafanaProviderGitLab, GrafanaProviderGenericOAuth:
	default:
		return "", errors.BadInput.New("select a supported Grafana authentication provider", errors.WithData(ErrCodeInvalidProvider))
	}
	return kind, nil
}

func validateGrafanaProviderCompatibility(provider *OIDCProvider) errors.Error {
	if provider == nil || provider.GrafanaTarget == GrafanaProviderNone || provider.GrafanaTarget == GrafanaProviderGenericOAuth {
		return nil
	}
	issuer, err := url.Parse(provider.IssuerURL)
	if err != nil {
		return errors.BadInput.New("provide valid OIDC provider settings", errors.WithData(ErrCodeInvalidProvider))
	}
	host := strings.ToLower(issuer.Hostname())
	compatible := false
	switch provider.GrafanaTarget {
	case GrafanaProviderGoogle:
		compatible = host == "accounts.google.com"
	case GrafanaProviderAzureAD:
		compatible = host == "login.microsoftonline.com"
	case GrafanaProviderOkta:
		compatible = strings.HasSuffix(host, ".okta.com") || strings.HasSuffix(host, ".oktapreview.com")
	case GrafanaProviderGitLab:
		compatible = host == "gitlab.com"
	}
	if !compatible {
		return errors.BadInput.New("the selected Grafana provider is not compatible with this OIDC issuer", errors.WithData(ErrCodeInvalidProvider))
	}
	return nil
}

func validOIDCProviderKey(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') && !(character >= '0' && character <= '9') && character != '-' && character != '_' {
			return false
		}
	}
	return true
}

func normalizeOIDCScopes(raw string) string {
	seen := make(map[string]struct{})
	ordered := make([]string, 0)
	for _, scope := range strings.FieldsFunc(raw, func(character rune) bool { return character == ',' || character == ' ' }) {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		ordered = append(ordered, scope)
	}
	if _, ok := seen["openid"]; !ok {
		return ""
	}
	return strings.Join(ordered, " ")
}

func oidcProviderResponse(provider *OIDCProvider, configuration *OIDCProviderConfiguration) *OIDCProviderResponse {
	return &OIDCProviderResponse{
		ProviderKey: provider.ProviderKey, DisplayName: provider.DisplayName, IssuerURL: provider.IssuerURL,
		ClientID: provider.ClientID, Scopes: provider.Scopes, Enabled: provider.Enabled, RetiredAt: provider.RetiredAt,
		SecretConfigured:     hasOIDCProviderSecret(provider),
		DatabaseSourceActive: configuration.ActivatedAt != nil, GrafanaTarget: provider.GrafanaTarget,
		GrafanaSyncStatus: provider.GrafanaSyncStatus, GrafanaSyncedRevision: provider.GrafanaSyncedRevision,
		ProviderRevision: provider.Revision,
	}
}

func hasOIDCProviderSecret(provider *OIDCProvider) bool {
	return provider != nil && len(provider.EncryptedClientSecret) > 0 && len(provider.ClientSecretNonce) > 0 && provider.ClientSecretKeyID != ""
}

// reuseOIDCProviderCredential preserves ciphertext only when the OAuth client
// remains unchanged. A client ID change must include a replacement secret.
func reuseOIDCProviderCredential(provider, stored *OIDCProvider, clientSecret string) errors.Error {
	if clientSecret != "" {
		return nil
	}
	if stored == nil {
		return errors.BadInput.New("client secret is required", errors.WithData(ErrCodeInvalidProvider))
	}
	if provider.ClientID != stored.ClientID {
		return errors.BadInput.New("a replacement client secret is required when changing the client ID", errors.WithData(ErrCodeInvalidProvider))
	}
	if !hasOIDCProviderSecret(stored) {
		return errors.Default.New("stored OIDC provider credential is unavailable")
	}
	provider.EncryptedClientSecret = stored.EncryptedClientSecret
	provider.ClientSecretNonce = stored.ClientSecretNonce
	provider.ClientSecretKeyID = stored.ClientSecretKeyID
	return nil
}

func providerAuditDetail(providerKey string) string { return fmt.Sprintf("provider=%s", providerKey) }
