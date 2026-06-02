<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import type { DomainRecord } from '../types/modules'

export interface ImportRow {
  type: string
  host: string
  value: string
  ttl: number
  status: '启用' | '禁用'
  remark?: string
}

export type ConflictAction = 'overwrite' | 'skip'

export interface ConflictItem {
  incoming: ImportRow
  existing: DomainRecord
  action: ConflictAction
}

const props = defineProps<{
  modelValue: boolean
  existingRecords: DomainRecord[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'confirm', rows: ImportRow[], conflicts: ConflictItem[]): void
}>()

const { t } = useI18n()

const fileInput = ref<HTMLInputElement>()
const parsing = ref(false)
const step = ref<'upload' | 'preview'>('upload')

const parsedRows = ref<ImportRow[]>([])
const conflicts = ref<ConflictItem[]>([])
const cleanRows = computed(() =>
  parsedRows.value.filter(
    (r) => !conflicts.value.some((c) => c.incoming === r),
  ),
)

const totalNew = computed(() => cleanRows.value.length)
const totalConflict = computed(() => conflicts.value.length)
const totalSkip = computed(() => conflicts.value.filter((c) => c.action === 'skip').length)
const totalOverwrite = computed(() => conflicts.value.filter((c) => c.action === 'overwrite').length)

const conflictBadge = (action: ConflictAction) =>
  action === 'overwrite' ? 'mn-badge--warning' : 'mn-badge--neutral'

const statusLabel = (status: ImportRow['status']) =>
  status === '禁用' ? t('common.disabled') : t('common.enabled')

const _isConflict = (row: ImportRow): DomainRecord | undefined =>
  props.existingRecords.find(
    (e) => e.host === row.host && e.type === row.type,
  )

const VALID_TYPES = new Set(['A','AAAA','CNAME','MX','TXT','NS','SRV','PTR','CAA'])

const parseZoneFile = (text: string): ImportRow[] => {
  const rows: ImportRow[] = []
  for (const rawLine of text.split('\n')) {
    let remark = ''
    let line = rawLine
    const semiIdx = line.indexOf(';')
    if (semiIdx >= 0) {
      remark = line.slice(semiIdx + 1).trim()
      line = line.slice(0, semiIdx)
    }
    line = line.trim()
    if (!line || line.startsWith('$') || line.startsWith('#')) continue
    const parts = line.split(/\s+/)
    if (parts.length < 5) continue
    const inIdx = parts.findIndex((p) => p.toUpperCase() === 'IN')
    if (inIdx < 0 || inIdx + 2 >= parts.length) continue
    const recType = parts[inIdx + 1].toUpperCase()
    if (!VALID_TYPES.has(recType)) continue
    let host = parts[0].replace(/\.$/, '')
    let ttl = 600
    if (inIdx > 1) {
      const n = Number(parts[inIdx - 1])
      if (n > 0) ttl = n
    }
    const value = parts.slice(inIdx + 2).join(' ').replace(/^"(.*)"$/, '$1').replace(/\.$/, '')
    if (!host || !value) continue
    rows.push({ type: recType, host, value, ttl, status: '启用', remark } as ImportRow & { remark: string })
  }
  return rows
}

const parseCsvFile = (text: string): ImportRow[] | null => {
  const lines = text.trim().split('\n').filter(Boolean)
  if (lines.length < 2) { ElMessage.error(t('component.recordImport.csvEmpty')); return null }
  const header = lines[0].split(',').map((h) => h.trim().toLowerCase())
  const idx = (col: string) => header.indexOf(col)
  const ti = idx('type'), hi = idx('host'), vi = idx('value'), li = idx('ttl'), si = idx('status')
  if (ti < 0 || hi < 0 || vi < 0 || li < 0) {
    ElMessage.error(t('component.recordImport.csvMissingColumns'))
    return null
  }
  const rows: ImportRow[] = []
  for (let i = 1; i < lines.length; i++) {
    const cols = lines[i].split(',').map((c) => c.trim().replace(/^"|"$/g, ''))
    const type = cols[ti], host = cols[hi], value = cols[vi]
    const ttl = Number(cols[li]) || 600
    const rawStatus = (cols[si] ?? '').toLowerCase()
    const status: '启用' | '禁用' = ['禁用', 'disabled', 'disable', 'false', '0'].includes(rawStatus)
      ? '禁用'
      : '启用'
    if (!type || !host || !value) continue
    rows.push({ type, host, value, ttl, status })
  }
  return rows
}

