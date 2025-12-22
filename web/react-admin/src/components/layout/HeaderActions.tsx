import { LogoutOutlined, SettingOutlined, UserOutlined } from '@ant-design/icons'
import { useQueryClient } from '@tanstack/react-query'
import { Avatar, Button, Dropdown, Flex, Input, Select, Space, message, type MenuProps } from 'antd'
import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useSessionStore } from '../../store/session'
import { useTenants } from '../../hooks/useTenants'
import PreferenceDrawer from './PreferenceDrawer'
import { useI18n } from '../../hooks/useI18n'

const { Search } = Input

export default function HeaderActions() {
  const profile = useSessionStore((state) => state.profile)
  const clearSession = useSessionStore((state) => state.clearSession)
  const activeTenantId = useSessionStore((state) => state.activeTenantId)
  const tenants = useSessionStore((state) => state.availableTenants)
  const setActiveTenant = useSessionStore((state) => state.setActiveTenant)
  const navigate = useNavigate()
  const { isLoading: tenantsLoading } = useTenants()
  const queryClient = useQueryClient()
  const [preferencesOpen, setPreferencesOpen] = useState(false)
  const { t } = useI18n()

  const tenantOptions = useMemo(
    () => tenants.map((tenant) => ({ label: tenant.name, value: tenant.id })),
    [tenants],
  )

  const menuItems = useMemo<MenuProps['items']>(() => {
    return [
      {
        key: 'profile',
        icon: <UserOutlined />,
        label: profile?.displayName ?? t('anonymousUser'),
        disabled: true,
      },
      { type: 'divider' },
      {
        key: 'settings',
        icon: <SettingOutlined />,
        label: t('profileLabel'),
        onClick: () => navigate('/app/settings'),
      },
      {
        key: 'logout',
        icon: <LogoutOutlined />,
        label: t('logoutLabel'),
        danger: true,
        onClick: () => {
          clearSession()
          navigate('/auth/login')
        },
      },
    ]
  }, [clearSession, navigate, profile?.displayName, t])

  return (
    <Flex gap={16} align="center">
      <Select
        showSearch
        optionFilterProp="label"
        placeholder={t('selectTenantPlaceholder')}
        loading={tenantsLoading}
        value={activeTenantId ?? undefined}
        style={{ width: 220 }}
        options={tenantOptions}
        onChange={(value) => {
          setActiveTenant(value)
          queryClient.invalidateQueries()
          message.success(t('tenantSwitched'))
        }}
        allowClear={false}
      />
      <Search placeholder={t('searchPlaceholder')} allowClear style={{ width: 320 }} />
      <Button type="default" ghost onClick={() => setPreferencesOpen(true)}>
        <Space size={4}>
          <SettingOutlined />
          <span>{t('appearanceLanguage')}</span>
        </Space>
      </Button>
      <Dropdown menu={{ items: menuItems }} placement="bottomRight" trigger={['click']}>
        <Avatar src={profile?.avatarUrl} style={{ background: '#5d5bf3' }}>
          {profile?.displayName?.[0] ?? 'A'}
        </Avatar>
      </Dropdown>
      <PreferenceDrawer open={preferencesOpen} onClose={() => setPreferencesOpen(false)} />
    </Flex>
  )
}
