import { KeyOutlined, PlusOutlined } from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Button,
  Card,
  Col,
  Drawer,
  Form,
  Input,
  InputNumber,
  List,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
  message,
  type TableColumnsType,
} from 'antd'
import dayjs from 'dayjs'
import { useMemo, useState } from 'react'
import { useTenants } from '../hooks/useTenants'
import { createApiKey, listApiKeys, revokeApiKey } from '../services/access'
import { listSessions, terminateSession } from '../services/auth'
import { createTenant, deleteTenant, updateTenant } from '../services/tenants'
import { createRole, deleteRole, listRoles, updateRole } from '../services/rbac'
import { useI18n } from '../hooks/useI18n'
import type {
  AccessApiKey,
  CreateApiKeyPayload,
  SessionSummary,
  TenantPayload,
  TenantSummary,
  RbacRolePayload,
  RbacRoleSummary,
} from '../types/api'

type TenantFormValues = {
  name: string
  contact?: string
  description?: string
  status?: 'active' | 'suspended'
  pools?: number
  leases?: number
  staticBindings?: number
}

type CreateKeyForm = CreateApiKeyPayload

type RoleFormValues = {
  name: string
  description?: string
  permissions: number
}

export default function SystemManagementPage() {
  const { t } = useI18n()
  const tenantsQuery = useTenants()
  const tenants = tenantsQuery.data ?? []
  const tenantsLoading = tenantsQuery.isLoading
  const queryClient = useQueryClient()
  const [tenantDrawerOpen, setTenantDrawerOpen] = useState(false)
  const [editingTenant, setEditingTenant] = useState<TenantSummary | null>(null)
  const [tenantForm] = Form.useForm<TenantFormValues>()
  const [apiKeyModalOpen, setApiKeyModalOpen] = useState(false)
  const [apiKeyForm] = Form.useForm<CreateKeyForm>()
  const [issuedToken, setIssuedToken] = useState<string | null>(null)
  const [roleDrawerOpen, setRoleDrawerOpen] = useState(false)
  const [editingRole, setEditingRole] = useState<RbacRoleSummary | null>(null)
  const [roleForm] = Form.useForm<RoleFormValues>()
  const [apiKeyFilter, setApiKeyFilter] = useState('')
  const tenantStatusMeta = useMemo(
    () => ({
      active: { color: 'green', label: t('systemTenantStatusActive') },
      suspended: { color: 'default', label: t('systemTenantStatusSuspended') },
    }),
    [t],
  )

  const { data: apiKeys, isLoading: apiKeysLoading } = useQuery({
    queryKey: ['api-keys'],
    queryFn: listApiKeys,
    staleTime: 45_000,
  })

  const { data: sessions, isLoading: sessionsLoading } = useQuery({
    queryKey: ['sessions'],
    queryFn: listSessions,
    refetchInterval: 60_000,
  })

  const createTenantMutation = useMutation({
    mutationFn: (payload: TenantPayload) => createTenant(payload),
    onSuccess: () => {
      message.success(t('systemTenantCreateSuccess'))
      queryClient.invalidateQueries({ queryKey: ['tenants'] })
      closeTenantDrawer()
    },
    onError: (error: unknown) => {
      message.error(error instanceof Error ? error.message : t('systemTenantCreateError'))
    },
  })

  const updateTenantMutation = useMutation({
    mutationFn: ({ tenantId, payload }: { tenantId: string; payload: TenantPayload }) =>
      updateTenant(tenantId, payload),
    onSuccess: () => {
      message.success(t('systemTenantUpdateSuccess'))
      queryClient.invalidateQueries({ queryKey: ['tenants'] })
      closeTenantDrawer()
    },
    onError: (error: unknown) => {
      message.error(error instanceof Error ? error.message : t('systemTenantUpdateError'))
    },
  })

  const deleteTenantMutation = useMutation({
    mutationFn: (tenantId: string) => deleteTenant(tenantId),
    onSuccess: () => {
      message.success(t('systemTenantDeleteSuccess'))
      queryClient.invalidateQueries({ queryKey: ['tenants'] })
    },
    onError: () => message.error(t('systemTenantDeleteError')),
  })

  const createKeyMutation = useMutation({
    mutationFn: (payload: CreateApiKeyPayload) => createApiKey(payload),
    onSuccess: (response) => {
      setIssuedToken(response.token)
      message.success(t('systemApiKeyCreateSuccess'))
      apiKeyForm.resetFields()
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
    },
    onError: (error: unknown) => {
      message.error(error instanceof Error ? error.message : t('systemApiKeyCreateError'))
    },
  })

  const revokeKeyMutation = useMutation({
    mutationFn: (keyId: string) => revokeApiKey(keyId),
    onSuccess: () => {
      message.success(t('systemApiKeyRevokeSuccess'))
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
    },
    onError: () => message.error(t('systemApiKeyRevokeError')),
  })

  const terminateSessionMutation = useMutation({
    mutationFn: (sessionId: string) => terminateSession(sessionId),
    onSuccess: () => {
      message.success(t('systemSessionTerminateSuccess'))
      queryClient.invalidateQueries({ queryKey: ['sessions'] })
    },
    onError: () => message.error(t('systemSessionTerminateError')),
  })

  const { data: roles, isLoading: rolesLoading } = useQuery({
    queryKey: ['roles'],
    queryFn: listRoles,
    staleTime: 60_000,
  })

  const createRoleMutation = useMutation({
    mutationFn: (payload: RbacRolePayload) => createRole(payload),
    onSuccess: () => {
      message.success(t('systemRoleCreateSuccess'))
      queryClient.invalidateQueries({ queryKey: ['roles'] })
      closeRoleDrawer()
    },
    onError: (error: unknown) =>
      message.error(error instanceof Error ? error.message : t('systemRoleCreateError')),
  })

  const updateRoleMutation = useMutation({
    mutationFn: ({ roleId, payload }: { roleId: string; payload: RbacRolePayload }) =>
      updateRole(roleId, payload),
    onSuccess: () => {
      message.success(t('systemRoleUpdateSuccess'))
      queryClient.invalidateQueries({ queryKey: ['roles'] })
      closeRoleDrawer()
    },
    onError: (error: unknown) =>
      message.error(error instanceof Error ? error.message : t('systemRoleUpdateError')),
  })

  const deleteRoleMutation = useMutation({
    mutationFn: (roleId: string) => deleteRole(roleId),
    onSuccess: () => {
      message.success(t('systemRoleDeleteSuccess'))
      queryClient.invalidateQueries({ queryKey: ['roles'] })
    },
    onError: () => message.error(t('systemRoleDeleteError')),
  })

  const tenantColumns: TableColumnsType<TenantSummary> = [
    { title: t('systemFieldName'), dataIndex: 'name', key: 'name' },
    {
      title: t('systemFieldContact'),
      dataIndex: 'contact',
      key: 'contact',
      render: (value?: string) => value ?? '—',
    },
    {
      title: t('systemFieldStatus'),
      dataIndex: 'status',
      key: 'status',
      render: (value?: 'active' | 'suspended') =>
        value ? (
          <Tag color={tenantStatusMeta[value].color}>{tenantStatusMeta[value].label}</Tag>
        ) : (
          <Tag>{t('commonUnknown')}</Tag>
        ),
    },
    {
      title: t('systemQuotaTitle'),
      key: 'quotas',
      render: (_, record) => (
        <Space direction="vertical" size={2}>
          <Typography.Text type="secondary">
            {t('systemQuotaPools', { count: record.quotas?.pools ?? '—' })}
          </Typography.Text>
          <Typography.Text type="secondary">
            {t('systemQuotaLeases', { count: record.quotas?.leases ?? '—' })}
          </Typography.Text>
          <Typography.Text type="secondary">
            {t('systemQuotaBindings', { count: record.quotas?.staticBindings ?? '—' })}
          </Typography.Text>
        </Space>
      ),
    },
    {
      title: t('systemActionsLabel'),
      key: 'actions',
      render: (_, record) => (
        <Space>
          <Button type="link" onClick={() => openTenantDrawer(record)}>
            {t('commonEdit')}
          </Button>
          <Popconfirm
            title={t('systemTenantDeleteConfirm')}
            okText={t('commonDelete')}
            cancelText={t('commonCancel')}
            okButtonProps={{ danger: true, loading: deleteTenantMutation.isPending }}
            onConfirm={() => deleteTenantMutation.mutate(record.id)}
          >
            <Button type="link" danger loading={deleteTenantMutation.isPending}>
              {t('commonDelete')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const roleColumns: TableColumnsType<RbacRoleSummary> = [
    { title: t('systemFieldName'), dataIndex: 'name', key: 'name' },
    {
      title: t('systemFieldDescription'),
      dataIndex: 'description',
      key: 'description',
      render: (value?: string) => value ?? '—',
    },
    {
      title: t('systemRolePermissionsLabel'),
      dataIndex: 'permissions',
      key: 'permissions',
      render: (value: number) => value.toLocaleString(),
    },
    {
      title: t('systemActionsLabel'),
      key: 'actions',
      render: (_, record) => (
        <Space>
          <Button type="link" onClick={() => openRoleDrawer(record)}>
            {t('commonEdit')}
          </Button>
          <Popconfirm
            title={t('systemRoleDeleteConfirm')}
            okText={t('commonDelete')}
            cancelText={t('commonCancel')}
            okButtonProps={{ danger: true, loading: deleteRoleMutation.isPending }}
            onConfirm={() => deleteRoleMutation.mutate(record.id)}
          >
            <Button type="link" danger loading={deleteRoleMutation.isPending}>
              {t('commonDelete')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const totalTenants = tenants.length
  const activeTenants = tenants.filter((tenant) => tenant.status !== 'suspended').length
  const totalApiKeys = apiKeys?.length ?? 0
  const roleCount = roles?.length ?? 0
  const activeSessionsCount = sessions?.filter((session) => session.active).length ?? 0
  const filteredApiKeys = useMemo(() => {
    if (!apiKeys) return []
    const term = apiKeyFilter.trim().toLowerCase()
    if (!term) return apiKeys
    return apiKeys.filter((key) =>
      [key.label, key.scope].some((field) => field?.toLowerCase().includes(term)),
    )
  }, [apiKeys, apiKeyFilter])

  function openTenantDrawer(tenant?: TenantSummary) {
    setTenantDrawerOpen(true)
    if (tenant) {
      setEditingTenant(tenant)
      tenantForm.setFieldsValue({
        name: tenant.name,
        contact: tenant.contact,
        description: tenant.description,
        status: tenant.status ?? 'active',
        pools: tenant.quotas?.pools,
        leases: tenant.quotas?.leases,
        staticBindings: tenant.quotas?.staticBindings,
      })
    } else {
      setEditingTenant(null)
      tenantForm.resetFields()
      tenantForm.setFieldsValue({ status: 'active' })
    }
  }

  function closeTenantDrawer() {
    setTenantDrawerOpen(false)
    setEditingTenant(null)
    tenantForm.resetFields()
  }

  function openRoleDrawer(role?: RbacRoleSummary) {
    setRoleDrawerOpen(true)
    if (role) {
      setEditingRole(role)
      roleForm.setFieldsValue({
        name: role.name,
        description: role.description,
        permissions: role.permissions,
      })
    } else {
      setEditingRole(null)
      roleForm.resetFields()
      roleForm.setFieldsValue({ permissions: 0 })
    }
  }

  function closeRoleDrawer() {
    setRoleDrawerOpen(false)
    setEditingRole(null)
    roleForm.resetFields()
  }

  async function handleTenantSubmit() {
    const values = await tenantForm.validateFields()
    const payload: TenantPayload = {
      name: values.name,
      contact: values.contact,
      description: values.description,
      status: values.status,
      quotas: {
        pools: values.pools,
        leases: values.leases,
        staticBindings: values.staticBindings,
      },
    }
    if (editingTenant) {
      updateTenantMutation.mutate({ tenantId: editingTenant.id, payload })
      return
    }
    createTenantMutation.mutate(payload)
  }

  async function handleCreateApiKey() {
    const values = await apiKeyForm.validateFields()
    createKeyMutation.mutate(values)
  }

  async function handleRoleSubmit() {
    const values = await roleForm.validateFields()
    const payload: RbacRolePayload = {
      name: values.name,
      description: values.description,
      permissions: values.permissions,
    }
    if (editingRole) {
      updateRoleMutation.mutate({ roleId: editingRole.id, payload })
      return
    }
    createRoleMutation.mutate(payload)
  }

  function closeApiKeyModal() {
    setApiKeyModalOpen(false)
    setIssuedToken(null)
    apiKeyForm.resetFields()
  }

  return (
    <>
      <Row gutter={[24, 24]}>
        <Col xs={24} md={6}>
          <Card className="glass-panel" loading={tenantsLoading}>
            <Statistic
              title={t('systemTenantsTotalStatistic')}
              value={totalTenants}
              valueStyle={{ color: '#5d5bf3' }}
            />
            <Statistic title={t('systemTenantsActiveStatistic')} value={activeTenants} style={{ marginTop: 16 }} />
          </Card>
        </Col>
        <Col xs={24} md={6}>
          <Card className="glass-panel" loading={apiKeysLoading}>
            <Statistic title={t('systemApiKeysTotalStatistic')} value={totalApiKeys} />
            <Button
              type="link"
              icon={<KeyOutlined />}
              style={{ paddingLeft: 0, marginTop: 16 }}
              onClick={() => setApiKeyModalOpen(true)}
            >
              {t('systemNewApiKeyButton')}
            </Button>
          </Card>
        </Col>
        <Col xs={24} md={6}>
          <Card className="glass-panel" loading={sessionsLoading}>
            <Statistic title={t('systemSessionsActiveStatistic')} value={activeSessionsCount} />
            <Typography.Text type="secondary">{t('systemSessionsAutoRefresh')}</Typography.Text>
          </Card>
        </Col>
        <Col xs={24} md={6}>
          <Card className="glass-panel" loading={rolesLoading}>
            <Statistic title={t('systemRolesTotalStatistic')} value={roleCount} />
            <Button type="link" style={{ paddingLeft: 0, marginTop: 16 }} onClick={() => openRoleDrawer()}>
              {t('systemNewRoleButton')}
            </Button>
          </Card>
        </Col>
      </Row>

      <Row gutter={[24, 24]} style={{ marginTop: 24 }}>
        <Col span={24}>
          <Card
            className="glass-panel"
            title={t('systemTenantsSectionTitle')}
            extra={
              <Button type="primary" icon={<PlusOutlined />} onClick={() => openTenantDrawer()}>
                {t('systemNewTenantButton')}
              </Button>
            }
          >
            <Table<TenantSummary>
              rowKey="id"
              loading={tenantsLoading}
              dataSource={tenants}
              columns={tenantColumns}
              pagination={{
                pageSize: 8,
                showTotal: (total) => t('systemTenantTableTotal', { count: total }),
              }}
            />
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card className="glass-panel" title={t('systemApiKeysSectionTitle')} loading={apiKeysLoading}>
            <Input.Search
              placeholder={t('systemSearchApiKeysPlaceholder')}
              allowClear
              value={apiKeyFilter}
              onChange={(event) => setApiKeyFilter(event.target.value)}
              onSearch={(value) => setApiKeyFilter(value)}
              style={{ marginBottom: 16 }}
            />
            <List<AccessApiKey>
              dataSource={filteredApiKeys}
              locale={{ emptyText: t('systemApiKeysEmpty') }}
              renderItem={(key) => (
                <List.Item
                  actions={[
                    <Popconfirm
                      key="revoke"
                      title={t('systemApiKeyRevokeConfirm')}
                      okText={t('systemApiKeyRevokeButton')}
                      cancelText={t('commonCancel')}
                      okButtonProps={{ danger: true, loading: revokeKeyMutation.isPending }}
                      onConfirm={() => revokeKeyMutation.mutate(key.id)}
                    >
                      <Button type="link" danger loading={revokeKeyMutation.isPending}>
                        {t('systemApiKeyRevokeButton')}
                      </Button>
                    </Popconfirm>,
                  ]}
                >
                  <List.Item.Meta
                    title={key.label}
                    description={
                      <Space direction="vertical" size={0}>
                        <Typography.Text type="secondary">
                          {t('systemScopeLabel')} {key.scope}{' '}
                          <Typography.Text copyable={{ text: key.scope }} style={{ marginLeft: 8 }}>
                            {t('systemScopeCopyLabel')}
                          </Typography.Text>
                        </Typography.Text>
                        <Typography.Text type="secondary">
                          {t('systemCreatedAtLabel', { timestamp: dayjs(key.createdAt).format('MM-DD HH:mm') })}
                        </Typography.Text>
                      </Space>
                    }
                  />
                  <div style={{ textAlign: 'right' }}>
                    <Tag color={key.lastUsedAt ? 'green' : 'default'}>
                      {key.lastUsedAt
                        ? t('systemLastUsedLabel', { timestamp: dayjs(key.lastUsedAt).format('MM-DD HH:mm') })
                        : t('commonNeverUsed')}
                    </Tag>
                    {key.expiresAt && (
                      <Typography.Text type="secondary" style={{ display: 'block' }}>
                        {t('systemExpiresAtLabel', {
                          timestamp: dayjs(key.expiresAt).format('MM-DD HH:mm'),
                        })}
                      </Typography.Text>
                    )}
                  </div>
                </List.Item>
              )}
            />
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card className="glass-panel" title={t('systemSessionsSectionTitle')} loading={sessionsLoading}>
            <List<SessionSummary>
              dataSource={sessions ?? []}
              locale={{ emptyText: t('systemSessionsEmpty') }}
              renderItem={(session) => (
                <List.Item
                  actions={
                    session.active
                      ? [
                          <Popconfirm
                            key="terminate"
                            title={t('systemSessionTerminateConfirm')}
                            okText={t('systemSessionForceLogout')}
                            cancelText={t('commonCancel')}
                            okButtonProps={{ danger: true, loading: terminateSessionMutation.isPending }}
                            onConfirm={() => terminateSessionMutation.mutate(session.id)}
                          >
                            <Button type="link" danger loading={terminateSessionMutation.isPending}>
                              {t('systemSessionForceLogout')}
                            </Button>
                          </Popconfirm>,
                        ]
                      : []
                  }
                >
                  <List.Item.Meta
                    title={`${session.user} · ${session.tenant}`}
                    description={`${dayjs(session.issuedAt).format('MM-DD HH:mm')} → ${dayjs(session.expiresAt).format('MM-DD HH:mm')}`}
                  />
                  <Tag color={session.active ? 'processing' : 'default'}>
                    {session.active ? t('systemSessionActiveTag') : t('systemSessionExpiredTag')}
                  </Tag>
                </List.Item>
              )}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[24, 24]}>
        <Col span={24}>
          <Card
            className="glass-panel"
            title={t('systemRolesSectionTitle')}
            extra={
              <Button type="primary" onClick={() => openRoleDrawer()}>
                {t('systemNewRoleButton')}
              </Button>
            }
            loading={rolesLoading}
          >
            <Table<RbacRoleSummary>
              rowKey="id"
              dataSource={roles ?? []}
              pagination={{
                pageSize: 10,
                showTotal: (total) => t('systemRoleTableTotal', { count: total }),
              }}
              columns={roleColumns}
            />
          </Card>
        </Col>
      </Row>

      <Drawer
        title={
          editingTenant
            ? t('systemTenantDrawerEditTitle', { name: editingTenant.name })
            : t('systemTenantDrawerCreateTitle')
        }
        width={520}
        open={tenantDrawerOpen}
        onClose={closeTenantDrawer}
        destroyOnClose
      >
        <Form layout="vertical" form={tenantForm} initialValues={{ status: 'active' }}>
          <Form.Item
            label={t('systemFieldName')}
            name="name"
            rules={[{ required: true, message: t('systemTenantNameRequired') }]}
          >
            <Input placeholder={t('systemTenantNamePlaceholder')} allowClear />
          </Form.Item>
          <Form.Item label={t('systemFieldContact')} name="contact">
            <Input placeholder={t('systemTenantContactPlaceholder')} allowClear />
          </Form.Item>
          <Form.Item label={t('systemFieldStatus')} name="status">
            <Select
              options={[
                { label: t('systemStatusActiveOption'), value: 'active' },
                { label: t('systemStatusSuspendedOption'), value: 'suspended' },
              ]}
            />
          </Form.Item>
          <Form.Item label={t('systemFieldDescription')} name="description">
            <Input.TextArea rows={3} placeholder={t('systemTenantDescriptionPlaceholder')} />
          </Form.Item>
          <Typography.Title level={5}>{t('systemQuotaTitle')}</Typography.Title>
          <Row gutter={16}>
            <Col span={8}>
              <Form.Item label={t('systemFieldPools')} name="pools">
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item label={t('systemFieldLeases')} name="leases">
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item label={t('systemFieldStaticBindings')} name="staticBindings">
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item>
            <Space>
              <Button onClick={closeTenantDrawer}>{t('commonCancel')}</Button>
              <Button
                type="primary"
                onClick={handleTenantSubmit}
                loading={createTenantMutation.isPending || updateTenantMutation.isPending}
              >
                {t('commonSave')}
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Drawer>

      <Modal
        title={t('systemApiKeyModalTitle')}
        open={apiKeyModalOpen}
        onCancel={closeApiKeyModal}
        onOk={handleCreateApiKey}
        okText={t('commonCreate')}
        confirmLoading={createKeyMutation.isPending}
      >
        <Form layout="vertical" form={apiKeyForm}>
          <Form.Item
            label={t('systemFieldName')}
            name="label"
            rules={[{ required: true, message: t('systemApiKeyNameRequired') }]}
          >
            <Input placeholder={t('systemApiKeyNamePlaceholder')} allowClear />
          </Form.Item>
          <Form.Item
            label={t('systemApiKeyScopeLabel')}
            name="scope"
            rules={[{ required: true, message: t('systemApiKeyScopeRequired') }]}
            tooltip={t('systemApiKeyScopeTooltip')}
          >
            <Input allowClear />
          </Form.Item>
          <Form.Item label={t('systemApiKeyExpiresLabel')} name="expiresInHours">
            <InputNumber min={1} max={720} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
        {issuedToken && (
          <Alert
            style={{ marginTop: 16 }}
            type="success"
            showIcon
            message={t('systemApiKeyAlertTitle')}
            description={
              <Space direction="vertical" style={{ width: '100%' }}>
                <Typography.Text copyable={{ text: issuedToken }}>{issuedToken}</Typography.Text>
                <Typography.Text type="secondary">{t('systemApiKeyAlertDescription')}</Typography.Text>
              </Space>
            }
          />
        )}
      </Modal>

      <Drawer
        title={
          editingRole
            ? t('systemRoleDrawerEditTitle', { name: editingRole.name })
            : t('systemRoleDrawerCreateTitle')
        }
        width={420}
        open={roleDrawerOpen}
        onClose={closeRoleDrawer}
        destroyOnClose
      >
        <Form layout="vertical" form={roleForm}>
          <Form.Item
            label={t('systemRoleNameLabel')}
            name="name"
            rules={[{ required: true, message: t('systemRoleNameRequired') }]}
          >
            <Input placeholder={t('systemRoleNamePlaceholder')} allowClear />
          </Form.Item>
          <Form.Item label={t('systemRoleDescriptionLabel')} name="description">
            <Input.TextArea rows={3} placeholder={t('systemRoleDescriptionPlaceholder')} />
          </Form.Item>
          <Form.Item
            label={t('systemRolePermissionsLabel')}
            name="permissions"
            rules={[{ required: true, message: t('systemRolePermissionsRequired') }]}
          >
            <InputNumber min={0} max={1024} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item>
            <Space>
              <Button onClick={closeRoleDrawer}>{t('commonCancel')}</Button>
              <Button
                type="primary"
                onClick={handleRoleSubmit}
                loading={createRoleMutation.isPending || updateRoleMutation.isPending}
              >
                {t('commonSave')}
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Drawer>
    </>
  )
}
