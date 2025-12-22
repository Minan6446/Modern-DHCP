import { ThunderboltOutlined } from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Button,
  Card,
  Col,
  Descriptions,
  Divider,
  Drawer,
  Empty,
  List,
  Row,
  Segmented,
  Skeleton,
  Statistic,
  Tag,
  Timeline,
  Typography,
  message,
} from 'antd'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import { useMemo, useState, type MouseEvent } from 'react'
import { createJob, fetchJob, fetchSchedules, fetchSnapshot, listJobs } from '../services/automation'
import { useSessionStore } from '../store/session'
import type { AutomationJobRun, AutomationJobStatus, AutomationSchedule } from '../types/api'

dayjs.extend(relativeTime)

const scheduleFilterOptions = [
  { label: '全部', value: 'all' },
  { label: '运行中', value: 'enabled' },
  { label: '已暂停', value: 'disabled' },
]

const jobStatusOptions: Array<{ label: string; value: AutomationJobStatus | 'all' }> = [
  { label: '全部', value: 'all' },
  { label: '等待', value: 'pending' },
  { label: '进行中', value: 'running' },
  { label: '成功', value: 'succeeded' },
  { label: '失败', value: 'failed' },
]

const statusTagMap: Record<AutomationJobStatus, { tag: string; color: string; timeline: string }> = {
  pending: { tag: 'processing', color: '#91d5ff', timeline: 'blue' },
  running: { tag: 'processing', color: '#5d5bf3', timeline: 'cyan' },
  succeeded: { tag: 'success', color: '#52c41a', timeline: 'green' },
  failed: { tag: 'error', color: '#ff4d4f', timeline: 'red' },
}

