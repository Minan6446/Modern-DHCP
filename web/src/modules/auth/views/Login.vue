<template>
  <div class="login-page">
    <div class="bg-topology"></div>

    <div class="login-card">
      <div class="brand-section">
        <img class="brand-img" :src="logo" alt="Modern DHCP" />
        <div class="brand-text">
          <div class="title-row">
            <h1 class="app-title">{{ t('app.title') }}</h1>
          </div>
          <p class="app-subtitle">{{ t('login.subtitle') }}</p>
        </div>
      </div>

      <div class="form-title">{{ t('login.loginTitle') }}</div>

      <el-alert v-if="topError" class="top-error" type="error" :closable="false" show-icon :title="topError" />
      <el-alert v-if="lockedNotice" class="top-error" type="warning" :closable="false" show-icon :title="lockedNotice" />

      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-position="top"
        class="login-form"
        hide-required-asterisk
        :disabled="submitting"
        @keyup.enter="handleSubmit"
      >
        <el-form-item :label="t('login.username')" prop="username">
          <el-input
            v-model.trim="form.username"
            autocomplete="username"
            clearable
            :prefix-icon="User"
            :placeholder="t('login.usernamePlaceholder')"
            @input="onUsernameChange"
          />
        </el-form-item>

        <el-form-item :label="t('login.password')" prop="password">
          <el-input
            v-model="form.password"
            :type="passwordVisible ? 'text' : 'password'"
            autocomplete="current-password"
            :prefix-icon="Lock"
            :placeholder="t('login.passwordPlaceholder')"
            @input="clearTopError"
          >
            <template #suffix>
              <el-button text class="pw-toggle" :disabled="submitting" @click="togglePasswordVisible">
                <el-icon><View v-if="!passwordVisible" /><Hide v-else /></el-icon>
              </el-button>
            </template>
          </el-input>
        </el-form-item>

        <div class="form-actions">
          <el-checkbox v-model="form.remember" :disabled="submitting">{{ t('login.remember') }}</el-checkbox>
          <el-link type="primary" :underline="false" @click="openResetDialog">{{ t('login.forgot') }}</el-link>
        </div>

        <el-button class="login-btn" type="primary" :loading="submitting" :disabled="submitting || isLocked" @click="handleSubmit">
          {{ submitting ? t('login.loggingIn') : t('login.login') }}
        </el-button>

        <div class="agreement">
          {{ t('login.agreementPrefix') }}
          <el-link type="primary" :underline="false" @click="openAgreement('agreement')">{{ t('login.userAgreement') }}</el-link>
          {{ t('login.and') }}
          <el-link type="primary" :underline="false" @click="openAgreement('privacy')">{{ t('login.privacyPolicy') }}</el-link>
        </div>
      </el-form>

      <div class="footer">
        <div class="support">{{ t('login.support') }}</div>
        <div class="copyright">{{ t('login.copyright') }}</div>
      </div>
    </div>

    <el-dialog v-model="resetVisible" :title="t('login.reset.title')" width="420px" destroy-on-close>
      <el-steps :active="resetStep" simple finish-status="success" class="reset-steps">
        <el-step :title="t('login.reset.step1')" />
        <el-step :title="t('login.reset.step2')" />
        <el-step :title="t('login.reset.step3')" />
      </el-steps>

      <div v-if="resetStep === 0" class="reset-panel">
        <el-form label-position="top">
          <el-form-item :label="t('login.username')">
            <el-input v-model.trim="resetForm.username" :placeholder="t('login.usernamePlaceholder')" />
          </el-form-item>
          <el-form-item :label="t('login.reset.verifyMethod')">
            <el-radio-group v-model="resetForm.method">
              <el-radio-button label="email">{{ t('login.reset.emailVerify') }}</el-radio-button>
              <el-radio-button label="phone">{{ t('login.reset.phoneVerify') }}</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item v-if="resetForm.method === 'email'" :label="t('login.reset.email')">
            <el-input v-model.trim="resetForm.email" :placeholder="t('login.reset.emailPlaceholder')" />
          </el-form-item>
          <el-form-item v-else :label="t('login.reset.phone')">
            <el-input v-model.trim="resetForm.phone" :placeholder="t('login.reset.phonePlaceholder')" />
          </el-form-item>
        </el-form>
      </div>

      <div v-else-if="resetStep === 1" class="reset-panel">
        <el-form label-position="top">
          <el-form-item :label="t('login.reset.verifyCode')">
            <div class="captcha-row">
              <el-input v-model.trim="resetForm.code" maxlength="6" :placeholder="t('login.reset.verifyCodePlaceholder')" />
              <el-button :disabled="countdown > 0" @click="sendResetCode">
                {{ countdown > 0 ? `${countdown}s` : t('login.reset.sendCode') }}
              </el-button>
            </div>
          </el-form-item>
        </el-form>
      </div>

      <div v-else class="reset-panel">
        <el-form label-position="top">
          <el-form-item :label="t('login.reset.newPassword')">
            <el-input v-model="resetForm.newPassword" type="password" show-password :placeholder="t('login.reset.newPasswordPlaceholder')" />
          </el-form-item>
          <el-form-item :label="t('login.reset.confirmPassword')">
            <el-input v-model="resetForm.confirmPassword" type="password" show-password :placeholder="t('login.reset.confirmPasswordPlaceholder')" />
          </el-form-item>
        </el-form>
      </div>

      <template #footer>
        <el-button @click="closeResetDialog">{{ t('login.reset.cancel') }}</el-button>
        <el-button v-if="resetStep > 0" @click="resetStep--">{{ t('login.reset.previous') }}</el-button>
        <el-button type="primary" @click="nextResetStep">{{ resetStep === 2 ? t('login.reset.finish') : t('login.reset.next') }}</el-button>
      </template>
    </el-dialog>

    <AgreementDialog
      v-model="agreementVisible"
      :initial-tab="agreementInitialTab"
      :required="agreementRequiredMode"
      @agreed="handleAgreementAccepted"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { ElMessageBox } from 'element-plus';
