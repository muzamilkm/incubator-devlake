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

// ClaudeEnterpriseAnalyticsRecord stores one item from a Claude Enterprise
// Analytics API endpoint. The raw endpoint item is preserved so future
// live-key validation can refine typed mappings without changing raw replay.
type ClaudeEnterpriseAnalyticsRecord struct {
	common.NoPKModel

	ConnectionId   uint64 `gorm:"primaryKey" json:"connectionId"`
	ScopeId        string `gorm:"primaryKey;type:varchar(128)" json:"scopeId"`
	OrganizationId string `gorm:"primaryKey;type:varchar(128)" json:"organizationId"`
	Endpoint       string `gorm:"primaryKey;type:varchar(100)" json:"endpoint"`
	RecordId       string `gorm:"primaryKey;type:varchar(64)" json:"recordId"`

	Date      string `json:"date" gorm:"type:varchar(32)"`
	Grain     string `json:"grain" gorm:"type:varchar(32)"`
	UserId    string `json:"userId" gorm:"type:varchar(255)"`
	UserEmail string `json:"userEmail" gorm:"type:varchar(255)"`
	Product   string `json:"product" gorm:"type:varchar(100)"`
	Model     string `json:"model" gorm:"type:varchar(255)"`
	RawJson   string `json:"rawJson" gorm:"type:longtext"`
}

func (ClaudeEnterpriseAnalyticsRecord) TableName() string {
	return "_tool_claude_enterprise_analytics_records"
}
