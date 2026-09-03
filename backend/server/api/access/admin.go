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
	"time"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
)

func (s *Service) ListUsers(query PageQuery) (*PaginatedUsers, errors.Error) {
	query, valid := query.Normalize()
	if !valid {
		return nil, errors.BadInput.New(invalidPageSizeMessage)
	}
	count, err := s.db.Count(dal.From(&AccessUser{}), dal.Where("hidden_at IS NULL"))
	if err != nil {
		return nil, errors.Default.Wrap(err, "error counting access users")
	}
	users := make([]AccessUser, 0)
	if err := s.db.All(&users, dal.Where("hidden_at IS NULL"), dal.Orderby("email ASC"), dal.Offset(query.Offset()), dal.Limit(query.PageSize)); err != nil {
		return nil, errors.Default.Wrap(err, "error listing access users")
	}
	return &PaginatedUsers{Users: users, Count: count, Page: query.Page, PageSize: query.PageSize}, nil
}

func (s *Service) ListDomains(query PageQuery) (*PaginatedDomains, errors.Error) {
	query, valid := query.Normalize()
	if !valid {
		return nil, errors.BadInput.New(invalidPageSizeMessage)
	}
	count, err := s.db.Count(dal.From(&AccessDomain{}), dal.Where("hidden_at IS NULL"))
	if err != nil {
		return nil, errors.Default.Wrap(err, "error counting access domains")
	}
	domains := make([]AccessDomain, 0)
	if err := s.db.All(&domains, dal.Where("hidden_at IS NULL"), dal.Orderby("domain ASC"), dal.Offset(query.Offset()), dal.Limit(query.PageSize)); err != nil {
		return nil, errors.Default.Wrap(err, "error listing access domains")
	}
	return &PaginatedDomains{Domains: domains, Count: count, Page: query.Page, PageSize: query.PageSize}, nil
}

func (s *Service) ListAuditEvents() ([]AuditEvent, errors.Error) {
	events := make([]AuditEvent, 0)
	if err := s.db.All(&events, dal.Orderby("created_at DESC"), dal.Limit(100)); err != nil {
		return nil, errors.Default.Wrap(err, "error listing access audit events")
	}
	return events, nil
}

func (s *Service) CreateDomain(actor string, input AccessDomain) (*AccessDomain, errors.Error) {
	domain := normalizeDomain(input.Domain)
	if !validDomain(domain) || !validRole(input.DefaultRole) {
		return nil, errors.BadInput.New("provide a valid domain and default role", errors.WithData(ErrCodeInvalidDomain))
	}
	existing := &AccessDomain{}
	if err := s.db.First(existing, dal.Where("domain = ?", domain)); err == nil {
		if existing.HiddenAt == nil {
			return nil, errors.BadInput.New("this domain already has a DevLake access policy", errors.WithData(ErrCodeDuplicateDomain))
		}
		existing.DefaultRole = input.DefaultRole
		existing.Status = StatusActive
		existing.HiddenAt = nil
		if updateErr := s.db.Update(existing); updateErr != nil {
			return nil, errors.Default.Wrap(updateErr, "error restoring access domain")
		}
		s.audit(actor, "domain.restored", nil, domainAuditDetail(existing.Domain))
		return existing, nil
	} else if !s.db.IsErrorNotFound(err) {
		return nil, errors.Default.Wrap(err, "error looking up access domain")
	}
	input.Domain = domain
	input.Status = StatusActive
	if err := s.db.Create(&input); err != nil {
		if s.db.IsDuplicationError(err) {
			return nil, errors.BadInput.New("this domain already has a DevLake access policy", errors.WithData(ErrCodeDuplicateDomain))
		}
		return nil, errors.Default.Wrap(err, "error creating access domain")
	}
	s.audit(actor, "domain.created", nil, domainAuditDetail(input.Domain))
	return &input, nil
}

