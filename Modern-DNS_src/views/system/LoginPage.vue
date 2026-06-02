<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useAppStore } from '../../stores/app'
import { loginByPassword, totpSetup, totpVerify } from '../../api/auth'
import { validateFormAndFocus } from '../../utils/interaction'
import BrandLogo from '../../components/BrandLogo.vue'

const { t } = useI18n()

const REMEMBER_KEY = 'modern-dns-remember-login'

const router = useRouter()
const route = useRoute()
const appStore = useAppStore()

const loading = ref(false)
const errorText = ref('')
const formRef = ref()

const loginForm = reactive({
  username: '',
  password: '',
  remember: false,
})

const loginRules = computed(() => ({
  username: [{ required: true, message: t('login.usernameRequired'), trigger: 'blur' }],
  password: [{ required: true, message: t('login.passwordRequired'), trigger: 'blur' }],
}))

const canSubmit = computed(
  () => Boolean(loginForm.username.trim() && loginForm.password.trim()) && !loading.value,
)

const rememberCurrentLogin = () => {
  if (loginForm.remember) {
    localStorage.setItem(
      REMEMBER_KEY,
      JSON.stringify({ username: loginForm.username, password: loginForm.password, remember: true }),
    )
    return
  }
  localStorage.removeItem(REMEMBER_KEY)
}

const loadRememberedLogin = () => {
  const raw = localStorage.getItem(REMEMBER_KEY)
  if (!raw) {
    return
  }
  try {
    const data = JSON.parse(raw)
    loginForm.username = data.username || ''
    loginForm.password = data.password || ''
    loginForm.remember = Boolean(data.remember)
  } catch (_error) {
    localStorage.removeItem(REMEMBER_KEY)
  }
}

// ── MFA enrolment wizard state ─────────────────────────────────────────
// idle  → username/password form
// verify → MFA block (after backend returned ENROLL_MFA + we ran setup)
type MfaStage = 'idle' | 'verify'
const mfaStage = ref<MfaStage>('idle')
const mfaEnrollToken = ref('')
const mfaSecret = ref('')
const mfaOtpAuth = ref('')
const mfaCode = ref('')
const mfaSubmitting = ref(false)

const resetMfaState = () => {
  mfaStage.value = 'idle'
  mfaEnrollToken.value = ''
  mfaSecret.value = ''
  mfaOtpAuth.value = ''
  mfaCode.value = ''
}

const navigateAfterLogin = async () => {
  const redirectPath = typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboard/overview'
  await router.replace(redirectPath)
}

const handleLogin = async () => {
  errorText.value = ''
  const valid = await validateFormAndFocus(formRef.value)
  if (!valid) {
    return
  }

  loading.value = true
  try {
    const result = await loginByPassword({
      username: loginForm.username.trim(),
      password: loginForm.password,
    })

    if (result.code !== 0) {
      errorText.value = result.message || t('login.loginFailed')
      return
    }

    // Branch 1: backend demands MFA enrolment before issuing real tokens.
    if (result.data && 'action' in result.data && result.data.action === 'ENROLL_MFA') {
      // Re-cast inside the branch — TS doesn't narrow optional discriminants
      // through `'action' in obj` reliably across union members.
      const enroll = result.data as { action: 'ENROLL_MFA'; enrollToken: string }
      mfaEnrollToken.value = enroll.enrollToken
      const setup = await totpSetup(mfaEnrollToken.value)
      if (setup.code !== 0 || !setup.data) {
        errorText.value = setup.message || t('login.mfaSetupFailed')
        resetMfaState()
        return
      }
      mfaSecret.value = setup.data.secret
      mfaOtpAuth.value = setup.data.otpauth
      mfaStage.value = 'verify'
      return
    }

    // Branch 2: normal login.
    if (result.data && 'token' in result.data) {
      appStore.login(result.data)
      rememberCurrentLogin()
      ElMessage.success(t('login.loginSuccess'))
      await navigateAfterLogin()
    }
  } catch (_error) {
    errorText.value = t('login.loginFailed')
  } finally {
    loading.value = false
  }
}

