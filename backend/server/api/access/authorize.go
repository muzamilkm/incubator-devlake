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

	"github.com/gin-gonic/gin"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
)

const identityContextKey = "devlake_access_identity"

func SetIdentity(c *gin.Context, identity Identity) { c.Set(identityContextKey, identity) }

func GetIdentity(c *gin.Context) (Identity, bool) {
	value, ok := c.Get(identityContextKey)
	if !ok {
		return Identity{}, false
	}
	identity, ok := value.(Identity)
	return identity, ok
}

// Authorize accepts only a verified OIDC identity. It is invoked before a
// DevLake session is issued, so denied identities never receive a cookie.
func (s *Service) Authorize(identity Identity) (*Principal, errors.Error) {
	if !s.Enabled() {
		return &Principal{}, nil
	}
	identity.Email = normalizeEmail(identity.Email)
	if identity.Issuer == "" || identity.Subject == "" || identity.Email == "" {
		return nil, errors.Unauthorized.New("verified OIDC identity is incomplete")
	}

	user, accessIdentity, err := s.userForIdentity(s.db, identity)
	if err == nil {
		return s.authorizeExistingUser(user, accessIdentity, identity)
	}
	if !s.db.IsErrorNotFound(err) {
		return nil, errors.Default.Wrap(err, "error looking up access user")
	}
	invitation, invitationFound, invitationErr := s.claimInvitation(identity)
	if invitationErr != nil {
		return nil, invitationErr
	}
	if invitationFound {
		s.audit("", "user.invitation_claimed", invitation, "")
		claimedUser, claimedIdentity, lookupErr := s.userForIdentity(s.db, identity)
		if lookupErr != nil {
			return nil, errors.Default.Wrap(lookupErr, "error reading claimed access identity")
		}
		return s.authorizeExistingUser(claimedUser, claimedIdentity, identity)
	}
	if existingUserErr := s.rejectExistingEmailIdentity(identity.Email); existingUserErr != nil {
		return nil, existingUserErr
	}

	if principal, bootstrapErr := s.bootstrap(identity); bootstrapErr != nil || principal != nil {
		return principal, bootstrapErr
	}

	domain, ok := emailDomain(identity.Email)
	if !ok {
		return nil, errors.Unauthorized.New("verified email is invalid")
	}
	accessDomain := &AccessDomain{}
	err = s.db.First(accessDomain, dal.Where("domain = ? AND hidden_at IS NULL", domain))
	if err == nil && accessDomain.Status == StatusActive {
		user = &AccessUser{
			Issuer: identity.Issuer, Subject: identity.Subject, Email: identity.Email,
			DisplayName: identity.DisplayName, Role: accessDomain.DefaultRole, Status: StatusActive,
		}
		if createErr := s.createUserWithIdentity(user, identity); createErr != nil {
			if s.db.IsDuplicationError(createErr) {
				existing, existingIdentity, lookupErr := s.userForIdentity(s.db, identity)
				if lookupErr == nil {
					return s.authorizeExistingUser(existing, existingIdentity, identity)
				}
				if s.db.IsErrorNotFound(lookupErr) {
					return nil, errors.Default.New("domain-authorized user was not available after duplicate identity creation")
				}
				return nil, errors.Default.Wrap(lookupErr, "error reading domain-authorized user after duplicate identity creation")
			}
			return nil, errors.Default.Wrap(createErr, "error creating domain-authorized user")
		}
		s.audit("", "user.domain_provisioned", user, "")
		return &Principal{UserID: user.ID, Role: user.Role}, nil
	}
	if err != nil && !s.db.IsErrorNotFound(err) {
		return nil, errors.Default.Wrap(err, "error looking up access domain")
	}
	return nil, errors.Unauthorized.New("this account is not allowed to access DevLake")
}

