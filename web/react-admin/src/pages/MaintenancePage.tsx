import { useQuery } from '@tanstack/react-query'
import { Card, Col, List, Progress, Row, Statistic, Tag, Typography } from 'antd'
import { fetchMaintenanceOverview } from '../services/features'
import type { MaintenanceOverview } from '../types/api'

export default function MaintenancePage() {
  const { data, isLoading } = useQuery<MaintenanceOverview>({
    queryKey: ['maintenance-overview'],
    queryFn: fetchMaintenanceOverview,
    staleTime: 120_000,
  })

  return (
    <Row gutter={[24, 24]}>
      <Col xs={24} md={6}>
        <Card loading={isLoading} className="glass-panel">
          <Statistic title="备份窗口" value={data?.backupWindow ?? '--'} />
          <Tag color={data?.backupsEnabled ? 'green' : 'red'} style={{ marginTop: 12 }}>
            {data?.backupsEnabled ? '已启用' : '未启用'}
          </Tag>
        </Card>
      </Col>
      <Col xs={24} md={6}>
        <Card loading={isLoading} className="glass-panel">
          <Statistic title="最近备份" value={data?.lastBackupAt ?? '--'} />
        </Card>
      </Col>
      <Col xs={24} md={12}>
        <Card title="容量预测" loading={isLoading} className="glass-panel">
          <List
            dataSource={data?.forecasts ?? []}
            renderItem={(item) => (
              <List.Item>
                <List.Item.Meta title={item.metric} description={`当前 ${item.current}% · 上限 ${item.limit}%`} />
                <div style={{ minWidth: 160 }}>
                  <Typography.Text type="secondary">30 天预测 {item.projection30d}%</Typography.Text>
                  <Progress percent={Math.min(100, Math.round((item.projection30d / item.limit) * 100))} size="small" showInfo={false} />
                </div>
              </List.Item>
            )}
          />
        </Card>
      </Col>
      <Col span={24}>
        <Card title="维护任务" loading={isLoading} className="glass-panel">
          <List
            dataSource={data?.tasks ?? []}
            renderItem={(task) => (
              <List.Item>
                <List.Item.Meta title={task.title} description={`${task.window} · Owner ${task.owner}`} />
                <Tag color={task.status === 'completed' ? 'green' : task.status === 'running' ? 'orange' : 'blue'}>
                  {task.status.toUpperCase()}
                </Tag>
              </List.Item>
            )}
          />
        </Card>
      </Col>
      <Col span={24}>
        <Card className="glass-panel">
          <Typography.Text type="secondary">
            维护 & 备份数据暂由 mock service 提供；未来可接入 `/maintenance/*` 与 `/backups/*` 真实端点。
          </Typography.Text>
        </Card>
      </Col>
    </Row>
  )
}
