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

// ClaudeEnterpriseUsageReport stores one per-user token usage row from the
// Claude Enterprise Analytics user_usage_report endpoint. It remains
// tool-layer only until live validation proves which usage metrics can be
// safely normalized into a shared domain model.
type ClaudeEnterpriseUsageReport struct {
	common.NoPKModel

	ReportId        string `gorm:"primaryKey;type:varchar(64)" json:"reportId"`
	ConnectionId    uint64 `gorm:"primaryKey" json:"connectionId"`
	ScopeId         string `gorm:"primaryKey;type:varchar(128)" json:"scopeId"`
	OrganizationId  string `gorm:"primaryKey;type:varchar(128)" json:"organizationId"`
	StartingAt      string `gorm:"type:varchar(64)" json:"startingAt"`
	EndingAt        string `gorm:"type:varchar(64)" json:"endingAt"`
	UserId          string `gorm:"type:varchar(255)" json:"userId"`
	UserEmail       string `gorm:"type:varchar(255)" json:"userEmail"`
	DeletedActor    bool   `json:"deletedActor"`
	Product         string `gorm:"type:varchar(100)" json:"product"`
	Model           string `gorm:"type:varchar(255)" json:"model"`
	ContextWindow   string `gorm:"type:varchar(32)" json:"contextWindow"`
	InferenceGeo    string `gorm:"type:varchar(32)" json:"inferenceGeo"`
	Speed           string `gorm:"type:varchar(32)" json:"speed"`
	DataRefreshedAt string `gorm:"type:varchar(64)" json:"dataRefreshedAt"`

	UncachedInputTokens        int64 `json:"uncachedInputTokens"`
	OutputTokens               int64 `json:"outputTokens"`
	CacheReadInputTokens       int64 `json:"cacheReadInputTokens"`
	CacheCreation1hInputTokens int64 `json:"cacheCreation1hInputTokens" gorm:"column:cache_creation_1h_input_tokens"`
	CacheCreation5mInputTokens int64 `json:"cacheCreation5mInputTokens" gorm:"column:cache_creation_5m_input_tokens"`
	TotalTokens                int64 `json:"totalTokens"`
	RequestCount               int64 `json:"requestCount"`
	WebSearchRequests          int64 `json:"webSearchRequests"`

	RawJson string `gorm:"type:longtext" json:"rawJson"`
}

func (ClaudeEnterpriseUsageReport) TableName() string {
	return "_tool_claude_enterprise_usage_reports"
}
