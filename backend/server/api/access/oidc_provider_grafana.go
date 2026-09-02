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
	"time"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
)

func (s *Service) RetryGrafanaOIDCProviderSync(ctx context.Context, actor, providerKey string) (*OIDCProviderResponse, errors.Error) {
	s.oidcLifecycleMu.Lock()
	defer s.oidcLifecycleMu.Unlock()

	provider, candidate, err := s.currentOIDCProvider(providerKey)
	if err != nil {
		return nil, err
	}
	effective := effectiveOIDCProvider(provider, candidate)
	if effective.GrafanaTarget == GrafanaProviderNone {
		return s.providerResponse(provider), nil
	}
	if s.oidcRuntime == nil || s.grafanaSSO == nil {
		return nil, errors.Unavailable.New("Grafana SSO administration is not configured", errors.WithData(ErrCodeProviderBlocked))
	}
	prepared, prepareErr := s.oidcRuntime.PrepareOIDCProvider(ctx, effective, "")
	if prepareErr != nil {
		return nil, prepareErr
	}
	if syncErr := s.syncGrafana(ctx, effective, prepared.GrafanaSettings, provider.Enabled && candidate == nil); syncErr != nil {
		s.audit(actor, auditProviderGrafanaSyncFailed, nil, providerAuditDetail(providerKey))
		return nil, syncErr
	}
	s.audit(actor, auditProviderGrafanaSyncSucceeded, nil, providerAuditDetail(providerKey))
	return s.providerResponse(provider), nil
}

// SelectGenericOIDCProvider switches Grafana's single generic OAuth slot only
// after the new provider configuration has been accepted by Grafana. Database
// state is changed afterwards, so a failed external request leaves the prior
// selected provider authoritative.
func (s *Service) SelectGenericOIDCProvider(ctx context.Context, actor, providerKey string) (*OIDCProviderResponse, errors.Error) {
	s.oidcLifecycleMu.Lock()
	defer s.oidcLifecycleMu.Unlock()

	provider, candidate, err := s.currentOIDCProvider(providerKey)
	if err != nil {
		return nil, err
	}
	if candidate != nil {
		return nil, errors.BadInput.New("activate the staged OIDC provider revision before selecting it for Grafana", errors.WithData(ErrCodeProviderBlocked))
	}
	if !provider.Enabled {
		return nil, errors.BadInput.New("enable the OIDC provider before selecting it for Grafana", errors.WithData(ErrCodeProviderBlocked))
	}
	if s.oidcRuntime == nil || s.grafanaSSO == nil {
		return nil, errors.Unavailable.New("Grafana SSO administration is not configured", errors.WithData(ErrCodeProviderBlocked))
	}
	if err := validateGrafanaProviderCompatibility(&OIDCProvider{IssuerURL: provider.IssuerURL, GrafanaTarget: GrafanaProviderGenericOAuth}); err != nil {
		return nil, err
	}
	prepared, prepareErr := s.oidcRuntime.PrepareOIDCProvider(ctx, provider, "")
	if prepareErr != nil {
		return nil, prepareErr
	}
	previous, previousErr := s.selectedGrafanaProvider(GrafanaProviderGenericOAuth, provider.ID)
	if previousErr != nil {
		return nil, previousErr
	}
	prepared.GrafanaSettings.Enabled = true
	if err := s.grafanaSSO.PutProvider(ctx, GrafanaProviderGenericOAuth, prepared.GrafanaSettings); err != nil {
		s.logger.Error(err, "access: Grafana generic OAuth switch failed provider=%s", provider.ProviderKey)
		return nil, errors.Unavailable.New("Grafana OAuth configuration could not be synchronized", errors.WithData(ErrCodeProviderBlocked))
	}
	if err := s.persistGenericSelection(provider); err != nil {
		if previous == nil {
			s.recordGrafanaCompensationFailure(provider, err)
			return nil, errors.Unavailable.New("Grafana OAuth selection requires operator recovery", errors.WithData(ErrCodeProviderBlocked))
		}
		if compensationErr := s.restoreGrafanaProvider(ctx, previous); compensationErr != nil {
			s.recordGrafanaCompensationFailure(provider, compensationErr)
			return nil, errors.Unavailable.New("Grafana OAuth selection requires operator recovery", errors.WithData(ErrCodeProviderBlocked))
		}
		return nil, err
	}
	s.audit(actor, auditProviderGrafanaTargetSelected, nil, providerAuditDetail(providerKey))
	return s.providerResponse(provider), nil
}

func (s *Service) selectedGrafanaProvider(target GrafanaProviderKind, excludedID uint64) (*OIDCProvider, errors.Error) {
	providers := make([]OIDCProvider, 0)
	if err := s.db.All(&providers, dal.Where("grafana_target = ? AND retired_at IS NULL AND id <> ?", target, excludedID)); err != nil {
		return nil, errors.Default.Wrap(err, "error reading selected Grafana provider")
	}
	if len(providers) == 0 {
		return nil, nil
	}
	return &providers[0], nil
}

