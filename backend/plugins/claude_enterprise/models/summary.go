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

// ClaudeEnterpriseSummary stores one daily adoption summary from the Claude
// Enterprise Analytics summaries endpoint. It intentionally remains tool-layer
// only because DevLake has no compatible domain model for organization seats
// and active-user adoption metrics.
type ClaudeEnterpriseSummary struct {
	common.NoPKModel

	ConnectionId   uint64 `gorm:"primaryKey" json:"connectionId"`
	ScopeId        string `gorm:"primaryKey;type:varchar(128)" json:"scopeId"`
	OrganizationId string `gorm:"primaryKey;type:varchar(128)" json:"organizationId"`
	Date           string `gorm:"primaryKey;type:varchar(32)" json:"date"`
	Grain          string `json:"grain" gorm:"type:varchar(32)"`
	StartingAt     string `json:"startingAt" gorm:"type:varchar(64)"`
	EndingAt       string `json:"endingAt" gorm:"type:varchar(64)"`

	AssignedSeatCount                int     `json:"assignedSeatCount"`
	PendingInviteCount               int     `json:"pendingInviteCount"`
	DailyActiveUserCount             int     `json:"dailyActiveUserCount"`
	WeeklyActiveUserCount            int     `json:"weeklyActiveUserCount"`
	MonthlyActiveUserCount           int     `json:"monthlyActiveUserCount"`
	DailyAdoptionRate                float64 `json:"dailyAdoptionRate"`
	WeeklyAdoptionRate               float64 `json:"weeklyAdoptionRate"`
	MonthlyAdoptionRate              float64 `json:"monthlyAdoptionRate"`
	ChatDailyActiveUserCount         int     `json:"chatDailyActiveUserCount"`
	ChatWeeklyActiveUserCount        int     `json:"chatWeeklyActiveUserCount"`
	ChatMonthlyActiveUserCount       int     `json:"chatMonthlyActiveUserCount"`
	ClaudeCodeDailyActiveUserCount   int     `json:"claudeCodeDailyActiveUserCount"`
	ClaudeCodeWeeklyActiveUserCount  int     `json:"claudeCodeWeeklyActiveUserCount"`
	ClaudeCodeMonthlyActiveUserCount int     `json:"claudeCodeMonthlyActiveUserCount"`
	CoworkDailyActiveUserCount       int     `json:"coworkDailyActiveUserCount"`
	CoworkWeeklyActiveUserCount      int     `json:"coworkWeeklyActiveUserCount"`
	CoworkMonthlyActiveUserCount     int     `json:"coworkMonthlyActiveUserCount"`

	RawJson string `json:"rawJson" gorm:"type:longtext"`
}

func (ClaudeEnterpriseSummary) TableName() string {
	return "_tool_claude_enterprise_summaries"
}
