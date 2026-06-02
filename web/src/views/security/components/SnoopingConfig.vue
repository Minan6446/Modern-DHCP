<template>
  <el-card v-loading="loading" shadow="never">
    <template #header>
      <div class="card-header">
        <span>{{ t('security.snoopingTitle') }}</span>
        <div class="actions">
          <el-button size="small" :disabled="!canView || loading" @click="loadAll">{{
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
      <el-col :span="12">
        <div class="section-title">{{ t('security.trustPortTitle') }}</div>
        <el-table :data="ports" size="small" border height="220">
          <el-table-column prop="device" :label="t('security.comp.snoopColDevice')" width="120" />
          <el-table-column prop="port" :label="t('security.comp.snoopColPort')" width="100" />
          <el-table-column prop="vlan" label="VLAN" width="80" />
          <el-table-column :label="t('security.comp.snoopColTrust')" width="90">
            <template #default="{ row }">
              <el-switch v-model="row.trusted" size="small" />
            </template>
          </el-table-column>
          <el-table-column :label="t('security.comp.snoopColRate')" width="110">
            <template #default="{ row }">
              <el-input-number v-model="row.rateLimitPps" :min="0" :max="2000" size="small" />
            </template>
          </el-table-column>
        </el-table>
      </el-col>
      <el-col :span="12">
        <div class="section-title">{{ t('security.bindingViewer') }}</div>
        <el-table :data="bindings" size="small" border height="220">
          <el-table-column prop="mac" label="MAC" width="130" />
          <el-table-column prop="ip" label="IP" width="130" />
          <el-table-column prop="vlan" label="VLAN" width="80" />
          <el-table-column prop="port" :label="t('security.comp.snoopColBindPort')" width="100" />
          <el-table-column prop="source" :label="t('security.comp.snoopColSource')" width="100" />
        </el-table>
        <div class="pager">
          <el-pagination
            layout="prev, pager, next"
            :page-size="query.pageSize"
            :current-page="query.page"
            :total="total"
            @current-change="onPage"
          />
        </div>
      </el-col>
    </el-row>
    <el-divider />
    <div class="section-title">{{ t('security.violationPolicy') }}</div>
    <el-form :model="policy" inline>
      <el-form-item :label="t('security.violationAction')">
        <el-select v-model="policy.action" style="width: 180px">
          <el-option :label="t('security.drop')" value="drop" />
          <el-option :label="t('security.shutdown')" value="shutdown" />
          <el-option :label="t('security.alert')" value="alert" />
          <el-option :label="t('security.rateLimit')" value="rate-limit" />
        </el-select>
      </el-form-item>
      <el-form-item v-if="policy.action === 'shutdown'" :label="t('security.blockDuration')">
        <el-input-number v-model="policy.blockDurationSeconds" :min="0" />
      </el-form-item>
      <el-form-item v-if="policy.action === 'alert'" :label="t('security.alertChannels')">
        <el-select v-model="policy.alertChannels" multiple filterable style="width: 260px">
          <el-option label="Email" value="email" />
          <el-option label="Webhook" value="webhook" />
          <el-option label="Syslog" value="syslog" />
        </el-select>
      </el-form-item>
    </el-form>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import {
  listTrustPorts,
  listSnoopingBindings,
  saveTrustPorts,
  saveViolationPolicy
} from '@/api/security';
import type { TrustPort, SnoopingBinding, ViolationPolicy } from '@/types/security';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { ElMessageBox } from 'element-plus';
import { showHttpError } from '@/shared/errors/errorToast';
import { showError, showSuccess } from '@/shared/errors/messageToast';
import { useI18n } from 'vue-i18n';

const ports = ref<TrustPort[]>([]);
const bindings = ref<SnoopingBinding[]>([]);
const query = ref({ page: 1, pageSize: 8 });
const total = ref(0);
const policy = ref<ViolationPolicy>({ action: 'drop', alertChannels: [] });
const loading = ref(false);
const error = ref('');
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const { t } = useI18n();
const canView = ref(false);
const canManage = ref(false);

const loadPorts = async () => {
  try {
    const { data } = await listTrustPorts({ tenantId: tenantStore.currentTenantId });
    ports.value = data.data;
  } catch (e) {
    error.value = t('security.loadFailPort');
    showError(error.value);
  }
};

const loadBindings = async () => {
  try {
    const { data } = await listSnoopingBindings({
      ...query.value,
      tenantId: tenantStore.currentTenantId
    });
    bindings.value = data.data.items;
    total.value = data.data.total;
  } catch (e) {
    error.value = t('security.loadFailSource');
    bindings.value = [];
    total.value = 0;
    showError(error.value);
  }
};

const save = async () => {
  if (!canManage.value) {
    showError(t('security.noPermissionSave'));
    return;
  }
  if (!policy.value.action) {
    showError(t('security.chooseAction'));
    return;
  }
  try {
    await ElMessageBox.confirm(t('security.saveConfirm'), t('common.confirm'), { type: 'warning' });
  } catch {
    return;
  }
  try {
    await saveTrustPorts(ports.value, { tenantId: tenantStore.currentTenantId });
    await saveViolationPolicy({ ...policy.value, tenantId: tenantStore.currentTenantId });
    showSuccess(t('security.saved'));
    loadAll();
  } catch (e) {
    showHttpError(error, t('security.saveFail'));
  }
};

const onPage = (p: number) => {
  query.value.page = p;
  loadBindings();
};

const loadAll = async () => {
  if (!canView.value) {
    error.value = t('security.noPermissionView');
    ports.value = [];
    bindings.value = [];
    return;
  }
  loading.value = true;
  error.value = '';
  await Promise.all([loadPorts(), loadBindings()]);
  loading.value = false;
};

onMounted(() => {
  canView.value = permissionStore.can('security.view');
  canManage.value = permissionStore.can('security.manage');
  loadAll();
});
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

.section-title {
  font-weight: 600;
  margin-bottom: 6px;
}

.mb-2 {
  margin-bottom: 8px;
}

.pager {
  margin-top: 6px;
  display: flex;
  justify-content: flex-end;
}
</style>
