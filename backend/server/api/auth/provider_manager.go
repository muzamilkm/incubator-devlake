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
	stdctx "context"
	stderrs "errors"
	"fmt"
	"strings"

	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/helpers/oidchelper"
	"github.com/apache/incubator-devlake/server/api/access"
)

const publicOIDCCallbackPath = "/api" + PathCallback

// PrepareOIDCProvider validates discovery and produces the only internal form that
// carries a plaintext client secret. The access package persists the ciphertext and
// invokes Grafana; auth retains ownership of discovery and secret protection.
func (s *Service) PrepareOIDCProvider(ctx stdctx.Context, provider *access.OIDCProvider, clientSecret string) (*access.PreparedOIDCProvider, errors.Error) {
	if provider == nil {
		return nil, errors.BadInput.New("provide valid OIDC provider settings", errors.WithData(access.ErrCodeInvalidProvider))
	}
	cfg, err := s.databaseProviderConfig(provider, clientSecret)
	if err != nil {
		return nil, errors.BadInput.Wrap(err, "provide valid OIDC provider settings", errors.WithData(access.ErrCodeInvalidProvider))
	}
	prepared, err := s.prepareProviderCredentials(provider, clientSecret)
	if err != nil {
		return nil, errors.Default.Wrap(err, "prepare OIDC provider credential")
	}
	discovered, err := oidchelper.NewProvider(cfg).OIDC(ctx)
	if err != nil {
		return nil, errors.Unavailable.Wrap(err, "OIDC provider discovery is unavailable")
	}
	endpoint := discovered.Endpoint()
	if endpoint.AuthURL == "" || endpoint.TokenURL == "" {
		return nil, errors.BadInput.New("OIDC discovery does not provide usable OAuth endpoints", errors.WithData(access.ErrCodeInvalidProvider))
	}
	var metadata struct {
		UserInfoEndpoint string `json:"userinfo_endpoint"`
	}
	if err := discovered.Claims(&metadata); err != nil || strings.TrimSpace(metadata.UserInfoEndpoint) == "" {
		return nil, errors.BadInput.New("OIDC discovery must provide a user info endpoint", errors.WithData(access.ErrCodeInvalidProvider))
	}
	prepared.GrafanaSettings = access.GrafanaSSOSettings{
		Name:         provider.DisplayName,
		ClientID:     provider.ClientID,
		ClientSecret: clientSecret,
		AuthURL:      endpoint.AuthURL,
		TokenURL:     endpoint.TokenURL,
		APIURL:       metadata.UserInfoEndpoint,
		Scopes:       provider.Scopes,
		AllowSignUp:  false,
		AutoLogin:    false,
	}
	if prepared.GrafanaSettings.ClientSecret == "" {
		secret, decryptErr := s.providerSecret(provider)
		if decryptErr != nil {
			return nil, errors.Default.Wrap(decryptErr, "read OIDC provider credential")
		}
		prepared.GrafanaSettings.ClientSecret = string(secret)
	}
	return prepared, nil
}

// RefreshOIDCProvider atomically replaces the provider execution state after a
// committed access-owned transition. Existing requests retain their snapshot; the next
// request observes the new authoritative database provider.
func (s *Service) RefreshOIDCProvider(ctx stdctx.Context) errors.Error {
	_ = ctx
	cfg, protector, providerWarnings, err := loadProviderSource(s.bootstrapCfg, s.db, s.basicRes)
	if err != nil {
		var sourceReadErr *providerSourceReadError
		if stderrs.As(err, &sourceReadErr) {
			s.logger.Warn(err, "auth: database OIDC provider refresh failed; retaining last-known-good provider state")
			return errors.Default.Wrap(err, "refresh database OIDC provider")
		}
		s.replaceProviderState(databaseOIDCUnavailableConfig(s.bootstrapCfg))
		return errors.Default.Wrap(err, "refresh database OIDC provider")
	}
	for _, warning := range providerWarnings {
		s.logger.Warn(warning, "auth: database OIDC provider omitted from runtime")
	}
	if cfg == s.bootstrapCfg {
		return errors.Default.New("database OIDC provider source is not active")
	}
	s.replaceProviderState(cfg)
	s.providerMu.Lock()
	s.protector = protector
	s.providerMu.Unlock()
	return nil
}

func databaseOIDCUnavailableConfig(base *oidchelper.Config) *oidchelper.Config {
	cfg := *base
	cfg.OIDCEnabled = true
	cfg.Providers = map[string]*oidchelper.ProviderConfig{}
	return &cfg
}

func (s *Service) databaseProviderConfig(provider *access.OIDCProvider, clientSecret string) (*oidchelper.ProviderConfig, error) {
	baseCfg, _ := s.providerState()
	if baseCfg == nil || strings.TrimSpace(baseCfg.PublicURL) == "" {
		return nil, fmt.Errorf("AUTH_PUBLIC_URL is required")
	}
	allowHTTP := allowLocalOIDC(baseCfg.PublicURL)
	issuerURL, err := oidchelper.ValidateIssuerURL(provider.IssuerURL, allowHTTP)
	if err != nil {
		return nil, err
	}
	if clientSecret == "" && len(provider.EncryptedClientSecret) == 0 {
		return nil, fmt.Errorf("client secret is required")
	}
	return &oidchelper.ProviderConfig{
		Name: provider.ProviderKey, IssuerURL: issuerURL.String(), ClientID: provider.ClientID,
		ClientSecret: clientSecret, RedirectURL: baseCfg.PublicURL + publicOIDCCallbackPath,
		DisplayName: provider.DisplayName, Scopes: strings.Fields(provider.Scopes), HTTPClient: oidchelper.NewRestrictedHTTPClient(allowHTTP),
	}, nil
}

func (s *Service) prepareProviderCredentials(provider *access.OIDCProvider, clientSecret string) (*access.PreparedOIDCProvider, error) {
	if clientSecret == "" {
		return &access.PreparedOIDCProvider{
			EncryptedClientSecret: provider.EncryptedClientSecret,
			ClientSecretNonce:     provider.ClientSecretNonce,
			ClientSecretKeyID:     provider.ClientSecretKeyID,
		}, nil
	}
	protector, err := s.credentialProtector()
	if err != nil {
		return nil, err
	}
	credential, err := protector.Protect([]byte(clientSecret), providerCredentialAAD(provider.ProviderKey))
	if err != nil {
		return nil, err
	}
	return &access.PreparedOIDCProvider{
		EncryptedClientSecret: credential.Ciphertext,
		ClientSecretNonce:     credential.Nonce,
		ClientSecretKeyID:     credential.KeyID,
	}, nil
}

func (s *Service) providerSecret(provider *access.OIDCProvider) ([]byte, error) {
	protector, err := s.credentialProtector()
	if err != nil {
		return nil, err
	}
	return protector.Unprotect(ProtectedCredential{
		Ciphertext: provider.EncryptedClientSecret,
		Nonce:      provider.ClientSecretNonce,
		KeyID:      provider.ClientSecretKeyID,
	}, providerCredentialAAD(provider.ProviderKey))
}

func (s *Service) credentialProtector() (CredentialProtector, error) {
	s.providerMu.RLock()
	protector := s.protector
	s.providerMu.RUnlock()
	if protector != nil {
		return protector, nil
	}
	protector, err := loadCredentialProtector(s.basicRes)
	if err != nil {
		return nil, err
	}
	s.providerMu.Lock()
	if s.protector == nil {
		s.protector = protector
	} else {
		protector = s.protector
	}
	s.providerMu.Unlock()
	return protector, nil
}
