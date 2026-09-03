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

import { equal } from 'node:assert/strict';
import { test } from 'node:test';
import { AxiosError, AxiosHeaders, HttpStatusCode } from 'axios';

import {
  ACCESS_ERROR_CODE,
  GRAFANA_PROVIDER_KIND,
  OIDC_PROVIDER_SYNC_STATUS,
  type OIDCProvider,
} from '../../api/access';

import {
  ACCESS_ERROR,
  canActivateOIDCProvider,
  getAuthenticationState,
  formFromOIDCProvider,
  getCreateDomainError,
  getCreateUserError,
  getOIDCProviderError,
  getOIDCProviderStatus,
  isValidDomain,
  isValidEmail,
  isValidOIDCProviderInput,
  normalizeOIDCProviderInput,
  canSelectGenericOIDCProvider,
  normalizeDomain,
} from './utils';
import { AUTHENTICATION_STATE, OIDC_PROVIDER_STATUS, OIDC_PROVIDER_STATUS_COLOR } from './constants';

const createAxiosError = (status: number, data: unknown) =>
  new AxiosError('Request failed', 'ERR_BAD_REQUEST', undefined, undefined, {
    status,
    statusText: status === 400 ? 'Bad Request' : 'Internal Server Error',
    headers: {},
    config: { headers: new AxiosHeaders() },
    data,
  });

test('normalizes allowed-domain input before it is submitted', () => {
  equal(normalizeDomain(' Example.COM '), 'example.com');
});

test('rejects invalid allowed-domain input locally', () => {
  equal(isValidDomain('example.com'), true);
  equal(isValidDomain('example'), true);
  equal(isValidDomain(''), false);
  equal(isValidDomain('example..com'), false);
  equal(isValidDomain('person@example.com'), false);
  equal(isValidDomain('@example.com'), false);
  equal(isValidDomain('[192.168.1.1]'), false);
  equal(isValidDomain('example.com.'), false);
});

test('rejects invalid email input locally', () => {
  equal(isValidEmail('person@example.com'), true);
  equal(isValidEmail('person@example'), true);
  equal(isValidEmail('@example.com'), false);
  equal(isValidEmail('person@example.com '), true);
  equal(isValidEmail('person @example.com'), false);
  equal(isValidEmail('person@example..com'), false);
});

test('maps create-user error codes to safe UI copy', () => {
  const duplicateErr = createAxiosError(HttpStatusCode.BadRequest, {
    code: ACCESS_ERROR_CODE.DUPLICATE_USER,
    message: 'this email already has a DevLake access entry',
  });
  equal(getCreateUserError(duplicateErr), ACCESS_ERROR.DUPLICATE_USER);

  const invalidErr = createAxiosError(HttpStatusCode.BadRequest, {
    code: ACCESS_ERROR_CODE.INVALID_USER,
    message: 'provide a valid email and role',
  });
  equal(getCreateUserError(invalidErr), ACCESS_ERROR.INVALID_USER);

  const serverErr = createAxiosError(HttpStatusCode.InternalServerError, {
    message: 'internal server error',
  });
  equal(getCreateUserError(serverErr), ACCESS_ERROR.REQUEST_FAILED);

  equal(getCreateUserError(new Error('network error')), ACCESS_ERROR.REQUEST_FAILED);
});

test('maps create-domain error codes to safe UI copy', () => {
  const duplicateErr = createAxiosError(HttpStatusCode.BadRequest, {
    code: ACCESS_ERROR_CODE.DUPLICATE_DOMAIN,
    message: 'this domain already has a DevLake access policy',
  });
  equal(getCreateDomainError(duplicateErr), ACCESS_ERROR.DUPLICATE_DOMAIN);

  const invalidErr = createAxiosError(HttpStatusCode.BadRequest, {
    code: ACCESS_ERROR_CODE.INVALID_DOMAIN,
    message: 'provide a valid domain and default role',
  });
  equal(getCreateDomainError(invalidErr), ACCESS_ERROR.INVALID_DOMAIN);

  const serverErr = createAxiosError(HttpStatusCode.InternalServerError, {
    message: 'internal server error',
  });
  equal(getCreateDomainError(serverErr), ACCESS_ERROR.REQUEST_FAILED);

  equal(getCreateDomainError(new Error('network error')), ACCESS_ERROR.REQUEST_FAILED);
});

test('normalizes and validates OIDC provider settings locally', () => {
  const provider = normalizeOIDCProviderInput({
    providerKey: ' Google-Workspace ',
    displayName: ' Google Workspace ',
    issuerUrl: 'https://accounts.example.com///',
    clientId: ' client-id ',
    clientSecret: ' secret ',
    scopes: 'openid, profile openid email',
    grafanaTarget: GRAFANA_PROVIDER_KIND.GOOGLE,
    confirmDevlakeOnly: false,
  });

  equal(provider.providerKey, 'google-workspace');
  equal(provider.issuerUrl, 'https://accounts.example.com');
  equal(provider.scopes, 'openid profile email');
  equal(isValidOIDCProviderInput(provider), true);
  equal(isValidOIDCProviderInput({ ...provider, providerKey: 'invalid/key' }), false);
  equal(isValidOIDCProviderInput({ ...provider, issuerUrl: 'http://issuer.example.com' }), false);
  equal(isValidOIDCProviderInput({ ...provider, scopes: 'profile email' }), false);
  equal(isValidOIDCProviderInput({ ...provider, issuerUrl: 'http://localhost:5556' }), false);
  equal(isValidOIDCProviderInput({ ...provider, issuerUrl: 'http://localhost:5556' }, undefined, true), true);
});

