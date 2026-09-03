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
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/impls/logruslog"
	"github.com/apache/incubator-devlake/server/api/shared"
)

type currentResponse struct {
	Enabled bool   `json:"enabled"`
	Role    string `json:"role,omitempty"`
}

// outputError keeps access-directory validation messages useful without exposing
// stack traces when local development enables them globally.
func outputError(c *gin.Context, err errors.Error) {
	status := err.GetType().GetHttpCode()
	message := "unable to process access request"
	code := accessErrorCode(err)
	if safeAccessError(status, code) {
		if safeMessage := err.Messages().Get(); safeMessage != "" {
			message = strings.TrimSuffix(safeMessage, fmt.Sprintf(" (%d)", status))
		}
	} else {
		code = ""
	}
	logruslog.Global.Error(err, "HTTP %d access API error", status)
	c.JSON(status, &ApiErrorResponse{Success: false, Message: message, Code: code})
}

func accessErrorCode(err errors.Error) string {
	if errData := err.GetData(); errData != nil {
		if code, ok := errData.(string); ok {
			return code
		}
	}
	return ""
}

func safeAccessError(status int, code string) bool {
	if status >= http.StatusBadRequest && status < http.StatusInternalServerError {
		return true
	}
	return status == http.StatusServiceUnavailable && code == ErrCodeProviderBlocked
}

func GetCurrent(c *gin.Context) {
	service := Default()
	if service == nil || !service.Enabled() {
		shared.ApiOutputSuccess(c, currentResponse{Enabled: false}, http.StatusOK)
		return
	}
	principal, err := service.CurrentPrincipal(c)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, currentResponse{Enabled: true, Role: principal.Role}, http.StatusOK)
}

func GetGrafanaLogin(c *gin.Context) {
	service := Default()
	if service == nil || !service.Enabled() {
		outputError(c, errors.Unauthorized.New("native OIDC authentication is required"))
		return
	}
	identity, ok := GetIdentity(c)
	if !ok {
		outputError(c, errors.Unauthorized.New("native OIDC authentication is required"))
		return
	}
	response, err := service.GrafanaLoginURL(identity)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, response, http.StatusOK)
}

func ListLinkableOIDCProviders(c *gin.Context) {
	service := Default()
	if service == nil || !service.Enabled() {
		outputError(c, errors.Unauthorized.New("native OIDC authentication is required"))
		return
	}
	identity, ok := GetIdentity(c)
	if !ok {
		outputError(c, errors.Unauthorized.New("native OIDC authentication is required"))
		return
	}
	providers, err := service.LinkableOIDCProviders(identity)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, providers, http.StatusOK)
}

func ListUsers(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	query, ok := listQuery(c)
	if !ok {
		return
	}
	users, err := Default().ListUsers(query)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, users, http.StatusOK)
}

func PostUser(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	input := CreateUserInput{}
	if err := c.ShouldBindJSON(&input); err != nil {
		outputError(c, errors.BadInput.Wrap(err, "invalid access user", errors.WithData(ErrCodeInvalidUser)))
		return
	}
	actor, _ := GetIdentity(c)
	user, err := Default().CreateUser(actor.Email, input.Email, input.Role)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, user, http.StatusCreated)
}

func ListDomains(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	query, ok := listQuery(c)
	if !ok {
		return
	}
	domains, err := Default().ListDomains(query)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, domains, http.StatusOK)
}

func ListAuditEvents(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	events, err := Default().ListAuditEvents()
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, events, http.StatusOK)
}

func PostDomain(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	input := CreateDomainInput{}
	if err := c.ShouldBindJSON(&input); err != nil {
		outputError(c, errors.BadInput.Wrap(err, "invalid access domain", errors.WithData(ErrCodeInvalidDomain)))
		return
	}
	actor, _ := GetIdentity(c)
	domain, err := Default().CreateDomain(actor.Email, AccessDomain{Domain: input.Domain, DefaultRole: input.DefaultRole})
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, domain, http.StatusCreated)
}

