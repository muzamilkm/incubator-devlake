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

import { useState, useEffect, useMemo } from 'react';
import { useLoaderData, Outlet, useNavigate, useLocation } from 'react-router-dom';
import { Helmet } from 'react-helmet';
import { Layout as AntdLayout, Menu, Divider, Dropdown, Button, message } from 'antd';
import { UserOutlined, LogoutOutlined } from '@ant-design/icons';
import type { MenuProps } from 'antd';

import API from '@/api';
import { DEVLAKE_ENDPOINT } from '@/config';
import { PageLoading, Logo, ExternalLink } from '@/components';
import { init, selectError, selectStatus } from '@/features';
import { OnboardCard } from '@/routes/onboard/components';
import { OtelAttention } from '@/routes/otel/attention';
import { useAppDispatch, useAppSelector } from '@/hooks';

import { ACCESS_PATH, menuItems, menuItemsMatch, headerItems } from './config';
import type { AccessCurrent, LinkableOIDCProvider } from '@/api/access';
import { canManageAccess } from '@/routes/access/guard';

const { Sider, Header, Content, Footer } = AntdLayout;

const brandName = import.meta.env.DEVLAKE_BRAND_NAME ?? 'DevLake';

export const Layout = () => {
  const [openKeys, setOpenKeys] = useState<string[]>([]);
  const [selectedKeys, setSelectedKeys] = useState<string[]>([]);
  const [linkableProviders, setLinkableProviders] = useState<LinkableOIDCProvider[]>();
  const [linkProvidersFailed, setLinkProvidersFailed] = useState(false);

  const { version, plugins, user, access } = useLoaderData() as {
    version: string;
    plugins: string[];
    user: { authenticated: boolean; name: string; email: string } | null;
    access: AccessCurrent | null;
  };

  const visibleMenuItems = useMemo(
    () => menuItems.filter((item) => item.key !== ACCESS_PATH || canManageAccess(access)),
    [access],
  );

  const handleLogout = async () => {
    try {
      const res = await API.auth.logout();
      if (res.logoutUrl) {
        window.location.href = res.logoutUrl;
        return;
      }
    } catch (e) {
      // fall through to /login regardless
    }
    window.location.href = '/login';
  };

  const openDashboard = async () => {
    try {
      const { url } = await API.access.getGrafanaLogin();
      window.location.assign(url);
    } catch {
      window.location.assign('/grafana/');
    }
  };

  const loadLinkableProviders = (open: boolean) => {
    if (!open || !user?.authenticated || !access?.enabled || linkableProviders) return;
    API.access
      .listLinkableOIDCProviders()
      .then((providers) => {
        setLinkableProviders(providers);
        setLinkProvidersFailed(false);
      })
      .catch(() => setLinkProvidersFailed(true));
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
    { key: 'logout', icon: <LogoutOutlined />, label: 'Sign out', onClick: handleLogout },
  ];

  const navigate = useNavigate();
  const { pathname } = useLocation();

  const dispatch = useAppDispatch();
  const status = useAppSelector(selectStatus);
  const error = useAppSelector(selectError);

  useEffect(() => {
    dispatch(init(plugins));
  }, []);

  useEffect(() => {
    const curMenuItem = menuItemsMatch[pathname];
    const parentKey = curMenuItem?.parentKey;
    if (parentKey) {
      setOpenKeys([parentKey]);
    }
  }, []);

  useEffect(() => {
    const selectedKeys = pathname.split('/').reduce((acc, cur, i, arr) => {
      if (i === 0) {
        acc.push('/');
        return acc;
      } else {
        acc.push(`${arr.slice(0, i + 1).join('/')}`);
        return acc;
      }
    }, [] as string[]);
    setSelectedKeys(selectedKeys);
  }, [pathname]);

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
  }, [pathname]);

  const title = useMemo(() => {
    const curMenuItem = menuItemsMatch[pathname];
    return curMenuItem?.label ?? '';
  }, [pathname]);

  if (['idle', 'loading'].includes(status)) {
    return <PageLoading />;
  }

  if (status === 'failed') {
    throw error.message;
  }

  return (
    <AntdLayout style={{ height: '100%', overflow: 'hidden' }}>
      <Helmet>
        <title>
          {title ? `${title} - ` : ''}
          {brandName}
        </title>
      </Helmet>
      <Sider>
        {import.meta.env.DEVLAKE_TITLE_CUSTOM ? (
          <h2 style={{ margin: '36px 0', textAlign: 'center', color: '#fff' }}>
            {import.meta.env.DEVLAKE_TITLE_CUSTOM}
          </h2>
        ) : (
          <Logo style={{ padding: 24 }} />
        )}
        <Menu
          mode="inline"
          theme="dark"
          items={visibleMenuItems}
          openKeys={openKeys}
          selectedKeys={selectedKeys}
          onClick={({ key }) => navigate(key)}
          onOpenChange={(keys) => setOpenKeys(keys)}
        />
        <div style={{ position: 'absolute', right: 0, bottom: 20, left: 0, color: '#fff', textAlign: 'center' }}>
          {version}
        </div>
      </Sider>
      <AntdLayout>
        <Header
          style={{
            display: 'flex',
            justifyContent: 'flex-end',
            alignItems: 'center',
            padding: '0 24px',
            height: 50,
            background: 'transparent',
          }}
        >
          {headerItems
            .filter((item) =>
              import.meta.env.DEVLAKE_COPYRIGHT_HIDE ? !['Dashboards', 'GitHub', 'Slack'].includes(item.label) : true,
            )
            .map((item, i, arr) => (
              <span key={item.label} style={{ display: 'flex', alignItems: 'center' }}>
                {item.dashboard ? (
                  <Button
                    type="link"
                    onClick={openDashboard}
                    style={{ display: 'flex', alignItems: 'center', padding: 0 }}
                  >
                    {item.icon}
                    <span style={{ marginLeft: 4 }}>{item.label}</span>
                  </Button>
                ) : (
                  <ExternalLink link={item.link} style={{ display: 'flex', alignItems: 'center' }}>
                    {item.icon}
                    <span style={{ marginLeft: 4 }}>{item.label}</span>
                  </ExternalLink>
                )}
                {i !== arr.length - 1 && <Divider type="vertical" />}
              </span>
            ))}
          {user?.authenticated && (
            <>
              <Divider type="vertical" />
              <Dropdown menu={{ items: accountMenuItems }} placement="bottomRight" onOpenChange={loadLinkableProviders}>
                <Button type="text" icon={<UserOutlined />}>
                  {user.name || user.email || 'Account'}
                </Button>
              </Dropdown>
            </>
          )}
        </Header>
        <Content style={{ overflowY: 'auto' }}>
          <div style={{ padding: 24, margin: '0 auto', maxWidth: 1280 }}>
            <OtelAttention />
            <OnboardCard style={{ marginBottom: 32 }} />
            <Outlet />
          </div>
          {!import.meta.env.DEVLAKE_COPYRIGHT_HIDE && (
            <Footer>
              <p style={{ textAlign: 'center' }}>Apache 2.0 License</p>
            </Footer>
          )}
        </Content>
      </AntdLayout>
    </AntdLayout>
  );
};
