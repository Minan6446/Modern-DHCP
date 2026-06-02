<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import BaseModal from '../../components/BaseModal.vue'
import { useForwardRules } from '../../composables/setting/useForwardRules'
import { confirmRiskAction, validateFormAndFocus } from '../../utils/interaction'
import type { ForwardRule } from '../../types/modules'

const route = useRoute()
const { t } = useI18n()
const ruleFormRef = ref<FormInstance>()

const {
  searchForm,
  loading,
  pagination,
  tableData,
  dialogVisible,
  formData,
  selectedRules,
  refreshLoading,
  deletingRuleId,
  switchingRuleId,
  batchRuleLoading,
  ruleExportLoading,
  ruleImportLoading,
  reorderLoading,
  canReorder,
  lastUpdated,
  upstreamOptions,
  ruleModalTitle,
  refreshContext,
  openCreateDialog,
  openEditDialog,
  closeDialog,
  resetForm,
  handleSearch,
  resetFilters,
  handlePageChange,
  handlePageSizeChange,
  handleSelectionChange,
  handleSortChange,
  submitRule,
  removeRule,
  toggleRuleStatus,
  handleBatchRuleAction,
  exportRules,
  parseImportedRules,
  moveRulePriority,
  handlePriorityDragEnd,
  handleRuleDomainsInput,
  handleRuleRemarkInput,
  debounceAdjustRulePriority,
} = useForwardRules()

const domainPattern = /^(?=.{1,253}$)(?!-)(?:[a-zA-Z0-9-]{1,63}\.)+[a-zA-Z]{2,63}$/

const validateDomainList = (_rule: unknown, value: string, callback: (error?: Error) => void): void => {
  const items = String(value || '')
    .split(/[\s,\n]+/)
    .map((item) => item.trim())
    .filter(Boolean)

  if (!items.length) {
    callback(new Error(t('forward.valDomainRequired')))
    return
  }
  if (items.some((item) => !domainPattern.test(item))) {
    callback(new Error(t('forward.valDomainInvalid')))
    return
  }
  callback()
}

const validateUpstreamId = (_rule: unknown, value: number, callback: (error?: Error) => void): void => {
  if (!Number.isInteger(Number(value)) || Number(value) <= 0) {
    callback(new Error(t('forward.valUpstream')))
    return
  }
  callback()
}

const ruleRules = computed<FormRules>(() => ({
  domains: [{ validator: validateDomainList, trigger: ['blur', 'change'] }],
  upstreamId: [{ validator: validateUpstreamId, trigger: ['change', 'blur'] }],
  priority: [
    { required: true, message: t('forward.valPriority'), trigger: ['blur', 'change'] },
    {
      validator: (_rule: unknown, value: number, callback: (error?: Error) => void) => {
        const priority = Number(value)
        if (!Number.isInteger(priority) || priority < 1 || priority > 100) {
          callback(new Error(t('forward.valPriorityRange')))
          return
        }
        callback()
      },
      trigger: ['blur', 'change'],
    },
  ],
}))

const statusTagType = (status: string): 'success' | 'info' => (status === '启用' ? 'success' : 'info')

/* ── native drag-sort ── */
const dragFromIndex = ref<number | null>(null)
const dragOverIndex = ref<number | null>(null)

const onDragStart = (e: DragEvent, index: number): void => {
  dragFromIndex.value = index
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(index))
  }
}

const onDragOver = (e: DragEvent, index: number): void => {
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
  dragOverIndex.value = index
}

const onDrop = async (_e: DragEvent, index: number): Promise<void> => {
  const from = dragFromIndex.value
  dragFromIndex.value = null
  dragOverIndex.value = null
  if (from === null || from === index) return
  await handlePriorityDragEnd({ oldIndex: from, newIndex: index })
}

const onDragEnd = (): void => {
  dragFromIndex.value = null
  dragOverIndex.value = null
}

const handleOpenCreateDialog = (): void => {
  openCreateDialog()
  ruleFormRef.value?.clearValidate()
}

const handleOpenEditDialog = (row: ForwardRule): void => {
  openEditDialog(row)
  ruleFormRef.value?.clearValidate()
}

const handleSubmit = async (): Promise<void> => {
  const valid = await validateFormAndFocus(ruleFormRef.value)
  if (!valid) {
    return
  }

  try {
    await submitRule()
  } catch (_error) {
    ElMessage.error(t('forward.ruleSaveFailed'))
  }
}

