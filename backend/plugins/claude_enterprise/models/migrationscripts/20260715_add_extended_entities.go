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

// addClaudeEnterpriseExtendedEntities adds the Phase 14 extended adoption
// entity tables (skills, connectors, chat projects, plugins, artifacts) as an
// additive migration. It does not modify or drop any existing table.
type addClaudeEnterpriseExtendedEntities struct{}

func (script *addClaudeEnterpriseExtendedEntities) Up(basicRes context.BasicRes) errors.Error {
	return migrationhelper.AutoMigrateTables(
		basicRes,
		&claudeSkill20260715{},
		&claudeConnector20260715{},
		&claudeChatProject20260715{},
		&claudePluginAdoption20260715{},
		&claudeArtifact20260715{},
	)
}

func (script *addClaudeEnterpriseExtendedEntities) Version() uint64 {
	return 20260715000001
}

func (script *addClaudeEnterpriseExtendedEntities) Name() string {
	return "add Claude Enterprise extended adoption entity tables"
}

type claudeSkill20260715 struct {
	archived.NoPKModel
	ConnectionId                            uint64 `gorm:"primaryKey" json:"connectionId"`
	ScopeId                                 string `gorm:"primaryKey;type:varchar(255)" json:"scopeId"`
	OrganizationId                          string `gorm:"primaryKey;type:varchar(255)" json:"organizationId"`
	Date                                    string `gorm:"primaryKey;type:varchar(32)" json:"date"`
	SkillName                               string `gorm:"primaryKey;type:varchar(255)" json:"skillName"`
	SkillDisplayName                        string `gorm:"type:varchar(255)" json:"skillDisplayName"`
	DistinctUserCount                       int64  `json:"distinctUserCount"`
	InvocationCount                         int64  `json:"invocationCount"`
	EnableCount                             int64  `json:"enableCount"`
	AttributedListPrice                     string `gorm:"type:varchar(128)" json:"attributedListPrice"`
	EstimatedOverageSpend                   string `gorm:"type:varchar(128)" json:"estimatedOverageSpend"`
	Currency                                string `gorm:"type:varchar(16)" json:"currency"`
	ShareStatus                             string `gorm:"type:varchar(32)" json:"shareStatus"`
	ChatDistinctConversationSkillUsedCount  int64  `json:"chatDistinctConversationSkillUsedCount"`
	ClaudeCodeDistinctSessionSkillUsedCount int64  `json:"claudeCodeDistinctSessionSkillUsedCount"`
	CoworkDistinctSessionSkillUsedCount     int64  `json:"coworkDistinctSessionSkillUsedCount"`
	RawJson                                 string `gorm:"type:longtext" json:"rawJson"`
}

func (claudeSkill20260715) TableName() string {
	return "_tool_claude_enterprise_skills"
}

type claudeConnector20260715 struct {
	archived.NoPKModel
	ConnectionId                                uint64 `gorm:"primaryKey" json:"connectionId"`
	ScopeId                                     string `gorm:"primaryKey;type:varchar(255)" json:"scopeId"`
	OrganizationId                              string `gorm:"primaryKey;type:varchar(255)" json:"organizationId"`
	Date                                        string `gorm:"primaryKey;type:varchar(32)" json:"date"`
	ConnectorName                               string `gorm:"primaryKey;type:varchar(255)" json:"connectorName"`
	DistinctUserCount                           int64  `json:"distinctUserCount"`
	ReadCallCount                               int64  `json:"readCallCount"`
	WriteCallCount                              int64  `json:"writeCallCount"`
	UnclassifiedCallCount                       int64  `json:"unclassifiedCallCount"`
	ChatDistinctConversationConnectorUsedCount  int64  `json:"chatDistinctConversationConnectorUsedCount"`
	ClaudeCodeDistinctSessionConnectorUsedCount int64  `json:"claudeCodeDistinctSessionConnectorUsedCount"`
	CoworkDistinctSessionConnectorUsedCount     int64  `json:"coworkDistinctSessionConnectorUsedCount"`
	RawJson                                     string `gorm:"type:longtext" json:"rawJson"`
}

