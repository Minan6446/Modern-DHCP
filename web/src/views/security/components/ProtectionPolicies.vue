<template>
  <el-card v-loading="loading" shadow="never">
    <template #header>
      <div class="card-header">
        <span>{{ t('security.protectionTitle') }}</span>
        <div class="actions">
          <el-button size="small" :disabled="!canView || loading" @click="load">{{
            t('security.refresh')
          }}</el-button>
          <el-button size="small" type="primary" :disabled="!canManage || loading" @click="save">{{
            t('security.save')
          }}</el-button>
        </div>
      </div>
    </template>
    <el-alert v-if="error" type="error" :closable="false" :title="error" class="mb-2" />
    <el-row :gutter="12">
      <el-col :span="8">
        <div class="section-title">{{ t('security.daiTitle') }}</div>
        <el-form :model="dai" label-width="120px" size="small">
          <el-form-item :label="t('security.enable')">
            <el-switch v-model="dai.enabled" :disabled="!canManage" />
          </el-form-item>
          <el-form-item :label="t('security.validateMac')">
            <el-switch v-model="dai.validateMac" :disabled="!canManage" />
          </el-form-item>
          <el-form-item :label="t('security.validateIp')">
            <el-switch v-model="dai.validateIp" :disabled="!canManage" />
          </el-form-item>
          <el-form-item :label="t('security.ppsLimit')">
            <el-input-number
              v-model="dai.rateLimitPps"
              :min="0"
              :max="5000"
              :disabled="!canManage"
            />
          </el-form-item>
        </el-form>
      </el-col>
      <el-col :span="8">
        <div class="section-title">{{ t('security.sourceTitle') }}</div>
        <el-form :model="sourceGuard" label-width="120px" size="small">
          <el-form-item :label="t('security.enable')">
            <el-switch v-model="sourceGuard.enabled" :disabled="!canManage" />
          </el-form-item>
          <el-form-item :label="t('security.defaultAction')">
            <el-select v-model="sourceGuard.defaultAction" :disabled="!canManage">
              <el-option :label="t('security.allow')" value="permit" />
              <el-option :label="t('security.deny')" value="deny" />
            </el-select>
          </el-form-item>
        </el-form>
        <el-alert type="info" :closable="false" :title="t('security.exceptionList')" class="mt-2" />
        <div class="exception-actions">
          <el-input
            v-model="newException.mac"
            size="small"
            placeholder="MAC"
            style="width: 120px"
            :disabled="!canManage"
          />
          <el-input
            v-model="newException.ip"
            size="small"
            placeholder="IP"
            style="width: 120px"
            :disabled="!canManage"
          />
          <el-input
            v-model="newException.port"
            size="small"
            :placeholder="t('security.comp.protectPhPort')"
            style="width: 120px"
            :disabled="!canManage"
          />
          <el-button size="small" type="primary" :disabled="!canManage" @click="addException">{{
            t('security.add')
          }}</el-button>
        </div>
        <el-table :data="sourceGuard.exceptions" size="small" border height="160">
          <el-table-column prop="mac" label="MAC" />
          <el-table-column prop="ip" label="IP" />
          <el-table-column prop="port" :label="t('security.comp.protectColPort')" />
          <el-table-column width="110" :label="t('security.action')">
            <template #default="{ row, $index }">
              <el-button link size="small" :disabled="!canManage" @click="editException($index)">{{
                t('common.edit')
              }}</el-button>
              <el-button
                link
                type="danger"
                size="small"
                :disabled="!canManage"
                @click="removeException($index)"
                >{{ t('security.remove') }}</el-button
              >
            </template>
          </el-table-column>
        </el-table>
      </el-col>
      <el-col :span="8">
        <div class="section-title">{{ t('security.portTitle') }}</div>
        <div class="exception-actions">
          <el-input
            v-model="newProfile.name"
            size="small"
            :placeholder="t('security.name')"
            style="width: 120px"
            :disabled="!canManage"
          />
          <el-input-number
            v-model="newProfile.maxMacs"
            size="small"
            :min="1"
            :max="64"
            :disabled="!canManage"
          />
          <el-checkbox v-model="newProfile.sticky" :disabled="!canManage">{{
            t('security.sticky')
          }}</el-checkbox>
          <el-checkbox v-model="newProfile.shutdownOnViolation" :disabled="!canManage">{{
            t('security.shutdown')
          }}</el-checkbox>
          <el-button size="small" type="primary" :disabled="!canManage" @click="addProfile">{{
            t('security.add')
          }}</el-button>
        </div>
        <el-table :data="portProfiles" size="small" border height="240">
          <el-table-column prop="name" :label="t('security.name')" />
          <el-table-column prop="maxMacs" :label="t('security.maxMacs')" width="90" />
          <el-table-column :label="t('security.sticky')" width="80">
            <template #default="{ row }">
              <el-tag :type="row.sticky ? 'success' : 'info'">{{
                row.sticky ? t('security.comp.protectStickyYes') : t('security.comp.protectStickyNo')
              }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="shutdownOnViolation" :label="t('security.shutdown')" width="110">
            <template #default="{ row }">
              <el-tag :type="row.shutdownOnViolation ? 'danger' : 'info'">{{
                row.shutdownOnViolation ? t('security.comp.protectStickyYes') : t('security.comp.protectStickyNo')
              }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column width="110" :label="t('security.action')">
            <template #default="{ row, $index }">
              <el-button link size="small" :disabled="!canManage" @click="editProfile($index)">{{
                t('common.edit')
              }}</el-button>
              <el-button
                link
                type="danger"
                size="small"
                :disabled="!canManage"
                @click="removeProfile($index)"
                >{{ t('security.remove') }}</el-button
              >
            </template>
          </el-table-column>
        </el-table>
      </el-col>
    </el-row>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import {
  deletePortProfile,
  getDAIConfig,
  getSourceGuard,
  listPortProfiles,
  savePortProfile,
  updateDAIConfig,
  updateSourceGuard
} from '@/api/security';
import type { DAIConfig, SourceGuardConfig, PortSecurityProfile } from '@/types/security';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { ElMessageBox } from 'element-plus';
import { showHttpError } from '@/shared/errors/errorToast';
import { showError, showSuccess } from '@/shared/errors/messageToast';
import { useI18n } from 'vue-i18n';
import { isIPv4, isIPv6 } from '@/utils/ip';
import { isMac } from '@/utils/mac';

const dai = ref<DAIConfig>({
  enabled: true,
  validateMac: true,
  validateIp: true,
  rateLimitPps: 500
});
const sourceGuard = ref<SourceGuardConfig>({
  enabled: true,
  defaultAction: 'deny',
  exceptions: []
});
const portProfiles = ref<PortSecurityProfile[]>([]);
const newException = ref({ mac: '', ip: '', port: '' });
const newProfile = ref({ name: '', maxMacs: 4, sticky: false, shutdownOnViolation: false });
const editingException = ref<number | null>(null);
const editingProfile = ref<number | null>(null);
const loading = ref(false);
const error = ref('');
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const { t } = useI18n();
const canView = computed(() => permissionStore.can('security.view'));
const canManage = computed(() => permissionStore.can('security.manage'));

const load = async () => {
  if (!canView.value) {
    error.value = t('security.noPermissionView');
    return;
  }
  loading.value = true;
  error.value = '';
  try {
    const { data } = await getDAIConfig({ tenantId: tenantStore.currentTenantId });
    dai.value = data.data;
  } catch (e) {
    error.value = t('security.loadFailDai');
    showError(error.value);
  }

  try {
    const { data } = await getSourceGuard({ tenantId: tenantStore.currentTenantId });
    sourceGuard.value = data.data;
  } catch (e) {
    error.value = error.value || t('security.loadFailSource');
    showHttpError(error, t('security.loadFailSource'));
  }

  try {
    const { data } = await listPortProfiles({ tenantId: tenantStore.currentTenantId });
    portProfiles.value = data.data;
  } catch (e) {
    error.value = error.value || t('security.loadFailPort');
    showHttpError(error, t('security.loadFailPort'));
  } finally {
    loading.value = false;
  }
};

const save = async () => {
  if (!canManage.value) {
    showError(t('security.noPermissionSave'));
    return;
  }
  if (!dai.value.rateLimitPps || dai.value.rateLimitPps < 0) {
    showError(t('security.ppsInvalid'));
    return;
  }
  if (!sourceGuard.value.defaultAction) {
    showError(t('security.chooseAction'));
    return;
  }
  try {
    await ElMessageBox.confirm(t('security.saveConfirm'), t('common.confirm'), { type: 'warning' });
  } catch {
    return;
  }
  try {
    await updateDAIConfig({ ...dai.value, tenantId: tenantStore.currentTenantId });
    await updateSourceGuard({ ...sourceGuard.value, tenantId: tenantStore.currentTenantId });
    showSuccess(t('security.saved'));
    load();
  } catch (e) {
    showHttpError(error, t('security.saveFail'));
  }
};

const addException = () => {
  if (!canManage.value) return;
  if (!newException.value.mac || !newException.value.ip || !newException.value.port) {
    showError(t('security.fillException'));
    return;
  }
  if (!isMac(newException.value.mac)) {
    showError(t('security.comp.protectMacFormatError'));
    return;
  }
  if (!isIPv4(newException.value.ip) && !isIPv6(newException.value.ip)) {
    showError(t('security.comp.protectIpFormatError'));
    return;
  }
  if (editingException.value !== null) {
    sourceGuard.value.exceptions.splice(editingException.value, 1, { ...newException.value });
  } else {
    sourceGuard.value.exceptions.push({ ...newException.value });
  }
  newException.value = { mac: '', ip: '', port: '' };
  editingException.value = null;
};

const removeException = (idx: number) => {
  if (!canManage.value) return;
  sourceGuard.value.exceptions.splice(idx, 1);
};

const editException = (idx: number) => {
  if (!canManage.value) return;
  const item = sourceGuard.value.exceptions[idx];
  if (!item) return;
  newException.value = { mac: item.mac, ip: item.ip, port: item.port ?? '' };
  editingException.value = idx;
};

const addProfile = async () => {
  if (!canManage.value) return;
  if (!newProfile.value.name) {
    showError(t('security.name') + ' ' + t('option.editorRequired'));
    return;
  }
  const entry = {
    id: `local-${Date.now()}`,
    name: newProfile.value.name,
    maxMacs: newProfile.value.maxMacs,
    sticky: newProfile.value.sticky,
    shutdownOnViolation: newProfile.value.shutdownOnViolation,
    agingMinutes: 1440
  };
  try {
    await savePortProfile({ ...entry, tenantId: tenantStore.currentTenantId });
    if (editingProfile.value !== null) {
      portProfiles.value.splice(editingProfile.value, 1, { ...entry });
    } else {
      portProfiles.value.push({ ...entry });
    }
    newProfile.value = { name: '', maxMacs: 4, sticky: false, shutdownOnViolation: false };
    editingProfile.value = null;
    showSuccess(t('security.saved'));
  } catch (e) {
    showHttpError(error, t('security.saveFail'));
  }
};

const removeProfile = (idx: number) => {
  if (!canManage.value) return;
  const item = portProfiles.value[idx];
  ElMessageBox.confirm(t('security.deleteConfirm'), t('common.confirm'), { type: 'warning' })
    .then(async () => {
      try {
        await deletePortProfile(item.id, { tenantId: tenantStore.currentTenantId });
        portProfiles.value.splice(idx, 1);
        showSuccess(t('security.deleted'));
      } catch (e) {
        showHttpError(error, t('security.deleteFail'));
      }
    })
    .catch(() => {});
};

const editProfile = (idx: number) => {
  if (!canManage.value) return;
  const item = portProfiles.value[idx];
  if (!item) return;
  newProfile.value = {
    name: item.name,
    maxMacs: item.maxMacs,
    sticky: item.sticky,
    shutdownOnViolation: item.shutdownOnViolation
  };
  editingProfile.value = idx;
};

onMounted(load);

watch(
  () => tenantStore.currentTenantId,
  () => {
    dai.value = { enabled: true, validateMac: true, validateIp: true, rateLimitPps: 500 };
    sourceGuard.value = { enabled: true, defaultAction: 'deny', exceptions: [] };
    portProfiles.value = [];
    load();
  }
);
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.actions {
  display: flex;
  gap: 8px;
}

.exception-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 8px 0;
}

.section-title {
  font-weight: 600;
  margin-bottom: 6px;
}

.mt-2 {
  margin-top: 8px;
}

.mb-2 {
  margin-bottom: 8px;
}
</style>