const submitMfaCode = async () => {
  errorText.value = ''
  const code = mfaCode.value.trim()
  if (!/^\d{6}$/.test(code)) {
    errorText.value = t('login.mfaCodeInvalid')
    return
  }
  mfaSubmitting.value = true
  try {
    const r = await totpVerify(mfaEnrollToken.value, code)
    if (r.code !== 0 || !r.data?.token || !r.data?.user) {
      errorText.value = r.message || t('login.mfaVerifyFailed')
      return
    }
    appStore.login({ token: r.data.token, user: r.data.user })
    rememberCurrentLogin()
    ElMessage.success(t('login.mfaEnrolled'))
    resetMfaState()
    await navigateAfterLogin()
  } catch (_error) {
    errorText.value = t('login.mfaVerifyFailed')
  } finally {
    mfaSubmitting.value = false
  }
}

const cancelMfa = () => {
  resetMfaState()
  loginForm.password = ''
}

const copySecret = async () => {
  try {
    await navigator.clipboard.writeText(mfaSecret.value)
    ElMessage.success(t('common.copied'))
  } catch {
    ElMessage.error(t('login.mfaCopyFailed'))
  }
}

const features = computed(() => [
  { text: t('login.feature1') },
  { text: t('login.feature2') },
  { text: t('login.feature3') },
  { text: t('login.feature4') },
  { text: t('login.feature5') },
])

onMounted(() => {
  loadRememberedLogin()
})
</script>

<template>
  <div class="login-root">
    <!-- Left decorative panel -->
    <div class="login-left" aria-hidden="true">
      <div class="left-inner">
        <div class="brand-mark">
          <BrandLogo :size="40" />
          <span class="brand-name">Modern DNS</span>
        </div>

        <div class="left-headline">
          <h2>{{ $t('login.headline') }}</h2>
          <p>{{ $t('login.headlineDesc') }}</p>
        </div>

        <ul class="feature-list">
          <li v-for="f in features" :key="f.text" class="feature-item">
            <span class="feature-icon">
              <svg viewBox="0 0 16 16" fill="none" width="14" height="14"><path d="M3 8l3.5 3.5L13 4.5" stroke="white" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>
            </span>
            {{ f.text }}
          </li>
        </ul>

        <!-- Decorative nodes -->
        <div class="deco-nodes">
          <div class="deco-node deco-node--1"></div>
          <div class="deco-node deco-node--2"></div>
          <div class="deco-node deco-node--3"></div>
          <div class="deco-line deco-line--1"></div>
          <div class="deco-line deco-line--2"></div>
        </div>
      </div>
    </div>

    <!-- Right login panel -->
    <div class="login-right">
      <div class="login-form-wrap">
        <div class="login-header">
          <BrandLogo :size="48" class="login-logo-sm" />
          <h1 class="login-title">{{ $t('login.welcome') }}</h1>
          <p class="login-subtitle">{{ $t('login.subtitle') }}</p>
        </div>

        <el-alert
          v-if="errorText"
          :title="errorText"
          type="error"
          show-icon
          :closable="false"
          class="login-alert"
        />

        <!-- MFA enrolment wizard ─ shown only after the backend returns
             ENROLL_MFA. The username/password form is hidden in this stage
             so the operator focuses on completing the bind. -->
        <div v-if="mfaStage === 'verify'" class="mfa-wizard">
          <h3 class="mfa-title">{{ $t('login.mfaTitle') }}</h3>
          <p class="mfa-intro">{{ $t('login.mfaIntro') }}</p>

          <div class="mfa-secret-row">
            <span class="mfa-secret-label">{{ $t('login.mfaSecret') }}</span>
            <code class="mfa-secret-value">{{ mfaSecret }}</code>
            <el-button link type="primary" size="small" @click="copySecret">{{ $t('common.copy') }}</el-button>
          </div>

          <a class="mfa-otpauth-link" :href="mfaOtpAuth">
            {{ $t('login.mfaOpenLink') }}
          </a>

          <el-input
            v-model="mfaCode"
            :placeholder="$t('login.mfaCodePlaceholder')"
            maxlength="6"
            size="large"
            class="login-input mfa-code-input"
            @keyup.enter="submitMfaCode"
          />

          <div class="mfa-actions">
            <el-button
              type="primary"
              size="large"
              :loading="mfaSubmitting"
              :disabled="mfaCode.trim().length !== 6"
              class="login-btn mfa-submit"
              @click="submitMfaCode"
            >{{ $t('login.mfaVerify') }}</el-button>
            <el-button
              text
              :disabled="mfaSubmitting"
              class="mfa-cancel"
              @click="cancelMfa"
            >{{ $t('login.mfaCancel') }}</el-button>
          </div>
        </div>

        <el-form
          v-else
          ref="formRef"
          :model="loginForm"
          :rules="loginRules"
          label-position="top"
          hide-required-asterisk
          class="login-form"
          @keyup.enter="canSubmit && handleLogin()"
        >
          <el-form-item prop="username">
            <template #label>
              <span class="form-label">{{ $t('login.username') }}</span>
            </template>
            <el-input
              v-model="loginForm.username"
              clearable
              :placeholder="$t('login.usernameRequired')"
              size="large"
              tabindex="1"
              autocomplete="username"
              class="login-input"
            >
              <template #prefix>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" width="16" height="16" style="color:#94a3b8"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>
              </template>
            </el-input>
          </el-form-item>

          <el-form-item prop="password">
            <template #label>
              <span class="form-label">{{ $t('login.password') }}</span>
            </template>
            <el-input
              v-model="loginForm.password"
              show-password
              :placeholder="$t('login.passwordRequired')"
              size="large"
              tabindex="2"
              autocomplete="current-password"
              class="login-input"
            >
              <template #prefix>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" width="16" height="16" style="color:#94a3b8"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
              </template>
            </el-input>
          </el-form-item>

          <div class="login-options">
            <el-checkbox v-model="loginForm.remember" tabindex="3" class="remember-check">{{ $t('login.rememberMe') }}</el-checkbox>
          </div>

          <el-button
            type="primary"
            class="login-btn"
            size="large"
            :loading="loading"
            :disabled="!canSubmit"
            tabindex="4"
            @click="handleLogin"
          >
            <span v-if="!loading">{{ $t('login.login') }}</span>
            <span v-else>{{ $t('login.loggingIn') }}</span>
          </el-button>
        </el-form>

        <footer class="login-footer">
          <div>{{ $t('login.copyright') }}</div>
          <div class="login-support">
            {{ $t('login.techSupport') }}：
            <a href="mailto:minan959@163.com">minan959@163.com</a>
          </div>
        </footer>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* ── Root layout ── */
