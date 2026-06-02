<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import {
  listAclRulesApi,
  createAclRuleApi,
  updateAclRuleApi,
  deleteAclRuleApi,
  toggleAclRuleApi,
  type AclRule,
} from '../../api/acl'
import StatusPill from '../../components/StatusPill.vue'

const { t } = useI18n()

const rules = ref<AclRule[]>([])
const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const editingId = ref<number | null>(null)
const keyword = ref('')
const filterType = ref('')

const fetchRules = async () => {
  loading.value = true
  try {
    const res = await listAclRulesApi()
    rules.value = res.data?.list ?? []
  } catch {
    ElMessage.error(t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

onMounted(fetchRules)

const filteredRules = computed(() => [...rules.value]
  .filter(r => (!keyword.value || r.name.includes(keyword.value) || r.cidr.includes(keyword.value)) && (!filterType.value || r.type === filterType.value))
  .sort((a, b) => a.priority - b.priority)
)

const kpiAllow = computed(() => rules.value.filter(r => r.type === '允许' && r.status === '启用').length)
const kpiDeny = computed(() => rules.value.filter(r => r.type === '拒绝' && r.status === '启用').length)
const kpiHits = computed(() => rules.value.reduce((s, r) => s + r.hitCount, 0))

const form = reactive<Omit<AclRule,'id'|'hitCount'|'createdAt'>>({
  name: '', cidr: '', type: '允许', priority: 10, queryTypes: ['A','AAAA'], zones: '*', status: '启用', remark: ''
})

const queryTypeOptions = ['A','AAAA','MX','TXT','CNAME','NS','SRV','CAA','ANY']

const typeLabel = (value: '允许' | '拒绝') => value === '允许' ? t('security.acl.allow') : t('security.acl.deny')
const statusLabel = (value: '启用' | '禁用') => value === '启用' ? t('common.enabled') : t('common.disabled')

const openCreate = () => {
  isEdit.value = false; editingId.value = null
  Object.assign(form, { name: '', cidr: '', type: '允许', priority: 10, queryTypes: ['A','AAAA'], zones: '*', status: '启用', remark: '' })
  dialogVisible.value = true
}

const openEdit = (row: AclRule) => {
  isEdit.value = true; editingId.value = row.id
  Object.assign(form, { name: row.name, cidr: row.cidr, type: row.type, priority: row.priority, queryTypes: [...row.queryTypes], zones: row.zones, status: row.status, remark: row.remark })
  dialogVisible.value = true
}

const submit = async () => {
  if (!form.name || !form.cidr) { ElMessage.warning(t('security.acl.validation.nameAndCidr')); return }
  submitting.value = true
  try {
    if (isEdit.value && editingId.value) {
      await updateAclRuleApi(editingId.value, { ...form })
      ElMessage.success(t('security.acl.messages.updated'))
    } else {
      await createAclRuleApi({ ...form })
      ElMessage.success(t('security.acl.messages.added'))
    }
    dialogVisible.value = false
    await fetchRules()
  } catch (err: any) {
    ElMessage.error(err?.message || t('common.saveFailed'))
  } finally {
    submitting.value = false
  }
}

const removeRule = async (row: AclRule) => {
  await ElMessageBox.confirm(t('security.acl.confirmDelete', { name: row.name }), t('security.acl.deleteTitle'), { type: 'warning' })
  try {
    await deleteAclRuleApi(row.id)
    ElMessage.success(t('common.deleted'))
    await fetchRules()
  } catch (err: any) {
    ElMessage.error(err?.message || t('common.deleteFailed'))
  }
}

const toggleStatus = async (row: AclRule) => {
  const next: '启用' | '禁用' = row.status === '启用' ? '禁用' : '启用'
  try {
    await toggleAclRuleApi(row.id, next)
    row.status = next
  } catch (err: any) {
    ElMessage.error(err?.message || t('common.saveFailed'))
  }
}
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('security.aclTitle') }}</h1>
        <p class="page-subtitle">{{ $t('security.aclSubtitle') }}</p>
      </div>
      <div class="page-actions">
        <el-button type="primary" @click="openCreate">{{ $t('security.acl.addRule') }}</el-button>
      </div>
    </div>

    <div class="mn-stats-strip">
      <div class="mn-stat-tile mn-stat-tile--success"><span class="mn-stat-value">{{ kpiAllow }}</span><span class="mn-stat-label">{{ $t('security.acl.kpi.allowRules') }}</span></div>
      <div class="mn-stat-tile mn-stat-tile--danger"><span class="mn-stat-value">{{ kpiDeny }}</span><span class="mn-stat-label">{{ $t('security.acl.kpi.denyRules') }}</span></div>
      <div class="mn-stat-tile mn-stat-tile--accent"><span class="mn-stat-value">{{ kpiHits.toLocaleString() }}</span><span class="mn-stat-label">{{ $t('security.acl.kpi.totalHits') }}</span></div>
      <div class="mn-stat-tile"><span class="mn-stat-value">{{ rules.length }}</span><span class="mn-stat-label">{{ $t('security.acl.kpi.totalRules') }}</span></div>
    </div>

    <el-card class="mn-card">
      <div class="mn-toolbar">
        <div class="mn-toolbar-filters">
          <el-input v-model="keyword" clearable :placeholder="$t('security.acl.searchPlaceholder')" style="width:220px">
            <template #prefix><svg class="mn-input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></template>
          </el-input>
          <el-select v-model="filterType" clearable :placeholder="$t('common.type')" style="width:110px">
            <el-option :label="$t('security.acl.allow')" value="允许" /><el-option :label="$t('security.acl.deny')" value="拒绝" />
          </el-select>
        </div>
      </div>

      <div class="acl-tip">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="14" height="14"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
        {{ $t('security.acl.tip') }}
      </div>

      <el-table :data="filteredRules" v-loading="loading" stripe class="mn-table" table-layout="auto">
        <el-table-column prop="priority" :label="$t('forward.priority')" width="80" align="center" sortable>
          <template #default="{ row }"><span class="mn-mono priority-num">{{ row.priority }}</span></template>
        </el-table-column>
        <el-table-column prop="name" :label="$t('security.acl.ruleName')" min-width="160" show-overflow-tooltip />
        <el-table-column prop="cidr" :label="$t('security.acl.clientCidr')" min-width="170">
          <template #default="{ row }"><span class="mn-mono">{{ row.cidr }}</span></template>
        </el-table-column>
        <el-table-column prop="type" :label="$t('common.action')" width="90" align="center">
          <template #default="{ row }">
            <StatusPill :intent="row.type === '允许' ? 'success' : 'danger'">{{ typeLabel(row.type) }}</StatusPill>
          </template>
        </el-table-column>
        <el-table-column prop="queryTypes" :label="$t('record.type')" min-width="180">
          <template #default="{ row }">
            <div class="tag-list">
              <StatusPill v-for="t in row.queryTypes" :key="t" intent="neutral" size="xs">{{ t }}</StatusPill>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="zones" :label="$t('security.acl.zoneScope')" min-width="160" show-overflow-tooltip>
          <template #default="{ row }"><span class="mn-mono">{{ row.zones }}</span></template>
        </el-table-column>
        <el-table-column prop="hitCount" :label="$t('security.acl.hitCount')" width="100" align="right" sortable>
          <template #default="{ row }"><span class="mn-mono" style="color:var(--app-accent);font-weight:600">{{ row.hitCount.toLocaleString() }}</span></template>
        </el-table-column>
        <el-table-column prop="status" :label="$t('common.status')" width="90" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.status === '启用'" size="small" @change="toggleStatus(row)" />
          </template>
        </el-table-column>
        <el-table-column :label="$t('common.operation')" width="120" fixed="right">
          <template #default="{ row }">
            <div class="mn-row-ops">
              <el-button link type="primary" @click="openEdit(row)">{{ $t('common.edit') }}</el-button>
              <el-button link type="danger" @click="removeRule(row)">{{ $t('common.delete') }}</el-button>
            </div>
          </template>
        </el-table-column>
        <template #empty><el-empty :description="$t('security.acl.empty')" /></template>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="isEdit ? $t('security.acl.editRule') : $t('security.acl.addRule')" width="540px" append-to-body>
      <el-form :model="form" label-position="top">
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:0 16px">
          <el-form-item :label="$t('security.acl.ruleName')"><el-input v-model="form.name" :placeholder="$t('security.acl.namePlaceholder')" /></el-form-item>
          <el-form-item :label="$t('security.acl.priorityHint')"><el-input-number v-model="form.priority" :min="1" :max="999" controls-position="right" style="width:100%" /></el-form-item>
          <el-form-item :label="$t('security.acl.clientCidr')"><el-input v-model="form.cidr" :placeholder="$t('security.acl.cidrPlaceholder')" /></el-form-item>
          <el-form-item :label="$t('common.action')">
            <el-radio-group v-model="form.type">
              <el-radio-button :label="$t('security.acl.allow')" value="允许" /><el-radio-button :label="$t('security.acl.deny')" value="拒绝" />
            </el-radio-group>
          </el-form-item>
        </div>
        <el-form-item :label="$t('security.acl.queryTypes')">
          <el-checkbox-group v-model="form.queryTypes">
            <el-checkbox-button v-for="t in queryTypeOptions" :key="t" :label="t" :value="t" />
          </el-checkbox-group>
        </el-form-item>
        <el-form-item :label="$t('security.acl.zoneScopeHint')"><el-input v-model="form.zones" :placeholder="$t('security.acl.zonePlaceholder')" /></el-form-item>
        <el-form-item :label="$t('common.remark')"><el-input v-model="form.remark" :placeholder="$t('common.remarkOptional')" /></el-form-item>
        <el-form-item :label="$t('common.status')">
          <el-radio-group v-model="form.status">
            <el-radio-button :label="statusLabel('启用')" value="启用" /><el-radio-button :label="statusLabel('禁用')" value="禁用" />
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">{{ isEdit ? $t('common.save') : $t('common.add') }}</el-button>
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
.acl-tip { display:flex; align-items:center; gap:7px; padding:8px 12px; border-radius:6px; background:rgba(22,93,255,0.05); border:1px solid rgba(22,93,255,0.15); color:var(--app-text-secondary); font-size:12px; margin-bottom:12px; }
.priority-num { font-weight:700; color:var(--app-accent); }
.tag-list { display:flex; flex-wrap:wrap; gap:4px; }
</style>
