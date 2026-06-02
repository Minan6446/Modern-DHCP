import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import * as zoneApi from '../api/zone'
import type { DomainDetail, DomainRecord, DomainSoa, DomainZone } from '../types/modules'

const EMPTY_DETAIL = (): DomainDetail => ({
  records: [],
  soa: { mname: '', rname: '', refresh: 3600, retry: 600, expire: 1209600, minimumTtl: 300 },
  dnssec: { enabled: false, ksk: [], zsk: [] },
})

type ZoneCreatePayload = Pick<DomainZone, 'zoneId' | 'domain' | 'type' | 'remark' | 'upstream'>

export const useDomainStore = defineStore('domain', () => {
  const loading = ref(false)
  const submitting = ref(false)
  const zones = ref<DomainZone[]>([])
  const details = ref<Record<string, DomainDetail>>({})
  const lastUpdated = ref('')
  let timerId: number | null = null
  // De-duplicates concurrent fetchDetail(id) calls so we never fan out N
  // identical requests when the page mounts and effects run in parallel.
  const detailInflight = new Map<string, Promise<void>>()

  /* ── fetch ── */
  const fetchDomainData = async (clearDetails = false) => {
    loading.value = true
    try {
      const { data } = await zoneApi.getZones({ pageSize: 999 })
      zones.value = data.list
      if (clearDetails) details.value = {}
      lastUpdated.value = new Date().toLocaleString('zh-CN', { hour12: false })
    } finally {
      loading.value = false
    }
  }

  /* ── detail load ──
   *
   * Always re-fetches from the backend so the records list never falls out
   * of sync with the database (e.g. after another tab/admin/import path
   * mutates records). Concurrent calls for the same id share one request via
   * detailInflight; cached `details.value[key]` is left in place during the
   * fetch so the table doesn't visibly empty before the response arrives.
   */
  const fetchDetail = async (id: number | string | string[]): Promise<void> => {
    const key = String(Array.isArray(id) ? id[0] : id)
    const inflight = detailInflight.get(key)
    if (inflight) return inflight
    const p = (async () => {
      try {
        const { data } = await zoneApi.getZoneDetail(key)
        details.value[key] = data
      } finally {
        detailInflight.delete(key)
      }
    })()
    detailInflight.set(key, p)
    return p
  }

  /* ── polling ── */
  const startPolling = () => {
    stopPolling()
    timerId = window.setInterval(fetchDomainData, 30000)
  }

  const stopPolling = () => {
    if (timerId) { window.clearInterval(timerId); timerId = null }
  }

  /* ── selectors ── */
  const getZoneById = (id: number | string | string[]): DomainZone | undefined =>
    zones.value.find((item) => String(item.id) === String(Array.isArray(id) ? id[0] : id))

  const getDetailById = (id: number | string | string[]): DomainDetail =>
    details.value[String(Array.isArray(id) ? id[0] : id)] ?? EMPTY_DETAIL()

  const sortedZones = computed(() => [...zones.value])

  /* ── Zone CRUD ── */
  const createZone = async (payload: ZoneCreatePayload): Promise<DomainZone> => {
    submitting.value = true
    try {
      const { data } = await zoneApi.createZone({
        zoneId: payload.zoneId,
        domain: payload.domain,
        type: payload.type,
        remark: payload.remark,
        upstream: payload.upstream,
      })
      zones.value.unshift(data)
      details.value[data.id] = EMPTY_DETAIL()
      return data
    } finally {
      submitting.value = false
    }
  }

  const updateZone = async (id: number | string | string[], payload: Partial<DomainZone>): Promise<void> => {
    submitting.value = true
    try {
      const key = String(Array.isArray(id) ? id[0] : id)
      const { data } = await zoneApi.updateZone(key, payload)
      const zone = getZoneById(key)
      if (zone) Object.assign(zone, data)
    } finally {
      submitting.value = false
    }
  }

  const batchUpdateZoneStatus = async (ids: Array<number | string>, status: string): Promise<void> => {
    await zoneApi.batchUpdateZoneStatus(ids, status)
    const set = new Set(ids.map(String))
    zones.value.forEach((item) => { if (set.has(String(item.id))) item.status = status })
  }

  const deleteZones = async (ids: Array<number | string>): Promise<void> => {
    await zoneApi.deleteZones({ ids: ids.map(Number) })
    const set = new Set(ids.map(String))
    zones.value = zones.value.filter((item) => !set.has(String(item.id)))
    Object.keys(details.value).forEach((key) => { if (set.has(key)) delete details.value[key] })
  }

  /* ── SOA ── */
  const saveSoa = async (id: number | string | string[], payload: DomainSoa): Promise<void> => {
    submitting.value = true
    try {
      const key = String(Array.isArray(id) ? id[0] : id)
      await zoneApi.updateSoa(key, payload)
      if (details.value[key]) details.value[key].soa = { ...payload }
    } finally {
      submitting.value = false
    }
  }

  /* ── Records ── */
  const saveRecord = async (
    id: number | string | string[],
    payload: Omit<DomainRecord, 'id' | 'updatedAt'> & Partial<Pick<DomainRecord, 'id'>>,
  ): Promise<void> => {
    submitting.value = true
    const key = String(Array.isArray(id) ? id[0] : id)
    try {
      const body: zoneApi.RecordUpsertPayload = {
        type: payload.type,
        host: payload.host,
        value: payload.value,
        ttl: payload.ttl,
        status: payload.status as '启用' | '禁用',
        remark: payload.remark ?? '',
      }
      if (payload.id) {
        const { data } = await zoneApi.updateRecord(key, payload.id, body)
        const bucket = details.value[key]?.records ?? []
        const target = bucket.find((r) => r.id === payload.id)
        if (target) Object.assign(target, data)
      } else {
        const { data } = await zoneApi.createRecord(key, body)
        if (!details.value[key]) details.value[key] = EMPTY_DETAIL()
        details.value[key].records.unshift(data)
      }
    } finally {
      submitting.value = false
    }
  }

  const deleteRecords = async (id: number | string | string[], recordIds: number[]): Promise<void> => {
    const key = String(Array.isArray(id) ? id[0] : id)
    await zoneApi.deleteRecords(key, { ids: recordIds })
    const set = new Set(recordIds.map(Number))
    if (details.value[key]) {
      details.value[key].records = details.value[key].records.filter((item) => !set.has(item.id))
    }
  }

  const toggleRecordStatus = async (
    id: number | string | string[],
    recordId: number,
    status: string,
  ): Promise<void> => {
    const key = String(Array.isArray(id) ? id[0] : id)
    await zoneApi.updateRecordStatus(key, recordId, { status: status as '启用' | '禁用' })
    const target = details.value[key]?.records.find((item) => item.id === recordId)
    if (target) {
      target.status = status
      target.updatedAt = new Date().toLocaleString('zh-CN', { hour12: false })
    }
  }

  /* ── DNSSEC ── */
  const updateDnssec = async (id: number | string | string[], enabled: boolean): Promise<void> => {
    const key = String(Array.isArray(id) ? id[0] : id)
    await zoneApi.toggleDnssec(key, { enabled })
    if (details.value[key]) details.value[key].dnssec.enabled = enabled
  }

  const rotateDnssecKey = async (id: number | string | string[], keyType: 'ksk' | 'zsk'): Promise<void> => {
    const key = String(Array.isArray(id) ? id[0] : id)
    const { data } = await zoneApi.rotateDnssecKey(key, keyType)
    const list = details.value[key]?.dnssec[keyType]
    if (list) {
      list.unshift({
        id: Number(data.id),
        keyId: (data as any).keyId ?? `${keyType.toUpperCase()}-${data.id}`,
        keyTag: (data as any).keyTag ?? 0,
        algorithm: (data as any).algorithm ?? 'ECDSAP256SHA256',
        ds: (data as any).ds ?? '',
        createdAt: new Date().toLocaleString('zh-CN', { hour12: false }),
        status: '生效中',
      })
    }
  }

  return {
    loading,
    submitting,
    zones,
    details,
    lastUpdated,
    sortedZones,
    fetchDomainData,
    fetchDetail,
    startPolling,
    stopPolling,
    getZoneById,
    getDetailById,
    createZone,
    updateZone,
    batchUpdateZoneStatus,
    deleteZones,
    saveSoa,
    saveRecord,
    deleteRecords,
    toggleRecordStatus,
    updateDnssec,
    rotateDnssecKey,
  }
})