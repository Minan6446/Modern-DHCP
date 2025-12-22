import { ThunderboltOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { Card, Col, List, Progress, Row, Space, Statistic, Tag, Typography } from 'antd'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import { useMemo } from 'react'
import { fetchAlertFeed, fetchMonitoredPools } from '../services/monitoring'
import { fetchOperationsLog, fetchOverviewSnapshot } from '../services/dashboard'
import type {
  AlertFeedEntry,
  AlertLifecycle,
  AlertSeverity,
  DimensionCount,
  OperationLogEntry,
  OverviewSnapshot,
  PoolUsageSummary,
} from '../types/api'

dayjs.extend(relativeTime)

function topPools(snapshot?: OverviewSnapshot) {
  if (!snapshot?.poolUsage?.length) {
    return []
  }
  return [...snapshot.poolUsage].sort((a, b) => b.utilization - a.utilization)
}

function takeTop(items?: DimensionCount[], limit = 4) {
  if (!items?.length) return []
  return items.slice(0, limit)
}

const severityMeta: Record<AlertSeverity, { color: string; label: string }> = {
  info: { color: 'cyan', label: '信息' },
  warning: { color: 'gold', label: '警告' },
  critical: { color: 'red', label: '严重' },
}

const lifecycleMeta: Record<AlertLifecycle, { color: string; label: string }> = {
  open: { color: 'blue', label: '未处理' },
  acknowledged: { color: 'green', label: '已确认' },
  suppressed: { color: 'purple', label: '已抑制' },
}

export default function DashboardPage() {
  const { data: overview, isLoading: overviewLoading } = useQuery({
    queryKey: ['monitoring-overview'],
    queryFn: fetchOverviewSnapshot,
  })

  const { data: operations, isLoading: opsLoading } = useQuery({
    queryKey: ['operations-log'],
    queryFn: fetchOperationsLog,
  })

  const { data: monitoredPools, isLoading: poolsLoading } = useQuery({
    queryKey: ['monitoring-pools', 6],
    queryFn: () => fetchMonitoredPools(6),
  })

  const { data: alertFeed, isLoading: alertsLoading } = useQuery({
    queryKey: ['monitoring-alert-feed', 6],
    queryFn: () => fetchAlertFeed({ limit: 6 }),
    refetchInterval: 60_000,
  })

  const sortedPools = useMemo(() => topPools(overview), [overview])
  const busiestPool = sortedPools[0]

  const poolHotspots = useMemo(() => {
    if (!monitoredPools?.pools?.length) return []
    return [...monitoredPools.pools].sort((a, b) => b.utilization - a.utilization)
  }, [monitoredPools])

  const totals = useMemo(() => {
    const phases = overview?.requestPhases ?? []
    return phases.reduce(
      (acc, phase) => {
        acc.success += phase.success
        acc.failure += phase.failure
        return acc
      },
      { success: 0, failure: 0 },
    )
  }, [overview?.requestPhases])

  const totalRequests = totals.success + totals.failure
  const successRate = totalRequests ? Math.round((totals.success / totalRequests) * 100) : 0
  const rateLimit = overview?.security?.rateLimit
  const clientDist = overview?.clientDistribution
  const alertTotals = alertFeed?.totals
  const alertItems = alertFeed?.alerts ?? []

  return (
    <Row gutter={[24, 24]}>
      <Col span={24}>
        <Row gutter={24}>
          <Col xs={24} md={6}>
            <Card loading={overviewLoading} className="glass-panel">
              <Statistic
                title="CPU / 内存"
                value={overview ? `${overview.systemHealth.cpuPercent.toFixed(1)}%` : '--'}
                valueStyle={{ color: '#5d5bf3' }}
              />
              <Typography.Text type="secondary">
                内存 {overview ? overview.systemHealth.memoryPercent.toFixed(1) : '--'}% · Goroutines {overview?.systemHealth.goroutines ?? '--'}
              </Typography.Text>
            </Card>
          </Col>
          <Col xs={24} md={6}>
            <Card loading={overviewLoading} className="glass-panel">
              <Statistic title="最繁忙地址池" value={busiestPool?.name ?? '暂无数据'} />
              <Progress
                percent={busiestPool?.utilization ?? 0}
                strokeColor={{ from: '#5d5bf3', to: '#00c6fb' }}
                showInfo={false}
                style={{ marginTop: 12 }}
              />
              <Typography.Text type="secondary">
                已用 {busiestPool?.allocated?.toLocaleString() ?? '--'} / {busiestPool?.capacity?.toLocaleString() ?? '--'}
              </Typography.Text>
            </Card>
          </Col>
          <Col xs={24} md={6}>
            <Card loading={overviewLoading} className="glass-panel">
              <Statistic title="请求成功率" value={successRate} suffix="%" />
              <Typography.Text type="secondary">
                窗口内 {totalRequests.toLocaleString()} 次请求
              </Typography.Text>
            </Card>
          </Col>
          <Col xs={24} md={6}>
            <Card loading={overviewLoading} className="glass-panel">
              <Statistic title="速率限制命中" value={rateLimit?.totalHits ?? 0} />
              <Typography.Text type="secondary">
                窗口 {rateLimit?.window ?? '--'} · 最后节点 {rateLimit?.lastMac ?? 'N/A'}
              </Typography.Text>
            </Card>
          </Col>
        </Row>
      </Col>
      <Col xs={24} lg={12}>
        <Card title="请求阶段延迟" loading={overviewLoading} className="glass-panel">
          <List
            dataSource={overview?.requestPhases ?? []}
            locale={{ emptyText: '暂无数据' }}
            renderItem={(phase) => (
              <List.Item>
                <List.Item.Meta
                  title={`${phase.protocol} · ${phase.message}`}
                  description={`均值 ${phase.averageMs.toFixed(1)}ms · P95 ${phase.p95Ms.toFixed(1)}ms`}
                />
                <Tag color={phase.failure > 0 ? 'warning' : 'success'}>
                  {phase.success.toLocaleString()} 成功 / {phase.failure.toLocaleString()} 失败
                </Tag>
              </List.Item>
            )}
          />
        </Card>
      </Col>
      <Col xs={24} lg={12}>
        <Card title="客户端分布" loading={overviewLoading} className="glass-panel">
          <Row gutter={16}>
            <Col span={12}>
              <Typography.Text type="secondary">设备类型</Typography.Text>
              <List
                size="small"
                dataSource={takeTop(clientDist?.byDeviceType)}
                locale={{ emptyText: '暂无数据' }}
                renderItem={(item) => (
                  <List.Item>
                    <span>{item.key}</span>
                    <Tag color="default">{item.count}</Tag>
                  </List.Item>
                )}
              />
            </Col>
            <Col span={12}>
              <Typography.Text type="secondary">位置</Typography.Text>
              <List
                size="small"
                dataSource={takeTop(clientDist?.byLocation)}
                locale={{ emptyText: '暂无数据' }}
                renderItem={(item) => (
                  <List.Item>
                    <span>{item.key}</span>
                    <Tag color="blue">{item.count}</Tag>
                  </List.Item>
                )}
              />
            </Col>
            <Col span={24}>
              <Typography.Text type="secondary">VLAN</Typography.Text>
              <List
                size="small"
                dataSource={takeTop(clientDist?.byVlan)}
                locale={{ emptyText: '暂无数据' }}
                renderItem={(item) => (
                  <List.Item>
                    <span>VLAN {item.key}</span>
                    <Tag color="purple">{item.count}</Tag>
                  </List.Item>
                )}
              />
            </Col>
          </Row>
        </Card>
      </Col>
      <Col xs={24} lg={12}>
        <Card
          title="容量热点"
          className="glass-panel"
          loading={poolsLoading && !poolHotspots.length}
          extra={
            monitoredPools?.generatedAt && (
              <Typography.Text type="secondary">
                更新 {dayjs(monitoredPools.generatedAt).fromNow()}
              </Typography.Text>
            )
          }
        >
          <List<PoolUsageSummary>
            size="small"
            dataSource={poolHotspots}
            locale={{ emptyText: '暂无地址池' }}
            renderItem={(pool) => (
              <List.Item>
                <List.Item.Meta
                  title={pool.name}
                  description={
                    <Typography.Text type="secondary">
                      {pool.scope ?? '全局'}
                      {' · '}
                      {pool.location ?? '未标注位置'}
                      {' · '}
                      {pool.vlanId ? `VLAN ${pool.vlanId}` : '无 VLAN'}
                    </Typography.Text>
                  }
                />
                <div style={{ textAlign: 'right' }}>
                  <Tag color={pool.utilization >= 90 ? 'volcano' : pool.utilization >= 75 ? 'gold' : 'green'}>
                    {pool.utilization.toFixed(1)}%
                  </Tag>
                  <Typography.Text type="secondary" style={{ display: 'block' }}>
                    {pool.allocated.toLocaleString()} / {pool.capacity.toLocaleString()}
                  </Typography.Text>
                </div>
              </List.Item>
            )}
          />
        </Card>
      </Col>
      <Col xs={24} lg={12}>
        <Card
          title="告警收件箱"
          className="glass-panel"
          loading={alertsLoading && !alertItems.length}
          extra={
            alertFeed?.generatedAt && (
              <Typography.Text type="secondary">
                更新 {dayjs(alertFeed.generatedAt).fromNow()}
              </Typography.Text>
            )
          }
        >
          {alertTotals && (
            <Space size={8} wrap style={{ marginBottom: 16 }}>
              <Tag color="red">开放 {alertTotals.open}</Tag>
              <Tag color="gold">已确认 {alertTotals.acknowledged}</Tag>
              <Tag color="default">已抑制 {alertTotals.suppressed}</Tag>
            </Space>
          )}
          <List<AlertFeedEntry>
            size="small"
            dataSource={alertItems}
            locale={{ emptyText: '暂无告警' }}
            renderItem={(alert) => (
              <List.Item>
                <List.Item.Meta
                  title={
                    <Space size={8} wrap>
                      <Tag color={severityMeta[alert.severity].color}>{severityMeta[alert.severity].label}</Tag>
                      <Typography.Text strong>{alert.summary}</Typography.Text>
                    </Space>
                  }
                  description={
                    <>
                      <Typography.Text type="secondary">
                        {alert.category} · {alert.source}
                      </Typography.Text>
                      <div style={{ color: 'rgba(248,249,255,0.65)' }}>{alert.details}</div>
                    </>
                  }
                />
                <div style={{ textAlign: 'right' }}>
                  <Tag color={lifecycleMeta[alert.lifecycle].color}>{lifecycleMeta[alert.lifecycle].label}</Tag>
                  <Typography.Text type="secondary" style={{ display: 'block' }}>
                    {alert.tenantId}
                  </Typography.Text>
                  <Typography.Text type="secondary" style={{ display: 'block' }}>
                    {dayjs(alert.updatedAt ?? alert.createdAt).fromNow()}
                  </Typography.Text>
                </div>
              </List.Item>
            )}
          />
        </Card>
      </Col>
      <Col span={24}>
        <Card title="运维操作记录" loading={opsLoading} className="glass-panel">
          <List<OperationLogEntry>
            dataSource={operations ?? []}
            locale={{ emptyText: '暂无操作' }}
            renderItem={(item) => (
              <List.Item>
                <List.Item.Meta
                  title={
                    <Typography.Text>
                      {item.action}
                      <Typography.Text type="secondary" style={{ marginLeft: 8 }}>
                        {item.resource}
                      </Typography.Text>
                    </Typography.Text>
                  }
                  description={
                    <>
                      <Typography.Text>{item.actor}</Typography.Text>
                      <Typography.Text type="secondary" style={{ marginLeft: 8 }}>
                        {item.scope}
                      </Typography.Text>
                      <div style={{ color: 'rgba(248,249,255,0.55)' }}>{item.description}</div>
                    </>
                  }
                />
                <div style={{ textAlign: 'right' }}>
                  <Tag icon={<ThunderboltOutlined />} color={item.status === 'success' ? 'success' : 'error'}>
                    {item.status.toUpperCase()}
                  </Tag>
                  <Typography.Text type="secondary" style={{ display: 'block' }}>
                    {dayjs(item.createdAt).fromNow()}
                  </Typography.Text>
                </div>
              </List.Item>
            )}
          />
        </Card>
      </Col>
    </Row>
  )
}
