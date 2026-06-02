import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { getAlerts, handleAlertEvent, markAllAlertsRead } from '../api/dashboard'

export interface AlertNotice {
  id: number
  level: string
  type: string
  domain: string
  content: string
  status: string
  read: boolean
  triggeredAt: string
  updatedAt: string
}

export const useAlertStore = defineStore('alert', () => {
  const notices = ref<AlertNotice[]>([])
  const loading = ref(false)

  const unreadCount = computed(() => notices.value.filter(n => !n.read).length)

  const fetchAlerts = async () => {
    loading.value = true
    try {
      const res = await getAlerts()
      const data = (res as any).data ?? res
      const rows = data?.rows ?? []
      notices.value = rows.map((r: any) => ({
        id: r.id,
        level: r.level || 'info',
        type: r.type || '',
        domain: r.domain || '',
        content: r.content || '',
        status: r.status || '未读',
        read: !!r.read,
        triggeredAt: r.triggeredAt || '',
        updatedAt: r.updatedAt || '',
      }))
    } finally {
      loading.value = false
    }
  }

  const markRead = (id: number) => {
    const item = notices.value.find(n => n.id === id)
    if (item) item.read = true
  }

  const markAllRead = async () => {
    await markAllAlertsRead()
    notices.value.forEach(n => { n.read = true })
  }

  const handleAlert = async (id: number) => {
    await handleAlertEvent(id)
    const item = notices.value.find(n => n.id === id)
    if (item) {
      item.status = '已处理'
      item.read = true
    }
  }

  const clearHandled = () => {
    notices.value = notices.value.filter(n => n.status !== '已处理')
  }

  return {
    notices,
    loading,
    unreadCount,
    fetchAlerts,
    markRead,
    markAllRead,
    handleAlert,
    clearHandled,
  }
})