func PatchDomain(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	id, ok := accessID(c, "domain")
	if !ok {
		return
	}
	input := UpdateDomainInput{}
	if err := c.ShouldBindJSON(&input); err != nil {
		outputError(c, errors.BadInput.Wrap(err, "invalid access domain update", errors.WithData(ErrCodeInvalidDomain)))
		return
	}
	actor, _ := GetIdentity(c)
	domain, err := Default().UpdateDomain(actor.Email, id, input.DefaultRole, input.Status)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, domain, http.StatusOK)
}

func HideDomain(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	id, ok := accessID(c, "domain")
	if !ok {
		return
	}
	actor, _ := GetIdentity(c)
	domain, err := Default().HideDomain(actor.Email, id)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, domain, http.StatusOK)
}

func PatchUser(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	id, ok := accessID(c, "user")
	if !ok {
		return
	}
	input := UpdateUserInput{}
	if err := c.ShouldBindJSON(&input); err != nil {
		outputError(c, errors.BadInput.Wrap(err, "invalid access user update", errors.WithData(ErrCodeInvalidUser)))
		return
	}
	actor, _ := GetIdentity(c)
	user, err := Default().UpdateUser(actor.Email, id, input.Role, input.Status)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, user, http.StatusOK)
}

func HideUser(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	id, ok := accessID(c, "user")
	if !ok {
		return
	}
	actor, _ := GetIdentity(c)
	user, err := Default().HideUser(actor.Email, id)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, user, http.StatusOK)
}

func GetOIDCProvider(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	provider, err := Default().GetOIDCProvider()
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, Default().decorateOIDCProviderResponse(provider), http.StatusOK)
}

func ListOIDCProviders(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	providers, err := Default().GetOIDCProviders()
	if err != nil {
		outputError(c, err)
		return
	}
	for _, provider := range providers {
		Default().decorateOIDCProviderResponse(provider)
	}
	shared.ApiOutputSuccess(c, providers, http.StatusOK)
}

func GetOIDCProviderCallbacks(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	callbacks, err := Default().OIDCProviderCallbacks()
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, callbacks, http.StatusOK)
}

func ValidateOIDCProvider(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	input, ok := oidcProviderInput(c)
	if !ok {
		return
	}
	if err := Default().ValidateOIDCProvider(c.Request.Context(), input); err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, nil, http.StatusNoContent)
}

func PutOIDCProvider(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	input, ok := oidcProviderInput(c)
	if !ok {
		return
	}
	actor, _ := GetIdentity(c)
	provider, err := Default().SaveOIDCProvider(c.Request.Context(), actor.Email, input)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, Default().decorateOIDCProviderResponse(provider), http.StatusOK)
}

func ActivateOIDCProvider(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	actor, _ := GetIdentity(c)
	providerKey, ok := singletonProviderKey(c)
	if !ok {
		return
	}
	provider, err := Default().ActivateOIDCProvider(c.Request.Context(), actor.Email, providerKey)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, Default().decorateOIDCProviderResponse(provider), http.StatusOK)
}

func EnableOIDCProvider(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	actor, _ := GetIdentity(c)
	providerKey, ok := singletonProviderKey(c)
	if !ok {
		return
	}
	provider, err := Default().EnableOIDCProvider(c.Request.Context(), actor.Email, providerKey)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, Default().decorateOIDCProviderResponse(provider), http.StatusOK)
}

func DisableOIDCProvider(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	actor, _ := GetIdentity(c)
	providerKey, ok := singletonProviderKey(c)
	if !ok {
		return
	}
	provider, err := Default().DisableOIDCProvider(c.Request.Context(), actor.Email, providerKey)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, Default().decorateOIDCProviderResponse(provider), http.StatusOK)
}

