<template>
  <div class="events-panel">
    <div class="filters">
      <el-form :model="filters" inline size="small" @submit.prevent>
        <el-form-item :label="t('monitoring.status')">
          <el-select v-model="filters.status" clearable style="width: 140px" @change="refreshNow">
            <el-option label="firing" value="firing" />
            <el-option label="resolved" value="resolved" />
            <el-option label="ack" value="ack" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('monitoring.severity')">
          <el-select v-model="filters.severity" clearable style="width: 140px" @change="refreshNow">
            <el-option label="info" value="info" />
            <el-option label="warning" value="warning" />
            <el-option label="critical" value="critical" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button class="sky-btn" @click="refreshNow">{{ t('monitoring.apply') }}</el-button>
          <el-button @click="resetFilters">{{ t('monitoring.reset') }}</el-button>
        </el-form-item>
      </el-form>
    </div>
    <el-table
      v-loading="loading"
      :data="rows"
      size="small"
      border
      height="220"
      :empty-text="emptyText"
    >
      <el-table-column prop="severity" :label="t('monitoring.severity')" width="90" />
      <el-table-column prop="message" :label="t('common.name')" />
      <el-table-column prop="status" :label="t('monitoring.status')" width="100" />
    </el-table>
    <div class="pager">
      <el-pagination
        background
        layout="prev, pager, next, sizes, total"
        :total="pagination.total"
        :current-page="pagination.page"
        :page-size="pagination.pageSize"
        :page-sizes="[10, 20, 50]"
        @current-change="setPage"
        @size-change="setPageSize"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue';
import { useTableFetch } from '@/shared/composables/useTableFetch';
import { listAlertEvents } from '@/api/monitoring';
import type { AlertEvent } from '@/types/monitoring';
import { useI18n } from 'vue-i18n';

const props = defineProps<{ tenantId?: string | null }>();

const { t } = useI18n();

const { rows, filters, loading, pagination, refresh, refreshNow, setPage, setPageSize } =
  useTableFetch<AlertEvent, { status: string; severity: string }>(
    async ({ page, pageSize, filters, signal }) => {
      const { data } = await listAlertEvents(
        {
          tenantId: props.tenantId || undefined,
          page,
          pageSize,
          status: filters.status,
          severity: filters.severity
        },
        signal
      );
      const payload = data.data;
      if (Array.isArray(payload)) return { items: payload, total: payload.length };
      return { items: payload.items, total: payload.total };
    },
    {
      filters: { status: '', severity: '' },
      initialPage: 1,
      initialPageSize: 10,
      delay: 150
    }
  );

const emptyText = computed(() => (loading.value ? t('monitoring.loading') : t('monitoring.empty')));

const resetFilters = () => {
  filters.status = '';
  filters.severity = '';
  refreshNow();
};

watch(
  () => props.tenantId,
  () => {
    setPage(1);
    refreshNow();
  }
);
</script>

<style scoped>
.events-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.filters {
  display: flex;
  justify-content: space-between;
}

.sky-btn {
  background-color: #5ac8fa;
  border-color: #5ac8fa;
  color: #fff;
}

.sky-btn:not(.is-disabled):hover,
.sky-btn:not(.is-disabled):focus {
  background-color: #48b4ef;
  border-color: #48b4ef;
  color: #fff;
}

.pager {
  display: flex;
  justify-content: flex-end;
}
</style>
