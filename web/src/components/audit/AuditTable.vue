<template>
  <div class="table-wrapper">
    <div class="bulk-row">
      <span>{{ t('system.audit.selectedN', { n: selectedRows.length }) }}</span>
      <el-button plain size="small" :disabled="!selectedRows.length || exporting" @click="$emit('exportSelected')">
        {{ t('system.audit.exportSelected') }}
      </el-button>
      <el-button plain size="small" :disabled="!selectedRows.length" @click="$emit('clearSelection')">
        {{ t('system.audit.clearSelection') }}
      </el-button>
    </div>

    <el-table
      ref="tableRef"
      v-loading="loading"
      :data="rows"
      stripe
      highlight-current-row
      @selection-change="$emit('selectionChange', $event)"
      @row-click="(row: any) => $emit('rowClick', row)"
      @sort-change="$emit('sortChange', $event)"
      style="width: 100%"
    >
      <el-table-column type="selection" width="40" />
      <el-table-column :label="t('system.audit.colTime')" prop="createdAt" sortable="custom" width="170" />
      <el-table-column :label="t('system.audit.colUser')" prop="username" min-width="140" />
      <el-table-column :label="t('system.audit.colIp')" prop="ip" width="140" />
      <el-table-column :label="t('system.audit.colAction')" prop="operationType" min-width="180">
        <template #default="{ row }">
          <span>{{ operationLabel(row) }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('system.audit.colResult')" prop="result" width="90" />
      <el-table-column :label="t('system.audit.colRisk')" prop="riskLevel" width="80" />
      <el-table-column :label="t('system.audit.colResource')" prop="resourceName" min-width="160" />
      <el-table-column :label="t('system.audit.colDetail')" width="60">
        <template #default="{ row }">
          <el-button link type="primary" size="small" @click.stop="$emit('viewDetail', row)">
            {{ t('system.audit.viewDetail') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="page-row">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        background
        @current-change="$emit('pageChange', $event)"
        @size-change="$emit('sizeChange', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';

interface AuditRecord {
  id: string;
  createdAt: string;
  username: string;
  ip: string;
  operationType: string;
  result: string;
  riskLevel?: string;
  resourceName?: string;
  [key: string]: any;
}

const props = withDefaults(defineProps<{
  rows: AuditRecord[];
  selected: AuditRecord[];
  total: number;
  loading: boolean;
  exporting: boolean;
  page: number;
  pageSize: number;
  operationLabel: (row: AuditRecord) => string;
}>(), {
  selected: () => [],
  operationLabel: () => ''
});

const emit = defineEmits<{
  selectionChange: [rows: AuditRecord[]];
  rowClick: [row: AuditRecord];
  sortChange: [sort: any];
  viewDetail: [row: AuditRecord];
  exportSelected: [];
  clearSelection: [];
  pageChange: [page: number];
  sizeChange: [size: number];
}>();

const { t } = useI18n();
const tableRef = ref<any>();
</script>

<style scoped>
.table-wrapper {
  margin-top: 8px;
}

.bulk-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.page-row {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
