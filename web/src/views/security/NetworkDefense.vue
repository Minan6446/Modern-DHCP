<template>
  <div class="security-network page-block">
    <section class="global-control-bar">
      <div class="global-control-text">
        <h2>{{ t('security.network.pageTitle') }}</h2>
        <p>{{ t('security.network.pageSubtitle') }}</p>
      </div>
      <div class="global-control-actions">
        <div class="global-switch-wrap">
          <span class="switch-label">{{ t('security.network.globalSwitch') }}</span>
          <el-switch v-model="globalProtectionEnabled" size="large" />
        </div>
        <el-button size="small" :loading="loading" @click="loadAll">{{ t('security.network.refresh') }}</el-button>
      </div>
    </section>

    <section class="stats-grid">
      <el-card shadow="never" class="status-card" :class="{ 'is-off': !daiStatusOn }">
        <div class="status-card-body">
          <div class="status-main">
            <p>{{ t('security.network.daiStatusTitle') }}</p>
            <el-tag :type="daiStatusOn ? 'success' : 'danger'" effect="dark" size="large">
              {{ daiStatusOn ? 'ON' : 'OFF' }}
            </el-tag>
          </div>
          <div class="status-sub">{{ t('security.network.blockedCount', { count: daiBlockedCount }) }}</div>
        </div>
      </el-card>
      <el-card shadow="never" class="status-card" :class="{ 'is-off': !sourceGuardStatusOn }">
        <div class="status-card-body">
          <div class="status-main">
            <p>{{ t('security.network.sourceGuardStatusTitle') }}</p>
            <el-tag :type="sourceGuardStatusOn ? 'success' : 'danger'" effect="dark" size="large">
              {{ sourceGuardStatusOn ? 'ON' : 'OFF' }}
            </el-tag>
          </div>
          <div class="status-sub">{{ t('security.network.blockedCount', { count: sourceGuardBlockedCount }) }}</div>
        </div>
      </el-card>
      <el-card shadow="never" class="status-card">
        <div class="status-card-body">
          <div class="status-main">
            <p>{{ t('security.network.portProfileCountTitle') }}</p>
            <span class="count-value">{{ portProfiles.length }}</span>
          </div>
        </div>
      </el-card>
    </section>

    <section class="card-container">
      <div class="panel-column">
        <el-collapse v-model="leftPanels" class="feature-collapse">
          <el-collapse-item name="dai">
          <template #title>
            <div class="collapse-title-wrap">
              <span>{{ t('security.network.daiCollapseTitle') }}</span>
              <el-switch
                v-model="daiConfig.enabled"
                :disabled="globalDisabled"
                @click.stop
                @change="saveDai"
              />
            </div>
          </template>
          <el-form label-position="top" :disabled="globalDisabled || !daiConfig.enabled">
            <el-form-item :label="t('security.validateMac')">
              <el-switch v-model="daiConfig.validateMac" />
            </el-form-item>
            <el-form-item :label="t('security.validateIp')">
              <el-switch v-model="daiConfig.validateIp" />
            </el-form-item>
            <el-form-item :label="t('security.ppsLimit')">
              <el-input-number v-model="daiConfig.rateLimitPps" :min="0" />
            </el-form-item>
            <el-form-item>
              <el-button
                type="primary"
                size="small"
                :disabled="globalDisabled || !daiConfig.enabled"
                @click="saveDai"
                >{{ t('common.save') }}</el-button
              >
            </el-form-item>
          </el-form>
          </el-collapse-item>

          <el-collapse-item name="port-security">
            <template #title>
              <div class="collapse-title-wrap">
                <span>{{ t('security.network.portCollapseTitle') }}</span>
                <el-button
                  size="small"
                  type="primary"
                  :disabled="globalDisabled"
                  @click.stop="openProfileDialog()"
                  >{{ t('security.network.addBtn') }}</el-button
                >
              </div>
            </template>
            <div class="table-zone" :class="{ 'is-disabled': globalDisabled }">
              <el-table :data="portProfiles" border stripe size="small">
                <template v-if="!portProfiles.length" #empty>
                  <div class="empty-inline">
                    <el-empty
                      :image-size="0"
                      :description="t('security.network.portProfileEmpty')"
                    />
                  </div>
                </template>
              <el-table-column prop="name" :label="t('security.name')" min-width="160" />
              <el-table-column prop="maxMacs" :label="t('security.maxMacs')" width="130" />
              <el-table-column prop="sticky" :label="t('security.sticky')" width="120">
                <template #default="{ row }">
                  <el-tag :type="row.sticky ? 'success' : 'info'">{{
                    row.sticky ? t('common.yes') : t('common.no')
                  }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column
                prop="shutdownOnViolation"
                :label="t('security.shutdown')"
                width="150"
              >
                <template #default="{ row }">
                  <el-tag :type="row.shutdownOnViolation ? 'danger' : 'info'">
                    {{ row.shutdownOnViolation ? t('security.alert') : t('security.allow') }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="agingMinutes" label="Aging" width="120" />
              <el-table-column :label="t('common.actions')" width="180">
                <template #default="{ row }">
                  <el-button
                    size="small"
                    text
                    :disabled="globalDisabled"
                    @click="openProfileDialog(row)"
                    >{{ t('common.edit') }}</el-button
                  >
                  <el-button
                    size="small"
                    text
                    type="danger"
                    :disabled="globalDisabled"
                    @click="deleteProfile(row)"
                    >{{ t('common.delete') }}</el-button
                  >
                </template>
              </el-table-column>
              </el-table>
            </div>
          </el-collapse-item>
        </el-collapse>
      </div>

      <div class="panel-column">
        <el-collapse v-model="rightPanels" class="feature-collapse">
          <el-collapse-item name="source-guard">
          <template #title>
            <div class="collapse-title-wrap">
              <span>{{ t('security.network.sourceGuardCollapseTitle') }}</span>
              <el-switch
                v-model="sourceGuard.enabled"
                :disabled="globalDisabled"
                @click.stop
                @change="saveSourceGuard"
              />
            </div>
          </template>
          <el-form label-position="top" :disabled="globalDisabled || !sourceGuard.enabled">
            <el-form-item :label="t('security.defaultAction')">
              <el-radio-group v-model="sourceGuard.defaultAction">
                <el-radio label="permit">{{ t('security.allow') }}</el-radio>
                <el-radio label="deny">{{ t('security.deny') }}</el-radio>
              </el-radio-group>
            </el-form-item>
            <div class="exception-header">
              <span>{{ t('security.exceptionList') }}</span>
              <el-button
                size="small"
                type="primary"
                :disabled="globalDisabled || !sourceGuard.enabled"
                @click="addException"
                >{{ t('security.add') }}</el-button
              >
            </div>
            <div class="table-zone" :class="{ 'is-disabled': globalDisabled || !sourceGuard.enabled }">
              <el-table :data="sourceGuard.exceptions" border size="small" stripe>
                <template v-if="!sourceGuard.exceptions?.length" #empty>
                  <div class="empty-inline">
                    <el-empty :image-size="0" :description="t('security.network.exceptionEmpty')" />
                    <el-button
                      size="small"
                      type="primary"
                      :disabled="globalDisabled || !sourceGuard.enabled"
                      @click="addException"
                      >{{ t('security.network.addException') }}</el-button
                    >
                  </div>
                </template>
                <el-table-column prop="mac" label="MAC" min-width="160">
                  <template #default="{ row }">
                    <el-input v-model="row.mac" size="small" />
                  </template>
                </el-table-column>
                <el-table-column prop="ip" label="IP" min-width="160">
                  <template #default="{ row }">
                    <el-input v-model="row.ip" size="small" />
                  </template>
                </el-table-column>
                <el-table-column prop="vlan" label="VLAN" width="120">
                  <template #default="{ row }">
                    <el-input-number v-model="row.vlan" :min="1" />
                  </template>
                </el-table-column>
                <el-table-column prop="port" label="Port" width="140">
                  <template #default="{ row }">
                    <el-input v-model="row.port" size="small" />
                  </template>
                </el-table-column>
                <el-table-column :label="t('common.actions')" width="120">
                  <template #default="{ $index }">
                    <el-button
                      size="small"
                      text
                      type="danger"
                      :disabled="globalDisabled || !sourceGuard.enabled"
                      @click="removeException($index)"
                      >{{ t('security.remove') }}</el-button
                    >
                  </template>
                </el-table-column>
              </el-table>
            </div>
            <el-form-item>
              <el-button
                type="primary"
                size="small"
                :disabled="globalDisabled || !sourceGuard.enabled"
                @click="saveSourceGuard"
                >{{ t('common.save') }}</el-button
              >
            </el-form-item>
          </el-form>
          </el-collapse-item>

          <el-collapse-item name="recent-records">
            <template #title>
              <div class="collapse-title-wrap">
                <span>{{ t('security.network.timelineCollapseTitle') }}</span>
                <el-button
                  size="small"
                  text
                  :loading="timelineLoading"
                  :disabled="globalDisabled"
                  @click.stop="loadTimeline"
                  >{{ t('security.network.timelineRefresh') }}</el-button
                >
              </div>
            </template>
            <div class="table-zone" :class="{ 'is-disabled': globalDisabled }">
              <el-table :data="timeline" border stripe size="small">
                <template v-if="!timeline.length" #empty>
                  <div class="empty-inline">
                    <el-empty
                      :image-size="0"
                      :description="t('security.network.timelineEmptyDetail')"
                    />
                  </div>
                </template>
              <el-table-column prop="occurredAt" :label="t('security.network.colTime')" min-width="170">
                <template #default="{ row }">{{ formatTs(row.occurredAt) }}</template>
              </el-table-column>
              <el-table-column prop="type" :label="t('security.network.colEvent')" min-width="140" />
              <el-table-column prop="description" :label="t('security.network.colDescription')" min-width="220" />
              <el-table-column prop="sourceIp" :label="t('security.network.colSourceIp')" width="140" />
              <el-table-column prop="port" :label="t('security.network.colPort')" width="110" />
              <el-table-column prop="vlan" label="VLAN" width="90" />
              </el-table>
            </div>
          </el-collapse-item>
        </el-collapse>
      </div>
    </section>

    <el-dialog
      v-model="profileDialogVisible"
      width="520px"
      :title="profileForm.id ? t('common.edit') : t('security.network.portProfileAdd')"
    >
      <el-form label-width="120px" :disabled="globalDisabled">
        <el-form-item :label="t('security.name')">
          <el-input v-model="profileForm.name" />
        </el-form-item>
        <el-form-item :label="t('security.maxMacs')">
          <el-input-number v-model="profileForm.maxMacs" :min="1" />
        </el-form-item>
        <el-form-item :label="t('security.sticky')">
          <el-switch v-model="profileForm.sticky" />
        </el-form-item>
        <el-form-item :label="t('security.shutdown')">
          <el-switch v-model="profileForm.shutdownOnViolation" />
        </el-form-item>
        <el-form-item label="Aging (min)">
          <el-input-number v-model="profileForm.agingMinutes" :min="0" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="profileDialogVisible = false">{{ t('common.cancel') }}</el-button>
          <el-button
            type="primary"
            :loading="profileSaving"
            :disabled="globalDisabled"
            @click="saveProfile"
            >{{ t('common.save') }}</el-button
          >
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { ElMessageBox } from 'element-plus';
import { showHttpError } from '@/shared/errors/errorToast';
import { showSuccess } from '@/shared/errors/messageToast';
import { useTenantStore } from '@/store/tenant';
import {
  getDAIConfig,
  updateDAIConfig,
  getSourceGuard,
  updateSourceGuard,
  listPortProfiles,
  savePortProfile,
  deletePortProfile,
  listThreatEvents
} from '@/api/security';
import type {
  DAIConfig,
  SourceGuardConfig,
  PortSecurityProfile,
  ThreatEvent
} from '@/types/security';
import { formatTs } from '@/utils/time';

const { t } = useI18n();
const tenantStore = useTenantStore();

const loading = ref(false);
const timelineLoading = ref(false);
const profileSaving = ref(false);
const globalProtectionEnabled = ref(true);

const globalDisabled = computed(() => !globalProtectionEnabled.value);
const leftPanels = ref(['dai', 'port-security']);
const rightPanels = ref(['source-guard', 'recent-records']);

const daiConfig = reactive<DAIConfig>({
  enabled: false,
  validateMac: true,
  validateIp: true,
  rateLimitPps: 600
});
const sourceGuard = reactive<SourceGuardConfig>({
  enabled: false,
  defaultAction: 'permit',
  exceptions: []
});
const portProfiles = ref<PortSecurityProfile[]>([]);
const timeline = ref<ThreatEvent[]>([]);
const allThreatEvents = ref<ThreatEvent[]>([]);

const profileDialogVisible = ref(false);
const profileForm = reactive<Partial<PortSecurityProfile>>({
  name: '',
  maxMacs: 10,
  sticky: true,
  shutdownOnViolation: true,
  agingMinutes: 1440
});

const daiBlockedCount = computed(
  () => allThreatEvents.value.filter((event) => /dai|arp/i.test(`${event.type} ${event.description || ''}`)).length
);

const sourceGuardBlockedCount = computed(
  () =>
    allThreatEvents.value.filter((event) =>
      /(source guard|ip source|source[-\s]?ip)/i.test(`${event.type} ${event.description || ''}`)
    ).length
);

const daiStatusOn = computed(() => globalProtectionEnabled.value && daiConfig.enabled);
const sourceGuardStatusOn = computed(() => globalProtectionEnabled.value && sourceGuard.enabled);

const tenantParam = () => ({ tenantId: tenantStore.currentTenantId || undefined });

const loadDai = async () => {
  try {
    const res = await getDAIConfig(tenantParam());
    Object.assign(daiConfig, res.data.data || {});
  } catch (error) {
    showHttpError(error, t('security.loadFailDai'));
  }
};

const loadSourceGuard = async () => {
  try {
    const res = await getSourceGuard(tenantParam());
    Object.assign(sourceGuard, res.data.data || {});
    sourceGuard.exceptions = res.data.data?.exceptions || [];
  } catch (error) {
    showHttpError(error, t('security.loadFailSource'));
  }
};

const loadPortProfiles = async () => {
  try {
    const res = await listPortProfiles(tenantParam());
    portProfiles.value = res.data.data || [];
  } catch (error) {
    showHttpError(error, t('security.loadFailPort'));
  }
};

const loadTimeline = async () => {
  timelineLoading.value = true;
  try {
    const res = await listThreatEvents(tenantParam());
    const events = res.data.data || [];
    allThreatEvents.value = events;
    timeline.value = events.slice(0, 6);
  } catch (error) {
    showHttpError(error, t('monitoring.loadFail'));
  } finally {
    timelineLoading.value = false;
  }
};

const loadAll = async () => {
  loading.value = true;
  await Promise.all([loadDai(), loadSourceGuard(), loadPortProfiles(), loadTimeline()]);
  loading.value = false;
};

const addException = () => {
  sourceGuard.exceptions?.push({ mac: '', ip: '', vlan: undefined, port: '' });
};

const removeException = (index: number) => {
  sourceGuard.exceptions?.splice(index, 1);
};

const saveDai = async () => {
  try {
    await updateDAIConfig({ ...daiConfig, ...tenantParam() });
    showSuccess(t('security.saved'));
  } catch (error) {
    showHttpError(error, t('security.saveFail'));
  }
};

const saveSourceGuard = async () => {
  try {
    await updateSourceGuard({ ...sourceGuard, ...tenantParam() });
    showSuccess(t('security.saved'));
  } catch (error) {
    showHttpError(error, t('security.saveFail'));
  }
};

const openProfileDialog = (profile?: PortSecurityProfile) => {
  if (profile) {
    Object.assign(profileForm, profile);
  } else {
    Object.assign(profileForm, {
      id: undefined,
      name: '',
      maxMacs: 10,
      sticky: true,
      shutdownOnViolation: true,
      agingMinutes: 1440
    });
  }
  profileDialogVisible.value = true;
};

const saveProfile = async () => {
  profileSaving.value = true;
  try {
    await savePortProfile({ ...profileForm, ...tenantParam() });
    showSuccess(t('security.saved'));
    profileDialogVisible.value = false;
    await loadPortProfiles();
  } catch (error) {
    showHttpError(error, t('security.saveFail'));
  } finally {
    profileSaving.value = false;
  }
};

const deleteProfile = async (profile: PortSecurityProfile) => {
  try {
    await ElMessageBox.confirm(t('security.deleteConfirm'), t('common.delete'), {
      type: 'warning'
    });
    await deletePortProfile(profile.id, tenantParam());
    showSuccess(t('security.deleted'));
    await loadPortProfiles();
  } catch (error) {
    if (error !== 'cancel') showHttpError(error, t('security.deleteFail'));
  }
};

watch(
  () => tenantStore.currentTenantId,
  () => {
    loadAll();
  }
);

loadAll();
</script>

<style scoped>
.security-network {
  --security-page-bg: var(--el-fill-color-page);
  --security-card-bg: var(--el-bg-color);
  --security-border: var(--el-border-color-light);
  --security-text-title: var(--el-text-color-primary);
  display: flex;
  flex-direction: column;
  gap: 16px;
  background: var(--security-page-bg);
  padding: 16px;
  border-radius: 12px;
}

.global-control-bar {
  border: 1px solid var(--el-border-color-light);
  border-radius: 12px;
  background: var(--security-card-bg);
  padding: 16px 18px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  box-shadow: 0 2px 10px rgba(31, 45, 61, 0.06);
}

.global-control-text h2 {
  margin: 0;
  font-size: 20px;
}

.global-control-text p {
  margin: 6px 0 0;
  color: var(--el-text-color-secondary);
}

.global-control-actions {
  display: flex;
  align-items: center;
  gap: 14px;
}

.global-switch-wrap {
  display: flex;
  align-items: center;
  gap: 10px;
}

.switch-label {
  font-weight: 600;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}

.status-card {
  border-radius: 8px;
  background: var(--security-card-bg);
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.05);
  border: 1px solid var(--security-border);
}

.status-card.is-off {
  border-color: var(--el-color-danger-light-5);
  box-shadow: 0 2px 12px rgba(245, 108, 108, 0.15);
}

.status-card-body {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  justify-content: center;
  min-height: 88px;
  gap: 10px;
}

.status-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.status-sub {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.status-card-body p {
  margin: 0;
  color: var(--el-text-color-regular);
}

.count-value {
  font-size: 28px;
  line-height: 1;
  font-weight: 700;
}

.card-container {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  align-items: stretch;
}

.panel-column {
  display: flex;
  min-height: 100%;
}

.panel-column .feature-collapse {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.feature-collapse {
  border-top: none;
}

.feature-collapse :deep(.el-collapse-item) {
  border: 1px solid var(--security-border);
  border-radius: 8px;
  margin-bottom: 0;
  overflow: hidden;
  background: var(--security-card-bg);
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.05);
}

.feature-collapse :deep(.el-collapse-item__header) {
  padding: 0 20px;
  font-size: 16px;
  font-weight: 700;
  color: var(--security-text-title);
  background: var(--security-card-bg);
}

.feature-collapse :deep(.el-collapse-item__wrap) {
  border-bottom: none;
}

.feature-collapse :deep(.el-collapse-item__content) {
  padding: 20px;
}

.collapse-title-wrap {
  width: 100%;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  padding-right: 8px;
}

.collapse-title-wrap > span {
  font-size: 16px;
  font-weight: 700;
  color: var(--security-text-title);
}

.exception-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.table-zone.is-disabled {
  opacity: 0.55;
  pointer-events: none;
}

.empty-inline {
  padding: 10px 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

@media (max-width: 1200px) {
  .stats-grid {
    grid-template-columns: 1fr;
  }

  .card-container {
    grid-template-columns: 1fr;
  }

  .global-control-bar {
    flex-direction: column;
    align-items: flex-start;
  }
}

@media (max-width: 768px) {
  .stats-grid {
    grid-template-columns: 1fr;
  }
}
</style>
