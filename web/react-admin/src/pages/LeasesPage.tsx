import { DownloadOutlined, ScheduleOutlined } from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Button,
  Card,
  Col,
  DatePicker,
  Divider,
  Descriptions,
  Drawer,
  Empty,
  Form,
  Input,
  InputNumber,
  List,
  Popconfirm,
  Progress,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
  type TableColumnsType,
} from 'antd'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import { useCallback, useMemo, useState } from 'react'
import {
  exportLeaseHistory,
  fetchLeaseHistory,
  fetchLeaseInsights,
  listLeases,
  releaseLease,
  scheduleLeaseHistory,
} from '../services/leases'
import type { ExportJobSummary, LeaseHistoryQuery, LeaseRecord, ReportingArtifact } from '../types/api'
import type { Dayjs } from 'dayjs'
import { useSessionStore } from '../store/session'
import {
  useLeaseFilterGuards,
  type LeaseFilterFieldErrors,
  type LeaseFilterPayload,
} from '../hooks/useLeaseFilterGuards'
import { useI18n } from '../hooks/useI18n'

dayjs.extend(relativeTime)
const { RangePicker } = DatePicker
const stateColors: Record<LeaseRecord['state'], string> = {
  ACTIVE: 'success',
  EXPIRED: 'default',
  RECLAIMED: 'warning',
}

