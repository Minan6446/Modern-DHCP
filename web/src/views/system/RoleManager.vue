<template>
  <div class="page surface-card">
    <el-tabs class="module-tabs" stretch>
      <el-tab-pane :label="t('system.role.pageTitle')">
        <el-card shadow="never" class="module-card">
          <template #header>
            <div class="module-header">
              <div>
                <div class="module-title">{{ t('system.role.pageTitle') }}</div>
                <div class="module-subtitle">{{ t('system.role.pageSubtitle') }}</div>
              </div>
              <el-button v-if="canManageRoles" type="primary" @click="openRoleForm()">{{ t('system.role.createRole') }}</el-button>
            </div>
          </template>

          <div class="toolbar-row">
            <el-input v-model="roleKeyword" :placeholder="t('system.role.roleSearchPh')" clearable class="w-280" @input="onRoleSearch" />
            <el-select v-model="roleScopeFilter" clearable :placeholder="t('system.role.scopePh')" class="w-140" @change="onRoleFilterChange">
              <el-option value="global" :label="t('system.role.scopeGlobal')" />
              <el-option value="tenant" :label="t('system.role.scopeTenant')" />
            </el-select>
            <el-button :loading="roleLoading" @click="fetchRoles">{{ t('system.role.refresh') }}</el-button>
          </div>

          <div v-if="selectedRoleRows.length" class="bulk-row">
            <span>{{ t('system.role.selectedN', { n: selectedRoleRows.length }) }}</span>
            <el-button
              v-if="canManageRoles"
              type="danger"
              plain
              :disabled="bulkRoleDeleteDisabled"
              :loading="actionLoading"
              @click="batchDeleteRoles"
            >
              {{ t('system.role.batchDelete') }}
            </el-button>
          </div>

          <el-table
            v-loading="roleLoading"
            :data="roleRows"
            border
            stripe
            class="data-table"
            @selection-change="onRoleSelectionChange"
          >
            <el-table-column type="selection" width="52" />
            <el-table-column prop="name" :label="t('system.role.colName')" min-width="180" />
            <el-table-column prop="description" :label="t('system.role.colDesc')" min-width="200" show-overflow-tooltip />
            <el-table-column :label="t('system.role.colScope')" width="120">
              <template #default="{ row }">{{ row.scope === 'global' ? t('system.role.scopeGlobal') : t('system.role.scopeTenant') }}</template>
            </el-table-column>
            <el-table-column :label="t('system.role.colPermCount')" width="110">
              <template #default="{ row }">{{ row.permissions?.length || 0 }}</template>
            </el-table-column>
            <el-table-column :label="t('system.role.colLinkedUsers')" width="120">
              <template #default="{ row }">
                <el-button text type="primary" @click="linkUsersByRole(row)">{{ roleUsageCountById[row.id] || 0 }}</el-button>
              </template>
            </el-table-column>
            <el-table-column :label="t('system.role.colCreated')" min-width="170">
              <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
            </el-table-column>
            <el-table-column :label="t('system.role.colAction')" fixed="right" width="280">
              <template #default="{ row }">
                <el-button v-if="canManageRoles" type="primary" plain size="small" @click="openRoleForm(row)">{{ t('system.role.editBtn') }}</el-button>
                <el-button v-if="canManageAssignments" plain size="small" @click="openAssign(row)">{{ t('system.role.assignBtn') }}</el-button>
                <el-popconfirm v-if="canManageRoles" :title="t('system.role.deleteConfirmMsg')" @confirm="removeRole(row)">
                  <template #reference>
                    <el-button type="danger" plain size="small" :disabled="cannotDeleteRole(row)">{{ t('system.role.deleteBtn') }}</el-button>
                  </template>
                </el-popconfirm>
              </template>
            </el-table-column>
          </el-table>

          <div class="pager-row">
            <el-pagination
              v-model:current-page="rolePage"
              v-model:page-size="rolePageSize"
              :total="roleTotal"
              layout="total, sizes, prev, pager, next, jumper"
              @current-change="fetchRoles"
              @size-change="fetchRoles"
            />
          </div>
        </el-card>
      </el-tab-pane>

      <el-tab-pane :label="t('system.role.userTitle')">
        <el-card shadow="never" class="module-card">
          <template #header>
            <div class="module-header">
              <div>
                <div class="module-title">{{ t('system.role.userTitle') }}</div>
                <div class="module-subtitle">{{ t('system.role.userSubtitle') }}</div>
              </div>
              <el-button v-if="canManageUsers" type="primary" @click="openUserDialog">{{ t('system.role.createUser') }}</el-button>
            </div>
          </template>

      <div class="toolbar-row user-toolbar">
        <el-input v-model="userKeyword" :placeholder="t('system.role.userSearchPh')" clearable class="w-300" @input="onUserSearch" />
        <el-select v-model="userStatus" clearable :placeholder="t('system.role.statusPh')" class="w-140" @change="onUserFilterChange">
          <el-option value="active" :label="t('system.role.statusActive')" />
          <el-option value="disabled" :label="t('system.role.statusDisabled')" />
        </el-select>
        <el-select
          v-model="linkedRoleFilterId"
          clearable
          filterable
          :placeholder="t('system.role.rolePh')"
          class="w-200"
          @change="onUserFilterChange"
        >
          <el-option v-for="item in roleRows" :key="item.id" :label="item.name" :value="item.id" />
        </el-select>
        <el-button :loading="userLoading" @click="fetchUsers">{{ t('system.role.refresh') }}</el-button>
      </div>

      <div v-if="linkedRoleFilterId" class="link-tip">
        <el-tag type="info" closable @close="clearLinkedRoleFilter">
          {{ t('system.role.filterByRole', { name: roleNameById(linkedRoleFilterId) }) }}
        </el-tag>
      </div>

      <div v-if="selectedUserRows.length" class="bulk-row">
        <span>{{ t('system.role.selectedN', { n: selectedUserRows.length }) }}</span>
        <el-button v-if="canManageUsers" plain :loading="userActionLoading" @click="batchUpdateUserStatus('active')">
          {{ t('system.role.batchEnable') }}
        </el-button>
        <el-button v-if="canManageUsers" plain :loading="userActionLoading" @click="batchUpdateUserStatus('disabled')">
          {{ t('system.role.batchDisable') }}
        </el-button>
        <el-button
          v-if="canManageUsers"
          type="danger"
          plain
          :disabled="selectedUserRows.every(isProtectedUser)"
          :loading="userActionLoading"
          @click="batchDeleteUsers"
        >
          {{ t('system.role.batchDeleteUsers') }}
        </el-button>
      </div>

      <el-table
        v-loading="userLoading"
        :data="userRows"
        border
        stripe
        class="data-table"
        @selection-change="onUserSelectionChange"
      >
        <el-table-column type="selection" width="52" />
        <el-table-column prop="username" :label="t('system.role.colUsername')" min-width="130" />
        <el-table-column prop="displayName" :label="t('system.role.colDisplayName')" min-width="120" />
        <el-table-column prop="email" :label="t('system.role.colEmail')" min-width="210" show-overflow-tooltip />
        <el-table-column prop="phone" :label="t('system.role.colPhone')" min-width="140" />
        <el-table-column :label="t('system.role.colRoles')" min-width="200">
          <template #default="{ row }">
            <el-space v-if="(userRoleNamesByUserId[row.id] || []).length" wrap>
              <el-tag
                v-for="roleName in userRoleNamesByUserId[row.id]"
                :key="`${row.id}-${roleName}`"
                size="small"
                type="info"
              >
                {{ roleName }}
              </el-tag>
            </el-space>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('system.role.colStatus')" width="100">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('system.role.colLastLogin')" min-width="170">
          <template #default="{ row }">{{ formatTime(row.lastLoginAt) }}</template>
        </el-table-column>
        <el-table-column :label="t('system.role.colUserAction')" fixed="right" width="360">
          <template #default="{ row }">
            <el-button v-if="canManageUsers" type="primary" plain size="small" @click="openEditUser(row)">{{ t('system.role.editUser') }}</el-button>
            <el-button v-if="canManageUsers" plain size="small" @click="resetUserPassword(row)">{{ t('system.role.resetPwd') }}</el-button>
            <el-button
              v-if="canManageUsers"
              plain
              size="small"
              :type="row.status === 'active' ? 'warning' : 'success'"
              @click="toggleUserStatus(row)"
            >
              {{ row.status === 'active' ? t('system.role.disableBtn') : t('system.role.enableBtn') }}
            </el-button>
            <el-popconfirm v-if="canManageUsers" :title="t('system.role.deleteUserConfirm')" @confirm="removeUser(row)">
              <template #reference>
                <el-button type="danger" plain size="small" :disabled="isProtectedUser(row)">{{ t('system.role.deleteUserBtn') }}</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>

      <div class="pager-row">
        <el-pagination
          v-model:current-page="userPage"
          v-model:page-size="userPageSize"
          :total="userTotal"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="fetchUsers"
          @size-change="fetchUsers"
        />
      </div>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <el-dialog
      v-model="roleDialog.visible"
      :title="roleDialog.title"
      width="600px"
      class="spec-dialog standard-form-dialog"
      destroy-on-close
      :close-on-click-modal="false"
      :close-on-press-escape="false"
      :before-close="handleRoleBeforeClose"
    >
      <el-form
        ref="roleFormRef"
        class="standard-form"
        :model="roleForm"
        :rules="roleRules"
        label-width="110px"
        label-position="right"
        require-asterisk-position="left"
      >
        <div class="form-section-title">{{ t('system.role.formBasic') }}</div>
        <el-form-item :label="t('system.role.formRoleName')" prop="name">
          <div class="field-wrap">
            <el-input v-model="roleForm.name" :placeholder="t('system.role.formRoleNamePh')" @input="onRoleNameInput" />
            <div class="form-help">{{ t('system.role.formRoleNameHelp') }}</div>
            <div v-if="roleNameChecking" class="inline-state">{{ t('system.role.nameChecking') }}</div>
            <div v-else-if="roleNameDuplicate" class="inline-state danger">{{ t('system.role.nameDuplicate') }}</div>
          </div>
        </el-form-item>
        <el-form-item :label="t('system.role.formScope')" prop="scope">
          <div class="field-wrap">
            <el-select v-model="roleForm.scope" class="full-width">
              <el-option value="global" :label="t('system.role.scopeGlobal')" />
              <el-option value="tenant" :label="t('system.role.scopeTenant')" />
            </el-select>
            <div class="form-help">{{ t('system.role.formScopeHelp') }}</div>
          </div>
        </el-form-item>
        <el-form-item :label="t('system.role.formDesc')" prop="description">
          <div class="field-wrap">
            <el-input v-model="roleForm.description" type="textarea" :rows="3" :placeholder="t('system.role.formDescPh')" />
            <div class="form-help">{{ t('system.role.formDescHelp') }}</div>
          </div>
        </el-form-item>

        <div class="form-section-divider" />
        <div class="form-section-title">{{ t('system.role.formPermConfig') }}</div>
        <el-form-item :label="t('system.role.formPermSearch')">
          <div class="field-wrap">
            <el-input v-model="permissionKeyword" clearable :placeholder="t('system.role.formPermSearchPh')" />
            <div class="form-help">{{ t('system.role.formPermSearchHelp') }}</div>
          </div>
        </el-form-item>
        <el-form-item :label="t('system.role.formPermActions')">
          <div class="field-wrap actions-wrap">
            <el-button @click="checkAllPermissions">{{ t('system.role.permSelectAll') }}</el-button>
            <el-button @click="invertPermissions">{{ t('system.role.permInvert') }}</el-button>
            <el-button @click="clearPermissions">{{ t('system.role.permClear') }}</el-button>
            <el-button type="primary" plain @click="permissionPreviewVisible = true">{{ t('system.role.permPreview') }}</el-button>
          </div>
        </el-form-item>
        <el-form-item :label="t('system.role.formPermTree')" prop="permissions">
          <div class="field-wrap">
            <el-tree
              ref="treeRef"
              :data="permissionTree"
              node-key="key"
              show-checkbox
              check-on-click-node
              class="permission-tree"
              :props="{ children: 'children', label: 'label' }"
              :filter-node-method="permissionFilterNode"
              @check="onTreeCheck"
            />
            <div class="form-help">{{ t('system.role.formPermTreeHelp') }}</div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer fixed-actions">
          <el-button @click="closeRoleDialog">{{ t('system.role.btnCancel') }}</el-button>
          <el-button type="primary" :loading="actionLoading" :disabled="actionLoading" @click="submitRole">{{ t('system.role.btnSave') }}</el-button>
        </div>
      </template>
    </el-dialog>

    <el-dialog v-model="permissionPreviewVisible" :title="t('system.role.permPreviewTitle')" width="600px" class="spec-dialog" destroy-on-close>
      <div class="permission-preview">
        <template v-if="selectedPermissionItems.length">
          <el-tag v-for="item in selectedPermissionItems" :key="item.key" class="preview-tag">
            {{ item.label }}
          </el-tag>
        </template>
        <el-empty v-else :description="t('system.role.permPreviewEmpty')" />
      </div>
      <template #footer>
        <div class="dialog-footer">
          <el-button type="primary" @click="permissionPreviewVisible = false">{{ t('system.role.permPreviewClose') }}</el-button>
        </div>
      </template>
    </el-dialog>

    <el-dialog
      v-model="userDialog.visible"
      :title="userDialog.title"
      width="600px"
      class="spec-dialog standard-form-dialog"
      destroy-on-close
      :close-on-click-modal="false"
      :close-on-press-escape="false"
      :before-close="handleUserBeforeClose"
    >
      <el-form
        ref="userFormRef"
        class="standard-form"
        :model="userForm"
        :rules="userRules"
        label-width="110px"
        label-position="right"
        require-asterisk-position="left"
      >
        <div class="form-section-title">{{ t('system.role.formUserBasic') }}</div>
        <el-form-item :label="t('system.role.formUsername')" prop="username">
          <div class="field-wrap">
            <el-input v-model="userForm.username" :placeholder="t('system.role.formUsernamePh')" @input="onUsernameInput" />
            <div class="form-help">{{ t('system.role.formUsernameHelp') }}</div>
            <div v-if="usernameChecking" class="inline-state">{{ t('system.role.usernameChecking') }}</div>
            <div v-else-if="usernameDuplicate" class="inline-state danger">{{ t('system.role.usernameDuplicate') }}</div>
          </div>
        </el-form-item>
        <el-form-item :label="t('system.role.formDisplayName')" prop="displayName">
          <div class="field-wrap">
            <el-input v-model="userForm.displayName" :placeholder="t('system.role.formDisplayNamePh')" />
            <div class="form-help">{{ t('system.role.formDisplayNameHelp') }}</div>
          </div>
        </el-form-item>
        <el-form-item :label="t('system.role.formPassword')" prop="password">
          <div class="field-wrap">
            <el-input
              v-model="userForm.password"
              :type="showPassword ? 'text' : 'password'"
              :placeholder="t('system.role.formPasswordPh')"
              @input="passwordTouched = true"
            >
              <template #suffix>
                <el-button link class="password-visibility" @click="showPassword = !showPassword">
                  <el-icon v-if="showPassword"><Hide /></el-icon>
                  <el-icon v-else><View /></el-icon>
                </el-button>
              </template>
            </el-input>
            <div class="form-help">
              {{ t('system.role.pwdStrength') }}<strong :class="`strength-${passwordStrength.level}`">{{ passwordStrength.label }}</strong>
            </div>
            <div v-if="passwordFormatError" class="inline-state danger">{{ passwordFormatError }}</div>
          </div>
        </el-form-item>
        <el-form-item :label="t('system.role.formEmail')" prop="email">
          <div class="field-wrap">
            <el-input v-model="userForm.email" :placeholder="t('system.role.formEmailPh')" @input="onEmailInput" />
            <div class="form-help">{{ t('system.role.formEmailHelp') }}</div>
            <div v-if="emailFormatError" class="inline-state danger">{{ emailFormatError }}</div>
          </div>
        </el-form-item>
        <el-form-item :label="t('system.role.formPhone')" prop="phone">
          <div class="field-wrap">
            <el-input v-model="userForm.phone" :placeholder="t('system.role.formPhonePh')" @input="onPhoneInput" />
            <div class="form-help">{{ t('system.role.formPhoneHelp') }}</div>
            <div v-if="phoneFormatError" class="inline-state danger">{{ phoneFormatError }}</div>
          </div>
        </el-form-item>

        <div class="form-section-divider" />
        <div class="form-section-title">{{ t('system.role.formStatusConfig') }}</div>
        <el-form-item :label="t('system.role.formStatus')" prop="status">
          <div class="field-wrap">
            <el-select v-model="userForm.status" class="full-width">
              <el-option value="active" :label="t('system.role.statusActive')" />
              <el-option value="disabled" :label="t('system.role.statusDisabled')" />
            </el-select>
          </div>
        </el-form-item>
        <el-form-item v-if="canManageAssignments" :label="t('system.role.formRoleSelect')">
          <div class="field-wrap">
            <el-select v-model="userForm.roles" multiple filterable collapse-tags collapse-tags-tooltip class="full-width">
              <el-option v-for="item in roleRows" :key="item.id" :label="item.name" :value="item.id" />
            </el-select>
            <div class="form-help">{{ t('system.role.formRoleSelectHelp') }}</div>
          </div>
        </el-form-item>

        <div v-if="!editingUserId" class="form-section-divider" />
        <div v-if="!editingUserId" class="form-section-title">{{ t('system.role.formContinue') }}</div>
        <el-form-item v-if="!editingUserId" :label="t('system.role.formContinueLabel')">
          <div class="field-wrap">
            <el-switch v-model="createAndContinue" :active-text="t('system.role.continueOn')" :inactive-text="t('system.role.continueOff')" />
            <div class="form-help">{{ t('system.role.continueHelp') }}</div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer fixed-actions">
          <el-button @click="closeUserDialog">{{ t('system.role.btnCancel') }}</el-button>
          <el-button type="primary" :loading="userActionLoading" :disabled="userActionLoading" @click="submitUser">{{ t('system.role.btnSave') }}</el-button>
        </div>
      </template>
    </el-dialog>

    <el-dialog v-model="assignDialog.visible" :title="t('system.role.assignTitle')" width="760px" class="spec-dialog" destroy-on-close>
      <div class="assign-header">
        <div>
          {{ t('system.role.assignCurrentRole') }}<strong>{{ assignDialog.roleName }}</strong>
        </div>
        <el-input v-model="assignKeyword" :placeholder="t('system.role.assignSearchPh')" clearable class="w-260" @input="onAssignSearch" />
      </div>
      <el-table
        v-loading="assignLoading"
        :data="assignRows"
        border
        stripe
        height="360"
        @selection-change="onAssignSelectionChange"
      >
        <el-table-column type="selection" width="52" />
        <el-table-column prop="username" :label="t('system.role.assignUserCol')" min-width="160" />
        <el-table-column prop="displayName" :label="t('system.role.assignNameCol')" min-width="150" />
        <el-table-column :label="t('system.role.assignStatusCol')" width="120">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
      </el-table>
      <div class="pager-row">
        <el-pagination
          v-model:current-page="assignPage"
          v-model:page-size="assignPageSize"
          :total="assignTotal"
          layout="total, sizes, prev, pager, next"
          @current-change="fetchAssignUsers"
          @size-change="fetchAssignUsers"
        />
      </div>
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="assignDialog.visible = false">{{ t('system.role.btnCancel') }}</el-button>
          <el-button
            type="primary"
            :loading="assignSubmitting"
            :disabled="assignSubmitting || !selectedAssignUsers.length"
            @click="submitAssign"
          >
            {{ t('system.role.btnSave') }}
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue';
import { useDebounceFn } from '@vueuse/core';
import { ElMessageBox, type FormInstance, type FormRules } from 'element-plus';
import type { TreeInstance } from 'element-plus/es/components/tree';
import { Hide, View } from '@element-plus/icons-vue';
import { useI18n } from 'vue-i18n';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { useSystemViewStore } from '@/store/systemView';
import { showAuthError } from '@/shared/errors/errorToast';
import { showSuccess, showWarning } from '@/shared/errors/messageToast';
import { getPermissionTree } from '@/api/roles';
import {
  listRoles,
  createRole,
  updateRole,
  deleteRole,
  assignRoleUsers,
  listAssignments,
  createAssignment,
  deleteAssignment
} from '@/api/system/role';
import { listUsers, createUser, updateUser, deleteUser, updateUserStatus } from '@/api/system/user';
import type { PermissionNode, RoleDetail, User, UserPayload } from '@/types/system';

