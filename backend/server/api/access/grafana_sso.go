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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const grafanaSSOSettingsPathPrefix = "/api/v1/sso-settings/"

type GrafanaSSOSettings struct {
	Name         string `json:"name"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	AuthURL      string `json:"authUrl,omitempty"`
	TokenURL     string `json:"tokenUrl,omitempty"`
	APIURL       string `json:"apiUrl,omitempty"`
	Scopes       string `json:"scopes"`
	Enabled      bool   `json:"enabled"`
	AllowSignUp  bool   `json:"allowSignUp"`
	AutoLogin    bool   `json:"autoLogin"`
}

type GrafanaSSOClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

func NewGrafanaSSOClient(baseURL, username, password string, client *http.Client) (*GrafanaSSOClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || strings.TrimSpace(username) == "" || password == "" {
		return nil, fmt.Errorf("Grafana SSO API URL and management credentials are required")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &GrafanaSSOClient{baseURL: baseURL, username: username, password: password, client: client}, nil
}

// PutProvider uses Grafana's documented SSO Settings API. Its errors are
// deliberately classified without returning Grafana response bodies, which can
// contain sensitive deployment details.
func (c *GrafanaSSOClient) PutProvider(ctx context.Context, provider GrafanaProviderKind, settings GrafanaSSOSettings) error {
	if !isGrafanaSSOProvider(provider) {
		return fmt.Errorf("unsupported Grafana SSO provider")
	}
	body, err := json.Marshal(struct {
		Settings GrafanaSSOSettings `json:"settings"`
	}{Settings: settings})
	if err != nil {
		return fmt.Errorf("encode Grafana SSO settings: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+grafanaSSOSettingsPathPrefix+string(provider), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create Grafana SSO request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("Grafana SSO request failed")
	}
	defer func() {
		if response.Body != nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
	}()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Grafana SSO request returned status %d", response.StatusCode)
	}
	return nil
}

func isGrafanaSSOProvider(provider GrafanaProviderKind) bool {
	switch provider {
	case GrafanaProviderGoogle, GrafanaProviderAzureAD, GrafanaProviderOkta, GrafanaProviderGitLab, GrafanaProviderGenericOAuth:
		return true
	default:
		return false
	}
}
