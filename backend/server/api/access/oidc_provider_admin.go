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
	"strings"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
)

const (
	auditProviderCreated                   = "provider.created"
	auditProviderUpdated                   = "provider.updated"
	auditProviderActivated                 = "provider.database_activated"
	auditProviderEnabled                   = "provider.enabled"
	auditProviderDisabled                  = "provider.disabled"
	auditProviderRetired                   = "provider.retired"
	auditProviderGrafanaSyncSucceeded      = "provider.grafana_sync_succeeded"
	auditProviderGrafanaSyncFailed         = "provider.grafana_sync_failed"
	auditProviderGrafanaCompensationFailed = "provider.grafana_sync_compensation_failed"
	auditProviderGrafanaTargetSelected     = "provider.grafana_target_selected"
)

func (s *Service) GetOIDCProviders() ([]*OIDCProviderResponse, errors.Error) {
	providers := make([]OIDCProvider, 0)
	if err := s.db.All(&providers, dal.Where("retired_at IS NULL"), dal.Orderby("provider_key ASC")); err != nil {
		return nil, errors.Default.Wrap(err, "error listing OIDC providers")
	}
	configuration, err := s.databaseOIDCConfiguration()
	if err != nil {
		return nil, err
	}
	responses := make([]*OIDCProviderResponse, 0, len(providers))
	for index := range providers {
		provider := &providers[index]
		if _, candidate, candidateErr := s.currentOIDCProvider(provider.ProviderKey); candidateErr != nil {
			return nil, candidateErr
		} else if candidate != nil {
			response := oidcProviderResponse(effectiveOIDCProvider(provider, candidate), configuration)
			response.HasCandidate = true
			responses = append(responses, response)
			continue
		}
		responses = append(responses, oidcProviderResponse(provider, configuration))
	}
	return responses, nil
}

func (s *Service) ValidateOIDCProvider(ctx context.Context, input OIDCProviderInput) errors.Error {
	if _, _, err := s.oidcProviderCallbacks(); err != nil {
		return err
	}
	provider, secret, err := normalizeOIDCProviderInput(input)
	if err != nil {
		return err
	}
	if s.oidcRuntime == nil {
		return errors.Unavailable.New("OIDC provider administration is not configured", errors.WithData(ErrCodeProviderBlocked))
	}
	if _, err := s.resolveOIDCProviderInput(provider, secret); err != nil {
		return err
	}
	_, runtimeErr := s.oidcRuntime.PrepareOIDCProvider(ctx, provider, secret)
	return runtimeErr
}

func (s *Service) SaveOIDCProvider(ctx context.Context, actor string, input OIDCProviderInput) (*OIDCProviderResponse, errors.Error) {
	s.oidcLifecycleMu.Lock()
	defer s.oidcLifecycleMu.Unlock()

	if _, _, err := s.oidcProviderCallbacks(); err != nil {
		return nil, err
	}
	provider, secret, err := normalizeOIDCProviderInput(input)
	if err != nil {
		return nil, err
	}
	if s.oidcRuntime == nil {
		return nil, errors.Unavailable.New("OIDC provider administration is not configured", errors.WithData(ErrCodeProviderBlocked))
	}
	current, resolveErr := s.resolveOIDCProviderInput(provider, secret)
	if resolveErr != nil {
		return nil, resolveErr
	}
	if current != nil && input.Revision != 0 && current.Revision != input.Revision {
		return nil, errors.BadInput.New("the OIDC provider changed; refresh it before saving", errors.WithData(ErrCodeProviderRevisionConflict))
	}
	currentID := uint64(0)
	if current != nil {
		currentID = current.ID
	}
	if issuerErr := s.ensureIssuerAvailable(provider.IssuerURL, currentID); issuerErr != nil {
		return nil, issuerErr
	}
	if conflictErr := s.validateGrafanaTargetAssignment(provider, current); conflictErr != nil {
		return nil, conflictErr
	}
	prepared, prepareErr := s.oidcRuntime.PrepareOIDCProvider(ctx, provider, secret)
	if prepareErr != nil {
		return nil, prepareErr
	}
	persisted, saveErr := s.persistOIDCCandidate(provider, prepared, current)
	if saveErr != nil {
		return nil, saveErr
	}
	action := auditProviderCreated
	if current != nil {
		action = auditProviderUpdated
	}
	s.audit(actor, action, nil, providerAuditDetail(persisted.ProviderKey))
	return s.providerResponse(persisted), nil
}