func (s *Service) bootstrap(identity Identity) (*Principal, errors.Error) {
	if s.cfg.BootstrapAdminEmail == "" || identity.Email != s.cfg.BootstrapAdminEmail {
		return nil, nil
	}
	var user *AccessUser
	err := s.withTransaction("bootstrap administrator", func(tx dal.Transaction) errors.Error {
		count, err := tx.Count(dal.From(&AccessUser{}))
		if err != nil {
			return errors.Default.Wrap(err, "error checking access bootstrap state")
		}
		if count != 0 {
			return nil
		}
		if err := tx.Create(&BootstrapClaim{Key: bootstrapClaimKey}); err != nil {
			if tx.IsDuplicationError(err) {
				return err
			}
			return errors.Default.Wrap(err, "error claiming bootstrap administrator")
		}
		now := time.Now()
		u := &AccessUser{
			Issuer: identity.Issuer, Subject: identity.Subject, Email: identity.Email,
			DisplayName: identity.DisplayName, Role: RoleCustomerAdmin, Status: StatusActive, LastLoginAt: &now,
		}
		if err := tx.Create(u); err != nil {
			return errors.Default.Wrap(err, "error creating bootstrap administrator")
		}
		if err := tx.Create(newAccessIdentity(u.ID, identity, now)); err != nil {
			return errors.Default.Wrap(err, "error creating bootstrap administrator identity")
		}
		user = u
		return nil
	})
	if err != nil {
		if s.db.IsDuplicationError(err) {
			existing, existingIdentity, lookupErr := s.userForIdentity(s.db, identity)
			if lookupErr == nil {
				return s.authorizeExistingUser(existing, existingIdentity, identity)
			} else if !s.db.IsErrorNotFound(lookupErr) {
				return nil, errors.Default.Wrap(lookupErr, "error reading bootstrap administrator")
			}
			return nil, errors.Unauthorized.New("the bootstrap administrator has already been claimed")
		}
		return nil, err
	}
	if user == nil {
		return nil, nil
	}
	s.audit(identity.Email, "bootstrap.consumed", user, "")
	s.logger.Info("access: bootstrap administrator provisioned email=%s", identity.Email)
	return &Principal{UserID: user.ID, Role: user.Role}, nil
}