func (s *Service) CreateUser(actor, email, role string) (*AccessUser, errors.Error) {
	email = normalizeEmail(email)
	if _, ok := emailDomain(email); !ok || !validRole(role) {
		return nil, errors.BadInput.New("provide a valid email and role", errors.WithData(ErrCodeInvalidUser))
	}
	visible := &AccessUser{}
	if err := s.db.First(visible, dal.Where("email = ? AND hidden_at IS NULL", email)); err == nil {
		return nil, errors.BadInput.New("this email already has a DevLake access entry", errors.WithData(ErrCodeDuplicateUser))
	} else if !s.db.IsErrorNotFound(err) {
		return nil, errors.Default.Wrap(err, "error looking up access user")
	}
	existing := &AccessUser{}
	if err := s.db.First(existing, dal.Where("email = ? AND hidden_at IS NOT NULL", email)); err == nil {
		existing.Role = role
		existing.Status = StatusActive
		existing.DisabledAt = nil
		existing.HiddenAt = nil
		if updateErr := s.db.Update(existing); updateErr != nil {
			return nil, errors.Default.Wrap(updateErr, "error restoring access user")
		}
		s.audit(actor, "user.restored", existing, "")
		return existing, nil
	} else if !s.db.IsErrorNotFound(err) {
		return nil, errors.Default.Wrap(err, "error looking up access user")
	}
	user := &AccessUser{
		Email: email, Role: role, Status: StatusActive,
		Subject: invitationSubject(email),
	}
	if err := s.db.Create(user); err != nil {
		if s.db.IsDuplicationError(err) {
			return nil, errors.BadInput.New("this email already has a DevLake access entry", errors.WithData(ErrCodeDuplicateUser))
		}
		return nil, errors.Default.Wrap(err, "error creating access user")
	}
	s.audit(actor, "user.invited", user, "")
	return user, nil
}

func (s *Service) UpdateDomain(actor string, id uint64, role, status string) (*AccessDomain, errors.Error) {
	if !validRole(role) || !validStatus(status) {
		return nil, errors.BadInput.New("provide a valid default role and status", errors.WithData(ErrCodeInvalidDomain))
	}
	domain := &AccessDomain{}
	if err := s.db.First(domain, dal.Where("id = ? AND hidden_at IS NULL", id)); err != nil {
		if s.db.IsErrorNotFound(err) {
			return nil, errors.NotFound.New("access domain not found")
		}
		return nil, errors.Default.Wrap(err, "error looking up access domain")
	}
	domain.DefaultRole = role
	domain.Status = status
	if err := s.db.Update(domain); err != nil {
		return nil, errors.Default.Wrap(err, "error updating access domain")
	}
	s.audit(actor, "domain.updated", nil, fmt.Sprintf("%s role=%s status=%s", domainAuditDetail(domain.Domain), role, status))
	return domain, nil
}

func (s *Service) UpdateUser(actor string, id uint64, role, status string) (*AccessUser, errors.Error) {
	return s.updateUser(actor, id, role, status, false)
}

// HideUser retains the user and its audit history but disables the account before
// excluding it from the management UI.
func (s *Service) HideUser(actor string, id uint64) (*AccessUser, errors.Error) {
	return s.updateUser(actor, id, "", StatusDisabled, true)
}