const { t } = useI18n();
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const systemView = useSystemViewStore();

const canReadRoles = computed(() => permissionStore.can('rbac.role.read'));
const canManageRoles = computed(() => permissionStore.can('rbac.role.manage'));
const canReadUsers = computed(() => permissionStore.can('auth.user.read'));
const canManageUsers = computed(() => permissionStore.can('auth.user.manage'));
const canReadAssignments = computed(() => permissionStore.can('rbac.assignment.read'));
const canManageAssignments = computed(() => permissionStore.can('rbac.assignment.write'));

const roleRows = ref<RoleDetail[]>([]);
const roleTotal = ref(0);
const rolePage = ref(systemView.role.page);
const rolePageSize = ref(systemView.role.pageSize);
const roleKeyword = ref(systemView.role.keyword);
const roleScopeFilter = ref('');
const roleLoading = ref(false);
const selectedRoleRows = ref<RoleDetail[]>([]);
const roleUsageCountById = ref<Record<string, number>>({});

const userRows = ref<User[]>([]);
const userTotal = ref(0);
const userPage = ref(systemView.user.page);
const userPageSize = ref(systemView.user.pageSize);
const userKeyword = ref(systemView.user.keyword);
const userStatus = ref<string>(systemView.user.status || '');
const linkedRoleFilterId = ref<string>(systemView.user.roleId);
const userLoading = ref(false);
const userActionLoading = ref(false);
const selectedUserRows = ref<User[]>([]);
const userRoleNamesByUserId = ref<Record<string, string[]>>({});

