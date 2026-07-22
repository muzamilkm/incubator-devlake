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

package tasks

import (
	"encoding/json"

	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/plugin"
	"github.com/apache/incubator-devlake/plugins/claude_enterprise/models"
)

// ExtractArtifactsMeta extracts artifacts into both the raw-preserving
// _tool_claude_enterprise_analytics_records table and the typed
// _tool_claude_enterprise_artifacts table. No ai_activities converter is
// added (Section 7): DevLake's domain model has no artifact concept.
var ExtractArtifactsMeta = newExtractMeta(artifactsEndpoint, ExtractArtifacts)

func ExtractArtifacts(taskCtx plugin.SubTaskContext) errors.Error {
	return extractTypedAnalyticsEndpoint(taskCtx, artifactsEndpoint, func(raw []byte, params analyticsRawParams) (interface{}, errors.Error) {
		return BuildArtifactRecord(raw, params)
	})
}

// BuildArtifactRecord extracts a dashboard-ready daily artifact adoption row
// while keeping the full raw JSON available in the generic analytics record.
// See models.ClaudeEnterpriseArtifact for provisional-field documentation.
func BuildArtifactRecord(raw []byte, params analyticsRawParams) (*models.ClaudeEnterpriseArtifact, errors.Error) {
	var item map[string]interface{}
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, errors.Default.Wrap(err, "failed to parse Claude Enterprise artifact item")
	}

	date := firstNonEmpty(params.RequestDate, firstString(item, "date", "starting_date", "day"))
	if date == "" {
		return nil, errors.BadInput.New("Claude Enterprise artifact item is missing date")
	}
	artifactType := firstString(item, "artifact_type")
	if artifactType == "" {
		return nil, errors.BadInput.New("Claude Enterprise artifact item is missing artifact type")
	}

	return &models.ClaudeEnterpriseArtifact{
		ConnectionId:                   params.ConnectionId,
		ScopeId:                        params.ScopeId,
		OrganizationId:                 params.OrganizationId,
		Date:                           date,
		ArtifactType:                   artifactType,
		IsShared:                       firstBool(item, "is_shared"),
		ArtifactsCreatedCount:          firstInt64(item, "artifacts_created_count"),
		PublishedArtifactsCreatedCount: firstInt64(item, "published_artifacts_created_count"),
		DistinctUserCount:              firstInt64(item, "distinct_user_count"),
		ArtifactId:                     firstString(item, "artifact_id"),
		ArtifactTitle:                  firstString(item, "artifact_title"),
		CreatorUserId:                  firstString(item, "creator_id"),
		CreatorEmail:                   firstString(item, "creator_email"),
		ViewCount:                      firstInt64(item, "view_count"),
		ShareCount:                     firstInt64(item, "share_count"),
		RawJson:                        string(raw),
	}, nil
}
