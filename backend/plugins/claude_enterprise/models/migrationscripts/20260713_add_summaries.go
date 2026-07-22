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

type addClaudeEnterpriseSummaries struct{}

func (script *addClaudeEnterpriseSummaries) Up(basicRes context.BasicRes) errors.Error {
	return migrationhelper.AutoMigrateTables(
		basicRes,
		&claudeSummary20260713{},
	)
}

func (script *addClaudeEnterpriseSummaries) Version() uint64 {
	return 20260713000001
}

func (script *addClaudeEnterpriseSummaries) Name() string {
	return "add Claude Enterprise summaries table"
}

type claudeSummary20260713 struct {
	archived.NoPKModel
	ConnectionId       uint64 `gorm:"primaryKey" json:"connectionId"`
	ScopeId            string `gorm:"primaryKey;type:varchar(255)" json:"scopeId"`
	OrganizationId     string `gorm:"primaryKey;type:varchar(255)" json:"organizationId"`
	Date               string `gorm:"primaryKey;type:varchar(32)" json:"date"`
	Grain              string `gorm:"type:varchar(32)" json:"grain"`
	StartingAt         string `gorm:"type:varchar(64)" json:"startingAt"`
	EndingAt           string `gorm:"type:varchar(64)" json:"endingAt"`
	AssignedSeats      int    `json:"assignedSeats"`
	PendingInvites     int    `json:"pendingInvites"`
	DailyActiveUsers   int    `json:"dailyActiveUsers"`
	WeeklyActiveUsers  int    `json:"weeklyActiveUsers"`
	MonthlyActiveUsers int    `json:"monthlyActiveUsers"`
	RawJson            string `gorm:"type:longtext" json:"rawJson"`
}

func (claudeSummary20260713) TableName() string {
	return "_tool_claude_enterprise_summaries"
}