const parseFile = async (file: File) => {
  parsing.value = true
  parsedRows.value = []
  conflicts.value = []
  try {
    const raw = await file.arrayBuffer()
    // 去掉 UTF-8 BOM
    const bom = new Uint8Array(raw.slice(0, 3))
    const hasBom = bom[0] === 0xEF && bom[1] === 0xBB && bom[2] === 0xBF
    const text = new TextDecoder('utf-8').decode(hasBom ? raw.slice(3) : raw)

    const name = file.name.toLowerCase()
    const isZone = name.endsWith('.zone') || name.endsWith('.txt')

    let rows: ImportRow[] | null
    if (isZone) {
      rows = parseZoneFile(text)
      if (!rows.length) { ElMessage.error(t('component.recordImport.zoneInvalid')); return }
    } else {
      rows = parseCsvFile(text)
      if (!rows) return
    }

    parsedRows.value = rows
    conflicts.value = rows
      .map((row) => {
        const existing = _isConflict(row)
        if (!existing) return null
        return { incoming: row, existing, action: 'skip' as ConflictAction }
      })
      .filter(Boolean) as ConflictItem[]

    step.value = 'preview'
  } finally {
    parsing.value = false
  }
}

const onFileChange = async (e: Event) => {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  await parseFile(file)
  if (fileInput.value) fileInput.value.value = ''
}

const setAllConflict = (action: ConflictAction) => {
  conflicts.value.forEach((c) => (c.action = action))
}

const confirm = () => {
  emit('confirm', parsedRows.value, conflicts.value)
  close()
}

const close = () => {
  step.value = 'upload'
  parsedRows.value = []
  conflicts.value = []
  emit('update:modelValue', false)
}
</script>

