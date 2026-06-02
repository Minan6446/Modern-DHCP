<template>
  <div class="page-wrap">
    <div class="page-header surface-card">
      <h3>{{ t('monitoring.config.title') }}</h3>
      <p class="desc">{{ t('monitoring.config.desc') }}</p>
    </div>

    <div class="filter-bar surface-card">
      <div class="bar-left"></div>
      <div class="bar-middle"></div>
      <div class="bar-right">
        <el-button :loading="loading" @click="loadConfig">{{ t('monitoring.config.refresh') }}</el-button>
        <el-button type="primary" :loading="saving" @click="saveCurrent">{{ t('monitoring.config.saveCurrentTab') }}</el-button>
      </div>
    </div>

    <section class="stat-grid">
      <el-card v-for="item in statCards" :key="item.key" shadow="never" class="stat-card">
        <div class="stat-title">{{ item.title }}</div>
        <div class="stat-value">{{ item.value }}</div>
        <div class="stat-sub">{{ item.sub }}</div>
      </el-card>
    </section>

    <el-card class="surface-card content-card" v-loading="loading || saving">
      <el-tabs v-model="activeTab" class="tabs">
        <el-tab-pane :label="t('monitoring.config.tabThresholds')" name="thresholds">
          <div class="threshold-layout">
            <el-card shadow="never" class="inner-card">
              <template #header>
                <div class="inner-title">{{ t('monitoring.config.resourceTitle') }}</div>
              </template>
              <el-form label-width="160px">
                <el-form-item :label="t('monitoring.config.poolUsage')">
                  <el-input-number v-model="thresholds.resource.poolUsage" :min="0" :max="100" /> %
                </el-form-item>
                <el-form-item :label="t('monitoring.config.leaseUsage')">
                  <el-input-number v-model="thresholds.resource.leaseUsage" :min="0" :max="100" /> %
                </el-form-item>
                <el-form-item :label="t('monitoring.config.renewFail')">
                  <el-input-number v-model="thresholds.resource.renewFail" :min="0" :max="100" /> %
                </el-form-item>
                <el-form-item :label="t('monitoring.config.failedRequestRatio')">
                  <el-input-number v-model="thresholds.resource.failedRequestRatio" :min="0" :max="100" /> %
                </el-form-item>
              </el-form>
            </el-card>

            <el-card shadow="never" class="inner-card">
              <template #header>
                <div class="inner-title">{{ t('monitoring.config.serverTitle') }}</div>
              </template>
              <el-form label-width="160px">
                <el-form-item :label="t('monitoring.config.responseTimeout')">
                  <el-input-number v-model="thresholds.server.responseTimeout" :min="1" :max="60" /> {{ t('monitoring.config.unitSecond') }}
                </el-form-item>
                <el-form-item :label="t('monitoring.config.responseTime')">
                  <el-input-number v-model="thresholds.server.responseTimeMs" :min="1" :max="10000" /> ms
                </el-form-item>
                <el-form-item :label="t('monitoring.config.cpuUsage')">
                  <el-input-number v-model="thresholds.server.cpuUsage" :min="0" :max="100" /> %
                </el-form-item>
                <el-form-item :label="t('monitoring.config.memoryUsage')">
                  <el-input-number v-model="thresholds.server.memoryUsage" :min="0" :max="100" /> %
                </el-form-item>
                <el-form-item :label="t('monitoring.config.processCheck')">
                  <el-switch v-model="thresholds.server.processCheck" />
                </el-form-item>
              </el-form>
            </el-card>

            <el-card shadow="never" class="inner-card">
              <template #header>
                <div class="inner-title">{{ t('monitoring.config.networkTitle') }}</div>
              </template>
              <el-form label-width="160px">
                <el-form-item :label="t('monitoring.config.conflictSensitivity')">
                  <el-select v-model="thresholds.network.conflictSensitivity" style="width: 140px">
                    <el-option :label="t('monitoring.config.sensitivityHigh')" value="high" />
                    <el-option :label="t('monitoring.config.sensitivityMedium')" value="medium" />
                    <el-option :label="t('monitoring.config.sensitivityLow')" value="low" />
                  </el-select>
                </el-form-item>
                <el-form-item :label="t('monitoring.config.abnormalQps')">
                  <el-input-number v-model="thresholds.network.abnormalQps" :min="1" :max="10000" /> {{ t('monitoring.config.unitPerMin') }}
                </el-form-item>
                <el-form-item :label="t('monitoring.config.duplicateIpDetection')">
                  <el-switch v-model="thresholds.network.duplicateIpDetection" />
                </el-form-item>
                <el-form-item :label="t('monitoring.config.unauthorizedServerDetection')">
                  <el-switch v-model="thresholds.network.unauthorizedServerDetection" />
                </el-form-item>
              </el-form>
            </el-card>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('monitoring.config.tabNotify')" name="notify">
          <div class="notify-layout">
            <el-card shadow="never" class="inner-card">
              <template #header>
                <div class="inner-title">{{ t('monitoring.config.channelTitle') }}</div>
              </template>
              <el-form label-width="120px">
                <el-form-item :label="t('monitoring.config.channelEmail')">
                  <el-switch v-model="notify.channels.email" />
                </el-form-item>
                <el-form-item :label="t('monitoring.config.channelSms')">
                  <el-switch v-model="notify.channels.sms" />
                </el-form-item>
                <el-form-item label="Webhook">
                  <el-switch v-model="notify.channels.webhook" />
                </el-form-item>
                <el-form-item v-if="notify.channels.webhook" :label="t('monitoring.config.channelWebhookUrl')">
                  <el-input v-model="notify.channels.webhookUrl" placeholder="https://..." />
                </el-form-item>
              </el-form>
            </el-card>

            <el-card shadow="never" class="inner-card">
              <template #header>
                <div class="inner-title">{{ t('monitoring.config.policyTitle') }}</div>
              </template>
              <el-form label-width="110px">
                <el-form-item :label="t('monitoring.config.policyEmergency')">
                  <el-checkbox-group v-model="notify.policies.emergency">
                    <el-checkbox label="email">{{ t('monitoring.config.policyEmail') }}</el-checkbox>
                    <el-checkbox label="sms">{{ t('monitoring.config.policySms') }}</el-checkbox>
                    <el-checkbox label="webhook">{{ t('monitoring.config.policyWebhook') }}</el-checkbox>
                  </el-checkbox-group>
                </el-form-item>
                <el-form-item :label="t('monitoring.config.policyCritical')">
                  <el-checkbox-group v-model="notify.policies.critical">
                    <el-checkbox label="email">{{ t('monitoring.config.policyEmail') }}</el-checkbox>
                    <el-checkbox label="webhook">{{ t('monitoring.config.policyWebhook') }}</el-checkbox>
                  </el-checkbox-group>
                </el-form-item>
                <el-form-item :label="t('monitoring.config.policyInfo')">
                  <el-checkbox-group v-model="notify.policies.info">
                    <el-checkbox label="email">{{ t('monitoring.config.policyEmail') }}</el-checkbox>
                  </el-checkbox-group>
                </el-form-item>
              </el-form>
            </el-card>

            <el-card shadow="never" class="inner-card template-card">
              <template #header>
                <div class="inner-title-row">
                  <span class="inner-title">{{ t('monitoring.config.templateTitle') }}</span>
                  <el-button type="primary" link @click="addTemplate">{{ t('monitoring.config.templateAdd') }}</el-button>
                </div>
              </template>
              <el-table :data="templates" border stripe>
                <el-table-column prop="name" :label="t('monitoring.config.templateColName')" min-width="140" />
                <el-table-column prop="lang" :label="t('monitoring.config.templateColLang')" width="120" />
                <el-table-column :label="t('monitoring.config.templateColChannel')" width="100">
                  <template #default="{ row }">
                    <el-tag size="small">{{ row.channel }}</el-tag>
                  </template>
                </el-table-column>
                <el-table-column prop="updatedAt" :label="t('monitoring.config.templateColUpdated')" width="160">
                  <template #default="{ row }">{{ row.updatedAt ? new Date(row.updatedAt).toLocaleString() : '-' }}</template>
                </el-table-column>
                <el-table-column :label="t('monitoring.config.templateColActions')" width="150" fixed="right">
                  <template #default="{ row }">
                    <el-button type="primary" link @click="openEditTemplate(row)">{{ t('monitoring.config.templateEdit') }}</el-button>
                    <el-button type="danger" link @click="removeTemplate(row)">{{ t('monitoring.config.templateDelete') }}</el-button>
                  </template>
                </el-table-column>
              </el-table>
            </el-card>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('monitoring.config.tabReceivers')" name="receivers">
          <div class="receiver-toolbar">
            <div class="toolbar-left">
              <el-input v-model="receiverKeyword" clearable :placeholder="t('monitoring.config.receiverSearch')" class="search" />
            </div>
            <div class="toolbar-right">
              <el-button @click="pickFile">{{ t('monitoring.config.receiverBulkImport') }}</el-button>
              <el-button @click="exportReceiversAction">{{ t('monitoring.config.receiverExport') }}</el-button>
              <el-button type="primary" @click="addReceiver">{{ t('monitoring.config.receiverAdd') }}</el-button>
            </div>
          </div>
          <el-table :data="filteredReceivers" border stripe>
            <el-table-column type="selection" width="48" />
            <el-table-column prop="name" :label="t('monitoring.config.receiverColName')" width="110" />
            <el-table-column prop="email" :label="t('monitoring.config.receiverColEmail')" min-width="220" />
            <el-table-column prop="phone" :label="t('monitoring.config.receiverColPhone')" width="130" />
            <el-table-column :label="t('monitoring.config.receiverColLevel')" min-width="160">
              <template #default="{ row }">{{ formatLevels(row.levels) }}</template>
            </el-table-column>
            <el-table-column prop="department" :label="t('monitoring.config.receiverColDept')" min-width="160" />
            <el-table-column :label="t('monitoring.config.receiverColActions')" width="150" fixed="right">
              <template #default="{ row }">
                <el-button type="primary" link @click="editReceiver(row)">{{ t('monitoring.config.receiverEdit') }}</el-button>
                <el-button type="danger" link @click="removeReceiver(row)">{{ t('monitoring.config.receiverDelete') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <el-dialog v-model="templateDialogVisible" :title="templateForm.id ? t('monitoring.config.templateDialogEdit') : t('monitoring.config.templateDialogCreate')" width="640px">
      <el-form :model="templateForm" label-width="120px">
        <el-form-item :label="t('monitoring.config.templateFormName')"><el-input v-model="templateForm.name" /></el-form-item>
        <el-form-item :label="t('monitoring.config.templateFormLang')">
          <el-select v-model="templateForm.lang" style="width: 180px">
            <el-option :label="t('monitoring.config.templateLangZh')" value="zh-CN" />
            <el-option :label="t('monitoring.config.templateLangEn')" value="en-US" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('monitoring.config.templateFormChannel')">
          <el-select v-model="templateForm.channel" style="width: 180px">
            <el-option :label="t('monitoring.config.templateChannelEmail')" value="email" />
            <el-option :label="t('monitoring.config.templateChannelSms')" value="sms" />
            <el-option :label="t('monitoring.config.templateChannelWebhook')" value="webhook" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="templateForm.channel === 'email'" :label="t('monitoring.config.templateFormSubject')">
          <el-input v-model="templateForm.subject" />
        </el-form-item>
        <el-form-item :label="t('monitoring.config.templateFormBody')">
          <el-input v-model="templateForm.body" type="textarea" :rows="8" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="templateDialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveTemplate">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="receiverDialogVisible" :title="receiverEditingId ? t('monitoring.config.receiverDialogEdit') : t('monitoring.config.receiverDialogCreate')" width="520px">
      <el-form :model="receiverForm" label-width="120px">
        <el-form-item :label="t('monitoring.config.receiverFormName')" required><el-input v-model="receiverForm.name" /></el-form-item>
        <el-form-item :label="t('monitoring.config.receiverFormEmail')" required><el-input v-model="receiverForm.email" /></el-form-item>
        <el-form-item :label="t('monitoring.config.receiverFormPhone')"><el-input v-model="receiverForm.phone" /></el-form-item>
        <el-form-item :label="t('monitoring.config.receiverFormLevel')">
          <el-checkbox-group v-model="receiverForm.levels">
            <el-checkbox label="emergency">{{ t('monitoring.config.receiverLevelEmergency') }}</el-checkbox>
            <el-checkbox label="critical">{{ t('monitoring.config.receiverLevelCritical') }}</el-checkbox>
            <el-checkbox label="info">{{ t('monitoring.config.receiverLevelInfo') }}</el-checkbox>
            <el-checkbox label="all">{{ t('monitoring.config.receiverLevelAll') }}</el-checkbox>
          </el-checkbox-group>
        </el-form-item>
        <el-form-item :label="t('monitoring.config.receiverFormDept')"><el-input v-model="receiverForm.department" /></el-form-item>
        <el-form-item :label="t('monitoring.config.receiverFormSchedule')"><el-input v-model="receiverForm.schedule" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="receiverDialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveReceiver">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { ElMessageBox } from 'element-plus';
import { showHttpError } from '@/shared/errors/errorToast';
import { useI18n } from 'vue-i18n';
import { showError, showSuccess } from '@/shared/errors/messageToast';
import {
  createAlertReceiver,
  createAlertTemplate,
  deleteAlertReceiver,
  deleteAlertTemplate,
  exportAlertReceivers,
  getAlertNotifyConfig,
  getAlertThresholdConfig,
  importAlertReceivers,
  listAlertReceivers,
  listAlertTemplates,
  saveAlertNotifyConfig,
  saveAlertThresholdConfig,
  updateAlertReceiver,
  updateAlertTemplate
} from '@/api/monitoring';
import type { AlertNotifyConfig, AlertReceiver, AlertTemplate, AlertThresholdConfig } from '@/types/monitoring';

const { t } = useI18n();
const route = useRoute();
const loading = ref(false);
const saving = ref(false);
const activeTab = ref<'thresholds' | 'notify' | 'receivers'>('thresholds');
const receiverKeyword = ref('');

const activeTabLabel = computed(() => {
  if (activeTab.value === 'thresholds') return t('monitoring.config.tabLabelThresholds');
  if (activeTab.value === 'notify') return t('monitoring.config.tabLabelNotify');
  return t('monitoring.config.tabLabelReceivers');
});

const thresholds = ref<AlertThresholdConfig>({
  resource: {
    poolUsage: 85,
    leaseUsage: 90,
    renewFail: 10,
    leaseTimeDrift: 30,
    failedRequestRatio: 5,
    subnetImbalance: 20,
    logErrorThreshold: 50
  },
  server: {
    responseTimeout: 5,
    responseTimeMs: 200,
    cpuUsage: 85,
    memoryUsage: 85,
    processCheck: true
  },
  network: {
    conflictSensitivity: 'medium',
    abnormalQps: 120,
    duplicateIpDetection: true,
    unauthorizedServerDetection: true
  }
});

const notify = ref<AlertNotifyConfig>({
  channels: { email: true, sms: false, webhook: true, webhookUrl: '' },
  policies: {
    emergency: ['email', 'sms', 'webhook'],
    critical: ['email', 'webhook'],
    info: ['email']
  }
});

const templates = ref<AlertTemplate[]>([]);
const receivers = ref<AlertReceiver[]>([]);

const templateDialogVisible = ref(false);
const templateForm = ref<Partial<AlertTemplate>>({});
const receiverDialogVisible = ref(false);
const receiverForm = ref<Partial<AlertReceiver>>({ levels: ['emergency'], serverGroups: [] });
const receiverEditingId = ref<string | null>(null);

const unwrap = <T>(resp: any): T => (resp?.data?.data ?? resp?.data ?? resp) as T;

const statCards = computed(() => [
  { key: 'tab', title: t('monitoring.config.statCurrentTab'), value: activeTabLabel.value, sub: t('monitoring.config.statCurrentTabSub') },
  { key: 'templates', title: t('monitoring.config.statTemplates'), value: templates.value.length, sub: t('monitoring.config.statTemplatesSub') },
  { key: 'receivers', title: t('monitoring.config.statReceivers'), value: receivers.value.length, sub: t('monitoring.config.statReceiversSub') },
  { key: 'critical', title: t('monitoring.config.statCritical'), value: notify.value.policies.emergency.length, sub: t('monitoring.config.statCriticalSub') }
]);

const filteredReceivers = computed(() => {
  const kw = receiverKeyword.value.trim().toLowerCase();
  if (!kw) return receivers.value;
  return receivers.value.filter((item) =>
    [item.name, item.email, item.department || ''].join(' ').toLowerCase().includes(kw)
  );
});

const loadConfig = async () => {
  loading.value = true;
  try {
    const [thresholdResp, notifyResp, templateResp, receiverResp] = await Promise.all([
      getAlertThresholdConfig(),
      getAlertNotifyConfig(),
      listAlertTemplates(),
      listAlertReceivers()
    ]);
    thresholds.value = unwrap<AlertThresholdConfig>(thresholdResp) || thresholds.value;
    notify.value = unwrap<AlertNotifyConfig>(notifyResp) || notify.value;
    templates.value = unwrap<AlertTemplate[]>(templateResp) || [];
    receivers.value = unwrap<AlertReceiver[]>(receiverResp) || [];
  } catch (err) {
    showHttpError(err, t('monitoring.config.loadConfigFail'));
  } finally {
    loading.value = false;
  }
};

const saveThresholds = async () => {
  await saveAlertThresholdConfig(thresholds.value);
  showSuccess(t('monitoring.config.thresholdsSaved'));
};

const saveNotify = async () => {
  await saveAlertNotifyConfig(notify.value);
  showSuccess(t('monitoring.config.notifySaved'));
};

const saveCurrent = async () => {
  saving.value = true;
  try {
    if (activeTab.value === 'thresholds') await saveThresholds();
    if (activeTab.value === 'notify') await saveNotify();
    if (activeTab.value === 'receivers') showSuccess(t('monitoring.config.receiverAutoSave'));
  } catch (err) {
    showHttpError(err, t('monitoring.config.saveConfigFail'));
  } finally {
    saving.value = false;
  }
};

const addTemplate = () => {
  templateForm.value = { name: '', lang: 'zh-CN', channel: 'email', subject: '', body: '' };
  templateDialogVisible.value = true;
};

const openEditTemplate = (tpl: AlertTemplate) => {
  templateForm.value = { ...tpl };
  templateDialogVisible.value = true;
};

const removeTemplate = async (tpl: AlertTemplate) => {
  await ElMessageBox.confirm(t('monitoring.config.deleteTemplateConfirm', { name: tpl.name }), t('monitoring.config.deleteTemplateTitle'), { type: 'warning' });
  try {
    await deleteAlertTemplate(tpl.id);
    showSuccess(t('monitoring.config.templateDeleted'));
    await loadConfig();
  } catch (err) {
    showHttpError(err, t('monitoring.config.deleteTemplateFail'));
  }
};

const saveTemplate = async () => {
  try {
    if (templateForm.value.id) {
      await updateAlertTemplate(templateForm.value.id, templateForm.value);
    } else {
      await createAlertTemplate(templateForm.value);
    }
    templateDialogVisible.value = false;
    showSuccess(t('monitoring.config.templateSaved'));
    await loadConfig();
  } catch (err) {
    showHttpError(err, t('monitoring.config.saveTemplateFail'));
  }
};

const resetReceiverForm = () => {
  receiverForm.value = {
    name: '',
    email: '',
    phone: '',
    levels: ['emergency'],
    department: '',
    schedule: '',
    serverGroups: []
  };
};

const addReceiver = () => {
  resetReceiverForm();
  receiverEditingId.value = null;
  receiverDialogVisible.value = true;
};

const editReceiver = (receiver: AlertReceiver) => {
  receiverEditingId.value = receiver.id;
  receiverForm.value = { ...receiver, serverGroups: receiver.serverGroups ? [...receiver.serverGroups] : [] };
  receiverDialogVisible.value = true;
};

const removeReceiver = async (receiver: AlertReceiver) => {
  await ElMessageBox.confirm(t('monitoring.config.deleteReceiverConfirm', { name: receiver.name }), t('monitoring.config.deleteReceiverTitle'), { type: 'warning' });
  try {
    await deleteAlertReceiver(receiver.id);
    showSuccess(t('monitoring.config.receiverDeleted'));
    await loadConfig();
  } catch (err) {
    showHttpError(err, t('monitoring.config.deleteReceiverFail'));
  }
};

const saveReceiver = async () => {
  if (!receiverForm.value.name || !receiverForm.value.email) {
    showError(t('monitoring.config.receiverRequired'));
    return;
  }
  try {
    if (receiverEditingId.value) {
      await updateAlertReceiver(receiverEditingId.value, receiverForm.value);
    } else {
      await createAlertReceiver(receiverForm.value);
    }
    receiverDialogVisible.value = false;
    showSuccess(t('monitoring.config.receiverSaved'));
    await loadConfig();
  } catch (err) {
    showHttpError(err, t('monitoring.config.saveReceiverFail'));
  }
};

const formatLevels = (levels: AlertReceiver['levels']) =>
  levels
    .map((level) => {
      if (level === 'emergency') return t('monitoring.config.formatLevelEmergency');
      if (level === 'critical') return t('monitoring.config.formatLevelCritical');
      if (level === 'info') return t('monitoring.config.formatLevelInfo');
      return t('monitoring.config.formatLevelAll');
    })
    .join(' / ');

const pickFile = () => {
  const input = document.createElement('input');
  input.type = 'file';
  input.accept = '.csv,.xlsx,.xls';
  input.onchange = async () => {
    if (!input.files || input.files.length === 0) return;
    const form = new FormData();
    form.append('file', input.files[0]);
    try {
      await importAlertReceivers(form);
      showSuccess(t('monitoring.config.importReceiverSuccess'));
      await loadConfig();
    } catch (err) {
      showHttpError(err, t('monitoring.config.importReceiverFail'));
    }
  };
  input.click();
};

const exportReceiversAction = async () => {
  try {
    const resp = await exportAlertReceivers();
    const blob = resp?.data instanceof Blob ? resp.data : new Blob([resp as any], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'alert_receivers.csv';
    link.click();
    URL.revokeObjectURL(url);
    showSuccess(t('monitoring.config.exportReceiverSuccess'));
  } catch (err) {
    showHttpError(err, t('monitoring.config.exportReceiverFail'));
  }
};

onMounted(() => {
  const tab = String(route.query.tab || '').trim();
  if (tab === 'thresholds' || tab === 'notify' || tab === 'receivers') {
    activeTab.value = tab;
  }
  loadConfig();
});
</script>

<style scoped>
.page-wrap {
  --page-bg: #f5f7fa;
  --surface-bg: #ffffff;
  --surface-border: #ebeef5;
  --text-primary: #111827;
  --text-secondary: #6b7280;
  --text-tertiary: #9ca3af;
  padding: 12px;
  background: var(--page-bg);
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.surface-card {
  border: 1px solid var(--surface-border);
  border-radius: 4px;
  background: var(--surface-bg);
  color: var(--text-primary);
}

.page-header {
  padding: 12px 16px;
}

.page-header h3 {
  margin: 0;
}

.desc {
  margin: 4px 0 0;
  color: var(--text-secondary);
}

.filter-bar {
  height: 48px;
  padding: 8px 16px;
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 8px;
}

.bar-left,
.bar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.stat-card {
  border: 1px solid var(--surface-border);
}

.stat-title {
  font-size: 13px;
  color: var(--text-secondary);
}

.stat-value {
  margin-top: 6px;
  font-size: 24px;
  font-weight: 700;
}

.stat-sub {
  margin-top: 6px;
  font-size: 12px;
  color: var(--text-tertiary);
}

.content-card :deep(.el-card__body) {
  padding: 16px;
}

.tabs :deep(.el-tabs__header) {
  margin-bottom: 12px;
}

.threshold-layout {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.notify-layout {
  display: grid;
  grid-template-columns: minmax(280px, 1fr) minmax(280px, 1fr) minmax(480px, 1.4fr);
  gap: 12px;
}

.inner-card {
  border: 1px solid var(--surface-border);
  background: var(--surface-bg);
}

.inner-title {
  font-weight: 600;
}

.inner-title-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.receiver-toolbar {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}

.toolbar-left,
.toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.search {
  width: 260px;
}

.filter-bar :deep(.el-input__wrapper),
.filter-bar :deep(.el-select__wrapper) {
  min-height: 32px;
}

.filter-bar :deep(.el-button) {
  height: 32px;
}

@media (max-width: 1500px) {
  .notify-layout {
    grid-template-columns: 1fr;
  }
}
</style>