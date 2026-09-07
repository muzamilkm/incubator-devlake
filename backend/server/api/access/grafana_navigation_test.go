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
	"testing"

	"github.com/apache/incubator-devlake/core/errors"
	mockdal "github.com/apache/incubator-devlake/mocks/core/dal"
	"github.com/stretchr/testify/mock"
)

func TestGrafanaLoginURL(t *testing.T) {
	t.Run("uses the enabled provider selected by the authenticated issuer", func(t *testing.T) {
		db := &mockdal.Dal{}
		db.On("First", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			provider := args.Get(0).(*OIDCProvider)
			provider.GrafanaTarget = GrafanaProviderGoogle
		}).Return(nil)
		service := &Service{cfg: Config{GrafanaPublicURL: "https://grafana.customer.example"}, db: db}

		response, err := service.GrafanaLoginURL(Identity{Issuer: "https://accounts.google.com"})
		if err != nil {
			t.Fatalf("GrafanaLoginURL() error = %v", err)
		}
		if response.URL != "https://grafana.customer.example/login/google" {
			t.Fatalf("GrafanaLoginURL() URL = %q", response.URL)
		}
	})

	t.Run("falls back to Grafana local login when no enabled provider matches", func(t *testing.T) {
		db := &mockdal.Dal{}
		notFoundErr := errors.NotFound.New("not found")
		db.On("First", mock.Anything, mock.Anything).Return(notFoundErr)
		db.On("IsErrorNotFound", notFoundErr).Return(true)
		service := &Service{cfg: Config{GrafanaPublicURL: "https://grafana.customer.example"}, db: db}

		response, err := service.GrafanaLoginURL(Identity{Issuer: "https://login.microsoftonline.com/customer/v2.0"})
		if err != nil {
			t.Fatalf("GrafanaLoginURL() error = %v", err)
		}
		if response.URL != "https://grafana.customer.example/login" {
			t.Fatalf("GrafanaLoginURL() URL = %q", response.URL)
		}
	})
}
