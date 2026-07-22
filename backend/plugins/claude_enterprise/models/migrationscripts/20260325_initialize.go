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
	"github.com/apache/incubator-devlake/core/context"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/models/migrationscripts/archived"
	"github.com/apache/incubator-devlake/helpers/migrationhelper"
)

// addClaudeEnterpriseInitialTables creates the initial Claude Enterprise tool-layer tables.
type addClaudeEnterpriseInitialTables struct{}

func (script *addClaudeEnterpriseInitialTables) Up(basicRes context.BasicRes) errors.Error {
	return migrationhelper.AutoMigrateTables(
		basicRes,
		&claudeConnection20260325{},
		&claudeScope20260325{},
		&claudeScopeConfig20260325{},
		&claudeAnalyticsRecord20260325{},
	)
}

func (script *addClaudeEnterpriseInitialTables) Version() uint64 {
	return 20260325000001
}

func (script *addClaudeEnterpriseInitialTables) Name() string {
	return "add Claude Enterprise initial tables"
}

type claudeConnection20260325 struct {
	archived.Model
	Name             string `gorm:"type:varchar(100);uniqueIndex" json:"name"`
	Endpoint         string `gorm:"type:varchar(255)" json:"endpoint"`
	Proxy            string `gorm:"type:varchar(255)" json:"proxy"`
	RateLimitPerHour int    `json:"rateLimitPerHour"`
	AnalyticsApiKey  string `json:"analyticsApiKey"`
	OrganizationId   string `gorm:"type:varchar(255)" json:"organizationId"`
}

func (claudeConnection20260325) TableName() string {
	return "_tool_claude_enterprise_connections"
}

type claudeScope20260325 struct {
	archived.NoPKModel
	ConnectionId   uint64 `json:"connectionId" gorm:"primaryKey"`
	ScopeConfigId  uint64 `json:"scopeConfigId,omitempty"`
	Id             string `json:"id" gorm:"primaryKey;type:varchar(255)"`
	Name           string `json:"name" gorm:"type:varchar(255)"`
	OrganizationId string `json:"organizationId" gorm:"type:varchar(255)"`
}

func (claudeScope20260325) TableName() string {
	return "_tool_claude_enterprise_scopes"
}

type claudeScopeConfig20260325 struct {
	archived.Model
	ConnectionId uint64 `json:"connectionId" gorm:"primaryKey"`
	Name         string `gorm:"type:varchar(255)" json:"name"`
}

func (claudeScopeConfig20260325) TableName() string {
	return "_tool_claude_enterprise_scope_configs"
}

type claudeAnalyticsRecord20260325 struct {
	archived.NoPKModel
	ConnectionId   uint64 `gorm:"primaryKey" json:"connectionId"`
	ScopeId        string `gorm:"primaryKey;type:varchar(128)" json:"scopeId"`
	OrganizationId string `gorm:"primaryKey;type:varchar(128)" json:"organizationId"`
	Endpoint       string `gorm:"primaryKey;type:varchar(100)" json:"endpoint"`
	RecordId       string `gorm:"primaryKey;type:varchar(64)" json:"recordId"`
	Date           string `gorm:"type:varchar(32)" json:"date"`
	Grain          string `gorm:"type:varchar(32)" json:"grain"`
	UserId         string `gorm:"type:varchar(255)" json:"userId"`
	UserEmail      string `gorm:"type:varchar(255)" json:"userEmail"`
	Product        string `gorm:"type:varchar(100)" json:"product"`
	Model          string `gorm:"type:varchar(255)" json:"model"`
	RawJson        string `gorm:"type:longtext" json:"rawJson"`
}

func (claudeAnalyticsRecord20260325) TableName() string {
	return "_tool_claude_enterprise_analytics_records"
}
