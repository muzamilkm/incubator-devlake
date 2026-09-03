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

import React, { useEffect, useRef, useState } from 'react';
import { LogoutOutlined } from '@ant-design/icons';
import type { MenuProps } from 'antd';
import { message } from 'antd';

import API from '@/api';
import type { AccessCurrent, LinkableOIDCProvider } from '@/api/access';
import { DEVLAKE_ENDPOINT } from '@/config';

const FAILURE_COOLDOWN_MS = 10_000;

type UseAccountMenuOptions = {
  user?: { authenticated: boolean };
  access?: AccessCurrent | null;
  handleLogout: () => void;
};

export const useAccountMenu = ({ user, access, handleLogout }: UseAccountMenuOptions) => {
  const [linkableProviders, setLinkableProviders] = useState<LinkableOIDCProvider[]>();
  const [linkProvidersFailed, setLinkProvidersFailed] = useState(false);
  const lastFailedAtRef = useRef<number>(0);

  const loadLinkableProviders = (open: boolean) => {
    if (!open || !user?.authenticated || !access?.enabled || linkableProviders) return;
    const now = Date.now();
    if (linkProvidersFailed && now - lastFailedAtRef.current < FAILURE_COOLDOWN_MS) {
      return;
    }
    API.access
      .listLinkableOIDCProviders()
      .then((providers) => {
        setLinkableProviders(providers);
        setLinkProvidersFailed(false);
      })
      .catch(() => {
        lastFailedAtRef.current = Date.now();
        setLinkProvidersFailed(true);
      });
  };

  const startIdentityLink = (providerKey: string) => {
    const returnURL = `${window.location.pathname}${window.location.search}`;
    window.location.assign(
      `${DEVLAKE_ENDPOINT}/auth/link-identity?provider=${encodeURIComponent(
        providerKey,
      )}&return_url=${encodeURIComponent(returnURL)}`,
    );
  };

  const accountMenuItems: MenuProps['items'] = [
    ...(linkableProviders?.map((provider) => ({
      key: `link-identity-${provider.providerKey}`,
      label: `Add ${provider.displayName} sign-in`,
      onClick: () => startIdentityLink(provider.providerKey),
    })) ?? []),
    ...(linkableProviders?.length === 0
      ? [{ key: 'no-linkable-providers', disabled: true, label: 'No additional sign-in providers' }]
      : []),
    ...(linkProvidersFailed
      ? [{ key: 'linkable-providers-failed', disabled: true, label: 'Additional sign-in methods are unavailable' }]
      : []),
    { type: 'divider' },
    { key: 'logout', icon: React.createElement(LogoutOutlined), label: 'Sign out', onClick: handleLogout },
  ];

  return {
    accountMenuItems,
    loadLinkableProviders,
  };
};

export const useIdentityLinkNotification = () => {
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const identityLinkResult = params.get('identity_link');
    if (!identityLinkResult) return;
    if (identityLinkResult === 'linked') {
      message.success('Additional sign-in method added.');
    } else {
      message.error('The additional sign-in method could not be added. Please try again.');
    }
    params.delete('identity_link');
    const query = params.toString();
    window.history.replaceState(null, '', `${window.location.pathname}${query ? `?${query}` : ''}`);
  }, []);
};