// claimInvitation conditionally binds an email invitation to the verified OIDC
// identity. A concurrent claimant can update only an unclaimed row; the final
// read authorizes the winner and rejects every other identity.
func (s *Service) claimInvitation(identity Identity) (*AccessUser, bool, errors.Error) {
	var claimed *AccessUser
	err := s.withTransaction("invitation claim", func(tx dal.Transaction) errors.Error {
		invitation := &AccessUser{}
		err := tx.First(invitation, dal.Where("issuer = ? AND subject = ? AND hidden_at IS NULL", "", invitationSubject(identity.Email)))
		if err != nil {
			if tx.IsErrorNotFound(err) {
				return nil
			}
			return errors.Default.Wrap(err, "error looking up invited access user")
		}
		now := time.Now()
		if err := tx.UpdateColumns(
			&AccessUser{},
			[]dal.DalSet{
				{ColumnName: "issuer", Value: identity.Issuer},
				{ColumnName: "subject", Value: identity.Subject},
				{ColumnName: "email", Value: identity.Email},
				{ColumnName: "display_name", Value: identity.DisplayName},
				{ColumnName: "last_login_at", Value: now},
			},
			dal.Where("id = ? AND issuer = ? AND subject = ?", invitation.ID, "", invitationSubject(identity.Email)),
		); err != nil {
			return errors.Default.Wrap(err, "error claiming invited access user")
		}
		c := &AccessUser{}
		if err := tx.First(c, dal.Where("id = ?", invitation.ID)); err != nil {
			return errors.Default.Wrap(err, "error reading claimed access user")
		}
		if c.Issuer != identity.Issuer || c.Subject != identity.Subject {
			return errors.Unauthorized.New("this invitation has already been claimed")
		}
		if err := tx.Create(newAccessIdentity(c.ID, identity, now)); err != nil {
			if tx.IsDuplicationError(err) {
				return errors.Unauthorized.New("this OIDC identity is already linked to another account")
			}
			return errors.Default.Wrap(err, "error creating invited access identity")
		}
		claimed = c
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if claimed == nil {
		return nil, false, nil
	}
	return claimed, true, nil
}

func (s *Service) authorizeExistingUser(user *AccessUser, accessIdentity *AccessIdentity, identity Identity) (*Principal, errors.Error) {
	if user.HiddenAt != nil || user.Status != StatusActive {
		return nil, errors.Unauthorized.New("this account is disabled")
	}
	now := time.Now()
	err := s.withTransaction("identity login", func(tx dal.Transaction) errors.Error {
		accessIdentity.VerifiedEmail = identity.Email
		accessIdentity.DisplayName = identity.DisplayName
		accessIdentity.LastLoginAt = &now
		if err := tx.Update(accessIdentity); err != nil {
			return errors.Default.Wrap(err, "error recording access identity login")
		}
		if err := tx.UpdateColumns(user, []dal.DalSet{{ColumnName: "last_login_at", Value: &now}}, dal.Where("id = ?", user.ID)); err != nil {
			return errors.Default.Wrap(err, "error recording access user login")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	user.LastLoginAt = &now
	return &Principal{UserID: user.ID, Role: user.Role}, nil
}

func (s *Service) CurrentPrincipal(c *gin.Context) (*Principal, errors.Error) {
	if !s.Enabled() {
		return nil, errors.HttpStatus(404).New("access management is not enabled")
	}
	identity, ok := GetIdentity(c)
	if !ok {
		return nil, errors.Unauthorized.New("native OIDC authentication is required")
	}
	return s.AuthorizeSession(identity)
}

// AuthorizeSession validates a previously issued native OIDC session against the
// current access directory without updating login metadata on every request.
func (s *Service) AuthorizeSession(identity Identity) (*Principal, errors.Error) {
	if !s.Enabled() {
		return &Principal{}, nil
	}
	if identity.Issuer == "" || identity.Subject == "" {
		return nil, errors.Unauthorized.New("native session identity is incomplete")
	}
	user, _, err := s.userForIdentity(s.db, identity)
	if err != nil {
		if s.db.IsErrorNotFound(err) {
			return nil, errors.Unauthorized.New("this account is not allowed to access DevLake")
		}
		return nil, errors.Default.Wrap(err, "error looking up current access user")
	}
	if user.HiddenAt != nil || user.Status != StatusActive {
		return nil, errors.Unauthorized.New("this account is disabled")
	}
	return &Principal{UserID: user.ID, Role: user.Role}, nil
}

func (s *Service) createUserWithIdentity(user *AccessUser, identity Identity) errors.Error {
	return s.withTransaction("user creation", func(tx dal.Transaction) errors.Error {
		if err := tx.Create(user); err != nil {
			return err
		}
		now := time.Now()
		return tx.Create(newAccessIdentity(user.ID, identity, now))
	})
}

func newAccessIdentity(userID uint64, identity Identity, linkedAt time.Time) *AccessIdentity {
	return &AccessIdentity{
		AccessUserID: userID,
		Issuer:       identity.Issuer, Subject: identity.Subject,
		VerifiedEmail: identity.Email, DisplayName: identity.DisplayName,
		LinkedAt: linkedAt, LastLoginAt: &linkedAt,
	}
}

func (s *Service) userForIdentity(db dal.Dal, identity Identity) (*AccessUser, *AccessIdentity, errors.Error) {
	accessIdentity := &AccessIdentity{}
	if err := db.First(accessIdentity, dal.Where("issuer = ? AND subject = ?", identity.Issuer, identity.Subject)); err != nil {
		return nil, nil, err
	}
	user := &AccessUser{}
	if err := db.First(user, dal.Where("id = ?", accessIdentity.AccessUserID)); err != nil {
		return nil, nil, err
	}
	return user, accessIdentity, nil
}

func (s *Service) rejectExistingEmailIdentity(email string) errors.Error {
	user := &AccessUser{}
	if err := s.db.First(user, dal.Where("email = ?", email)); err != nil {
		if s.db.IsErrorNotFound(err) {
			return nil
		}
		return errors.Default.Wrap(err, "error checking access user email")
	}
	return errors.Unauthorized.New("this account must link the new OIDC identity while signed in")
}

func (s *Service) RequireAdmin(c *gin.Context) (*Principal, errors.Error) {
	principal, err := s.CurrentPrincipal(c)
	if err != nil {
		return nil, err
	}
	if principal.Role != RoleCustomerAdmin {
		return nil, errors.Forbidden.New("customer administrator access is required")
	}
	return principal, nil
}
