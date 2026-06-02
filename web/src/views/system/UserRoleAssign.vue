<template>
  <div class="page surface-card">
    <div class="toolbar">
      <el-select
        v-model="selectedRoleId"
        :placeholder="t('system.role.selectRole')"
        filterable
        style="min-width: 240px"
      >
        <el-option v-for="r in roles" :key="r.id" :label="r.name" :value="r.id" />
      </el-select>
      <el-button @click="openUserPicker">{{ t('system.role.selectUsers') }}</el-button>
    </div>

    <el-alert
      type="info"
      :closable="false"
      :title="t('system.role.assignHint')"
      show-icon
    />

    <SystemBatchActionBar
      :selected-count="selectedUsers.length"
      :label="t('system.role.selectedUsers')"
      :action-label="t('system.role.assign')"
      :disabled="!canSubmit"
      :loading="assignLoading"
      @action="assign"
    >
      <el-empty v-if="!selectedUsers.length" :description="t('system.role.noUsers')" />
      <div v-else class="chips">
        <el-tag v-for="user in selectedUsers" :key="user.id" closable @close="removeUser(user.id)">
          {{ user.displayName || user.username || user.id }}
        </el-tag>
      </div>
    </SystemBatchActionBar>

    <el-table v-loading="loading" :data="roles" border stripe height="100%" @row-click="setRole">
      <el-table-column type="index" width="50" />
      <el-table-column prop="name" :label="t('common.name')" />
      <el-table-column prop="scope" :label="t('common.scope')" />
      <el-table-column prop="description" :label="t('common.description')" />
    </el-table>

    <el-dialog
      v-model="userDialog.visible"
      :title="t('system.role.selectUsers')"
      width="60%"
    >
      <SystemFilterBar
        v-model="userKeyword"
        :placeholder="t('system.role.searchUser')"
        @search="debouncedUserSearch"
      />
      <el-table
        v-loading="userLoading"
        :data="userRows"
        border
        stripe
        height="360"
        @selection-change="onUserSelect"
      >
        <el-table-column type="selection" width="50" />
        <el-table-column prop="username" :label="t('common.username')" />
        <el-table-column prop="displayName" :label="t('common.displayName')" />
        <el-table-column prop="status" :label="t('common.status')" />
        <el-table-column prop="roles" :label="t('system.role.attached')">
          <template #default="{ row }">
            <el-tag v-for="r in row.roles" :key="r.id" size="small">{{ r.name }}</el-tag>
          </template>
        </el-table-column>
      </el-table>
      <div class="pager">
        <el-pagination
          v-model:current-page="userPage"
          v-model:page-size="userPageSize"
          :total="userTotal"
          layout="prev, pager, next, jumper"
          @current-change="fetchUsers"
          @size-change="fetchUsers"
        />
      </div>
      <template #footer>
        <el-button @click="userDialog.visible = false">{{ t('common.cancel') }}</el-button>
        <el-button
          type="primary"
          :loading="userLoading"
          :disabled="!tempSelected.length"
          @click="applySelection"
        >
          {{ t('common.confirm') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { listRoles, assignRoleUsers } from '@/api/system/role';
import { listUsers } from '@/api/system/user';
import type { RoleDetail, User } from '@/types/system';
import SystemBatchActionBar from './components/SystemBatchActionBar.vue';
import SystemFilterBar from './components/SystemFilterBar.vue';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { useI18n } from 'vue-i18n';
import { useDebounceFn } from '@vueuse/core';
import { showAuthError } from '@/shared/errors/errorToast';
import { showSuccess, showWarning } from '@/shared/errors/messageToast';

const roles = ref<RoleDetail[]>([]);
const loading = ref(false);
const assignLoading = ref(false);
const selectedRoleId = ref('');
const selectedUsers = ref<User[]>([]);
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const { t } = useI18n();

const userDialog = reactive({ visible: false });
const userRows = ref<User[]>([]);
const userTotal = ref(0);
const userPage = ref(1);
const userPageSize = ref(10);
const userKeyword = ref('');
const userLoading = ref(false);
const tempSelected = ref<User[]>([]);

const canSubmit = computed(() => !!selectedRoleId.value && selectedUsers.value.length > 0);

const fetchRoles = async () => {
  if (!permissionStore.can('__role_admin__')) return;
  loading.value = true;
  try {
    const { data } = await listRoles({ page: 1, pageSize: 200 });
    roles.value = data.data.items;
  } finally {
    loading.value = false;
  }
};

const fetchUsers = async () => {
  userLoading.value = true;
  try {
    const { data } = await listUsers({
      page: userPage.value,
      pageSize: userPageSize.value,
      keyword: userKeyword.value
    });
    userRows.value = data.data.items;
    userTotal.value = data.data.total;
  } finally {
    userLoading.value = false;
  }
};

const debouncedUserSearch = useDebounceFn(() => {
  userPage.value = 1;
  fetchUsers();
}, 250);

function setRole(row: RoleDetail) {
  selectedRoleId.value = row.id;
}

function onUserSelect(list: User[]) {
  tempSelected.value = list;
}

function applySelection() {
  selectedUsers.value = tempSelected.value;
  userDialog.visible = false;
}

function openUserPicker() {
  userDialog.visible = true;
  userPage.value = 1;
  tempSelected.value = selectedUsers.value.slice();
  fetchUsers();
}

function removeUser(id: string) {
  selectedUsers.value = selectedUsers.value.filter((u) => u.id !== id);
}

const assign = async () => {
  if (!selectedRoleId.value) {
    showWarning(t('system.role.selectRoleHint'));
    return;
  }
  if (!selectedUsers.value.length) {
    showWarning(t('system.role.selectUsersHint'));
    return;
  }
  assignLoading.value = true;
  try {
    await assignRoleUsers(
      selectedRoleId.value,
      selectedUsers.value.map((u) => u.id)
    );
    showSuccess(t('system.role.assigned'));
  } catch (e) {
    showAuthError(e, t('common.actionFailed'));
  } finally {
    assignLoading.value = false;
  }
};

onMounted(() => {
  fetchRoles();
});

watch(
  () => tenantStore.currentTenantId,
  () => {
    fetchRoles();
    selectedUsers.value = [];
    selectedRoleId.value = '';
  }
);
</script>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: 100%;
  padding: 16px;
}

.toolbar {
  display: flex;
  gap: 8px;
  align-items: center;
}

.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.dialog-toolbar {
  margin-bottom: 12px;
}

.pager {
  display: flex;
  justify-content: flex-end;
  padding: 8px 0;
}
</style>
