import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Card, Col, List, Row, Statistic, Tag, Typography } from 'antd'
import { fetchIntegrationMatrix } from '../services/features'
import type { IntegrationAdapter, IntegrationMatrix } from '../types/api'

function typeLabel(type: IntegrationAdapter['type']) {
  switch (type) {
    case 'monitoring':
      return '监控'
    case 'cmdb':
      return 'CMDB'
    case 'itsm':
      return 'ITSM'
    default:
      return 'Webhook'
  }
}

export default function IntegrationsPage() {
  const { data, isLoading } = useQuery<IntegrationMatrix>({
    queryKey: ['integration-matrix'],
    queryFn: fetchIntegrationMatrix,
    staleTime: 120_000,
  })

  const grouped = useMemo(() => {
    const entries = data?.adapters ?? []
    return {
      connected: entries.filter((a) => a.status === 'connected'),
      warning: entries.filter((a) => a.status === 'warning'),
      disconnected: entries.filter((a) => a.status === 'disconnected'),
    }
  }, [data?.adapters])

  return (
    <Row gutter={[24, 24]}>
      <Col xs={24} md={8}>
        <Card loading={isLoading} className="glass-panel">
          <Statistic title="Webhook 推送 (24h)" value={data?.webhookDeliveries24h ?? 0} />
        </Card>
      </Col>
      <Col xs={24} md={8}>
        <Card loading={isLoading} className="glass-panel">
          <Statistic title="API 调用 (24h)" value={data?.apiCalls24h ?? 0} />
        </Card>
      </Col>
      <Col xs={24} md={8}>
        <Card loading={isLoading} className="glass-panel">
          <Statistic title="连接中的适配器" value={grouped.connected.length} />
        </Card>
      </Col>
      {(['connected', 'warning', 'disconnected'] as const).map((category) => (
        <Col xs={24} lg={8} key={category}>
          <Card
            title={
              category === 'connected' ? '运行正常' : category === 'warning' ? '告警 / 需关注' : '未连接'
            }
            loading={isLoading}
            className="glass-panel"
          >
            <List
              dataSource={grouped[category]}
              locale={{ emptyText: '暂无' }}
              renderItem={(adapter) => (
                <List.Item>
                  <List.Item.Meta
                    title={adapter.name}
                    description={
                      <Typography.Text type="secondary">
                        {typeLabel(adapter.type)} · {adapter.endpoint}
                      </Typography.Text>
                    }
                  />
                  <Tag color={category === 'connected' ? 'green' : category === 'warning' ? 'gold' : 'default'}>
                    {adapter.lastSyncAt ? `上次同步 ${new Date(adapter.lastSyncAt).toLocaleString()}` : '尚未同步'}
                  </Tag>
                </List.Item>
              )}
            />
          </Card>
        </Col>
      ))}
      <Col span={24}>
        <Card className="glass-panel">
          <Typography.Text type="secondary">
            将来可在此页面配置 API 令牌、Webhook 模板与第三方系统映射。当前采用 mock service，避免导航空白。
          </Typography.Text>
        </Card>
      </Col>
    </Row>
  )
}
