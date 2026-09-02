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

import axios, { HttpStatusCode } from 'axios';

import {
  ACCESS_ERROR_CODE,
  GRAFANA_PROVIDER_KIND,
  OIDC_PROVIDER_SYNC_STATUS,
  type AccessApiErrorResponse,
  type OIDCProvider,
  type OIDCProviderInput,
} from '../../api/access';
import { OIDC_PROVIDER_STATUS } from './constants';

export const ACCESS_ERROR = {
  DUPLICATE_DOMAIN: 'This domain already has a DevLake access policy.',
  DUPLICATE_USER: 'This email already has a DevLake access entry.',
  INVALID_DOMAIN: 'Enter a valid email domain and role, then try again.',
  INVALID_USER: 'Enter a valid email and role, then try again.',
  REQUEST_FAILED: 'Unable to update access settings. Please try again.',
  INVALID_OIDC_PROVIDER: 'Enter valid OIDC provider settings and include the openid scope.',
  OIDC_PROVIDER_BLOCKED: 'OIDC provider settings cannot be applied until the deployment prerequisites are available.',
  OIDC_PROVIDER_FAILED: 'OIDC provider settings could not be completed. Please try again.',
  OIDC_PROVIDER_STALE: 'This provider changed. Refresh the page before saving it.',
  GRAFANA_TARGET_CONFLICT: 'Another provider already controls this Grafana sign-in option.',
} as const;

export const normalizeDomain = (value: string) => value.trim().toLowerCase();

export const isValidDomain = (value: string) => {
  const domain = normalizeDomain(value);
  return (
    domain.length > 0 &&
    !/[\s@]/.test(domain) &&
    !domain.startsWith('.') &&
    !domain.endsWith('.') &&
    !domain.includes('..') &&
    !(domain.startsWith('[') && domain.endsWith(']'))
  );
};

export const isValidEmail = (value: string) => {
  const email = value.trim();
  const at = email.indexOf('@');
  return at > 0 && at === email.lastIndexOf('@') && isValidDomain(email.slice(at + 1)) && !/\s/.test(email);
};

const extractErrorCode = (error: unknown): string | undefined => {
  if (!axios.isAxiosError<AccessApiErrorResponse>(error) || error.response?.status !== HttpStatusCode.BadRequest) {
    return undefined;
  }
  return typeof error.response.data?.code === 'string' ? error.response.data.code : undefined;
};

const extractOIDCProviderErrorCode = (error: unknown): string | undefined => {
  if (!axios.isAxiosError<AccessApiErrorResponse>(error)) return undefined;
  const response = error.response;
  const status = response?.status;
  if (status !== HttpStatusCode.BadRequest && status !== HttpStatusCode.ServiceUnavailable) return undefined;
  return typeof response?.data?.code === 'string' ? response.data.code : undefined;
};

const serverMessage = (error: unknown) => {
  if (!axios.isAxiosError<{ message?: unknown }>(error) || error.response?.status !== HttpStatusCode.BadRequest) {
    return '';
  }
  return typeof error.response.data?.message === 'string' ? error.response.data.message : '';
};

export const getCreateUserError = (error: unknown) => {
  const code = extractErrorCode(error);
  if (code === ACCESS_ERROR_CODE.DUPLICATE_USER) return ACCESS_ERROR.DUPLICATE_USER;
  if (code === ACCESS_ERROR_CODE.INVALID_USER) return ACCESS_ERROR.INVALID_USER;

  const message = serverMessage(error);
  if (message.includes('this email already has a DevLake access entry')) return ACCESS_ERROR.DUPLICATE_USER;
  return message ? ACCESS_ERROR.INVALID_USER : ACCESS_ERROR.REQUEST_FAILED;
};

export const getCreateDomainError = (error: unknown) => {
  const code = extractErrorCode(error);
  if (code === ACCESS_ERROR_CODE.DUPLICATE_DOMAIN) return ACCESS_ERROR.DUPLICATE_DOMAIN;
  if (code === ACCESS_ERROR_CODE.INVALID_DOMAIN) return ACCESS_ERROR.INVALID_DOMAIN;

  const message = serverMessage(error);
  if (message.includes('this domain already has a DevLake access policy')) return ACCESS_ERROR.DUPLICATE_DOMAIN;
  return message ? ACCESS_ERROR.INVALID_DOMAIN : ACCESS_ERROR.REQUEST_FAILED;
};

export const normalizeOIDCProviderInput = (provider: OIDCProviderInput): OIDCProviderInput => ({
  providerKey: provider.providerKey.trim().toLowerCase(),
  displayName: provider.displayName.trim(),
  issuerUrl: provider.issuerUrl.trim().replace(/\/+$/, ''),
  clientId: provider.clientId.trim(),
  clientSecret: provider.clientSecret.trim(),
  scopes: provider.scopes
    .split(/[\s,]+/)
    .filter(Boolean)
    .filter((scope, index, scopes) => scopes.indexOf(scope) === index)
    .join(' '),
  grafanaTarget: provider.grafanaTarget,
  confirmDevlakeOnly: provider.confirmDevlakeOnly,
  revision: provider.revision,
});