func (s *Service) updateUser(actor string, id uint64, role, status string, hide bool) (*AccessUser, errors.Error) {
	if !hide && (!validRole(role) || !validStatus(status)) {
		return nil, errors.BadInput.New("provide a valid role and status", errors.WithData(ErrCodeInvalidUser))
	}
	tx := s.db.Begin()
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				s.logger.Error(rollbackErr, "access: rollback user change id=%d", id)
			}
		}
	}()

	user := &AccessUser{}
	if err := tx.First(user, dal.Where("id = ? AND hidden_at IS NULL", id)); err != nil {
		if tx.IsErrorNotFound(err) {
			return nil, errors.NotFound.New("access user not found")
		}
		return nil, errors.Default.Wrap(err, "error looking up access user")
	}
	if hide {
		role = user.Role
		status = StatusDisabled
	}
	removesActiveAdmin := user.Role == RoleCustomerAdmin && user.Status == StatusActive && (status == StatusDisabled || role != RoleCustomerAdmin)
	if removesActiveAdmin {
		activeAdmins, err := tx.Count(dal.From(&AccessUser{}), dal.Where("role = ? AND status = ? AND hidden_at IS NULL", RoleCustomerAdmin, StatusActive))
		if err != nil {
			return nil, errors.Default.Wrap(err, "error checking customer administrators")
		}
		if activeAdmins <= 1 {
			return nil, errors.BadInput.New("keep at least one active customer administrator")
		}
	}
	user.Role = role
	user.Status = status
	if status == StatusDisabled {
		now := time.Now()
		user.DisabledAt = &now
	} else {
		user.DisabledAt = nil
	}
	if hide {
		now := time.Now()
		user.HiddenAt = &now
	}
	if err := tx.Update(user); err != nil {
		return nil, errors.Default.Wrap(err, "error saving access user")
	}
	var revokedSessionIDs []string
	if status == StatusDisabled && s.sessionRevoker != nil {
		identities := make([]AccessIdentity, 0)
		if err := tx.All(&identities, dal.Where("access_user_id = ?", user.ID)); err != nil {
			return nil, errors.Default.Wrap(err, "error reading access identities for disabled user")
		}
		for _, identity := range identities {
			providerKeys, err := s.providerKeysForIssuer(tx, identity.Issuer)
			if err != nil {
				return nil, err
			}
			ids, err := s.sessionRevoker.RevokePersistentSessions(tx, providerKeys, identity.Subject)
			if err != nil {
				s.logger.Error(err, "access: revoke sessions for disabled user id=%d email=%s", user.ID, user.Email)
				return nil, errors.Default.Wrap(err, "error revoking sessions for disabled access user")
			}
			revokedSessionIDs = append(revokedSessionIDs, ids...)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, errors.Default.Wrap(err, "error committing access user change")
	}
	committed = true
	if s.sessionRevoker != nil && len(revokedSessionIDs) > 0 {
		s.sessionRevoker.CacheRevokedSessions(revokedSessionIDs)
	}
	action := "user.updated"
	detail := fmt.Sprintf("role=%s status=%s", role, status)
	if hide {
		action = "user.hidden"
		detail = ""
	}
	s.audit(actor, action, user, detail)
	return user, nil
}

// HideDomain retains the policy and audit history but prevents new domain-based
// user provisioning before excluding it from the management UI.
func (s *Service) HideDomain(actor string, id uint64) (*AccessDomain, errors.Error) {
	domain := &AccessDomain{}
	if err := s.db.First(domain, dal.Where("id = ? AND hidden_at IS NULL", id)); err != nil {
		if s.db.IsErrorNotFound(err) {
			return nil, errors.NotFound.New("access domain not found")
		}
		return nil, errors.Default.Wrap(err, "error looking up access domain")
	}
	now := time.Now()
	domain.Status = StatusDisabled
	domain.HiddenAt = &now
	if err := s.db.Update(domain); err != nil {
		return nil, errors.Default.Wrap(err, "error hiding access domain")
	}
	s.audit(actor, "domain.hidden", nil, domainAuditDetail(domain.Domain))
	return domain, nil
}

func (s *Service) providerKeysForIssuer(tx dal.Transaction, issuer string) ([]string, errors.Error) {
	providers := make([]OIDCProvider, 0)
	if err := tx.All(&providers, dal.Where("issuer_url = ?", issuer)); err != nil {
		return nil, errors.Default.Wrap(err, "error reading providers for issuer")
	}
	keys := make([]string, 0, len(providers))
	for _, p := range providers {
		keys = append(keys, p.ProviderKey)
	}
	return keys, nil
}
