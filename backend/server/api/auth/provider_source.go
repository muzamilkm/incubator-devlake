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
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/apache/incubator-devlake/core/context"
	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/helpers/oidchelper"
	"github.com/apache/incubator-devlake/server/api/access"
)

// providerSourceReadError identifies a transient failure while reading the
// database source. Callers can retain a last-known-good runtime configuration
// for this error while failing closed for a confirmed invalid source.
type providerSourceReadError struct {
	cause error
}

func (e *providerSourceReadError) Error() string { return e.cause.Error() }
func (e *providerSourceReadError) Unwrap() error { return e.cause }

const (
	oidcCredentialKeyIDConfig         = "AUTH_OIDC_CREDENTIAL_KEY_ID"
	oidcCredentialKeyConfig           = "AUTH_OIDC_CREDENTIAL_KEY"
	oidcCredentialPreviousKeyIDConfig = "AUTH_OIDC_CREDENTIAL_PREVIOUS_KEY_ID"
	oidcCredentialPreviousKeyConfig   = "AUTH_OIDC_CREDENTIAL_PREVIOUS_KEY"
)

// loadProviderSource preserves environment providers until the activation record
// exists. Once it does, database providers are authoritative. An empty provider set
// with an active source fails closed rather than falling back to environment credentials.
// Individual invalid providers are omitted with redacted warnings. Operators recover an
// unusable active source by restoring or enabling a valid provider through supported
// administration before restarting.
func loadProviderSource(cfg *oidchelper.Config, db dal.Dal, config context.BasicRes) (*oidchelper.Config, CredentialProtector, []error, error) {
	providers, databaseSource, err := access.LoadDatabaseOIDCProviders(db)
	if err != nil {
		return nil, nil, nil, &providerSourceReadError{cause: fmt.Errorf("load database OIDC providers: %w", err)}
	}
	if !databaseSource {
		return cfg, nil, nil, nil
	}
	protector, loadErr := loadCredentialProtector(config)
	if loadErr != nil {
		return nil, nil, nil, loadErr
	}
	if len(providers) == 0 {
		return nil, nil, nil, fmt.Errorf("database OIDC source is active but has no enabled provider")
	}
	effective := *cfg
	effective.OIDCEnabled = true
	if cfg.PublicURL == "" {
		return nil, nil, nil, fmt.Errorf("AUTH_PUBLIC_URL is required when database OIDC configuration is active")
	}
	allowHTTP := allowLocalOIDC(cfg.PublicURL)
	effective.Providers = make(map[string]*oidchelper.ProviderConfig, len(providers))
	warnings := make([]error, 0)
	for _, provider := range providers {
		providerConfig, providerErr := databaseProviderRuntimeConfig(cfg.PublicURL, allowHTTP, protector, &provider)
		if providerErr != nil {
			warnings = append(warnings, fmt.Errorf("database OIDC provider %q omitted: %w", provider.ProviderKey, providerErr))
			continue
		}
		effective.Providers[provider.ProviderKey] = providerConfig
	}
	if len(effective.Providers) == 0 {
		return nil, nil, warnings, fmt.Errorf("database OIDC source has no usable enabled providers")
	}
	return &effective, protector, warnings, nil
}

func databaseProviderRuntimeConfig(publicURL string, allowHTTP bool, protector CredentialProtector, provider *access.OIDCProvider) (*oidchelper.ProviderConfig, error) {
	secret, err := protector.Unprotect(ProtectedCredential{Ciphertext: provider.EncryptedClientSecret, Nonce: provider.ClientSecretNonce, KeyID: provider.ClientSecretKeyID}, providerCredentialAAD(provider.ProviderKey))
	if err != nil {
		return nil, fmt.Errorf("decrypt credential: %w", err)
	}
	issuerURL, err := oidchelper.ValidateIssuerURL(provider.IssuerURL, allowHTTP)
	if err != nil {
		return nil, fmt.Errorf("validate issuer URL: %w", err)
	}
	return &oidchelper.ProviderConfig{
		Name: provider.ProviderKey, IssuerURL: issuerURL.String(), ClientID: provider.ClientID,
		ClientSecret: string(secret), RedirectURL: publicURL + publicOIDCCallbackPath, DisplayName: provider.DisplayName,
		Scopes:     strings.FieldsFunc(provider.Scopes, func(r rune) bool { return r == ',' || r == ' ' }),
		HTTPClient: oidchelper.NewRestrictedHTTPClient(allowHTTP),
	}, nil
}

func allowLocalOIDC(publicURL string) bool {
	return oidchelper.AllowsLocalOIDCURL(publicURL)
}

func loadCredentialProtector(basicRes context.BasicRes) (CredentialProtector, error) {
	config := basicRes.GetConfigReader()
	primaryKey, err := decodeCredentialKey(config.GetString(oidcCredentialKeyConfig))
	if err != nil {
		return nil, err
	}
	previousKey, err := decodeCredentialKey(config.GetString(oidcCredentialPreviousKeyConfig))
	if err != nil {
		return nil, err
	}
	return newAESGCMKeyring(strings.TrimSpace(config.GetString(oidcCredentialKeyIDConfig)), primaryKey,
		strings.TrimSpace(config.GetString(oidcCredentialPreviousKeyIDConfig)), previousKey)
}

func decodeCredentialKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("OIDC credential key must be base64 encoded")
	}
	return key, nil
}

func providerCredentialAAD(providerKey string) []byte {
	return []byte(fmt.Sprintf("auth_oidc_providers:%s:client_secret", providerKey))
}
