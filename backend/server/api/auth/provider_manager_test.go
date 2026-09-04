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

package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/apache/incubator-devlake/helpers/oidchelper"
	"github.com/apache/incubator-devlake/server/api/access"
)

func TestDatabaseProviderConfigUsesPublicAPICallback(t *testing.T) {
	service := &Service{runtimeCfg: &oidchelper.Config{PublicURL: "https://devlake.example.com"}}
	provider, err := service.databaseProviderConfig(&access.OIDCProvider{
		ProviderKey: "google", DisplayName: "Google", IssuerURL: "https://accounts.example.com", ClientID: "client",
	}, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if provider.RedirectURL != "https://devlake.example.com/api/auth/callback" {
		t.Fatalf("redirect URL = %q", provider.RedirectURL)
	}
}

func TestDatabaseOIDCUnavailableConfigFailsClosed(t *testing.T) {
	base := &oidchelper.Config{AuthEnabled: true, OIDCEnabled: false, Providers: map[string]*oidchelper.ProviderConfig{"google": {Name: "google"}}}
	cfg := databaseOIDCUnavailableConfig(base)
	if !cfg.OIDCEnabled || len(cfg.Providers) != 0 {
		t.Fatalf("unavailable config = %#v", cfg)
	}
}

func TestGetMethodsUsesDatabaseProviderSnapshot(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })

	response := httptest.NewRecorder()
	requestContext, _ := gin.CreateTestContext(response)
	service := &Service{
		runtimeCfg: &oidchelper.Config{
			OIDCEnabled: true,
			Providers: map[string]*oidchelper.ProviderConfig{
				"google": {Name: "google", DisplayName: "Google"},
			},
		},
	}

	service.GetMethods(requestContext)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestGetMethodsListsMultipleProvidersInStableOrder(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })

	response := httptest.NewRecorder()
	requestContext, _ := gin.CreateTestContext(response)
	service := &Service{
		runtimeCfg: &oidchelper.Config{
			OIDCEnabled: true,
			Providers: map[string]*oidchelper.ProviderConfig{
				"okta":   {Name: "okta", DisplayName: "Okta"},
				"google": {Name: "google", DisplayName: "Google"},
			},
		},
	}

	service.GetMethods(requestContext)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	methods := Methods{}
	if err := json.Unmarshal(response.Body.Bytes(), &methods); err != nil {
		t.Fatalf("decode methods: %v", err)
	}
	if len(methods.Providers) != 2 || methods.Providers[0].Name != "google" || methods.Providers[1].Name != "okta" {
		t.Fatalf("provider methods = %#v, want stable google/okta order", methods.Providers)
	}
}
