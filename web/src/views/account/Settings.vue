<template>
  <div class="settings-page">
    <el-card class="section-card" shadow="never">
      <template #header>
        <div class="section-header">
          <span class="section-title">账号基础信息</span>
          <span class="section-desc">展示当前登录账号的基本身份与登录信息</span>
        </div>
      </template>
      <div class="info-grid">
        <div class="info-item">
          <div class="info-label">登录账号</div>
          <div class="info-value">{{ accountInfo.username }}</div>
        </div>
        <div class="info-item">
          <div class="info-label">用户角色</div>
          <div class="info-value">{{ accountInfo.roles }}</div>
        </div>
        <div class="info-item">
          <div class="info-label">账号创建时间</div>
          <div class="info-value">{{ accountInfo.createdAt }}</div>
        </div>
        <div class="info-item">
          <div class="info-label">最后登录时间</div>
          <div class="info-value">{{ accountInfo.lastLoginAt }}</div>
        </div>
        <div class="info-item full-width">
          <div class="info-label">最近登录 IP</div>
          <div class="info-value">{{ accountInfo.lastLoginIp }}</div>
        </div>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <div class="section-header">
          <span class="section-title">{{ t('settings.passwordTitle') }}</span>
          <span class="section-desc">{{ t('settings.passwordDesc') }}</span>
        </div>
      </template>

      <el-form
        ref="pwdFormRef"
        :model="pwdForm"
        :rules="pwdRules"
        label-width="120px"
        label-position="right"
        class="password-form"
      >
        <el-form-item prop="currentPassword" :label="t('settings.currentPassword')" required>
          <el-input
            v-model="pwdForm.currentPassword"
            type="password"
            show-password
            autocomplete="current-password"
            maxlength="64"
            class="input-max"
            @input="validateField('currentPassword')"
          />
        </el-form-item>

        <el-form-item
          prop="newPassword"
          :label="t('settings.newPassword')"
          required
          :error="newPasswordInlineError"
        >
          <div class="field-stack input-max">
            <el-input
              v-model="pwdForm.newPassword"
              type="password"
              show-password
              autocomplete="new-password"
              maxlength="64"
              @input="validateField('newPassword')"
            />
            <div class="strength-row">
              <span class="strength-label">密码强度：{{ strengthText }}</span>
              <el-progress
                :percentage="strengthPercent"
                :stroke-width="8"
                :color="strengthColor"
                :show-text="false"
              />
            </div>
          </div>
        </el-form-item>

        <el-form-item
          prop="confirmPassword"
          :label="t('settings.confirmPassword')"
          required
          :error="confirmInlineError"
        >
          <el-input
            v-model="pwdForm.confirmPassword"
            type="password"
            show-password
            autocomplete="new-password"
            maxlength="64"
            class="input-max"
            @input="validateField('confirmPassword')"
          />
        </el-form-item>

        <el-form-item class="actions-item">
          <el-button type="primary" :loading="pwdPending" @click="submitPassword">{{
            t('common.save')
          }}</el-button>
          <el-button :disabled="pwdPending" @click="resetPasswordForm">{{
            t('common.reset')
          }}</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <div class="section-header">
          <span class="section-title">{{ t('settings.preferencesTitle') }}</span>
          <span class="section-desc">{{ t('settings.preferencesDesc') }}</span>
        </div>
      </template>

      <div class="prefs">
        <div class="pref-row">
          <div class="pref-main">
            <div class="pref-label">通知总开关</div>
            <div class="pref-tip">关闭后将停止所有通知提醒</div>
          </div>
          <el-switch v-model="prefs.notifyEnabled" @change="savePrefs" />
        </div>

        <div class="pref-row">
          <div class="pref-main">
            <div class="pref-label">{{ t('settings.notifyEmail') }}</div>
            <div class="pref-tip">{{ t('settings.notifyTip') }}</div>
          </div>
          <el-switch v-model="prefs.email" :disabled="!prefs.notifyEnabled" @change="savePrefs" />
        </div>

        <div class="pref-row">
          <div class="pref-main">
            <div class="pref-label">{{ t('settings.notifyInapp') }}</div>
            <div class="pref-tip">{{ t('settings.inappTip') }}</div>
          </div>
          <el-switch v-model="prefs.inapp" :disabled="!prefs.notifyEnabled" @change="savePrefs" />
        </div>

        <div class="pref-row">
          <div class="pref-main">
            <div class="pref-label">安全告警通知</div>
            <div class="pref-tip">登录异常、权限异常、风险操作提醒</div>
          </div>
          <el-switch v-model="prefs.security" :disabled="!prefs.notifyEnabled" @change="savePrefs" />
        </div>

        <div class="pref-row">
          <div class="pref-main">
            <div class="pref-label">任务执行通知</div>
            <div class="pref-tip">导入导出、批量任务、自动化任务状态提醒</div>
          </div>
          <el-switch v-model="prefs.task" :disabled="!prefs.notifyEnabled" @change="savePrefs" />
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { type FormInstance, type FormRules } from 'element-plus';
import { showError, showSuccess } from '@/shared/errors/messageToast';
import { useI18n } from 'vue-i18n';
import { changePassword } from '@/api/auth';
import { useAuthStore } from '@/modules/auth/store';
import { listUsers } from '@/api/system/user';
import { listSessions } from '@/api/sessions';

