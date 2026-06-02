<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import BaseModal from '@/components/BaseModal.vue'
import {
  listApiKeysApi,
  createApiKeyApi,
  updateApiKeyApi,
  toggleApiKeyApi,
  revokeApiKeyApi,
  type ApiKeyRecord,
} from '@/api/setting'

const { t } = useI18n()

const ALL_SCOPES = computed(() => [
  { value: 'zone:read', label: t('apiKey.scope.zoneRead') },
  { value: 'zone:write', label: t('apiKey.scope.zoneWrite') },
  { value: 'record:read', label: t('apiKey.scope.recordRead') },
  { value: 'record:write', label: t('apiKey.scope.recordWrite') },
  { value: 'cluster:read', label: t('apiKey.scope.clusterRead') },
  { value: 'stats:read', label: t('apiKey.scope.statsRead') },
  { value: 'admin', label: t('apiKey.scope.admin') },
])

const loading        = ref(false)
const keys           = ref<ApiKeyRecord[]>([])
const newKeySecret   = ref('')
const revealVisible  = ref(false)
const createVisible  = ref(false)
const editVisible    = ref(false)
const submitting     = ref(false)
const editSubmitting = ref(false)

const form = reactive({
  name: '',
  scope: [] as string[],
  expiresAt: '',
  noExpiry: true,
})

const editForm = reactive({
  id: 0,
  name: '',
  scope: [] as string[],
  expiresAt: '',
  noExpiry: true,
})

type ApiKeyStatus = 'normal' | 'disabled' | 'revoked'

const toStatusKey = (status: string): ApiKeyStatus => {
  const value = status.toLowerCase()
  if (status === '正常' || status === '启用' || value === 'normal' || value === 'enabled') {
    return 'normal'
  }
  if (status === '已禁用' || status === '禁用' || value === 'disabled') {
    return 'disabled'
  }
  return 'revoked'
}

const statusClass = (s: string) => {
  const key = toStatusKey(s)
  return key === 'normal' ? 'mn-badge--success' : key === 'disabled' ? 'mn-badge--neutral' : 'mn-badge--danger'
}

const statusLabel = (s: string) => t(`apiKey.status.${toStatusKey(s)}`)
const isNormalStatus = (s: string) => toStatusKey(s) === 'normal'

const scopeLabel = (v: string) => ALL_SCOPES.value.find(s => s.value === v)?.label ?? v

const loadKeys = async () => {
  loading.value = true
  try {
    const { data } = await listApiKeysApi()
    keys.value = data.list ?? []
  } finally {
    loading.value = false
  }
}

const openCreate = () => {
  Object.assign(form, { name: '', scope: [], expiresAt: '', noExpiry: true })
  createVisible.value = true
}

const submitCreate = async () => {
  if (!form.name.trim()) { ElMessage.warning(t('apiKey.validation.nameRequired')); return }
  if (!form.scope.length) { ElMessage.warning(t('apiKey.validation.scopeRequired')); return }
  submitting.value = true
  try {
    const { data } = await createApiKeyApi({ name: form.name, scope: form.scope, expiresAt: form.expiresAt, noExpiry: form.noExpiry })
    await loadKeys()
    createVisible.value = false
    if (data.secret) {
      newKeySecret.value = data.secret
      revealVisible.value = true
    }
  } finally {
    submitting.value = false
  }
}

const toggleStatus = async (key: ApiKeyRecord) => {
  const nextStatus: ApiKeyStatus = isNormalStatus(key.status) ? 'disabled' : 'normal'
  await ElMessageBox.confirm(
    t('apiKey.confirm.toggleMessage', { name: key.name, status: t(`apiKey.status.${nextStatus}`) }),
    t('apiKey.confirm.toggleTitle'),
    { type: 'warning' },
  )
  const { data } = await toggleApiKeyApi(key.id)
  key.status = data.status
  ElMessage.success(t('apiKey.messages.statusUpdated', { status: statusLabel(key.status) }))
}

const revokeKey = async (key: ApiKeyRecord) => {
  await ElMessageBox.confirm(
    t('apiKey.confirm.revokeMessage', { name: key.name }),
    t('apiKey.confirm.revokeTitle'),
    { type: 'error', confirmButtonText: t('apiKey.confirm.revokeButton') },
  )
  await revokeApiKeyApi(key.id)
  await loadKeys()
  ElMessage.success(t('apiKey.messages.revoked'))
}

const copyKey = async (text: string) => {
  await navigator.clipboard.writeText(text).catch(() => {})
  ElMessage.success(t('apiKey.messages.copied'))
}

const openEdit = (key: ApiKeyRecord) => {
  Object.assign(editForm, {
    id: key.id,
    name: key.name,
    scope: [...key.scope],
    expiresAt: key.expiresAt === 'never' ? '' : key.expiresAt,
    noExpiry: key.expiresAt === 'never',
  })
  editVisible.value = true
}