test('allows stored OIDC credentials only for the unchanged client ID', () => {
  const provider = normalizeOIDCProviderInput({
    providerKey: 'google',
    displayName: 'Google',
    issuerUrl: 'https://accounts.example.com',
    clientId: 'client-a',
    clientSecret: '',
    scopes: 'openid profile email',
    grafanaTarget: GRAFANA_PROVIDER_KIND.GENERIC_OAUTH,
    confirmDevlakeOnly: false,
  });
  const configuredProvider: OIDCProvider = {
    providerKey: 'google',
    displayName: 'Google',
    issuerUrl: 'https://accounts.example.com',
    clientId: 'client-a',
    scopes: 'openid profile email',
    enabled: true,
    secretConfigured: true,
    databaseSourceActive: true,
    grafanaSyncStatus: OIDC_PROVIDER_SYNC_STATUS.SYNCHRONIZED,
    grafanaSyncedRevision: 1,
    providerRevision: 1,
    hasCandidate: false,
    grafanaTarget: GRAFANA_PROVIDER_KIND.GENERIC_OAUTH,
    devlakeCallbackUrl: 'https://devlake.example.com/api/auth/callback',
    grafanaCallbackUrl: 'https://grafana.example.com/login/generic_oauth',
    allowLocalOidc: false,
  };

  equal(isValidOIDCProviderInput(provider, configuredProvider), true);
  equal(isValidOIDCProviderInput({ ...provider, clientId: 'client-b' }, configuredProvider), false);
});

test('creates a write-only OIDC provider form from configured state', () => {
  const provider: OIDCProvider = {
    providerKey: 'google',
    displayName: 'Google',
    issuerUrl: 'https://accounts.google.com',
    clientId: 'client',
    scopes: 'openid profile email',
    enabled: true,
    secretConfigured: true,
    databaseSourceActive: true,
    grafanaSyncStatus: OIDC_PROVIDER_SYNC_STATUS.SYNCHRONIZED,
    grafanaSyncedRevision: 1,
    providerRevision: 1,
    hasCandidate: false,
    grafanaTarget: GRAFANA_PROVIDER_KIND.GENERIC_OAUTH,
    devlakeCallbackUrl: 'https://devlake.example.com/api/auth/callback',
    grafanaCallbackUrl: 'https://grafana.example.com/login/generic_oauth',
    allowLocalOidc: false,
  };

  equal(formFromOIDCProvider(provider).clientSecret, '');
  equal(formFromOIDCProvider(provider).scopes, provider.scopes);
  equal(formFromOIDCProvider().scopes, 'openid profile email');
});

test('maps OIDC provider errors to safe user-facing messages', () => {
  const invalidProvider = createAxiosError(HttpStatusCode.BadRequest, {
    code: ACCESS_ERROR_CODE.INVALID_OIDC_PROVIDER,
  });
  const blockedProvider = createAxiosError(HttpStatusCode.BadRequest, {
    code: ACCESS_ERROR_CODE.OIDC_PROVIDER_BLOCKED,
  });
  const unavailableBlockedProvider = createAxiosError(HttpStatusCode.ServiceUnavailable, {
    code: ACCESS_ERROR_CODE.OIDC_PROVIDER_BLOCKED,
  });
  const unavailableUnknownProvider = createAxiosError(HttpStatusCode.ServiceUnavailable, {
    code: 'GRAFANA_CREDENTIAL_REJECTED',
  });

  equal(getOIDCProviderError(invalidProvider), ACCESS_ERROR.INVALID_OIDC_PROVIDER);
  equal(getOIDCProviderError(blockedProvider), ACCESS_ERROR.OIDC_PROVIDER_BLOCKED);
  equal(getOIDCProviderError(unavailableBlockedProvider), ACCESS_ERROR.OIDC_PROVIDER_BLOCKED);
  equal(getOIDCProviderError(unavailableUnknownProvider), ACCESS_ERROR.OIDC_PROVIDER_FAILED);
  equal(getOIDCProviderError(new Error('network error')), ACCESS_ERROR.OIDC_PROVIDER_FAILED);
});

