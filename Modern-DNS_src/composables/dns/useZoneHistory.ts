import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import type { DomainRecord } from '../../types/modules'
import { useDomainStore } from '../../stores/domain'
import { confirmRiskAction } from '../../utils/interaction'

export interface RecordSnapshot {
  id: string
  timestamp: number
  label: string
  records: DomainRecord[]
}

const MAX_HISTORY = 20

export function useZoneHistory(zoneId: () => string | number | string[]) {
  const { t } = useI18n()
  const domainStore = useDomainStore()
  const history = ref<RecordSnapshot[]>([])
  const pointer = ref(-1)

  const canUndo = computed(() => pointer.value > 0)
  const canRedo = computed(() => pointer.value < history.value.length - 1)
  const currentSnapshot = computed<RecordSnapshot | null>(() => history.value[pointer.value] ?? null)

  const _push = (label: string, records: DomainRecord[]) => {
    const snap: RecordSnapshot = {
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      timestamp: Date.now(),
      label,
      records: records.map((r) => ({ ...r })),
    }
    // discard any redo branch
    if (pointer.value < history.value.length - 1) {
      history.value.splice(pointer.value + 1)
    }
    history.value.push(snap)
    if (history.value.length > MAX_HISTORY) history.value.shift()
    pointer.value = history.value.length - 1
  }

  /** Call before any mutating action to record the current state */
  const snapshot = (label: string) => {
    const current = domainStore.getDetailById(zoneId()).records
    _push(label, current)
  }

  const undo = async () => {
    if (!canUndo.value) return
    const targetLabel = history.value[pointer.value - 1]?.label ?? ''
    await confirmRiskAction({
      title: t('record.undoConfirmTitle'),
      action: t('record.undoConfirmAction', { label: targetLabel }),
      risk: t('record.undoConfirmRisk'),
    })
    pointer.value--
    _applySnapshot(history.value[pointer.value])
  }

  const redo = async () => {
    if (!canRedo.value) return
    pointer.value++
    _applySnapshot(history.value[pointer.value])
  }

  const restoreTo = async (snapId: string) => {
    const snap = history.value.find((s) => s.id === snapId)
    if (!snap) return
    await confirmRiskAction({
      title: t('record.rollbackConfirmTitle'),
      action: t('record.rollbackConfirmAction', { label: snap.label }),
      risk: t('record.rollbackConfirmRisk'),
    })
    _applySnapshot(snap)
    pointer.value = history.value.indexOf(snap)
    ElMessage.success(t('record.rolledBackTo', { label: snap.label }))
  }

  const clearHistory = () => {
    history.value = []
    pointer.value = -1
  }

  const _applySnapshot = (snap: RecordSnapshot) => {
    const detail = domainStore.getDetailById(zoneId())
    detail.records = snap.records.map((r) => ({ ...r }))
    ElMessage.success(t('record.restoredTo', { label: snap.label }))
  }

  return {
    history,
    pointer,
    canUndo,
    canRedo,
    currentSnapshot,
    snapshot,
    undo,
    redo,
    restoreTo,
    clearHistory,
  }
}