const { t } = useI18n();
const authStore = useAuthStore();

const pwdFormRef = ref<FormInstance>();
const pwdForm = reactive({ currentPassword: '', newPassword: '', confirmPassword: '' });
const pwdPending = ref(false);

const prefKey = 'mdhcp-user-prefs';

const prefs = reactive<{
  notifyEnabled: boolean;
  email: boolean;
  inapp: boolean;
  security: boolean;
  task: boolean;
}>({
  notifyEnabled: true,
  email: true,
  inapp: true,
  security: true,
  task: true
});

const accountInfo = reactive({
  username: '-',
  roles: '-',
  createdAt: '-',
  lastLoginAt: '-',
  lastLoginIp: '-'
});

const formatTime = (value?: string) => {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
};

const passwordScore = computed(() => {
  const value = pwdForm.newPassword || '';
  let score = 0;
  if (value.length >= 8) score += 1;
  if (/[A-Z]/.test(value)) score += 1;
  if (/[a-z]/.test(value)) score += 1;
  if (/\d/.test(value)) score += 1;
  if (/[^A-Za-z0-9]/.test(value)) score += 1;
  return score;
});

const strengthPercent = computed(() => Math.round((passwordScore.value / 5) * 100));
const strengthText = computed(() => {
  if (!pwdForm.newPassword) return '未设置';
  if (passwordScore.value <= 2) return '弱';
  if (passwordScore.value <= 3) return '中';
  return '强';
});
const strengthColor = computed(() => {
  if (passwordScore.value <= 2) return '#ef4444';
  if (passwordScore.value <= 3) return '#f59e0b';
  return '#22c55e';
});

const newPasswordInlineError = computed(() => {
  if (!pwdForm.newPassword) return '';
  if (pwdForm.newPassword.length < 8) return t('settings.passwordMin');
  if (passwordScore.value <= 2) return '密码强度较弱，建议包含大小写字母、数字与符号';
  return '';
});

const confirmInlineError = computed(() => {
  if (!pwdForm.confirmPassword) return '';
  return pwdForm.confirmPassword !== pwdForm.newPassword ? t('settings.passwordMismatch') : '';
});

const validateField = (field: 'currentPassword' | 'newPassword' | 'confirmPassword') => {
  pwdFormRef.value?.validateField(field).catch(() => undefined);
};

const pwdRules: FormRules = {
  currentPassword: [{ required: true, message: t('settings.required'), trigger: ['blur', 'change'] }],
  newPassword: [
    { required: true, message: t('settings.required'), trigger: ['blur', 'change'] },
    { min: 8, message: t('settings.passwordMin'), trigger: ['blur', 'change'] }
  ],
  confirmPassword: [
    { required: true, message: t('settings.required'), trigger: ['blur', 'change'] },
    {
      validator: (_rule: unknown, value: string, callback: (error?: Error) => void) => {
        if (value !== pwdForm.newPassword) callback(new Error(t('settings.passwordMismatch')));
        else callback();
      },
      trigger: ['blur', 'change']
    }
  ]
};

const savePrefs = () => {
  localStorage.setItem(prefKey, JSON.stringify({ ...prefs }));
  showSuccess('偏好已自动保存');
};

const loadPrefs = () => {
  try {
    const saved = JSON.parse(localStorage.getItem(prefKey) || '{}');
    if (typeof saved.notifyEnabled === 'boolean') prefs.notifyEnabled = saved.notifyEnabled;
    if (typeof saved.email === 'boolean') prefs.email = saved.email;
    if (typeof saved.inapp === 'boolean') prefs.inapp = saved.inapp;
    if (typeof saved.security === 'boolean') prefs.security = saved.security;
    if (typeof saved.task === 'boolean') prefs.task = saved.task;
  } catch (err) {
    console.warn('Failed to load prefs', err);
  }
};

