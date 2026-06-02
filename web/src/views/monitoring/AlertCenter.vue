<template>
  <div class="alert-center page-block">
    <section class="hero-card alert-hero">
      <div>
        <p class="eyebrow">{{ t('monitoring.alerts.heroEyebrow') }}</p>
        <h2>{{ t('monitoring.alerts.heroTitle') }}</h2>
        <p class="desc">{{ t('monitoring.alerts.heroDesc') }}</p>
      </div>
      <div class="hero-stats">
        <div v-for="stat in heroStats" :key="stat.key" class="stat-chip">
          <span class="label">{{ stat.label }}</span>
          <span class="value">{{ stat.value }}</span>
          <span class="hint">{{ stat.hint }}</span>
        </div>
      </div>
      <div class="hero-actions">
        <el-button type="primary" size="small" @click="openRuleDrawer()">{{
          t('monitoring.alerts.addRule')
        }}</el-button>
      </div>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <div>
              <p class="card-eyebrow">{{ t('monitoring.alerts.rulesTitle') }}</p>
              <h3>{{ t('monitoring.alerts.rulesSubtitle') }}</h3>
            </div>
            <el-button size="small" class="sky-btn" :loading="loading" @click="loadRules">{{
              t('common.refresh')
            }}</el-button>
          </div>
        </template>
        <el-table :data="rules" size="small" border stripe>
          <template v-if="!rules.length" #empty>
            <el-empty :description="t('monitoring.alerts.noRules')" />
          </template>
          <el-table-column prop="name" :label="t('monitoring.alerts.ruleName')" min-width="180" />
          <el-table-column
            prop="metric"
            :label="t('monitoring.alerts.ruleMetric')"
            min-width="180"
          />
          <el-table-column prop="severity" :label="t('monitoring.alerts.ruleSeverity')" width="120">
            <template #default="{ row }">
              <el-tag :type="severityType(row.severity)">{{ row.severity }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('monitoring.alerts.ruleCondition')" min-width="160">
            <template #default="{ row }">
              {{ row.operator }} {{ row.threshold }} | {{ row.duration }}s
            </template>
          </el-table-column>
          <el-table-column prop="enabled" :label="t('monitoring.alerts.ruleEnabled')" width="120">
            <template #default="{ row }">
              <el-tag :type="row.enabled ? 'success' : 'info'">{{
                row.enabled
                  ? t('monitoring.alerts.ruleEnabledOn')
                  : t('monitoring.alerts.ruleEnabledOff')
              }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="160">
            <template #default="{ row }">
              <el-button size="small" text @click="openRuleDrawer(row)">{{
                t('common.edit')
              }}</el-button>
              <el-button size="small" text type="danger" @click="removeRule(row)">{{
                t('common.delete')
              }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <div>
              <p class="card-eyebrow">{{ t('monitoring.alerts.eventsTitle') }}</p>
              <h3>{{ t('monitoring.alerts.eventsSubtitle') }}</h3>
            </div>
            <div class="event-filters">
              <el-select
                v-model="eventFilters.severity"
                clearable
                size="small"
                :placeholder="t('monitoring.alerts.filterSeverity')"
              >
                <el-option v-for="opt in severityOptions" :key="opt" :label="opt" :value="opt" />
              </el-select>
              <el-select
                v-model="eventFilters.status"
                clearable
                size="small"
                :placeholder="t('monitoring.alerts.filterStatus')"
              >
                <el-option v-for="opt in statusOptions" :key="opt" :label="opt" :value="opt" />
              </el-select>
              <el-button size="small" class="sky-btn" @click="loadEvents">{{
                t('monitoring.apply')
              }}</el-button>
            </div>
          </div>
        </template>
        <el-table :data="events" size="small" border stripe>
          <template v-if="!events.length" #empty>
            <el-empty :description="t('monitoring.alerts.noEvents')" />
          </template>
          <el-table-column
            prop="severity"
            :label="t('monitoring.alerts.eventSeverity')"
            width="120"
          >
            <template #default="{ row }">
              <el-tag :type="severityType(row.severity)">{{ row.severity }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column
            prop="message"
            :label="t('monitoring.alerts.eventMessage')"
            min-width="240"
          />
          <el-table-column prop="status" :label="t('monitoring.alerts.eventStatus')" width="140">
            <template #default="{ row }">
              <el-tag
                :type="
                  row.status === 'firing' ? 'danger' : row.status === 'ack' ? 'warning' : 'success'
                "
                >{{ row.status }}</el-tag
              >
            </template>
          </el-table-column>
          <el-table-column
            prop="createdAt"
            :label="t('monitoring.alerts.eventTime')"
            min-width="200"
          >
            <template #default="{ row }">{{ formatTs(row.createdAt) }}</template>
          </el-table-column>
        </el-table>
      </el-card>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <div>
              <p class="card-eyebrow">{{ t('monitoring.alerts.channelsTitle') }}</p>
              <h3>{{ t('monitoring.alerts.channelsSubtitle') }}</h3>
            </div>
            <el-button size="small" type="primary" @click="openChannelDialog">{{
              t('monitoring.alerts.addChannel')
            }}</el-button>
          </div>
        </template>
        <el-table :data="channels" size="small" border stripe>
          <template v-if="!channels.length" #empty>
            <el-empty :description="t('monitoring.alerts.noChannels')" />
          </template>
          <el-table-column prop="name" :label="t('monitoring.alerts.channelName')" min-width="160" />
          <el-table-column prop="type" :label="t('monitoring.alerts.channelType')" width="140" />
          <el-table-column
            prop="target"
            :label="t('monitoring.alerts.channelTarget')"
            min-width="240"
          />
          <el-table-column
            prop="enabled"
            :label="t('monitoring.alerts.channelEnabled')"
            width="140"
          >
            <template #default="{ row }">
              <el-tag :type="row.enabled ? 'success' : 'info'">{{
                row.enabled
                  ? t('monitoring.alerts.channelActive')
                  : t('monitoring.alerts.channelInactive')
              }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </section>

    <el-drawer v-model="ruleDrawerVisible" :title="drawerTitle" size="40%">
      <el-form ref="ruleFormRef" :model="ruleForm" :rules="ruleRules" label-width="120px">
        <el-form-item prop="name" :label="t('monitoring.alerts.ruleName')">
          <el-input v-model="ruleForm.name" />
        </el-form-item>
        <el-form-item prop="metric" :label="t('monitoring.alerts.ruleMetric')">
          <el-input v-model="ruleForm.metric" />
        </el-form-item>
        <el-form-item prop="operator" :label="t('monitoring.alerts.ruleOperator')">
          <el-select v-model="ruleForm.operator">
            <el-option v-for="opt in operatorOptions" :key="opt" :label="opt" :value="opt" />
          </el-select>
        </el-form-item>
        <el-form-item prop="threshold" :label="t('monitoring.alerts.ruleThreshold')">
          <el-input-number v-model="ruleForm.threshold" :min="0" :step="1" />
        </el-form-item>
        <el-form-item prop="duration" :label="t('monitoring.alerts.ruleDuration')">
          <el-input-number v-model="ruleForm.duration" :min="1" :step="5" />
        </el-form-item>
        <el-form-item prop="severity" :label="t('monitoring.alerts.ruleSeverity')">
          <el-select v-model="ruleForm.severity">
            <el-option v-for="opt in severityOptions" :key="opt" :label="opt" :value="opt" />
          </el-select>
        </el-form-item>
        <el-form-item prop="match" :label="t('monitoring.alerts.ruleMatch')">
          <el-input v-model="ruleForm.match" :placeholder="t('monitoring.alerts.ruleMatchHint')" />
        </el-form-item>
        <el-form-item prop="enabled" :label="t('monitoring.alerts.ruleEnabled')">
          <el-switch v-model="ruleForm.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="drawer-footer">
          <el-button @click="ruleDrawerVisible = false">{{ t('common.cancel') }}</el-button>
          <el-button type="primary" :loading="savingRule" @click="submitRule">{{
            t('common.save')
          }}</el-button>
        </div>
      </template>
    </el-drawer>

    <el-dialog
      v-model="channelDialogVisible"
      :title="t('monitoring.alerts.channelDialogTitle')"
      width="480px"
    >
      <el-form ref="channelFormRef" :model="channelForm" :rules="channelRules" label-width="140px">
        <el-form-item prop="name" :label="t('monitoring.alerts.channelName')">
          <el-input v-model="channelForm.name" />
        </el-form-item>
        <el-form-item prop="type" :label="t('monitoring.alerts.channelType')">
          <el-select v-model="channelForm.type">
            <el-option v-for="opt in channelTypes" :key="opt" :label="opt" :value="opt" />
          </el-select>
        </el-form-item>
        <el-form-item prop="target" :label="t('monitoring.alerts.channelTarget')">
          <el-input v-model="channelForm.target" />
        </el-form-item>
        <el-form-item prop="enabled" :label="t('monitoring.alerts.channelEnabled')">
          <el-switch v-model="channelForm.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="channelDialogVisible = false">{{ t('common.cancel') }}</el-button>
          <el-button type="primary" :loading="savingChannel" @click="submitChannel">{{
            t('common.save')
          }}</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { ElMessageBox } from 'element-plus';
import { showHttpError } from '@/shared/errors/errorToast';
import { showSuccess } from '@/shared/errors/messageToast';
import type { FormInstance, FormRules } from 'element-plus';
import { useTenantStore } from '@/store/tenant';
import {
  listAlertRules,
  saveAlertRule,
  deleteAlertRule,
  listAlertEvents,
  listChannels,
  saveChannel
} from '@/api/monitoring';
import type { AlertEvent, AlertRule, NotificationChannel } from '@/types/monitoring';
import { formatTs } from '@/utils/time';

const { t } = useI18n();
const tenantStore = useTenantStore();
const loading = ref(false);
const rules = ref<AlertRule[]>([]);
const events = ref<AlertEvent[]>([]);
const channels = ref<NotificationChannel[]>([]);
const savingRule = ref(false);
const savingChannel = ref(false);

const severityOptions: AlertRule['severity'][] = ['info', 'warning', 'critical'];
const statusOptions: AlertEvent['status'][] = ['firing', 'ack', 'resolved'];
const operatorOptions: AlertRule['operator'][] = ['>', '>=', '<', '<='];
const channelTypes: NotificationChannel['type'][] = ['email', 'webhook', 'sms', 'pagerduty'];

const heroStats = computed(() => [
  {
    key: 'rules',
    label: t('monitoring.alerts.statRules'),
    value: rules.value.length,
    hint: t('monitoring.alerts.statRulesHint')
  },
  {
    key: 'firing',
    label: t('monitoring.alerts.statFiring'),
    value: events.value.filter((e) => e.status === 'firing').length,
    hint: t('monitoring.alerts.statFiringHint')
  },
  {
    key: 'channels',
    label: t('monitoring.alerts.statChannels'),
    value: channels.value.length,
    hint: t('monitoring.alerts.statChannelsHint')
  }
]);

const severityType = (severity: AlertRule['severity'] | AlertEvent['severity']) => {
  if (severity === 'critical') return 'danger';
  if (severity === 'warning') return 'warning';
  return 'info';
};

const ruleDrawerVisible = ref(false);
const ruleFormRef = ref<FormInstance>();
const ruleForm = reactive<Partial<AlertRule>>({
  id: '',
  name: '',
  metric: '',
  operator: '>' as AlertRule['operator'],
  threshold: 0,
  duration: 60,
  severity: 'warning',
  enabled: true,
  match: ''
});

const ruleRules: FormRules = {
  name: [{ required: true, message: t('monitoring.required'), trigger: 'blur' }],
  metric: [{ required: true, message: t('monitoring.required'), trigger: 'blur' }],
  threshold: [{ type: 'number', min: 0, message: t('monitoring.thresholdRange') }],
  duration: [{ type: 'number', min: 1, message: t('monitoring.durationRange') }]
};

const eventFilters = reactive({ severity: '', status: '' });

const channelDialogVisible = ref(false);
const channelFormRef = ref<FormInstance>();
const channelForm = reactive<Partial<NotificationChannel>>({
  id: '',
  name: '',
  type: 'email',
  target: '',
  enabled: true
});

const channelRules: FormRules = {
  name: [{ required: true, message: t('monitoring.required'), trigger: 'blur' }],
  type: [{ required: true, message: t('monitoring.required') }],
  target: [{ required: true, message: t('monitoring.required'), trigger: 'blur' }]
};

const drawerTitle = computed(() =>
  ruleForm.id ? t('monitoring.alerts.editRule') : t('monitoring.alerts.addRule')
);

const withTenant = () => tenantStore.currentTenantId || undefined;

const normalizeResponse = <T,>(payload: any): T[] => {
  if (!payload) return [];
  if (Array.isArray(payload)) return payload as T[];
  if (Array.isArray(payload.items)) return payload.items as T[];
  return [];
};

const loadRules = async () => {
  loading.value = true;
  try {
    const res = await listAlertRules({ tenantId: withTenant(), page: 1, pageSize: 200 });
    rules.value = normalizeResponse<AlertRule>(res.data.data);
  } catch (error) {
    showHttpError(error, t('monitoring.loadFail'));
  } finally {
    loading.value = false;
  }
};

const loadEvents = async () => {
  const params: Record<string, any> = { tenantId: withTenant(), page: 1, pageSize: 40 };
  if (eventFilters.severity) params.severity = eventFilters.severity;
  if (eventFilters.status) params.status = eventFilters.status;
  try {
    const res = await listAlertEvents(params as any);
    events.value = normalizeResponse<AlertEvent>(res.data.data);
  } catch (error) {
    showHttpError(error, t('monitoring.loadFail'));
  }
};

const loadChannels = async () => {
  try {
    const res = await listChannels({ tenantId: withTenant() });
    channels.value = res.data.data || [];
  } catch (error) {
    showHttpError(error, t('monitoring.loadFail'));
  }
};

const openRuleDrawer = (rule?: AlertRule) => {
  if (rule) {
    Object.assign(ruleForm, rule);
  } else {
    Object.assign(ruleForm, {
      id: '',
      name: '',
      metric: '',
      operator: '>' as AlertRule['operator'],
      threshold: 0,
      duration: 60,
      severity: 'warning',
      enabled: true,
      match: ''
    });
  }
  ruleDrawerVisible.value = true;
};

const submitRule = async () => {
  if (!ruleFormRef.value) return;
  try {
    await ruleFormRef.value.validate();
  } catch (error) {
    return;
  }
  try {
    savingRule.value = true;
    await saveAlertRule({ ...ruleForm, tenantId: withTenant() });
    showSuccess(t('monitoring.saved'));
    ruleDrawerVisible.value = false;
    await loadRules();
  } catch (error) {
    showHttpError(error, t('monitoring.saveFail'));
  } finally {
    savingRule.value = false;
  }
};

const removeRule = async (rule: AlertRule) => {
  try {
    await ElMessageBox.confirm(
      t('monitoring.alerts.deleteRuleConfirm'),
      t('monitoring.alerts.deleteRuleTitle'),
      { type: 'warning' }
    );
    await deleteAlertRule(rule.id, { tenantId: withTenant() });
    showSuccess(t('monitoring.deleted'));
    await loadRules();
  } catch (error) {
    if (error !== 'cancel') showHttpError(error, t('monitoring.deleteFail'));
  }
};

const openChannelDialog = () => {
  Object.assign(channelForm, { id: '', name: '', type: 'email', target: '', enabled: true });
  channelDialogVisible.value = true;
};

const submitChannel = async () => {
  if (!channelFormRef.value) return;
  try {
    await channelFormRef.value.validate();
  } catch (error) {
    return;
  }
  try {
    savingChannel.value = true;
    await saveChannel({ ...channelForm, tenantId: withTenant() });
    showSuccess(t('monitoring.saved'));
    channelDialogVisible.value = false;
    await loadChannels();
  } catch (error) {
    showHttpError(error, t('monitoring.saveFail'));
  } finally {
    savingChannel.value = false;
  }
};

const loadAll = async () => {
  await Promise.all([loadRules(), loadEvents(), loadChannels()]);
};

watch(
  () => tenantStore.currentTenantId,
  () => loadAll()
);

onMounted(() => loadAll());
</script>

<style scoped>
.alert-center {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.alert-hero {
  background: linear-gradient(120deg, #1d1b72, #be185d);
  color: #fff;
  position: relative;
  border-radius: 20px;
  padding: 24px 24px 28px;
}

.hero-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
  margin-top: 16px;
}

.stat-chip {
  background: rgba(15, 15, 40, 0.35);
  border-radius: 12px;
  padding: 12px 14px;
}

.stat-chip .label {
  font-size: 12px;
  opacity: 0.8;
}

.stat-chip .value {
  display: block;
  font-size: 26px;
  font-weight: 600;
}

.stat-chip .hint {
  font-size: 12px;
  opacity: 0.8;
}

.hero-actions {
  position: absolute;
  top: 24px;
  right: 24px;
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

.split-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 18px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.card-eyebrow {
  font-size: 12px;
  text-transform: uppercase;
  color: var(--el-text-color-secondary);
}

.event-filters {
  display: flex;
  gap: 8px;
  align-items: center;
}

.drawer-footer,
.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

@media (max-width: 960px) {
  .hero-actions {
    position: static;
    margin-top: 12px;
  }

  .event-filters {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
