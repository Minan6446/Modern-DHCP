<template>
  <div class="page surface-card">
    <el-card shadow="never" class="module-card">
      <div class="stats-grid">
        <div class="stat-card">
          <div class="stat-title">{{ t('system.session.statOnline') }}</div>
          <div class="stat-value">{{ stats.onlineSessions }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-title">{{ t('system.session.statTodayLogins') }}</div>
          <div class="stat-value">{{ stats.todayLogins }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-title">{{ t('system.session.statAbnormal') }}</div>
          <div class="stat-value warning">{{ stats.abnormalSessions }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-title">{{ t('system.session.statActiveUsers') }}</div>
          <div class="stat-value">{{ stats.activeUsers }}</div>
        </div>
      </div>

      <div class="filter-bulk-row">
        <div class="left-filters">
          <el-input
            v-model="userKeyword"
            :placeholder="t('system.session.searchUser')"
            clearable
            class="w-200"
            @input="onFilterChange"
          />
          <el-input
            v-model="ipKeyword"
            :placeholder="t('system.session.searchIp')"
            clearable
            class="w-180"
            @input="onFilterChange"
          />
          <el-select v-model="tenantFilter" :placeholder="t('system.session.filterTenant')" clearable class="w-160" @change="onFilterChange">
            <el-option v-for="item in tenantOptions" :key="item" :label="item" :value="item" />
          </el-select>
          <el-select v-model="statusFilter" :placeholder="t('system.session.filterStatus')" clearable class="w-120" @change="onFilterChange">
            <el-option value="active" :label="t('system.session.statusActive')" />
            <el-option value="stale" :label="t('system.session.statusStale')" />
          </el-select>
          <el-date-picker
            v-model="timeRange"
            type="datetimerange"
            :start-placeholder="t('system.session.startTime')"
            :end-placeholder="t('system.session.endTime')"
            value-format="YYYY-MM-DDTHH:mm:ss[Z]"
            class="w-360"
            @change="onFilterChange"
          />
          <el-button :loading="loading" @click="fetch">{{ t('system.session.refresh') }}</el-button>
        </div>

        <div class="right-bulk">
          <el-button @click="selectAllCurrentPage">{{ t('system.session.selectAll') }}</el-button>
          <el-button @click="inverseSelectionCurrentPage">{{ t('system.session.invertSelection') }}</el-button>
          <el-button
            type="danger"
            plain
            :disabled="!selectedRows.length"
            :loading="actionLoading"
            @click="batchKick"
            >{{ t('system.session.batchKick') }}</el-button
          >
        </div>
      </div>

      <el-table
        ref="tableRef"
        v-loading="loading"
        :data="pagedRows"
        border
        stripe
        class="data-table"
        @selection-change="onSelectionChange"
      >
        <el-table-column type="selection" width="52" :selectable="isRowSelectable" />
        <el-table-column prop="username" :label="t('system.session.colUser')" min-width="130" />
        <el-table-column prop="tenantId" :label="t('system.session.colTenant')" min-width="120" />
        <el-table-column prop="ip" :label="t('system.session.colIp')" min-width="140" />
        <el-table-column prop="userAgent" :label="t('system.session.colAgent')" min-width="220" show-overflow-tooltip />
        <el-table-column :label="t('system.session.colDuration')" width="130">
          <template #default="{ row }">{{ sessionDuration(row) }}</template>
        </el-table-column>
        <el-table-column :label="t('system.session.colStatus')" width="170">
          <template #default="{ row }">
            <el-space>
              <el-tag :type="statusTagType(row.status)">{{ statusLabel(row.status) }}</el-tag>
              <el-tag v-if="row.current" type="success">{{ t('system.session.currentSession') }}</el-tag>
            </el-space>
          </template>
        </el-table-column>
        <el-table-column :label="t('system.session.colCreated')" min-width="165">
          <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column :label="t('system.session.colLastSeen')" min-width="165">
          <template #default="{ row }">{{ formatTime(row.lastSeenAt) }}</template>
        </el-table-column>
        <el-table-column :label="t('system.session.colAction')" width="220" fixed="right" class-name="action-column">
          <template #default="{ row }">
            <div class="action-group">
              <el-button type="primary" plain size="small" @click="openDetail(row)">{{ t('system.session.detail') }}</el-button>
              <el-popconfirm
                v-if="!row.current"
                :title="t('system.session.kickConfirmMsg')"
                @confirm="kick(row)"
              >
                <template #reference>
                  <el-button type="danger" plain size="small" :loading="actionLoading">{{ t('system.session.kickBtn') }}</el-button>
                </template>
              </el-popconfirm>
              <el-tooltip v-else :content="t('system.session.currentTooltip')" placement="top">
                <el-button type="warning" plain size="small" disabled>{{ t('system.session.riskHint') }}</el-button>
              </el-tooltip>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="!loading && !pagedRows.length" :description="t('system.session.emptyDesc')">
        <el-button @click="fetch">{{ t('system.session.refreshSessions') }}</el-button>
      </el-empty>

      <div class="pager-row">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="filteredRows.length"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="onPageChange"
          @size-change="onSizeChange"
        />
      </div>
    </el-card>

    <el-drawer v-model="detailDrawer.visible" :title="t('system.session.drawerTitle')" size="520px">
      <div v-if="detailDrawer.row" class="detail-wrap">
        <el-descriptions :column="1" border>
          <el-descriptions-item :label="t('system.session.descId')">{{ detailDrawer.row.id }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.session.descUser')">{{ detailDrawer.row.username }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.session.descTenant')">{{ detailDrawer.row.tenantId || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.session.descUserId')">{{ detailDrawer.row.userId || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.session.descIp')">{{ detailDrawer.row.ip || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.session.descAgent')">{{ detailDrawer.row.userAgent || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.session.descStatus')">
            <el-space>
              <el-tag :type="statusTagType(detailDrawer.row.status)">{{ statusLabel(detailDrawer.row.status) }}</el-tag>
              <el-tag v-if="detailDrawer.row.current" type="success">{{ t('system.session.currentSession') }}</el-tag>
            </el-space>
          </el-descriptions-item>
          <el-descriptions-item :label="t('system.session.descDuration')">{{ sessionDuration(detailDrawer.row) }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.session.descCreated')">{{ formatTime(detailDrawer.row.createdAt) }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.session.descLastSeen')">{{ formatTime(detailDrawer.row.lastSeenAt) }}</el-descriptions-item>
        </el-descriptions>

        <div class="record-head">{{ t('system.session.activityTitle') }}</div>
        <el-timeline>
          <el-timeline-item :timestamp="formatTime(detailDrawer.row.createdAt)" placement="top">
            {{ t('system.session.activityCreated') }}
          </el-timeline-item>
          <el-timeline-item :timestamp="formatTime(detailDrawer.row.lastSeenAt)" placement="top">
            {{ t('system.session.activityLastHeart') }}
          </el-timeline-item>
          <el-timeline-item v-if="detailDrawer.row.current" :timestamp="t('system.session.activityCurrent')" placement="top">
            {{ t('system.session.activityCurrentProtected') }}
          </el-timeline-item>
        </el-timeline>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useDebounceFn } from '@vueuse/core';
import { ElMessageBox } from 'element-plus';
import { listSessions, forceLogout } from '@/api/sessions';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { useSystemViewStore } from '@/store/systemView';
import type { SessionInfo } from '@/types/system';
import { useI18n } from 'vue-i18n';
import { showAuthError } from '@/shared/errors/errorToast';
import { showSuccess, showWarning } from '@/shared/errors/messageToast';

const { t } = useI18n();
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const systemView = useSystemViewStore();

const tableRef = ref<any>();
const loading = ref(false);
const actionLoading = ref(false);
const allRows = ref<SessionInfo[]>([]);
const selectedRows = ref<SessionInfo[]>([]);

const userKeyword = ref(systemView.session.keyword || '');
const ipKeyword = ref(systemView.session.ip || '');
const statusFilter = ref(systemView.session.status || '');
const tenantFilter = ref(systemView.session.tenant || '');
const timeRange = ref<[string, string] | null>(
  systemView.session.startAt && systemView.session.endAt
    ? [systemView.session.startAt, systemView.session.endAt]
    : null
);

const page = ref(systemView.session.page || 1);
const pageSize = ref(systemView.session.pageSize || 10);
const durationNow = ref(Date.now());
let durationTimer: ReturnType<typeof setInterval> | null = null;

const detailDrawer = reactive<{ visible: boolean; row: SessionInfo | null }>({
  visible: false,
  row: null
});

const canReadSessions = computed(() => permissionStore.can('__role_admin__'));

const tenantOptions = computed(() => {
  const set = new Set<string>();
  allRows.value.forEach((item) => {
    const tenant = String(item.tenantId || '').trim();
    if (tenant) set.add(tenant);
  });
  return Array.from(set.values());
});

const filteredRows = computed(() => {
  let list = [...allRows.value];

  const userQ = userKeyword.value.trim().toLowerCase();
  if (userQ) {
    list = list.filter((item) => String(item.username || '').toLowerCase().includes(userQ));
  }

  const ipQ = ipKeyword.value.trim().toLowerCase();
  if (ipQ) {
    list = list.filter((item) => String(item.ip || '').toLowerCase().includes(ipQ));
  }

  if (tenantFilter.value) {
    list = list.filter((item) => String(item.tenantId || '') === tenantFilter.value);
  }

  if (statusFilter.value) {
    list = list.filter((item) => normalizedStatus(item.status) === statusFilter.value);
  }

  if (timeRange.value && timeRange.value.length === 2) {
    const start = Date.parse(timeRange.value[0]);
    const end = Date.parse(timeRange.value[1]);
    if (!Number.isNaN(start) && !Number.isNaN(end)) {
      list = list.filter((item) => {
        const ts = Date.parse(item.lastSeenAt || item.createdAt || '');
        if (Number.isNaN(ts)) return false;
        return ts >= start && ts <= end;
      });
    }
  }

  return list;
});

const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return filteredRows.value.slice(start, start + pageSize.value);
});

const stats = computed(() => {
  const rows = allRows.value;
  const now = new Date();
  const today = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;

  const onlineSessions = rows.filter((item) => normalizedStatus(item.status) === 'active').length;
  const todayLogins = rows.filter((item) => formatDatePart(item.createdAt) === today).length;
  const abnormalSessions = rows.filter((item) => normalizedStatus(item.status) === 'stale').length;
  const activeUsers = new Set(
    rows
      .filter((item) => normalizedStatus(item.status) === 'active')
      .map((item) => String(item.username || '').trim())
      .filter(Boolean)
  ).size;

  return { onlineSessions, todayLogins, abnormalSessions, activeUsers };
});

const fetch = async () => {
  if (!canReadSessions.value) return;
  loading.value = true;
  try {
    const startAt = timeRange.value?.[0] || '';
    const endAt = timeRange.value?.[1] || '';
    systemView.setSession({
      page: page.value,
      pageSize: pageSize.value,
      keyword: userKeyword.value,
      status: statusFilter.value,
      ip: ipKeyword.value,
      tenant: tenantFilter.value,
      startAt,
      endAt
    });

    const mergedKeyword = [userKeyword.value.trim(), ipKeyword.value.trim()].filter(Boolean).join(' ');
    const first = await listSessions({
      page: 1,
      pageSize: 200,
      keyword: mergedKeyword,
      status: statusFilter.value
    });
    const firstItems = first.data.data.items || [];
    const total = Number(first.data.data.total || firstItems.length);

    const all = [...firstItems];
    if (total > firstItems.length) {
      const pages = Math.ceil(total / 200);
      for (let index = 2; index <= pages; index += 1) {
        const next = await listSessions({
          page: index,
          pageSize: 200,
          keyword: mergedKeyword,
          status: statusFilter.value
        });
        all.push(...(next.data.data.items || []));
      }
    }

    allRows.value = all;
    if ((page.value - 1) * pageSize.value >= filteredRows.value.length) {
      page.value = 1;
    }
  } catch (e) {
    showAuthError(e, t('system.session.loadFail'));
  } finally {
    loading.value = false;
  }
};

const onFilterChange = useDebounceFn(() => {
  page.value = 1;
  selectedRows.value = [];
  const startAt = timeRange.value?.[0] || '';
  const endAt = timeRange.value?.[1] || '';
  systemView.setSession({
    page: 1,
    pageSize: pageSize.value,
    keyword: userKeyword.value,
    status: statusFilter.value,
    ip: ipKeyword.value,
    tenant: tenantFilter.value,
    startAt,
    endAt
  });
}, 250);

const onPageChange = (value: number) => {
  page.value = value;
  const startAt = timeRange.value?.[0] || '';
  const endAt = timeRange.value?.[1] || '';
  systemView.setSession({
    page: value,
    pageSize: pageSize.value,
    keyword: userKeyword.value,
    status: statusFilter.value,
    ip: ipKeyword.value,
    tenant: tenantFilter.value,
    startAt,
    endAt
  });
};

const onSizeChange = (value: number) => {
  pageSize.value = value;
  page.value = 1;
  const startAt = timeRange.value?.[0] || '';
  const endAt = timeRange.value?.[1] || '';
  systemView.setSession({
    page: 1,
    pageSize: value,
    keyword: userKeyword.value,
    status: statusFilter.value,
    ip: ipKeyword.value,
    tenant: tenantFilter.value,
    startAt,
    endAt
  });
};

const normalizedStatus = (status?: string) => (status === 'stale' ? 'stale' : 'active');

const statusLabel = (status?: string) => (normalizedStatus(status) === 'stale' ? t('system.session.statusStale') : t('system.session.statusActive'));

const statusTagType = (status?: string) => (normalizedStatus(status) === 'stale' ? 'warning' : 'success');

const formatDatePart = (value?: string) => {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
};

const formatTime = (value?: string) => {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`;
};

const sessionDuration = (row: SessionInfo) => {
  const start = Date.parse(row.createdAt || '');
  if (Number.isNaN(start)) return '-';
  const end = row.current ? durationNow.value : Date.parse(row.lastSeenAt || '');
  const baseEnd = Number.isNaN(end) ? durationNow.value : Math.max(end, start);
  const diff = Math.max(0, baseEnd - start);
  const day = Math.floor(diff / (24 * 3600 * 1000));
  const hour = Math.floor((diff % (24 * 3600 * 1000)) / (3600 * 1000));
  const minute = Math.floor((diff % (3600 * 1000)) / (60 * 1000));
  if (day > 0) return t('system.session.durationDay', { d: day, h: hour });
  if (hour > 0) return t('system.session.durationHour', { h: hour, m: minute });
  return t('system.session.durationMin', { m: minute });
};

const isRowSelectable = (row: SessionInfo) => !row.current;

const onSelectionChange = (rows: SessionInfo[]) => {
  selectedRows.value = rows.filter((item) => !item.current);
};

const selectAllCurrentPage = () => {
  if (!tableRef.value) return;
  tableRef.value.clearSelection();
  pagedRows.value.forEach((row) => {
    if (!row.current) {
      tableRef.value.toggleRowSelection(row, true);
    }
  });
};

const inverseSelectionCurrentPage = () => {
  if (!tableRef.value) return;
  const selectedSet = new Set(selectedRows.value.map((item) => item.id));
  tableRef.value.clearSelection();
  pagedRows.value.forEach((row) => {
    if (row.current) return;
    tableRef.value.toggleRowSelection(row, !selectedSet.has(row.id));
  });
};

const kick = async (row: SessionInfo) => {
  if (row.current) {
    showWarning(t('system.session.kickForbid'));
    return;
  }
  actionLoading.value = true;
  try {
    await forceLogout(row.id);
    showSuccess(t('system.session.kickSuccess'));
    await fetch();
  } catch (e) {
    showAuthError(e, t('system.session.kickFail'));
  } finally {
    actionLoading.value = false;
  }
};

const batchKick = async () => {
  if (!selectedRows.value.length) return;
  try {
    await ElMessageBox.confirm(t('system.session.batchConfirm', { count: selectedRows.value.length }), t('system.session.batchConfirmTitle'), {
      type: 'warning'
    });
  } catch {
    return;
  }

  actionLoading.value = true;
  try {
    for (const row of selectedRows.value) {
      await forceLogout(row.id);
    }
    showSuccess(t('system.session.batchSuccess'));
    selectedRows.value = [];
    await fetch();
  } catch (e) {
    showAuthError(e, t('system.session.batchFail'));
  } finally {
    actionLoading.value = false;
  }
};

const openDetail = (row: SessionInfo) => {
  detailDrawer.row = row;
  detailDrawer.visible = true;
};

onMounted(() => {
  fetch();
  durationTimer = setInterval(() => {
    durationNow.value = Date.now();
  }, 1000);
});

onUnmounted(() => {
  if (durationTimer) {
    clearInterval(durationTimer);
    durationTimer = null;
  }
});

watch(
  () => tenantStore.currentTenantId,
  () => {
    page.value = 1;
    tenantFilter.value = '';
    fetch();
  }
);
</script>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
  box-sizing: border-box;
}

.module-card {
  border-radius: 10px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 14px;
}

.stat-card {
  border: 1px solid var(--el-border-color-light);
  border-radius: 8px;
  padding: 12px;
  background: var(--el-fill-color-extra-light);
  min-height: 84px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
}

.stat-title {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.stat-value {
  font-size: 24px;
  font-weight: 600;
  line-height: 1;
}

.stat-value.warning {
  color: var(--el-color-warning);
}

.filter-bulk-row {
  margin-bottom: 12px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.left-filters,
.right-bulk {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.data-table :deep(.el-table__cell .cell) {
  white-space: nowrap;
}

.data-table :deep(.action-column .cell) {
  white-space: normal;
}

.action-group {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: nowrap;
}

.pager-row {
  margin-top: 12px;
  display: flex;
  justify-content: flex-end;
}

.detail-wrap {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.record-head {
  font-weight: 600;
}

.w-120 {
  width: 120px;
}

.w-160 {
  width: 160px;
}

.w-180 {
  width: 180px;
}

.w-200 {
  width: 200px;
}

.w-360 {
  width: 360px;
}
</style>