export default function AutomationPage() {
  const [scheduleFilter, setScheduleFilter] = useState<'all' | 'enabled' | 'disabled'>('all')
  const [statusFilter, setStatusFilter] = useState<'all' | AutomationJobStatus>('all')
  const [selectedSchedule, setSelectedSchedule] = useState<AutomationSchedule | null>(null)
  const [selectedJobId, setSelectedJobId] = useState<string | null>(null)
  const selectedType = selectedSchedule?.type
  const queryClient = useQueryClient()
  const profile = useSessionStore((state) => state.profile)

  const { data: schedules, isLoading: scheduleLoading } = useQuery({
    queryKey: ['automation-schedules'],
    queryFn: fetchSchedules,
  })

  const { data: snapshot, isLoading: snapshotLoading } = useQuery({
    queryKey: ['automation-snapshot'],
    queryFn: fetchSnapshot,
  })

  const { data: jobPage, isLoading: jobsLoading } = useQuery({
    queryKey: ['automation-jobs', selectedType ?? 'all', statusFilter],
    queryFn: () =>
      listJobs({
        limit: 40,
        types: selectedType,
        statuses: statusFilter === 'all' ? undefined : statusFilter,
      }),
  })

  const jobs = jobPage?.items ?? []

  const { data: jobDetail, isLoading: jobDetailLoading } = useQuery({
    queryKey: ['automation-job', selectedJobId],
    queryFn: () => fetchJob(selectedJobId!),
    enabled: Boolean(selectedJobId),
  })

  const filteredSchedules = useMemo(() => {
    if (!schedules) return []
    return schedules.filter((schedule) => {
      if (scheduleFilter === 'all') return true
      return scheduleFilter === 'enabled' ? schedule.enabled : !schedule.enabled
    })
  }, [scheduleFilter, schedules])

  const runMutation = useMutation<AutomationJobRun, Error, AutomationSchedule>({
    mutationFn: (schedule: AutomationSchedule) =>
      createJob({
        type: schedule.type,
        tenantId: schedule.tenantId,
        payload: schedule.payload,
        labels: {
          ...schedule.labels,
          schedule_ref: schedule.name ?? schedule.type,
        },
        channels: schedule.channels,
        source: 'ui.schedule-run',
        triggeredBy: profile?.displayName ?? profile?.id,
      }),
    onSuccess: () => {
      message.success('任务已入队')
      queryClient.invalidateQueries({ queryKey: ['automation-jobs'] })
    },
    onError: () => message.error('触发失败，请稍后重试'),
  })

  const handleScheduleSelect = (schedule: AutomationSchedule) => {
    setSelectedSchedule(schedule)
  }

  const handleRunNow = (schedule: AutomationSchedule, event: MouseEvent<HTMLElement>) => {
    event.stopPropagation()
    runMutation.mutate(schedule)
  }

  const handleJobSelect = (jobId: string) => {
    setSelectedJobId(jobId)
  }

  const renderTimeline = () => {
    if (jobsLoading) {
      return <Timeline pending="加载中..." />
    }
    if (jobs.length === 0) {
      return <Empty description="暂无执行记录" />
    }

    return (
      <Timeline
        items={jobs.map((job: AutomationJobRun) => {
          const meta = statusTagMap[job.status]
          const startedAt = job.startedAt ?? job.queuedAt
          const completedAt = job.completedAt ?? job.updatedAt
          const durationSeconds = startedAt && completedAt ? dayjs(completedAt).diff(dayjs(startedAt), 'second') : null
          return {
            color: meta.timeline,
            children: (
              <div
                onClick={() => handleJobSelect(job.id)}
                style={{ cursor: 'pointer' }}
                aria-hidden="true"
              >
                <Typography.Text strong>{job.type}</Typography.Text>
                <Typography.Text style={{ marginLeft: 8 }} type="secondary">
                  {job.triggeredBy} · {dayjs(startedAt ?? job.updatedAt).format('MM-DD HH:mm:ss')}
                </Typography.Text>
                <div style={{ marginTop: 8 }}>
                  <Tag color={meta.tag}>{job.status.toUpperCase()}</Tag>
                  <Typography.Text type="secondary" style={{ marginLeft: 8 }}>
                    来源 {job.source}
                  </Typography.Text>
                </div>
                {job.resultSummary && (
                  <Typography.Text style={{ display: 'block', marginTop: 8 }}>{job.resultSummary}</Typography.Text>
                )}
                {job.errorMessage && (
                  <Typography.Text type="danger" style={{ display: 'block', marginTop: 4 }}>
                    {job.errorMessage}
                  </Typography.Text>
                )}
                {durationSeconds !== null && (
                  <Typography.Text type="secondary" style={{ display: 'block', marginTop: 4 }}>
                    耗时 {durationSeconds}s
                  </Typography.Text>
                )}
              </div>
            ),
          }
        })}
      />
    )
  }

  return (
    <>
      <Row gutter={[24, 24]}>
      <Col xs={24} md={10}>
        <Row gutter={[24, 24]}>
          <Col span={24}>
            <Card className="glass-panel" title="调度器状态" loading={snapshotLoading}>
              <Row gutter={16}>
                <Col span={12}>
                  <Statistic title="排队任务" value={snapshot?.pendingJobs ?? 0} valueStyle={{ color: '#5d5bf3' }} />
                </Col>
                <Col span={12}>
                  <Statistic title="活动工作线程" value={snapshot?.activeWorkers ?? 0} />
                </Col>
                <Col span={12} style={{ marginTop: 16 }}>
                  <Statistic title="注册处理器" value={snapshot?.registeredHandlers ?? 0} />
                </Col>
                <Col span={12} style={{ marginTop: 16 }}>
                  <Typography.Text type="secondary">
                    运行自 {snapshot?.startedAt ? dayjs(snapshot.startedAt).fromNow() : '—'}
                  </Typography.Text>
                </Col>
              </Row>
            </Card>
          </Col>
          <Col span={24}>
            <Card
              className="glass-panel"
              title="计划任务"
              extra={
                <Segmented
                  options={scheduleFilterOptions}
                  value={scheduleFilter}
                  onChange={(value) => setScheduleFilter(value as 'all' | 'enabled' | 'disabled')}
                />
              }
              loading={scheduleLoading}
            >
              <List<AutomationSchedule>
                dataSource={filteredSchedules}
                locale={{ emptyText: '尚未配置计划任务' }}
                renderItem={(item) => (
                  <List.Item
                    onClick={() => handleScheduleSelect(item)}
                    style={{
                      cursor: 'pointer',
                      background:
                        selectedSchedule?.type === item.type ? 'rgba(93,91,243,0.08)' : 'transparent',
                      borderRadius: 12,
                      padding: '12px 16px',
                    }}
                    actions={[
                      <Tag key="interval" color="cyan">
                        {item.interval}
                      </Tag>,
                      <Button
                        key="run"
                        type="link"
                        icon={<ThunderboltOutlined />}
                        loading={runMutation.isPending && runMutation.variables?.type === item.type}
                        onClick={(event) => handleRunNow(item, event)}
                      >
                        运行一次
                      </Button>,
                    ]}
                  >
                    <List.Item.Meta
                      title={
                        <Typography.Text>
                          {item.name ?? item.type}
                          <Tag color={item.enabled ? 'success' : 'default'} style={{ marginLeft: 8 }}>
                            {item.enabled ? '启用' : '暂停'}
                          </Tag>
                        </Typography.Text>
                      }
                      description={`租户 ${item.tenantId} · 通道 ${item.channels.length}`}
                    />
                  </List.Item>
                )}
              />
            </Card>
          </Col>
        </Row>
      </Col>
        <Col xs={24} md={14}>
        <Card
          className="glass-panel"
          title={selectedSchedule ? `任务历史 · ${selectedSchedule.name ?? selectedSchedule.type}` : '全部任务历史'}
          extra={
            <Segmented
              options={jobStatusOptions}
              value={statusFilter}
              onChange={(value) => setStatusFilter(value as 'all' | AutomationJobStatus)}
            />
          }
        >
          {renderTimeline()}
          <Typography.Text type="secondary" style={{ marginTop: 16, display: 'block' }}>
            总计 {jobPage?.total ?? 0} 条 · 每次读取 {jobPage?.limit ?? 40} 条
          </Typography.Text>
        </Card>
        </Col>
      </Row>
      <Drawer
        destroyOnClose
        width={520}
        title="任务详情"
        open={Boolean(selectedJobId)}
        onClose={() => setSelectedJobId(null)}
      >
        {jobDetailLoading ? (
          <Skeleton active paragraph={{ rows: 6 }} />
        ) : jobDetail ? (
          <>
            <Tag color={statusTagMap[jobDetail.status].tag}>{jobDetail.status.toUpperCase()}</Tag>
            <Typography.Paragraph type="secondary">
              触发人 {jobDetail.triggeredBy} · 来源 {jobDetail.source}
            </Typography.Paragraph>
            <Descriptions column={1} size="small" colon>
              <Descriptions.Item label="任务类型">{jobDetail.type}</Descriptions.Item>
              <Descriptions.Item label="租户">{jobDetail.tenantId}</Descriptions.Item>
              <Descriptions.Item label="优先级">{jobDetail.priority ?? '—'}</Descriptions.Item>
              <Descriptions.Item label="尝试次数">{jobDetail.attempts ?? 0}</Descriptions.Item>
              <Descriptions.Item label="排队时间">
                {jobDetail.queuedAt ? dayjs(jobDetail.queuedAt).format('YYYY-MM-DD HH:mm:ss') : '—'}
              </Descriptions.Item>
              <Descriptions.Item label="开始时间">
                {jobDetail.startedAt ? dayjs(jobDetail.startedAt).format('YYYY-MM-DD HH:mm:ss') : '—'}
              </Descriptions.Item>
              <Descriptions.Item label="完成时间">
                {jobDetail.completedAt ? dayjs(jobDetail.completedAt).format('YYYY-MM-DD HH:mm:ss') : '—'}
              </Descriptions.Item>
            </Descriptions>
            {jobDetail.resultSummary && (
              <>
                <Divider />
                <Typography.Title level={5}>结果摘要</Typography.Title>
                <Typography.Paragraph>{jobDetail.resultSummary}</Typography.Paragraph>
              </>
            )}
            {jobDetail.errorMessage && (
              <>
                <Divider />
                <Typography.Title level={5} type="danger">
                  错误信息
                </Typography.Title>
                <Typography.Paragraph type="danger">{jobDetail.errorMessage}</Typography.Paragraph>
              </>
            )}
            {jobDetail.payload && Object.keys(jobDetail.payload).length > 0 && (
              <>
                <Divider />
                <Typography.Title level={5}>Payload</Typography.Title>
                <pre
                  style={{ background: 'rgba(255,255,255,0.04)', padding: 12, borderRadius: 12, overflow: 'auto' }}
                >
                  {JSON.stringify(jobDetail.payload, null, 2)}
                </pre>
              </>
            )}
            {jobDetail.labels && Object.keys(jobDetail.labels).length > 0 && (
              <>
                <Divider />
                <Typography.Title level={5}>Labels</Typography.Title>
                <List
                  size="small"
                  dataSource={Object.entries(jobDetail.labels)}
                  renderItem={([key, value]) => (
                    <List.Item>
                      <Typography.Text strong>{key}</Typography.Text>
                      <span>{value}</span>
                    </List.Item>
                  )}
                />
              </>
            )}
          </>
        ) : (
          <Empty description="请选择任务" />
        )}
      </Drawer>
    </>
  )
}