func (claudeConnector20260715) TableName() string {
	return "_tool_claude_enterprise_connectors"
}

type claudeChatProject20260715 struct {
	archived.NoPKModel
	ConnectionId              uint64 `gorm:"primaryKey" json:"connectionId"`
	ScopeId                   string `gorm:"primaryKey;type:varchar(255)" json:"scopeId"`
	OrganizationId            string `gorm:"primaryKey;type:varchar(255)" json:"organizationId"`
	Date                      string `gorm:"primaryKey;type:varchar(32)" json:"date"`
	ProjectId                 string `gorm:"primaryKey;type:varchar(255)" json:"projectId"`
	ProjectName               string `gorm:"type:varchar(255)" json:"projectName"`
	CreatedAt                 string `gorm:"type:varchar(64)" json:"createdAt"`
	CreatorUserId             string `gorm:"type:varchar(255)" json:"creatorUserId"`
	CreatorEmail              string `gorm:"type:varchar(255)" json:"creatorEmail"`
	DistinctUserCount         int64  `json:"distinctUserCount"`
	DistinctConversationCount int64  `json:"distinctConversationCount"`
	MessageCount              int64  `json:"messageCount"`
	RawJson                   string `gorm:"type:longtext" json:"rawJson"`
}

func (claudeChatProject20260715) TableName() string {
	return "_tool_claude_enterprise_chat_projects"
}

type claudePluginAdoption20260715 struct {
	archived.NoPKModel
	ConnectionId                             uint64 `gorm:"primaryKey" json:"connectionId"`
	ScopeId                                  string `gorm:"primaryKey;type:varchar(255)" json:"scopeId"`
	OrganizationId                           string `gorm:"primaryKey;type:varchar(255)" json:"organizationId"`
	Date                                     string `gorm:"primaryKey;type:varchar(32)" json:"date"`
	PluginId                                 string `gorm:"type:varchar(255)" json:"pluginId"`
	PluginName                               string `gorm:"primaryKey;type:varchar(255)" json:"pluginName"`
	DistinctUserCount                        int64  `json:"distinctUserCount"`
	InstallCount                             int64  `json:"installCount"`
	InvocationCount                          int64  `json:"invocationCount"`
	ClaudeCodeDistinctSessionPluginUsedCount int64  `json:"claudeCodeDistinctSessionPluginUsedCount"`
	CoworkDistinctSessionPluginUsedCount     int64  `json:"coworkDistinctSessionPluginUsedCount"`
	RawJson                                  string `gorm:"type:longtext" json:"rawJson"`
}

func (claudePluginAdoption20260715) TableName() string {
	return "_tool_claude_enterprise_plugins"
}

type claudeArtifact20260715 struct {
	archived.NoPKModel
	ConnectionId                   uint64 `gorm:"primaryKey" json:"connectionId"`
	ScopeId                        string `gorm:"primaryKey;type:varchar(255)" json:"scopeId"`
	OrganizationId                 string `gorm:"primaryKey;type:varchar(255)" json:"organizationId"`
	Date                           string `gorm:"primaryKey;type:varchar(32)" json:"date"`
	ArtifactType                   string `gorm:"primaryKey;type:varchar(100)" json:"artifactType"`
	IsShared                       bool   `gorm:"primaryKey" json:"isShared"`
	ArtifactsCreatedCount          int64  `json:"artifactsCreatedCount"`
	PublishedArtifactsCreatedCount int64  `json:"publishedArtifactsCreatedCount"`
	DistinctUserCount              int64  `json:"distinctUserCount"`
	RawJson                        string `gorm:"type:longtext" json:"rawJson"`
}

func (claudeArtifact20260715) TableName() string {
	return "_tool_claude_enterprise_artifacts"
}