import type { FormInstance, FormRules, FormItemRule } from 'element-plus';
import { Hide, Lock, User, View } from '@element-plus/icons-vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useDebounceFn } from '@vueuse/core';
import { useAuthStore } from '@/modules/auth/store';
import {
  completePasswordReset,
  startPasswordReset,
  verifyPasswordResetCode
} from '@/modules/auth/api';
import { usePermissionStore } from '@/store/permission';
import { hasPermission as hasPermissionHelper } from '@/shared/permission';
import { sanitizeRedirect } from '@/shared/security/redirect';
import AgreementDialog from '@/components/auth/AgreementDialog.vue';
import logo from '@/assets/logo.svg';
import { showError, showSuccess } from '@/shared/errors/messageToast';

const USERNAME_REGEX = /^[A-Za-z0-9]{2,50}$/;
const PASSWORD_REGEX = /^(?=.*[A-Za-z])(?=.*\d)[^\s]{6,20}$/;
const LOCK_THRESHOLD = 5;
const LOCK_MS = 10 * 60 * 1000;

const { t } = useI18n();
const auth = useAuthStore();
const permissionStore = usePermissionStore();
const router = useRouter();
const formRef = ref<FormInstance>();
const agreementVisible = ref(false);
const agreementInitialTab = ref<'agreement' | 'privacy'>('agreement');
const agreementRequiredMode = ref(false);
const submitting = ref(false);
const passwordVisible = ref(false);
const topError = ref('');

const form = reactive({
  username: auth.lastUsername || '',
  password: '',
  remember: auth.remember ?? true
});

const guardState = reactive({
  failed: 0,
  lockUntil: 0
});

const serverGuard = reactive({
  lockUntil: 0
});

const effectiveLockUntil = computed(() => Math.max(guardState.lockUntil, serverGuard.lockUntil));
const isLocked = computed(() => effectiveLockUntil.value > Date.now());
const lockedNotice = computed(() => {
  if (!isLocked.value) return '';
  const left = Math.ceil((effectiveLockUntil.value - Date.now()) / 1000);
  return t('login.lockedNotice', { seconds: left });
});

const guardStorageKey = computed(() => `mdhcp:login-guard:${(form.username || 'anonymous').toLowerCase()}`);
const loginMetaStorageKey = computed(() => `mdhcp:last-login:${(form.username || 'anonymous').toLowerCase()}`);

