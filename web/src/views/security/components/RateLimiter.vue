<template>
  <el-card v-loading="loading" shadow="never">
    <template #header>
      <div class="card-header">
        <span>{{ t('security.rateLimitTitle') }}</span>
        <div class="actions">
          <el-button size="small" :disabled="!canView || loading" @click="fetchRules">{{
            t('security.refresh')
          }}</el-button>
          <el-button
            size="small"
            type="primary"
            :disabled="!canManage || loading"
            @click="openCreate"
            >{{ t('security.add') }}</el-button
          >
        </div>
      </div>
    </template>
    <el-alert v-if="error" type="error" :closable="false" :title="error" class="mb-2" />
    <el-tabs v-model="tab" type="border-card" size="small">
      <el-tab-pane :label="t('security.comp.rateTabPort')" name="port">
        <el-table :data="portRules" border size="small" height="220">
          <el-table-column prop="target" :label="t('security.comp.rateColPort')" width="120" />
          <el-table-column prop="limitPps" label="PPS" width="100" />
          <el-table-column prop="burst" :label="t('security.comp.rateColBurst')" width="100" />
          <el-table-column prop="status" :label="t('security.comp.rateColStatus')" width="100" />
          <el-table-column :label="t('security.action')" width="120">
            <template #default="{ row }">
              <el-button link type="danger" :disabled="!canManage" @click="remove(row)">{{
                t('security.remove')
              }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
      <el-tab-pane :label="t('security.comp.rateTabMac')" name="mac">
        <el-table :data="macRules" border size="small" height="220">
          <el-table-column prop="target" label="MAC" width="180" />
          <el-table-column prop="limitPps" label="PPS" width="100" />
          <el-table-column prop="status" :label="t('security.comp.rateColStatus')" width="100" />
          <el-table-column :label="t('security.action')" width="120">
            <template #default="{ row }">
              <el-button link type="danger" :disabled="!canManage" @click="remove(row)">{{
                t('security.remove')
              }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
      <el-tab-pane :label="t('security.comp.rateTabDynamic')" name="dynamic">
        <el-table :data="dynamicRules" border size="small" height="220">
          <el-table-column prop="target" :label="t('security.comp.rateColScope')" width="160" />
          <el-table-column prop="limitPps" label="PPS" width="100" />
          <el-table-column prop="vlan" label="VLAN" width="100" />
          <el-table-column prop="status" :label="t('security.comp.rateColStatus')" width="100" />
          <el-table-column :label="t('security.action')" width="120">
            <template #default="{ row }">
              <el-button link type="danger" :disabled="!canManage" @click="remove(row)">{{
                t('security.remove')
              }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <el-dialog
      v-model="showDialog"
      :title="t('security.rateLimitCreate')"
      width="520px"
      destroy-on-close
    >
      <el-form :model="form" label-width="120px" label-position="left">
        <el-form-item :label="t('security.rateLimitScope')">
          <el-select v-model="form.scope" style="width: 180px">
            <el-option :label="t('security.rateLimitScopePort')" value="port" />
            <el-option :label="t('security.rateLimitScopeMac')" value="mac" />
            <el-option :label="t('security.rateLimitScopeDynamic')" value="dynamic" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('security.rateLimitTarget')">
          <el-input v-model="form.target" :placeholder="t('security.rateLimitTargetPlaceholder')" />
        </el-form-item>
        <el-form-item label="PPS">
          <el-input-number v-model="form.limitPps" :min="1" :max="5000" />
        </el-form-item>
        <el-form-item :label="t('security.rateLimitBurst')">
          <el-input-number v-model="form.burst" :min="0" :max="10000" />
        </el-form-item>
        <el-form-item v-if="form.scope !== 'mac'" label="VLAN">
          <el-input-number v-model="form.vlan" :min="1" :max="4094" />
        </el-form-item>
        <el-form-item :label="t('security.status')">
          <el-switch v-model="form.status" active-value="active" inactive-value="disabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :disabled="!canManage" @click="save">{{
          t('common.save')
        }}</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { deleteRateLimit, listRateLimits, saveRateLimit } from '@/api/security';
import type { RateLimitRule } from '@/types/security';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { ElMessageBox } from 'element-plus';
import { showHttpError } from '@/shared/errors/errorToast';
import { showError, showSuccess } from '@/shared/errors/messageToast';
import { useI18n } from 'vue-i18n';

const tab = ref<'port' | 'mac' | 'dynamic'>('port');
const rules = ref<RateLimitRule[]>([]);
const showDialog = ref(false);
const form = reactive({
  scope: 'port',
  target: '',
  limitPps: 500,
  burst: 100,
  vlan: undefined as number | undefined,
  status: 'active'
});
const loading = ref(false);
const error = ref('');
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const { t } = useI18n();
const canView = computed(() => permissionStore.can('security.view'));
const canManage = computed(() => permissionStore.can('security.manage'));

const fetchRules = async () => {
  if (!canView.value) {
    error.value = t('security.noPermissionView');
    rules.value = [];
    return;
  }
  loading.value = true;
  error.value = '';
  try {
    const { data } = await listRateLimits({
      page: 1,
      pageSize: 200,
      tenantId: tenantStore.currentTenantId
    });
    rules.value = data.data.items;
  } catch (e) {
    rules.value = [];
    error.value = t('security.loadFailRate');
    showError(error.value);
  } finally {
    loading.value = false;
  }
};

const portRules = computed(() => rules.value.filter((r) => r.scope === 'port' && !r.dynamic));
const macRules = computed(() => rules.value.filter((r) => r.scope === 'mac'));
const dynamicRules = computed(() => rules.value.filter((r) => r.dynamic || r.target.includes('*')));

const openCreate = () => {
  if (!canManage.value) {
    showError(t('security.noPermissionSave'));
    return;
  }
  showDialog.value = true;
};

const save = async () => {
  if (!canManage.value) {
    showError(t('security.noPermissionSave'));
    return;
  }
  if (!form.target || !form.limitPps) {
    showError(t('option.editorRequired'));
    return;
  }
  if (form.limitPps < 1) {
    showError(t('security.ppsInvalid'));
    return;
  }
  try {
    await saveRateLimit({
      ...(form as unknown as RateLimitRule),
      tenantId: tenantStore.currentTenantId
    });
    showSuccess(t('security.saved'));
    showDialog.value = false;
    fetchRules();
  } catch (e) {
    showHttpError(error, t('security.saveFail'));
  }
};

const remove = async (row: RateLimitRule) => {
  if (!canManage.value) {
    showError(t('security.noPermissionSave'));
    return;
  }
  try {
    await ElMessageBox.confirm(t('security.deleteConfirm'), t('common.confirm'), {
      type: 'warning'
    });
  } catch {
    return;
  }
  try {
    await deleteRateLimit(row.id, { tenantId: tenantStore.currentTenantId });
    showSuccess(t('security.deleted'));
    fetchRules();
  } catch (e) {
    showHttpError(error, t('security.deleteFail'));
  }
};

onMounted(fetchRules);
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

.mb-2 {
  margin-bottom: 8px;
}
</style>
