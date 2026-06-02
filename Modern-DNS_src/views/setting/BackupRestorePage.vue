<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { UploadFilled } from '@element-plus/icons-vue'
import { useBackupRestore } from '../../composables/setting/useBackupRestore'

const route = useRoute()
const { t } = useI18n()
const activeTab = ref('backup')

const {
  backupScopeOptions,
  backupFormatOptions,
  filterForm,
  backupForm,
  restoreForm,
  pagination,
  loading,
  backupCreating,
  backupDeletingId,
  uploadLoading,
  restoreLoading,
  backupList,
  recentHistoryRecords,
  fetchBackupList,
  handleSearch,
  handleResetFilter,
  handlePageChange,
  handleCreateBackup,
  handleDownloadBackup,
  handleDeleteBackup,
  beforeBackupUpload,
  handleRestore,
  formatDateTime,
  scopeLabel,
  autoBackupConfig,
  autoBackupLoading,
  autoBackupSaving,
  fetchAutoBackupConfig,
  saveAutoBackupConfig,
} = useBackupRestore()

// ════ Parse "2.4MB" / "512KB" etc. into MB ════
const parseFileSizeMB = (size: string): number => {
  const m = /([\d.]+)\s*(KB|MB|GB|B)?/i.exec(String(size || ''))
  if (!m) return 0
  const n = parseFloat(m[1])
  const unit = (m[2] || 'MB').toUpperCase()
  if (unit === 'GB') return n * 1024
  if (unit === 'KB') return n / 1024
  if (unit === 'B') return n / (1024 * 1024)
  return n
}

const stats = computed(() => {
  const list = recentHistoryRecords.value
  const latest = list[0]
  const totalSizeMB = list.reduce((sum, item) => sum + parseFileSizeMB(item.fileSize), 0)
  const jsonCount = list.filter((item) => item.format === 'JSON').length
  return {
    total: pagination.total || list.length,
    latestTime: latest ? formatDateTime(latest.backupTime) : t('common.none'),
    totalSize: totalSizeMB >= 1024
      ? `${(totalSizeMB / 1024).toFixed(2)} GB`
      : `${totalSizeMB.toFixed(1)} MB`,
    jsonRatio: list.length ? Math.round((jsonCount / list.length) * 100) : 0,
  }
})

onMounted(async () => {
  // Load list and auto-backup config in parallel — they don't depend on
  // each other, and the auto-backup form should be ready by the time
  // the operator tabs into the schedule panel.
  await Promise.all([
    fetchBackupList(),
    fetchAutoBackupConfig(),
  ])
})
</script>