const rules = computed<FormRules>(() => ({
  username: [
    { required: true, message: t('login.rules.usernameRequired'), trigger: ['change', 'blur'] },
    {
      validator: (_: FormItemRule, value: string, cb: (e?: Error) => void) => {
        if (!USERNAME_REGEX.test(value || '')) cb(new Error(t('login.rules.usernameFormat')));
        else cb();
      },
      trigger: ['change', 'blur']
    }
  ],
  password: [
    { required: true, message: t('login.rules.passwordRequired'), trigger: ['change', 'blur'] },
    {
      validator: (_: FormItemRule, value: string, cb: (e?: Error) => void) => {
        if (!PASSWORD_REGEX.test(value || '')) cb(new Error(t('login.rules.passwordFormat')));
        else cb();
      },
      trigger: ['change', 'blur']
    }
  ]
}));

const defaultLandingCandidates = [
  '/dashboard',
  '/pool/ipv4',
  '/lease/active',
  '/binding/mac',
  '/option/list',
  '/monitor/overview',
  '/cluster/overview',
  '/security/access',
  '/system/roles',
  '/system/login-audit',
  '/account/settings'
];

const canAccessTarget = (target: string) => {
  const resolved = router.resolve(target);
  if (!resolved?.matched?.length) return false;
  return resolved.matched.every((record) => {
    const permission = record.meta?.permission as string | string[] | undefined;
    return hasPermissionHelper(permission, permissionStore, auth.user);
  });
};

const resolvePostLoginRedirect = () => {
  const requestedRedirect = sanitizeRedirect(auth.consumePostLoginRedirect());
  if (requestedRedirect && requestedRedirect !== '/' && canAccessTarget(requestedRedirect)) {
    return requestedRedirect;
  }
  return defaultLandingCandidates.find((path) => canAccessTarget(path)) || '/forbidden';
};

const loadGuardState = () => {
  try {
    const parsed = JSON.parse(localStorage.getItem(guardStorageKey.value) || '{}');
    guardState.failed = Number(parsed.failed || 0);
    guardState.lockUntil = Number(parsed.lockUntil || 0);
  } catch {
    guardState.failed = 0;
    guardState.lockUntil = 0;
  }
};

const persistGuardState = () => {
  localStorage.setItem(guardStorageKey.value, JSON.stringify({ failed: guardState.failed, lockUntil: guardState.lockUntil }));
};

const clearGuardState = () => {
  guardState.failed = 0;
  guardState.lockUntil = 0;
  persistGuardState();
};

const onUsernameChange = () => {
  clearTopError();
  loadGuardState();
  serverGuard.lockUntil = 0;
};

const clearTopError = () => {
  topError.value = '';
  auth.error = '';
};

const formatErrorMessage = (err: any) => {
  const status = err?.response?.status as number | undefined;
  const msg = String(err?.response?.data?.message || err?.message || '').toLowerCase();
  if (status === 423 || msg.includes('locked')) return t('login.errors.locked');
  if (status === 401 || msg.includes('invalid')) return t('login.errors.invalid');
  if (status === 429) return t('login.errors.tooMany');
  if (status === 403) return t('login.errors.disabled');
  if (msg.includes('network')) return t('login.errors.network');
  if (msg.includes('timeout')) return t('login.errors.timeout');
  return t('login.errors.generic');
};

const togglePasswordVisible = () => {
  passwordVisible.value = !passwordVisible.value;
};

const showLastLoginDialog = async (serverLastLoginAt?: string) => {
  const current = {
    at: new Date().toISOString(),
    ua: navigator.userAgent,
    ip: t('login.currentTerminal')
  };
  const oldRaw = localStorage.getItem(loginMetaStorageKey.value);
  localStorage.setItem(loginMetaStorageKey.value, JSON.stringify(current));
  if (!oldRaw) return;
  try {
    const previous = JSON.parse(oldRaw) as { at?: string; ua?: string; ip?: string };
    if (serverLastLoginAt) {
      previous.at = serverLastLoginAt;
    }
    const abnormal = !!previous.ua && previous.ua !== current.ua;
    const title = abnormal ? t('login.lastLoginAbnormal') : t('login.lastLoginTitle');
    const unknown = t('login.lastLoginUnknown');
    const detail = `
      <div style="line-height:1.7;">
        <div>${t('login.lastLoginTime', { time: previous.at || unknown })}</div>
        <div>${t('login.lastLoginLocation', { location: previous.ip || unknown })}</div>
        <div style="color:${abnormal ? '#F53F3F' : '#4B5563'};font-weight:${abnormal ? 700 : 400};">${abnormal ? t('login.lastLoginEnvAbnormal') : t('login.lastLoginEnvNormal')}</div>
      </div>
    `;
    await ElMessageBox.alert(detail, title, {
      confirmButtonText: t('login.lastLoginAck'),
      dangerouslyUseHTMLString: true,
      type: abnormal ? 'warning' : 'info'
    });
  } catch {
    // ignore parse errors
  }
};