func RetireOIDCProvider(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	actor, _ := GetIdentity(c)
	providerKey, ok := singletonProviderKey(c)
	if !ok {
		return
	}
	provider, err := Default().RetireOIDCProvider(c.Request.Context(), actor.Email, providerKey)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, Default().decorateOIDCProviderResponse(provider), http.StatusOK)
}

func RetryGrafanaOIDCProviderSync(c *gin.Context) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	actor, _ := GetIdentity(c)
	providerKey, ok := singletonProviderKey(c)
	if !ok {
		return
	}
	provider, err := Default().RetryGrafanaOIDCProviderSync(c.Request.Context(), actor.Email, providerKey)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, Default().decorateOIDCProviderResponse(provider), http.StatusOK)
}

type oidcProviderAction func(context.Context, string, string) (*OIDCProviderResponse, errors.Error)

func ActivateOIDCProviderByKey(c *gin.Context) {
	runOIDCProviderAction(c, Default().ActivateOIDCProvider)
}

func EnableOIDCProviderByKey(c *gin.Context) {
	runOIDCProviderAction(c, Default().EnableOIDCProvider)
}

func DisableOIDCProviderByKey(c *gin.Context) {
	runOIDCProviderAction(c, Default().DisableOIDCProvider)
}

func RetireOIDCProviderByKey(c *gin.Context) {
	runOIDCProviderAction(c, Default().RetireOIDCProvider)
}

func RetryGrafanaOIDCProviderSyncByKey(c *gin.Context) {
	runOIDCProviderAction(c, Default().RetryGrafanaOIDCProviderSync)
}

func SelectGenericOIDCProvider(c *gin.Context) {
	runOIDCProviderAction(c, Default().SelectGenericOIDCProvider)
}

func runOIDCProviderAction(c *gin.Context, action oidcProviderAction) {
	if _, ok := requireAdmin(c); !ok {
		return
	}
	providerKey, ok := pathProviderKey(c)
	if !ok {
		return
	}
	actor, _ := GetIdentity(c)
	provider, err := action(c.Request.Context(), actor.Email, providerKey)
	if err != nil {
		outputError(c, err)
		return
	}
	shared.ApiOutputSuccess(c, Default().decorateOIDCProviderResponse(provider), http.StatusOK)
}

func singletonProviderKey(c *gin.Context) (string, bool) {
	providerKey, err := Default().singleOIDCProviderKey()
	if err != nil {
		outputError(c, err)
		return "", false
	}
	return providerKey, true
}

func pathProviderKey(c *gin.Context) (string, bool) {
	providerKey, err := normalizeOIDCProviderKey(c.Param("providerKey"))
	if err != nil {
		outputError(c, err)
		return "", false
	}
	return providerKey, true
}

func oidcProviderInput(c *gin.Context) (OIDCProviderInput, bool) {
	input := OIDCProviderInput{}
	if err := c.ShouldBindJSON(&input); err != nil {
		outputError(c, errors.BadInput.Wrap(err, "invalid OIDC provider settings", errors.WithData(ErrCodeInvalidProvider)))
		return OIDCProviderInput{}, false
	}
	return input, true
}

func requireAdmin(c *gin.Context) (*Principal, bool) {
	principal, err := Default().RequireAdmin(c)
	if err != nil {
		outputError(c, err)
		return nil, false
	}
	return principal, true
}

func listQuery(c *gin.Context) (PageQuery, bool) {
	query := PageQuery{}
	if err := c.ShouldBindQuery(&query); err != nil {
		outputError(c, errors.BadInput.Wrap(err, "invalid access list query"))
		return PageQuery{}, false
	}
	query, valid := query.Normalize()
	if !valid {
		outputError(c, errors.BadInput.New(invalidPageSizeMessage))
		return PageQuery{}, false
	}
	return query, true
}

func accessID(c *gin.Context, resource string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		outputError(c, errors.BadInput.New("invalid access "+resource+" id"))
		return 0, false
	}
	return id, true
}
