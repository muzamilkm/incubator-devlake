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

func (s *Service) ActivateOIDCProvider(ctx context.Context, actor, providerKey string) (*OIDCProviderResponse, errors.Error) {
	s.oidcLifecycleMu.Lock()
	defer s.oidcLifecycleMu.Unlock()

	provider, candidate, err := s.currentOIDCProvider(providerKey)
	if err != nil {
		return nil, err
	}
	effective := effectiveOIDCProvider(provider, candidate)
	if effective.GrafanaSyncStatus == OIDCProviderStatusCompensationFailed {
		return nil, errors.Unavailable.New("OIDC provider requires operator recovery before it can be activated", errors.WithData(ErrCodeProviderBlocked))
	}
	if s.oidcRuntime == nil {
		return nil, errors.Unavailable.New("OIDC provider administration is not configured", errors.WithData(ErrCodeProviderBlocked))
	}
	if assignmentErr := s.ensureGrafanaTargetAvailable(effective.GrafanaTarget, provider.ID); assignmentErr != nil {
		return nil, assignmentErr
	}
	hasGrafanaSync := effective.GrafanaTarget != GrafanaProviderNone
	if hasGrafanaSync {
		if s.grafanaSSO == nil {
			return nil, errors.Unavailable.New("Grafana SSO administration is not configured", errors.WithData(ErrCodeProviderBlocked))
		}
		prepared, prepareErr := s.oidcRuntime.PrepareOIDCProvider(ctx, effective, "")
		if prepareErr != nil {
			return nil, prepareErr
		}
		prepared.GrafanaSettings.Enabled = true
		if syncErr := s.grafanaSSO.PutProvider(ctx, effective.GrafanaTarget, prepared.GrafanaSettings); syncErr != nil {
			s.logger.Error(syncErr, "access: Grafana OAuth synchronization failed provider=%s target=%s", effective.ProviderKey, effective.GrafanaTarget)
			s.recordGrafanaSyncFailure(provider)
			s.audit(actor, auditProviderGrafanaSyncFailed, nil, providerAuditDetail(providerKey))
			return nil, errors.Unavailable.New("Grafana OAuth configuration could not be synchronized", errors.WithData(ErrCodeProviderBlocked))
		}
	}
	if activateErr := s.activateOIDCProvider(provider, candidate, hasGrafanaSync, effective.Revision); activateErr != nil {
		if candidate != nil && hasGrafanaSync {
			if compensationErr := s.restoreGrafanaProvider(ctx, provider); compensationErr != nil {
				s.recordGrafanaCompensationFailure(provider, compensationErr)
				return nil, errors.Unavailable.New("OIDC provider activation requires operator recovery", errors.WithData(ErrCodeProviderBlocked))
			}
			if compensationErr := s.recordGrafanaCompensated(provider, provider.GrafanaSyncedRevision); compensationErr != nil {
				return nil, compensationErr
			}
		} else if hasGrafanaSync {
			s.recordGrafanaCompensationFailure(provider, activateErr)
			return nil, errors.Unavailable.New("OIDC provider activation requires operator recovery", errors.WithData(ErrCodeProviderBlocked))
		}
		return nil, activateErr
	}
	if refreshErr := s.oidcRuntime.RefreshOIDCProvider(ctx); refreshErr != nil {
		return nil, errors.Unavailable.New("OIDC provider was activated but is not ready; retry after provider discovery recovers", errors.WithData(ErrCodeProviderBlocked))
	}
	s.audit(actor, auditProviderActivated, nil, providerAuditDetail(providerKey))
	return s.providerResponse(provider)
}

func (s *Service) restoreGrafanaProvider(ctx context.Context, provider *OIDCProvider) errors.Error {
	prepared, err := s.oidcRuntime.PrepareOIDCProvider(ctx, provider, "")
	if err != nil {
		return err
	}
	prepared.GrafanaSettings.Enabled = true
	if err := s.grafanaSSO.PutProvider(ctx, provider.GrafanaTarget, prepared.GrafanaSettings); err != nil {
		return errors.Unavailable.New("Grafana OAuth configuration could not be restored", errors.WithData(ErrCodeProviderBlocked))
	}
	return nil
}

