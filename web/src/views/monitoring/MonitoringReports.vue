<template>
  <div class="monitor-reports page-block">
    <section class="hero-card reports-hero">
      <div>
        <p class="eyebrow">{{ t('monitoring.reports.heroEyebrow') }}</p>
        <h2>{{ t('monitoring.reports.heroTitle') }}</h2>
        <p class="desc">{{ t('monitoring.reports.heroDesc') }}</p>
      </div>
      <div class="hero-stats">
        <div v-for="stat in heroStats" :key="stat.key" class="stat-card">
          <span class="label">{{ stat.label }}</span>
          <span class="value">{{ stat.value }}</span>
          <span class="hint">{{ stat.hint }}</span>
        </div>
      </div>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <div>
              <p class="card-eyebrow">{{ t('monitoring.reports.requestTitle') }}</p>
              <h3>{{ t('monitoring.reports.requestSubtitle') }}</h3>
            </div>
          </div>
        </template>
        <el-form ref="reportFormRef" :model="reportForm" label-width="140px">
          <el-form-item prop="type" :label="t('monitoring.reports.typeLabel')">
            <el-select v-model="reportForm.type">
              <el-option
                v-for="opt in reportTypes"
                :key="opt"
                :label="reportTypeLabel(opt)"
                :value="opt"
              />
            </el-select>
          </el-form-item>
          <el-form-item prop="range" :label="t('monitoring.reports.rangeLabel')">
            <el-date-picker
              v-model="reportForm.range"
              type="datetimerange"
              unlink-panels
              :start-placeholder="t('monitoring.reports.rangeStart')"
              :end-placeholder="t('monitoring.reports.rangeEnd')"
              value-format="YYYY-MM-DDTHH:mm:ssZ"
            />
          </el-form-item>
          <el-form-item prop="format" :label="t('monitoring.reports.formatLabel')">
            <el-radio-group v-model="reportForm.format">
              <el-radio-button v-for="fmt in formatOptions" :key="fmt" :label="fmt">{{
                fmt.toUpperCase()
              }}</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="submitting" @click="submitReport">{{
              t('monitoring.reports.submitReport')
            }}</el-button>
            <el-button text @click="resetForm">{{ t('common.reset') }}</el-button>
          </el-form-item>
        </el-form>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <div>
              <p class="card-eyebrow">{{ t('monitoring.reports.historyTitle') }}</p>
              <h3>{{ t('monitoring.reports.historySubtitle') }}</h3>
            </div>
            <div class="history-filters">
              <el-select
                v-model="filters.type"
                clearable
                size="small"
                :placeholder="t('monitoring.reports.filterType')"
              >
                <el-option
                  v-for="opt in reportTypes"
                  :key="opt"
                  :label="reportTypeLabel(opt)"
                  :value="opt"
                />
              </el-select>
              <el-select
                v-model="filters.status"
                clearable
                size="small"
                :placeholder="t('monitoring.reports.filterStatus')"
              >
                <el-option
                  v-for="opt in statusOptions"
                  :key="opt"
                  :label="statusLabel(opt)"
                  :value="opt"
                />
              </el-select>
              <el-button size="small" class="sky-btn" :loading="loading" @click="loadTasks">{{
                t('monitoring.apply')
              }}</el-button>
            </div>
          </div>
        </template>
        <el-table :data="tasks" size="small" border stripe>
          <template v-if="!tasks.length" #empty>
            <el-empty :description="t('monitoring.reports.empty')" />
          </template>
          <el-table-column prop="type" :label="t('monitoring.reports.columnType')" min-width="160">
            <template #default="{ row }">{{ reportTypeLabel(row.type) }}</template>
          </el-table-column>
          <el-table-column prop="status" :label="t('monitoring.reports.columnStatus')" width="140">
            <template #default="{ row }">
              <el-tag :type="statusType(row.status)">{{ statusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column
            prop="createdAt"
            :label="t('monitoring.reports.columnCreated')"
            min-width="200"
          >
            <template #default="{ row }">{{ formatTs(row.createdAt) }}</template>
          </el-table-column>
          <el-table-column :label="t('monitoring.reports.columnDownload')" width="160">
            <template #default="{ row }">
              <el-button
                size="small"
                text
                :disabled="!row.downloadUrl || row.status !== 'done'"
                @click="download(row)"
                >{{ t('monitoring.reports.download') }}</el-button
              >
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { showHttpError } from '@/shared/errors/errorToast';
import { showSuccess, showWarning } from '@/shared/errors/messageToast';
import type { FormInstance } from 'element-plus';
import dayjs from 'dayjs';
import { useTenantStore } from '@/store/tenant';
import { createReportTask, listReportTasks } from '@/api/monitoring';
import type { ReportRequest, ReportTask } from '@/types/monitoring';
import { formatTs } from '@/utils/time';

const { t } = useI18n();
const tenantStore = useTenantStore();
const loading = ref(false);
const submitting = ref(false);
const tasks = ref<ReportTask[]>([]);
const reportTypes: ReportRequest['type'][] = ['ops-daily', 'performance', 'capacity', 'security'];
const formatOptions: ReportRequest['format'][] = ['pdf', 'csv'];
const statusOptions: ReportTask['status'][] = ['pending', 'running', 'done', 'failed'];

const reportFormRef = ref<FormInstance>();
const reportForm = reactive<{
  type: ReportRequest['type'];
  range: string[];
  format: ReportRequest['format'];
}>({
  type: 'ops-daily',
  range: [],
  format: 'pdf'
});

const filters = reactive<{ type: string; status: string }>({ type: '', status: '' });

const heroStats = computed(() => [
  {
    key: 'pending',
    label: t('monitoring.reports.statPending'),
    value: tasks.value.filter((task) => task.status === 'pending' || task.status === 'running')
      .length,
    hint: t('monitoring.reports.statPendingHint')
  },
  {
    key: 'completed',
    label: t('monitoring.reports.statCompleted'),
    value: tasks.value.filter((task) => task.status === 'done').length,
    hint: t('monitoring.reports.statCompletedHint')
  },
  {
    key: 'failed',
    label: t('monitoring.reports.statFailed'),
    value: tasks.value.filter((task) => task.status === 'failed').length,
    hint: t('monitoring.reports.statFailedHint')
  }
]);

const reportTypeLabel = (type: ReportRequest['type']) =>
  t(`monitoring.reports.types.${type}` as never, type);
const statusLabel = (status: ReportTask['status']) =>
  t(`monitoring.reports.status.${status}` as never, status);
const statusType = (status: ReportTask['status']) => {
  if (status === 'failed') return 'danger';
  if (status === 'pending' || status === 'running') return 'warning';
  if (status === 'done') return 'success';
  return 'info';
};

const normalizeTasks = (payload: any): ReportTask[] => {
  if (!payload) return [];
  if (Array.isArray(payload)) return payload as ReportTask[];
  if (Array.isArray(payload.items)) return payload.items as ReportTask[];
  return [];
};

const withTenant = () => tenantStore.currentTenantId || undefined;

const loadTasks = async () => {
  loading.value = true;
  try {
    const params: any = { tenantId: withTenant(), page: 1, pageSize: 50 };
    if (filters.type) params.type = filters.type;
    if (filters.status) params.status = filters.status;
    const res = await listReportTasks(params);
    tasks.value = normalizeTasks(res.data.data);
  } catch (error) {
    showHttpError(error, t('monitoring.loadFail'));
  } finally {
    loading.value = false;
  }
};

const resetForm = () => {
  Object.assign(reportForm, { type: 'ops-daily', range: [], format: 'pdf' });
};

const submitReport = async () => {
  if (!reportForm.range || reportForm.range.length !== 2) {
    showWarning(t('monitoring.dateRangeRequired'));
    return;
  }
  try {
    submitting.value = true;
    await createReportTask({
      type: reportForm.type,
      range: {
        from: dayjs(reportForm.range[0]).toISOString(),
        to: dayjs(reportForm.range[1]).toISOString()
      },
      format: reportForm.format,
      tenantId: withTenant()
    });
    showSuccess(t('monitoring.submitted'));
    resetForm();
    await loadTasks();
  } catch (error) {
    showHttpError(error, t('monitoring.submitFail'));
  } finally {
    submitting.value = false;
  }
};

const download = (task: ReportTask) => {
  if (!task.downloadUrl) return;
  window.open(task.downloadUrl, '_blank');
};

watch(
  () => tenantStore.currentTenantId,
  () => loadTasks()
);

onMounted(() => loadTasks());
</script>

<style scoped>
.monitor-reports {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.reports-hero {
  background: linear-gradient(120deg, #022c22, #047857);
  color: #fff;
  border-radius: 20px;
  padding: 24px 24px 28px;
}

.hero-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 12px;
  margin-top: 16px;
}

.stat-card {
  background: rgba(2, 44, 34, 0.4);
  border-radius: 12px;
  padding: 12px 14px;
}

.stat-card .label {
  font-size: 12px;
  opacity: 0.8;
}

.stat-card .value {
  display: block;
  font-size: 26px;
  font-weight: 600;
}

.stat-card .hint {
  font-size: 12px;
  opacity: 0.8;
}

.split-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 18px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-eyebrow {
  font-size: 12px;
  text-transform: uppercase;
  color: var(--el-text-color-secondary);
}

.history-filters {
  display: flex;
  gap: 8px;
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

@media (max-width: 960px) {
  .history-filters {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
