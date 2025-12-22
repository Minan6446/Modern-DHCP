import { useQuery } from '@tanstack/react-query'
import { Card, Col, Descriptions, List, Progress, Row, Statistic, Tag, Typography } from 'antd'
import { fetchClusterOverview } from '../services/features'
import type { ClusterOverview, ClusterNodeStatus } from '../types/api'

function healthColor(health: ClusterNodeStatus['health']) {
  if (health === 'healthy') return 'green'
  if (health === 'warning') return 'gold'
  return 'red'
}

export default function ClusterPage() {
  const { data, isLoading } = useQuery<ClusterOverview>({
    queryKey: ['cluster-overview'],
    queryFn: fetchClusterOverview,
    staleTime: 60_000,
  })

  return (
    <Row gutter={[24, 24]}>
      <Col xs={24} md={8}>
        <Card loading={isLoading} className="glass-panel">
          <Statistic title="集群模式" value={data?.mode === 'active-active' ? '主动-主动' : '主动-备用'} />
          <Statistic
            title="复制延迟 (ms)"
            value={data?.replicationLagMs ?? 0}
            suffix="ms"
            style={{ marginTop: 16 }}
          />
          <Tag color={data?.failoverReady ? 'green' : 'red'} style={{ marginTop: 16 }}>
            {data?.failoverReady ? '故障切换就绪' : '检查复制状态'}
          </Tag>
        </Card>
      </Col>
      <Col xs={24} md={16}>
        <Card title="节点状态" loading={isLoading} className="glass-panel">
          <List
            dataSource={data?.nodes ?? []}
            renderItem={(node) => (
              <List.Item>
                <List.Item.Meta
                  title={
                    <Typography.Text strong>
                      {node.id} · {node.role.toUpperCase()}
                    </Typography.Text>
                  }
                  description={`版本 ${node.version} · 地址 ${node.address}`}
                />
                <div style={{ textAlign: 'right', minWidth: 180 }}>
                  <Tag color={healthColor(node.health)}>{node.health}</Tag>
                  <div>
                    <Typography.Text type="secondary">CPU {node.cpuPercent.toFixed(1)}%</Typography.Text>
                    <Progress percent={Math.round(node.cpuPercent)} size="small" showInfo={false} />
                  </div>
                  <div>
                    <Typography.Text type="secondary">内存 {node.memoryPercent.toFixed(1)}%</Typography.Text>
                    <Progress percent={Math.round(node.memoryPercent)} size="small" showInfo={false} />
                  </div>
                </div>
              </List.Item>
            )}
          />
        </Card>
      </Col>
      <Col span={24}>
        <Card title="运维提示" loading={isLoading} className="glass-panel">
          <Descriptions column={3} bordered size="small">
            <Descriptions.Item label="待处理操作">{data?.pendingActions ?? 0}</Descriptions.Item>
            <Descriptions.Item label="最大同步延迟">{data?.nodes?.reduce((max, node) => Math.max(max, node.syncLagMs), 0) ?? 0} ms</Descriptions.Item>
            <Descriptions.Item label="最后心跳">
              {data?.nodes?.map((node) => `${node.id}: ${node.lastHeartbeat}`).join(' / ') ?? '--'}
            </Descriptions.Item>
          </Descriptions>
          <Typography.Paragraph type="secondary" style={{ marginTop: 16 }}>
            当前页面使用 mock 数据源，后端集群 API 完成后替换 `fetchClusterOverview` 即可。
          </Typography.Paragraph>
        </Card>
      </Col>
    </Row>
  )
}
