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

	"github.com/google/uuid"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
)

// LinkableOIDCProviders returns enabled providers that are not already linked
// to the authenticated access user. Provider selection remains server-owned:
// callers can start linking only with one of these non-secret provider keys.
func (s *Service) LinkableOIDCProviders(identity Identity) ([]LinkableOIDCProviderResponse, errors.Error) {
	principal, err := s.AuthorizeSession(identity)
	if err != nil {
		return nil, err
	}
	identities := make([]AccessIdentity, 0)
	if err := s.db.All(&identities, dal.Where("access_user_id = ?", principal.UserID)); err != nil {
		return nil, errors.Default.Wrap(err, "error reading linked OIDC identities")
	}
	linkedIssuers := make(map[string]struct{}, len(identities))
	for _, linkedIdentity := range identities {
		linkedIssuers[linkedIdentity.Issuer] = struct{}{}
	}
	providers := make([]OIDCProvider, 0)
	if err := s.db.All(&providers, dal.Where("enabled = ? AND retired_at IS NULL", true), dal.Orderby("provider_key ASC")); err != nil {
		return nil, errors.Default.Wrap(err, "error reading enabled OIDC providers")
	}
	linkable := make([]LinkableOIDCProviderResponse, 0, len(providers))
	for _, provider := range providers {
		if _, linked := linkedIssuers[provider.IssuerURL]; linked {
			continue
		}
		linkable = append(linkable, LinkableOIDCProviderResponse{ProviderKey: provider.ProviderKey, DisplayName: provider.DisplayName})
	}
	return linkable, nil
}

const identityLinkStateLifetime = 10 * time.Minute

// BeginIdentityLink creates the server-side half of a fresh provider-bound OIDC
// linking flow. Auth encrypts only the opaque state ID into its browser state cookie.
func (s *Service) BeginIdentityLink(userID uint64, providerKey string) (string, errors.Error) {
	user := &AccessUser{}
	if err := s.db.First(user, dal.Where("id = ?", userID)); err != nil {
		if s.db.IsErrorNotFound(err) {
			return "", errors.Unauthorized.New("this account is not allowed to access DevLake")
		}
		return "", errors.Default.Wrap(err, "error reading access user for identity link")
	}
	if user.HiddenAt != nil || user.Status != StatusActive {
		return "", errors.Unauthorized.New("this account is disabled")
	}

	now := time.Now()
	state := &IdentityLinkState{
		ID: uuid.NewString(), AccessUserID: userID, ProviderKey: providerKey,
		ExpiresAt: now.Add(identityLinkStateLifetime),
	}
	if err := s.db.Create(state); err != nil {
		return "", errors.Default.Wrap(err, "error creating OIDC identity link state")
	}
	return state.ID, nil
}

// CompleteIdentityLink consumes a server-side link state and attaches the freshly
// verified OIDC identity to its existing access user. It never uses email equality as
// an ownership signal.
func (s *Service) CompleteIdentityLink(stateID, providerKey string, identity Identity) errors.Error {
	identity.Email = normalizeEmail(identity.Email)
	if stateID == "" || providerKey == "" || identity.Issuer == "" || identity.Subject == "" || identity.Email == "" {
		return errors.Unauthorized.New("OIDC identity link is invalid")
	}

	tx := s.db.Begin()
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				s.logger.Error(rollbackErr, "access: rollback OIDC identity link")
			}
		}
	}()

	state := &IdentityLinkState{}
	if err := tx.First(state, dal.Where("id = ? AND provider_key = ? AND expires_at > ? AND consumed_at IS NULL", stateID, providerKey, time.Now())); err != nil {
		if tx.IsErrorNotFound(err) {
			return errors.Unauthorized.New("OIDC identity link has expired or was already used")
		}
		return errors.Default.Wrap(err, "error reading OIDC identity link state")
	}
	if err := tx.Create(&IdentityLinkClaim{StateID: state.ID}); err != nil {
		if tx.IsDuplicationError(err) {
			return errors.Unauthorized.New("OIDC identity link has expired or was already used")
		}
		return errors.Default.Wrap(err, "error claiming OIDC identity link state")
	}

	user := &AccessUser{}
	if err := tx.First(user, dal.Where("id = ?", state.AccessUserID)); err != nil {
		if tx.IsErrorNotFound(err) {
			return errors.Unauthorized.New("this account is not allowed to access DevLake")
		}
		return errors.Default.Wrap(err, "error reading access user for OIDC identity link")
	}
	if user.HiddenAt != nil || user.Status != StatusActive {
		return errors.Unauthorized.New("this account is disabled")
	}

	existing := &AccessIdentity{}
	if err := tx.First(existing, dal.Where("issuer = ? AND subject = ?", identity.Issuer, identity.Subject)); err == nil {
		return errors.BadInput.New("this OIDC identity is already linked", errors.WithData(ErrCodeIdentityLinked))
	} else if !tx.IsErrorNotFound(err) {
		return errors.Default.Wrap(err, "error reading existing OIDC identity")
	}

	now := time.Now()
	if err := tx.Create(newAccessIdentity(user.ID, identity, now)); err != nil {
		if tx.IsDuplicationError(err) {
			return errors.BadInput.New("this OIDC identity is already linked", errors.WithData(ErrCodeIdentityLinked))
		}
		return errors.Default.Wrap(err, "error creating linked OIDC identity")
	}
	if err := tx.UpdateColumns(&IdentityLinkState{}, []dal.DalSet{{ColumnName: "consumed_at", Value: now}}, dal.Where("id = ?", state.ID)); err != nil {
		return errors.Default.Wrap(err, "error consuming OIDC identity link state")
	}
	if err := tx.Commit(); err != nil {
		return errors.Default.Wrap(err, "error committing OIDC identity link")
	}
	committed = true
	s.audit(user.Email, "identity.linked", user, "provider="+providerKey)
	return nil
}
