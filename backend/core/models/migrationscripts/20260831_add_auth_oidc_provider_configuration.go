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
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/models/migrationscripts/archived"
	"github.com/apache/incubator-devlake/core/plugin"
	"github.com/apache/incubator-devlake/helpers/migrationhelper"
)

var _ plugin.MigrationScript = (*addAuthOIDCProviderConfiguration)(nil)

type authOIDCProviderConfiguration20260831 struct {
	ID                    string `gorm:"primaryKey;type:varchar(64)"`
	ActivatedAt           *time.Time
	CandidateProviderID   uint64 `gorm:"index:idx_auth_oidc_provider_candidate"`
	ProviderRevision      uint64 `gorm:"not null;default:0"`
	GrafanaSyncStatus     string `gorm:"type:varchar(32);not null;default:'pending'"`
	GrafanaSyncedRevision uint64 `gorm:"not null;default:0"`
	GrafanaLastSyncedAt   *time.Time
	GrafanaLastErrorCode  string `gorm:"type:varchar(64)"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (authOIDCProviderConfiguration20260831) TableName() string {
	return "auth_oidc_provider_configuration"
}

type authOIDCProvider20260831 struct {
	archived.Model
	ProviderKey           string     `gorm:"type:varchar(64);uniqueIndex:idx_auth_oidc_provider_key"`
	DisplayName           string     `gorm:"type:varchar(255)"`
	IssuerURL             string     `gorm:"type:varchar(512);index:idx_auth_oidc_provider_issuer"`
	ClientID              string     `gorm:"type:varchar(512)"`
	EncryptedClientSecret []byte
	ClientSecretNonce     []byte
	ClientSecretKeyID     string     `gorm:"type:varchar(64)"`
	Scopes                string     `gorm:"type:text"`
	Enabled               bool       `gorm:"index:idx_auth_oidc_provider_enabled"`
	RetiredAt             *time.Time `gorm:"index:idx_auth_oidc_provider_retired"`
}

func (authOIDCProvider20260831) TableName() string { return "auth_oidc_providers" }

type authOIDCProviderCandidate20260831 struct {
	archived.Model
	ProviderKey           string `gorm:"type:varchar(64);index:idx_auth_oidc_provider_candidate_key"`
	DisplayName           string `gorm:"type:varchar(255)"`
	IssuerURL             string `gorm:"type:varchar(512)"`
	ClientID              string `gorm:"type:varchar(512)"`
	EncryptedClientSecret []byte
	ClientSecretNonce     []byte
	ClientSecretKeyID     string     `gorm:"type:varchar(64)"`
	Scopes                string     `gorm:"type:text"`
	Revision              uint64     `gorm:"not null"`
	PromotedAt            *time.Time `gorm:"index"`
}

func (authOIDCProviderCandidate20260831) TableName() string { return "auth_oidc_provider_candidates" }

type addAuthOIDCProviderConfiguration struct{}

func (*addAuthOIDCProviderConfiguration) Up(basicRes context.BasicRes) errors.Error {
	return migrationhelper.AutoMigrateTables(basicRes,
		new(authOIDCProviderConfiguration20260831),
		new(authOIDCProvider20260831),
		new(authOIDCProviderCandidate20260831),
	)
}

func (*addAuthOIDCProviderConfiguration) Version() uint64 { return 20260831000001 }
func (*addAuthOIDCProviderConfiguration) Name() string {
	return "add database OIDC provider configuration"
}