<template>
  <div class="page-shell backup-page">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('menu.' + route.meta.titleKey) }}</h1>
        <p class="page-subtitle">{{ $t('backup.subtitle') }}</p>
      </div>
    </div>

    <!-- Stats strip -->
    <div class="bk-stats">
      <div class="bk-stat">
        <div class="bk-stat-icon bk-stat-icon-total">
          <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M20 6h-8l-2-2H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2z" /></svg>
        </div>
        <div class="bk-stat-body">
          <div class="bk-stat-label">{{ $t('backup.stats.total') }}</div>
          <div class="bk-stat-value">{{ stats.total }}</div>
        </div>
      </div>
      <div class="bk-stat">
        <div class="bk-stat-icon bk-stat-icon-time">
          <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M11.99 2C6.47 2 2 6.48 2 12s4.47 10 9.99 10C17.52 22 22 17.52 22 12S17.52 2 11.99 2zM12 20a8 8 0 1 1 0-16 8 8 0 0 1 0 16zm.5-13H11v6l5.25 3.15.75-1.23-4.5-2.67z" /></svg>
        </div>
        <div class="bk-stat-body">
          <div class="bk-stat-label">{{ $t('backup.stats.latestTime') }}</div>
          <div class="bk-stat-value bk-stat-value-sm">{{ stats.latestTime }}</div>
        </div>
      </div>
      <div class="bk-stat">
        <div class="bk-stat-icon bk-stat-icon-size">
          <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M20 6h-8l-2-2H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2zm-1 12H5V8h14z" /></svg>
        </div>
        <div class="bk-stat-body">
          <div class="bk-stat-label">{{ $t('backup.stats.totalSize') }}</div>
          <div class="bk-stat-value">{{ stats.totalSize }}</div>
        </div>
      </div>
      <div class="bk-stat">
        <div class="bk-stat-icon bk-stat-icon-ratio">
          <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8zm4 18H6V4h7v5h5z" /></svg>
        </div>
        <div class="bk-stat-body">
          <div class="bk-stat-label">{{ $t('backup.stats.jsonRatio') }}</div>
          <div class="bk-stat-value">{{ stats.jsonRatio }}%</div>
        </div>
      </div>
    </div>

    <el-card class="content-card bk-main-card">
      <el-tabs v-model="activeTab" class="bk-tabs">

        <!-- ═══════════════ Tab 1: 备份管理 ═══════════════ -->
        <el-tab-pane name="backup">
          <template #label>
            <span class="bk-tab-label">
              <svg class="bk-tab-icon" viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M19 9h-4V3H9v6H5l7 7zM5 18v2h14v-2z" /></svg>
              {{ $t('backup.tabBackup') }}
            </span>
          </template>

          <div class="bk-two-col">
            <!-- Left: 配置备份 -->
            <div class="bk-panel">
              <div class="bk-panel-header">
                <div class="bk-section-icon bk-section-icon-export">
                  <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M19 9h-4V3H9v6H5l7 7zM5 18v2h14v-2z" /></svg>
                </div>
                <div class="bk-panel-title-block">
                  <div class="bk-panel-title">{{ $t('backup.backupPanelTitle') }}</div>
                  <div class="bk-panel-desc">{{ $t('backup.backupPanelDesc') }}</div>
                </div>
              </div>
              <div class="bk-panel-body">
                <el-form label-position="top" class="bk-form" v-loading="backupCreating">
                  <el-form-item :label="$t('backup.backupScope')">
                    <el-checkbox-group v-model="backupForm.backupScope" class="bk-checkboxes">
                      <el-checkbox v-for="item in backupScopeOptions" :key="item.value" :label="item.value" :value="item.value">{{ item.label }}</el-checkbox>
                    </el-checkbox-group>
                  </el-form-item>
                  <el-form-item :label="$t('backup.backupFormat')">
                    <el-select v-model="backupForm.backupFormat" class="bk-select">
                      <el-option v-for="item in backupFormatOptions" :key="item" :label="item" :value="item" />
                    </el-select>
                  </el-form-item>
                  <el-form-item class="bk-action-item">
                    <el-button type="primary" :loading="backupCreating" :disabled="backupCreating" @click="handleCreateBackup">
                      <el-icon><svg viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M19 9h-4V3H9v6H5l7 7zM5 18v2h14v-2z" /></svg></el-icon>
                      {{ $t('backup.createBackup') }}
                    </el-button>
                  </el-form-item>
                </el-form>
              </div>
            </div>

            <!-- Right: 最近备份记录 -->
            <div class="bk-panel">
              <div class="bk-panel-header">
                <div class="bk-section-icon bk-section-icon-recent">
                  <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M13 3a9 9 0 0 0-9 9H1l3.89 3.89.07.14L9 12H6a7 7 0 1 1 2.05 4.95l-1.42 1.41A9 9 0 1 0 13 3zm-1 5v5l4.28 2.54.72-1.21L13.5 12.25V8z" /></svg>
                </div>
                <div class="bk-panel-title-block">
                  <div class="bk-panel-title">{{ $t('backup.recentTitle') }}</div>
                  <div class="bk-panel-desc">{{ $t('backup.recentDesc') }}</div>
                </div>
                <span class="bk-count-badge">{{ recentHistoryRecords.length }}</span>
              </div>
              <div class="bk-panel-body bk-panel-body-scroll">
                <el-empty v-if="!recentHistoryRecords.length && !loading" :image-size="60" :description="$t('backup.noRecent')" />
                <div v-else class="bk-recent-list">
                  <div v-for="item in recentHistoryRecords" :key="item.id" class="bk-recent-item">
                    <div class="bk-recent-main">
                      <div class="bk-recent-id">
                        {{ item.backupId }}
                        <span class="bk-format-tag" :class="item.format === 'JSON' ? 'is-json' : 'is-excel'">{{ item.format }}</span>
                      </div>
                      <div class="bk-recent-scope">{{ item.backupScope.map((scope) => scopeLabel(scope)).join(' / ') }}</div>
                    </div>
                    <div class="bk-recent-meta">
                      <span class="bk-recent-time">{{ formatDateTime(item.backupTime) }}</span>
                      <span class="bk-recent-size">{{ item.fileSize }}</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </el-tab-pane>

        <!-- ═══════════════ Tab 2: 还原管理 ═══════════════ -->
        <el-tab-pane name="restore">
          <template #label>
            <span class="bk-tab-label">
              <svg class="bk-tab-icon" viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M9 16h6v-6h4l-7-7-7 7h4zm-4 2h14v2H5z" /></svg>
              {{ $t('backup.tabRestore') }}
            </span>
          </template>

          <div class="bk-panel">
            <div class="bk-panel-header">
              <div class="bk-section-icon bk-section-icon-restore">
                <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M9 16h6v-6h4l-7-7-7 7h4zm-4 2h14v2H5z" /></svg>
              </div>
              <div class="bk-panel-title-block">
                <div class="bk-panel-title">{{ $t('backup.restorePanelTitle') }}</div>
                <div class="bk-panel-desc">{{ $t('backup.restorePanelDesc') }}</div>
              </div>
              <div v-if="restoreForm.fileReady" class="bk-panel-badge is-active">{{ $t('backup.fileReady') }}</div>
              <div v-else class="bk-panel-badge is-inactive">{{ $t('backup.filePending') }}</div>
            </div>
            <div class="bk-panel-body">
              <el-upload
                drag
                action="#"
                :show-file-list="false"
                :before-upload="beforeBackupUpload"
                accept=".json,.xlsx,.xls"
                class="bk-upload"
                :disabled="uploadLoading || restoreLoading"
              >
                <el-icon class="bk-upload-icon"><UploadFilled /></el-icon>
                <div class="bk-upload-text">{{ $t('backup.dragOrUpload') }}</div>
                <div class="bk-upload-tip">{{ $t('backup.uploadTip') }}</div>
              </el-upload>

              <el-form label-position="top" class="bk-form bk-restore-form" v-loading="restoreLoading || uploadLoading">
                <div class="bk-fields-row">
                  <el-form-item :label="$t('backup.currentFile')">
                    <div class="bk-readonly-value">{{ restoreForm.fileName || $t('common.none') }}</div>
                  </el-form-item>
                  <el-form-item :label="$t('backup.fileFormat')">
                    <div class="bk-readonly-value">
                      <span v-if="restoreForm.format" class="bk-format-tag" :class="restoreForm.format === 'JSON' ? 'is-json' : 'is-excel'">{{ restoreForm.format }}</span>
                      <span v-else>{{ $t('common.none') }}</span>
                    </div>
                  </el-form-item>
                </div>
                <el-form-item :label="$t('backup.restorableScope')">
                  <el-checkbox-group v-model="restoreForm.restoreScope" class="bk-checkboxes">
                    <el-checkbox v-for="item in restoreForm.availableScope" :key="item" :label="item" :value="item">{{ scopeLabel(item) }}</el-checkbox>
                  </el-checkbox-group>
                  <div v-if="!restoreForm.availableScope.length" class="bk-assist-text">{{ $t('backup.restorableScopeHint') }}</div>
                </el-form-item>
                <el-form-item v-if="restoreForm.uploadProgress > 0" :label="$t('backup.uploadProgress')">
                  <el-progress :percentage="restoreForm.uploadProgress" :stroke-width="8" />
                </el-form-item>
                <el-form-item v-if="restoreForm.restoreProgress > 0" :label="$t('backup.restoreProgress')">
                  <el-progress :percentage="restoreForm.restoreProgress" :stroke-width="8" status="warning" />
                </el-form-item>
                <el-form-item class="bk-action-item">
                  <el-button type="warning" class="bk-restore-btn" :loading="restoreLoading" :disabled="restoreLoading || uploadLoading || !restoreForm.fileReady" @click="handleRestore">
                    <el-icon><svg viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M9 16h6v-6h4l-7-7-7 7h4zm-4 2h14v2H5z" /></svg></el-icon>
                    {{ $t('backup.startRestore') }}
                  </el-button>
                </el-form-item>
              </el-form>
            </div>
          </div>
        </el-tab-pane>

        <!-- ═══════════════ Tab 3: 自动备份 ═══════════════ -->
        <el-tab-pane name="schedule">
          <template #label>
            <span class="bk-tab-label">
              <svg class="bk-tab-icon" viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M12 20q-3.35 0-5.675-2.325Q4 15.35 4 12t2.325-5.675Q8.65 4 12 4q1.725 0 3.3.7T18 6.75V4h2v7h-7V9h4.2q-.8-1.4-2.187-2.2Q13.625 6 12 6 9.5 6 7.75 7.75T6 12q0 2.5 1.75 4.25T12 18q1.925 0 3.475-1.1T17.65 14h2.1q-.7 2.65-2.85 4.325Q14.75 20 12 20Zm2.8-4.8L11 11.4V7h2v3.6l3.2 3.2Z"/></svg>
              {{ $t('backup.tabAuto') }}
            </span>
          </template>

          <div class="bk-panel">
            <div class="bk-panel-header">
              <div class="bk-section-icon bk-section-icon-export">
                <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M12 20q-3.35 0-5.675-2.325Q4 15.35 4 12t2.325-5.675Q8.65 4 12 4q1.725 0 3.3.7T18 6.75V4h2v7h-7V9h4.2q-.8-1.4-2.187-2.2Q13.625 6 12 6 9.5 6 7.75 7.75T6 12q0 2.5 1.75 4.25T12 18q1.925 0 3.475-1.1T17.65 14h2.1q-.7 2.65-2.85 4.325Q14.75 20 12 20Zm2.8-4.8L11 11.4V7h2v3.6l3.2 3.2Z"/></svg>
              </div>
              <div class="bk-panel-title-block">
                <div class="bk-panel-title">{{ $t('backup.autoPanelTitle') }}</div>
                <div class="bk-panel-desc">{{ $t('backup.autoPanelDesc') }}</div>
              </div>
              <div class="bk-panel-badge" :class="autoBackupConfig.autoBackup ? 'is-active' : 'is-inactive'">
                {{ autoBackupConfig.autoBackup ? $t('backup.autoEnabled') : $t('backup.autoDisabled') }}
              </div>
            </div>
            <div class="bk-panel-body">
              <el-form label-position="top" class="bk-form" v-loading="autoBackupLoading">
                <div class="bk-fields-row">
                  <el-form-item :label="$t('backup.autoSwitch')">
                    <div class="bk-auto-switch-row">
                      <el-switch v-model="autoBackupConfig.autoBackup" />
                      <span class="bk-assist-text">{{ $t('backup.autoSwitchDesc') }}</span>
                    </div>
                  </el-form-item>
                  <el-form-item :label="$t('backup.autoCycle')">
                    <el-select v-model="autoBackupConfig.backupCycle" :disabled="!autoBackupConfig.autoBackup" class="bk-select">
                      <el-option :label="$t('backup.cycleDaily')" value="daily" />
                      <el-option :label="$t('backup.cycleWeekly')" value="weekly" />
                      <el-option :label="$t('backup.cycleMonthly')" value="monthly" />
                    </el-select>
                  </el-form-item>
                </div>
                <div class="bk-fields-row">
                  <el-form-item :label="$t('backup.autoRetention')">
                    <el-input-number
                      v-model="autoBackupConfig.backupRetentionDays"
                      :min="1"
                      :max="365"
                      :step="1"
                      :disabled="!autoBackupConfig.autoBackup"
                      class="bk-select"
                    />
                    <div class="bk-assist-text">{{ $t('backup.autoRetentionHint') }}</div>
                  </el-form-item>
                  <el-form-item :label="$t('backup.autoStorageType')">
                    <el-select v-model="autoBackupConfig.backupStorageType" :disabled="!autoBackupConfig.autoBackup" class="bk-select">
                      <el-option :label="$t('backup.storageLocal')" value="local" />
                      <el-option :label="$t('backup.storageS3')" value="s3" />
                      <el-option :label="$t('backup.storageSftp')" value="sftp" />
                    </el-select>
                  </el-form-item>
                </div>
                <el-form-item :label="$t('backup.autoStoragePath')">
                  <el-input
                    v-model="autoBackupConfig.backupStoragePath"
                    :placeholder="$t('backup.autoStoragePathPlaceholder')"
                    :disabled="!autoBackupConfig.autoBackup"
                  />
                  <div class="bk-assist-text">{{ $t('backup.autoStoragePathHint') }}</div>
                </el-form-item>
                <el-form-item class="bk-action-item">
                  <el-button
                    type="primary"
                    :loading="autoBackupSaving"
                    :disabled="autoBackupSaving || autoBackupLoading"
                    @click="saveAutoBackupConfig"
                  >
                    {{ $t('backup.autoSave') }}
                  </el-button>
                  <el-button :disabled="autoBackupSaving || autoBackupLoading" @click="fetchAutoBackupConfig">
                    {{ $t('common.reset') }}
                  </el-button>
                </el-form-item>
              </el-form>
            </div>
          </div>
        </el-tab-pane>

        <!-- ═══════════════ Tab 4: 历史记录 ═══════════════ -->
        <el-tab-pane name="history">
          <template #label>
            <span class="bk-tab-label">
              <svg class="bk-tab-icon" viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M13 3a9 9 0 0 0-9 9H1l3.89 3.89.07.14L9 12H6a7 7 0 1 1 2.05 4.95l-1.42 1.41A9 9 0 1 0 13 3zm-1 5v5l4.28 2.54.72-1.21L13.5 12.25V8z" /></svg>
              {{ $t('backup.tabHistory') }}
            </span>
          </template>

          <div class="bk-toolbar">
            <div class="bk-filter-row">
              <el-input
                v-model="filterForm.keyword"
                clearable
                :placeholder="$t('backup.searchPlaceholder')"
                class="bk-search"
                @keyup.enter="handleSearch"
              >
                <template #prefix>
                  <el-icon><svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M15.5 14h-.79l-.28-.27a6.5 6.5 0 1 0-.7.7l.27.28v.79l5 5 1.5-1.5-5-5zm-6 0A4.5 4.5 0 1 1 14 9.5 4.5 4.5 0 0 1 9.5 14z" /></svg></el-icon>
                </template>
              </el-input>
              <el-select v-model="filterForm.format" clearable :placeholder="$t('backup.allFormats')" class="bk-filter">
                <el-option :label="$t('backup.allFormats')" value="" />
                <el-option label="JSON" value="JSON" />
                <el-option label="Excel" value="Excel" />
              </el-select>
              <el-button type="primary" @click="handleSearch">{{ $t('common.search') }}</el-button>
              <el-button @click="handleResetFilter">{{ $t('common.reset') }}</el-button>
            </div>
            <el-button :loading="loading" :disabled="loading" @click="fetchBackupList">
              <el-icon><svg viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M17.65 6.35A8 8 0 1 0 19.73 14h-2.08A6 6 0 1 1 12 6a5.9 5.9 0 0 1 4.22 1.78L13 11h7V4z" /></svg></el-icon>
              {{ $t('common.refresh') }}
            </el-button>
          </div>

          <el-table v-loading="loading" :data="backupList" class="bk-table" row-key="id">
            <el-table-column prop="backupId" :label="$t('backup.backupId')" min-width="180">
              <template #default="{ row }">
                <span class="bk-backup-id">{{ row.backupId }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="$t('backup.backupScope')" min-width="220">
              <template #default="{ row }">
                <div class="bk-scope-chips">
                  <span v-for="s in row.backupScope" :key="s" class="bk-scope-chip">{{ scopeLabel(s) }}</span>
                </div>
              </template>
            </el-table-column>
            <el-table-column :label="$t('backup.format')" width="100" align="center">
              <template #default="{ row }">
                <span class="bk-format-tag" :class="row.format === 'JSON' ? 'is-json' : 'is-excel'">{{ row.format }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="$t('backup.backupTime')" width="180">
              <template #default="{ row }">
                <span class="bk-datetime">{{ formatDateTime(row.backupTime) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="fileSize" :label="$t('backup.fileSize')" width="110" align="right">
              <template #default="{ row }">
                <span class="bk-size">{{ row.fileSize }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="$t('common.operation')" width="140" fixed="right">
              <template #default="{ row }">
                <div class="bk-actions">
                  <el-button link type="primary" @click="handleDownloadBackup(row)">{{ $t('backup.download') }}</el-button>
                  <el-divider direction="vertical" />
                  <el-button link type="danger" :loading="backupDeletingId === row.id" :disabled="Boolean(backupDeletingId)" @click="handleDeleteBackup(row)">{{ $t('common.delete') }}</el-button>
                </div>
              </template>
            </el-table-column>
            <template #empty>
              <div class="bk-empty">
                <el-empty :image-size="80" :description="$t('backup.noMatch')" />
              </div>
            </template>
          </el-table>

          <div class="bk-pagination">
            <el-pagination
              :current-page="pagination.page"
              :page-size="pagination.size"
              layout="total, sizes, prev, pager, next, jumper"
              background
              :page-sizes="[10, 20, 50, 100]"
              :total="pagination.total"
              @current-change="(page) => handlePageChange(page)"
              @size-change="(size) => handlePageChange(1, size)"
            />
          </div>
        </el-tab-pane>

      </el-tabs>
    </el-card>
  </div>
</template>

<style scoped>
/* ═══════════════ Page shell ═══════════════ */
.backup-page {
  font-size: 13px;
}

.backup-page :deep(.el-card__body) {
  padding: 0;
}

.bk-main-card {
  overflow: hidden;
}

/* ═══════════════ Stats strip ═══════════════ */
.bk-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.bk-stat {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px 20px;
  background: var(--app-bg-secondary);
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  transition: box-shadow 0.2s, transform 0.2s;
}

.bk-stat:hover {
  box-shadow: 0 6px 18px rgba(22, 93, 255, 0.12);
  transform: translateY(-1px);
}

.bk-stat-icon {
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  flex-shrink: 0;
}

.bk-stat-icon-total { background: var(--app-accent-soft); color: var(--app-accent); }
.bk-stat-icon-time { background: rgba(20, 201, 201, 0.1); color: #14C9C9; }
.bk-stat-icon-size { background: rgba(114, 46, 209, 0.1); color: #722ED1; }
.bk-stat-icon-ratio { background: rgba(0, 180, 42, 0.1); color: var(--app-success); }

.bk-stat-body {
  flex: 1;
  min-width: 0;
}

.bk-stat-label {
  font-size: 12px;
  color: var(--app-text-regular);
  margin-bottom: 4px;
}

.bk-stat-value {
  font-size: 22px;
  font-weight: 700;
  color: var(--app-title);
  line-height: 1.15;
  font-variant-numeric: tabular-nums;
}

.bk-stat-value-sm {
  font-size: 14px;
  font-weight: 600;
}

/* ═══════════════ Tabs ═══════════════ */
.bk-tabs :deep(.el-tabs__header) {
  margin: 0;
  padding: 0 20px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

.bk-tabs :deep(.el-tabs__nav-wrap::after) {
  display: none;
}

.bk-tabs :deep(.el-tabs__item) {
  font-size: 14px;
  height: 48px;
  line-height: 48px;
  padding: 0 20px !important;
}

.bk-tabs :deep(.el-tabs__content) {
  padding: 20px;
}

.bk-tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 500;
}

.bk-tab-icon {
  flex-shrink: 0;
}

/* ═══════════════ Two-column layout (Tab 1) ═══════════════ */
.bk-two-col {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 16px;
}

/* ═══════════════ Panel ═══════════════ */
.bk-panel {
  background: var(--app-bg-secondary);
  border: 1px solid var(--app-border);
  border-radius: 10px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.bk-panel-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--app-border);
  background: linear-gradient(180deg, #FAFBFC 0%, #FFFFFF 100%);
}

.bk-section-icon {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  flex-shrink: 0;
}

.bk-section-icon-export { background: var(--app-accent-soft); color: var(--app-accent); }
.bk-section-icon-recent { background: rgba(20, 201, 201, 0.1); color: #14C9C9; }
.bk-section-icon-restore { background: rgba(255, 125, 0, 0.1); color: var(--app-warning); }

.bk-panel-title-block {
  flex: 1;
  min-width: 0;
}

.bk-panel-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-title);
  line-height: 1.4;
}

.bk-panel-desc {
  font-size: 12px;
  color: var(--app-text-regular);
  margin-top: 2px;
}

.bk-panel-badge {
  padding: 3px 10px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
  flex-shrink: 0;
  border: 1px solid transparent;
}

.bk-panel-badge.is-active {
  background: rgba(0, 180, 42, 0.1);
  color: var(--app-success);
  border-color: rgba(0, 180, 42, 0.25);
}

.bk-panel-badge.is-inactive {
  background: rgba(134, 144, 156, 0.08);
  color: var(--app-disabled);
  border-color: var(--app-border);
}

.bk-count-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 22px;
  height: 22px;
  padding: 0 8px;
  border-radius: 11px;
  background: var(--app-accent-soft);
  color: var(--app-accent);
  font-size: 12px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.bk-panel-body {
  padding: 18px 20px;
  flex: 1;
}

.bk-panel-body-scroll {
  max-height: 360px;
  overflow-y: auto;
}

/* ═══════════════ Form ═══════════════ */
.bk-form :deep(.el-form-item) {
  margin-bottom: 18px;
}

.bk-form :deep(.el-form-item:last-child) {
  margin-bottom: 0;
}

.bk-form :deep(.el-form-item__label) {
  font-size: 13px;
  color: var(--app-text-regular);
  font-weight: 500;
  padding-bottom: 6px;
}

.bk-auto-switch-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.bk-fields-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 20px;
}

.bk-checkboxes {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 16px;
}

.bk-select {
  width: 220px;
}

.bk-action-item :deep(.el-form-item__content) {
  justify-content: flex-start;
}

.bk-readonly-value {
  display: flex;
  align-items: center;
  min-height: 32px;
  padding: 4px 12px;
  background: #FAFBFC;
  border: 1px solid var(--app-border);
  border-radius: 6px;
  font-size: 13px;
  color: var(--app-title);
  word-break: break-all;
}

.bk-assist-text {
  font-size: 12px;
  color: var(--app-disabled);
  line-height: 1.4;
}

/* ═══════════════ Upload (restore tab) ═══════════════ */
.bk-upload {
  display: block;
  margin-bottom: 18px;
}

.bk-upload :deep(.el-upload-dragger) {
  border-radius: 10px;
  border: 1.5px dashed var(--app-border);
  background: #FAFBFC;
  min-height: 150px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  transition: all 0.2s;
}

.bk-upload :deep(.el-upload-dragger:hover) {
  border-color: var(--app-accent);
  background: var(--app-accent-soft);
}

.bk-upload-icon {
  font-size: 40px;
  color: var(--app-accent);
}

.bk-upload-text {
  font-size: 14px;
  font-weight: 500;
  color: var(--app-title);
}

.bk-upload-tip {
  font-size: 12px;
  color: var(--app-text-regular);
}

.bk-restore-btn {
  background: var(--app-warning);
  border-color: var(--app-warning);
}

.bk-restore-btn:hover:not(.is-disabled),
.bk-restore-btn:focus-visible:not(.is-disabled) {
  background: #FF9A2E;
  border-color: #FF9A2E;
}

/* ═══════════════ Recent backup list ═══════════════ */
.bk-recent-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.bk-recent-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid var(--app-border);
  border-radius: 8px;
  background: var(--app-bg-secondary);
  transition: all 0.2s;
}

.bk-recent-item:hover {
  border-color: var(--app-accent-muted);
  background: #FBFDFF;
  box-shadow: 0 2px 8px rgba(22, 93, 255, 0.06);
}

.bk-recent-main {
  min-width: 0;
  flex: 1;
}

.bk-recent-id {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-title);
  font-variant-numeric: tabular-nums;
}

.bk-recent-scope {
  font-size: 12px;
  color: var(--app-text-regular);
  margin-top: 3px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bk-recent-meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
  align-items: flex-end;
  flex-shrink: 0;
  font-variant-numeric: tabular-nums;
}

.bk-recent-time {
  font-size: 12px;
  color: var(--app-text-regular);
}

.bk-recent-size {
  font-size: 12px;
  font-weight: 600;
  color: var(--app-title);
}

/* ═══════════════ Format tag ═══════════════ */
.bk-format-tag {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.3px;
  border: 1px solid transparent;
}

.bk-format-tag.is-json {
  background: var(--app-accent-soft);
  color: var(--app-accent);
  border-color: rgba(22, 93, 255, 0.2);
}

.bk-format-tag.is-excel {
  background: rgba(0, 180, 42, 0.1);
  color: var(--app-success);
  border-color: rgba(0, 180, 42, 0.25);
}

/* ═══════════════ Toolbar (history tab) ═══════════════ */
.bk-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.bk-filter-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  flex-wrap: wrap;
}

.bk-search {
  width: 280px;
  min-width: 200px;
}

.bk-filter {
  width: 140px;
}

/* ═══════════════ History table ═══════════════ */
.bk-table :deep(.el-table__header-wrapper th) {
  background: var(--app-table-header) !important;
  color: var(--app-text-regular);
  font-weight: 600;
  font-size: 13px;
}

.bk-table :deep(.el-table__row) {
  transition: background 0.2s;
}

.bk-table :deep(.el-table__row:hover > td) {
  background: var(--app-table-hover) !important;
}

.bk-backup-id {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-title);
  font-variant-numeric: tabular-nums;
}

.bk-scope-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.bk-scope-chip {
  display: inline-block;
  padding: 2px 8px;
  font-size: 12px;
  border-radius: 4px;
  background: var(--app-bg);
  color: var(--app-text-regular);
  border: 1px solid var(--app-border);
}

.bk-datetime,
.bk-size {
  font-size: 13px;
  color: var(--app-text-regular);
  font-variant-numeric: tabular-nums;
}

.bk-size {
  font-weight: 600;
  color: var(--app-title);
}

.bk-actions {
  display: flex;
  align-items: center;
  gap: 2px;
}

.bk-actions :deep(.el-button.is-link) {
  padding: 0 6px;
  height: auto;
}

.bk-actions :deep(.el-divider--vertical) {
  margin: 0 2px;
  border-color: var(--app-border);
}

.bk-empty {
  padding: 40px 0;
}

.bk-pagination {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 1280px) {
  .bk-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .bk-two-col {
    grid-template-columns: 1fr;
  }

  .bk-search {
    width: 100%;
  }
}

@media (max-width: 768px) {
  .bk-stats {
    grid-template-columns: 1fr;
  }

  .bk-fields-row {
    grid-template-columns: 1fr;
  }

  .bk-filter-row {
    width: 100%;
  }

  .bk-filter {
    flex: 1;
    min-width: 120px;
  }
}
</style>