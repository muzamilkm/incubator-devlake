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

import { Button, Popconfirm, Space, Tag, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';

import { GRAFANA_PROVIDER_KIND, OIDC_PROVIDER_SYNC_STATUS, type OIDCProvider } from '@/api/access';
import { GRAFANA_PROVIDER_LABEL, OIDC_PROVIDER_STATUS_COLOR } from './constants';
import { canActivateOIDCProvider, canSelectGenericOIDCProvider, getOIDCProviderStatus } from './utils';

export type Operation =
  | 'validate'
  | 'save'
  | 'activate'
  | 'enable'
  | 'disable'
  | 'retire'
  | 'grafana-sync'
  | 'select-generic';

export type ActiveOperation = {
  action: Operation;
  providerKey?: string;
};

type GetAuthenticationColumnsProps = {
  onEdit: (provider: OIDCProvider) => void;
  onAction: (action: Exclude<Operation, 'validate' | 'save'>, provider: OIDCProvider) => void;
  isOperating: boolean;
  isActionOperating: (action: Operation, providerKey?: string) => boolean;
  enabledProviderCount?: number;
};

const providerActionMessage = (action: Operation, provider: OIDCProvider) => {
  if (action === 'disable') {
    return `DevLake sign-in through ${provider.displayName} will stop. Grafana access remains independently managed.`;
  }
  if (action === 'retire') {
    return `${provider.displayName} will no longer be available for DevLake sign-in. This does not delete Grafana users.`;
  }
  if (action === 'select-generic') {
    return `Grafana Generic OAuth will start using ${provider.displayName}. This changes the /login/generic_oauth route.`;
  }
  return `Activate ${provider.displayName} for DevLake sign-in.`;
};

export const getAuthenticationColumns = ({
  onEdit,
  onAction,
  isOperating,
  isActionOperating,
  enabledProviderCount,
}: GetAuthenticationColumnsProps): ColumnsType<OIDCProvider> => [
  {
    title: 'Provider',
    key: 'provider',
    render: (_: unknown, provider) => (
      <Space direction="vertical" size={0}>
        <Button type="link" style={{ padding: 0 }} onClick={() => onEdit(provider)}>
          {provider.displayName}
        </Button>
        <span>{provider.providerKey}</span>
      </Space>
    ),
  },
  {
    title: 'DevLake',
    key: 'devlake',
    render: (_: unknown, provider) => {
      const status = getOIDCProviderStatus(provider);
      return <Tag color={OIDC_PROVIDER_STATUS_COLOR[status] ?? 'default'}>{status}</Tag>;
    },
  },
  {
    title: 'Grafana',
    key: 'grafana',
    render: (_: unknown, provider) => (
      <Space direction="vertical" size={0}>
        <span>{GRAFANA_PROVIDER_LABEL[provider.grafanaTarget]}</span>
        {provider.grafanaTarget === GRAFANA_PROVIDER_KIND.GENERIC_OAUTH && (
          <Tag color="blue">Selected Generic OAuth</Tag>
        )}
        {provider.grafanaTarget === GRAFANA_PROVIDER_KIND.NONE && <span>Uses Grafana's own login</span>}
      </Space>
    ),
  },
  {
    title: 'Actions',
    key: 'actions',
    render: (_: unknown, provider) => (
      <Space wrap size="small">
        <Button size="small" onClick={() => onEdit(provider)}>
          Edit
        </Button>
        {canActivateOIDCProvider(provider) && (
          <Popconfirm
            title="Activate OIDC provider?"
            description={providerActionMessage('activate', provider)}
            okText="Activate"
            onConfirm={() => onAction('activate', provider)}
          >
            <Button
              size="small"
              type="primary"
              loading={isActionOperating('activate', provider.providerKey)}
              disabled={isOperating}
            >
              Activate
            </Button>
          </Popconfirm>
        )}
        {provider.enabled && !provider.hasCandidate && (
          enabledProviderCount !== undefined && enabledProviderCount <= 1 ? (
            <Tooltip title="At least one OIDC provider must remain enabled.">
              <span>
                <Button size="small" disabled>
                  Disable
                </Button>
              </span>
            </Tooltip>
          ) : (
            <Popconfirm
              title="Disable OIDC provider?"
              description={providerActionMessage('disable', provider)}
              okText="Disable"
              onConfirm={() => onAction('disable', provider)}
            >
              <Button size="small" loading={isActionOperating('disable', provider.providerKey)} disabled={isOperating}>
                Disable
              </Button>
            </Popconfirm>
          )
        )}
        {!provider.enabled && !provider.hasCandidate && (
          <Button
            size="small"
            loading={isActionOperating('enable', provider.providerKey)}
            disabled={isOperating}
            onClick={() => onAction('enable', provider)}
          >
            Enable
          </Button>
        )}
        {canSelectGenericOIDCProvider(provider) && (
          <Popconfirm
            title="Select for Grafana Generic OAuth?"
            description={providerActionMessage('select-generic', provider)}
            okText="Switch Grafana"
            onConfirm={() => onAction('select-generic', provider)}
          >
            <Button
              size="small"
              loading={isActionOperating('select-generic', provider.providerKey)}
              disabled={isOperating}
            >
              Use in Grafana
            </Button>
          </Popconfirm>
        )}
        {(provider.grafanaSyncStatus === OIDC_PROVIDER_SYNC_STATUS.FAILED ||
          provider.grafanaSyncStatus === OIDC_PROVIDER_SYNC_STATUS.COMPENSATION_FAILED) && (
          <Button
            size="small"
            loading={isActionOperating('grafana-sync', provider.providerKey)}
            disabled={isOperating}
            onClick={() => onAction('grafana-sync', provider)}
          >
            Retry Grafana
          </Button>
        )}
        {!provider.enabled && !provider.hasCandidate && (
          <Popconfirm
            title="Retire OIDC provider?"
            description={providerActionMessage('retire', provider)}
            okText="Retire"
            onConfirm={() => onAction('retire', provider)}
          >
            <Button
              size="small"
              danger
              loading={isActionOperating('retire', provider.providerKey)}
              disabled={isOperating}
            >
              Retire
            </Button>
          </Popconfirm>
        )}
      </Space>
    ),
  },
];