.login-root {
  display: flex;
  min-height: 100vh;
  background: #f0f4ff;
}

/* ── Left panel ── */
.login-left {
  flex: 0 0 480px;
  background: linear-gradient(145deg, #1d4ed8 0%, #2563eb 40%, #0ea5e9 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 40px;
  position: relative;
  overflow: hidden;
}

.login-left::before {
  content: '';
  position: absolute;
  inset: 0;
  background: radial-gradient(ellipse at 30% 20%, rgba(255,255,255,0.08) 0%, transparent 60%),
              radial-gradient(ellipse at 70% 80%, rgba(14,165,233,0.3) 0%, transparent 50%);
  pointer-events: none;
}

.left-inner {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 360px;
}

.brand-mark {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 56px;
}

.brand-name {
  font-size: 18px;
  font-weight: 700;
  color: #fff;
  letter-spacing: 0.5px;
}

.left-headline h2 {
  margin: 0 0 12px;
  color: #fff;
  font-size: 36px;
  font-weight: 800;
  line-height: 1.25;
  letter-spacing: -0.5px;
}

.left-headline p {
  margin: 0 0 40px;
  color: rgba(255,255,255,0.75);
  font-size: 14px;
  letter-spacing: 1px;
}

.feature-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.feature-item {
  display: flex;
  align-items: center;
  gap: 10px;
  color: rgba(255,255,255,0.9);
  font-size: 14px;
}

.feature-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: rgba(255,255,255,0.2);
  flex-shrink: 0;
}

/* Decorative nodes */
.deco-nodes {
  position: absolute;
  bottom: -40px;
  right: -20px;
  width: 200px;
  height: 160px;
  pointer-events: none;
}

.deco-node {
  position: absolute;
  border-radius: 50%;
  background: rgba(255,255,255,0.15);
}

.deco-node--1 { width: 80px; height: 80px; bottom: 20px; right: 20px; animation: float 6s ease-in-out infinite; }
.deco-node--2 { width: 50px; height: 50px; bottom: 80px; right: 90px; animation: float 8s ease-in-out infinite reverse; }
.deco-node--3 { width: 30px; height: 30px; bottom: 50px; right: 150px; animation: float 5s ease-in-out infinite 1s; }

.deco-line {
  position: absolute;
  height: 1px;
  background: rgba(255,255,255,0.2);
  transform-origin: left center;
}