const handleSubmit = useDebounceFn(async () => {
  if (submitting.value) return;
  clearTopError();

  if (!agreementAccepted.value) {
    topError.value = t('login.mustAgree');
    openAgreement('agreement', true);
    return;
  }

  if (isLocked.value) {
    topError.value = lockedNotice.value;
    return;
  }

  if (!formRef.value) return;
  try {
    await formRef.value.validate();
  } catch {
    topError.value = t('login.formErrors');
    return;
  }

  submitting.value = true;
  try {
    const loginData = await auth.login({
      username: form.username,
      password: form.password,
      remember: form.remember
    });
    clearGuardState();
    serverGuard.lockUntil = 0;
    form.password = '';
    await showLastLoginDialog((loginData as any)?.lastLoginAt);
    await showSuccess(t('login.success'));
    await router.push(resolvePostLoginRedirect());
  } catch (err: any) {
    const responseData = (err as any)?.response?.data || {};
    guardState.failed += 1;
    if (guardState.failed >= LOCK_THRESHOLD) {
      guardState.lockUntil = Date.now() + LOCK_MS;
    }
    persistGuardState();
    if (responseData?.lockUntil) {
      const ts = Date.parse(String(responseData.lockUntil));
      if (!Number.isNaN(ts)) {
        serverGuard.lockUntil = ts;
      }
    }
    topError.value = guardState.lockUntil > Date.now()
      ? t('login.lockedRetry', { seconds: Math.ceil((guardState.lockUntil - Date.now()) / 1000) })
      : formatErrorMessage(err);
    showError(topError.value);
  } finally {
    submitting.value = false;
  }
}, 250);

const resetVisible = ref(false);
const resetStep = ref(0);
const countdown = ref(0);
const resetForm = reactive({
  method: 'email' as 'email' | 'phone',
  username: '',
  email: '',
  phone: '',
  challengeId: '',
  code: '',
  newPassword: '',
  confirmPassword: ''
});
let timer: number | null = null;

const openResetDialog = () => {
  resetVisible.value = true;
  resetStep.value = 0;
  Object.assign(resetForm, {
    method: 'email',
    username: '',
    email: '',
    phone: '',
    challengeId: '',
    code: '',
    newPassword: '',
    confirmPassword: ''
  });
};

const closeResetDialog = () => {
  resetVisible.value = false;
};

const sendResetCode = () => {
  if (!resetForm.username) {
    showError(t('login.reset.usernameMissing'));
    return;
  }
  if (resetForm.method === 'email') {
    if (!resetForm.email || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(resetForm.email)) {
      showError(t('login.reset.emailInvalid'));
      return;
    }
  } else if (!/^1\d{10}$/.test(resetForm.phone)) {
    showError(t('login.reset.phoneInvalid'));
    return;
  }
  startPasswordReset({ username: resetForm.username, email: resetForm.method === 'email' ? resetForm.email : resetForm.phone } as any)
    .then(({ data }) => {
      resetForm.challengeId = String(data?.challengeId || '');
      showSuccess(resetForm.method === 'email' ? t('login.reset.codeSentEmail') : t('login.reset.codeSentPhone'));
      countdown.value = 60;
      if (timer) window.clearInterval(timer);
      timer = window.setInterval(() => {
        countdown.value -= 1;
        if (countdown.value <= 0 && timer) {
          window.clearInterval(timer);
          timer = null;
        }
      }, 1000);
    })
    .catch((err) => {
      showError(String(err?.response?.data?.message || t('login.reset.codeSendFail')));
    });
};