func (s *Service) syncGrafana(ctx context.Context, provider *OIDCProvider, settings GrafanaSSOSettings, enabled bool) errors.Error {
	if provider.GrafanaTarget == GrafanaProviderNone {
		return nil
	}
	settings.Enabled = enabled
	if err := s.grafanaSSO.PutProvider(ctx, provider.GrafanaTarget, settings); err != nil {
		s.logger.Error(err, "access: Grafana OAuth synchronization failed provider=%s target=%s", provider.ProviderKey, provider.GrafanaTarget)
		s.recordGrafanaSyncFailure(provider)
		return errors.Unavailable.New("Grafana OAuth configuration could not be synchronized", errors.WithData(ErrCodeProviderBlocked))
	}
	now := time.Now()
	if err := s.db.UpdateColumns(&OIDCProvider{}, []dal.DalSet{
		{ColumnName: "grafana_sync_status", Value: OIDCProviderStatusSynchronized},
		{ColumnName: "grafana_synced_revision", Value: provider.Revision},
		{ColumnName: "grafana_last_synced_at", Value: now},
		{ColumnName: "grafana_last_error_code", Value: ""},
	}, dal.Where("id = ?", provider.ID)); err != nil {
		return errors.Default.Wrap(err, "error recording Grafana OIDC synchronization")
	}
	provider.GrafanaSyncStatus = OIDCProviderStatusSynchronized
	provider.GrafanaSyncedRevision = provider.Revision
	provider.GrafanaLastSyncedAt = &now
	provider.GrafanaLastErrorCode = ""
	return nil
}

func (s *Service) recordGrafanaSyncFailure(provider *OIDCProvider) {
	provider.GrafanaSyncStatus = OIDCProviderStatusFailed
	provider.GrafanaLastErrorCode = ErrCodeProviderBlocked
	if err := s.db.UpdateColumns(&OIDCProvider{}, []dal.DalSet{
		{ColumnName: "grafana_sync_status", Value: provider.GrafanaSyncStatus},
		{ColumnName: "grafana_last_error_code", Value: provider.GrafanaLastErrorCode},
	}, dal.Where("id = ?", provider.ID)); err != nil {
		s.logger.Error(err, "access: record Grafana OIDC sync failure provider=%s", provider.ProviderKey)
	}
}

func (s *Service) recordGrafanaCompensationFailure(provider *OIDCProvider, cause errors.Error) {
	provider.GrafanaSyncStatus = OIDCProviderStatusCompensationFailed
	provider.GrafanaLastErrorCode = ErrCodeProviderBlocked
	if err := s.db.UpdateColumns(&OIDCProvider{}, []dal.DalSet{
		{ColumnName: "grafana_sync_status", Value: provider.GrafanaSyncStatus},
		{ColumnName: "grafana_last_error_code", Value: provider.GrafanaLastErrorCode},
	}, dal.Where("id = ?", provider.ID)); err != nil {
		s.logger.Error(err, "access: record Grafana OIDC compensation failure provider=%s", provider.ProviderKey)
	}
	s.logger.Error(cause, "access: Grafana OIDC compensation failed provider=%s", provider.ProviderKey)
}

func (s *Service) persistGenericSelection(provider *OIDCProvider) errors.Error {
	tx := s.db.Begin()
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				s.logger.Error(rollbackErr, "access: rollback Grafana generic OAuth selection provider=%s", provider.ProviderKey)
			}
		}
	}()
	if err := tx.UpdateColumns(&OIDCProvider{}, []dal.DalSet{{ColumnName: "grafana_target", Value: GrafanaProviderNone}}, dal.Where("grafana_target = ? AND id <> ?", GrafanaProviderGenericOAuth, provider.ID)); err != nil {
		return errors.Default.Wrap(err, "error clearing previous Grafana generic OAuth provider")
	}
	now := time.Now()
	if err := tx.UpdateColumns(&OIDCProvider{}, []dal.DalSet{
		{ColumnName: "grafana_target", Value: GrafanaProviderGenericOAuth},
		{ColumnName: "grafana_sync_status", Value: OIDCProviderStatusSynchronized},
		{ColumnName: "grafana_synced_revision", Value: provider.Revision},
		{ColumnName: "grafana_last_synced_at", Value: now},
		{ColumnName: "grafana_last_error_code", Value: ""},
	}, dal.Where("id = ?", provider.ID)); err != nil {
		return errors.Default.Wrap(err, "error selecting Grafana generic OAuth provider")
	}
	if err := tx.Commit(); err != nil {
		return errors.Default.Wrap(err, "error committing Grafana generic OAuth provider selection")
	}
	committed = true
	provider.GrafanaTarget = GrafanaProviderGenericOAuth
	provider.GrafanaSyncStatus = OIDCProviderStatusSynchronized
	provider.GrafanaSyncedRevision = provider.Revision
	provider.GrafanaLastSyncedAt = &now
	provider.GrafanaLastErrorCode = ""
	return nil
}