test('summarizes OIDC provider lifecycle state without exposing internal synchronization details', () => {
  const configuredProvider: OIDCProvider = {
    providerKey: 'google',
    displayName: 'Google',
    issuerUrl: 'https://accounts.google.com',
    clientId: 'client',
    scopes: 'openid profile email',
    enabled: false,
    secretConfigured: true,
    databaseSourceActive: false,
    grafanaSyncStatus: OIDC_PROVIDER_SYNC_STATUS.SYNCHRONIZED,
    grafanaSyncedRevision: 1,
    providerRevision: 1,
    hasCandidate: false,
    grafanaTarget: GRAFANA_PROVIDER_KIND.GENERIC_OAUTH,
    devlakeCallbackUrl: 'https://devlake.example.com/api/auth/callback',
    grafanaCallbackUrl: 'https://grafana.example.com/login/generic_oauth',
    allowLocalOidc: false,
  };

  equal(getOIDCProviderStatus(undefined), OIDC_PROVIDER_STATUS.CONFIGURED);
  equal(getOIDCProviderStatus(configuredProvider), OIDC_PROVIDER_STATUS.CONFIGURED);
  equal(canActivateOIDCProvider(configuredProvider), true);
  equal(
    getOIDCProviderStatus({ ...configuredProvider, databaseSourceActive: true, enabled: true }),
    OIDC_PROVIDER_STATUS.ACTIVE,
  );
  equal(
    getOIDCProviderStatus({ ...configuredProvider, grafanaSyncStatus: OIDC_PROVIDER_SYNC_STATUS.COMPENSATION_FAILED }),
    OIDC_PROVIDER_STATUS.RECOVERY,
  );
  const compensatedProvider = { ...configuredProvider, grafanaSyncStatus: OIDC_PROVIDER_SYNC_STATUS.COMPENSATED };
  equal(getOIDCProviderStatus(compensatedProvider), OIDC_PROVIDER_STATUS.COMPENSATED);
  equal(canActivateOIDCProvider(compensatedProvider), true);
  equal(OIDC_PROVIDER_STATUS_COLOR[OIDC_PROVIDER_STATUS.ACTIVE], 'green');
  equal(OIDC_PROVIDER_STATUS_COLOR[OIDC_PROVIDER_STATUS.COMPENSATED], 'orange');
  equal(OIDC_PROVIDER_STATUS_COLOR[OIDC_PROVIDER_STATUS.RECOVERY], 'red');
  equal(OIDC_PROVIDER_STATUS_COLOR[OIDC_PROVIDER_STATUS.CONFIGURED], 'orange');
});

test('summarizes the authentication configuration state without inferring environment configuration', () => {
  const provider: OIDCProvider = {
    providerKey: 'google',
    displayName: 'Google',
    issuerUrl: 'https://accounts.google.com',
    clientId: 'client',
    scopes: 'openid profile email',
    enabled: false,
    secretConfigured: true,
    databaseSourceActive: false,
    grafanaSyncStatus: OIDC_PROVIDER_SYNC_STATUS.SYNCHRONIZED,
    grafanaSyncedRevision: 1,
    providerRevision: 1,
    hasCandidate: false,
    grafanaTarget: GRAFANA_PROVIDER_KIND.GOOGLE,
    devlakeCallbackUrl: 'https://devlake.example.com/api/auth/callback',
    grafanaCallbackUrl: 'https://grafana.example.com/login/google',
  };

  equal(getAuthenticationState([]), AUTHENTICATION_STATE.NO_MANAGED_OIDC);
  equal(getAuthenticationState([{ ...provider, hasCandidate: true }]), AUTHENTICATION_STATE.ACTIVATION_REQUIRED);
  equal(getAuthenticationState([provider]), AUTHENTICATION_STATE.NO_ACTIVE_OIDC);
  equal(
    getAuthenticationState([{ ...provider, enabled: true, databaseSourceActive: true }]),
    AUTHENTICATION_STATE.OIDC_ACTIVE,
  );
});

test('requires explicit DevLake-only confirmation and identifies a Generic OAuth candidate', () => {
  const devLakeOnly = normalizeOIDCProviderInput({
    providerKey: 'custom',
    displayName: 'Custom OIDC',
    issuerUrl: 'https://id.example.com',
    clientId: 'client',
    clientSecret: 'secret',
    scopes: 'openid email',
    grafanaTarget: GRAFANA_PROVIDER_KIND.NONE,
    confirmDevlakeOnly: false,
  });
  equal(isValidOIDCProviderInput(devLakeOnly), false);
  equal(isValidOIDCProviderInput({ ...devLakeOnly, confirmDevlakeOnly: true }), true);

  const provider: OIDCProvider = {
    providerKey: 'custom',
    displayName: 'Custom OIDC',
    issuerUrl: 'https://id.example.com',
    clientId: 'client',
    scopes: 'openid email',
    grafanaTarget: GRAFANA_PROVIDER_KIND.NONE,
    enabled: true,
    secretConfigured: true,
    databaseSourceActive: true,
    grafanaSyncStatus: OIDC_PROVIDER_SYNC_STATUS.NOT_APPLICABLE,
    grafanaSyncedRevision: 0,
    providerRevision: 1,
    hasCandidate: false,
    devlakeCallbackUrl: 'https://devlake.example.com/api/auth/callback',
    grafanaCallbackUrl: 'https://grafana.example.com/login',
  };
  equal(canSelectGenericOIDCProvider(provider), true);
  equal(canSelectGenericOIDCProvider({ ...provider, enabled: false }), false);
});
