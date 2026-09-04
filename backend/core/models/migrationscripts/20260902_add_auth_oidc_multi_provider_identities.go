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

package migrationscripts

import (
	"time"

	"github.com/apache/incubator-devlake/core/context"
	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/models/migrationscripts/archived"
	"github.com/apache/incubator-devlake/core/plugin"
	"github.com/apache/incubator-devlake/helpers/migrationhelper"
)

var _ plugin.MigrationScript = (*addAuthOIDCMultiProviderIdentities)(nil)

type authAccessIdentity20260902 struct {
	archived.Model
	AccessUserID  uint64 `gorm:"index:idx_auth_access_identity_user"`
	Issuer        string `gorm:"type:varchar(512);uniqueIndex:idx_auth_access_identity_issuer_subject"`
	Subject       string `gorm:"type:varchar(255);uniqueIndex:idx_auth_access_identity_issuer_subject"`
	VerifiedEmail string `gorm:"type:varchar(255);index:idx_auth_access_identity_email"`
	DisplayName   string `gorm:"type:varchar(255)"`
	LinkedAt      time.Time
	LastLoginAt   *time.Time
}

func (authAccessIdentity20260902) TableName() string { return "auth_access_identities" }

type authAccessIdentityLinkState20260902 struct {
	ID           string     `gorm:"primaryKey;type:varchar(36)"`
	AccessUserID uint64     `gorm:"index:idx_auth_access_identity_link_user"`
	ProviderKey  string     `gorm:"type:varchar(64);index:idx_auth_access_identity_link_provider"`
	ExpiresAt    time.Time  `gorm:"index"`
	ConsumedAt   *time.Time `gorm:"index"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (authAccessIdentityLinkState20260902) TableName() string {
	return "auth_access_identity_link_states"
}

type authAccessIdentityLinkClaim20260902 struct {
	StateID   string `gorm:"primaryKey;type:varchar(36)"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (authAccessIdentityLinkClaim20260902) TableName() string {
	return "auth_access_identity_link_claims"
}

type authAccessUserIdentitySource20260902 struct {
	archived.Model
	Issuer      string
	Subject     string
	Email       string
	DisplayName string
	LastLoginAt *time.Time
}

func (authAccessUserIdentitySource20260902) TableName() string { return "auth_access_users" }

type authOIDCProviderMulti20260902 struct {
	IssuerURL             string `gorm:"type:varchar(512);index:idx_auth_oidc_provider_issuer"`
	Revision              uint64 `gorm:"not null;default:0"`
	GrafanaTarget         string `gorm:"type:varchar(32);not null;default:'none';index:idx_auth_oidc_provider_grafana_target"`
	GrafanaSyncStatus     string `gorm:"type:varchar(32);not null;default:'pending'"`
	GrafanaSyncedRevision uint64 `gorm:"not null;default:0"`
	GrafanaLastSyncedAt   *time.Time
	GrafanaLastErrorCode  string `gorm:"type:varchar(64)"`
}

func (authOIDCProviderMulti20260902) TableName() string { return "auth_oidc_providers" }

type authOIDCProviderCandidateMulti20260902 struct {
	archived.Model
	ProviderID    uint64 `gorm:"index:idx_auth_oidc_provider_candidate_provider"`
	GrafanaTarget string `gorm:"type:varchar(32);not null;default:'none'"`
}

func (authOIDCProviderCandidateMulti20260902) TableName() string {
	return "auth_oidc_provider_candidates"
}

type authOIDCProviderMigrationSource20260902 struct {
	archived.Model
	ProviderKey string
	RetiredAt   *time.Time
}

func (authOIDCProviderMigrationSource20260902) TableName() string { return "auth_oidc_providers" }

type authOIDCProviderCandidateMigrationSource20260902 struct {
	archived.Model
	ProviderKey string
	ProviderID  uint64
}

func (authOIDCProviderCandidateMigrationSource20260902) TableName() string {
	return "auth_oidc_provider_candidates"
}

type authOIDCProviderConfigurationMigrationSource20260902 struct {
	ID                    string
	ProviderRevision      uint64
	GrafanaSyncStatus     string
	GrafanaSyncedRevision uint64
	GrafanaLastSyncedAt   *time.Time
	GrafanaLastErrorCode  string
}

func (authOIDCProviderConfigurationMigrationSource20260902) TableName() string {
	return "auth_oidc_provider_configuration"
}

type addAuthOIDCMultiProviderIdentities struct{}

func (*addAuthOIDCMultiProviderIdentities) Up(basicRes context.BasicRes) errors.Error {
	db := basicRes.GetDal()
	if db.HasTable((authOIDCProviderMulti20260902{}).TableName()) {
		_ = db.DropIndexes((authOIDCProviderMulti20260902{}).TableName(), "idx_auth_oidc_provider_issuer")
	}

	if err := migrationhelper.AutoMigrateTables(
		basicRes,
		new(authAccessIdentity20260902),
		new(authAccessIdentityLinkState20260902),
		new(authAccessIdentityLinkClaim20260902),
		new(authOIDCProviderMulti20260902),
		new(authOIDCProviderCandidateMulti20260902),
	); err != nil {
		return err
	}

	if err := migrateAuthAccessIdentities20260902(db); err != nil {
		return err
	}
	return migrateOIDCProviderState20260902(db)
}

func migrateAuthAccessIdentities20260902(db dal.Dal) errors.Error {
	users := make([]authAccessUserIdentitySource20260902, 0)
	if err := db.All(&users, dal.Where("issuer <> ? AND subject <> ? AND subject NOT LIKE ?", "", "", "email:%")); err != nil {
		return err
	}
	for _, user := range users {
		identity := &authAccessIdentity20260902{}
		err := db.First(identity, dal.Where("issuer = ? AND subject = ?", user.Issuer, user.Subject))
		if err == nil {
			if identity.AccessUserID != user.ID {
				return errors.Default.New("existing OIDC identity is owned by a different access user")
			}
			continue
		}
		if !db.IsErrorNotFound(err) {
			return err
		}
		linkedAt := user.CreatedAt
		if user.LastLoginAt != nil {
			linkedAt = *user.LastLoginAt
		}
		if err := db.Create(&authAccessIdentity20260902{
			AccessUserID: user.ID,
			Issuer:       user.Issuer, Subject: user.Subject, VerifiedEmail: user.Email,
			DisplayName: user.DisplayName, LinkedAt: linkedAt, LastLoginAt: user.LastLoginAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

func migrateOIDCProviderState20260902(db dal.Dal) errors.Error {
	providers := make([]authOIDCProviderMigrationSource20260902, 0)
	if err := db.All(&providers); err != nil {
		return err
	}
	providerIDs := make(map[string]uint64, len(providers))
	for _, provider := range providers {
		providerIDs[provider.ProviderKey] = provider.ID
	}

	candidates := make([]authOIDCProviderCandidateMigrationSource20260902, 0)
	if err := db.All(&candidates); err != nil {
		return err
	}
	for _, candidate := range candidates {
		providerID, ok := providerIDs[candidate.ProviderKey]
		if !ok || candidate.ProviderID == providerID {
			continue
		}
		if err := db.UpdateColumn(&authOIDCProviderCandidateMulti20260902{}, "provider_id", providerID, dal.Where("id = ?", candidate.ID)); err != nil {
			return err
		}
	}

	activeProviders := make([]authOIDCProviderMigrationSource20260902, 0)
	if err := db.All(&activeProviders, dal.Where("retired_at IS NULL")); err != nil {
		return err
	}
	if len(activeProviders) != 1 || !db.HasTable((authOIDCProviderConfigurationMigrationSource20260902{}).TableName()) {
		return nil
	}
	configuration := &authOIDCProviderConfigurationMigrationSource20260902{}
	if err := db.First(configuration, dal.Where("id = ?", "default")); err != nil {
		if db.IsErrorNotFound(err) {
			return nil
		}
		return err
	}
	provider := activeProviders[0]
	return db.UpdateColumns(&authOIDCProviderMulti20260902{}, []dal.DalSet{
		{ColumnName: "revision", Value: configuration.ProviderRevision},
		{ColumnName: "grafana_target", Value: "generic_oauth"},
		{ColumnName: "grafana_sync_status", Value: configuration.GrafanaSyncStatus},
		{ColumnName: "grafana_synced_revision", Value: configuration.GrafanaSyncedRevision},
		{ColumnName: "grafana_last_synced_at", Value: configuration.GrafanaLastSyncedAt},
		{ColumnName: "grafana_last_error_code", Value: configuration.GrafanaLastErrorCode},
	}, dal.Where("id = ?", provider.ID))
}

func (*addAuthOIDCMultiProviderIdentities) Version() uint64 { return 20260902000001 }

func (*addAuthOIDCMultiProviderIdentities) Name() string {
	return "add OIDC multi-provider access identities"
}
