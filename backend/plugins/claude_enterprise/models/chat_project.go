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

package models

import "github.com/apache/incubator-devlake/core/models/common"

// ClaudeEnterpriseChatProject stores one daily adoption row for a Claude chat
// project from the Claude Enterprise Analytics
// `GET /v1/organizations/analytics/apps/chat/projects` endpoint (Section 4.2
// notes this endpoint's nested "apps/chat" path). It remains tool-layer only
// (Section 7); DevLake's ai_activities domain model has no project concept.
//
// The exact response shape is not documented in this repository. Field names
// below are a reasonable synthetic-fixture-driven shape consistent with the
// documented /users and /summaries endpoints; fields marked "Provisional"
// should be revisited once Phase 17 live-key validation confirms the actual
// response body.
type ClaudeEnterpriseChatProject struct {
	common.NoPKModel

	ConnectionId   uint64 `gorm:"primaryKey" json:"connectionId"`
	ScopeId        string `gorm:"primaryKey;type:varchar(255)" json:"scopeId"`
	OrganizationId string `gorm:"primaryKey;type:varchar(255)" json:"organizationId"`
	Date           string `gorm:"primaryKey;type:varchar(32)" json:"date"`
	ProjectId      string `gorm:"primaryKey;type:varchar(255)" json:"projectId"`

	ProjectName               string `json:"projectName" gorm:"type:varchar(255)"`
	CreatedAt                 string `json:"createdAt" gorm:"type:varchar(64)"`
	CreatorUserId             string `json:"creatorUserId" gorm:"type:varchar(255)"`
	CreatorEmail              string `json:"creatorEmail" gorm:"type:varchar(255)"`
	DistinctUserCount         int64  `json:"distinctUserCount"`
	DistinctConversationCount int64  `json:"distinctConversationCount"`
	MessageCount              int64  `json:"messageCount"`
	MembersCount              int    `gorm:"-" json:"-"`
	ConversationsCount        int64  `gorm:"-" json:"-"`

	RawJson string `json:"rawJson" gorm:"type:longtext"`
}

func (ClaudeEnterpriseChatProject) TableName() string {
	return "_tool_claude_enterprise_chat_projects"
}