func (s *Service) EnableOIDCProvider(ctx context.Context, actor, providerKey string) (*OIDCProviderResponse, errors.Error) {
	s.oidcLifecycleMu.Lock()
	defer s.oidcLifecycleMu.Unlock()

	provider, candidate, err := s.currentOIDCProvider(providerKey)
	if err != nil {
		return nil, err
	}
	if candidate != nil {
		return nil, errors.BadInput.New("activate the staged OIDC provider revision before enabling it", errors.WithData(ErrCodeProviderBlocked))
	}
	configuration, configurationErr := s.databaseOIDCConfiguration()
	if configurationErr != nil {
		return nil, configurationErr
	}
	if configuration.ActivatedAt == nil {
		return nil, errors.BadInput.New("activate database OIDC configuration before enabling the provider", errors.WithData(ErrCodeProviderBlocked))
	}
	if provider.Enabled {
		return s.providerResponse(provider)
	}
	if provider.GrafanaTarget != GrafanaProviderNone {
		if s.grafanaSSO == nil || s.oidcRuntime == nil {
			return nil, errors.Unavailable.New("Grafana SSO administration is not configured", errors.WithData(ErrCodeProviderBlocked))
		}
		prepared, prepareErr := s.oidcRuntime.PrepareOIDCProvider(ctx, provider, "")
		if prepareErr != nil {
			return nil, prepareErr
		}
		if syncErr := s.syncGrafana(ctx, provider, prepared.GrafanaSettings, true); syncErr != nil {
			return nil, syncErr
		}
	}
	return s.setOIDCProviderEnabled(ctx, actor, provider, true)
}

func (s *Service) DisableOIDCProvider(ctx context.Context, actor, providerKey string) (*OIDCProviderResponse, errors.Error) {
	s.oidcLifecycleMu.Lock()
	defer s.oidcLifecycleMu.Unlock()

	provider, candidate, err := s.currentOIDCProvider(providerKey)
	if err != nil {
		return nil, err
	}
	if candidate != nil {
		return nil, errors.BadInput.New("activate or replace the staged OIDC provider revision before disabling it", errors.WithData(ErrCodeProviderBlocked))
	}
	if !provider.Enabled {
		return s.providerResponse(provider)
	}
	if err := s.ensureAnotherEnabledProvider(provider.ID); err != nil {
		return nil, err
	}
	return s.setOIDCProviderEnabled(ctx, actor, provider, false)
}

func (s *Service) RetireOIDCProvider(ctx context.Context, actor, providerKey string) (*OIDCProviderResponse, errors.Error) {
	s.oidcLifecycleMu.Lock()
	defer s.oidcLifecycleMu.Unlock()

	provider, candidate, err := s.currentOIDCProvider(providerKey)
	if err != nil {
		return nil, err
	}
	if candidate != nil {
		return nil, errors.BadInput.New("activate or replace the staged OIDC provider revision before retiring it", errors.WithData(ErrCodeProviderBlocked))
	}
	if provider.Enabled {
		if err := s.ensureAnotherEnabledProvider(provider.ID); err != nil {
			return nil, err
		}
	}
	return s.setOIDCProviderRetired(ctx, actor, provider)
}

