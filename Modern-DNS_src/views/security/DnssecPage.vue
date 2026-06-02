<script setup lang="ts">
import { useDnssec } from '../../composables/setting/useDnssec'

const {
  loading,
  submitting,
  checking,
  filters,
  pager,
  detailVisible,
  detail,
  rowActionId,
  bulkChecking,
  sortedRows,
  pagedRows,
  statsCards,
  formatDateTime,
  keySummary,
  totalKeys,
  rowClassName,
  dnssecStatusLabel,
  signatureStatusLabel,
  resetFilters,
  refreshData,
  handleSortChange,
  openDetail,
  closeDetail,
  runCheckAll,
  runCheckOne,
  toggleEnabled,
  generateKey,
  deleteKey,
  copyKeys,
} = useDnssec()
</script>

<template>
  <div class="dnssec-page stack-card">
    <el-card class="hero-card" shadow="never">
      <div class="hero-header">
        <div>
          <div class="hero-title">{{ $t('security.dnssecManage') }}</div>
          <div class="hero-subtitle">{{ $t('security.dnssecManageSubtitle') }}</div>
        </div>
        <div class="hero-actions">
          <el-button :loading="loading" @click="refreshData()">{{ $t('security.refreshData') }}</el-button>
          <el-button type="primary" :loading="bulkChecking" :disabled="checking || rowActionId !== null" @click="runCheckAll">
            {{ $t('security.checkAll') }}
          </el-button>
        </div>
      </div>

      <div class="stats-grid">
        <div v-for="item in statsCards" :key="item.key" class="stat-card" :class="`stat-card-${item.key}`">
          <div class="stat-label">{{ item.label }}</div>
          <div class="stat-value">{{ item.value }}</div>
        </div>
      </div>
    </el-card>

    <el-card class="table-card" shadow="never">
      <div class="toolbar">
        <div class="toolbar-filters">
          <el-input v-model="filters.keyword" clearable :placeholder="$t('common.searchDomain')" class="toolbar-input" />
          <el-select v-model="filters.dnssecStatus" clearable :placeholder="$t('security.dnssecStatus')" class="toolbar-select">
            <el-option :label="$t('security.opened')" value="已开启" />
            <el-option :label="$t('security.notOpened')" value="未开启" />
          </el-select>
          <el-select v-model="filters.signatureStatus" clearable :placeholder="$t('security.signatureStatus')" class="toolbar-select">
            <el-option :label="$t('security.valid')" value="有效" />
            <el-option :label="$t('security.invalid')" value="异常" />
            <el-option :label="$t('security.notChecked')" value="未检查" />
          </el-select>
        </div>
        <div class="toolbar-actions">
          <el-button @click="resetFilters">{{ $t('common.resetFilter') }}</el-button>
        </div>
      </div>

      <el-table
        v-if="sortedRows.length"
        :data="pagedRows"
        stripe
        v-loading="loading"
        :row-class-name="rowClassName"
        @sort-change="handleSortChange"
      >
        <el-table-column prop="domain" :label="$t('cache.domain')" min-width="220" sortable="custom" />
        <el-table-column prop="dnssecStatus" :label="$t('security.dnssecStatus')" width="180" sortable="custom">
          <template #default="{ row }">
            <div class="status-cell">
              <el-switch
                :model-value="row.dnssecStatus === '已开启'"
                :loading="rowActionId === row.id"
                :disabled="rowActionId !== null || checking"
                inline-prompt
                @change="(value) => toggleEnabled(row, Boolean(value))"
              />
              <el-tag :type="row.dnssecStatus === '已开启' ? 'success' : 'info'" round>
                {{ dnssecStatusLabel(row.dnssecStatus) }}
              </el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="signatureStatus" :label="$t('security.signatureStatus')" width="140" sortable="custom">
          <template #default="{ row }">
            <el-tag :type="row.signatureStatus === '有效' ? 'success' : row.signatureStatus === '异常' ? 'danger' : 'info'" round>
              {{ signatureStatusLabel(row.signatureStatus) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="$t('security.keySummary')" min-width="180">
          <template #default="{ row }">
            <div class="key-summary">
              <span>{{ keySummary(row) }}</span>
              <el-tag size="small" effect="plain">{{ $t('security.totalKeys', { n: totalKeys(row) }) }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="lastCheckAt" :label="$t('security.lastCheckAt')" min-width="180" sortable="custom">
          <template #default="{ row }">{{ formatDateTime(row.lastCheckAt) }}</template>
        </el-table-column>
        <el-table-column :label="$t('common.operation')" min-width="320" fixed="right">
          <template #default="{ row }">
            <div class="table-operations">
              <el-button link type="primary" @click="openDetail(row)">{{ $t('cache.viewDetail') }}</el-button>
              <el-button
                link
                type="warning"
                :loading="rowActionId === row.id && checking"
                :disabled="rowActionId !== null || checking || row.dnssecStatus !== '已开启'"
                @click="runCheckOne(row)"
              >
                {{ $t('security.recheck') }}
              </el-button>
              <el-button link type="success" :disabled="rowActionId !== null || submitting" @click="generateKey(row, 'ksk')">{{ $t('security.generateKsk') }}</el-button>
              <el-button link type="success" plain :disabled="rowActionId !== null || submitting" @click="generateKey(row, 'zsk')">{{ $t('security.generateZsk') }}</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-else :description="$t('security.noDnssecData')" />

      <div v-if="sortedRows.length" class="pagination-wrap">
        <el-pagination
          v-model:current-page="pager.page"
          v-model:page-size="pager.size"
          layout="total, sizes, prev, pager, next"
          background
          :page-sizes="[10, 20, 50]"
          :total="sortedRows.length"
        />
      </div>
    </el-card>

    <el-dialog v-model="detailVisible" :title="$t('security.detailTitle')" width="920px" destroy-on-close @closed="closeDetail">
      <template v-if="detail">
        <div class="detail-overview">
          <div class="detail-item">
            <span class="detail-label">{{ $t('cache.domain') }}</span>
            <span class="detail-value">{{ detail.domain }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-label">{{ $t('security.dnssecStatus') }}</span>
            <el-tag :type="detail.dnssecStatus === '已开启' ? 'success' : 'info'" round>{{ dnssecStatusLabel(detail.dnssecStatus) }}</el-tag>
          </div>
          <div class="detail-item">
            <span class="detail-label">{{ $t('security.signatureStatus') }}</span>
            <el-tag :type="detail.signatureStatus === '有效' ? 'success' : detail.signatureStatus === '异常' ? 'danger' : 'info'" round>
              {{ signatureStatusLabel(detail.signatureStatus) }}
            </el-tag>
          </div>
          <div class="detail-item">
            <span class="detail-label">{{ $t('security.lastCheck') }}</span>
            <span class="detail-value">{{ formatDateTime(detail.lastCheckAt) }}</span>
          </div>
        </div>

        <div class="detail-actions">
          <el-button :disabled="rowActionId !== null || submitting" @click="copyKeys(detail)">{{ $t('security.copyKeyInfo') }}</el-button>
          <el-button type="success" :disabled="rowActionId !== null || submitting" @click="generateKey(detail, 'ksk')">{{ $t('security.generateKsk') }}</el-button>
          <el-button type="success" plain :disabled="rowActionId !== null || submitting" @click="generateKey(detail, 'zsk')">{{ $t('security.generateZsk') }}</el-button>
        </div>

        <div class="detail-grid">
          <el-card shadow="never" class="detail-card">
            <template #header>
              <div class="detail-card-header">
                <span>{{ $t('security.keyList', { type: 'KSK' }) }}</span>
                <el-tag size="small" effect="plain">{{ $t('security.keyCount', { n: detail.ksk.length }) }}</el-tag>
              </div>
            </template>

            <el-empty v-if="!detail.ksk.length" :description="$t('security.noKeyData', { type: 'KSK' })" />
            <div v-else class="key-list">
              <div v-for="item in detail.ksk" :key="item.keyId" class="key-item">
                <div>
                  <div class="key-id">{{ item.keyId }}</div>
                  <div class="key-meta">{{ item.status }} · {{ formatDateTime(item.createdAt) }}</div>
                </div>
                <el-button link type="danger" :disabled="rowActionId !== null || submitting" @click="deleteKey(detail, 'ksk', item)">{{ $t('common.delete') }}</el-button>
              </div>
            </div>
          </el-card>

          <el-card shadow="never" class="detail-card">
            <template #header>
              <div class="detail-card-header">
                <span>{{ $t('security.keyList', { type: 'ZSK' }) }}</span>
                <el-tag size="small" effect="plain">{{ $t('security.keyCount', { n: detail.zsk.length }) }}</el-tag>
              </div>
            </template>

            <el-empty v-if="!detail.zsk.length" :description="$t('security.noKeyData', { type: 'ZSK' })" />
            <div v-else class="key-list">
              <div v-for="item in detail.zsk" :key="item.keyId" class="key-item">
                <div>
                  <div class="key-id">{{ item.keyId }}</div>
                  <div class="key-meta">{{ item.status }} · {{ formatDateTime(item.createdAt) }}</div>
                </div>
                <el-button link type="danger" :disabled="rowActionId !== null || submitting" @click="deleteKey(detail, 'zsk', item)">{{ $t('common.delete') }}</el-button>
              </div>
            </div>
          </el-card>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.dnssec-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.hero-card,
.table-card,
.detail-card {
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 20px;
}

.hero-header,
.toolbar,
.detail-card-header,
.detail-actions,
.status-cell,
.key-summary,
.table-operations,
.key-item {
  display: flex;
  align-items: center;
}

.hero-header,
.toolbar,
.detail-actions {
  justify-content: space-between;
  gap: 16px;
}

.hero-title {
  font-size: 24px;
  font-weight: 700;
  color: #102a43;
}

.hero-subtitle {
  margin-top: 6px;
  color: #52606d;
}

.hero-actions,
.toolbar-filters,
.toolbar-actions,
.detail-grid {
  display: flex;
  gap: 12px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
  margin-top: 20px;
}

.stat-card {
  padding: 18px;
  border-radius: 18px;
  color: #102a43;
}

.stat-card-opened {
  background: linear-gradient(135deg, #e6fffa 0%, #f0fff4 100%);
}

.stat-card-valid {
  background: linear-gradient(135deg, #eff6ff 0%, #eef2ff 100%);
}

.stat-card-abnormal {
  background: linear-gradient(135deg, #fff1f2 0%, #fff7ed 100%);
}

.stat-card-unopened {
  background: linear-gradient(135deg, #f8fafc 0%, #f1f5f9 100%);
}

.stat-label {
  font-size: 13px;
  color: #52606d;
}

.stat-value {
  margin-top: 10px;
  font-size: 28px;
  font-weight: 700;
}

.toolbar {
  flex-wrap: wrap;
  margin-bottom: 16px;
}

.toolbar-filters {
  flex: 1;
  flex-wrap: wrap;
}

.toolbar-input {
  width: 260px;
}

.toolbar-select {
  width: 160px;
}

.status-cell,
.key-summary,
.table-operations,
.detail-card-header,
.key-item {
  gap: 10px;
}

.table-operations {
  flex-wrap: wrap;
}

.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  margin-top: 18px;
}

.detail-overview {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.detail-item {
  padding: 14px 16px;
  border-radius: 14px;
  background: #f8fafc;
}

.detail-label {
  display: block;
  margin-bottom: 8px;
  font-size: 12px;
  color: #7b8794;
}

.detail-value,
.key-id {
  font-weight: 600;
  color: #102a43;
}

.detail-actions {
  margin: 18px 0;
  justify-content: flex-start;
  flex-wrap: wrap;
}

.detail-grid {
  align-items: stretch;
}

.detail-card {
  flex: 1;
}

.key-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.key-item {
  justify-content: space-between;
  padding: 12px 14px;
  border-radius: 14px;
  background: #f8fafc;
}

.key-meta {
  margin-top: 4px;
  font-size: 12px;
  color: #7b8794;
}

:deep(.dnssec-row-abnormal) {
  --el-table-tr-bg-color: #fff1f2;
}

@media (max-width: 960px) {
  .stats-grid,
  .detail-overview {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .detail-grid {
    flex-direction: column;
  }
}

@media (max-width: 640px) {
  .hero-header,
  .toolbar {
    flex-direction: column;
    align-items: stretch;
  }

  .hero-actions,
  .toolbar-filters {
    flex-direction: column;
  }

  .toolbar-input,
  .toolbar-select {
    width: 100%;
  }

  .stats-grid,
  .detail-overview {
    grid-template-columns: 1fr;
  }
}
</style>
