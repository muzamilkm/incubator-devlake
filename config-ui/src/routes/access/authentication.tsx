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

import { useEffect, useMemo, useState } from 'react';
import { CopyOutlined, PlusOutlined } from '@ant-design/icons';
import { Alert, Button, Checkbox, Input, Modal, Popconfirm, Select, Space, Table, Tag, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { CopyToClipboard } from 'react-copy-to-clipboard';

import API from '@/api';
import {
  GRAFANA_PROVIDER_KIND,
  OIDC_PROVIDER_SYNC_STATUS,
  type OIDCProvider,
  type OIDCProviderInput,
} from '@/api/access';
import { Block, Message } from '@/components';
import { operator } from '@/utils';

import {
  GRAFANA_PROVIDER_LABEL,
  GRAFANA_PROVIDER_OPTIONS,
  OIDC_PROVIDER_MESSAGE,
  OIDC_PROVIDER_STATUS_COLOR,
} from './constants';
import { SectionHeader, SectionTitle } from './styled';
import {
  canActivateOIDCProvider,
  canSelectGenericOIDCProvider,
  formFromOIDCProvider,
  getOIDCProviderError,
  getOIDCProviderStatus,
  isValidOIDCProviderInput,
  normalizeOIDCProviderInput,
} from './utils';

type Props = {
  providers: OIDCProvider[];
  loadFailed: boolean;
  onRefresh: () => void;
};

type Operation = 'validate' | 'save' | 'activate' | 'enable' | 'disable' | 'retire' | 'grafana-sync' | 'select-generic';

type ActiveOperation = {
  action: Operation;
  providerKey?: string;
};

const Callback = ({ label, value }: { label: string; value: string }) => (
  <Block title={label} description={OIDC_PROVIDER_MESSAGE.CALLBACK_DESCRIPTION}>
    <Input
      readOnly
      value={value || 'Deployment public URL is not configured.'}
      addonAfter={
        value ? (
          <CopyToClipboard text={value} onCopy={() => message.success('Callback URL copied.')}>
            <Tooltip title={`Copy ${label}`}>
              <Button type="text" icon={<CopyOutlined />} aria-label={`Copy ${label}`} />
            </Tooltip>
          </CopyToClipboard>
        ) : undefined
      }
    />
  </Block>
);

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

export const Authentication = ({ providers, loadFailed, onRefresh }: Props) => {
  const [editorOpen, setEditorOpen] = useState(false);
  const [selectedProvider, setSelectedProvider] = useState<OIDCProvider>();
  const [form, setForm] = useState<OIDCProviderInput>(() => formFromOIDCProvider());
  const [operating, setOperating] = useState<ActiveOperation>();
  const [operationError, setOperationError] = useState<string>();
  const [operationSuccess, setOperationSuccess] = useState<string>();

  const isOperating = Boolean(operating);
  const normalizedInput = useMemo(() => normalizeOIDCProviderInput(form), [form]);
  const validInput = isValidOIDCProviderInput(form, selectedProvider);
  const requiresReplacementSecret =
    !selectedProvider?.secretConfigured || form.clientId.trim() !== selectedProvider.clientId;
  const isActionOperating = (action: Operation, providerKey?: string) =>
    operating?.action === action && operating.providerKey === providerKey;

  useEffect(() => {
    if (!selectedProvider) return;
    const refreshed = providers.find((provider) => provider.providerKey === selectedProvider.providerKey);
    if (refreshed) setSelectedProvider(refreshed);
  }, [providers, selectedProvider?.providerKey]);

  const openEditor = (provider?: OIDCProvider) => {
    setSelectedProvider(provider);
    setForm(formFromOIDCProvider(provider));
    setOperationError(undefined);
    setOperationSuccess(undefined);
    setEditorOpen(true);
  };

  const closeEditor = () => {
    if (!isOperating) setEditorOpen(false);
  };

  const updateField = <Key extends keyof OIDCProviderInput>(field: Key, value: OIDCProviderInput[Key]) => {
    setForm((current) => ({ ...current, [field]: value }));
    setOperationError(undefined);
    setOperationSuccess(undefined);
  };

  const execute = async (action: Operation, request: () => Promise<unknown>, providerKey?: string) => {
    setOperationError(undefined);
    setOperationSuccess(undefined);
    const [success, result] = await operator(request, {
      hideToast: true,
      setOperating: (active) => setOperating(active ? { action, providerKey } : undefined),
      formatReason: getOIDCProviderError,
    });
    if (!success) {
      setOperationError(getOIDCProviderError(result));
      return;
    }
    if (action === 'validate') {
      setOperationSuccess(OIDC_PROVIDER_MESSAGE.VALIDATED);
      return;
    }
    if (action === 'save') {
      setEditorOpen(false);
      message.success('OIDC provider saved.');
    } else if (action === 'grafana-sync') {
      message.success(OIDC_PROVIDER_MESSAGE.GRAFANA_SYNCHRONIZED);
    } else {
      message.success('OIDC provider updated.');
    }
    onRefresh();
  };

  const save = () => execute('save', () => API.access.saveOIDCProvider(normalizedInput));
  const validate = () => execute('validate', () => API.access.validateOIDCProvider(normalizedInput));
  const performAction = (action: Exclude<Operation, 'validate' | 'save'>, provider: OIDCProvider) => {
    const requests = {
      activate: () => API.access.activateOIDCProvider(provider.providerKey),
      enable: () => API.access.enableOIDCProvider(provider.providerKey),
      disable: () => API.access.disableOIDCProvider(provider.providerKey),
      retire: () => API.access.retireOIDCProvider(provider.providerKey),
      'grafana-sync': () => API.access.retryGrafanaOIDCProviderSync(provider.providerKey),
      'select-generic': () => API.access.selectGenericOIDCProvider(provider.providerKey),
    } as const;
    return execute(action, requests[action], provider.providerKey);
  };

  const columns: ColumnsType<OIDCProvider> = [
    {
      title: 'Provider',
      key: 'provider',
      render: (_: unknown, provider) => (
        <Space direction="vertical" size={0}>
          <Button type="link" style={{ padding: 0 }} onClick={() => openEditor(provider)}>
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
          <Button size="small" onClick={() => openEditor(provider)}>
            Edit
          </Button>
          {canActivateOIDCProvider(provider) && (
            <Popconfirm
              title="Activate OIDC provider?"
              description={providerActionMessage('activate', provider)}
              okText="Activate"
              onConfirm={() => performAction('activate', provider)}
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
            <Popconfirm
              title="Disable OIDC provider?"
              description={providerActionMessage('disable', provider)}
              okText="Disable"
              onConfirm={() => performAction('disable', provider)}
            >
              <Button size="small" loading={isActionOperating('disable', provider.providerKey)} disabled={isOperating}>
                Disable
              </Button>
            </Popconfirm>
          )}
          {!provider.enabled && !provider.hasCandidate && (
            <Button
              size="small"
              loading={isActionOperating('enable', provider.providerKey)}
              disabled={isOperating}
              onClick={() => performAction('enable', provider)}
            >
              Enable
            </Button>
          )}
          {canSelectGenericOIDCProvider(provider) && (
            <Popconfirm
              title="Select for Grafana Generic OAuth?"
              description={providerActionMessage('select-generic', provider)}
              okText="Switch Grafana"
              onConfirm={() => performAction('select-generic', provider)}
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
              onClick={() => performAction('grafana-sync', provider)}
            >
              Retry Grafana
            </Button>
          )}
          {!provider.enabled && !provider.hasCandidate && (
            <Popconfirm
              title="Retire OIDC provider?"
              description={providerActionMessage('retire', provider)}
              okText="Retire"
              onConfirm={() => performAction('retire', provider)}
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

  if (loadFailed) {
    return (
      <>
        <SectionHeader $spaced>
          <SectionTitle>Authentication</SectionTitle>
        </SectionHeader>
        <Alert
          type="error"
          showIcon
          message="Authentication settings could not be loaded. Refresh the page and try again."
        />
      </>
    );
  }

  return (
    <>
      <SectionHeader $spaced>
        <SectionTitle>Authentication</SectionTitle>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => openEditor()}>
          Add provider
        </Button>
      </SectionHeader>
      <Message content="Grafana access remains independently managed. Providers marked DevLake only use Grafana's ordinary login." />
      {operationError && <Alert type="error" showIcon message={operationError} style={{ marginTop: 16 }} />}
      <Table
        rowKey="providerKey"
        size="middle"
        dataSource={providers}
        pagination={false}
        columns={columns}
        style={{ marginTop: 16 }}
      />

      <Modal
        title={selectedProvider ? `Edit ${selectedProvider.displayName}` : 'Add OIDC provider'}
        open={editorOpen}
        onCancel={closeEditor}
        footer={
          <Space>
            <Button onClick={closeEditor} disabled={isOperating}>
              Cancel
            </Button>
            <Button loading={isActionOperating('validate')} disabled={!validInput || isOperating} onClick={validate}>
              Validate
            </Button>
            <Button
              type="primary"
              loading={isActionOperating('save')}
              disabled={!validInput || isOperating}
              onClick={save}
            >
              Save provider
            </Button>
          </Space>
        }
        destroyOnClose
        width={720}
      >
        <Space direction="vertical" size={16} style={{ display: 'flex' }}>
          <Callback label="DevLake callback URL" value={selectedProvider?.devlakeCallbackUrl ?? ''} />
          <Callback label="Grafana callback URL" value={selectedProvider?.grafanaCallbackUrl ?? ''} />
          <Block
            title="Provider key"
            description="Use a stable lowercase identifier. It cannot change after creation."
            required
          >
            <Input
              value={form.providerKey}
              disabled={isOperating || Boolean(selectedProvider)}
              placeholder="google-workspace"
              onChange={(event) => updateField('providerKey', event.target.value)}
            />
          </Block>
          <Block title="Display name" required>
            <Input
              value={form.displayName}
              disabled={isOperating}
              placeholder="Google Workspace"
              onChange={(event) => updateField('displayName', event.target.value)}
            />
          </Block>
          <Block title="Issuer URL" required>
            <Input
              value={form.issuerUrl}
              disabled={isOperating || Boolean(selectedProvider)}
              placeholder="https://accounts.google.com"
              onChange={(event) => updateField('issuerUrl', event.target.value)}
            />
          </Block>
          <Block title="Client ID" required>
            <Input
              value={form.clientId}
              disabled={isOperating}
              onChange={(event) => updateField('clientId', event.target.value)}
            />
          </Block>
          <Block
            title="Client secret"
            description={
              selectedProvider?.secretConfigured ? OIDC_PROVIDER_MESSAGE.SECRET_REPLACEMENT_REQUIRED : undefined
            }
            required={requiresReplacementSecret}
          >
            <Input.Password
              value={form.clientSecret}
              disabled={isOperating}
              onChange={(event) => updateField('clientSecret', event.target.value)}
            />
          </Block>
          <Block title="Scopes" description="The openid scope is required." required>
            <Input
              value={form.scopes}
              disabled={isOperating}
              onChange={(event) => updateField('scopes', event.target.value)}
            />
          </Block>
          <Block
            title="Grafana sign-in option"
            description="Only one DevLake provider can control each Grafana sign-in option."
            required
          >
            <Select
              value={form.grafanaTarget}
              disabled={isOperating || Boolean(selectedProvider)}
              options={GRAFANA_PROVIDER_OPTIONS}
              onChange={(value) => updateField('grafanaTarget', value)}
            />
          </Block>
          {form.grafanaTarget === GRAFANA_PROVIDER_KIND.NONE && (
            <Checkbox
              checked={form.confirmDevlakeOnly}
              disabled={isOperating}
              onChange={(event) => updateField('confirmDevlakeOnly', event.target.checked)}
            >
              I understand this provider is for DevLake only and opens Grafana's ordinary login.
            </Checkbox>
          )}
          {selectedProvider?.hasCandidate && (
            <Alert
              type="info"
              showIcon
              message="A staged revision is awaiting activation. The active provider remains in use until then."
            />
          )}
          {selectedProvider?.grafanaSyncStatus === OIDC_PROVIDER_SYNC_STATUS.COMPENSATION_FAILED && (
            <Alert type="warning" showIcon message={OIDC_PROVIDER_MESSAGE.RECOVERY_REQUIRED} />
          )}
          {operationError && <Alert type="error" showIcon message={operationError} />}
          {operationSuccess && <Alert type="success" showIcon message={operationSuccess} />}
        </Space>
      </Modal>
    </>
  );
};
