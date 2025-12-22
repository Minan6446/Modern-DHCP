import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Card, Col, Row, Statistic, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { fetchStaticBindingSummary } from '../services/features'
import type { StaticBindingRecord, StaticBindingSummary } from '../types/api'

export default function StaticBindingsPage() {
  const { data, isLoading } = useQuery<StaticBindingSummary>({
    queryKey: ['static-binding-summary'],
    queryFn: fetchStaticBindingSummary,
    staleTime: 60_000,
  })

  const columns = useMemo<ColumnsType<StaticBindingRecord>>(
    () => [
      { title: '设备', dataIndex: 'device' },
      { title: '租户', dataIndex: 'tenantId' },
      { title: 'MAC', dataIndex: 'macAddress' },
      { title: '固定 IP', dataIndex: 'ipAddress' },
      { title: '主机名', dataIndex: 'hostname' },
      {
        title: '状态',
        dataIndex: 'status',
        render: (value: StaticBindingRecord['status']) => (
          <Tag color={value === 'online' ? 'green' : value === 'offline' ? 'default' : 'blue'}>{value.toUpperCase()}</Tag>
        ),
      },
      { title: '最近在线', dataIndex: 'lastSeenAt' },
    ],
    [],
  )

  return (
    <Row gutter={[24, 24]}>
      <Col xs={24} md={6}>
        <Card loading={isLoading} className="glass-panel">
          <Statistic title="静态绑定总数" value={data?.total ?? 0} />
        </Card>
      </Col>
      <Col xs={24} md={6}>
        <Card loading={isLoading} className="glass-panel">
          <Statistic title="在线设备" value={data?.active ?? 0} />
        </Card>
      </Col>
      <Col xs={24} md={6}>
        <Card loading={isLoading} className="glass-panel">
          <Statistic title="待导入任务" value={data?.pendingImports ?? 0} />
        </Card>
      </Col>
      <Col span={24}>
        <Card title="静态绑定列表" loading={isLoading} className="glass-panel">
          <Typography.Paragraph type="secondary" style={{ marginBottom: 16 }}>
            模拟数据来自 mock service，后端接口准备就绪后可直接替换。
          </Typography.Paragraph>
          <Table
            rowKey="id"
            columns={columns}
            dataSource={data?.bindings ?? []}
            pagination={{ pageSize: 8, showTotal: (total) => `共 ${total} 条` }}
          />
        </Card>
      </Col>
    </Row>
  )
}
