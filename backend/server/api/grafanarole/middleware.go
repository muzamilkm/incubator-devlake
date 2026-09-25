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

package grafanarole

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/apache/incubator-devlake/server/api/access"
	"github.com/apache/incubator-devlake/server/api/shared"
)

// UserHeader is set by Grafana's datasource proxy when
// GF_DATAPROXY_SEND_USER_HEADER=true; its value is the signed-in user's login.
const UserHeader = "X-Grafana-User"

// RequireGrafanaAdmin gates a route for both caller types that can reach it:
// the Grafana datasource proxy (a REST API-key identity) and a config-ui
// session/proxy-header caller.
//
// A session/proxy-header caller must hold the DevLake customer-admin role.
// RequireAuth upstream only proves the caller is *some* authenticated member,
// not an admin, so without this check any signed-in member could call these
// routes directly (bypassing config-ui, which merely hides the controls) and
// grant themselves visibility into an arbitrary project. When the Access
// Directory itself is disabled (a legacy, role-less deployment mode), there is
// no admin/member distinction to enforce, so the caller passes through
// unchanged to preserve prior behavior for that mode.
//
// Every other outcome fails closed: no X-Grafana-User, no config, or an
// unreachable Grafana all yield 403, because the proxy admits any signed-in
// Grafana user including a Viewer.
func RequireGrafanaAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, viaApiKey := shared.GetRestAuthUser(c.Request); !viaApiKey {
			accessService := access.Default()
			if !accessService.Enabled() {
				c.Next()
				return
			}
			if _, err := accessService.RequireAdmin(c); err != nil {
				deny(c)
				return
			}
			c.Next()
			return
		}

		service := Default()
		identity := c.GetHeader(UserHeader)

		admin, err := service.IsAdmin(identity)
		if err != nil {
			if service != nil && service.logger != nil {
				service.logger.Error(err, "grafana admin check failed for %q", identity)
			}
			deny(c)
			return
		}
		if !admin {
			if service != nil && service.logger != nil {
				service.logger.Warn(nil, "grafana user %q is not an org Admin; denying user-project-mapping change", identity)
			}
			deny(c)
			return
		}
		c.Next()
	}
}

// deny returns one message for every failure mode, for both caller types this
// middleware gates: distinguishing "you are a Viewer"/"you are a member" from
// "the lookup is misconfigured" would leak configuration to an unprivileged
// caller.
func deny(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"success": false,
		"message": "administrator role required",
	})
}