export const formFromOIDCProvider = (provider?: OIDCProvider): OIDCProviderInput => ({
  providerKey: provider?.providerKey ?? '',
  displayName: provider?.displayName ?? '',
  issuerUrl: provider?.issuerUrl ?? '',
  clientId: provider?.clientId ?? '',
  clientSecret: '',
  scopes: provider?.scopes ?? 'openid profile email',
  grafanaTarget: provider?.grafanaTarget ?? GRAFANA_PROVIDER_KIND.NONE,
  confirmDevlakeOnly: provider?.grafanaTarget === GRAFANA_PROVIDER_KIND.NONE,
  revision: provider?.providerRevision,
});

export const isValidOIDCProviderInput = (provider: OIDCProviderInput, configuredProvider?: OIDCProvider) => {
  const normalized = normalizeOIDCProviderInput(provider);
  let issuer: URL;
  try {
    issuer = new URL(normalized.issuerUrl);
  } catch {
    return false;
  }
  const isLocalHTTP =
    issuer.protocol === 'http:' && (issuer.hostname === 'localhost' || issuer.hostname === '127.0.0.1');
  const requiresReplacementSecret =
    !configuredProvider?.secretConfigured || normalized.clientId !== configuredProvider.clientId;
  return (
    /^[a-z0-9_-]{1,64}$/.test(normalized.providerKey) &&
    normalized.displayName.length > 0 &&
    normalized.clientId.length > 0 &&
    (!requiresReplacementSecret || normalized.clientSecret.length > 0) &&
    (issuer.protocol === 'https:' || isLocalHTTP) &&
    normalized.scopes.split(' ').includes('openid') &&
    (normalized.grafanaTarget !== GRAFANA_PROVIDER_KIND.NONE || normalized.confirmDevlakeOnly)
  );
};

export const getOIDCProviderError = (error: unknown) => {
  const code = extractOIDCProviderErrorCode(error);
  if (code === ACCESS_ERROR_CODE.INVALID_OIDC_PROVIDER) return ACCESS_ERROR.INVALID_OIDC_PROVIDER;
  if (code === ACCESS_ERROR_CODE.OIDC_PROVIDER_BLOCKED || code === ACCESS_ERROR_CODE.OIDC_PROVIDER_MISSING) {
    return ACCESS_ERROR.OIDC_PROVIDER_BLOCKED;
  }
  if (code === ACCESS_ERROR_CODE.OIDC_PROVIDER_REVISION_CONFLICT) return ACCESS_ERROR.OIDC_PROVIDER_STALE;
  if (code === ACCESS_ERROR_CODE.GRAFANA_TARGET_CONFLICT) return ACCESS_ERROR.GRAFANA_TARGET_CONFLICT;
  return ACCESS_ERROR.OIDC_PROVIDER_FAILED;
};

export const getOIDCProviderStatus = (provider?: OIDCProvider) => {
  if (!provider) return OIDC_PROVIDER_STATUS.CONFIGURED;
  if (provider.retiredAt) return OIDC_PROVIDER_STATUS.RETIRED;
  if (provider.grafanaSyncStatus === OIDC_PROVIDER_SYNC_STATUS.COMPENSATION_FAILED)
    return OIDC_PROVIDER_STATUS.RECOVERY;
  if (provider.grafanaSyncStatus === OIDC_PROVIDER_SYNC_STATUS.FAILED) return OIDC_PROVIDER_STATUS.FAILED;
  if (!provider.enabled) return OIDC_PROVIDER_STATUS.DISABLED;
  if (provider.grafanaTarget === GRAFANA_PROVIDER_KIND.NONE) return OIDC_PROVIDER_STATUS.DEVLAKE_ONLY;
  if (provider.providerRevision > provider.grafanaSyncedRevision) return OIDC_PROVIDER_STATUS.PENDING;
  return OIDC_PROVIDER_STATUS.ACTIVE;
};

export const canActivateOIDCProvider = (provider?: OIDCProvider) => {
  if (!provider?.providerKey || provider.grafanaSyncStatus === OIDC_PROVIDER_SYNC_STATUS.COMPENSATION_FAILED)
    return false;
  return !provider.databaseSourceActive || provider.hasCandidate;
};

export const canSelectGenericOIDCProvider = (provider: OIDCProvider) =>
  provider.enabled &&
  provider.grafanaTarget === GRAFANA_PROVIDER_KIND.NONE &&
  provider.grafanaSyncStatus !== OIDC_PROVIDER_SYNC_STATUS.COMPENSATION_FAILED;