func (s *Service) resolveOIDCProviderInput(provider *OIDCProvider, clientSecret string) (*OIDCProvider, errors.Error) {
	current := &OIDCProvider{}
	if err := s.db.First(current, dal.Where("provider_key = ? AND retired_at IS NULL", provider.ProviderKey)); err != nil {
		if s.db.IsErrorNotFound(err) {
			return nil, reuseOIDCProviderCredential(provider, nil, clientSecret)
		}
		return nil, errors.Default.Wrap(err, "error reading OIDC provider")
	}
	if current.IssuerURL != provider.IssuerURL {
		return nil, errors.BadInput.New("an existing OIDC provider issuer cannot be changed; create a new provider instead", errors.WithData(ErrCodeProviderBlocked))
	}
	credentialSource := current
	if _, candidate, err := s.currentOIDCProvider(provider.ProviderKey); err != nil {
		return nil, err
	} else if candidate != nil {
		credentialSource = oidcProviderFromCandidate(candidate)
	}
	if err := reuseOIDCProviderCredential(provider, credentialSource, clientSecret); err != nil {
		return nil, err
	}
	return current, nil
}

func (s *Service) validateGrafanaTargetAssignment(provider, current *OIDCProvider) errors.Error {
	if provider.GrafanaTarget == GrafanaProviderNone {
		if current != nil && current.GrafanaTarget != GrafanaProviderNone {
			return errors.BadInput.New("an active Grafana provider mapping cannot be removed by editing; select a replacement or retire the provider", errors.WithData(ErrCodeProviderBlocked))
		}
		return nil
	}
	if current != nil && current.GrafanaTarget != provider.GrafanaTarget {
		return errors.BadInput.New("an active Grafana provider mapping cannot be changed by editing; use the explicit Grafana selection action", errors.WithData(ErrCodeProviderBlocked))
	}
	currentID := uint64(0)
	if current != nil {
		currentID = current.ID
	}
	return s.ensureGrafanaTargetAvailable(provider.GrafanaTarget, currentID)
}

func (s *Service) ensureGrafanaTargetAvailable(target GrafanaProviderKind, providerID uint64) errors.Error {
	if target == GrafanaProviderNone {
		return nil
	}
	providers := make([]OIDCProvider, 0)
	if err := s.db.All(&providers, dal.Where("grafana_target = ? AND retired_at IS NULL", target)); err != nil {
		return errors.Default.Wrap(err, "error checking Grafana provider target")
	}
	for _, existing := range providers {
		if existing.ID != providerID {
			return errors.BadInput.New("another OIDC provider already controls this Grafana login", errors.WithData(ErrCodeGrafanaTargetConflict))
		}
	}
	return nil
}

func (s *Service) ensureIssuerAvailable(issuerURL string, providerID uint64) errors.Error {
	existing := &OIDCProvider{}
	if err := s.db.First(existing, dal.Where("issuer_url = ? AND retired_at IS NULL AND id <> ?", issuerURL, providerID)); err == nil {
		return errors.BadInput.New("another active OIDC provider already uses this issuer URL", errors.WithData(ErrCodeInvalidProvider))
	} else if !s.db.IsErrorNotFound(err) {
		return errors.Default.Wrap(err, "error checking existing OIDC provider issuer")
	}
	return nil
}

func (s *Service) databaseOIDCConfiguration() (*OIDCProviderConfiguration, errors.Error) {
	configuration := &OIDCProviderConfiguration{}
	if err := s.db.First(configuration, dal.Where("id = ?", OIDCProviderSourceKey)); err != nil {
		if s.db.IsErrorNotFound(err) {
			return &OIDCProviderConfiguration{}, nil
		}
		return nil, errors.Default.Wrap(err, "error reading OIDC provider configuration")
	}
	return configuration, nil
}

func normalizeOIDCProviderKey(value string) (string, errors.Error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !validOIDCProviderKey(value) {
		return "", errors.BadInput.New("provide a valid OIDC provider key", errors.WithData(ErrCodeInvalidProvider))
	}
	return value, nil
}

func (s *Service) providerResponse(provider *OIDCProvider) *OIDCProviderResponse {
	configuration, err := s.databaseOIDCConfiguration()
	if err != nil {
		return nil
	}
	return oidcProviderResponse(provider, configuration)
}
