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
	"time"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
)

// persistOIDCCandidate stages an enabled provider update without changing the
// runtime provider. New and disabled providers can be stored directly because
// they are not admitting interactive users yet.
func (s *Service) persistOIDCCandidate(provider *OIDCProvider, prepared *PreparedOIDCProvider, current *OIDCProvider) (*OIDCProvider, errors.Error) {
	tx := s.db.Begin()
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				s.logger.Error(rollbackErr, "access: rollback OIDC provider candidate provider=%s", provider.ProviderKey)
			}
		}
	}()

	now := time.Now()
	provider.EncryptedClientSecret = prepared.EncryptedClientSecret
	provider.ClientSecretNonce = prepared.ClientSecretNonce
	provider.ClientSecretKeyID = prepared.ClientSecretKeyID
	provider.GrafanaSyncStatus = oidcGrafanaInitialStatus(provider.GrafanaTarget)
	provider.GrafanaSyncedRevision = 0
	provider.GrafanaLastSyncedAt = nil
	provider.GrafanaLastErrorCode = ""

	if current == nil {
		provider.Revision = 1
		provider.CreatedAt = now
		provider.UpdatedAt = now
		if err := tx.Create(provider); err != nil {
			return nil, errors.Default.Wrap(err, "error creating OIDC provider")
		}
	} else if current.Enabled {
		candidate := oidcCandidateFromProvider(provider, current.ID, current.Revision+1, now)
		if err := tx.Create(candidate); err != nil {
			return nil, errors.Default.Wrap(err, "error creating OIDC provider candidate")
		}
		if err := tx.UpdateColumns(&OIDCProvider{}, []dal.DalSet{
			{ColumnName: "revision", Value: candidate.Revision},
			{ColumnName: "grafana_sync_status", Value: provider.GrafanaSyncStatus},
			{ColumnName: "grafana_synced_revision", Value: uint64(0)},
			{ColumnName: "grafana_last_synced_at", Value: nil},
			{ColumnName: "grafana_last_error_code", Value: ""},
		}, dal.Where("id = ?", current.ID)); err != nil {
			return nil, errors.Default.Wrap(err, "error recording OIDC provider candidate")
		}
		provider.ID = current.ID
		provider.CreatedAt = current.CreatedAt
		provider.Enabled = current.Enabled
		provider.Revision = candidate.Revision
	} else {
		provider.ID = current.ID
		provider.CreatedAt = current.CreatedAt
		provider.Enabled = current.Enabled
		provider.Revision = current.Revision + 1
		provider.UpdatedAt = now
		if err := tx.Update(provider); err != nil {
			return nil, errors.Default.Wrap(err, "error updating OIDC provider")
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, errors.Default.Wrap(err, "error committing OIDC provider candidate")
	}
	committed = true
	return provider, nil
}

func oidcCandidateFromProvider(provider *OIDCProvider, providerID, revision uint64, now time.Time) *OIDCProviderCandidate {
	candidate := &OIDCProviderCandidate{
		ProviderID: providerID, ProviderKey: provider.ProviderKey, DisplayName: provider.DisplayName,
		IssuerURL: provider.IssuerURL, ClientID: provider.ClientID,
		EncryptedClientSecret: provider.EncryptedClientSecret, ClientSecretNonce: provider.ClientSecretNonce,
		ClientSecretKeyID: provider.ClientSecretKeyID, Scopes: provider.Scopes,
		Revision: revision, GrafanaTarget: provider.GrafanaTarget,
	}
	candidate.CreatedAt = now
	candidate.UpdatedAt = now
	return candidate
}

func oidcGrafanaInitialStatus(target GrafanaProviderKind) string {
	if target == GrafanaProviderNone {
		return OIDCProviderStatusNotApplicable
	}
	return OIDCProviderStatusPending
}

func (s *Service) currentOIDCProvider(providerKey string) (*OIDCProvider, *OIDCProviderCandidate, errors.Error) {
	provider := &OIDCProvider{}
	if err := s.db.First(provider, dal.Where("provider_key = ? AND retired_at IS NULL", providerKey)); err != nil {
		if s.db.IsErrorNotFound(err) {
			return nil, nil, errors.NotFound.New("OIDC provider is not configured", errors.WithData(ErrCodeProviderMissing))
		}
		return nil, nil, errors.Default.Wrap(err, "error reading OIDC provider")
	}
	candidates := make([]OIDCProviderCandidate, 0)
	if err := s.db.All(&candidates, dal.Where("provider_id = ? AND promoted_at IS NULL", provider.ID), dal.Orderby("id DESC")); err != nil {
		return nil, nil, errors.Default.Wrap(err, "error reading OIDC provider candidate")
	}
	if len(candidates) == 0 {
		return provider, nil, nil
	}
	return provider, &candidates[0], nil
}

func oidcProviderFromCandidate(candidate *OIDCProviderCandidate) *OIDCProvider {
	return &OIDCProvider{
		ProviderKey: candidate.ProviderKey, DisplayName: candidate.DisplayName, IssuerURL: candidate.IssuerURL,
		ClientID: candidate.ClientID, EncryptedClientSecret: candidate.EncryptedClientSecret,
		ClientSecretNonce: candidate.ClientSecretNonce, ClientSecretKeyID: candidate.ClientSecretKeyID,
		Scopes: candidate.Scopes, Revision: candidate.Revision, GrafanaTarget: candidate.GrafanaTarget,
	}
}

func effectiveOIDCProvider(provider *OIDCProvider, candidate *OIDCProviderCandidate) *OIDCProvider {
	if candidate == nil {
		return provider
	}
	effective := oidcProviderFromCandidate(candidate)
	effective.ID = provider.ID
	effective.CreatedAt = provider.CreatedAt
	effective.Enabled = provider.Enabled
	effective.RetiredAt = provider.RetiredAt
	effective.GrafanaSyncStatus = provider.GrafanaSyncStatus
	effective.GrafanaSyncedRevision = provider.GrafanaSyncedRevision
	effective.GrafanaLastSyncedAt = provider.GrafanaLastSyncedAt
	effective.GrafanaLastErrorCode = provider.GrafanaLastErrorCode
	return effective
}
