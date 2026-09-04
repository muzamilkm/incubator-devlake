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
	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
)

// GrafanaLoginURL chooses a public Grafana route from the authenticated session
// identity. The browser cannot supply either a provider or an internal endpoint.
func (s *Service) GrafanaLoginURL(identity Identity) (*GrafanaLoginResponse, errors.Error) {
	if s.cfg.GrafanaPublicURL == "" {
		return nil, errors.Unavailable.New("Grafana public URL is not configured", errors.WithData(ErrCodeProviderBlocked))
	}
	provider := &OIDCProvider{}
	if err := s.db.First(provider, dal.Where("issuer_url = ? AND enabled = ? AND retired_at IS NULL", identity.Issuer, true)); err != nil {
		if s.db.IsErrorNotFound(err) {
			return &GrafanaLoginResponse{URL: s.cfg.GrafanaPublicURL + grafanaLoginPath(GrafanaProviderNone)}, nil
		}
		return nil, errors.Default.Wrap(err, "error resolving OIDC provider for Grafana navigation")
	}
	return &GrafanaLoginResponse{URL: s.cfg.GrafanaPublicURL + grafanaLoginPath(provider.GrafanaTarget)}, nil
}

func grafanaLoginPath(target GrafanaProviderKind) string {
	if !isGrafanaSSOProvider(target) {
		return "/login"
	}
	return "/login/" + string(target)
}
