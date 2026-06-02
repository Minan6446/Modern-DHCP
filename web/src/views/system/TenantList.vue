<template>
  <div class="page">
    <div class="toolbar">
      <el-input
        v-model="keyword"
        :placeholder="t('system.tenant.search')"
        clearable
        @input="debouncedSearch"
      />
      <el-button type="primary" @click="openForm()">{{ t('system.tenant.create') }}</el-button>
    </div>
    <el-table v-loading="loading" :data="rows" border stripe height="100%">
      <el-table-column prop="name" :label="t('system.tenant.columns.name')" />
      <el-table-column prop="code" :label="t('system.tenant.columns.code')" />
      <el-table-column prop="status" :label="t('system.tenant.columns.status')" />
      <el-table-column prop="quota.pools" :label="t('system.tenant.columns.pools')" />
      <el-table-column prop="quota.leases" :label="t('system.tenant.columns.leases')" />
      <el-table-column fixed="right" :label="t('system.tenant.columns.actions')" width="160">
        <template #default="{ row }">
          <el-button size="small" text @click="openForm(row)">{{
            t('common.edit')
          }}</el-button>
          <el-popconfirm :title="t('system.tenant.deleteConfirm')" @confirm="remove(row.id)">
            <el-button size="small" text type="danger">{{ t('common.delete') }}</el-button>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>
    <div class="pager">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :total="total"
        layout="prev, pager, next, jumper"
        @current-change="fetch"
        @size-change="fetch"
      />
    </div>

    <el-drawer v-model="drawer.visible" :title="drawer.title" size="40%">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px">
        <el-form-item :label="t('common.name')" prop="name"
          ><el-input v-model="form.name"
        /></el-form-item>
        <el-form-item :label="t('common.code')" prop="code"
          ><el-input v-model="form.code"
        /></el-form-item>
        <el-form-item :label="t('common.status')" prop="status">
          <el-select v-model="form.status">
            <el-option value="active" :label="t('system.tenant.form.statusActive')" />
            <el-option value="disabled" :label="t('system.tenant.form.statusDisabled')" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('system.tenant.form.quotaPools')" prop="quota.pools"
          ><el-input-number v-model="form.quota.pools"
        /></el-form-item>
        <el-form-item :label="t('system.tenant.form.quotaLeases')" prop="quota.leases"
          ><el-input-number v-model="form.quota.leases"
        /></el-form-item>
        <el-form-item :label="t('system.tenant.form.quotaUsers')" prop="quota.users"
          ><el-input-number v-model="form.quota.users"
        /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="drawer.visible = false">{{ t('common.cancel') }}</el-button>
        <el-button
          type="primary"
          :loading="actionLoading"
          :disabled="actionLoading"
          @click="submit"
          >{{ t('common.save') }}</el-button
        >
      </template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue';
import { listTenants, createTenant, updateTenant, deleteTenant } from '@/api/system/tenant';
import type { TenantDetail } from '@/types/system';
import type { FormInstance, FormRules } from 'element-plus';
import { useDebounceFn } from '@vueuse/core';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { useI18n } from 'vue-i18n';
import { useSystemViewStore } from '@/store/systemView';
import { showHttpError } from '@/shared/errors/errorToast';
import { showSuccess } from '@/shared/errors/messageToast';

const rows = ref<TenantDetail[]>([]);
const loading = ref(false);
const actionLoading = ref(false);
const total = ref(0);
const systemView = useSystemViewStore();
const page = ref(systemView.tenant.page);
const pageSize = ref(systemView.tenant.pageSize);
const keyword = ref(systemView.tenant.keyword);
const { t } = useI18n();
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();

const drawer = reactive({ visible: false, title: t('system.tenant.create') });
const formRef = ref<FormInstance>();
const form = reactive<TenantDetail>({
  id: '',
  name: '',
  code: '',
  status: 'active',
  quota: { pools: 10, leases: 1000, users: 50 }
});

const rules: FormRules = {
  name: [
    { required: true, message: t('common.required'), trigger: 'blur' },
    { min: 2, max: 50, message: t('common.lengthLimit'), trigger: 'blur' }
  ],
  code: [
    { required: true, message: t('common.required'), trigger: 'blur' },
    {
      pattern: /^[a-z0-9-]{2,32}$/,
      message: t('common.codeRule'),
      trigger: 'blur'
    }
  ],
  quota: {
    pools: [
      {
        validator: (_: unknown, val: number, cb: (e?: Error) => void) => {
          if (val > 0) return cb();
          cb(new Error(t('common.gtZero')));
        },
        trigger: 'change'
      }
    ],
    leases: [
      {
        validator: (_: unknown, val: number, cb: (e?: Error) => void) => {
          if (val > 0) return cb();
          cb(new Error(t('common.gtZero')));
        },
        trigger: 'change'
      }
    ],
    users: [
      {
        validator: (_: unknown, val: number, cb: (e?: Error) => void) => {
          if (val > 0) return cb();
          cb(new Error(t('common.gtZero')));
        },
        trigger: 'change'
      }
    ]
  }
};

const fetch = async () => {
  if (!permissionStore.can('__role_admin__')) return;
  loading.value = true;
  try {
    systemView.setTenant({ page: page.value, pageSize: pageSize.value, keyword: keyword.value });
    const { data } = await listTenants({
      page: page.value,
      pageSize: pageSize.value,
      keyword: keyword.value
    });
    rows.value = data.data.items;
    total.value = data.data.total;
  } finally {
    loading.value = false;
  }
};

const debouncedSearch = useDebounceFn(() => {
  page.value = 1;
  fetch();
}, 250);

function openForm(row?: TenantDetail) {
  if (row) {
    Object.assign(form, row);
    drawer.title = t('system.tenant.edit');
  } else {
    Object.assign(form, {
      id: '',
      name: '',
      code: '',
      status: 'active',
      quota: { pools: 10, leases: 1000, users: 50 }
    });
    drawer.title = t('system.tenant.create');
  }
  drawer.visible = true;
}

const submit = () => {
  formRef.value?.validate(async (valid: boolean) => {
    if (!valid) return;
    actionLoading.value = true;
    try {
      if (form.id) {
        await updateTenant(form.id, form);
        showSuccess(t('system.tenant.updated'));
      } else {
        await createTenant(form);
        showSuccess(t('system.tenant.created'));
      }
      drawer.visible = false;
      fetch();
    } catch (e) {
      showHttpError(e, t('common.actionFailed'));
    } finally {
      actionLoading.value = false;
    }
  });
};

const remove = async (id: string) => {
  actionLoading.value = true;
  try {
    await deleteTenant(id);
    showSuccess(t('system.tenant.deleted'));
    fetch();
  } catch (e) {
    showHttpError(e, t('common.actionFailed'));
  } finally {
    actionLoading.value = false;
  }
};

onMounted(fetch);

watch(
  () => tenantStore.currentTenantId,
  () => {
    page.value = 1;
    fetch();
  }
);
</script>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: 100%;
}

.toolbar {
  display: flex;
  gap: 8px;
}

.pager {
  display: flex;
  justify-content: flex-end;
  padding: 8px 0;
}
</style>