const submitEdit = async () => {
  if (!editForm.name.trim()) { ElMessage.warning(t('apiKey.validation.nameRequired')); return }
  if (!editForm.scope.length) { ElMessage.warning(t('apiKey.validation.scopeRequired')); return }
  editSubmitting.value = true
  try {
    await updateApiKeyApi(editForm.id, { name: editForm.name, scope: editForm.scope, expiresAt: editForm.expiresAt, noExpiry: editForm.noExpiry })
    await loadKeys()
    editVisible.value = false
  } finally {
    editSubmitting.value = false
  }
  ElMessage.success(t('apiKey.messages.updated'))
}

onMounted(loadKeys)
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('apiKey.title') }}</h1>
        <p class="page-subtitle">{{ $t('apiKey.subtitle') }}</p>
      </div>
      <el-button type="primary" @click="openCreate">
        <template #icon>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
        </template>
        {{ $t('apiKey.create') }}
      </el-button>
    </div>

    <!-- Notice -->
    <div class="notice-bar">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="notice-icon"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
      {{ $t('apiKey.notice') }}
    </div>

    <el-card class="mn-card">
      <el-table :data="keys" stripe class="mn-table" table-layout="auto">
        <el-table-column prop="name" :label="$t('apiKey.keyName')" min-width="160">
          <template #default="{ row }">
            <div class="key-name-cell">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="key-icon"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/></svg>
              <span class="key-name">{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="prefix" :label="$t('apiKey.prefix')" width="160">
          <template #default="{ row }"><code class="key-prefix">{{ row.prefix }}</code></template>
        </el-table-column>
        <el-table-column :label="$t('apiKey.permissions')" min-width="200">
          <template #default="{ row }">
            <div class="scope-list">
              <span v-for="s in row.scope" :key="s" class="scope-tag">{{ scopeLabel(s) }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="createdBy" :label="$t('apiKey.createdBy')" width="90" align="center"/>
        <el-table-column prop="lastUsedAt" :label="$t('apiKey.lastUsed')" min-width="170">
          <template #default="{ row }"><span class="mn-time">{{ row.lastUsedAt }}</span></template>
        </el-table-column>
        <el-table-column prop="expiresAt" :label="$t('apiKey.expiresAt')" width="120" align="center">
          <template #default="{ row }">
            <span v-if="row.expiresAt === 'never'" class="expire-never">{{ $t('apiKey.neverExpires') }}</span>
            <span v-else class="mn-time">{{ row.expiresAt }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" :label="$t('common.status')" width="90" align="center">
          <template #default="{ row }">
            <span :class="['mn-badge', statusClass(row.status)]">{{ statusLabel(row.status) }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="$t('common.operation')" width="200" fixed="right">
          <template #default="{ row }">
            <div class="mn-row-ops">
              <el-tooltip :content="$t('apiKey.tooltips.edit')" placement="top">
                <el-button link type="primary" @click="openEdit(row)">{{ $t('common.edit') }}</el-button>
              </el-tooltip>
              <el-tooltip :content="isNormalStatus(row.status) ? $t('apiKey.tooltips.disable') : $t('apiKey.tooltips.enable')" placement="top">
                <el-button link :type="isNormalStatus(row.status) ? 'warning' : 'success'" @click="toggleStatus(row)">
                  {{ isNormalStatus(row.status) ? $t('common.disable') : $t('common.enable') }}
                </el-button>
              </el-tooltip>
              <el-tooltip :content="$t('apiKey.tooltips.revoke')" placement="top">
                <el-button link type="danger" @click="revokeKey(row)">{{ $t('apiKey.revokeKey') }}</el-button>
              </el-tooltip>
            </div>
          </template>
        </el-table-column>
        <template #empty><el-empty :description="$t('apiKey.noData')" /></template>
      </el-table>
    </el-card>

    <!-- Create dialog -->
    <BaseModal
      v-model="createVisible"
      :title="$t('apiKey.createDialogTitle')"
      :width="500"
      :loading="submitting"
      :confirm-text="$t('apiKey.generateKey')"
      :cancel-text="$t('common.cancel')"
      @confirm="submitCreate"
    >
      <el-form :model="form" label-position="top" class="create-form">
        <el-form-item :label="$t('apiKey.keyName')" required>
          <el-input v-model="form.name" :placeholder="$t('apiKey.namePlaceholder')" maxlength="40" show-word-limit />
        </el-form-item>
        <el-form-item :label="$t('apiKey.permissions')" required>
          <div class="scope-grid">
            <label v-for="s in ALL_SCOPES" :key="s.value" class="scope-item">
              <el-checkbox v-model="form.scope" :label="s.value">{{ s.label }}</el-checkbox>
            </label>
          </div>
        </el-form-item>
        <el-form-item :label="$t('apiKey.validity')">
          <div class="expiry-row">
            <el-checkbox v-model="form.noExpiry">{{ $t('apiKey.neverExpires') }}</el-checkbox>
            <el-date-picker
              v-if="!form.noExpiry"
              v-model="form.expiresAt"
              type="date"
              :placeholder="$t('apiKey.selectExpiryDate')"
              value-format="YYYY-MM-DD"
              style="flex:1"
            />
          </div>
        </el-form-item>
      </el-form>
    </BaseModal>

    <!-- Edit dialog -->
    <BaseModal
      v-model="editVisible"
      :title="$t('apiKey.editDialogTitle')"
      :width="500"
      :loading="editSubmitting"
      :confirm-text="$t('common.save')"
      :cancel-text="$t('common.cancel')"
      @confirm="submitEdit"
    >
      <el-form :model="editForm" label-position="top" class="create-form">
        <el-form-item :label="$t('apiKey.keyName')" required>
          <el-input v-model="editForm.name" :placeholder="$t('apiKey.namePlaceholder')" maxlength="40" show-word-limit />
        </el-form-item>
        <el-form-item :label="$t('apiKey.permissions')" required>
          <div class="scope-grid">
            <label v-for="s in ALL_SCOPES" :key="s.value" class="scope-item">
              <el-checkbox v-model="editForm.scope" :label="s.value">{{ s.label }}</el-checkbox>
            </label>
          </div>
        </el-form-item>
        <el-form-item :label="$t('apiKey.validity')">
          <div class="expiry-row">
            <el-checkbox v-model="editForm.noExpiry">{{ $t('apiKey.neverExpires') }}</el-checkbox>
            <el-date-picker
              v-if="!editForm.noExpiry"
              v-model="editForm.expiresAt"
              type="date"
              :placeholder="$t('apiKey.selectExpiryDate')"
              value-format="YYYY-MM-DD"
              style="flex:1"
            />
          </div>
        </el-form-item>
      </el-form>
    </BaseModal>

    <!-- Reveal secret dialog -->
    <BaseModal
      v-model="revealVisible"
      :title="$t('apiKey.revealTitle')"
      :width="480"
      :show-confirm="false"
      :cancel-text="$t('apiKey.revealClose')"
    >
      <div class="reveal-body">
        <div class="reveal-warn">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="reveal-icon"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
          {{ $t('apiKey.revealWarning') }} <strong>{{ $t('apiKey.revealIrrecoverable') }}</strong>
        </div>
        <div class="secret-box">
          <code class="secret-text">{{ newKeySecret }}</code>
          <el-button type="primary" size="small" @click="copyKey(newKeySecret)">{{ $t('common.copy') }}</el-button>
        </div>
      </div>
    </BaseModal>
  </div>
</template>

<style scoped>
.notice-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  background: rgba(255,125,0,0.06);
  border: 1px solid rgba(255,125,0,0.25);
  border-radius: 8px;
  font-size: 13px;
  color: var(--app-warning);
}
.notice-icon { width:15px; height:15px; flex-shrink:0; }

.key-name-cell { display:flex; align-items:center; gap:7px; }
.key-icon { width:14px; height:14px; color:var(--app-accent); flex-shrink:0; }
.key-name { font-size:13px; font-weight:600; color:var(--dns-text-title-color); }
.key-prefix { font-family:'JetBrains Mono','Consolas',monospace; font-size:12px; color:var(--dns-text-body-color); background:var(--dns-bg-page); padding:2px 8px; border-radius:4px; }

.scope-list { display:flex; flex-wrap:wrap; gap:4px; }
.scope-tag { display:inline-flex; padding:2px 7px; border-radius:4px; font-size:11px; font-weight:500; background:rgba(22,93,255,0.08); color:var(--app-accent); }

.expire-never { font-size:12px; color:var(--app-success); }

.mn-row-ops { display:flex; align-items:center; justify-content:flex-end; gap:4px; }

/* Create form */
.create-form :deep(.el-form-item__label) { font-size:13px; font-weight:500; padding-bottom:4px; }
.create-form :deep(.el-form-item) { margin-bottom:16px; }
.scope-grid { display:grid; grid-template-columns:1fr 1fr; gap:4px 16px; }
.scope-item { cursor:pointer; }
.expiry-row { display:flex; align-items:center; gap:12px; width:100%; }

/* Reveal dialog */
.reveal-body { display:flex; flex-direction:column; gap:14px; }
.reveal-warn {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 12px 14px;
  background: rgba(245,63,63,0.06);
  border: 1px solid rgba(245,63,63,0.2);
  border-radius: 8px;
  font-size: 13px;
  color: var(--app-danger);
  line-height: 1.5;
}
.reveal-icon { width:16px; height:16px; flex-shrink:0; margin-top:1px; }
.secret-box {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  background: var(--dns-bg-page);
  border: 1px solid var(--dns-border-color);
  border-radius: 8px;
}
.secret-text {
  flex: 1;
  font-family: 'JetBrains Mono','Consolas',monospace;
  font-size: 12px;
  word-break: break-all;
  color: var(--dns-text-title-color);
}
</style>
