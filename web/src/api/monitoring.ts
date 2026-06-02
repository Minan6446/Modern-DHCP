import type { ApiResponse, PageQuery, PageResult } from '@/types/system';
import type {
  MetricPoint,
  PerfSnapshot,
  ServerPerformanceMetric,
  AllocationBreakdown,
  PoolUsageSummary,
  PoolUsageSummarySnapshot,
  LeaseStatusSummary,
  AnomalyRecord,
  SyntheticCheck,
  AbnormalRequestStat,
  AlertRule,
  AlertEvent,
  NotificationChannel,
  ReportRequest,
  ReportTask,
  AlertActiveSummary,
  AlertTrendPoint,
  AlertHistorySummary,
  AlertHistoryCompare,
  AlertThresholdConfig,
  AlertNotifyConfig,
  AlertTemplate,
  AlertReceiver,
  AlertAnalyticsSummary,
  AlertTypeDistributionItem,
  AlertTimeDistributionItem,
  AlertTrendCompare
} from '@/types/monitoring';
import type { LeaseInsights } from '@/types/lease';
import { httpClient } from '@/shared/api-client/http';
import { normalizePageResult } from '@/shared/api-client/page';

const normalize = <T>(payload: any): ApiResponse<T> =>
  payload && typeof payload === 'object' && 'code' in payload && 'data' in payload
    ? (payload as ApiResponse<T>)
    : ({ code: 0, message: '', data: payload as T } as ApiResponse<T>);

export const getRealtimeSnapshot = (params?: { tenantId?: string }) =>
  httpClient
    .get<ApiResponse<PerfSnapshot> | PerfSnapshot>('/monitoring/realtime/snapshot', { params })
    .then((response) => ({ ...response, data: normalize<PerfSnapshot>(response.data) }));
