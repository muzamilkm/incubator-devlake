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
import { CopyOutlined } from '@ant-design/icons';
import { Alert, Button, Checkbox, Input, Modal, Select, Space, Tooltip, message } from 'antd';
import { CopyToClipboard } from 'react-copy-to-clipboard';

import API from '@/api';
import {
  ACCESS_ERROR_CODE,
  GRAFANA_PROVIDER_KIND,
  OIDC_PROVIDER_SYNC_STATUS,
  type OIDCCallbacks,
  type OIDCProvider,
  type OIDCProviderInput,
} from '@/api/access';
import { Block } from '@/components';
import { operator } from '@/utils';

import { GRAFANA_PROVIDER_OPTIONS, OIDC_PROVIDER_MESSAGE } from './constants';
import {
  formFromOIDCProvider,
  getOIDCProviderError,
  getOIDCProviderErrorCode,
  isValidOIDCProviderInput,
  normalizeOIDCProviderInput,
} from './utils';

type Props = {
  open: boolean;
  provider?: OIDCProvider;
  callbacks?: OIDCCallbacks;
  onClose: () => void;
  onSaved: () => void;
  onGrafanaSyncFailed: (message: string) => void;
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

export const AuthenticationEditor = ({ open, provider, callbacks, onClose, onSaved, onGrafanaSyncFailed }: Props) => {
  const [form, setForm] = useState<OIDCProviderInput>(() => formFromOIDCProvider(provider));
  const [validating, setValidating] = useState(false);
  const [saving, setSaving] = useState(false);
  const [operationError, setOperationError] = useState<string>();
  const [operationSuccess, setOperationSuccess] = useState<string>();

  const isOperating = validating || saving;
  const normalizedInput = useMemo(() => normalizeOIDCProviderInput(form), [form]);
  const validInput = isValidOIDCProviderInput(form, provider, provider?.allowLocalOidc ?? callbacks?.allowLocalOidc);
  const requiresReplacementSecret = !provider?.secretConfigured || form.clientId.trim() !== provider.clientId;

  useEffect(() => {
    if (open) {
      setForm(formFromOIDCProvider(provider));
      setOperationError(undefined);
      setOperationSuccess(undefined);
    }
  }, [open, provider]);

  const updateField = <Key extends keyof OIDCProviderInput>(field: Key, value: OIDCProviderInput[Key]) => {
    setForm((current) => ({ ...current, [field]: value }));
    setOperationError(undefined);
    setOperationSuccess(undefined);
  };

  const handleValidate = async () => {
    setOperationError(undefined);
    setOperationSuccess(undefined);
    const [success, result] = await operator(() => API.access.validateOIDCProvider(normalizedInput), {
      hideToast: true,
      setOperating: setValidating,
      formatReason: getOIDCProviderError,
    });
    if (!success) {
      setOperationError(getOIDCProviderError(result));
      return;
    }
    setOperationSuccess(OIDC_PROVIDER_MESSAGE.VALIDATED);
  };

  const handleSave = async () => {
    setOperationError(undefined);
    setOperationSuccess(undefined);
    const [success, result] = await operator(() => API.access.saveOIDCProvider(normalizedInput), {
      hideToast: true,
      setOperating: setSaving,
      formatReason: getOIDCProviderError,
    });
    if (!success) {
      const code = getOIDCProviderErrorCode(result);
      if (code === ACCESS_ERROR_CODE.GRAFANA_SYNC_FAILED) {
        onGrafanaSyncFailed(OIDC_PROVIDER_MESSAGE.GRAFANA_SYNC_FAILED);
        return;
      }
      setOperationError(getOIDCProviderError(result));
      return;
    }
    message.success('OIDC provider saved.');
    onSaved();
  };

  return (
    <Modal
      title={provider ? `Edit ${provider.displayName}` : 'Add OIDC provider'}
      open={open}
      onCancel={() => {
        if (!isOperating) onClose();
      }}
      footer={
        <Space>
          <Button onClick={onClose} disabled={isOperating}>
            Cancel
          </Button>
          <Button loading={validating} disabled={!validInput || isOperating} onClick={handleValidate}>
            Validate
          </Button>
          <Button type="primary" loading={saving} disabled={!validInput || isOperating} onClick={handleSave}>
            Save provider
          </Button>
        </Space>
      }
      destroyOnClose
      width={720}
    >
      <Space direction="vertical" size={16} style={{ display: 'flex' }}>
        <Callback
          label="DevLake callback URL"
          value={provider?.devlakeCallbackUrl ?? callbacks?.devlakeCallbackUrl ?? ''}
        />
        <Callback
          label="Grafana callback URL"
          value={provider?.grafanaCallbackUrl ?? callbacks?.grafanaCallbackUrls[form.grafanaTarget] ?? ''}
        />
        <Block
          title="Provider key"
          description="Use a stable lowercase identifier. It cannot change after creation."
          required
        >
          <Input
            value={form.providerKey}
            disabled={isOperating || Boolean(provider)}
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
            disabled={isOperating || Boolean(provider)}
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
          description={provider?.secretConfigured ? OIDC_PROVIDER_MESSAGE.SECRET_REPLACEMENT_REQUIRED : undefined}
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
            disabled={isOperating || Boolean(provider)}
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
        {provider?.hasCandidate && (
          <Alert
            type="info"
            showIcon
            message="A staged revision is awaiting activation. The active provider remains in use until then."
          />
        )}
        {provider?.grafanaSyncStatus === OIDC_PROVIDER_SYNC_STATUS.COMPENSATION_FAILED && (
          <Alert type="warning" showIcon message={OIDC_PROVIDER_MESSAGE.RECOVERY_REQUIRED} />
        )}
        {operationError && <Alert type="error" showIcon message={operationError} />}
        {operationSuccess && <Alert type="success" showIcon message={operationSuccess} />}
      </Space>
    </Modal>
  );
};
