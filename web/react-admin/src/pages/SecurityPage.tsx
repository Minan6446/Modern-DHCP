import { useQuery } from '@tanstack/react-query'
import { Card, Col, List, Row, Statistic, Tag, Typography } from 'antd'
import { fetchSecurityOverview } from '../services/features'
import type { AlertFeedEntry, SecurityOverview } from '../types/api'

function controlStatusTag(status: string) {
  switch (status) {
    case 'enabled':
      return <Tag color="green">已启用</Tag>
    case 'degraded':
      return <Tag color="gold">降级</Tag>
    default:
      return <Tag>未启用</Tag>
  }
}

const severityColorMap = {
  info: 'cyan',
  warning: 'orange',
  critical: 'red',
} as const

export default function SecurityPage() {
  const { data, isLoading } = useQuery<SecurityOverview>({
    queryKey: ['security-overview'],
    queryFn: fetchSecurityOverview,
    refetchInterval: 60_000,
  })

  return (
    <Row gutter={[24, 24]}>
      <Col xs={24} md={8}>
        <Card loading={isLoading} className="glass-panel">
          <Statistic title="速率限制命中" value={data?.rateLimitHits ?? 0} />
        </Card>
      </Col>
      <Col xs={24} md={8}>
        <Card loading={isLoading} className="glass-panel">
          <Statistic title="Snooping 违规" value={data?.snoopingViolations ?? 0} />
        </Card>
      </Col>
      <Col xs={24} md={8}>
        <Card loading={isLoading} className="glass-panel">
          <Statistic title="疑似非法服务器" value={data?.rogueServers ?? 0} />
        </Card>
      </Col>
      <Col xs={24} lg={12}>
        <Card title="安全控制状态" loading={isLoading} className="glass-panel">
          <List
            dataSource={data?.controls ?? []}
            renderItem={(control) => (
              <List.Item>
                <List.Item.Meta title={control.name} description={`上次事件：${control.lastEventAt ?? '未知'}`} />
                {controlStatusTag(control.status)}
              </List.Item>
            )}
          />
        </Card>
      </Col>
      <Col xs={24} lg={12}>
        <Card title="最新告警" loading={isLoading} className="glass-panel">
          <List
            dataSource={data?.recentFindings ?? []}
            renderItem={(alert: AlertFeedEntry) => (
              <List.Item>
                <List.Item.Meta
                  title={
                    <Typography.Text>
                      {alert.summary}
                      <Tag color={severityColorMap[alert.severity]} style={{ marginLeft: 8 }}>
                        {alert.severity.toUpperCase()}
                      </Tag>
                    </Typography.Text>
                  }
                  description={`${alert.tenantId} · ${alert.details}`}
                />
                <Tag color="blue">{alert.lifecycle}</Tag>
              </List.Item>
            )}
          />
        </Card>
      </Col>
      <Col span={24}>
        <Card className="glass-panel">
          <Typography.Text type="secondary">
            本页面使用 mock 安全态势数据，等待 `/security/*` 与 `/monitoring/alerts` 的后端端点落地后即可替换。
          </Typography.Text>
        </Card>
      </Col>
    </Row>
  )
}
