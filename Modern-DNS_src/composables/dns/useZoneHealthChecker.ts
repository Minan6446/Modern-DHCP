import { computed, onBeforeUnmount, ref } from 'vue'
import { useDomainStore } from '../../stores/domain'

export type HealthLevel = 'healthy' | 'warning' | 'error' | 'unknown'

export interface HealthCheckItem {
  key: string
  label: string
  status: HealthLevel
  message: string
}

export interface ZoneHealth {
  level: HealthLevel
  items: HealthCheckItem[]
  checkedAt: number | null
}

const POLL_INTERVAL = 30_000

export function useZoneHealthChecker(zoneId: () => string | number | string[]) {
  const domainStore = useDomainStore()
  const checking = ref(false)
  const health = ref<ZoneHealth>({ level: 'unknown', items: [], checkedAt: null })
  let timer: ReturnType<typeof setInterval> | null = null

  const levelOrder: Record<HealthLevel, number> = { healthy: 0, unknown: 1, warning: 2, error: 3 }

  const _worstLevel = (items: HealthCheckItem[]): HealthLevel => {
    if (!items.length) return 'unknown'
    return items.reduce<HealthLevel>((worst, item) => {
      return levelOrder[item.status] > levelOrder[worst] ? item.status : worst
    }, 'healthy')
  }

  const check = async () => {
    if (checking.value) return
    checking.value = true
    try {
      const detail = domainStore.getDetailById(zoneId())
      const zone = domainStore.getZoneById(zoneId())
      const items: HealthCheckItem[] = []

      // 1. Zone status
      items.push({
        key: 'zone-status',
        label: 'Zone 运行状态',
        status: zone?.status === '正常' ? 'healthy' : zone?.status === '同步中' ? 'warning' : 'error',
        message: zone?.status === '正常' ? 'Zone 运行正常' : `当前状态：${zone?.status ?? '未知'}`,
      })

      // 2. SOA existence
      const soa = detail?.soa
      items.push({
        key: 'soa',
        label: 'SOA 记录',
        status: soa?.mname ? 'healthy' : 'error',
        message: soa?.mname ? `主 NS：${soa.mname}` : '缺少 SOA 主域名服务器配置',
      })

      // 3. Record count
      const count = detail?.records?.length ?? 0
      items.push({
        key: 'record-count',
        label: '解析记录数',
        status: count === 0 ? 'warning' : 'healthy',
        message: count === 0 ? '该 Zone 暂无解析记录' : `共 ${count} 条记录`,
      })

      // 4. DNSSEC
      const dnssec = detail?.dnssec
      if (dnssec?.enabled) {
        const hasKsk = dnssec.ksk?.length > 0
        const hasZsk = dnssec.zsk?.length > 0
        items.push({
          key: 'dnssec',
          label: 'DNSSEC 密钥',
          status: hasKsk && hasZsk ? 'healthy' : 'warning',
          message: hasKsk && hasZsk ? 'KSK / ZSK 均已就绪' : `缺少 ${!hasKsk ? 'KSK' : 'ZSK'} 密钥`,
        })
      } else {
        items.push({
          key: 'dnssec',
          label: 'DNSSEC 密钥',
          status: 'unknown',
          message: 'DNSSEC 未启用',
        })
      }

      // 5. Disabled records ratio
      const records = detail?.records ?? []
      const disabled = records.filter((r) => r.status === '禁用').length
      const ratio = records.length ? disabled / records.length : 0
      items.push({
        key: 'disabled-ratio',
        label: '禁用记录占比',
        status: ratio > 0.5 ? 'warning' : 'healthy',
        message: ratio > 0.5 ? `${disabled}/${records.length} 条记录已禁用` : `${disabled} 条记录已禁用`,
      })

      health.value = {
        level: _worstLevel(items),
        items,
        checkedAt: Date.now(),
      }
    } finally {
      checking.value = false
    }
  }

  const startPolling = () => {
    check()
    timer = setInterval(check, POLL_INTERVAL)
  }

  const stopPolling = () => {
    if (timer) { clearInterval(timer); timer = null }
  }

  onBeforeUnmount(stopPolling)

  const healthBadgeClass = computed((): string => {
    const map: Record<HealthLevel, string> = {
      healthy: 'mn-badge--success',
      warning: 'mn-badge--warning',
      error: 'mn-badge--danger',
      unknown: 'mn-badge--neutral',
    }
    return map[health.value.level]
  })

  const healthLabel = computed((): string => {
    const map: Record<HealthLevel, string> = {
      healthy: '健康',
      warning: '警告',
      error: '异常',
      unknown: '未检测',
    }
    return map[health.value.level]
  })

  return {
    checking,
    health,
    check,
    startPolling,
    stopPolling,
    healthBadgeClass,
    healthLabel,
  }
}
