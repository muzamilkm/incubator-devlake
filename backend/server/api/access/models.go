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

// Package access owns the fork-specific interactive DevLake access directory.
// It intentionally does not use DevLake's imported engineering-data users or
// Grafana's independent user store.
package access

import (
	"context"
	"time"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/models/common"
)

const (
	RoleCustomerAdmin = "customer_admin"
	RoleMember        = "member"

	StatusActive   = "active"
	StatusDisabled = "disabled"

	bootstrapClaimKey = "default"

	DefaultPageSize = 10
	MediumPageSize  = 25
	LargePageSize   = 50

	invalidPageSizeMessage = "pageSize must be 10, 25, or 50"

	ErrCodeDuplicateUser            = "DUPLICATE_USER"
	ErrCodeDuplicateDomain          = "DUPLICATE_DOMAIN"
	ErrCodeInvalidUser              = "INVALID_USER"
	ErrCodeInvalidDomain            = "INVALID_DOMAIN"
	ErrCodeInvalidProvider          = "INVALID_OIDC_PROVIDER"
	ErrCodeProviderBlocked          = "OIDC_PROVIDER_BLOCKED"
	ErrCodeProviderMissing          = "OIDC_PROVIDER_MISSING"
	ErrCodeProviderRevisionConflict = "OIDC_PROVIDER_REVISION_CONFLICT"
	ErrCodeGrafanaTargetConflict    = "GRAFANA_TARGET_CONFLICT"
	ErrCodeIdentityLinked           = "OIDC_IDENTITY_LINKED"

	OIDCProviderSourceKey                = "default"
	OIDCProviderStatusPending            = "pending"
	OIDCProviderStatusSynchronized       = "synchronized"
	OIDCProviderStatusFailed             = "failed"
	OIDCProviderStatusCompensationFailed = "compensation_failed"
	OIDCProviderStatusNotApplicable      = "not_applicable"

	authOIDCCallbackPath = "/api/auth/callback"
)

// GrafanaProviderKind is the closed set of Grafana OSS SSO providers that this
// integration may configure. It is intentionally distinct from a DevLake OIDC
// provider key: a customer may name its provider freely, but cannot cause Lake
// to call an arbitrary Grafana settings endpoint.
type GrafanaProviderKind string

const (
	GrafanaProviderNone         GrafanaProviderKind = "none"
	GrafanaProviderGoogle       GrafanaProviderKind = "google"
	GrafanaProviderAzureAD      GrafanaProviderKind = "azuread"
	GrafanaProviderOkta         GrafanaProviderKind = "okta"
	GrafanaProviderGitLab       GrafanaProviderKind = "gitlab"
	GrafanaProviderGenericOAuth GrafanaProviderKind = "generic_oauth"
)

type ApiErrorResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// OIDCProviderRuntime is implemented by auth. Access owns administrative state
// transitions, while auth retains ownership of OIDC discovery, encryption, runtime
// execution, and persistent-session semantics.
type OIDCProviderRuntime interface {
	PrepareOIDCProvider(ctx context.Context, provider *OIDCProvider, clientSecret string) (*PreparedOIDCProvider, errors.Error)
	RefreshOIDCProvider(ctx context.Context) errors.Error
	RevokeProviderSessions(tx dal.Transaction, providerKey string) ([]string, errors.Error)
	CacheRevokedSessions(ids []string)
}

// PreparedOIDCProvider is deliberately internal to the backend boundary. It carries
// a write-only secret only long enough to persist ciphertext and synchronize Grafana.
type PreparedOIDCProvider struct {
	EncryptedClientSecret []byte
	ClientSecretNonce     []byte
	ClientSecretKeyID     string
	GrafanaSettings       GrafanaSSOSettings
}

