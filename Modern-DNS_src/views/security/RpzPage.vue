<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import {
  listRpzRulesApi,
  createRpzRuleApi,
  updateRpzRuleApi,
  deleteRpzRuleApi,
  toggleRpzRuleApi,
  type RpzRule,
} from '../../api/security'
import StatusPill from '../../components/StatusPill.vue'

const { t } = useI18n()
const rules = ref<RpzRule[]>([])

const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const keyword = ref('')
const filterAction = ref('')
const filterStatus = ref('')

const form = reactive<Omit<RpzRule, 'id' | 'hitCount' | 'createdAt' | 'updatedAt'>>({
  name: '', type: t('security.matchDomain'), pattern: '', action: t('security.block'), redirectTo: '', status: t('common.enabled'), category: t('security.custom')
})

const fetchRules = async () => {
  loading.value = true
  try {
    const res = await listRpzRulesApi()
    const data = (res as any).data ?? res
    rules.value = Array.isArray(data) ? data : (data?.list ?? [])
  } catch {
    ElMessage.error(t('security.loadRpzFailed'))
  } finally {
    loading.value = false
  }
}

const filteredRules = computed(() => rules.value.filter(r => {
  const matchK = !keyword.value || r.name.includes(keyword.value) || r.pattern.includes(keyword.value)
  const matchA = !filterAction.value || r.action === filterAction.value
  const matchS = !filterStatus.value || r.status === filterStatus.value
  return matchK && matchA && matchS
}))

const kpiEnabled = computed(() => rules.value.filter(r => r.status === '启用').length)
const kpiBlock = computed(() => rules.value.filter(r => r.action === '拦截' || r.action === 'NXDOMAIN').length)
const kpiHits = computed(() => rules.value.reduce((s, r) => s + r.hitCount, 0))

const editingId = ref<number | null>(null)

const openCreate = () => {
  isEdit.value = false
  editingId.value = null
  Object.assign(form, { name: '', type: '域名', pattern: '', action: '拦截', redirectTo: '', status: '启用', category: '自定义' })
  dialogVisible.value = true
}

const openEdit = (row: RpzRule) => {
  isEdit.value = true
  editingId.value = row.id
  Object.assign(form, { name: row.name, type: row.type, pattern: row.pattern, action: row.action, redirectTo: row.redirectTo, status: row.status, category: row.category })
  dialogVisible.value = true
}

const submitRule = async () => {
  if (!form.name || !form.pattern) { ElMessage.warning(t('security.fillNamePattern')); return }
  submitting.value = true
  try {
    if (isEdit.value && editingId.value) {
      await updateRpzRuleApi({ id: editingId.value, ...form })
      ElMessage.success(t('security.ruleUpdated'))
    } else {
      await createRpzRuleApi({ ...form })
      ElMessage.success(t('security.ruleAdded'))
    }
    dialogVisible.value = false
    await fetchRules()
  } catch {
    ElMessage.error(t('security.saveFailed'))
  } finally {
    submitting.value = false
  }
}

const removeRule = async (row: RpzRule) => {
  await ElMessageBox.confirm(t('security.deleteConfirm', { name: row.name }), t('security.deleteConfirmTitle'), { type: 'warning' })
  try {
    await deleteRpzRuleApi(row.id)
    ElMessage.success(t('security.deleted'))
    await fetchRules()
  } catch {
    ElMessage.error(t('security.deleteFailed'))
  }
}

const toggleStatus = async (row: RpzRule) => {
  const next = row.status === t('common.enabled') ? t('common.disabled') : t('common.enabled')
  try {
    await toggleRpzRuleApi(row.id, next)
    row.status = next
    ElMessage.success(t('security.statusToggled', { status: next }))
  } catch {
    ElMessage.error(t('security.toggleFailed'))
  }
}

const actionColor: Record<string, string> = { [t('security.block')]: '#ef4444', [t('security.redirect')]: '#f59e0b', [t('security.pass')]: '#10b981', 'NXDOMAIN': '#6b7280' }
const categoryColor: Record<string, string> = { [t('security.malware')]: '#ef4444', [t('security.ad')]: '#f59e0b', [t('security.phishing')]: '#8b5cf6', [t('security.custom')]: '#3b82f6' }