const nextResetStep = () => {
  if (resetStep.value === 0) {
    if (!resetForm.username) return showError(t('login.reset.usernameRequiredField'));
    if (resetForm.method === 'email' && !resetForm.email) return showError(t('login.reset.emailRequired'));
    if (resetForm.method === 'phone' && !resetForm.phone) return showError(t('login.reset.phoneRequired'));
    sendResetCode();
    resetStep.value = 1;
    return;
  }
  if (resetStep.value === 1) {
    if (!resetForm.code) return showError(t('login.reset.codeRequired'));
    verifyPasswordResetCode({ challengeId: resetForm.challengeId, code: resetForm.code })
      .then(() => {
        resetStep.value = 2;
      })
      .catch((err) => {
        showError(String(err?.response?.data?.message || t('login.reset.codeVerifyFail')));
      });
    return;
  }
  if (!PASSWORD_REGEX.test(resetForm.newPassword)) return showError(t('login.reset.passwordWeak'));
  if (resetForm.newPassword !== resetForm.confirmPassword) return showError(t('login.reset.passwordMismatch'));
  completePasswordReset({ challengeId: resetForm.challengeId, newPassword: resetForm.newPassword })
    .then(() => {
      showSuccess(t('login.reset.success'));
      resetVisible.value = false;
    })
    .catch((err) => {
      showError(String(err?.response?.data?.message || t('login.reset.fail')));
    });
};

watch(
  () => form.username,
  () => {
    loadGuardState();
  }
);

onMounted(() => {
  loadGuardState();
  if (!agreementAccepted.value) {
    openAgreement('agreement', true);
  }
  nextTick(() => {
    const usernameInput = document.querySelector('input[autocomplete="username"]') as HTMLInputElement | null;
    if (usernameInput && !usernameInput.value) usernameInput.focus();
  });
});

onUnmounted(() => {
  if (timer) {
    window.clearInterval(timer);
    timer = null;
  }
});

const AGREEMENT_STORAGE_KEY = 'mdhcp:agreement:accepted';
const agreementAccepted = ref(localStorage.getItem(AGREEMENT_STORAGE_KEY) === '1');

const openAgreement = (tab: 'agreement' | 'privacy', required = false) => {
  agreementInitialTab.value = tab;
  agreementRequiredMode.value = required;
  agreementVisible.value = true;
};

const handleAgreementAccepted = () => {
  agreementAccepted.value = true;
  agreementRequiredMode.value = false;
  localStorage.setItem(AGREEMENT_STORAGE_KEY, '1');
};
</script>

<style scoped lang="scss">
.login-page {
  --card-bg: rgba(255, 255, 255, 0.96);
  --card-border: rgba(255, 255, 255, 0.5);
  --title: #0f172a;
  --subtitle: #64748b;
  --line: #e2e8f0;
  --text: #334155;
  --muted: #94a3b8;
  --input-bg: #ffffff;
  --input-border: #d0d7e2;
  --input-hover: #7aa7ff;
  --input-focus: #165dff;
  --primary: #165dff;
  --primary-hover: #3b82f6;
  --primary-active: #0f4eea;
  --danger-bg: #fff1f0;
  --danger-border: #ffccc7;
  --danger-text: #f53f3f;
  min-height: 100vh;
  width: 100%;
  display: grid;
  place-items: center;
  padding: 24px;
  position: relative;
  overflow: hidden;
  background: linear-gradient(145deg, #162d70 0%, #0f172a 100%);
}

.bg-topology {
  position: absolute;
  inset: 0;
  background-image:
    radial-gradient(circle at 20% 20%, rgba(125, 211, 252, 0.14), transparent 26%),
    radial-gradient(circle at 80% 70%, rgba(59, 130, 246, 0.12), transparent 30%),
    linear-gradient(rgba(255, 255, 255, 0.04) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.04) 1px, transparent 1px);
  background-size: auto, auto, 26px 26px, 26px 26px;
  opacity: 0.45;
}

.bg-orb {
  position: absolute;
  border-radius: 999px;
  filter: blur(60px);
  opacity: 0.22;
}

.orb-a {
  width: 280px;
  height: 280px;
  background: #7dd3fc;
  top: 8%;
  left: 10%;
}

.orb-b {
  width: 220px;
  height: 220px;
  background: #93c5fd;
  bottom: 10%;
  right: 10%;
}