func (s *Service) activateOIDCProvider(provider *OIDCProvider, candidate *OIDCProviderCandidate, syncGrafana bool, syncedRevision uint64) errors.Error {
	now := time.Now()
	err := s.withTransaction("OIDC provider activation", func(tx dal.Transaction) errors.Error {
		providerSets := []dal.DalSet{{ColumnName: "enabled", Value: true}}
		if candidate != nil {
			providerSets = append(providerSets, providerPromotionSets(candidate)...)
			if err := tx.UpdateColumns(&OIDCProviderCandidate{}, []dal.DalSet{{ColumnName: "promoted_at", Value: now}}, dal.Where("id = ?", candidate.ID)); err != nil {
				return errors.Default.Wrap(err, "error recording OIDC provider candidate promotion")
			}
		}
		if syncGrafana {
			providerSets = append(providerSets, []dal.DalSet{
				{ColumnName: "grafana_sync_status", Value: OIDCProviderStatusSynchronized},
				{ColumnName: "grafana_synced_revision", Value: syncedRevision},
				{ColumnName: "grafana_last_synced_at", Value: now},
				{ColumnName: "grafana_last_error_code", Value: ""},
			}...)
		}
		if err := tx.UpdateColumns(&OIDCProvider{}, providerSets, dal.Where("id = ? AND retired_at IS NULL", provider.ID)); err != nil {
			return errors.Default.Wrap(err, "error enabling OIDC provider")
		}
		cfg := &OIDCProviderConfiguration{}
		if err := tx.First(cfg, dal.Where("id = ?", OIDCProviderSourceKey)); err != nil {
			if tx.IsErrorNotFound(err) {
				if createErr := tx.Create(&OIDCProviderConfiguration{ID: OIDCProviderSourceKey, ActivatedAt: &now}); createErr != nil {
					return errors.Default.Wrap(createErr, "error creating database OIDC configuration")
				}
			} else {
				return errors.Default.Wrap(err, "error reading database OIDC configuration")
			}
		} else if cfg.ActivatedAt == nil {
			if updateErr := tx.UpdateColumns(&OIDCProviderConfiguration{}, []dal.DalSet{{ColumnName: "activated_at", Value: now}}, dal.Where("id = ?", OIDCProviderSourceKey)); updateErr != nil {
				return errors.Default.Wrap(updateErr, "error activating database OIDC source")
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	provider.Enabled = true
	if candidate != nil {
		applyCandidate(provider, candidate)
	}
	if syncGrafana {
		provider.GrafanaSyncStatus = OIDCProviderStatusSynchronized
		provider.GrafanaSyncedRevision = syncedRevision
		provider.GrafanaLastSyncedAt = &now
		provider.GrafanaLastErrorCode = ""
	}
	return nil
}

func providerPromotionSets(candidate *OIDCProviderCandidate) []dal.DalSet {
	return []dal.DalSet{
		{ColumnName: "display_name", Value: candidate.DisplayName}, {ColumnName: "client_id", Value: candidate.ClientID},
		{ColumnName: "encrypted_client_secret", Value: candidate.EncryptedClientSecret}, {ColumnName: "client_secret_nonce", Value: candidate.ClientSecretNonce},
		{ColumnName: "client_secret_key_id", Value: candidate.ClientSecretKeyID}, {ColumnName: "scopes", Value: candidate.Scopes},
		{ColumnName: "revision", Value: candidate.Revision}, {ColumnName: "grafana_target", Value: candidate.GrafanaTarget},
	}
}

func applyCandidate(provider *OIDCProvider, candidate *OIDCProviderCandidate) {
	provider.DisplayName = candidate.DisplayName
	provider.ClientID = candidate.ClientID
	provider.EncryptedClientSecret = candidate.EncryptedClientSecret
	provider.ClientSecretNonce = candidate.ClientSecretNonce
	provider.ClientSecretKeyID = candidate.ClientSecretKeyID
	provider.Scopes = candidate.Scopes
	provider.Revision = candidate.Revision
	provider.GrafanaTarget = candidate.GrafanaTarget
}

func (s *Service) setOIDCProviderEnabled(ctx context.Context, actor string, provider *OIDCProvider, enabled bool) (*OIDCProviderResponse, errors.Error) {
	if s.oidcRuntime == nil {
		return nil, errors.Unavailable.New("OIDC provider administration is not configured", errors.WithData(ErrCodeProviderBlocked))
	}
	var revokedIDs []string
	err := s.withTransaction("OIDC provider enabled state", func(tx dal.Transaction) errors.Error {
		if err := requireOIDCProviderState(tx, provider.ID, provider.Enabled); err != nil {
			return err
		}
		if err := tx.UpdateColumns(&OIDCProvider{}, []dal.DalSet{{ColumnName: "enabled", Value: enabled}}, dal.Where("id = ? AND retired_at IS NULL", provider.ID)); err != nil {
			return errors.Default.Wrap(err, "error updating OIDC provider enabled state")
		}
		ids, revokeErr := s.oidcRuntime.RevokeProviderSessions(tx, provider.ProviderKey)
		if revokeErr != nil {
			return errors.Default.Wrap(revokeErr, "error revoking OIDC provider sessions")
		}
		revokedIDs = ids
		return nil
	})
	if err != nil {
		return nil, err
	}
	provider.Enabled = enabled
	s.oidcRuntime.CacheRevokedSessions(revokedIDs)
	if refreshErr := s.oidcRuntime.RefreshOIDCProvider(ctx); refreshErr != nil && enabled {
		return nil, errors.Unavailable.New("OIDC provider was enabled but is not ready", errors.WithData(ErrCodeProviderBlocked))
	}
	action := auditProviderDisabled
	if enabled {
		action = auditProviderEnabled
	}
	s.audit(actor, action, nil, providerAuditDetail(provider.ProviderKey))
	return s.providerResponse(provider)
}

func (s *Service) setOIDCProviderRetired(ctx context.Context, actor string, provider *OIDCProvider) (*OIDCProviderResponse, errors.Error) {
	if s.oidcRuntime == nil {
		return nil, errors.Unavailable.New("OIDC provider administration is not configured", errors.WithData(ErrCodeProviderBlocked))
	}
	now := time.Now()
	var revokedIDs []string
	err := s.withTransaction("OIDC provider retirement", func(tx dal.Transaction) errors.Error {
		if err := requireOIDCProviderState(tx, provider.ID, provider.Enabled); err != nil {
			return err
		}
		if err := tx.UpdateColumns(&OIDCProvider{}, []dal.DalSet{{ColumnName: "enabled", Value: false}, {ColumnName: "retired_at", Value: now}}, dal.Where("id = ? AND retired_at IS NULL", provider.ID)); err != nil {
			return errors.Default.Wrap(err, "error retiring OIDC provider")
		}
		ids, revokeErr := s.oidcRuntime.RevokeProviderSessions(tx, provider.ProviderKey)
		if revokeErr != nil {
			return errors.Default.Wrap(revokeErr, "error revoking OIDC provider sessions")
		}
		revokedIDs = ids
		return nil
	})
	if err != nil {
		return nil, err
	}
	provider.Enabled = false
	provider.RetiredAt = &now
	s.oidcRuntime.CacheRevokedSessions(revokedIDs)
	if refreshErr := s.oidcRuntime.RefreshOIDCProvider(ctx); refreshErr != nil {
		return nil, errors.Unavailable.New("OIDC provider retirement completed but runtime refresh failed", errors.WithData(ErrCodeProviderBlocked))
	}
	s.audit(actor, auditProviderRetired, nil, providerAuditDetail(provider.ProviderKey))
	return s.providerResponse(provider)
}

func (s *Service) ensureAnotherEnabledProvider(providerID uint64) errors.Error {
	count, err := s.db.Count(dal.From(&OIDCProvider{}), dal.Where("enabled = ? AND retired_at IS NULL AND id <> ?", true, providerID))
	if err != nil {
		return errors.Default.Wrap(err, "error counting enabled OIDC providers")
	}
	if count == 0 {
		return errors.BadInput.New("at least one OIDC provider must remain enabled", errors.WithData(ErrCodeProviderBlocked))
	}
	return nil
}

// requireOIDCProviderState locks and verifies the state expected by a lifecycle
// transition before session revocation. DAL updates do not report affected rows, so
// this prevents a stale process from revoking sessions after another process changed
// the provider first.
func requireOIDCProviderState(tx dal.Transaction, providerID uint64, enabled bool) errors.Error {
	provider := &OIDCProvider{}
	if err := tx.First(provider,
		dal.Where("id = ? AND enabled = ? AND retired_at IS NULL", providerID, enabled),
		dal.Lock(true, false),
	); err != nil {
		if tx.IsErrorNotFound(err) {
			return errors.BadInput.New("OIDC provider state changed; refresh and retry", errors.WithData(ErrCodeProviderBlocked))
		}
		return errors.Default.Wrap(err, "error verifying OIDC provider state")
	}
	return nil
}