const actionLoading = ref(false);

const permissionTree = ref<PermissionNode[]>([]);
const treeRef = ref<TreeInstance>();
const permissionKeyword = ref('');
const permissionPreviewVisible = ref(false);

const roleDialog = reactive({ visible: false, title: '' });
const roleFormRef = ref<FormInstance>();
const roleNameChecking = ref(false);
const roleNameDuplicate = ref(false);
const roleDialogSnapshot = ref('');
const roleForm = reactive<RoleDetail>({
  id: '',
  name: '',
  description: '',
  scope: 'tenant',
  permissions: []
});

const assignDialog = reactive({ visible: false, roleId: '', roleName: '' });
const assignRows = ref<User[]>([]);
const assignTotal = ref(0);
const assignPage = ref(1);
const assignPageSize = ref(10);
const assignKeyword = ref('');
const assignLoading = ref(false);
const assignSubmitting = ref(false);
const selectedAssignUsers = ref<User[]>([]);

const userDialog = reactive({ visible: false, title: '' });
const userFormRef = ref<FormInstance>();
const editingUserId = ref('');
const editingSnapshot = ref<{ username: string; displayName: string; email: string; status: string } | null>(null);
const showPassword = ref(false);
const createAndContinue = ref(false);
const usernameChecking = ref(false);
const usernameDuplicate = ref(false);
const passwordTouched = ref(false);
const emailTouched = ref(false);
const phoneTouched = ref(false);
const userDialogSnapshot = ref('');
const userForm = reactive<UserPayload & { status?: 'active' | 'disabled'; roles: string[] }>({
  username: '',
  displayName: '',
  password: '',
  email: '',
  phone: '',
  status: 'active',
  roles: []
});