export default function LeasesPage() {
  const activeTenantId = useSessionStore((state) => state.activeTenantId)
  const tenantKey = activeTenantId ?? 'tenant-default'
  const [searchInput, setSearchInput] = useState('')
  const [query, setQuery] = useState('')
  const [stateFilter, setStateFilter] = useState<LeaseRecord['state'] | 'all'>('all')
  const [poolFilter, setPoolFilter] = useState<string | undefined>(undefined)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(15)
  const [selectedLease, setSelectedLease] = useState<LeaseRecord | null>(null)
  const [historyState, setHistoryState] = useState<LeaseRecord['state'] | 'all'>('all')
  const [historyRange, setHistoryRange] = useState<[Dayjs | null, Dayjs | null] | null>(null)
  const [lastExport, setLastExport] = useState<ReportingArtifact | null>(null)
  const [scheduleJob, setScheduleJob] = useState<ExportJobSummary | null>(null)
  const [scheduleForm] = Form.useForm()
  const [advancedForm] = Form.useForm()
  const [advancedFilters, setAdvancedFilters] = useState<LeaseFilterPayload>({})
  const [guardErrors, setGuardErrors] = useState<string[]>([])
  const [fieldErrors, setFieldErrors] = useState<LeaseFilterFieldErrors>({})
  const { validate: validateLeaseFilters } = useLeaseFilterGuards()
  const queryClient = useQueryClient()
  const { ipRange: appliedIpRange, macPrefix: appliedMacPrefix, lastSeenMinutes: appliedLastSeenMinutes } = advancedFilters
  const { t } = useI18n()

  const leaseStateLabels = useMemo(
    () => ({
      ACTIVE: t('leasesStateLabelActive'),
      EXPIRED: t('leasesStateLabelExpired'),
      RECLAIMED: t('leasesStateLabelReclaimed'),
    }),
    [t],
  )

  const stateSelectOptions = useMemo(
    () => [
      { label: t('leasesStateFilterAll'), value: 'all' },
      { label: leaseStateLabels.ACTIVE, value: 'ACTIVE' },
      { label: leaseStateLabels.EXPIRED, value: 'EXPIRED' },
      { label: leaseStateLabels.RECLAIMED, value: 'RECLAIMED' },
    ],
    [leaseStateLabels, t],
  )

  const resetHistoryFilters = useCallback(() => {
    setHistoryState('all')
    setHistoryRange(null)
  }, [])

  const handleApplyAdvancedFilters = () => {
    const rawValues = advancedForm.getFieldsValue()
    const result = validateLeaseFilters({
      ipRange: rawValues.ipRange,
      macPrefix: rawValues.macPrefix,
      lastSeenMinutes: rawValues.lastSeenMinutes,
    })
    if (!result.isValid) {
      setGuardErrors(result.errors)
      setFieldErrors(result.fieldErrors)
      return
    }
    setGuardErrors([])
    setFieldErrors({})
    setAdvancedFilters(result.payload)
    setPage(1)
  }

  const handleResetAdvancedFilters = () => {
    advancedForm.resetFields()
    setAdvancedFilters({})
    setGuardErrors([])
    setFieldErrors({})
    setPage(1)
  }

  const openLeaseDetails = useCallback(
    (lease: LeaseRecord) => {
      resetHistoryFilters()
      setSelectedLease(lease)
    },
    [resetHistoryFilters],
  )

  const { data, isLoading } = useQuery({
    queryKey: [
      'leases',
      tenantKey,
      query,
      stateFilter,
      poolFilter,
      page,
      pageSize,
      appliedIpRange,
      appliedMacPrefix,
      appliedLastSeenMinutes,
    ],
    queryFn: () =>
      listLeases({
        query,
        state: stateFilter === 'all' ? undefined : stateFilter,
        poolName: poolFilter,
        page,
        size: pageSize,
        ipRange: appliedIpRange,
        macPrefix: appliedMacPrefix,
        lastSeenMinutes: appliedLastSeenMinutes,
      }),
    keepPreviousData: true,
  })

  const { data: leaseInsights, isLoading: insightsLoading } = useQuery({
    queryKey: ['lease-insights', tenantKey],
    queryFn: fetchLeaseInsights,
    staleTime: 60_000,
  })
  
  const exportHistory = useMutation({
    mutationFn: () => {
      if (!selectedLease?.address) {
        return Promise.reject(new Error(t('leasesExportNoSelection')))
      }
      return exportLeaseHistory({
        format: 'csv',
        destination: 'ui-download',
        state: historyState === 'all' ? undefined : historyState,
        ipAddress: selectedLease.address,
        from: historyRange?.[0]?.toISOString(),
        to: historyRange?.[1]?.toISOString(),
        limit: 500,
      })
    },
    onSuccess: (response) => {
      const artifactName = response.artifact?.name ?? 'lease-history.csv'
      setLastExport(response.artifact ?? null)
      message.success(t('leasesExportSuccess', { name: artifactName }))
    },
    onError: (error: unknown) => {
      const description = error instanceof Error ? error.message : t('leasesExportError')
      message.error(description)
    },
  })

  const releaseMutation = useMutation({
    mutationFn: (leaseId: string) => releaseLease(leaseId),
    onSuccess: () => {
      message.success(t('leasesReleaseSuccess'))
      queryClient.invalidateQueries({ queryKey: ['leases'] })
      queryClient.invalidateQueries({ queryKey: ['lease-insights'] })
      resetHistoryFilters()
      setSelectedLease(null)
    },
    onError: (error: unknown) => {
      const description = error instanceof Error ? error.message : t('leasesReleaseError')
      message.error(description)
    },
  })

  const historyParams = useMemo<LeaseHistoryQuery | null>(() => {
    if (!selectedLease?.address) {
      return null
    }
    const params: LeaseHistoryQuery = {
      ip: selectedLease.address,
      limit: 20,
    }
    if (historyState !== 'all') {
      params.state = historyState
    }
    if (historyRange?.[0]) {
      params.from = historyRange[0].toISOString()
    }
    if (historyRange?.[1]) {
      params.to = historyRange[1].toISOString()
    }
    return params
  }, [selectedLease, historyRange, historyState])

  const { data: leaseHistory, isLoading: historyLoading } = useQuery({
    queryKey: ['lease-history', tenantKey, historyParams],
    queryFn: () => fetchLeaseHistory(historyParams as LeaseHistoryQuery),
    enabled: Boolean(historyParams),
  })

  const scheduleMutation = useMutation({
    mutationFn: async () => {
      if (!selectedLease?.address) {
        throw new Error(t('leasesScheduleNoSelection'))
      }
      const values = scheduleForm.getFieldsValue()
      return scheduleLeaseHistory({
        format: values.format,
        destination: values.destination,
        intervalHours: values.intervalHours,
        limit: values.limit,
        state: historyState === 'all' ? undefined : historyState,
        ipAddress: selectedLease.address,
        from: historyRange?.[0]?.toISOString(),
        to: historyRange?.[1]?.toISOString(),
      })
    },
    onSuccess: (response) => {
      setScheduleJob(response.job)
      message.success(t('leasesScheduleSuccess'))
    },
    onError: (error: unknown) => {
      const description = error instanceof Error ? error.message : t('leasesScheduleError')
      message.error(description)
    },
  })
  const poolOptions = useMemo(() => {
    const pools = new Set<string>()
    data?.items?.forEach((lease) => {
      if (lease.poolName) pools.add(lease.poolName)
    })
    return Array.from(pools)
  }, [data?.items])

  const hasAdvancedFilters = Boolean(appliedIpRange || appliedMacPrefix || appliedLastSeenMinutes)

  const stateStats = useMemo(() => {
    if (leaseInsights?.stateBreakdown) {
      return leaseInsights.stateBreakdown
    }
    return (data?.items ?? []).reduce(
      (acc, lease) => {
        acc[lease.state] += 1
        return acc
      },
      { ACTIVE: 0, EXPIRED: 0, RECLAIMED: 0 },
    )
  }, [leaseInsights, data])

  const totalLeases = stateStats.ACTIVE + stateStats.EXPIRED + stateStats.RECLAIMED
  const poolsAtRisk = leaseInsights?.poolsAtRisk ?? []
  const vendorMix = leaseInsights?.vendorMix ?? []
  const pressureIndex = leaseInsights?.pressureIndex ?? 0
  const renewalRatePercent = leaseInsights ? Math.round(leaseInsights.renewalRate * 100) : 0

  const columns = useMemo<TableColumnsType<LeaseRecord>>(
    () => [
      { title: t('leasesColumnAddress'), dataIndex: 'address', key: 'address' },
      {
        title: t('leasesColumnMacVendor'),
        dataIndex: 'macAddress',
        key: 'macAddress',
        render: (value: string, record) => (
          <Space direction="vertical" size={2}>
            <Typography.Text>{value}</Typography.Text>
            {record.clientVendor && (
              <Tag color="default" style={{ width: 'fit-content' }}>
                {record.clientVendor}
              </Tag>
            )}
          </Space>
        ),
      },
      {
        title: t('leasesColumnHostname'),
        dataIndex: 'hostname',
        key: 'hostname',
        render: (value?: string) => value ?? t('leasesHostnameNotReported'),
      },
      { title: t('leasesColumnPool'), dataIndex: 'poolName', key: 'poolName' },
      {
        title: t('leasesColumnNetwork'),
        key: 'network',
        render: (_, record) => (
          <Space direction="vertical" size={2}>
            <Typography.Text>{t('leasesNetworkVlanLabel', { id: record.vlanId ?? '—' })}</Typography.Text>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {record.relayAgent ?? t('leasesNetworkNoRelay')}
            </Typography.Text>
          </Space>
        ),
      },
      {
        title: t('leasesColumnState'),
        dataIndex: 'state',
        key: 'state',
        render: (value: LeaseRecord['state']) => (
          <Tag color={stateColors[value]}>{leaseStateLabels[value] ?? value}</Tag>
        ),
      },
      {
        title: t('leasesColumnExpiry'),
        dataIndex: 'expiresAt',
        key: 'expiresAt',
        render: (value: string) => dayjs(value).format('YYYY-MM-DD HH:mm'),
      },
      {
        title: t('leasesColumnActions'),
        key: 'actions',
        render: (_, record) => (
          <Button
            type="link"
            onClick={(event) => {
              event.stopPropagation()
              openLeaseDetails(record)
            }}
          >
            {t('leasesDetailsButton')}
          </Button>
        ),
      },
    ],
    [leaseStateLabels, openLeaseDetails, t],
  )

  return (
    <Row gutter={[24, 24]}>
      <Col span={24}>
        <Row gutter={[24, 24]}>
          <Col xs={24} md={6}>
            <Card className="glass-panel" loading={insightsLoading}>
              <Statistic title={t('leasesStatsPressureTitle')} value={pressureIndex} suffix="/ 100" valueStyle={{ color: pressureIndex >= 80 ? '#fa541c' : '#5d5bf3' }} />
              <Typography.Text type="secondary">{t('leasesStatsPressureDescription')}</Typography.Text>
            </Card>
          </Col>
          <Col xs={24} md={6}>
            <Card className="glass-panel" loading={insightsLoading}>
              <Statistic title={t('leasesStatsRenewalTitle')} value={renewalRatePercent} suffix="%" valueStyle={{ color: '#52c41a' }} />
              <Typography.Text type="secondary">{t('leasesStatsRenewalDescription')}</Typography.Text>
            </Card>
          </Col>
          <Col xs={24} md={6}>
            <Card className="glass-panel" loading={insightsLoading}>
              <Statistic title={t('leasesStatsExpiringTitle')} value={leaseInsights?.expiringNext24h ?? '—'} />
              <Typography.Text type="secondary">{t('leasesStatsExpiringDescription')}</Typography.Text>
            </Card>
          </Col>
          <Col xs={24} md={6}>
            <Card className="glass-panel" loading={insightsLoading}>
              <Statistic title={t('leasesStatsDeclinesTitle')} value={leaseInsights?.declines24h ?? '—'} />
              <Typography.Text type="secondary">{t('leasesStatsDeclinesDescription')}</Typography.Text>
            </Card>
          </Col>
        </Row>
      </Col>
      <Col span={24}>
        <Card className="glass-panel" title={t('leasesSearchCardTitle')}>
          {guardErrors.length > 0 && (
            <Alert
              type="error"
              showIcon
              message={t('leasesFilterAlertTitle')}
              description={
                <Space direction="vertical" size={2}>
                  {guardErrors.map((error, index) => (
                    <Typography.Text key={`${error}-${index}`}>{error}</Typography.Text>
                  ))}
                </Space>
              }
              style={{ marginBottom: 16 }}
            />
          )}
          <Row gutter={[16, 16]} align="middle">
            <Col xs={24} md={14}>
              <Input.Search
                placeholder={t('leasesSearchPlaceholder')}
                allowClear
                value={searchInput}
                onChange={(event) => {
                  const value = event.target.value
                  setSearchInput(value)
                  if (!value) {
                    setQuery('')
                    setPage(1)
                  }
                }}
                onSearch={(value) => {
                  setSearchInput(value)
                  setQuery(value.trim())
                  setPage(1)
                }}
                size="large"
              />
            </Col>
            <Col xs={12} md={4}>
              <Select
                value={stateFilter}
                onChange={(value) => {
                  setStateFilter(value)
                  setPage(1)
                }}
                style={{ width: '100%' }}
                options={stateSelectOptions}
              />
            </Col>
            <Col xs={12} md={4}>
              <Select
                allowClear
                placeholder={t('leasesPoolFilterPlaceholder')}
                value={poolFilter}
                onChange={(value) => {
                  setPoolFilter(value ?? undefined)
                  setPage(1)
                }}
                options={poolOptions.map((pool) => ({ label: pool, value: pool }))}
                style={{ width: '100%' }}
              />
            </Col>
            <Col xs={24} md={6}>
              <Card bordered={false} style={{ background: 'rgba(93,91,243,0.1)' }} loading={insightsLoading}>
                <Space direction="vertical" size={4}>
                  <Typography.Text type="secondary">
                    {t('leasesRealtimeStatsLabel', { count: totalLeases.toLocaleString() })}
                  </Typography.Text>
                  <Space size="large" wrap>
                    <Statistic title={leaseStateLabels.ACTIVE} value={stateStats.ACTIVE} valueStyle={{ fontSize: 16 }} />
                    <Statistic title={leaseStateLabels.EXPIRED} value={stateStats.EXPIRED} valueStyle={{ fontSize: 16 }} />
                    <Statistic title={leaseStateLabels.RECLAIMED} value={stateStats.RECLAIMED} valueStyle={{ fontSize: 16 }} />
                  </Space>
                </Space>
              </Card>
            </Col>
          </Row>
          <Divider plain>{t('leasesAdvancedDivider')}</Divider>
          <Form layout="vertical" form={advancedForm}>
            <Row gutter={[16, 16]}>
              <Col xs={24} md={8}>
                <Form.Item
                  label={t('leasesAdvancedIpLabel')}
                  name="ipRange"
                  validateStatus={fieldErrors.ipRange ? 'error' : undefined}
                  help={fieldErrors.ipRange}
                >
                  <Input placeholder={t('leasesAdvancedIpPlaceholder')} allowClear />
                </Form.Item>
              </Col>
              <Col xs={24} md={8}>
                <Form.Item
                  label={t('leasesAdvancedMacLabel')}
                  name="macPrefix"
                  validateStatus={fieldErrors.macPrefix ? 'error' : undefined}
                  help={fieldErrors.macPrefix}
                >
                  <Input placeholder={t('leasesAdvancedMacPlaceholder')} allowClear />
                </Form.Item>
              </Col>
              <Col xs={24} md={8}>
                <Form.Item
                  label={t('leasesAdvancedLastSeenLabel')}
                  name="lastSeenMinutes"
                  validateStatus={fieldErrors.lastSeenMinutes ? 'error' : undefined}
                  help={fieldErrors.lastSeenMinutes}
                >
                  <InputNumber min={5} max={1440} style={{ width: '100%' }} addonAfter={t('leasesAdvancedLastSeenAddon')} />
                </Form.Item>
              </Col>
            </Row>
            <Space>
              <Button type="primary" onClick={handleApplyAdvancedFilters}>
                {t('leasesApplyFilters')}
              </Button>
              <Button onClick={handleResetAdvancedFilters} disabled={!hasAdvancedFilters && !guardErrors.length}>
                {t('leasesClearFilters')}
              </Button>
            </Space>
          </Form>
          {hasAdvancedFilters && (
            <Space wrap style={{ marginTop: 12 }}>
              {appliedIpRange && <Tag color="gold">{t('leasesAppliedIpTag', { value: appliedIpRange })}</Tag>}
              {appliedMacPrefix && <Tag color="blue">{t('leasesAppliedMacTag', { value: appliedMacPrefix })}</Tag>}
              {appliedLastSeenMinutes && (
                <Tag color="purple">{t('leasesAppliedLastSeenTag', { minutes: appliedLastSeenMinutes })}</Tag>
              )}
            </Space>
          )}
        </Card>
      </Col>
      <Col span={24}>
        <Row gutter={[24, 24]}>
          <Col xs={24} md={12}>
            <Card className="glass-panel" title={t('leasesRiskPoolsTitle')} loading={insightsLoading}>
              <List
                dataSource={poolsAtRisk}
                locale={{ emptyText: t('leasesRiskPoolsEmpty') }}
                renderItem={(pool) => (
                  <List.Item>
                    <List.Item.Meta
                      title={pool.poolName}
                      description={t('leasesRiskPoolsDescription', {
                        tenant: pool.tenantId,
                        count: pool.activeLeases.toLocaleString(),
                      })}
                    />
                    <Space direction="vertical" align="end" size={4}>
                      <Tag color={pool.utilization >= 90 ? 'volcano' : 'warning'}>
                        {t('leasesRiskPoolUtilization', { percent: pool.utilization.toFixed(1) })}
                      </Tag>
                      <Typography.Text type="secondary">
                        {t('leasesRiskPoolEta', { eta: pool.exhaustionEta ?? '—' })}
                      </Typography.Text>
                    </Space>
                  </List.Item>
                )}
              />
            </Card>
          </Col>
          <Col xs={24} md={12}>
            <Card className="glass-panel" title={t('leasesVendorMixTitle')} loading={insightsLoading}>
              <List
                dataSource={vendorMix}
                locale={{ emptyText: t('leasesVendorMixEmpty') }}
                renderItem={(item) => (
                  <List.Item>
                    <Typography.Text>{item.key}</Typography.Text>
                    <Progress
                      percent={totalLeases ? Math.round((item.count / totalLeases) * 100) : 0}
                      showInfo
                      format={(percent) => `${percent}%`}
                      style={{ flex: 1, marginLeft: 16 }}
                    />
                  </List.Item>
                )}
              />
            </Card>
          </Col>
        </Row>
      </Col>
      <Col span={24}>
        <Card className="glass-panel" title={t('leasesTableTitle')}>
          <Table<LeaseRecord>
            rowKey="id"
            loading={isLoading}
            dataSource={data?.items ?? []}
            columns={columns}
            pagination={{
              current: page,
              pageSize,
              total: data?.total,
              showSizeChanger: true,
              showTotal: (total) => t('leasesTableTotal', { count: total ?? 0 }),
            }}
            onChange={(pagination) => {
              if (pagination.current) setPage(pagination.current)
              if (pagination.pageSize) setPageSize(pagination.pageSize)
            }}
            onRow={(record) => ({
              onClick: () => openLeaseDetails(record),
            })}
          />
        </Card>
      </Col>

      <Drawer
        title={t('leasesDrawerTitle')}
        width={520}
        open={Boolean(selectedLease)}
        onClose={() => {
          resetHistoryFilters()
          setSelectedLease(null)
        }}
        destroyOnClose
      >
        {selectedLease ? (
          <>
            <Space direction="vertical" size={8} style={{ width: '100%' }}>
              <Space>
                <Typography.Text strong style={{ fontSize: 18 }}>
                  {selectedLease.address}
                </Typography.Text>
                <Tag color={stateColors[selectedLease.state]}>
                  {leaseStateLabels[selectedLease.state] ?? selectedLease.state}
                </Tag>
              </Space>
              <Typography.Text type="secondary">
                {t('leasesDetailsFromPool', { pool: selectedLease.poolName ?? t('commonUnknown') })}
              </Typography.Text>
              <Space wrap>
                <Tooltip title={t('leasesCopyMacLabel')}>
                  <Button
                    size="small"
                    onClick={async () => navigator.clipboard.writeText(selectedLease.macAddress)}
                  >
                    {t('leasesCopyMacLabel')}
                  </Button>
                </Tooltip>
                <Tooltip title={t('leasesCopyHostnameLabel')}>
                  <Button
                    size="small"
                    onClick={async () => {
                      if (selectedLease.hostname) {
                        await navigator.clipboard.writeText(selectedLease.hostname)
                      }
                    }}
                    disabled={!selectedLease.hostname}
                  >
                    {t('leasesCopyHostnameLabel')}
                  </Button>
                </Tooltip>
                <Popconfirm
                  title={t('leasesForceReleaseConfirmTitle')}
                  description={t('leasesForceReleaseConfirmDescription')}
                  okText={t('leasesForceReleaseOk')}
                  cancelText={t('commonCancel')}
                  okButtonProps={{ loading: releaseMutation.isPending }}
                  onConfirm={() => releaseMutation.mutate(selectedLease.id)}
                >
                  <Button size="small" danger loading={releaseMutation.isPending}>
                    {t('leasesForceReleaseButton')}
                  </Button>
                </Popconfirm>
              </Space>
            </Space>
            <Descriptions column={1} style={{ marginTop: 24 }}>
              <Descriptions.Item label={t('leasesDetailsMacLabel')}>{selectedLease.macAddress}</Descriptions.Item>
              <Descriptions.Item label={t('leasesDetailsHostnameLabel')}>
                {selectedLease.hostname ?? t('leasesHostnameNotReported')}
              </Descriptions.Item>
              <Descriptions.Item label={t('leasesDetailsAssignedLabel')}>
                {selectedLease.assignedAt
                  ? dayjs(selectedLease.assignedAt).format('YYYY-MM-DD HH:mm:ss')
                  : t('commonUnknown')}
              </Descriptions.Item>
              <Descriptions.Item label={t('leasesDetailsLastSeenLabel')}>
                {selectedLease.lastSeenAt
                  ? dayjs(selectedLease.lastSeenAt).format('YYYY-MM-DD HH:mm:ss')
                  : '—'}
              </Descriptions.Item>
              <Descriptions.Item label={t('leasesDetailsExpiresLabel')}>
                {dayjs(selectedLease.expiresAt).format('YYYY-MM-DD HH:mm:ss')}
                <Typography.Text style={{ marginLeft: 8 }}>
                  {dayjs(selectedLease.expiresAt).fromNow()}
                </Typography.Text>
              </Descriptions.Item>
              <Descriptions.Item label={t('leasesDetailsVlanLabel')}>
                {selectedLease.vlanId ?? '—'}
              </Descriptions.Item>
              <Descriptions.Item label={t('leasesDetailsRelayLabel')}>
                {selectedLease.relayAgent ?? '—'}
              </Descriptions.Item>
              <Descriptions.Item label={t('leasesDetailsVendorLabel')}>
                {selectedLease.clientVendor ?? '—'}
              </Descriptions.Item>
              <Descriptions.Item label={t('leasesDetailsProfileLabel')}>
                {selectedLease.profile ?? '—'}
              </Descriptions.Item>
            </Descriptions>
            <Typography.Paragraph type="secondary" style={{ marginTop: 16 }}>
              {t('leasesReleaseInfo')}
            </Typography.Paragraph>
            <Typography.Title level={5} style={{ marginTop: 24 }}>
              {t('leasesHistoryTitle')}
            </Typography.Title>
            <Space style={{ marginBottom: 16 }} wrap>
              <Select
                value={historyState}
                onChange={(value) => setHistoryState(value as LeaseRecord['state'] | 'all')}
                options={stateSelectOptions}
                style={{ minWidth: 140 }}
              />
              <RangePicker
                showTime
                value={historyRange ?? undefined}
                onChange={(value) => setHistoryRange(value)}
                allowClear
              />
              <Button
                icon={<DownloadOutlined />}
                onClick={() => exportHistory.mutate()}
                loading={exportHistory.isPending}
                disabled={!historyParams}
              >
                {t('leasesHistoryExportCsv')}
              </Button>
            </Space>
            <List
              loading={historyLoading}
              dataSource={leaseHistory?.items ?? []}
              locale={{ emptyText: t('leasesHistoryEmpty') }}
              renderItem={(item) => (
                <List.Item>
                  <List.Item.Meta
                    title={
                      <Space size={8} wrap>
                        <Tag color={stateColors[item.state as LeaseRecord['state']] ?? 'default'}>
                          {leaseStateLabels[item.state as LeaseRecord['state']] ?? item.state}
                        </Tag>
                        <Typography.Text>{dayjs(item.updatedAt).format('YYYY-MM-DD HH:mm:ss')}</Typography.Text>
                      </Space>
                    }
                    description={
                      <Space direction="vertical" size={2}>
                        <Typography.Text type="secondary">
                          {t('leasesHistorySecurityLine', {
                            security: item.securityState || '—',
                            client: item.clientId || '—',
                          })}
                        </Typography.Text>
                        <Typography.Text type="secondary">
                          {t('leasesHistoryExpiryLine', {
                            time: dayjs(item.expiresAt).format('YYYY-MM-DD HH:mm:ss'),
                          })}
                        </Typography.Text>
                      </Space>
                    }
                  />
                  <Space direction="vertical" align="end" size={2}>
                    <Typography.Text type="secondary">
                      {t('leasesHistoryMacLine', { mac: item.identifier ?? '—' })}
                    </Typography.Text>
                    {item.cooldownUntil && (
                      <Tag color="purple">
                        {t('leasesHistoryCooldownTag', {
                          time: dayjs(item.cooldownUntil).format('MM-DD HH:mm'),
                        })}
                      </Tag>
                    )}
                  </Space>
                </List.Item>
              )}
            />
            {lastExport && (
              <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
                {t('leasesLastExportLabel', {
                  name: lastExport.name,
                  time: dayjs(lastExport.generatedAt).fromNow(),
                })}
                <Button
                  size="small"
                  style={{ marginLeft: 8 }}
                  onClick={async () => {
                    await navigator.clipboard.writeText(lastExport.path)
                    message.success(t('leasesCopyPathSuccess'))
                  }}
                >
                  {t('leasesCopyPathButton')}
                </Button>
              </Typography.Paragraph>
            )}
            <Divider />
            <Typography.Title level={5}>{t('leasesScheduleTitle')}</Typography.Title>
            <Typography.Paragraph type="secondary">
              {t('leasesScheduleDescription')}
            </Typography.Paragraph>
            <Form
              form={scheduleForm}
              layout="vertical"
              initialValues={{ destination: 'ops-drops', format: 'csv', intervalHours: 24, limit: 1000 }}
              onFinish={() => scheduleMutation.mutate()}
              requiredMark={false}
            >
              <Form.Item
                label={t('leasesScheduleDestinationLabel')}
                name="destination"
                rules={[{ required: true, message: t('leasesScheduleDestinationRequired') }]}
              >
                <Input placeholder={t('leasesScheduleDestinationPlaceholder')} allowClear />
              </Form.Item>
              <Form.Item label={t('leasesScheduleFormatLabel')} name="format">
                <Select
                  options={[
                    { label: 'CSV', value: 'csv' },
                    { label: 'PDF', value: 'pdf' },
                  ]}
                />
              </Form.Item>
              <Form.Item
                label={t('leasesScheduleIntervalLabel')}
                name="intervalHours"
                rules={[{ required: true, message: t('leasesScheduleIntervalRequired') }]}
              >
                <Select
                  options={[
                    { label: t('leasesScheduleIntervalOptionHours', { hours: 6 }), value: 6 },
                    { label: t('leasesScheduleIntervalOptionHours', { hours: 12 }), value: 12 },
                    { label: t('leasesScheduleIntervalOptionHours', { hours: 24 }), value: 24 },
                  ]}
                />
              </Form.Item>
              <Form.Item label={t('leasesScheduleLimitLabel')} name="limit">
                <InputNumber min={100} max={2000} step={100} style={{ width: '100%' }} />
              </Form.Item>
              <Form.Item>
                <Button
                  type="primary"
                  icon={<ScheduleOutlined />}
                  htmlType="submit"
                  loading={scheduleMutation.isPending}
                  disabled={!historyParams}
                >
                  {t('leasesScheduleButton')}
                </Button>
              </Form.Item>
            </Form>
            {scheduleJob && (
              <Typography.Paragraph type="secondary">
                {t('leasesScheduleJobSummary', {
                  jobId: scheduleJob.jobId,
                  nextRun: dayjs(scheduleJob.nextRun).format('YYYY-MM-DD HH:mm'),
                  interval: scheduleJob.interval,
                })}
              </Typography.Paragraph>
            )}
          </>
        ) : (
          <Empty description={t('leasesDrawerEmpty')} />
        )}
      </Drawer>
    </Row>
  )
}
