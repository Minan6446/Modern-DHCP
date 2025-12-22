import { useQuery } from '@tanstack/react-query'
import { Card, Col, List, Progress, Row, Segmented, Statistic, Tag, Typography } from 'antd'
import dayjs from 'dayjs'
import { useMemo, useState } from 'react'
import { fetchAlertFeed, fetchMonitoredPools } from '../services/monitoring'
import type { AlertFeedEntry, AlertSeverity } from '../types/api'

const severityColor: Record<AlertSeverity, string> = {
  info: 'processing',
  warning: 'warning',
  critical: 'error',
}

const lifecycleColor = {
  open: 'processing',
  acknowledged: 'success',
  suppressed: 'default',
} as const

export default function MonitoringPage() {
  const [filter, setFilter] = useState<'all' | AlertSeverity>('all')
  const { data: alertFeed, isLoading: alertLoading } = useQuery({
    queryKey: ['monitoring-alerts', filter],
    queryFn: () => fetchAlertFeed(filter === 'all' ? undefined : { severity: filter }),
  })

  const { data: poolSnapshot, isLoading: poolsLoading } = useQuery({
    queryKey: ['monitoring-pools'],
    queryFn: () => fetchMonitoredPools(8),
  })

  const alerts = alertFeed?.alerts ?? []
  const openAlerts = alerts.filter((alert) => alert.lifecycle === 'open')
  const totalAlerts = (alertFeed?.totals.open ?? 0) + (alertFeed?.totals.acknowledged ?? 0) + (alertFeed?.totals.suppressed ?? 0)
  const openPercent = totalAlerts ? Math.round((openAlerts.length / totalAlerts) * 100) : 0

  const poolHotspots = useMemo(() => {
    if (!poolSnapshot?.pools?.length) {
      return []
    }
    return [...poolSnapshot.pools].sort((a, b) => b.utilization - a.utilization).slice(0, 5)
  }, [poolSnapshot])

  return (
    <Row gutter={[24, 24]}>
      <Col xs={24} md={8}>
        <Card className="glass-panel" title="告警统计" loading={alertLoading}>
          <Statistic title="开放" value={alertFeed?.totals.open ?? 0} valueStyle={{ color: '#ff7875' }} />
          <Statistic title="已确认" value={alertFeed?.totals.acknowledged ?? 0} style={{ marginTop: 16 }} />
          <Statistic title="已压制" value={alertFeed?.totals.suppressed ?? 0} style={{ marginTop: 16 }} />
          <Progress percent={openPercent} showInfo={false} style={{ marginTop: 12 }} />
        </Card>
      </Col>
      <Col xs={24} md={16}>
        <Card className="glass-panel" title="热点地址池" loading={poolsLoading}>
          <List
            dataSource={poolHotspots}
            locale={{ emptyText: '暂无监控数据' }}
            renderItem={(pool) => (
              <List.Item>
                <List.Item.Meta
                  title={pool.name}
                  description={`利用率 ${pool.utilization.toFixed(1)}% · VLAN ${pool.vlanId ?? '—'} · ${pool.location ?? '未知'}`}
                />
                <div style={{ width: 160 }}>
                  <Progress percent={Math.min(pool.utilization, 100)} showInfo={false} strokeColor={{ from: '#ff7875', to: '#ffa940' }} />
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
        <Card
          className="glass-panel"
          title="统一告警流"
          extra={
            <Segmented
              options={[
                { label: '全部', value: 'all' },
                { label: '信息', value: 'info' },
                { label: '警告', value: 'warning' },
                { label: '严重', value: 'critical' },
              ]}
              value={filter}
              onChange={(value) => setFilter(value as typeof filter)}
            />
          }
        >
          <List<AlertFeedEntry>
            loading={alertLoading}
            dataSource={alerts}
            locale={{ emptyText: '当前无告警' }}
            renderItem={(item) => (
              <List.Item
                actions={[
                  <Tag key="lifecycle" color={lifecycleColor[item.lifecycle]}>
                    {item.lifecycle.toUpperCase()}
                  </Tag>,
                  <Tag key="time">{dayjs(item.createdAt).format('MM-DD HH:mm')}</Tag>,
                ]}
              >
                <List.Item.Meta
                  title={
                    <Typography.Text>
                      {item.summary}
                      <Tag color={severityColor[item.severity]} style={{ marginLeft: 8 }}>
                        {item.severity.toUpperCase()}
                      </Tag>
                    </Typography.Text>
                  }
                  description={item.details}
                />
                <Typography.Text type="secondary">{item.source}</Typography.Text>
              </List.Item>
            )}
          />
        </Card>
      </Col>
    </Row>
  )
}