onMounted(fetchRules)
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('security.rpzTitle') }}</h1>
        <p class="page-subtitle">{{ $t('security.rpzSubtitle') }}</p>
      </div>
      <div class="page-actions">
        <el-button type="primary" @click="openCreate">{{ $t('security.addRlzRule') }}</el-button>
      </div>
    </div>

    <div class="mn-stats-strip">
      <div class="mn-stat-tile mn-stat-tile--success"><span class="mn-stat-value">{{ kpiEnabled }}</span><span class="mn-stat-label">{{ $t('security.enabledRules') }}</span></div>
      <div class="mn-stat-tile mn-stat-tile--danger"><span class="mn-stat-value">{{ kpiBlock }}</span><span class="mn-stat-label">{{ $t('security.blockRules') }}</span></div>
      <div class="mn-stat-tile mn-stat-tile--accent"><span class="mn-stat-value">{{ kpiHits.toLocaleString() }}</span><span class="mn-stat-label">{{ $t('security.totalHits') }}</span></div>
      <div class="mn-stat-tile"><span class="mn-stat-value">{{ rules.length }}</span><span class="mn-stat-label">{{ $t('security.totalRules') }}</span></div>
    </div>

    <el-card class="mn-card">
      <div class="mn-toolbar">
        <div class="mn-toolbar-filters">
          <el-input v-model="keyword" clearable :placeholder="$t('security.searchRule')" style="width:240px">
            <template #prefix><svg class="mn-input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></template>
          </el-input>
          <el-select v-model="filterAction" clearable :placeholder="$t('security.action')" style="width:120px">
            <el-option v-for="a in [t('security.block'),t('security.redirect'),t('security.pass'),'NXDOMAIN']" :key="a" :label="a" :value="a" />
          </el-select>
          <el-select v-model="filterStatus" clearable :placeholder="$t('common.status')" style="width:110px">
            <el-option :label="$t('common.enabled')" value="启用" /><el-option :label="$t('common.disabled')" value="禁用" />
          </el-select>
        </div>
        <!-- Mirror the page-header CTA inside the toolbar so the "新增"
             button stays visible when this page is embedded as a tab
             pane (DomainAccessPage hides nested .page-header). -->
        <div class="mn-toolbar-actions">
          <el-button type="primary" @click="openCreate">{{ $t('security.addRlzRule') }}</el-button>
        </div>
      </div>

      <el-table :data="filteredRules" v-loading="loading" stripe class="mn-table" table-layout="auto">
        <el-table-column prop="name" :label="$t('security.ruleName')" min-width="160" show-overflow-tooltip />
        <el-table-column prop="category" :label="$t('security.category')" width="100" align="center">
          <template #default="{ row }">
            <!-- category badge keeps per-category hue — leave the
                 inline-style .type-badge here, NOT a StatusPill. -->
            <span class="type-badge" :style="{ background: categoryColor[row.category] }">{{ row.category }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="type" :label="$t('security.matchType')" width="90" align="center">
          <template #default="{ row }"><StatusPill intent="neutral" size="xs">{{ row.type }}</StatusPill></template>
        </el-table-column>
        <el-table-column prop="pattern" :label="$t('security.matchPattern')" min-width="200" show-overflow-tooltip>
          <template #default="{ row }"><span class="mn-mono">{{ row.pattern }}</span></template>
        </el-table-column>
        <el-table-column prop="action" :label="$t('security.action')" width="110" align="center">
          <template #default="{ row }">
            <span class="type-badge" :style="{ background: actionColor[row.action] }">{{ row.action }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="redirectTo" :label="$t('security.redirectTo')" width="140">
          <template #default="{ row }"><span class="mn-mono">{{ row.redirectTo || '—' }}</span></template>
        </el-table-column>
        <el-table-column prop="hitCount" :label="$t('security.hitCount')" width="100" align="right" sortable>
          <template #default="{ row }"><span class="mn-mono" style="color:var(--app-accent);font-weight:600">{{ row.hitCount.toLocaleString() }}</span></template>
        </el-table-column>
        <el-table-column prop="status" :label="$t('common.status')" width="90" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.status === '启用'" size="small" @change="toggleStatus(row)" />
          </template>
        </el-table-column>
        <el-table-column prop="createdAt" :label="$t('common.createdAt')" min-width="120">
          <template #default="{ row }"><span class="mn-time">{{ row.createdAt }}</span></template>
        </el-table-column>
        <el-table-column :label="$t('common.operation')" width="120">
          <template #default="{ row }">
            <div class="mn-row-ops">
              <el-button link type="primary" @click="openEdit(row)">{{ $t('common.edit') }}</el-button>
              <el-button link type="danger" @click="removeRule(row)">{{ $t('common.delete') }}</el-button>
            </div>
          </template>
        </el-table-column>
        <template #empty><el-empty :description="$t('security.noRlzRules')" /></template>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="isEdit ? $t('security.editRlzRule') : $t('security.addRlzRule')" width="520px" append-to-body>
      <el-form :model="form" label-position="top">
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:0 16px">
          <el-form-item :label="$t('security.ruleName')"><el-input v-model="form.name" :placeholder="$t('security.ruleNamePlaceholder')" /></el-form-item>
          <el-form-item :label="$t('security.category')">
            <el-select v-model="form.category" style="width:100%">
              <el-option v-for="c in [t('security.malware'),t('security.ad'),t('security.phishing'),t('security.custom')]" :key="c" :label="c" :value="c" />
            </el-select>
          </el-form-item>
          <el-form-item :label="$t('security.matchType')">
            <el-select v-model="form.type" style="width:100%">
              <el-option v-for="item in [t('security.matchDomain'),t('security.matchIp'),t('security.matchNs'),t('security.wildcard')]" :key="item" :label="item" :value="item" />
            </el-select>
          </el-form-item>
          <el-form-item :label="$t('security.action')">
            <el-select v-model="form.action" style="width:100%">
              <el-option v-for="a in [t('security.block'),t('security.redirect'),t('security.pass'),'NXDOMAIN']" :key="a" :label="a" :value="a" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item :label="$t('security.matchPattern')"><el-input v-model="form.pattern" :placeholder="$t('security.patternPlaceholder')" /></el-form-item>
        <el-form-item v-if="form.action === t('security.redirect')" :label="$t('security.redirectTo')"><el-input v-model="form.redirectTo" placeholder="10.0.0.1" /></el-form-item>
        <el-form-item :label="$t('common.status')">
          <el-radio-group v-model="form.status">
            <el-radio-button :label="$t('common.enabled')" value="启用" /><el-radio-button :label="$t('common.disabled')" value="禁用" />
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="submitting" @click="submitRule">{{ isEdit ? $t('common.save') : $t('common.add') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.mn-stats-strip { display:flex; gap:14px; flex-wrap:wrap; }
.mn-stat-tile { display:flex; flex-direction:row; align-items:center; gap:14px; padding:14px 24px; border-radius:10px; border:1px solid var(--app-border); background:var(--app-bg); min-width:100px; flex:1; box-shadow:var(--app-shadow-soft); }
.mn-stat-value { font-size:28px; font-weight:700; font-variant-numeric:tabular-nums; letter-spacing:-0.03em; line-height:1; }
.mn-stat-label { font-size:12px; font-weight:500; color:var(--app-text-regular); line-height:1.4; }
.mn-stat-tile--success .mn-stat-value { color:var(--app-success); }
.mn-stat-tile--danger .mn-stat-value  { color:var(--app-danger); }
.mn-stat-tile--warning .mn-stat-value { color:var(--app-warning); }
.mn-stat-tile--accent .mn-stat-value  { color:var(--app-accent); }
.mn-stat-tile--neutral .mn-stat-value { color:var(--app-text-secondary); }
.type-badge { display:inline-flex; align-items:center; justify-content:center; padding:2px 8px; border-radius:4px; font-size:11px; font-weight:700; color:#fff; }

/* Toolbar layout — filters on the left, action buttons on the right.
   The .mn-toolbar* classes are referenced in the template but the
   project doesn't have a shared stylesheet for them; each page that
   uses them re-declares the scoped rules. Without these, the
   "新增 RPZ 规则" button collapses to the natural inline position
   directly under the filter row. */
.mn-toolbar { display:flex; align-items:center; justify-content:space-between; gap:10px; flex-wrap:wrap; padding:12px 16px; background:var(--app-bg-secondary); border-bottom:1px solid var(--app-border); }
.mn-toolbar-filters { display:flex; align-items:center; flex-wrap:wrap; gap:8px; flex:1; }
.mn-toolbar-actions { display:flex; align-items:center; gap:8px; flex-shrink:0; }
.mn-input-icon { width:13px; height:13px; color:var(--app-text-regular); }

@media (max-width: 1200px) {
  .mn-toolbar { flex-direction: column; align-items: flex-start; }
  .mn-toolbar-actions { justify-content: flex-start; }
}
</style>
