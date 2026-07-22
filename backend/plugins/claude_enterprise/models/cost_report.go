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

// ClaudeEnterpriseCostReport stores one per-user cost row from the Claude
// Enterprise Analytics user_cost_report endpoint. Monetary fields are stored
// as strings to avoid decimal precision loss before live-key validation and
// dashboard-specific formatting decisions.
type ClaudeEnterpriseCostReport struct {
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
	CostType        string `gorm:"type:varchar(100)" json:"costType"`
	TokenType       string `gorm:"type:varchar(100)" json:"tokenType"`
	Currency        string `gorm:"type:varchar(16)" json:"currency"`
	DataRefreshedAt string `gorm:"type:varchar(64)" json:"dataRefreshedAt"`

	Amount       string `gorm:"type:varchar(128)" json:"amount"`
	ListAmount   string `gorm:"type:varchar(128)" json:"listAmount"`
	RequestCount *int64 `json:"requestCount"`

	RawJson string `gorm:"type:longtext" json:"rawJson"`
}

func (ClaudeEnterpriseCostReport) TableName() string {
	return "_tool_claude_enterprise_cost_reports"
}
