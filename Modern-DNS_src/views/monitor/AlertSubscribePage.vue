<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getAlertSubscribeRules, createAlertSubscribeRule, updateAlertSubscribeRule, deleteAlertSubscribeRule, toggleAlertSubscribeRule } from '../../api/monitor'

interface AlertRule {
  id: number
  name: string
  metric: 'QPS' | '响应时间' | '错误率' | 'NXDOMAIN率' | '缓存命中率'
  operator: '>' | '<' | '>=' | '<='
  threshold: number
  unit: string
  duration: number
  channels: string[]
  status: '启用' | '禁用'
  triggerCount: number
  lastTriggered: string
}

const rules = ref<AlertRule[]>([])

const { t } = useI18n()
const loading = ref(false)
const submitting = ref(false)
const ruleDialogVisible = ref(false)
const isEdit = ref(false)
const editingId = ref<number|null>(null)

const metricOptions = computed(() => [
  { value: 'QPS', label: 'QPS' },
  { value: '响应时间', label: t('monitor.metricResponseTime') },
  { value: '错误率', label: t('monitor.metricErrorRate') },
  { value: 'NXDOMAIN率', label: t('monitor.metricNxdomainRate') },
  { value: '缓存命中率', label: t('monitor.metricCacheHitRate') },
])

const channelOptions = computed(() => [
  { value: '邮件', label: t('monitor.channelEmail') },
  { value: 'WebHook', label: t('monitor.channelWebhook') },
  { value: '钉钉', label: t('monitor.channelDingTalk') },
  { value: '企业微信', label: t('monitor.channelWechatWork') },
])

const form = reactive<Omit<AlertRule,'id'|'triggerCount'|'lastTriggered'>>({
  name: '', metric: 'QPS', operator: '>', threshold: 1000, unit: 'QPS', duration: 5, channels: [], status: '启用'
})

const metricUnits: Record<string, string> = { 'QPS': 'QPS', '响应时间': 'ms', '错误率': '%', 'NXDOMAIN率': '%', '缓存命中率': '%' }

const updateUnit = () => { form.unit = metricUnits[form.metric] || '' }

const kpiEnabled = computed(() => rules.value.filter(r => r.status === '启用').length)
const kpiTriggered = computed(() => rules.value.reduce((s, r) => s + r.triggerCount, 0))

const metricLabel = (value: string) => {
  const map: Record<string, string> = {
    QPS: 'QPS',
    响应时间: t('monitor.metricResponseTime'),
    错误率: t('monitor.metricErrorRate'),
    NXDOMAIN率: t('monitor.metricNxdomainRate'),
    缓存命中率: t('monitor.metricCacheHitRate'),
  }
  return map[value] || value
}

const channelLabel = (value: string) => {
  const map: Record<string, string> = {
    邮件: t('monitor.channelEmail'),
    WebHook: t('monitor.channelWebhook'),
    钉钉: t('monitor.channelDingTalk'),
    企业微信: t('monitor.channelWechatWork'),
  }
  return map[value] || value
}

const fetchRules = async () => {
  loading.value = true
  try {
    const { data } = await getAlertSubscribeRules()
    rules.value = (data ?? []).map((r: any) => ({
      ...r,
      channels: r.channels ? (typeof r.channels === 'string' ? r.channels.split(',').filter(Boolean) : r.channels) : [],
    }))
  } finally {
    loading.value = false
  }
}

onMounted(() => fetchRules())

const openCreateRule = () => {
  isEdit.value = false; editingId.value = null
  Object.assign(form, { name: '', metric: 'QPS', operator: '>', threshold: 1000, unit: 'QPS', duration: 5, channels: [], status: '启用' })
  ruleDialogVisible.value = true
}

const openEditRule = (row: AlertRule) => {
  isEdit.value = true; editingId.value = row.id
  Object.assign(form, { name: row.name, metric: row.metric, operator: row.operator, threshold: row.threshold, unit: row.unit, duration: row.duration, channels: [...row.channels], status: row.status })
  ruleDialogVisible.value = true
}

const submitRule = async () => {
  if (!form.name) { ElMessage.warning(t('monitor.fillRuleName')); return }
  submitting.value = true
  try {
    const payload = { ...form, channels: form.channels.join(',') }
    if (isEdit.value && editingId.value) {
      await updateAlertSubscribeRule(editingId.value, payload)
      ElMessage.success(t('monitor.ruleUpdated'))
    } else {
      await createAlertSubscribeRule(payload)
      ElMessage.success(t('monitor.ruleCreated'))
    }
    ruleDialogVisible.value = false
    await fetchRules()
  } finally {
    submitting.value = false
  }
}

const removeRule = async (row: AlertRule) => {
  await ElMessageBox.confirm(t('monitor.deleteRuleConfirm', { name: row.name }), t('monitor.deleteRuleTitle'), { type: 'warning' })
  await deleteAlertSubscribeRule(row.id)
  ElMessage.success(t('monitor.deleted'))
  await fetchRules()
}

