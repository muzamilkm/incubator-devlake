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
	"strings"
	"sync"

	"github.com/apache/incubator-devlake/core/context"
	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/log"
)

type Config struct {
	Enabled             bool
	BootstrapAdminEmail string
	AuthPublicURL       string
	GrafanaPublicURL    string
}

// SessionRevoker persists revocations in the same transaction as an access-user
// disable. It returns the affected session IDs so the auth service can update its
// in-memory cache only after the transaction commits.
type SessionRevoker interface {
	RevokePersistentSessions(tx dal.Transaction, issuer, subject string) ([]string, errors.Error)
	CacheRevokedSessions(ids []string)
}

type Service struct {
	cfg             Config
	db              dal.Dal
	logger          log.Logger
	oidcLifecycleMu sync.Mutex
	sessionRevoker  SessionRevoker
	oidcRuntime     OIDCProviderRuntime
	grafanaSSO      *GrafanaSSOClient
}

var (
	defaultService *Service
	initOnce       sync.Once
)

func Init(basicRes context.BasicRes) {
	initOnce.Do(func() {
		cfg := basicRes.GetConfigReader()
		defaultService = &Service{
			cfg: Config{
				Enabled:             cfg.GetBool("AUTH_ACCESS_ENABLED"),
				BootstrapAdminEmail: normalizeEmail(cfg.GetString("AUTH_BOOTSTRAP_ADMIN_EMAIL")),
				AuthPublicURL:       strings.TrimRight(strings.TrimSpace(cfg.GetString("AUTH_PUBLIC_URL")), "/"),
				GrafanaPublicURL:    strings.TrimRight(strings.TrimSpace(cfg.GetString("GRAFANA_PUBLIC_URL")), "/"),
			},
			db:     basicRes.GetDal(),
			logger: basicRes.GetLogger(),
		}
		grafanaClient, err := NewGrafanaSSOClient(
			cfg.GetString("GRAFANA_INTERNAL_URL"),
			cfg.GetString("GRAFANA_MANAGEMENT_USER"),
			cfg.GetString("GRAFANA_MANAGEMENT_PASSWORD"),
			nil,
		)
		if err == nil {
			defaultService.grafanaSSO = grafanaClient
		}
	})
}

func (s *Service) oidcProviderCallbacks() (string, string, errors.Error) {
	if s.cfg.AuthPublicURL == "" || s.cfg.GrafanaPublicURL == "" {
		return "", "", errors.Unavailable.New("OIDC provider public URLs are not configured", errors.WithData(ErrCodeProviderBlocked))
	}
	return s.cfg.AuthPublicURL + authOIDCCallbackPath, s.cfg.GrafanaPublicURL, nil
}

func (s *Service) OIDCProviderCallbacks() (*OIDCProviderCallbacksResponse, errors.Error) {
	devLakeCallbackURL, grafanaPublicURL, err := s.oidcProviderCallbacks()
	if err != nil {
		return nil, err
	}
	callbackURLs := make(map[GrafanaProviderKind]string, 6)
	for _, target := range []GrafanaProviderKind{
		GrafanaProviderNone,
		GrafanaProviderGoogle,
		GrafanaProviderAzureAD,
		GrafanaProviderOkta,
		GrafanaProviderGitLab,
		GrafanaProviderGenericOAuth,
	} {
		callbackURLs[target] = grafanaPublicURL + grafanaLoginPath(target)
	}
	return &OIDCProviderCallbacksResponse{
		DevLakeCallbackURL:  devLakeCallbackURL,
		GrafanaCallbackURLs: callbackURLs,
	}, nil
}

func (s *Service) decorateOIDCProviderResponse(response *OIDCProviderResponse) *OIDCProviderResponse {
	if response == nil {
		return nil
	}
	devLakeCallbackURL, grafanaPublicURL, err := s.oidcProviderCallbacks()
	if err == nil {
		response.DevLakeCallbackURL = devLakeCallbackURL
		response.GrafanaCallbackURL = grafanaPublicURL + grafanaLoginPath(response.GrafanaTarget)
	}
	return response
}

func Default() *Service { return defaultService }

func SetSessionRevoker(revoker SessionRevoker) {
	if defaultService != nil {
		defaultService.sessionRevoker = revoker
	}
}

func SetOIDCProviderRuntime(runtime OIDCProviderRuntime) {
	if defaultService != nil {
		defaultService.oidcRuntime = runtime
	}
}

func (s *Service) Enabled() bool { return s != nil && s.cfg.Enabled }

// ValidateConfiguration ensures access-directory admission is backed by native
// OIDC only, rather than a legacy proxy identity that cannot consult the directory.
func ValidateConfiguration(authEnabled, oidcEnabled bool, forwardedUserSecret string) error {
	if !authEnabled {
		return fmt.Errorf("AUTH_ACCESS_ENABLED=true requires AUTH_ENABLED=true")
	}
	if !oidcEnabled {
		return fmt.Errorf("AUTH_ACCESS_ENABLED=true requires OIDC_ENABLED=true")
	}
	if strings.TrimSpace(forwardedUserSecret) != "" {
		return fmt.Errorf("AUTH_ACCESS_ENABLED=true cannot be combined with FORWARDED_USER_SECRET; remove trusted oauth2-proxy forwarded identity authentication before enabling the access directory")
	}
	return nil
}