const roleNameById = (id: string) => roleRows.value.find((item) => item.id === id)?.name || '-';

const roleIdNameMap = () => {
  const map = new Map<string, string>();
  roleRows.value.forEach((role) => {
    if (role.id && role.name) map.set(String(role.id), String(role.name));
  });
  return map;
};

const roleNameIdMap = () => {
  const map = new Map<string, string>();
  roleRows.value.forEach((role) => {
    if (role.id && role.name) map.set(String(role.name), String(role.id));
  });
  return map;
};

const userPrincipalId = (userId: string) => `user:${String(userId || '').trim()}`;

const formatTime = (value?: string) => {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`;
};

const statusLabel = (status?: string) => {
  if (status === 'active') return t('system.role.statusActive');
  if (status === 'disabled') return t('system.role.statusDisabled');
  if (status === 'pending') return t('system.role.statusPending');
  return status || '-';
};

const statusTagType = (status?: string) => {
  if (status === 'active') return 'success';
  if (status === 'disabled') return 'info';
  if (status === 'pending') return 'warning';
  return '';
};

const isProtectedUser = (row: User) => {
  const username = String(row?.username || '').toLowerCase();
  if (username === 'superadmin' || username === 'admin') return true;
  const roleNames = userRoleNamesByUserId.value[row.id] || [];
  return roleNames.some((name) => {
    const n = String(name || '').toLowerCase();
    return n.includes('super') || n.includes('admin');
  });
};

const cannotDeleteRole = (row: RoleDetail) => {
  const usage = roleUsageCountById.value[row.id] || 0;
  const name = String(row.name || '').toLowerCase();
  const protectedRole = name.includes('admin') || name.includes('super');
  return usage > 0 || protectedRole;
};

const bulkRoleDeleteDisabled = computed(
  () => selectedRoleRows.value.length === 0 || selectedRoleRows.value.some((row) => cannotDeleteRole(row))
);

const passwordStrength = computed(() => {
  const value = String(userForm.password || '');
  let score = 0;
  if (value.length >= 8) score += 1;
  if (/[A-Z]/.test(value) && /[a-z]/.test(value)) score += 1;
  if (/\d/.test(value)) score += 1;
  if (/[^A-Za-z0-9]/.test(value)) score += 1;
  if (score <= 1) return { level: 'weak', label: t('system.role.pwdWeak') };
  if (score <= 3) return { level: 'medium', label: t('system.role.pwdMedium') };
  return { level: 'strong', label: t('system.role.pwdStrong') };
});

const emailFormatError = computed(() => {
  if (!emailTouched.value) return '';
  const text = String(userForm.email || '').trim();
  if (!text) return '';
  if (/^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$/.test(text)) return '';
  return t('system.role.valEmailHint');
});

const phoneFormatError = computed(() => {
  if (!phoneTouched.value) return '';
  const text = String(userForm.phone || '').trim();
  if (!text) return '';
  if (/^(\+?\d{6,20}|\d{3,4}-\d{6,12})$/.test(text)) return '';
  return t('system.role.valPhoneHint');
});

const passwordFormatError = computed(() => {
  if (!passwordTouched.value) return '';
  const text = String(userForm.password || '');
  if (!text) return editingUserId.value ? '' : t('system.role.valPwdFullHint');
  if (text.length < 8) return t('system.role.valPwdMin');
  if (!/[A-Z]/.test(text) || !/[a-z]/.test(text) || !/\d/.test(text) || !/[^A-Za-z0-9]/.test(text)) {
    return t('system.role.valPwdComplex');
  }
  return '';
});

const permissionLabelMap = computed(() => {
  const map = new Map<string, string>();
  const walk = (nodes: PermissionNode[]) => {
    nodes.forEach((node) => {
      map.set(String(node.key), String(node.label || node.key));
      if (node.children?.length) walk(node.children);
    });
  };
  walk(permissionTree.value);
  return map;
});

const selectedPermissionItems = computed(() =>
  roleForm.permissions.map((key) => ({
    key,
    label: permissionLabelMap.value.get(String(key)) || String(key)
  }))
);

const serializeRoleForm = () =>
  JSON.stringify({
    id: String(roleForm.id || ''),
    name: String(roleForm.name || '').trim(),
    description: String(roleForm.description || '').trim(),
    scope: String(roleForm.scope || 'tenant'),
    permissions: [...(roleForm.permissions || [])].map(String).sort()
  });

const serializeUserForm = () =>
  JSON.stringify({
    editingUserId: String(editingUserId.value || ''),
    username: String(userForm.username || '').trim(),
    displayName: String(userForm.displayName || '').trim(),
    password: String(userForm.password || ''),
    email: String(userForm.email || '').trim(),
    phone: String(userForm.phone || '').trim(),
    status: String(userForm.status || 'active'),
    roles: [...(userForm.roles || [])].map(String).sort(),
    createAndContinue: !!createAndContinue.value
  });

const hasRoleDialogUnsavedChanges = () => serializeRoleForm() !== roleDialogSnapshot.value;

const hasUserDialogUnsavedChanges = () => serializeUserForm() !== userDialogSnapshot.value;

const confirmDiscardDialogChanges = async () => {
  await ElMessageBox.confirm(t('system.role.unsavedConfirm'), t('system.role.unsavedTitle'), {
    type: 'warning',
    confirmButtonText: t('system.role.unsavedOk'),
    cancelButtonText: t('system.role.unsavedCancel')
  });
};

const validateRoleName = (_: unknown, value: string, cb: (e?: Error) => void) => {
  const text = String(value || '').trim();
  if (!text) return cb(new Error(t('system.role.valRoleNameRequired')));
  if (text.length < 2 || text.length > 50) return cb(new Error(t('system.role.valRoleNameLength')));
  if (roleNameDuplicate.value) return cb(new Error(t('system.role.valRoleNameDup')));
  cb();
};

const validateUserName = (_: unknown, value: string, cb: (e?: Error) => void) => {
  const text = String(value || '').trim();
  if (!text) return cb(new Error(t('system.role.valUsernameRequired')));
  if (text.length < 2 || text.length > 50) return cb(new Error(t('system.role.valUsernameLength')));
  if (usernameDuplicate.value) return cb(new Error(t('system.role.valUsernameDup')));
  cb();
};

const validatePassword = (_: unknown, value: string, cb: (e?: Error) => void) => {
  const text = String(value || '');
  if (!editingUserId.value && !text) return cb(new Error(t('system.role.valPwdRequired')));
  if (!text) return cb();
  if (text.length < 8) return cb(new Error(t('system.role.valPwdMin')));
  if (!/[A-Z]/.test(text) || !/[a-z]/.test(text) || !/\d/.test(text) || !/[^A-Za-z0-9]/.test(text)) {
    return cb(new Error(t('system.role.valPwdComplex')));
  }
  cb();
};

const validateEmail = (_: unknown, value: string, cb: (e?: Error) => void) => {
  const text = String(value || '').trim();
  if (!text) return cb();
  if (!/^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$/.test(text)) {
    return cb(new Error(t('system.role.valEmailInvalid')));
  }
  cb();
};

const validatePhone = (_: unknown, value: string, cb: (e?: Error) => void) => {
  const text = String(value || '').trim();
  if (!text) return cb();
  if (!/^(\+?\d{6,20}|\d{3,4}-\d{6,12})$/.test(text)) {
    return cb(new Error(t('system.role.valPhoneInvalid')));
  }
  cb();
};

const roleRules: FormRules = {
  name: [{ required: true, validator: validateRoleName, trigger: 'blur' }],
  permissions: [
    {
      validator: (_: unknown, __: unknown, cb: (e?: Error) => void) => {
        if (roleForm.permissions.length) return cb();
        cb(new Error(t('system.role.valPermMin')));
      },
      trigger: 'change'
    }
  ]
};

const userRules: FormRules = {
  username: [{ required: true, validator: validateUserName, trigger: ['blur', 'change'] }],
  displayName: [{ required: true, message: t('system.role.valDisplayNameReq'), trigger: ['blur', 'change'] }],
  password: [{ validator: validatePassword, trigger: ['blur', 'change'] }],
  email: [{ validator: validateEmail, trigger: ['blur', 'change'] }],
  phone: [{ validator: validatePhone, trigger: ['blur', 'change'] }]
};

const checkRoleNameDuplicate = useDebounceFn(async (value: string) => {
  const text = String(value || '').trim();
  roleNameDuplicate.value = false;
  if (!text) return;
  roleNameChecking.value = true;
  try {
    const { data } = await listRoles({ page: 1, pageSize: 100, keyword: text });
    const duplicate = (data.data.items || []).some(
      (item) => item.name.trim().toLowerCase() === text.toLowerCase() && item.id !== roleForm.id
    );
    roleNameDuplicate.value = duplicate;
  } finally {
    roleNameChecking.value = false;
    roleFormRef.value?.validateField?.('name').catch(() => undefined);
  }
}, 280);

const checkUsernameDuplicate = useDebounceFn(async (value: string) => {
  const text = String(value || '').trim();
  usernameDuplicate.value = false;
  if (!text) return;
  usernameChecking.value = true;
  try {
    const { data } = await listUsers({ page: 1, pageSize: 100, keyword: text });
    const duplicate = (data.data.items || []).some(
      (item) => item.username.trim().toLowerCase() === text.toLowerCase() && item.id !== editingUserId.value
    );
    usernameDuplicate.value = duplicate;
  } finally {
    usernameChecking.value = false;
    userFormRef.value?.validateField?.('username').catch(() => undefined);
  }
}, 280);

const fetchRoles = async () => {
  if (!canReadRoles.value) return;
  systemView.setRole({ page: rolePage.value, pageSize: rolePageSize.value, keyword: roleKeyword.value });
  roleLoading.value = true;
  try {
    const { data } = await listRoles({ page: rolePage.value, pageSize: rolePageSize.value, keyword: roleKeyword.value });
    const rows = (data.data.items || []).filter((item) => {
      if (!roleScopeFilter.value) return true;
      return String(item.scope) === roleScopeFilter.value;
    });
    roleRows.value = rows;
    roleTotal.value = roleScopeFilter.value ? rows.length : data.data.total || 0;
    await refreshRoleUsageCounts();
  } finally {
    roleLoading.value = false;
  }
};

const refreshUserRoleBadges = async (users: User[]) => {
  const result: Record<string, string[]> = {};
  if (!canReadAssignments.value || !users.length) {
    userRoleNamesByUserId.value = result;
    return;
  }
  const nameToId = roleNameIdMap();
  await Promise.all(
    users.map(async (user) => {
      const uid = String(user.id || '').trim();
      if (!uid) return;
      const principalId = userPrincipalId(uid);
      const { data } = await listAssignments(principalId, { page: 1, pageSize: 200 });
      const names = (data.data.items || [])
        .map((item) => String(item.roleName || '').trim())
        .filter((name) => !!name && nameToId.has(name));
      result[uid] = Array.from(new Set(names));
    })
  );
  userRoleNamesByUserId.value = result;
};

const listAllUsersWithBaseFilter = async () => {
  const all: User[] = [];
  let current = 1;
  const size = 100;
  while (true) {
    const { data } = await listUsers({
      page: current,
      pageSize: size,
      keyword: userKeyword.value,
      status: userStatus.value || undefined
    });
    const items = data.data.items || [];
    all.push(...items);
    if (all.length >= (data.data.total || 0) || items.length < size) break;
    current += 1;
  }
  return all;
};

const fetchUsers = async () => {
  if (!canReadUsers.value) {
    userRows.value = [];
    userTotal.value = 0;
    return;
  }
  systemView.setUser({
    page: userPage.value,
    pageSize: userPageSize.value,
    keyword: userKeyword.value,
    status: userStatus.value,
    roleId: linkedRoleFilterId.value
  });
  userLoading.value = true;
  try {
    if (linkedRoleFilterId.value) {
      const allUsers = await listAllUsersWithBaseFilter();
      await refreshUserRoleBadges(allUsers);
      const targetRole = roleNameById(linkedRoleFilterId.value);
      const filtered = allUsers.filter((user) => (userRoleNamesByUserId.value[user.id] || []).includes(targetRole));
      userTotal.value = filtered.length;
      const start = (userPage.value - 1) * userPageSize.value;
      userRows.value = filtered.slice(start, start + userPageSize.value);
      await refreshUserRoleBadges(userRows.value);
      return;
    }

    const { data } = await listUsers({
      page: userPage.value,
      pageSize: userPageSize.value,
      keyword: userKeyword.value,
      status: userStatus.value || undefined
    });
    userRows.value = data.data.items || [];
    userTotal.value = data.data.total || 0;
    await refreshUserRoleBadges(userRows.value);
  } finally {
    userLoading.value = false;
  }
};

const refreshRoleUsageCounts = async () => {
  if (!canReadUsers.value || !canReadAssignments.value) {
    roleUsageCountById.value = {};
    return;
  }
  const counts: Record<string, number> = {};
  const nameToId = roleNameIdMap();
  const users = await listAllUsersWithBaseFilter();
  await Promise.all(
    users.map(async (user) => {
      const principalId = userPrincipalId(user.id);
      const { data } = await listAssignments(principalId, { page: 1, pageSize: 200 });
      const uniqueNames = Array.from(
        new Set((data.data.items || []).map((item) => String(item.roleName || '').trim()).filter(Boolean))
      );
      uniqueNames.forEach((name) => {
        const id = nameToId.get(name);
        if (!id) return;
        counts[id] = (counts[id] || 0) + 1;
      });
    })
  );
  roleUsageCountById.value = counts;
};

const loadPermissionTree = async () => {
  if (!canReadRoles.value) return;
  const { data } = await getPermissionTree();
  permissionTree.value = (data.data || []).filter((n: PermissionNode) => !n.key?.toLowerCase().startsWith('tenant'));
};

const onRoleSearch = useDebounceFn(() => {
  rolePage.value = 1;
  fetchRoles();
}, 250);

const onUserSearch = useDebounceFn(() => {
  userPage.value = 1;
  fetchUsers();
}, 250);

const onAssignSearch = useDebounceFn(() => {
  assignPage.value = 1;
  fetchAssignUsers();
}, 250);

const onRoleFilterChange = () => {
  rolePage.value = 1;
  fetchRoles();
};

const onUserFilterChange = () => {
  userPage.value = 1;
  fetchUsers();
};

const clearLinkedRoleFilter = () => {
  linkedRoleFilterId.value = '';
  onUserFilterChange();
};

const linkUsersByRole = (row: RoleDetail) => {
  linkedRoleFilterId.value = row.id;
  userPage.value = 1;
  fetchUsers();
};

const onRoleSelectionChange = (rows: RoleDetail[]) => {
  selectedRoleRows.value = rows;
};

const onUserSelectionChange = (rows: User[]) => {
  selectedUserRows.value = rows;
};

const onTreeCheck = () => {
  const checked = (treeRef.value?.getCheckedKeys(false) || []).map(String);
  roleForm.permissions = checked;
};

const permissionFilterNode = (value: string, data: PermissionNode) => {
  const keyword = String(value || '').trim().toLowerCase();
  if (!keyword) return true;
  return data.label.toLowerCase().includes(keyword) || data.key.toLowerCase().includes(keyword);
};

const checkAllPermissions = () => {
  const allKeys: string[] = [];
  const walk = (nodes: PermissionNode[]) => {
    nodes.forEach((node) => {
      allKeys.push(String(node.key));
      if (node.children?.length) walk(node.children);
    });
  };
  walk(permissionTree.value);
  treeRef.value?.setCheckedKeys(allKeys, false);
  roleForm.permissions = allKeys;
};

const invertPermissions = () => {
  const allKeys: string[] = [];
  const walk = (nodes: PermissionNode[]) => {
    nodes.forEach((node) => {
      allKeys.push(String(node.key));
      if (node.children?.length) walk(node.children);
    });
  };
  walk(permissionTree.value);
  const current = new Set((treeRef.value?.getCheckedKeys(false) || []).map(String));
  const next = allKeys.filter((key) => !current.has(key));
  treeRef.value?.setCheckedKeys(next, false);
  roleForm.permissions = next;
};

const clearPermissions = () => {
  treeRef.value?.setCheckedKeys([], false);
  roleForm.permissions = [];
};

const onRoleNameInput = () => {
  roleFormRef.value?.validateField?.('name').catch(() => undefined);
};

const onUsernameInput = () => {
  userFormRef.value?.validateField?.('username').catch(() => undefined);
};

const onEmailInput = () => {
  emailTouched.value = true;
  userFormRef.value?.validateField?.('email').catch(() => undefined);
};

const onPhoneInput = () => {
  phoneTouched.value = true;
  userFormRef.value?.validateField?.('phone').catch(() => undefined);
};

const handleRoleBeforeClose = async (done: () => void) => {
  if (actionLoading.value) return;
  if (!hasRoleDialogUnsavedChanges()) {
    done();
    return;
  }
  try {
    await confirmDiscardDialogChanges();
    done();
  } catch {
    return;
  }
};

const closeRoleDialog = async () => {
  if (actionLoading.value) return;
  if (!hasRoleDialogUnsavedChanges()) {
    roleDialog.visible = false;
    return;
  }
  try {
    await confirmDiscardDialogChanges();
    roleDialog.visible = false;
  } catch {
    return;
  }
};

const openRoleForm = (row?: RoleDetail) => {
  roleNameChecking.value = false;
  roleNameDuplicate.value = false;
  permissionKeyword.value = '';
  if (row) {
    Object.assign(roleForm, row);
    roleDialog.title = t('system.role.createRoleTitle');
    roleDialog.title = t('system.role.editRoleTitle');
  } else {
    Object.assign(roleForm, { id: '', name: '', description: '', scope: 'tenant', permissions: [] });
    roleDialog.title = t('system.role.createRoleTitle');
  }
  roleDialog.visible = true;
  requestAnimationFrame(async () => {
    treeRef.value?.setCheckedKeys(roleForm.permissions, false);
    await nextTick();
    roleDialogSnapshot.value = serializeRoleForm();
  });
};

const submitRole = () => {
  if (!canManageRoles.value) return;
  roleFormRef.value?.validate(async (valid: boolean) => {
    if (!valid || roleNameChecking.value || roleNameDuplicate.value) return;
    actionLoading.value = true;
    try {
      if (roleForm.id) {
        await updateRole(roleForm.id, roleForm);
        showSuccess(t('system.role.roleUpdated'));
      } else {
        await createRole(roleForm);
        showSuccess(t('system.role.roleCreated'));
      }
      roleDialog.visible = false;
      roleDialogSnapshot.value = serializeRoleForm();
      await fetchRoles();
      await fetchUsers();
    } catch (e) {
      showAuthError(e, t('system.role.roleSaveFail'));
    } finally {
      actionLoading.value = false;
    }
  });
};

const removeRole = async (row: RoleDetail) => {
  if (!canManageRoles.value || cannotDeleteRole(row)) return;
  actionLoading.value = true;
  try {
    await deleteRole(row.id);
    showSuccess(t('system.role.roleDeleted'));
    await fetchRoles();
    await fetchUsers();
  } catch (e) {
    showAuthError(e, t('system.role.roleDeleteFail'));
  } finally {
    actionLoading.value = false;
  }
};

const batchDeleteRoles = async () => {
  if (!canManageRoles.value || bulkRoleDeleteDisabled.value) return;
  try {
    await ElMessageBox.confirm(t('system.role.batchDeleteConfirm'), t('system.role.batchDeleteTitle'), { type: 'warning' });
    actionLoading.value = true;
    const ids = selectedRoleRows.value.map((item) => item.id);
    for (const id of ids) {
      await deleteRole(id);
    }
    showSuccess(t('system.role.batchDeleteOk'));
    selectedRoleRows.value = [];
    await fetchRoles();
    await fetchUsers();
  } catch (e: any) {
    if (e === 'cancel' || e === 'close') return;
    showAuthError(e, t('system.role.batchDeleteFail'));
  } finally {
    actionLoading.value = false;
  }
};

const openAssign = (row: RoleDetail) => {
  if (!canManageAssignments.value) return;
  assignDialog.roleId = row.id;
  assignDialog.roleName = row.name;
  assignDialog.visible = true;
  assignPage.value = 1;
  selectedAssignUsers.value = [];
  fetchAssignUsers();
};

const fetchAssignUsers = async () => {
  if (!canReadUsers.value) {
    assignRows.value = [];
    assignTotal.value = 0;
    return;
  }
  assignLoading.value = true;
  try {
    const { data } = await listUsers({ page: assignPage.value, pageSize: assignPageSize.value, keyword: assignKeyword.value });
    assignRows.value = data.data.items || [];
    assignTotal.value = data.data.total || 0;
  } finally {
    assignLoading.value = false;
  }
};

const onAssignSelectionChange = (rows: User[]) => {
  selectedAssignUsers.value = rows;
};

const submitAssign = async () => {
  if (!canManageAssignments.value || !assignDialog.roleId) return;
  if (!selectedAssignUsers.value.length) {
    showWarning(t('system.role.assignWarn'));
    return;
  }
  assignSubmitting.value = true;
  try {
    await assignRoleUsers(assignDialog.roleId, selectedAssignUsers.value.map((item) => item.id));
    showSuccess(t('system.role.assignOk'));
    assignDialog.visible = false;
    await fetchUsers();
    await fetchRoles();
  } catch (e) {
    showAuthError(e, t('system.role.assignFail'));
  } finally {
    assignSubmitting.value = false;
  }
};

const resetUserForm = () => {
  Object.assign(userForm, {
    username: '',
    displayName: '',
    password: '',
    email: '',
    phone: '',
    status: 'active',
    roles: []
  });
  showPassword.value = false;
  passwordTouched.value = false;
  emailTouched.value = false;
  phoneTouched.value = false;
  usernameChecking.value = false;
  usernameDuplicate.value = false;
};

const openUserDialog = () => {
  if (!canManageUsers.value) return;
  userDialog.title = t('system.role.createUserTitle');
  editingUserId.value = '';
  editingSnapshot.value = null;
  createAndContinue.value = false;
  resetUserForm();
  userDialog.visible = true;
  userDialogSnapshot.value = serializeUserForm();
  userFormRef.value?.clearValidate?.();
};

const handleUserBeforeClose = async (done: () => void) => {
  if (userActionLoading.value) return;
  if (!hasUserDialogUnsavedChanges()) {
    done();
    return;
  }
  try {
    await confirmDiscardDialogChanges();
    done();
  } catch {
    return;
  }
};

const closeUserDialog = async () => {
  if (userActionLoading.value) return;
  if (!hasUserDialogUnsavedChanges()) {
    userDialog.visible = false;
    editingUserId.value = '';
    editingSnapshot.value = null;
    return;
  }
  try {
    await confirmDiscardDialogChanges();
    userDialog.visible = false;
    editingUserId.value = '';
    editingSnapshot.value = null;
  } catch {
    return;
  }
};

const loadUserRoleSelection = async (user: User) => {
  if (!canReadAssignments.value) {
    userForm.roles = [];
    return;
  }
  const principalId = userPrincipalId(user.id);
  const nameToId = roleNameIdMap();
  const { data } = await listAssignments(principalId, { page: 1, pageSize: 200 });
  userForm.roles = Array.from(
    new Set(
      (data.data.items || [])
        .map((item) => nameToId.get(String(item.roleName || '').trim()) || '')
        .filter(Boolean)
    )
  );
};

const openEditUser = async (row: User) => {
  if (!canManageUsers.value) return;
  userDialog.title = t('system.role.editUserTitle');
  editingUserId.value = row.id;
  editingSnapshot.value = {
    username: row.username || '',
    displayName: row.displayName || '',
    email: row.email || '',
    status: row.status || 'active'
  };
  Object.assign(userForm, {
    username: row.username,
    displayName: row.displayName,
    password: '',
    email: row.email || '',
    phone: row.phone || '',
    status: row.status || 'active',
    roles: []
  });
  await loadUserRoleSelection(row);
  userDialog.visible = true;
  passwordTouched.value = false;
  emailTouched.value = false;
  phoneTouched.value = false;
  userDialogSnapshot.value = serializeUserForm();
  userFormRef.value?.clearValidate?.();
};

const syncUserRbacRoles = async (userId: string, selectedRoleIds: string[]) => {
  if (!canManageAssignments.value) return;
  const principalId = userPrincipalId(userId);
  const selectedIds = (selectedRoleIds || []).map((id) => String(id || '').trim()).filter(Boolean);
  const idToName = roleIdNameMap();
  const managedRoleNames = new Set(Array.from(idToName.values()));
  const targetNames = new Set(
    selectedIds
      .map((id) => idToName.get(id) || '')
      .map((name) => String(name || '').trim())
      .filter(Boolean)
  );
  const { data } = await listAssignments(principalId, { page: 1, pageSize: 200 });
  const existing = data.data.items || [];
  const managedExisting = existing.filter((item) => managedRoleNames.has(String(item.roleName || '').trim()));
  for (const item of managedExisting) {
    const roleName = String(item.roleName || '').trim();
    if (!targetNames.has(roleName) && item.id) {
      await deleteAssignment(item.id);
    }
  }
  const existingNames = new Set(managedExisting.map((item) => String(item.roleName || '').trim()).filter(Boolean));
  for (const roleName of targetNames) {
    if (!existingNames.has(roleName)) {
      await createAssignment({ principalId, roleName });
    }
  }
};

const submitUser = () => {
  if (!canManageUsers.value) return;
  userFormRef.value?.validate(async (valid: boolean) => {
    if (!valid || usernameChecking.value || usernameDuplicate.value) return;
    userActionLoading.value = true;
    try {
      if (editingUserId.value) {
        const snapshot = editingSnapshot.value;
        const current = {
          username: (userForm.username || '').trim(),
          displayName: (userForm.displayName || '').trim(),
          email: (userForm.email || '').trim(),
          status: (userForm.status || 'active').trim()
        };
        const before = {
          username: (snapshot?.username || '').trim(),
          displayName: (snapshot?.displayName || '').trim(),
          email: (snapshot?.email || '').trim(),
          status: (snapshot?.status || 'active').trim()
        };
        const changed =
          current.username !== before.username ||
          current.displayName !== before.displayName ||
          current.email !== before.email ||
          current.status !== before.status ||
          !!(userForm.password && userForm.password.trim());

        if (changed) {
          await updateUser(editingUserId.value, {
            username: userForm.username,
            displayName: userForm.displayName,
            email: userForm.email,
            phone: userForm.phone,
            status: userForm.status,
            password: userForm.password || undefined
          });
        }
        await syncUserRbacRoles(editingUserId.value, userForm.roles || []);
        showSuccess(t('system.role.userUpdated'));
        userDialog.visible = false;
        editingUserId.value = '';
        editingSnapshot.value = null;
      } else {
        const { data } = await createUser(userForm as UserPayload);
        const newId = (data as any)?.data?.id || (data as any)?.data?.user?.id || (data as any)?.id;
        if (newId) {
          await syncUserRbacRoles(String(newId), userForm.roles || []);
        }
        showSuccess(t('system.role.userCreated'));
        if (createAndContinue.value) {
          const keepRoles = [...(userForm.roles || [])];
          const keepStatus = userForm.status || 'active';
          resetUserForm();
          userForm.roles = keepRoles;
          userForm.status = keepStatus;
          userDialogSnapshot.value = serializeUserForm();
          userFormRef.value?.clearValidate?.();
        } else {
          userDialog.visible = false;
          editingUserId.value = '';
          editingSnapshot.value = null;
        }
      }
      await fetchUsers();
      await fetchRoles();
    } catch (e) {
      showAuthError(e, t('system.role.userSaveFail'));
    } finally {
      userActionLoading.value = false;
    }
  });
};

const removeUser = async (row: User) => {
  if (!canManageUsers.value || isProtectedUser(row)) return;
  try {
    await ElMessageBox.confirm(t('system.role.userDeleteConfirm', { name: row.username }), t('system.role.userDeleteTitle'), { type: 'warning' });
    userActionLoading.value = true;
    await deleteUser(row.id);
    showSuccess(t('system.role.userDeleted'));
    await fetchUsers();
    await fetchRoles();
  } catch (e: any) {
    if (e === 'cancel' || e === 'close') return;
    showAuthError(e, t('system.role.userDeleteFail'));
  } finally {
    userActionLoading.value = false;
  }
};

const batchUpdateUserStatus = async (status: 'active' | 'disabled') => {
  if (!canManageUsers.value || !selectedUserRows.value.length) return;
  try {
    await ElMessageBox.confirm(t('system.role.batchStatusConfirm', { action: status === 'active' ? t('system.role.enableBtn') : t('system.role.disableBtn') }), t('system.role.batchStatusTitle'), {
      type: 'warning'
    });
    userActionLoading.value = true;
    for (const user of selectedUserRows.value) {
      if (isProtectedUser(user) && status === 'disabled') continue;
      await updateUserStatus(user.id, { status });
    }
    showSuccess(t('system.role.batchStatusOk'));
    await fetchUsers();
  } catch (e: any) {
    if (e === 'cancel' || e === 'close') return;
    showAuthError(e, t('system.role.batchStatusFail'));
  } finally {
    userActionLoading.value = false;
  }
};

const batchDeleteUsers = async () => {
  if (!canManageUsers.value || !selectedUserRows.value.length) return;
  const removable = selectedUserRows.value.filter((item) => !isProtectedUser(item));
  if (!removable.length) {
    showWarning(t('system.role.protectedWarn'));
    return;
  }
  try {
    await ElMessageBox.confirm(t('system.role.batchDeleteUserConfirm', { count: removable.length }), t('system.role.batchDeleteUserTitle'), {
      type: 'warning'
    });
    userActionLoading.value = true;
    for (const user of removable) {
      await deleteUser(user.id);
    }
    showSuccess(t('system.role.batchDeleteUserOk'));
    await fetchUsers();
    await fetchRoles();
  } catch (e: any) {
    if (e === 'cancel' || e === 'close') return;
    showAuthError(e, t('system.role.batchDeleteUserFail'));
  } finally {
    userActionLoading.value = false;
  }
};

const generateTempPassword = () => {
  const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789!@#$%&*';
  let pass = '';
  for (let i = 0; i < 14; i += 1) {
    pass += alphabet[Math.floor(Math.random() * alphabet.length)];
  }
  return `${pass}aA1!`;
};

const resetUserPassword = async (row: User) => {
  if (!canManageUsers.value) return;
  try {
    await ElMessageBox.confirm(t('system.role.resetPwdConfirm', { name: row.username }), t('system.role.resetPwdTitle'), { type: 'warning' });
    userActionLoading.value = true;
    const temporaryPassword = generateTempPassword();
    await updateUser(row.id, {
      password: temporaryPassword,
      mustChangePassword: true
    } as UserPayload);
    showSuccess(t('system.role.resetPwdOk', { pwd: temporaryPassword }));
  } catch (e: any) {
    if (e === 'cancel' || e === 'close') return;
    showAuthError(e, t('system.role.resetPwdFail'));
  } finally {
    userActionLoading.value = false;
  }
};

const toggleUserStatus = async (row: User) => {
  if (!canManageUsers.value) return;
  if (isProtectedUser(row) && row.status === 'active') {
    showWarning(t('system.role.protectedDisable'));
    return;
  }
  const target = row.status === 'active' ? 'disabled' : 'active';
  try {
    await ElMessageBox.confirm(
      t('system.role.toggleConfirm', { name: row.username, action: target === 'active' ? t('system.role.enableBtn') : t('system.role.disableBtn') }),
      t('system.role.toggleTitle'),
      { type: 'warning' }
    );
    userActionLoading.value = true;
    await updateUserStatus(row.id, { status: target });
    showSuccess(t('system.role.toggleOk', { action: target === 'active' ? t('system.role.enableBtn') : t('system.role.disableBtn') }));
    await fetchUsers();
  } catch (e: any) {
    if (e === 'cancel' || e === 'close') return;
    showAuthError(e, t('system.role.toggleFail'));
  } finally {
    userActionLoading.value = false;
  }
};

watch(
  () => roleForm.name,
  (value) => {
    if (!roleDialog.visible) return;
    checkRoleNameDuplicate(value);
  }
);

watch(permissionKeyword, (value) => {
  treeRef.value?.filter(value);
});

watch(
  () => userForm.username,
  (value) => {
    if (!userDialog.visible) return;
    checkUsernameDuplicate(value);
  }
);

onMounted(async () => {
  await Promise.all([fetchRoles(), loadPermissionTree()]);
  await fetchUsers();
});

watch(
  () => tenantStore.currentTenantId,
  async () => {
    rolePage.value = 1;
    userPage.value = 1;
    linkedRoleFilterId.value = '';
    systemView.setRole({ page: 1 });
    systemView.setUser({ page: 1, roleId: '' });
    await Promise.all([fetchRoles(), fetchUsers()]);
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

.module-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
}

.module-title {
  font-weight: 600;
  font-size: 16px;
  line-height: 24px;
}

.module-subtitle {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  margin-top: 2px;
}

.toolbar-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.user-toolbar {
  margin-bottom: 8px;
}

.bulk-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  padding: 8px 10px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 8px;
  background: var(--el-fill-color-extra-light);
}

.link-tip {
  margin-bottom: 12px;
}

.data-table :deep(.el-table__cell .cell) {
  white-space: nowrap;
}

.pager-row {
  margin-top: 12px;
  display: flex;
  justify-content: flex-end;
}

.form-section-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  padding: 0 0 12px;
}

.form-section-divider {
  height: 1px;
  background: var(--el-border-color-lighter);
  margin: 6px 0 16px;
}

.field-wrap {
  width: 100%;
}

.form-help {
  margin-top: 4px;
  font-size: 12px;
  line-height: 18px;
  color: var(--el-text-color-secondary);
}

.inline-state {
  margin-top: 4px;
  font-size: 12px;
  color: var(--el-color-primary);
}

.inline-state.danger {
  color: var(--el-color-danger);
}

.actions-wrap {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.permission-tree {
  max-height: 280px;
  overflow: auto;
  padding: 8px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 8px;
}

.permission-preview {
  min-height: 120px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.preview-tag {
  max-width: 100%;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.fixed-actions {
  width: 100%;
}

.assign-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  gap: 8px;
}

.full-width {
  width: 100%;
}

.w-140 {
  width: 140px;
}

.w-200 {
  width: 200px;
}

.w-260 {
  width: 260px;
}

.w-280 {
  width: 280px;
}

.w-300 {
  width: 300px;
}

.strength-weak {
  color: var(--el-color-danger);
}

.strength-medium {
  color: var(--el-color-warning);
}

.strength-strong {
  color: var(--el-color-success);
}

.password-visibility {
  padding: 0;
}

.password-visibility .el-icon {
  font-size: 16px;
}

.standard-form :deep(.el-form-item) {
  margin-bottom: 18px;
}

.standard-form :deep(.el-form-item__label) {
  width: 110px !important;
  justify-content: flex-end;
  padding-right: 12px;
  white-space: nowrap;
}

.standard-form :deep(.el-form-item__content) {
  margin-left: 0 !important;
}

.standard-form :deep(.el-input),
.standard-form :deep(.el-select),
.standard-form :deep(.el-input-number),
.standard-form :deep(.el-textarea),
.standard-form :deep(.el-date-editor),
.standard-form :deep(.el-switch) {
  width: 100%;
}

.standard-form :deep(.el-input__wrapper),
.standard-form :deep(.el-select__wrapper),
.standard-form :deep(.el-input-number),
.standard-form :deep(.el-textarea__inner) {
  min-height: 36px;
  border-radius: 4px;
}

:deep(.spec-dialog.el-dialog) {
  border-radius: 8px;
}

:deep(.spec-dialog .el-dialog__body) {
  padding: 32px;
}

:deep(.spec-dialog .el-dialog__header) {
  padding: 32px 32px 0;
}

:deep(.spec-dialog .el-dialog__title) {
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
}

:deep(.spec-dialog .el-dialog__footer) {
  padding: 12px 32px 32px;
  border-top: 1px solid var(--el-border-color-lighter);
}

:deep(.spec-dialog .el-form-item__label) {
  width: 110px;
  text-align: right;
  padding-right: 12px;
}

@media (max-width: 900px) {
  .page {
    padding: 12px;
  }

  .module-header {
    flex-direction: column;
    align-items: stretch;
  }

  .toolbar-row {
    flex-direction: column;
    align-items: stretch;
  }

  .w-140,
  .w-200,
  .w-260,
  .w-280,
  .w-300 {
    width: 100%;
  }

  :deep(.spec-dialog.el-dialog) {
    width: calc(100vw - 24px) !important;
    margin: 12px auto;
  }

  :deep(.spec-dialog .el-dialog__body) {
    padding: 20px;
  }

  :deep(.spec-dialog .el-dialog__header),
  :deep(.spec-dialog .el-dialog__footer) {
    padding-left: 20px;
    padding-right: 20px;
  }
}
</style>
