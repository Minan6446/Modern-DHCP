import { CompassOutlined, PlusOutlined } from '@ant-design/icons'
import { Alert, Breadcrumb, FloatButton, Grid, Layout, Menu, Space, Tag, Typography } from 'antd'
import type { MenuProps } from 'antd'
import { useMemo, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { navigationItems } from '../../router'
import { useCapabilities } from '../../hooks/useCapabilities'
import { useI18n } from '../../hooks/useI18n'
import HeaderActions from './HeaderActions'

const { Header, Sider, Content } = Layout

export default function AppLayout() {
  const location = useLocation()
  const navigate = useNavigate()
  const screens = Grid.useBreakpoint()
  const [collapsed, setCollapsed] = useState(false)
  const [temporaryNoticeDismissed, setTemporaryNoticeDismissed] = useState(false)
  const { data: metadata } = useCapabilities()
  const { t } = useI18n()
  const temporaryCaps = metadata?.temporary ?? []

  const grantedSet = useMemo(() => {
    const base = new Set<string>()
    ;[...(metadata?.granted ?? []), ...(metadata?.temporary ?? [])].forEach((cap) => base.add(cap))
    return base
  }, [metadata?.granted, metadata?.temporary])

  const showTemporaryNotice = temporaryCaps.length > 0 && !temporaryNoticeDismissed

  const visibleNavItems = useMemo(() => {
    const hasMetadata = Boolean(metadata)
    return navigationItems.filter(
      (item) => !item.capability || !hasMetadata || grantedSet.has(item.capability),
    )
  }, [grantedSet, metadata])

  const selectedKey = useMemo(() => {
    const match = navigationItems.find((item) =>
      location.pathname.startsWith(`/app/${item.path}`),
    )
    return match ? `/app/${match.path}` : undefined
  }, [location.pathname])

  const menuItems = useMemo<MenuProps['items']>(
    () =>
      visibleNavItems.map((item) => ({
        key: `/app/${item.path}`,
        icon: item.icon,
        label: item.label,
      })),
    [visibleNavItems],
  )

  const pageMeta = navigationItems.find((item) => `/app/${item.path}` === selectedKey)

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        collapsible
        breakpoint="lg"
        width={256}
        collapsedWidth={screens.lg ? 80 : 0}
        style={{ borderRight: '1px solid rgba(255,255,255,0.08)' }}
        collapsed={screens.lg ? collapsed : true}
        onCollapse={(value) => setCollapsed(value)}
      >
        <div className="app-shell__brand">
          <span className="app-shell__brand-mark">DH</span>
          {!screens.lg ? null : <span>Modern DHCP</span>}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          items={menuItems}
          selectedKeys={selectedKey ? [selectedKey] : []}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>
      <Layout>
        <Header
          style={{
            padding: '0 32px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            backdropFilter: 'blur(12px)',
          }}
        >
          <div>
            <Typography.Title level={3} style={{ margin: 0, color: '#f8f9ff' }}>
              {pageMeta?.label ?? t('controlPanel')}
            </Typography.Title>
            <Typography.Text type="secondary" style={{ color: 'rgba(248,249,255,0.65)' }}>
              {pageMeta?.description ?? t('overview')}
            </Typography.Text>
            {temporaryCaps.length > 0 && (
              <Tag color="gold" style={{ marginTop: 8 }}>
                {t('temporaryAccess', { count: temporaryCaps.length })}
              </Tag>
            )}
          </div>
          <HeaderActions />
        </Header>
        <Content className="app-content">
          {showTemporaryNotice && (
            <Alert
              type="warning"
              showIcon
              closable
              onClose={() => setTemporaryNoticeDismissed(true)}
              message={t('temporaryBannerTitle')}
              description={
                <Space direction="vertical" size={8} style={{ width: '100%' }}>
                  <Typography.Text type="secondary">
                    {t('temporaryBannerDescription')}
                  </Typography.Text>
                  <Space wrap size={6}>
                    {temporaryCaps.map((cap) => (
                      <Tag key={cap} color="gold">
                        {cap}
                      </Tag>
                    ))}
                  </Space>
                </Space>
              }
              style={{ marginBottom: 18 }}
            />
          )}
          <Breadcrumb
            style={{ marginBottom: 18 }}
            items={[
              { title: t('breadcrumbRoot') },
              { title: pageMeta?.label ?? t('breadcrumbFallback') },
            ]}
          />
          <Outlet />
        </Content>
      </Layout>
      <FloatButton.Group trigger="hover" type="primary" style={{ right: 32 }} icon={<PlusOutlined />}>
        <FloatButton tooltip="快速创建地址池" icon={<CompassOutlined />} onClick={() => navigate('/app/pools')} />
        <FloatButton tooltip="发布自动化作业" icon={<PlusOutlined />} onClick={() => navigate('/app/automation')} />
      </FloatButton.Group>
    </Layout>
  )
}
