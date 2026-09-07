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

package access

import (
	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
)

// LoadDatabaseOIDCProviders returns providers, whether database configuration is
// authoritative, and an error. An empty provider list with databaseSource=false means
// no database source is active. An empty list with databaseSource=true means the active
// source has no enabled provider and callers must fail closed. It also returns
// databaseSource=false before this migration exists, because auth starts before
// migrations and must preserve the environment bootstrap on that boot.
func LoadDatabaseOIDCProviders(db dal.Dal) ([]OIDCProvider, bool, errors.Error) {
	if !db.HasTable((OIDCProviderConfiguration{}).TableName()) {
		return nil, false, nil
	}
	configuration := &OIDCProviderConfiguration{}
	if err := db.First(configuration, dal.Where("id = ?", OIDCProviderSourceKey)); err != nil {
		if db.IsErrorNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if configuration.ActivatedAt == nil {
		return nil, false, nil
	}
	providers := make([]OIDCProvider, 0)
	if err := db.All(&providers,
		dal.Where("enabled = ? AND retired_at IS NULL", true),
		dal.Orderby("provider_key ASC"),
	); err != nil {
		return nil, true, err
	}
	return providers, true, nil
}
