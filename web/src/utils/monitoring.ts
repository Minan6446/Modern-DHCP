import type { MetricPoint, PerfSnapshot } from '@/types/monitoring';

export const rollingAppend = (series: MetricPoint[], point: MetricPoint, max = 60) => {
  const next = [...series, point];
  if (next.length > max) next.shift();
  return next;
};

export const toMetricPoint = (value: number) => ({ value, timestamp: new Date().toISOString() });

export const normalizeSnapshot = (snap: Partial<PerfSnapshot>): PerfSnapshot => ({
  pps: snap.pps ?? 0,
  cpu: snap.cpu ?? 0,
  memory: snap.memory ?? 0,
  disk: snap.disk ?? 0,
  latencyP50: snap.latencyP50 ?? 0,
  latencyP95: snap.latencyP95 ?? 0,
  latencyP99: snap.latencyP99 ?? 0,
  successRate: snap.successRate ?? 0
});