type AccessUser struct {
	common.Model
	Issuer      string     `gorm:"type:varchar(512);uniqueIndex:idx_auth_access_identity" json:"issuer"`
	Subject     string     `gorm:"type:varchar(255);uniqueIndex:idx_auth_access_identity" json:"subject"`
	Email       string     `gorm:"type:varchar(255);index:idx_auth_access_email" json:"email"`
	DisplayName string     `gorm:"type:varchar(255)" json:"displayName"`
	Role        string     `gorm:"type:varchar(32)" json:"role"`
	Status      string     `gorm:"type:varchar(32);index" json:"status"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
	DisabledAt  *time.Time `json:"disabledAt,omitempty"`
	HiddenAt    *time.Time `json:"hiddenAt,omitempty"`
}

func (AccessUser) TableName() string { return "auth_access_users" }

// AccessIdentity is one verified OIDC identity owned by an access-directory user.
// AccessUser remains the authorization, role, and audit owner; an identity never
// grants access independently of its active parent user.
type AccessIdentity struct {
	common.Model
	AccessUserID  uint64     `gorm:"index:idx_auth_access_identity_user" json:"accessUserId"`
	Issuer        string     `gorm:"type:varchar(512);uniqueIndex:idx_auth_access_identity_issuer_subject" json:"issuer"`
	Subject       string     `gorm:"type:varchar(255);uniqueIndex:idx_auth_access_identity_issuer_subject" json:"subject"`
	VerifiedEmail string     `gorm:"type:varchar(255);index:idx_auth_access_identity_email" json:"verifiedEmail"`
	DisplayName   string     `gorm:"type:varchar(255)" json:"displayName"`
	LinkedAt      time.Time  `json:"linkedAt"`
	LastLoginAt   *time.Time `json:"lastLoginAt,omitempty"`
	DisabledAt    *time.Time `json:"disabledAt,omitempty"`
}

func (AccessIdentity) TableName() string { return "auth_access_identities" }

// IdentityLinkState is a one-time server-side record for a fresh OIDC callback
// that attaches an additional verified identity to an already authenticated user.
// It intentionally stores neither OAuth tokens nor client secrets.
type IdentityLinkState struct {
	ID           string     `gorm:"primaryKey;type:varchar(36)"`
	AccessUserID uint64     `gorm:"index:idx_auth_access_identity_link_user"`
	ProviderKey  string     `gorm:"type:varchar(64);index:idx_auth_access_identity_link_provider"`
	ExpiresAt    time.Time  `gorm:"index"`
	ConsumedAt   *time.Time `gorm:"index"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (IdentityLinkState) TableName() string { return "auth_access_identity_link_states" }

// IdentityLinkClaim makes link-state consumption durable and unique. DAL mutation
// methods do not expose affected-row counts, so this unique insert gives concurrent
// callback attempts an unambiguous single-use result.
type IdentityLinkClaim struct {
	StateID   string `gorm:"primaryKey;type:varchar(36)"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (IdentityLinkClaim) TableName() string { return "auth_access_identity_link_claims" }

// BootstrapClaim records that the configured bootstrap administrator has been
// consumed. Its unique key makes the first-admin transition safe across API
// processes and OIDC providers.
type BootstrapClaim struct {
	common.Model
	Key string `gorm:"type:varchar(64);uniqueIndex"`
}

func (BootstrapClaim) TableName() string { return "auth_access_bootstrap_claims" }

type AccessDomain struct {
	common.Model
	Domain      string     `gorm:"type:varchar(255);uniqueIndex" json:"domain"`
	DefaultRole string     `gorm:"type:varchar(32)" json:"defaultRole"`
	Status      string     `gorm:"type:varchar(32);index" json:"status"`
	HiddenAt    *time.Time `json:"hiddenAt,omitempty"`
}

func (AccessDomain) TableName() string { return "auth_access_domains" }

type AuditEvent struct {
	common.Model
	ActorEmail  string `gorm:"type:varchar(255);index" json:"actorEmail"`
	Action      string `gorm:"type:varchar(64);index" json:"action"`
	TargetID    uint64 `gorm:"index" json:"targetId"`
	TargetEmail string `gorm:"type:varchar(255);index" json:"targetEmail"`
	Detail      string `gorm:"type:text" json:"detail"`
}

func (AuditEvent) TableName() string { return "auth_access_audit_events" }

// OIDCProviderConfiguration makes the database OIDC source explicit. Its presence
// means environment providers are no longer authoritative; phase-two activation sets
// ActivatedAt only after Grafana accepts the matching provider revision.
type OIDCProviderConfiguration struct {
	ID                    string     `gorm:"primaryKey;type:varchar(64)"`
	ActivatedAt           *time.Time `json:"activatedAt,omitempty"`
	ProviderRevision      uint64     `gorm:"not null;default:0"`
	CandidateProviderID   uint64     `gorm:"index:idx_auth_oidc_provider_candidate"`
	GrafanaSyncStatus     string     `gorm:"type:varchar(32);not null;default:'pending'"`
	GrafanaSyncedRevision uint64     `gorm:"not null;default:0"`
	GrafanaLastSyncedAt   *time.Time `json:"grafanaLastSyncedAt,omitempty"`
	GrafanaLastErrorCode  string     `gorm:"type:varchar(64)"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (OIDCProviderConfiguration) TableName() string { return "auth_oidc_provider_configuration" }

// OIDCProvider stores customer-managed OIDC metadata. ClientSecret fields are never
// serialized and are encrypted by the auth credential protector before persistence.
type OIDCProvider struct {
	common.Model
	ProviderKey           string              `gorm:"type:varchar(64);uniqueIndex:idx_auth_oidc_provider_key" json:"providerKey"`
	DisplayName           string              `gorm:"type:varchar(255)" json:"displayName"`
	IssuerURL             string              `gorm:"type:varchar(512);uniqueIndex:idx_auth_oidc_provider_issuer" json:"issuerUrl"`
	ClientID              string              `gorm:"type:varchar(512)" json:"clientId"`
	EncryptedClientSecret []byte              `gorm:"type:blob" json:"-"`
	ClientSecretNonce     []byte              `gorm:"type:blob" json:"-"`
	ClientSecretKeyID     string              `gorm:"type:varchar(64)" json:"-"`
	Scopes                string              `gorm:"type:text" json:"scopes"`
	Enabled               bool                `gorm:"index:idx_auth_oidc_provider_enabled" json:"enabled"`
	Revision              uint64              `gorm:"not null;default:0" json:"revision"`
	RetiredAt             *time.Time          `gorm:"index:idx_auth_oidc_provider_retired" json:"retiredAt,omitempty"`
	GrafanaTarget         GrafanaProviderKind `gorm:"type:varchar(32);not null;default:'none';index:idx_auth_oidc_provider_grafana_target" json:"grafanaTarget"`
	GrafanaSyncStatus     string              `gorm:"type:varchar(32);not null;default:'pending'" json:"grafanaSyncStatus"`
	GrafanaSyncedRevision uint64              `gorm:"not null;default:0" json:"grafanaSyncedRevision"`
	GrafanaLastSyncedAt   *time.Time          `json:"grafanaLastSyncedAt,omitempty"`
	GrafanaLastErrorCode  string              `gorm:"type:varchar(64)" json:"grafanaLastErrorCode,omitempty"`
}

func (OIDCProvider) TableName() string { return "auth_oidc_providers" }

// OIDCProviderCandidate holds a pending revision separately from the active provider.
// It keeps an authenticated source live while a replacement is validated and staged in
// Grafana, and is retained after promotion for audit/recovery rather than hard-deleted.
type OIDCProviderCandidate struct {
	common.Model
	ProviderID            uint64              `gorm:"index:idx_auth_oidc_provider_candidate_provider"`
	ProviderKey           string              `gorm:"type:varchar(64);index:idx_auth_oidc_provider_candidate_key"`
	DisplayName           string              `gorm:"type:varchar(255)"`
	IssuerURL             string              `gorm:"type:varchar(512)"`
	ClientID              string              `gorm:"type:varchar(512)"`
	EncryptedClientSecret []byte              `gorm:"type:blob"`
	ClientSecretNonce     []byte              `gorm:"type:blob"`
	ClientSecretKeyID     string              `gorm:"type:varchar(64)"`
	Scopes                string              `gorm:"type:text"`
	Revision              uint64              `gorm:"not null"`
	PromotedAt            *time.Time          `gorm:"index"`
	GrafanaTarget         GrafanaProviderKind `gorm:"type:varchar(32);not null;default:'none'"`
}

func (OIDCProviderCandidate) TableName() string { return "auth_oidc_provider_candidates" }

type Identity struct {
	Issuer      string
	Subject     string
	Email       string
	DisplayName string
}

type Principal struct {
	UserID uint64
	Role   string
}

// PageQuery is the bounded, page-number query accepted by access-directory
// list endpoints. The UI deliberately exposes only the supported sizes.
type PageQuery struct {
	Page     int `form:"page"`
	PageSize int `form:"pageSize"`
}

func (query PageQuery) Normalize() (PageQuery, bool) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = DefaultPageSize
	}
	if query.PageSize != DefaultPageSize && query.PageSize != MediumPageSize && query.PageSize != LargePageSize {
		return PageQuery{}, false
	}
	return query, true
}