.login-card {
  width: 400px;
  max-width: calc(100vw - 32px);
  border-radius: 8px;
  background: var(--card-bg);
  border: 1px solid var(--card-border);
  box-shadow: 0 10px 30px rgba(7, 18, 44, 0.32);
  padding: 32px 24px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  position: relative;
  z-index: 1;
}

.brand-section {
  display: flex;
  align-items: center;
  gap: 12px;
}

.brand-img {
  width: 40px;
  height: 40px;
}

.brand-text {
  flex: 1;
  min-width: 0;
}

.title-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.app-title {
  margin: 0;
  font-size: 22px;
  line-height: 30px;
  color: var(--title);
  font-weight: 700;
}

.app-subtitle {
  margin: 0;
  font-size: 14px;
  line-height: 20px;
  color: var(--subtitle);
}

.form-title {
  font-size: 16px;
  line-height: 24px;
  font-weight: 600;
  color: var(--title);
}

.top-error {
  margin-bottom: 8px;
}

.login-form {
  display: flex;
  flex-direction: column;
  gap: 8px;

  :deep(.el-form-item) {
    margin-bottom: 8px;
  }

  :deep(.el-form-item__label) {
    color: var(--text);
    line-height: 20px;
    margin-bottom: 4px;
  }

  :deep(.el-input__wrapper) {
    border-radius: 4px;
    min-height: 36px;
    background: var(--input-bg);
    box-shadow: 0 0 0 1px var(--input-border) inset;
    transition: box-shadow 0.2s ease, background 0.2s ease;
  }

  :deep(.el-input__wrapper:hover) {
    box-shadow: 0 0 0 1px var(--input-hover) inset;
  }

  :deep(.el-input__wrapper.is-focus) {
    box-shadow: 0 0 0 2px color-mix(in oklab, var(--input-focus) 30%, transparent) inset;
  }

  :deep(.el-input.is-disabled .el-input__wrapper) {
    opacity: 0.7;
    cursor: not-allowed;
  }

  :deep(.el-form-item.is-error .el-input__wrapper) {
    box-shadow: 0 0 0 2px #f53f3f inset;
  }
}

.pw-toggle {
  height: 24px;
  width: 24px;
  padding: 0;
}

.captcha-row {
  display: grid;
  grid-template-columns: 1fr 112px;
  gap: 8px;
  align-items: center;
}

.captcha-box {
  height: 36px;
  border-radius: 4px;
  border: 1px solid var(--input-border);
  background:
    repeating-linear-gradient(135deg, rgba(22, 93, 255, 0.08), rgba(22, 93, 255, 0.08) 6px, transparent 6px, transparent 12px),
    var(--input-bg);
  color: var(--title);
  letter-spacing: 2px;
  font-weight: 700;
  cursor: pointer;
}

.form-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 4px;
}

.login-btn {
  width: 100%;
  height: 36px;
  border-radius: 4px;
  border-color: var(--primary);
  background: var(--primary);
  color: #fff;
  font-weight: 600;
  margin-top: 4px;
}

.login-btn:not(:disabled):hover {
  border-color: var(--primary-hover);
  background: var(--primary-hover);
}

.login-btn:not(:disabled):active {
  border-color: var(--primary-active);
  background: var(--primary-active);
}

.login-btn.is-disabled,
.login-btn:disabled {
  opacity: 0.55;
}

.agreement {
  margin-top: 8px;
  font-size: 12px;
  line-height: 20px;
  color: var(--muted);
  text-align: left;
}

.footer {
  margin-top: 8px;
  padding-top: 16px;
  border-top: 1px solid var(--line);
  display: flex;
  flex-direction: column;
  gap: 4px;
  text-align: center;
}

.support {
  font-size: 12px;
  color: var(--text);
}

.copyright {
  font-size: 12px;
  color: var(--muted);
}

.reset-steps {
  margin-bottom: 12px;
}

.reset-panel {
  min-height: 160px;
}

:deep(.el-dialog) {
  border-radius: 8px;
}

:deep(.el-dialog__header) {
  padding: 20px 20px 8px;
}

:deep(.el-dialog__body) {
  padding: 8px 20px 20px;
}

@media (max-width: 520px) {
  .login-page {
    padding: 12px;
  }

  .login-card {
    width: 100%;
    padding: 24px 18px;
  }

  .form-actions {
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;
  }
}
</style>