const channelTypeColor: Record<string, string> = { '邮件': '#3b82f6', 'WebHook': '#8b5cf6', '钉钉': '#10b981', '企业微信': '#f59e0b' }
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('monitor.subscribe') }}</h1>
        <p class="page-subtitle">{{ $t('monitor.subscribeSubtitle') }}</p>
      </div>
    </div>

    <div class="mn-stats-strip">
      <div class="mn-stat-tile mn-stat-tile--success"><span class="mn-stat-value">{{ kpiEnabled }}</span><span class="mn-stat-label">{{ $t('monitor.enabledRules') }}</span></div>
      <div class="mn-stat-tile mn-stat-tile--warning"><span class="mn-stat-value">{{ kpiTriggered }}</span><span class="mn-stat-label">{{ $t('monitor.totalTriggers') }}</span></div>
      <div class="mn-stat-tile"><span class="mn-stat-value">{{ rules.length }}</span><span class="mn-stat-label">{{ $t('monitor.totalRules') }}</span></div>
    </div>

    <el-card class="mn-card">
      <div class="mn-toolbar">
            <div class="mn-toolbar-filters"></div>
            <div class="mn-toolbar-actions">
              <el-button type="primary" @click="openCreateRule">{{ $t('monitor.createRule') }}</el-button>
            </div>
          </div>
          <el-table :data="rules" v-loading="loading" stripe class="mn-table" table-layout="auto">
            <el-table-column prop="name" :label="$t('monitor.ruleName')" min-width="160" />
            <el-table-column :label="$t('monitor.triggerCondition')" min-width="200">
              <template #default="{ row }">
                <span class="condition-text">{{ metricLabel(row.metric) }} {{ row.operator }} <strong>{{ row.threshold }}{{ row.unit }}</strong> {{ $t('monitor.durationMinutes', { n: row.duration }) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="channels" :label="$t('monitor.notifyChannel')" min-width="160">
              <template #default="{ row }">
                <div class="tag-list">
                  <span v-for="c in row.channels" :key="c" class="type-badge" :style="{ background: channelTypeColor[c] ?? '#6b7280' }">{{ channelLabel(c) }}</span>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="triggerCount" :label="$t('monitor.triggerCount')" width="90" align="right" sortable>
              <template #default="{ row }"><span class="mn-mono" style="color:var(--app-warning);font-weight:600">{{ row.triggerCount }}</span></template>
            </el-table-column>
            <el-table-column prop="lastTriggered" :label="$t('monitor.lastTriggered')" min-width="150">
              <template #default="{ row }"><span class="mn-time">{{ row.lastTriggered }}</span></template>
            </el-table-column>
            <el-table-column prop="status" :label="$t('common.status')" width="90" align="center">
              <template #default="{ row }">
                <el-switch :model-value="row.status === '启用'" size="small" @change="async () => { const s = row.status === '启用' ? '禁用' : '启用'; await toggleAlertSubscribeRule(row.id, s); row.status = s }" />
              </template>
            </el-table-column>
            <el-table-column :label="$t('common.operation')" width="120" fixed="right">
              <template #default="{ row }">
                <div class="mn-row-ops">
                  <el-button link type="primary" @click="openEditRule(row)">{{ $t('common.edit') }}</el-button>
                  <el-button link type="danger" @click="removeRule(row)">{{ $t('common.delete') }}</el-button>
                </div>
              </template>
            </el-table-column>
            <template #empty><el-empty :description="$t('monitor.noAlertRules')" /></template>
          </el-table>
    </el-card>

    <!-- Rule dialog -->
    <el-dialog v-model="ruleDialogVisible" :title="isEdit ? $t('monitor.editAlertRule') : $t('monitor.createAlertRule')" width="500px" append-to-body>
      <el-form :model="form" label-position="top">
        <el-form-item :label="$t('monitor.ruleName')"><el-input v-model="form.name" :placeholder="$t('monitor.ruleNamePlaceholder')" /></el-form-item>
        <div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:0 12px">
          <el-form-item :label="$t('monitor.monitorMetric')">
            <el-select v-model="form.metric" style="width:100%" @change="updateUnit">
              <el-option v-for="m in metricOptions" :key="m.value" :label="m.label" :value="m.value" />
            </el-select>
          </el-form-item>
          <el-form-item :label="$t('monitor.triggerCondition')">
            <el-select v-model="form.operator" style="width:100%">
              <el-option v-for="op in ['>','<','>=','<=']" :key="op" :label="op" :value="op" />
            </el-select>
          </el-form-item>
          <el-form-item :label="$t('monitor.thresholdLabel') + ' (' + form.unit + ')'">
            <el-input-number v-model="form.threshold" :min="0" controls-position="right" style="width:100%" />
          </el-form-item>
        </div>
        <el-form-item :label="$t('monitor.durationLabel')">
          <el-input-number v-model="form.duration" :min="1" :max="60" controls-position="right" style="width:180px" />
        </el-form-item>
        <el-form-item :label="$t('monitor.notifyChannel')">
          <el-checkbox-group v-model="form.channels">
            <el-checkbox-button v-for="c in channelOptions" :key="c.value" :label="c.value" :value="c.value">{{ c.label }}</el-checkbox-button>
          </el-checkbox-group>
        </el-form-item>
        <el-form-item :label="$t('common.status')">
          <el-radio-group v-model="form.status">
            <el-radio-button :label="$t('common.enable')" value="启用" /><el-radio-button :label="$t('common.disable')" value="禁用" />
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ruleDialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="submitting" @click="submitRule">{{ isEdit ? $t('common.save') : $t('common.create') }}</el-button>
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
.tag-list { display:flex; flex-wrap:wrap; gap:4px; }
.condition-text { font-size:13px; color:var(--app-text-primary,#1d2129); }
</style>