.deco-line--1 { width: 90px; bottom: 60px; right: 60px; transform: rotate(-30deg); }
.deco-line--2 { width: 60px; bottom: 95px; right: 100px; transform: rotate(15deg); }

@keyframes float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-12px); }
}

/* ── Right panel ── */
.login-right {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px 24px;
}

.login-form-wrap {
  width: 100%;
  max-width: 400px;
}

.login-header {
  margin-bottom: 32px;
}

.login-logo-sm {
  /* The component already paints its own dark rounded backdrop, so we
     only contribute the drop-shadow that was previously baked into the
     gradient div. Keeps the visual weight on the form header consistent
     across the new vector mark. */
  margin-bottom: 20px;
  border-radius: 12px;
  box-shadow: 0 4px 12px rgba(15, 42, 71, 0.35);
}

.login-title {
  margin: 0 0 6px;
  font-size: 28px;
  font-weight: 800;
  color: #0f172a;
  letter-spacing: -0.3px;
}

.login-subtitle {
  margin: 0;
  font-size: 14px;
  color: #64748b;
}

.login-alert {
  margin-bottom: 16px;
  border-radius: 8px;
}

.login-form {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.form-label {
  font-size: 13px;
  font-weight: 600;
  color: #374151;
}

.login-input :deep(.el-input__wrapper) {
  border-radius: 10px;
  box-shadow: 0 0 0 1px #e2e8f0;
  padding: 0 14px;
  transition: box-shadow 0.2s;
}

.login-input :deep(.el-input__wrapper:hover),
.login-input :deep(.el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 2px #2563eb;
}

.login-options {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 4px 0 20px;
}

.remember-check :deep(.el-checkbox__label) {
  font-size: 13px;
  color: #64748b;
}

.login-btn {
  width: 100%;
  height: 48px;
  border-radius: 10px;
  font-size: 15px;
  font-weight: 600;
  background: linear-gradient(135deg, #2563eb 0%, #0ea5e9 100%);
  border: none;
  letter-spacing: 0.5px;
  box-shadow: 0 4px 14px rgba(37,99,235,0.35);
  transition: opacity 0.2s, transform 0.1s;
}

.login-btn:not(:disabled):hover {
  opacity: 0.92;
  transform: translateY(-1px);
}

.login-btn:not(:disabled):active {
  transform: translateY(0);
}

.login-footer {
  margin-top: 32px;
  text-align: center;
  color: #94a3b8;
  font-size: 12px;
  line-height: 1.7;
}

.login-support {
  margin-top: 4px;
  color: #94a3b8;
}

.login-support a {
  color: var(--app-accent);
  text-decoration: none;
}

.login-support a:hover {
  text-decoration: underline;
}

/* ── MFA enrolment wizard ── */
.mfa-wizard {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.mfa-title {
  margin: 0;
  font-size: 18px;
  font-weight: 700;
  color: #0f172a;
}

.mfa-intro {
  margin: 0;
  font-size: 13px;
  color: #475569;
  line-height: 1.5;
}

.mfa-secret-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  background: #f1f5f9;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
}

.mfa-secret-label {
  font-size: 12px;
  color: #64748b;
  flex-shrink: 0;
}

.mfa-secret-value {
  flex: 1;
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, monospace;
  font-size: 13px;
  font-weight: 600;
  color: #0f172a;
  word-break: break-all;
  letter-spacing: 0.5px;
}

.mfa-otpauth-link {
  display: inline-block;
  font-size: 12px;
  color: #2563eb;
  text-decoration: none;
  word-break: break-all;
  padding: 8px 12px;
  background: rgba(37, 99, 235, 0.06);
  border: 1px dashed rgba(37, 99, 235, 0.3);
  border-radius: 8px;
}

.mfa-otpauth-link:hover {
  background: rgba(37, 99, 235, 0.1);
}

.mfa-code-input :deep(.el-input__inner) {
  letter-spacing: 8px;
  text-align: center;
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, monospace;
  font-size: 18px;
  font-weight: 600;
}

.mfa-actions {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 6px;
}

.mfa-submit {
  width: 100%;
}

.mfa-cancel {
  align-self: center;
  font-size: 13px;
  color: #64748b;
}

/* ── Responsive ── */
@media (max-width: 900px) {
  .login-left {
    display: none;
  }
  .login-root {
    background: #fff;
  }
}

@media (max-width: 480px) {
  .login-form-wrap {
    max-width: 100%;
  }
  .login-title {
    font-size: 24px;
  }
}
</style>