<template>
  <el-dialog
    :model-value="modelValue"
    :title="$t('component.recordImport.title')"
    width="720px"
    destroy-on-close
    @close="close"
  >
    <!-- Step 1: upload -->
    <div v-if="step === 'upload'" class="rid-upload-area">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
        <polyline points="17 8 12 3 7 8"/>
        <line x1="12" y1="3" x2="12" y2="15"/>
      </svg>
      <p class="rid-upload-title">{{ $t('component.recordImport.selectFile') }}</p>
      <p class="rid-upload-hint">{{ $t('component.recordImport.fileHint') }}</p>
      <el-button type="primary" :loading="parsing" @click="fileInput?.click()">{{ $t('component.recordImport.selectButton') }}</el-button>
      <input ref="fileInput" type="file" accept=".csv,.zone,.txt" style="display:none" @change="onFileChange" />
    </div>

    <!-- Step 2: preview -->
    <template v-else>
      <!-- summary bar -->
      <div class="rid-summary">
        <div class="rid-summary-item">
          <span class="rid-summary-num">{{ parsedRows.length }}</span>
          <span class="rid-summary-label">{{ $t('component.recordImport.summary.totalRows') }}</span>
        </div>
        <div class="rid-summary-item rid-summary-item--new">
          <span class="rid-summary-num">{{ totalNew }}</span>
          <span class="rid-summary-label">{{ $t('component.recordImport.summary.new') }}</span>
        </div>
        <div class="rid-summary-item rid-summary-item--conflict">
          <span class="rid-summary-num">{{ totalConflict }}</span>
          <span class="rid-summary-label">{{ $t('component.recordImport.summary.conflict') }}</span>
        </div>
        <div class="rid-summary-item rid-summary-item--overwrite">
          <span class="rid-summary-num">{{ totalOverwrite }}</span>
          <span class="rid-summary-label">{{ $t('component.recordImport.summary.overwrite') }}</span>
        </div>
        <div class="rid-summary-item rid-summary-item--skip">
          <span class="rid-summary-num">{{ totalSkip }}</span>
          <span class="rid-summary-label">{{ $t('component.recordImport.summary.skip') }}</span>
        </div>
      </div>

      <!-- conflict resolution -->
      <div v-if="conflicts.length" class="rid-conflicts">
        <div class="rid-conflicts-header">
          <span>{{ $t('component.recordImport.conflictsTitle') }}</span>
          <div class="rid-conflicts-bulk">
            <el-button size="small" @click="setAllConflict('overwrite')">{{ $t('component.recordImport.overwriteAll') }}</el-button>
            <el-button size="small" @click="setAllConflict('skip')">{{ $t('component.recordImport.skipAll') }}</el-button>
          </div>
        </div>
        <el-table :data="conflicts" size="small" class="rid-conflict-table">
          <el-table-column :label="$t('common.type')" width="70">
            <template #default="{ row }">
              <span class="rid-type-badge" :style="{ background: typeColor(row.incoming.type) }">{{ row.incoming.type }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="$t('component.recordImport.hostRecord')" prop="incoming.host" width="130" show-overflow-tooltip />
          <el-table-column :label="$t('component.recordImport.importedValue')" min-width="150" show-overflow-tooltip>
            <template #default="{ row }"><span class="rid-mono">{{ row.incoming.value }}</span></template>
          </el-table-column>
          <el-table-column :label="$t('component.recordImport.existingValue')" min-width="150" show-overflow-tooltip>
            <template #default="{ row }"><span class="rid-mono rid-existing">{{ row.existing.value }}</span></template>
          </el-table-column>
          <el-table-column :label="$t('component.recordImport.actionLabel')" width="130" align="center">
            <template #default="{ row }">
              <el-select v-model="row.action" size="small" style="width:100px">
                <el-option :label="$t('component.recordImport.overwrite')" value="overwrite" />
                <el-option :label="$t('component.recordImport.skip')" value="skip" />
              </el-select>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- new records preview (collapsed) -->
      <el-collapse class="rid-collapse" v-if="cleanRows.length">
        <el-collapse-item :title="$t('component.recordImport.newPreviewTitle', { count: cleanRows.length })" name="new">
          <el-table :data="cleanRows" size="small" max-height="200">
            <el-table-column :label="$t('common.type')" width="70">
              <template #default="{ row }">
                <span class="rid-type-badge" :style="{ background: typeColor(row.type) }">{{ row.type }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="host" :label="$t('component.recordImport.host')" width="110" show-overflow-tooltip />
            <el-table-column prop="value" :label="$t('common.value')" min-width="140" show-overflow-tooltip />
            <el-table-column prop="ttl" label="TTL" width="60" />
            <el-table-column :label="$t('common.status')" width="70">
              <template #default="{ row }">{{ statusLabel(row.status) }}</template>
            </el-table-column>
            <el-table-column prop="remark" :label="$t('common.remark')" min-width="100" show-overflow-tooltip />
          </el-table>
        </el-collapse-item>
      </el-collapse>

      <div class="rid-back">
        <el-button size="small" @click="step = 'upload'">{{ $t('component.recordImport.reselect') }}</el-button>
      </div>
    </template>

    <template #footer>
      <el-button @click="close">{{ $t('common.cancel') }}</el-button>
      <el-button v-if="step === 'preview'" type="primary" @click="confirm">
        {{ $t('component.recordImport.confirmImport', { count: totalNew + totalOverwrite }) }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script lang="ts">
const TYPE_COLORS: Record<string, string> = {
  A: '#3b82f6', AAAA: '#8b5cf6', CNAME: '#06b6d4', TXT: '#f59e0b',
  MX: '#10b981', NS: '#6366f1', SRV: '#f97316',
}
function typeColor(t: string) { return TYPE_COLORS[t] ?? '#6b7280' }
</script>

<style scoped>
/* Upload area */
.rid-upload-area {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 40px 20px;
  border: 2px dashed var(--app-border);
  border-radius: 10px;
  background: var(--app-bg-secondary);
}
.rid-upload-area svg { width: 40px; height: 40px; color: var(--app-text-secondary); opacity: 0.6; }
.rid-upload-title { font-size: 15px; font-weight: 600; color: var(--app-title); margin: 0; }
.rid-upload-hint { font-size: 12px; color: var(--app-text-regular); margin: 0; }
.rid-upload-hint code { font-family: monospace; background: var(--app-border); border-radius: 4px; padding: 1px 5px; }

/* Summary bar */
.rid-summary {
  display: flex;
  gap: 0;
  border: 1px solid var(--app-border);
  border-radius: 8px;
  overflow: hidden;
  margin-bottom: 16px;
}
.rid-summary-item {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 10px 8px;
  border-right: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}
.rid-summary-item:last-child { border-right: none; }
.rid-summary-num { font-size: 20px; font-weight: 700; color: var(--app-title); line-height: 1; }
.rid-summary-label { font-size: 11px; color: var(--app-text-secondary); margin-top: 4px; font-weight: 500; }
.rid-summary-item--new .rid-summary-num { color: var(--app-success); }
.rid-summary-item--conflict .rid-summary-num { color: var(--app-warning); }
.rid-summary-item--overwrite .rid-summary-num { color: var(--app-danger); }
.rid-summary-item--skip .rid-summary-num { color: var(--app-text-secondary); }

/* Conflict section */
.rid-conflicts { margin-bottom: 12px; }
.rid-conflicts-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--app-warning);
  border-bottom: 1px solid var(--app-border);
  margin-bottom: 8px;
}
.rid-conflicts-bulk { display: flex; gap: 6px; }
.rid-conflict-table { border-radius: 6px; overflow: hidden; }

/* Type badge */
.rid-type-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 700;
  color: #fff;
  letter-spacing: 0.04em;
}

/* Monospace */
.rid-mono { font-family: 'JetBrains Mono', 'Consolas', monospace; font-size: 11px; }
.rid-existing { color: var(--app-text-secondary); text-decoration: line-through; }

/* Collapse */
.rid-collapse { margin-bottom: 8px; }
.rid-collapse :deep(.el-collapse-item__header) { font-size: 12px; color: var(--app-text-regular); font-weight: 500; }

/* Back link */
.rid-back { margin-top: 6px; }
</style>
