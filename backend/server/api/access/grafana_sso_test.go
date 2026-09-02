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
	"context"
	"io"
	"net/http"
	"testing"
)

type grafanaRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper grafanaRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func TestGrafanaSSOClientUsesDocumentedSettingsEndpoint(t *testing.T) {
	for _, provider := range []GrafanaProviderKind{GrafanaProviderGoogle, GrafanaProviderAzureAD, GrafanaProviderOkta, GrafanaProviderGitLab, GrafanaProviderGenericOAuth} {
		t.Run(string(provider), func(t *testing.T) {
			client, err := NewGrafanaSSOClient("http://grafana.internal", "devlake-system", "machine-password", &http.Client{Transport: grafanaRoundTripper(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodPut || request.URL.Path != grafanaSSOSettingsPathPrefix+string(provider) {
					t.Fatalf("request = %s %s", request.Method, request.URL.Path)
				}
				username, password, ok := request.BasicAuth()
				if !ok || username != "devlake-system" || password != "machine-password" {
					t.Fatal("missing Grafana management credentials")
				}
				return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(nil), Header: make(http.Header)}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			if err := client.PutProvider(context.Background(), provider, GrafanaSSOSettings{}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGrafanaSSOClientRedactsUpstreamResponse(t *testing.T) {
	client, err := NewGrafanaSSOClient("http://grafana.internal", "devlake-system", "machine-password", &http.Client{Transport: grafanaRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(nil), Header: make(http.Header)}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	err = client.PutProvider(context.Background(), GrafanaProviderGenericOAuth, GrafanaSSOSettings{})
	if err == nil || err.Error() != "Grafana SSO request returned status 400" {
		t.Fatalf("error = %v", err)
	}
}

func TestGrafanaSSOClientRejectsUnknownProvider(t *testing.T) {
	client, err := NewGrafanaSSOClient("http://grafana.internal", "devlake-system", "machine-password", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.PutProvider(context.Background(), GrafanaProviderKind("unknown"), GrafanaSSOSettings{}); err == nil {
		t.Fatal("expected unsupported provider to be rejected")
	}
}

func TestNewGrafanaSSOClientRequiresManagementCredentials(t *testing.T) {
	testCases := []struct {
		name     string
		baseURL  string
		username string
		password string
	}{
		{name: "missing URL", username: "devlake-system", password: "machine-password"},
		{name: "missing username", baseURL: "http://grafana.internal", password: "machine-password"},
		{name: "missing password", baseURL: "http://grafana.internal", username: "devlake-system"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client, err := NewGrafanaSSOClient(testCase.baseURL, testCase.username, testCase.password, nil)
			if err == nil || client != nil {
				t.Fatalf("NewGrafanaSSOClient() = %v, %v; want nil client and error", client, err)
			}
		})
	}
}