const resetPasswordForm = () => {
  pwdForm.currentPassword = '';
  pwdForm.newPassword = '';
  pwdForm.confirmPassword = '';
  pwdFormRef.value?.clearValidate();
};

const submitPassword = async () => {
  if (!pwdFormRef.value) return;
  const valid = await pwdFormRef.value.validate().catch(() => false);
  if (!valid) return;
  if (newPasswordInlineError.value || confirmInlineError.value) return;
  pwdPending.value = true;
  try {
    await changePassword({
      currentPassword: pwdForm.currentPassword,
      newPassword: pwdForm.newPassword
    });
    showSuccess(t('settings.passwordUpdated'));
    resetPasswordForm();
  } catch (error) {
    console.error(error);
    showError(t('settings.passwordFail'));
  } finally {
    pwdPending.value = false;
  }
};

const loadAccountInfo = async () => {
  const username = authStore.user?.username || authStore.lastUsername || '-';
  accountInfo.username = username;
  accountInfo.roles =
    authStore.user?.roles?.map((role) => role.name).filter(Boolean).join('、') ||
    authStore.user?.displayName ||
    '-';

  try {
    const [usersRes, sessionsRes] = await Promise.all([
      listUsers({ page: 1, pageSize: 200, keyword: username, status: '' }),
      listSessions({ page: 1, pageSize: 200, keyword: username, status: '' })
    ]);

    const users = usersRes?.data?.data?.items || [];
    const matchedUser = users.find((item) => item.username === username) || users[0];
    if (matchedUser) {
      accountInfo.roles =
        matchedUser.roles?.map((role) => role.name).filter(Boolean).join('、') || accountInfo.roles;
      accountInfo.createdAt = formatTime((matchedUser as any).createdAt);
      accountInfo.lastLoginAt = formatTime(matchedUser.lastLoginAt);
    }

    const sessions = sessionsRes?.data?.data?.items || [];
    const mine = sessions.filter((item) => item.username === username);
    const sorted = [...mine].sort(
      (a, b) => new Date(b.lastSeenAt).getTime() - new Date(a.lastSeenAt).getTime()
    );
    if (sorted.length) {
      accountInfo.lastLoginAt = formatTime(sorted[0].lastSeenAt || sorted[0].createdAt);
      accountInfo.lastLoginIp = sorted[0].ip || '-';
      if (accountInfo.createdAt === '-') {
        accountInfo.createdAt = formatTime(sorted[sorted.length - 1].createdAt);
      }
    }
  } catch (error) {
    console.warn('load account info failed', error);
  }
};

onMounted(() => {
  loadPrefs();
  loadAccountInfo();
});
</script>

<style scoped>
.settings-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
  min-height: 100%;
  background: #f5f7fa;
}

.section-card {
  border-radius: 10px;
  border: 1px solid #e5e7eb;
}

.section-card :deep(.el-card__header) {
  padding: 14px 18px;
  border-bottom: 1px solid #eef1f6;
}

.section-card :deep(.el-card__body) {
  padding: 16px 18px;
}

.section-header {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.section-title {
  font-size: 16px;
  font-weight: 600;
  color: #111827;
}

.section-desc {
  color: #6b7280;
  font-size: 13px;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.info-item {
  padding: 10px 12px;
  border: 1px solid #eef1f6;
  border-radius: 8px;
  background: #fff;
}

.info-item.full-width {
  grid-column: 1 / -1;
}

.info-label {
  color: #6b7280;
  font-size: 13px;
}

.info-value {
  margin-top: 6px;
  color: #111827;
  font-weight: 600;
  word-break: break-all;
}

.password-form {
  max-width: 760px;
}

.input-max {
  max-width: 450px;
}

.field-stack {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.strength-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.strength-label {
  color: #4b5563;
  font-size: 12px;
}

.actions-item :deep(.el-form-item__content) {
  margin-left: 120px !important;
  justify-content: flex-start;
  gap: 8px;
}

.prefs {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.pref-row {
  display: grid;
  grid-template-columns: 1fr auto;
  align-items: center;
  gap: 16px;
  min-height: 48px;
  padding: 6px 0;
}

.pref-main {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.pref-label {
  font-weight: 500;
  color: #111827;
}

.pref-tip {
  color: #6b7280;
  font-size: 13px;
}

@media (max-width: 900px) {
  .info-grid {
    grid-template-columns: 1fr;
  }

  .actions-item :deep(.el-form-item__content) {
    margin-left: 0 !important;
  }

  .pref-row {
    grid-template-columns: 1fr;
    align-items: flex-start;
  }
}
</style>