const handleDelete = async (row: ForwardRule): Promise<void> => {
  try {
    await confirmRiskAction({
      title: t('forward.ruleDeleteTitle'),
      action: t('forward.ruleDeleteAction'),
      target: row.ruleId,
      risk: t('forward.ruleDeleteRisk'),
    })
    await removeRule(row)
  } catch (_error) {
    if (_error !== 'cancel') {
      ElMessage.error(t('forward.ruleDeleteFailed'))
    }
  }
}
</script>

<template>
  <div class="page-shell forward-rule-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('menu.' + route.meta.titleKey) }}</h1>
        <p class="page-subtitle">{{ $t('forward.title') }}</p>
      </div>
      <div class="forward-last-updated">{{ $t('common.lastRefreshTime') }}{{ lastUpdated || $t('common.loading') }}</div>
    </div>

    <el-alert
      v-if="!upstreamOptions.length"
      type="warning"
      :closable="false"
      :title="t('forward.noUpstreamAlert')"
    />

    <el-card class="mn-card">
      <div class="mn-toolbar">
        <div class="mn-toolbar-filters">
          <el-input v-model="searchForm.keyword" clearable :placeholder="t('forward.searchPlaceholder')" style="width: 240px" @keyup.enter="handleSearch">
            <template #prefix><svg class="mn-input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></template>
          </el-input>
          <el-select v-model="searchForm.status" clearable :placeholder="t('forward.filterStatus')" style="width: 120px">
            <el-option :label="t('forward.enabled')" value="启用" />
            <el-option :label="t('forward.disabled')" value="禁用" />
          </el-select>
          <el-date-picker
            v-model="searchForm.timeRange"
            type="daterange"
            value-format="YYYY-MM-DD"
            :range-separator="t('forward.dateRangeSep')"
            :start-placeholder="t('forward.dateStart')"
            :end-placeholder="t('forward.dateEnd')"
          />
          <el-button type="primary" @click="handleSearch">{{ t('forward.search') }}</el-button>
          <el-button @click="resetFilters">{{ t('forward.reset') }}</el-button>
          <el-button :loading="refreshLoading" :disabled="refreshLoading || batchRuleLoading" @click="refreshContext">
            <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></template>
            {{ t('forward.refresh') }}
          </el-button>
        </div>
        <div class="mn-toolbar-actions">
          <el-button type="primary" :disabled="!upstreamOptions.length" @click="handleOpenCreateDialog">
            <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg></template>
            {{ t('forward.addRule') }}
          </el-button>
          <el-button :loading="batchRuleLoading" :disabled="batchRuleLoading || !selectedRules.length" @click="handleBatchRuleAction('enable')">{{ t('forward.batchEnable') }}</el-button>
          <el-button :loading="batchRuleLoading" :disabled="batchRuleLoading || !selectedRules.length" @click="handleBatchRuleAction('disable')">{{ t('forward.batchDisable') }}</el-button>
          <el-button type="danger" plain :loading="batchRuleLoading" :disabled="batchRuleLoading || !selectedRules.length" @click="handleBatchRuleAction('delete')">{{ t('forward.batchDelete') }}</el-button>
          <el-upload :show-file-list="false" :auto-upload="false" :before-upload="parseImportedRules" accept=".xlsx,.xls,.json">
            <el-button :loading="ruleImportLoading" :disabled="ruleImportLoading || ruleExportLoading">{{ t('forward.import') }}</el-button>
          </el-upload>
          <el-dropdown @command="exportRules">
            <el-button :loading="ruleExportLoading" :disabled="ruleExportLoading || ruleImportLoading">
              <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg></template>
              {{ t('forward.exportRules') }}
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="xlsx">{{ t('forward.exportExcel') }}</el-dropdown-item>
                <el-dropdown-item command="json">{{ t('forward.exportJson') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>

      <template v-if="tableData.length">
        <el-table
          class="mn-table"
          :data="tableData"
          v-loading="loading"
          stripe
          row-key="id"
          height="560"
          :row-class-name="({ rowIndex }) => dragOverIndex === rowIndex ? 'drag-over-row' : ''"
          @selection-change="handleSelectionChange"
          @sort-change="handleSortChange"
        >
          <el-table-column width="40" align="center">
            <template #default="{ $index }">
              <div
                class="drag-grip"
                draggable="true"
                @dragstart="onDragStart($event, $index)"
                @dragover="onDragOver($event, $index)"
                @drop="onDrop($event, $index)"
                @dragend="onDragEnd"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="14" height="14"><circle cx="9" cy="5" r="1"/><circle cx="15" cy="5" r="1"/><circle cx="9" cy="12" r="1"/><circle cx="15" cy="12" r="1"/><circle cx="9" cy="19" r="1"/><circle cx="15" cy="19" r="1"/></svg>
              </div>
            </template>
          </el-table-column>
          <el-table-column type="selection" width="52" />
          <el-table-column prop="ruleId" :label="t('forward.ruleId')" min-width="140" sortable="custom" />
          <el-table-column prop="domains" :label="t('forward.domains')" min-width="260" sortable="custom" show-overflow-tooltip />
          <el-table-column :label="t('forward.upstreamServer')" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">{{ row.upstreamName }}</template>
          </el-table-column>
          <el-table-column prop="priority" :label="t('forward.priorityLabel')" width="120" sortable="custom" align="right">
            <template #default="{ row }">
              <div class="priority-cell">
                <svg class="drag-handle" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="9" cy="5" r="1"/><circle cx="15" cy="5" r="1"/><circle cx="9" cy="12" r="1"/><circle cx="15" cy="12" r="1"/><circle cx="9" cy="19" r="1"/><circle cx="15" cy="19" r="1"/></svg>
                <span class="mn-mono">{{ row.priority }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column :label="t('forward.statusLabel')" width="100" align="center">
            <template #default="{ row }">
              <span :class="['mn-badge', row.status === '启用' ? 'mn-badge--success' : 'mn-badge--neutral']">{{ row.status === '启用' ? t('forward.enabled') : t('forward.disabled') }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="createdAt" :label="t('forward.createdAt')" min-width="170" sortable="custom">
            <template #default="{ row }"><span class="mn-time">{{ row.createdAt }}</span></template>
          </el-table-column>
          <el-table-column :label="t('forward.operation')" width="220" align="right">
            <template #default="{ row }">
              <div class="mn-row-ops">
                <el-button link type="primary" :disabled="Boolean(deletingRuleId) || batchRuleLoading" @click="handleOpenEditDialog(row)">{{ t('forward.edit') }}</el-button>
                <el-switch
                  :model-value="row.status === '启用'"
                  :loading="switchingRuleId === row.id"
                  :disabled="Boolean(switchingRuleId) || batchRuleLoading || Boolean(deletingRuleId)"
                  inline-prompt
                  :active-text="t('forward.enableSwitch')"
                  :inactive-text="t('forward.disableSwitch')"
                  @change="toggleRuleStatus(row, $event)"
                />
                <el-button link type="danger" :loading="deletingRuleId === row.id" :disabled="Boolean(deletingRuleId) || batchRuleLoading" @click="handleDelete(row)">{{ t('forward.delete') }}</el-button>
              </div>
            </template>
          </el-table-column>
        </el-table>
        <div class="mn-pagination">
          <el-pagination
            :current-page="pagination.currentPage"
            :page-size="pagination.pageSize"
            size="small"
            background
            layout="total, sizes, prev, pager, next"
            :page-sizes="[10, 20, 50]"
            :total="pagination.total"
            @current-change="handlePageChange"
            @size-change="handlePageSizeChange"
          />
        </div>
      </template>

      <el-empty v-else :description="t('forward.noRules')" style="padding: 32px 0" />
    </el-card>

    <BaseModal
      v-model="dialogVisible"
      class="rule-modal"
      :title="ruleModalTitle"
      :width="520"
      :loading="loading"
      :confirm-disabled="loading"
      :confirm-text="t('forward.save')"
      :cancel-text="t('forward.cancel')"
      @confirm="handleSubmit"
      @cancel="closeDialog"
      @close="closeDialog"
      @closed="resetForm"
    >
      <el-form ref="ruleFormRef" :model="formData" :rules="ruleRules" label-width="110px" label-position="left" class="forward-rule-modal-form">
        <el-form-item :label="t('forward.domains')" prop="domains">
          <el-input
            v-model="formData.domains"
            type="textarea"
            :rows="3"
            resize="vertical"
            maxlength="2000"
            :placeholder="t('forward.domainPlaceholder')"
            @input="handleRuleDomainsInput"
            @blur="handleRuleDomainsInput"
          />
          <div class="field-tip">{{ t('forward.domainTip') }}</div>
        </el-form-item>
        <el-form-item :label="t('forward.upstreamServer')" prop="upstreamId">
          <el-select v-model="formData.upstreamId" :placeholder="t('forward.upstreamPlaceholder')">
            <el-option v-for="item in upstreamOptions" :key="item.value" :label="item.label" :value="item.value" :disabled="item.disabled" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('forward.priorityLabel')" prop="priority">
          <el-input-number v-model="formData.priority" :min="1" :max="100" controls-position="right" style="width: 100%" @change="debounceAdjustRulePriority" />
          <div class="field-tip">{{ t('forward.priorityTip2') }}</div>
        </el-form-item>
        <el-form-item :label="t('forward.remarkLabel')">
          <el-input v-model="formData.remark" type="textarea" :rows="3" resize="vertical" maxlength="100" show-word-limit :placeholder="t('forward.remarkPlaceholder')" @input="handleRuleRemarkInput" />
        </el-form-item>
        <el-form-item :label="t('forward.statusLabel')" class="rule-status-item">
          <el-switch v-model="formData.status" active-value="启用" inactive-value="禁用" />
        </el-form-item>
      </el-form>
    </BaseModal>
  </div>
</template>

<style scoped>
/* ═══════════════ Page ═══════════════ */
.forward-rule-shell {
  gap: 16px;
}

.forward-last-updated {
  font-size: 12px;
  color: var(--app-text-regular);
}

/* ═══════════════ Card ═══════════════ */
.mn-card {
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  overflow: hidden;
}

.mn-card :deep(.el-card__body) {
  padding: 0;
}

/* ═══════════════ Toolbar ═══════════════ */
.mn-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
  padding: 12px 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

.mn-toolbar-filters {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  flex: 1;
}

.mn-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  flex-shrink: 0;
}

.mn-input-icon {
  width: 13px;
  height: 13px;
  color: var(--app-text-regular);
}

/* ═══════════════ Table ═══════════════ */
.mn-table :deep(.el-table__header th) {
  background: var(--app-bg-secondary);
  color: var(--app-text-secondary);
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.mn-table :deep(.el-table__body tr:hover > td.el-table__cell) {
  background: rgba(22, 93, 255, 0.04) !important;
}

.mn-row-ops {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.priority-cell {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
}

.priority-actions {
  display: flex;
  gap: 6px;
}

/* ═══════════════ Pagination ═══════════════ */
.mn-pagination {
  display: flex;
  justify-content: flex-end;
  padding: 12px 16px;
  border-top: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}

/* ═══════════════ Badges ═══════════════ */
.mn-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px 9px;
  border-radius: 20px;
  font-size: 12px;
  font-weight: 500;
  line-height: 1.6;
  white-space: nowrap;
  border: 1px solid transparent;
}

.mn-badge--neutral {
  color: var(--app-text-secondary);
  background: rgba(78, 89, 105, 0.08);
  border-color: rgba(78, 89, 105, 0.18);
}

.mn-badge--success {
  color: var(--app-success);
  background: rgba(0, 180, 42, 0.08);
  border-color: rgba(0, 180, 42, 0.22);
}

/* ═══════════════ Mono / time ═══════════════ */
.mn-mono {
  font-family: var(--app-font-mono, 'JetBrains Mono', 'Consolas', monospace);
  font-size: 12px;
}

.mn-time {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--app-text-regular);
}

/* ═══════════════ Modal form ═══════════════ */
.field-tip {
  margin-top: 6px;
  font-size: 12px;
  color: var(--app-text-regular);
  line-height: 1.5;
}

/* ═══════════════ Drag sort ═══════════════ */
.drag-grip {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  cursor: grab;
  color: var(--app-text-secondary);
  border-radius: 4px;
  transition: color 0.15s, background 0.15s;
  margin: 0 auto;
}

.drag-grip:hover {
  color: var(--app-accent);
  background: rgba(22, 93, 255, 0.06);
}

.drag-grip:active { cursor: grabbing; }

.drag-handle {
  width: 12px;
  height: 12px;
  color: var(--app-text-secondary);
}

.priority-cell {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
}

.priority-actions {
  display: flex;
  gap: 6px;
}

/* deep selector to highlight drag-over row */
.mn-table :deep(.drag-over-row > td) {
  background: rgba(22, 93, 255, 0.06) !important;
  box-shadow: inset 0 -2px 0 var(--app-accent);
}

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 1200px) {
  .mn-toolbar {
    flex-direction: column;
    align-items: flex-start;
  }

  .mn-toolbar-actions {
    justify-content: flex-start;
  }
}
</style>
