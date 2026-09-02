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

import { useCallback, useMemo, useState } from 'react';
import { Button, Table } from 'antd';
import { PlusOutlined } from '@ant-design/icons';

import API from '@/api';
import { PageHeader } from '@/components';
import { useRefreshData } from '@/hooks';
import { operator } from '@/utils';
import { ACCESS_ROLE, type AccessDomain, type AccessRole, type AccessStatus, type AccessUser } from '@/api/access';

import { getAuditColumns, getDomainColumns, getUserColumns } from './columns';
import { Authentication } from './authentication';
import { BREADCRUMBS, DEFAULT_PAGE_SIZE, PAGE_DESCRIPTION, PAGE_SIZE_OPTIONS } from './constants';
import { CreateDomainModal, CreateUserModal } from './modals';
import { SectionHeader, SectionTitle } from './styled';
import { getCreateDomainError, getCreateUserError, isValidDomain, isValidEmail, normalizeDomain } from './utils';

type ModalState = 'user' | 'domain' | undefined;

const AUDIT_COLUMNS = getAuditColumns();

export const Access = () => {
  const [version, setVersion] = useState(0);
  const [userPage, setUserPage] = useState(1);
  const [userPageSize, setUserPageSize] = useState<(typeof PAGE_SIZE_OPTIONS)[number]>(DEFAULT_PAGE_SIZE);
  const [domainPage, setDomainPage] = useState(1);
  const [domainPageSize, setDomainPageSize] = useState<(typeof PAGE_SIZE_OPTIONS)[number]>(DEFAULT_PAGE_SIZE);
  const [modal, setModal] = useState<ModalState>();
  const [operating, setOperating] = useState(false);
  const [email, setEmail] = useState('');
  const [domain, setDomain] = useState('');
  const [role, setRole] = useState<AccessRole>(ACCESS_ROLE.MEMBER);
  const refresh = useCallback(() => setVersion((current) => current + 1), []);

  const { data, ready } = useRefreshData(async () => {
    const [users, domains, auditEvents, providerResult] = await Promise.all([
      API.access.listUsers({ page: userPage, pageSize: userPageSize }),
      API.access.listDomains({ page: domainPage, pageSize: domainPageSize }),
      API.access.listAuditEvents(),
      API.access
        .listOIDCProviders()
        .then((providers) => ({ providers, loadFailed: false }))
        .catch(() => ({ providers: [], loadFailed: true })),
    ]);
    return { users, domains, auditEvents, providerResult };
  }, [version, userPage, userPageSize, domainPage, domainPageSize]);
  const users = data?.users;
  const domains = data?.domains;
  const auditEvents = data?.auditEvents ?? [];

  const normalizedDomain = normalizeDomain(domain);
  const domainError =
    domain.length > 0 && !isValidDomain(domain) ? 'Enter a valid email domain, such as example.com.' : '';
  const emailError =
    email.length > 0 && !isValidEmail(email) ? 'Enter a valid email address, such as person@example.com.' : '';

  const closeModal = useCallback(() => {
    setModal(undefined);
    setEmail('');
    setDomain('');
    setRole(ACCESS_ROLE.MEMBER);
  }, []);

  const createUser = useCallback(async () => {
    const [success] = await operator(() => API.access.createUser({ email: email.trim().toLowerCase(), role }), {
      setOperating,
      formatReason: getCreateUserError,
    });
    if (success) {
      closeModal();
      refresh();
    }
  }, [closeModal, email, refresh, role]);

  const createDomain = useCallback(async () => {
    const [success] = await operator(() => API.access.createDomain({ domain: normalizedDomain, defaultRole: role }), {
      setOperating,
      formatReason: getCreateDomainError,
    });
    if (success) {
      closeModal();
      refresh();
    }
  }, [closeModal, normalizedDomain, refresh, role]);

  const updateUser = useCallback(
    async (user: AccessUser, nextStatus: AccessStatus) => {
      const [success] = await operator(() => API.access.updateUser(user.id, { role: user.role, status: nextStatus }));
      if (success) refresh();
    },
    [refresh],
  );

  const updateUserRole = useCallback(
    async (user: AccessUser, nextRole: AccessRole) => {
      const [success] = await operator(() => API.access.updateUser(user.id, { role: nextRole, status: user.status }));
      if (success) refresh();
    },
    [refresh],
  );

  const updateDomain = useCallback(
    async (accessDomain: AccessDomain, nextStatus: AccessStatus) => {
      const [success] = await operator(() =>
        API.access.updateDomain(accessDomain.id, { defaultRole: accessDomain.defaultRole, status: nextStatus }),
      );
      if (success) refresh();
    },
    [refresh],
  );

  const updateDomainRole = useCallback(
    async (accessDomain: AccessDomain, nextRole: AccessRole) => {
      const [success] = await operator(() =>
        API.access.updateDomain(accessDomain.id, { defaultRole: nextRole, status: accessDomain.status }),
      );
      if (success) refresh();
    },
    [refresh],
  );

  const hideUser = useCallback(
    async (user: AccessUser) => {
      const [success] = await operator(() => API.access.hideUser(user.id));
      if (success) refresh();
    },
    [refresh],
  );

  const hideDomain = useCallback(
    async (accessDomain: AccessDomain) => {
      const [success] = await operator(() => API.access.hideDomain(accessDomain.id));
      if (success) refresh();
    },
    [refresh],
  );

  const userColumns = useMemo(
    () =>
      getUserColumns({
        onRoleChange: updateUserRole,
        onStatusChange: updateUser,
        onRemove: hideUser,
      }),
    [hideUser, updateUser, updateUserRole],
  );

  const domainColumns = useMemo(
    () =>
      getDomainColumns({
        onRoleChange: updateDomainRole,
        onStatusChange: updateDomain,
        onRemove: hideDomain,
      }),
    [hideDomain, updateDomain, updateDomainRole],
  );

  return (
    <PageHeader breadcrumbs={BREADCRUMBS} description={PAGE_DESCRIPTION}>
      <SectionHeader>
        <SectionTitle>People</SectionTitle>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setModal('user')}>
          Add person
        </Button>
      </SectionHeader>
      <Table
        rowKey="id"
        size="middle"
        loading={!ready}
        dataSource={users?.users ?? []}
        pagination={{
          current: userPage,
          pageSize: userPageSize,
          total: users?.count ?? 0,
          pageSizeOptions: PAGE_SIZE_OPTIONS,
          showSizeChanger: true,
          onChange: (nextPage, nextPageSize) => {
            if (nextPageSize && nextPageSize !== userPageSize) {
              setUserPageSize(nextPageSize as (typeof PAGE_SIZE_OPTIONS)[number]);
              setUserPage(1);
              return;
            }
            setUserPage(nextPage);
          },
        }}
        columns={userColumns}
      />

      <SectionHeader $spaced>
        <SectionTitle>Allowed domains</SectionTitle>
        <Button icon={<PlusOutlined />} onClick={() => setModal('domain')}>
          Add domain
        </Button>
      </SectionHeader>
      <Table
        rowKey="id"
        size="middle"
        loading={!ready}
        dataSource={domains?.domains ?? []}
        pagination={{
          current: domainPage,
          pageSize: domainPageSize,
          total: domains?.count ?? 0,
          pageSizeOptions: PAGE_SIZE_OPTIONS,
          showSizeChanger: true,
          onChange: (nextPage, nextPageSize) => {
            if (nextPageSize && nextPageSize !== domainPageSize) {
              setDomainPageSize(nextPageSize as (typeof PAGE_SIZE_OPTIONS)[number]);
              setDomainPage(1);
              return;
            }
            setDomainPage(nextPage);
          },
        }}
        columns={domainColumns}
      />

      <Authentication
        providers={data?.providerResult.providers ?? []}
        loadFailed={data?.providerResult.loadFailed ?? false}
        onRefresh={refresh}
      />

      <SectionHeader $spaced>
        <SectionTitle>Recent access activity</SectionTitle>
      </SectionHeader>
      <Table
        rowKey="id"
        size="middle"
        loading={!ready}
        dataSource={auditEvents}
        pagination={false}
        columns={AUDIT_COLUMNS}
      />

      <CreateUserModal
        open={modal === 'user'}
        email={email}
        role={role}
        emailError={emailError}
        operating={operating}
        onEmailChange={setEmail}
        onRoleChange={setRole}
        onCancel={closeModal}
        onSubmit={createUser}
      />

      <CreateDomainModal
        open={modal === 'domain'}
        domain={domain}
        role={role}
        domainError={domainError}
        operating={operating}
        onDomainChange={setDomain}
        onRoleChange={setRole}
        onCancel={closeModal}
        onSubmit={createDomain}
      />
    </PageHeader>
  );
};
