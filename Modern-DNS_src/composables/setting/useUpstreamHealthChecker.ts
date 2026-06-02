import { computed, onBeforeUnmount, ref } from 'vue'
import type { ForwardServer } from '../../types/modules'
import { runLatencyTestApi, type LatencySummary } from '../../api/forward'

export type HealthStatus = 'online' | 'slow' | 'timeout' | 'checking' | 'idle'

export interface ServerHealthRecord {
  serverId: number
  status: HealthStatus
  latencyMs: number | null
  checkedAt: string | null
  errorCount: number
  lastError?: string
}

const fmt = (): string => new Date().toLocaleTimeString('zh-CN', { hour12: false })

const latencyToStatus = (ms: number | null): HealthStatus => {
  if (ms === null) return 'timeout'
  if (ms < 200) return 'online'
  return 'slow'
}

export function useUpstreamHealthChecker() {
  const healthMap = ref<Map<number, ServerHealthRecord>>(new Map())
  const checking = ref(false)
  const serverSummary = ref<LatencySummary | null>(null)
  let pollTimer: ReturnType<typeof setInterval> | null = null

  const getRecord = (id: number): ServerHealthRecord =>
    healthMap.value.get(id) ?? { serverId: id, status: 'idle', latencyMs: null, checkedAt: null, errorCount: 0 }

  const runCheck = async (servers: ForwardServer[]): Promise<void> => {
    if (!servers.length || checking.value) return
    checking.value = true

    // Mark all as checking
    for (const s of servers) {
      const prev = getRecord(s.id)
      healthMap.value.set(s.id, { ...prev, status: 'checking' })
    }

    try {
      // Forward the full per-row context — port and protocol — so the
      // backend can probe DoT/DoH/TCP/UDP upstreams over the same
      // transport the forwarder will actually use at query time.
      // Without this the previous frontend was silently hammering UDP
      // on port 53 against DoH-only endpoints and reporting them dead.
      const targets = servers.map((s) => ({
        id: s.id,
        label: s.name,
        ip: s.address,
        port: s.port,
        protocol: s.protocol,
      }))
      const { data } = await runLatencyTestApi(targets)
      const results = data?.results ?? []
      serverSummary.value = data?.summary ?? null
      const now = fmt()

      // Index by id when present (preferred — survives reordering),
      // else fall back to positional matching for backwards compat.
      const byId = new Map<number, typeof results[number]>()
      for (const r of results) {
        if (typeof r.id === 'number' && r.id > 0) byId.set(r.id, r)
      }

      for (let i = 0; i < servers.length; i++) {
        const s = servers[i]
        const r = byId.get(s.id) ?? results[i]
        const prev = getRecord(s.id)
        if (r) {
          const ms = r.latencyMs ?? null
          const status = r.status === 'fail' ? 'timeout' : latencyToStatus(ms)
          const errorCount = status === 'online' ? 0 : prev.errorCount + 1
          healthMap.value.set(s.id, { serverId: s.id, status, latencyMs: ms, checkedAt: now, errorCount, lastError: r.error })
        } else {
          healthMap.value.set(s.id, { serverId: s.id, status: 'timeout', latencyMs: null, checkedAt: now, errorCount: prev.errorCount + 1 })
        }
      }
    } catch {
      // On API failure, mark all as idle
      for (const s of servers) {
        healthMap.value.set(s.id, { serverId: s.id, status: 'idle', latencyMs: null, checkedAt: fmt(), errorCount: 0 })
      }
      serverSummary.value = null
    } finally {
      checking.value = false
    }
  }

  const startPolling = (getServers: () => ForwardServer[], intervalMs = 30_000): void => {
    stopPolling()
    runCheck(getServers())
    pollTimer = setInterval(() => runCheck(getServers()), intervalMs)
  }

  const stopPolling = (): void => {
    if (pollTimer) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  }

  const statusLabel = (id: number): string => {
    const s = getRecord(id).status
    return { online: '在线', slow: '慢响应', timeout: '超时', checking: '检测中', idle: '待检测' }[s] ?? '未知'
  }

  const statusColor = (id: number): string => {
    const s = getRecord(id).status
    return { online: 'var(--app-success)', slow: '#f59e0b', timeout: 'var(--app-danger)', checking: 'var(--app-accent)', idle: 'var(--app-text-secondary)' }[s] ?? '#999'
  }

  const latencyText = (id: number): string => {
    const r = getRecord(id)
    if (r.status === 'checking') return '...'
    if (r.latencyMs === null) return '超时'
    return `${r.latencyMs} ms`
  }

  const allRecords = computed<ServerHealthRecord[]>(() => [...healthMap.value.values()])

  // Prefer the authoritative summary from the latest backend probe so
  // KPIs in the strip can't drift from the per-card statuses (the
  // client-side aggregation would race with mid-flight "checking"
  // states). Fall back to derived values when no probe has run yet.
  const summary = computed(() => {
    if (serverSummary.value) {
      return {
        total: serverSummary.value.total,
        online: serverSummary.value.online,
        slow: serverSummary.value.slow,
        timeout: serverSummary.value.timeout,
        avgLatency: serverSummary.value.avgLatency,
      }
    }
    const vals = allRecords.value
    return {
      total: vals.length,
      online: vals.filter((r) => r.status === 'online').length,
      slow: vals.filter((r) => r.status === 'slow').length,
      timeout: vals.filter((r) => r.status === 'timeout').length,
      avgLatency: (() => {
        const lat = vals.filter((r) => r.latencyMs !== null).map((r) => r.latencyMs as number)
        return lat.length ? Math.round(lat.reduce((a, b) => a + b, 0) / lat.length) : null
      })(),
    }
  })

  onBeforeUnmount(stopPolling)

  return {
    healthMap,
    checking,
    allRecords,
    summary,
    getRecord,
    runCheck,
    startPolling,
    stopPolling,
    statusLabel,
    statusColor,
    latencyText,
  }
}
