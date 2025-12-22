import { useQuery } from '@tanstack/react-query'
import {
  Button,
  Card,
  Col,
  Descriptions,
  Drawer,
  Empty,
  Input,
  List,
  Progress,
  Row,
  Segmented,
  Select,
  Statistic,
  Table,
  Tag,
  Typography,
  type TableColumnsType,
} from 'antd'
import { useMemo, useState } from 'react'
import { fetchMonitoredPools } from '../services/monitoring'
import { listPools } from '../services/pools'
import type { MonitoringPoolsResponse, PoolSummary, PoolUsageSummary } from '../types/api'
import { useSessionStore } from '../store/session'

const statusMap: Record<PoolSummary['status'], { label: string; color: string }> = {
  ready: { label: '稳定', color: 'success' },
  warning: { label: '警告', color: 'warning' },
  critical: { label: '告急', color: 'error' },
}

export default function PoolsPage() {
  const activeTenantId = useSessionStore((state) => state.activeTenantId)
  const tenantKey = activeTenantId ?? 'tenant-default'
  const [searchInput, setSearchInput] = useState('')
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState<'all' | 'warning' | 'critical'>('all')
  const [roleFilter, setRoleFilter] = useState<string | undefined>(undefined)
  const [selectedPoolId, setSelectedPoolId] = useState<string | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['pools', tenantKey, search, filter, roleFilter],
    queryFn: () =>
      listPools({
        query: search,
        status: filter === 'all' ? undefined : filter,
        role: roleFilter,
      }),
  })

  const { data: monitoredPools, isLoading: monitoredLoading } = useQuery<MonitoringPoolsResponse>({
    queryKey: ['monitoring-pools', 8],
    queryFn: () => fetchMonitoredPools(8),
    staleTime: 60_000,
  })

  const summary = useMemo(() => {
    if (!data?.items?.length) {
      return { total: 0, warning: 0, critical: 0 }
    }
    return data.items.reduce(
      (acc, pool) => {
        acc.total += 1
        if (pool.status === 'warning') acc.warning += 1
        if (pool.status === 'critical') acc.critical += 1
        return acc
      },
      { total: 0, warning: 0, critical: 0 },
    )
  }, [data])

  const roleOptions = useMemo(() => {
    const roles = new Set<string>()
    data?.items?.forEach((pool) => roles.add(pool.role))
    return Array.from(roles)
  }, [data])

  const poolUsageMap = useMemo(() => {
    const map = new Map<string, PoolUsageSummary>()
    monitoredPools?.pools?.forEach((pool) => map.set(pool.poolId, pool))
    return map
  }, [monitoredPools])

  const poolHotspots = useMemo(() => {
    if (!monitoredPools?.pools?.length) return []
    return [...monitoredPools.pools].sort((a, b) => b.utilization - a.utilization).slice(0, 5)
  }, [monitoredPools])

  const selectedPool = data?.items?.find((pool) => pool.id === selectedPoolId)
  const selectedUsage = selectedPool ? poolUsageMap.get(selectedPool.id) : undefined

  const columns: TableColumnsType<PoolSummary> = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: 'CIDR', dataIndex: 'cidr', key: 'cidr' },
    { title: '角色', dataIndex: 'role', key: 'role' },
    {
      title: '容量',
      dataIndex: 'capacity',
      key: 'capacity',
      render: (_, record) => `${record.capacity.toLocaleString()} 地址`,
    },
    {
      title: '利用率',
      dataIndex: 'utilization',
      key: 'utilization',
      render: (value: number) => (
        <Progress percent={Math.min(100, Math.round(value ?? 0))} size="small" />
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (value: PoolSummary['status']) => (
        <Tag color={statusMap[value].color}>{statusMap[value].label}</Tag>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      render: (_, record) => (
        <Button
          type="link"
          onClick={(event) => {
            event.stopPropagation()
            setSelectedPoolId(record.id)
          }}
        >
          查看详情
        </Button>
      ),
    },
  ]

  return (
    <Row gutter={[24, 24]}>
      <Col xs={24} lg={10}>
        <Card className="glass-panel">
          <Statistic title="地址池总数" value={summary.total} />
          <Statistic title="预警" value={summary.warning} style={{ marginTop: 16 }} valueStyle={{ color: '#faad14' }} />
          <Statistic title="告急" value={summary.critical} style={{ marginTop: 16 }} valueStyle={{ color: '#ff4d4f' }} />
        </Card>
      </Col>
      <Col xs={24} lg={14}>
        <Card
          className="glass-panel"
          title="利用率热点"
          loading={monitoredLoading}
          extra={
            monitoredPools?.generatedAt && (
              <Typography.Text type="secondary">
                更新 {new Date(monitoredPools.generatedAt).toLocaleTimeString()}
              </Typography.Text>
            )
          }
        >
          <List
            dataSource={poolHotspots}
            locale={{ emptyText: '暂无监控数据' }}
            renderItem={(pool) => (
              <List.Item
                actions={[
                  <Button
                    type="link"
                    onClick={(event) => {
                      event.stopPropagation()
                      setSelectedPoolId(pool.poolId)
                    }}
                    key="view"
                  >
                    详情
                  </Button>,
                ]}
              >
                <List.Item.Meta
                  title={pool.name}
                  description={`Scope ${pool.scope ?? '全局'} · VLAN ${pool.vlanId ?? '—'} · ${pool.location ?? '未标注'}`}
                />
                <div style={{ minWidth: 180 }}>
                  <Typography.Text type="secondary">利用率 {pool.utilization.toFixed(1)}%</Typography.Text>
                  <Progress percent={Math.round(pool.utilization)} showInfo={false} size="small" />
                  <Typography.Text type="secondary">
                    {pool.allocated.toLocaleString()} / {pool.capacity.toLocaleString()}
                  </Typography.Text>
                </div>
              </List.Item>
            )}
          />
        </Card>
      </Col>
      <Col span={24}>
        <Card className="glass-panel" title="分层地址池">
          <Row justify="space-between" style={{ marginBottom: 16 }} gutter={16}>
            <Col xs={24} md={12}>
              <Input.Search
                placeholder="搜索地址池"
                allowClear
                value={searchInput}
                onChange={(event) => {
                  const value = event.target.value
                  setSearchInput(value)
                  if (!value) setSearch('')
                }}
                onSearch={(value) => {
                  setSearchInput(value)
                  setSearch(value.trim())
                }}
              />
            </Col>
            <Col>
              <Segmented
                options={[
                  { label: '全部', value: 'all' },
                  { label: '警告', value: 'warning' },
                  { label: '告急', value: 'critical' },
                ]}
                value={filter}
                onChange={(value) => setFilter(value as typeof filter)}
              />
            </Col>
            <Col>
              <Select
                allowClear
                placeholder="角色"
                style={{ minWidth: 160 }}
                value={roleFilter}
                onChange={(value) => setRoleFilter(value ?? undefined)}
                options={roleOptions.map((role) => ({ label: role, value: role }))}
              />
            </Col>
          </Row>
          <Table<PoolSummary>
            rowKey="id"
            loading={isLoading}
            dataSource={data?.items ?? []}
            columns={columns}
            pagination={{ pageSize: 10, showSizeChanger: false, total: data?.total }}
            onRow={(record) => ({
              onClick: () => setSelectedPoolId(record.id),
            })}
          />
        </Card>
      </Col>

      <Drawer
        title="地址池详情"
        width={480}
        open={Boolean(selectedPoolId)}
        onClose={() => setSelectedPoolId(null)}
        destroyOnClose
      >
        {selectedPool ? (
          <>
            <Typography.Title level={4} style={{ marginBottom: 8 }}>
              {selectedPool.name}
            </Typography.Title>
            <Typography.Text type="secondary">{selectedPool.cidr}</Typography.Text>
            <div style={{ marginTop: 16 }}>
              <Progress percent={Math.round(selectedUsage?.utilization ?? selectedPool.utilization)} />
              <Typography.Text type="secondary">
                利用率参考监控快照，供容量决策使用。
              </Typography.Text>
            </div>
            <Descriptions column={2} style={{ marginTop: 24 }}>
              <Descriptions.Item label="角色">{selectedPool.role}</Descriptions.Item>
              <Descriptions.Item label="状态">
                <Tag color={statusMap[selectedPool.status].color}>
                  {statusMap[selectedPool.status].label}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="容量">
                {selectedPool.capacity.toLocaleString()} 地址
              </Descriptions.Item>
              <Descriptions.Item label="利用率">
                {selectedPool.utilization.toFixed(1)}%
              </Descriptions.Item>
            </Descriptions>
            {selectedUsage && (
              <Descriptions column={2} style={{ marginTop: 16 }} title="监控属性">
                <Descriptions.Item label="Scope">{selectedUsage.scope ?? '全局'}</Descriptions.Item>
                <Descriptions.Item label="VLAN">{selectedUsage.vlanId ?? '—'}</Descriptions.Item>
                <Descriptions.Item label="位置">{selectedUsage.location ?? '未标注'}</Descriptions.Item>
                <Descriptions.Item label="已用 / 容量">
                  {selectedUsage.allocated.toLocaleString()} / {selectedUsage.capacity.toLocaleString()}
                </Descriptions.Item>
              </Descriptions>
            )}
            <Typography.Paragraph type="secondary" style={{ marginTop: 16 }}>
              更详细的策略、静态绑定与监控图表将在后续迭代接入对应 API。
            </Typography.Paragraph>
          </>
        ) : (
          <Empty description="选择一个地址池查看详情" />
        )}
      </Drawer>
    </Row>
  )
}