export const getMetricSeries = (
  metric: string,
  params: PageQuery & { range?: string; tenantId?: string }
) => httpClient.get<ApiResponse<MetricPoint[]>>(`/monitoring/metrics/${metric}`, { params });
export const getLatencyDistribution = (params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<Array<{ bucket: string; value: number }>>>('/monitoring/latency', {
    params
  });
export const listServerPerformance = (params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<ServerPerformanceMetric[]>>('/monitoring/servers/perf', { params });
export const getAllocationBreakdown = (params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<AllocationBreakdown>>('/monitoring/allocations/breakdown', { params });

export const listPoolUsage = (params?: { tenantId?: string }) =>
  httpClient
    .get<ApiResponse<PoolUsageSummary[]> | PoolUsageSummary[] | { pools: PoolUsageSummary[] }>(
      '/monitoring/pools/usage',
      { params }
    )
    .then((response) => {
      const payload: any = response.data;
      const normalized = Array.isArray(payload)
        ? payload
        : Array.isArray(payload?.data)
          ? payload.data
          : Array.isArray(payload?.pools)
            ? payload.pools
            : [];
      return {
        ...response,
        data: { code: 0, message: '', data: normalized } as ApiResponse<PoolUsageSummary[]>
      };
    });

export const getPoolUsageSummary = (params?: { tenantId?: string }) =>
  httpClient
    .get<ApiResponse<{ summary: PoolUsageSummarySnapshot }> | { summary: PoolUsageSummarySnapshot }>(
      '/monitoring/pools/summary',
      { params }
    )
    .then((response) => ({
      ...response,
      data: normalize<{ summary: PoolUsageSummarySnapshot }>(response.data)
    }));
export const getLeaseStatus = (params?: { tenantId?: string }) =>
  httpClient
    .get<ApiResponse<LeaseStatusSummary> | LeaseStatusSummary>('/monitoring/leases/status', {
      params
    })
    .then((response) => ({ ...response, data: normalize<LeaseStatusSummary>(response.data) }));
export const listAnomalies = (params?: { tenantId?: string; status?: string; severity?: string }) =>
  httpClient
    .get<ApiResponse<AnomalyRecord[]> | AnomalyRecord[]>('/monitoring/anomalies', { params })
    .then((response) => ({ ...response, data: normalize<AnomalyRecord[]>(response.data) }));
export const listSyntheticChecks = (params?: { tenantId?: string }) =>
  httpClient
    .get<ApiResponse<SyntheticCheck[]> | SyntheticCheck[]>('/monitoring/synthetic', { params })
    .then((response) => ({ ...response, data: normalize<SyntheticCheck[]>(response.data) }));
export const listAbnormalRequests = (params?: {
  tenantId?: string;
  type?: string;
  window?: string;
}) =>
  httpClient
    .get<ApiResponse<AbnormalRequestStat[]> | AbnormalRequestStat[]>(
      '/monitoring/requests/abnormal',
      { params }
    )
    .then((response) => ({ ...response, data: normalize<AbnormalRequestStat[]>(response.data) }));

export const listAlertRules = (params?: PageQuery & { tenantId?: string; keyword?: string }) =>
  httpClient
    .get<
      ApiResponse<PageResult<AlertRule> | AlertRule[]> | PageResult<AlertRule> | AlertRule[]
    >('/monitoring/alerts/rules', {
      params
    })
    .then((response) => ({ ...response, data: normalizePageResult<AlertRule>(response.data, params) }));
export const saveAlertRule = (payload: Partial<AlertRule> & { tenantId?: string }) =>
  httpClient.post<ApiResponse<AlertRule>>('/monitoring/alerts/rules', payload);
export const deleteAlertRule = (id: string, params?: { tenantId?: string }) =>
  httpClient.delete<ApiResponse<void>>(`/monitoring/alerts/rules/${id}`, { params });
export const listAlertEvents = (
  params?: PageQuery & { status?: string; severity?: string; tenantId?: string },
  signal?: AbortSignal
) =>
  httpClient
    .get<
      ApiResponse<PageResult<AlertEvent> | AlertEvent[]> | PageResult<AlertEvent> | AlertEvent[]
    >('/monitoring/alerts/events', {
      params,
      signal
    })
    .then((response) => ({ ...response, data: normalizePageResult<AlertEvent>(response.data, params) }));

export const acknowledgeAlertEvent = (
  alertId: string,
  payload?: { assignee?: string; ttlSeconds?: number; tenantId?: string }
) => httpClient.post<ApiResponse<any>>(`/monitoring/alerts/${alertId}/ack`, payload || {});

export const resolveAlertEvent = (
  alertId: string,
  payload?: { channel?: string; ttlSeconds?: number; assignee?: string; tenantId?: string }
) => httpClient.post<ApiResponse<any>>(`/monitoring/alerts/${alertId}/suppress`, payload || {});

export const listChannels = (params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<NotificationChannel[]>>('/monitoring/notifications/channels', {
    params
  });
export const saveChannel = (payload: Partial<NotificationChannel> & { tenantId?: string }) =>
  httpClient.post<ApiResponse<NotificationChannel>>('/monitoring/notifications/channels', payload);

// New alert alignment APIs
export const getAlertActiveSummary = (params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<AlertActiveSummary>>('/monitoring/alerts/active/summary', {
    params
  });

export const getAlertActiveTrend = (params?: { window?: string; step?: string; tenantId?: string }) =>
  httpClient.get<ApiResponse<AlertTrendPoint[]>>('/monitoring/alerts/active/trend', { params });

export const getAlertHistorySummary = (params?: { from?: string; to?: string; tenantId?: string }) =>
  httpClient.get<ApiResponse<AlertHistorySummary>>('/monitoring/alerts/history/summary', { params });

export const getAlertHistoryCompare = (params?: { range?: string; from?: string; to?: string; tenantId?: string }) =>
  httpClient.get<ApiResponse<AlertHistoryCompare>>('/monitoring/alerts/history/compare', { params });

export const getAlertThresholdConfig = (params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<AlertThresholdConfig>>('/monitoring/alerts/config/thresholds', { params });

export const saveAlertThresholdConfig = (payload: AlertThresholdConfig & { tenantId?: string }) =>
  httpClient.post<ApiResponse<void>>('/monitoring/alerts/config/thresholds', payload);

export const getAlertNotifyConfig = (params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<AlertNotifyConfig>>('/monitoring/alerts/config/notify', { params });

export const saveAlertNotifyConfig = (payload: AlertNotifyConfig & { tenantId?: string }) =>
  httpClient.post<ApiResponse<void>>('/monitoring/alerts/config/notify', payload);

export const listAlertTemplates = (params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<AlertTemplate[]>>('/monitoring/alerts/templates', { params });

export const createAlertTemplate = (payload: Partial<AlertTemplate> & { tenantId?: string }) =>
  httpClient.post<ApiResponse<AlertTemplate>>('/monitoring/alerts/templates', payload);

export const updateAlertTemplate = (id: string, payload: Partial<AlertTemplate> & { tenantId?: string }) =>
  httpClient.put<ApiResponse<AlertTemplate>>(`/monitoring/alerts/templates/${id}`, payload);

export const deleteAlertTemplate = (id: string, params?: { tenantId?: string }) =>
  httpClient.delete<ApiResponse<void>>(`/monitoring/alerts/templates/${id}`, { params });

export const listAlertReceivers = (params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<AlertReceiver[]>>('/monitoring/alerts/receivers', { params });

export const createAlertReceiver = (payload: Partial<AlertReceiver> & { tenantId?: string }) =>
  httpClient.post<ApiResponse<AlertReceiver>>('/monitoring/alerts/receivers', payload);

export const updateAlertReceiver = (id: string, payload: Partial<AlertReceiver> & { tenantId?: string }) =>
  httpClient.put<ApiResponse<AlertReceiver>>(`/monitoring/alerts/receivers/${id}`, payload);

export const deleteAlertReceiver = (id: string, params?: { tenantId?: string }) =>
  httpClient.delete<ApiResponse<void>>(`/monitoring/alerts/receivers/${id}`, { params });

export const importAlertReceivers = (payload: FormData, params?: { tenantId?: string }) =>
  httpClient.post<ApiResponse<void>>('/monitoring/alerts/receivers/import', payload, { params });

export const exportAlertReceivers = (params?: { tenantId?: string }) =>
  httpClient.get<Blob>('/monitoring/alerts/receivers/export', { params, responseType: 'blob' as any });

export const getAlertAnalyticsSummary = (params?: { range?: string; tenantId?: string }) =>
  httpClient.get<ApiResponse<AlertAnalyticsSummary>>('/monitoring/alerts/analytics/summary', { params });

export const getAlertTypeDistribution = (params?: { range?: string; tenantId?: string }) =>
  httpClient.get<ApiResponse<AlertTypeDistributionItem[]>>('/monitoring/alerts/analytics/type-distribution', {
    params
  });

export const getAlertTimeDistribution = (params?: { range?: string; step?: string; tenantId?: string }) =>
  httpClient.get<ApiResponse<AlertTimeDistributionItem[]>>('/monitoring/alerts/analytics/time-distribution', {
    params
  });

export const getAlertTrendCompare = (params?: { range?: string; compare?: string; tenantId?: string }) =>
  httpClient.get<ApiResponse<AlertTrendCompare>>('/monitoring/alerts/analytics/trend', { params });

export const createReportTask = (payload: ReportRequest & { tenantId?: string }) =>
  httpClient.post<ApiResponse<ReportTask>>('/reports', payload);
export const listReportTasks = (
  params?: PageQuery & { tenantId?: string; status?: string; type?: string },
  signal?: AbortSignal
) =>
  httpClient
    .get<ApiResponse<PageResult<ReportTask> | ReportTask[]> | PageResult<ReportTask> | ReportTask[]>('/reports', {
      params,
      signal
    })
    .then((response) => ({ ...response, data: normalizePageResult<ReportTask>(response.data, params) }));

export const getLeaseInsights = (params?: { tenantId?: string }) =>
  httpClient
    .get<ApiResponse<LeaseInsights> | LeaseInsights>('/leases/insights', { params })
    .then((response) => ({ ...response, data: normalize<LeaseInsights>(response.data) }));
