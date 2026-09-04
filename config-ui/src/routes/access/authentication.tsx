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

import { useMemo, useState } from 'react';
import { PlusOutlined } from '@ant-design/icons';
import { Alert, Button, Space, Table, Tooltip, message } from 'antd';

import API from '@/api';
import type { OIDCCallbacks, OIDCProvider } from '@/api/access';
import { Message } from '@/components';
import { operator } from '@/utils';

import { OIDC_PROVIDER_MESSAGE } from './constants';
import { SectionHeader, SectionTitle } from './styled';
import { getAuthenticationState, getOIDCProviderError } from './utils';
import { type ActiveOperation, type Operation, getAuthenticationColumns } from './authentication-columns';
import { AuthenticationEditor } from './authentication-editor';

type Props = {
  callbacks?: OIDCCallbacks;
  providers: OIDCProvider[];
  loadFailed: boolean;
  onRefresh: () => void;
};

export const Authentication = ({ callbacks, providers, loadFailed, onRefresh }: Props) => {
  const [editorOpen, setEditorOpen] = useState(false);
  const [selectedProvider, setSelectedProvider] = useState<OIDCProvider>();
  const [operating, setOperating] = useState<ActiveOperation>();
  const [pageError, setPageError] = useState<string>();
  const [pageWarning, setPageWarning] = useState<string>();

  const isOperating = Boolean(operating);
  const authenticationState = getAuthenticationState(providers);
  const isActionOperating = (action: Operation, providerKey?: string) =>
    operating?.action === action && operating.providerKey === providerKey;

  const openEditor = (provider?: OIDCProvider) => {
    setSelectedProvider(provider);
    setPageError(undefined);
    setPageWarning(undefined);
    setEditorOpen(true);
  };

  const closeEditor = () => {
    if (!isOperating) setEditorOpen(false);
  };

  const performAction = async (action: Exclude<Operation, 'validate' | 'save'>, provider: OIDCProvider) => {
    setPageError(undefined);
    setPageWarning(undefined);
    const requests = {
      activate: () => API.access.activateOIDCProvider(provider.providerKey),
      enable: () => API.access.enableOIDCProvider(provider.providerKey),
      disable: () => API.access.disableOIDCProvider(provider.providerKey),
      retire: () => API.access.retireOIDCProvider(provider.providerKey),
      'grafana-sync': () => API.access.retryGrafanaOIDCProviderSync(provider.providerKey),
      'select-generic': () => API.access.selectGenericOIDCProvider(provider.providerKey),
    } as const;

    const [success, result] = await operator(requests[action], {
      hideToast: true,
      setOperating: (active) => setOperating(active ? { action, providerKey: provider.providerKey } : undefined),
      formatReason: getOIDCProviderError,
    });
    if (!success) {
      setPageError(getOIDCProviderError(result));
      return;
    }
    if (action === 'grafana-sync') {
      message.success(OIDC_PROVIDER_MESSAGE.GRAFANA_SYNCHRONIZED);
    } else {
      message.success('OIDC provider updated.');
    }
    onRefresh();
  };

  const enabledProviderCount = useMemo(() => providers.filter((p) => p.enabled).length, [providers]);

  const columns = useMemo(
    () =>
      getAuthenticationColumns({
        onEdit: openEditor,
        onAction: performAction,
        isOperating,
        isActionOperating,
        enabledProviderCount,
      }),
    [isOperating, operating, enabledProviderCount],
  );

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
        <Space size="small">
          <Tooltip title="Review the OIDC configuration managed in User Management.">
            <Button onClick={() => openEditor(providers[0])}>{authenticationState}</Button>
          </Tooltip>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => openEditor()}>
            Add provider
          </Button>
        </Space>
      </SectionHeader>
      <Message content="Grafana access remains independently managed. Providers marked DevLake only use Grafana's ordinary login." />
      {pageError && <Alert type="error" showIcon message={pageError} style={{ marginTop: 16 }} />}
      {pageWarning && (
        <Alert
          type="warning"
          showIcon
          closable
          onClose={() => setPageWarning(undefined)}
          message={pageWarning}
          style={{ marginTop: 16 }}
        />
      )}
      <Table
        rowKey="providerKey"
        size="middle"
        dataSource={providers}
        pagination={false}
        columns={columns}
        style={{ marginTop: 16 }}
      />

      <AuthenticationEditor
        open={editorOpen}
        provider={selectedProvider}
        callbacks={callbacks}
        onClose={closeEditor}
        onSaved={() => {
          setEditorOpen(false);
          onRefresh();
        }}
      />
    </>
  );
};
