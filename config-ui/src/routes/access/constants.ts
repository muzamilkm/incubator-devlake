/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

import { ACCESS_ROLE, GRAFANA_PROVIDER_KIND, type AccessRole, type GrafanaProviderKind } from '@/api/access';

export const PATH_PREFIX = import.meta.env.DEVLAKE_PATH_PREFIX ?? '';
export const ACCESS_PATH = `${PATH_PREFIX}/access`;
export const BREADCRUMBS = [{ name: 'User Management', path: ACCESS_PATH }];
export const PAGE_DESCRIPTION =
  'Manage who can access DevLake. Grafana access remains independently managed in Grafana.';
export const PAGE_SIZE_OPTIONS: Array<10 | 25 | 50> = [10, 25, 50];
export const DEFAULT_PAGE_SIZE = PAGE_SIZE_OPTIONS[0];

export const ACCESS_STATUS_COLOR = {
  active: 'green',
  disabled: 'default',
} as const;

export const OIDC_PROVIDER_STATUS = {
  CONFIGURED: 'Configured',
  ACTIVE: 'Active',
  DISABLED: 'Disabled',
  DEVLAKE_ONLY: 'DevLake only',
  PENDING: 'Changes awaiting activation',
  FAILED: 'Grafana synchronization failed',
  COMPENSATED: 'Activation failed; Grafana restored',
  RECOVERY: 'Grafana recovery required',
  RETIRED: 'Retired',
} as const;

export const AUTHENTICATION_STATE = {
  NO_MANAGED_OIDC: 'No OIDC provider managed here',
  ACTIVATION_REQUIRED: 'OIDC activation required',
  NO_ACTIVE_OIDC: 'No active OIDC provider',
  OIDC_ACTIVE: 'OIDC sign-in active',
} as const;

export const OIDC_PROVIDER_MESSAGE = {
  GRAFANA_SYNCHRONIZED: 'Grafana OAuth configuration synchronized.',
  CALLBACK_DESCRIPTION: 'Register this exact callback URL with the customer OIDC provider.',
  SECRET_REPLACEMENT_REQUIRED: 'Required only when changing the client ID or rotating the secret.',
  VALIDATED: 'OIDC provider settings are valid.',
  GRAFANA_SYNC_FAILED:
    'OIDC provider was saved, but Grafana OAuth synchronization failed. Use Retry Grafana to complete synchronization.',
  RECOVERY_REQUIRED:
    'Grafana OAuth was disabled because the new configuration could not be safely rolled back. Retry synchronization after resolving the deployment issue.',
  ACTIVATION_COMPENSATED:
    'DevLake activation did not complete. Grafana was restored to its previous configuration; resolve the issue and activate again.',
} as const;

export const OIDC_PROVIDER_STATUS_COLOR: Record<string, string> = {
  [OIDC_PROVIDER_STATUS.ACTIVE]: 'green',
  [OIDC_PROVIDER_STATUS.DEVLAKE_ONLY]: 'blue',
  [OIDC_PROVIDER_STATUS.DISABLED]: 'default',
  [OIDC_PROVIDER_STATUS.FAILED]: 'red',
  [OIDC_PROVIDER_STATUS.COMPENSATED]: 'orange',
  [OIDC_PROVIDER_STATUS.RECOVERY]: 'red',
  [OIDC_PROVIDER_STATUS.CONFIGURED]: 'orange',
  [OIDC_PROVIDER_STATUS.PENDING]: 'orange',
};

export const GRAFANA_PROVIDER_OPTIONS: Array<{ value: GrafanaProviderKind; label: string }> = [
  { value: GRAFANA_PROVIDER_KIND.GOOGLE, label: 'Google' },
  { value: GRAFANA_PROVIDER_KIND.AZURE_AD, label: 'Microsoft Entra ID' },
  { value: GRAFANA_PROVIDER_KIND.OKTA, label: 'Okta' },
  { value: GRAFANA_PROVIDER_KIND.GITLAB, label: 'GitLab' },
  { value: GRAFANA_PROVIDER_KIND.GENERIC_OAUTH, label: 'Generic OAuth' },
  { value: GRAFANA_PROVIDER_KIND.NONE, label: 'DevLake only' },
];

export const GRAFANA_PROVIDER_LABEL: Record<GrafanaProviderKind, string> = Object.fromEntries(
  GRAFANA_PROVIDER_OPTIONS.map((option) => [option.value, option.label]),
) as Record<GrafanaProviderKind, string>;

export const ROLE_OPTIONS: Array<{ value: AccessRole; label: string }> = [
  { value: ACCESS_ROLE.MEMBER, label: 'Member' },
  { value: ACCESS_ROLE.CUSTOMER_ADMIN, label: 'Customer administrator' },
];