func (query PageQuery) Offset() int { return (query.Page - 1) * query.PageSize }

type PaginatedUsers struct {
	Users    []AccessUser `json:"users"`
	Count    int64        `json:"count"`
	Page     int          `json:"page"`
	PageSize int          `json:"pageSize"`
}

type PaginatedDomains struct {
	Domains  []AccessDomain `json:"domains"`
	Count    int64          `json:"count"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
}

type CreateUserInput struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type UpdateUserInput struct {
	Role   string `json:"role"`
	Status string `json:"status"`
}

type CreateDomainInput struct {
	Domain      string `json:"domain"`
	DefaultRole string `json:"defaultRole"`
}

type UpdateDomainInput struct {
	DefaultRole string `json:"defaultRole"`
	Status      string `json:"status"`
}

type OIDCProviderInput struct {
	ProviderKey        string              `json:"providerKey"`
	DisplayName        string              `json:"displayName"`
	IssuerURL          string              `json:"issuerUrl"`
	ClientID           string              `json:"clientId"`
	ClientSecret       string              `json:"clientSecret"`
	Scopes             string              `json:"scopes"`
	GrafanaTarget      GrafanaProviderKind `json:"grafanaTarget"`
	ConfirmDevLakeOnly bool                `json:"confirmDevlakeOnly"`
	Revision           uint64              `json:"revision"`
}

type OIDCProviderResponse struct {
	ProviderKey           string              `json:"providerKey"`
	DisplayName           string              `json:"displayName"`
	IssuerURL             string              `json:"issuerUrl"`
	ClientID              string              `json:"clientId"`
	Scopes                string              `json:"scopes"`
	Enabled               bool                `json:"enabled"`
	RetiredAt             *time.Time          `json:"retiredAt,omitempty"`
	SecretConfigured      bool                `json:"secretConfigured"`
	DatabaseSourceActive  bool                `json:"databaseSourceActive"`
	GrafanaSyncStatus     string              `json:"grafanaSyncStatus"`
	GrafanaSyncedRevision uint64              `json:"grafanaSyncedRevision"`
	ProviderRevision      uint64              `json:"providerRevision"`
	HasCandidate          bool                `json:"hasCandidate"`
	GrafanaTarget         GrafanaProviderKind `json:"grafanaTarget"`
	DevLakeCallbackURL    string              `json:"devlakeCallbackUrl"`
	GrafanaCallbackURL    string              `json:"grafanaCallbackUrl"`
}

// OIDCProviderCallbacksResponse exposes deployment-derived redirect URIs before
// an OIDC provider has been persisted. It contains no provider credentials.
type OIDCProviderCallbacksResponse struct {
	DevLakeCallbackURL  string                         `json:"devlakeCallbackUrl"`
	GrafanaCallbackURLs map[GrafanaProviderKind]string `json:"grafanaCallbackUrls"`
}

type GrafanaLoginResponse struct {
	URL string `json:"url"`
}

// LinkableOIDCProviderResponse is the deliberately minimal provider view for an
// authenticated person adding another sign-in method. It never exposes provider
// configuration or identity-link state.
type LinkableOIDCProviderResponse struct {
	ProviderKey string `json:"providerKey"`
	DisplayName string `json:"displayName"`
}
