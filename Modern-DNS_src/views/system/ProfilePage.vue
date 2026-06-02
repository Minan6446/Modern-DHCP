<script setup lang="ts">
import { reactive, ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useAppStore } from '../../stores/app'
import { setI18nLanguage } from '../../i18n'

const { t } = useI18n()

const appStore = useAppStore()

/* ── avatar initial ── */
const avatarInitial = computed(() => (appStore.user.name || 'U').charAt(0).toUpperCase())

/* ── profile form ── */
const profileForm = reactive({
  name: appStore.user.name,
  role: appStore.user.role,
  email: 'admin@modern-dns.local',
  phone: '',
  department: '',
  bio: '',
})

const profileSaving = ref(false)
const saveProfile = async () => {
  profileSaving.value = true
  await new Promise((r) => setTimeout(r, 600))
  appStore.login({ token: localStorage.getItem('modern-dns-token') || 'demo', user: { ...appStore.user, name: profileForm.name } })
  profileSaving.value = false
  ElMessage.success(t('profile.profileUpdated'))
}

/* ── password form ── */
const pwdForm = reactive({ current: '', next: '', confirm: '' })
const pwdSaving = ref(false)
const pwdStrength = computed(() => {
  const v = pwdForm.next
  if (!v) return 0
  let score = 0
  if (v.length >= 8) score++
  if (/[A-Z]/.test(v)) score++
  if (/[0-9]/.test(v)) score++
  if (/[^A-Za-z0-9]/.test(v)) score++
  return score
})
const pwdStrengthLabel = computed(() => ['', t('profile.pwdWeak'), t('profile.pwdFair'), t('profile.pwdStrong'), t('profile.pwdMax')][pwdStrength.value] || '')
const pwdStrengthClass = computed(() => ['', 'pwd-weak', 'pwd-fair', 'pwd-strong', 'pwd-max'][pwdStrength.value] || '')

const changePwd = async () => {
  if (!pwdForm.current) { ElMessage.warning(t('profile.enterCurrentPwd')); return }
  if (pwdForm.next.length < 8) { ElMessage.warning(t('profile.newPwdMinLength')); return }
  if (pwdForm.next !== pwdForm.confirm) { ElMessage.error(t('profile.passwordMismatch')); return }
  pwdSaving.value = true
  await new Promise((r) => setTimeout(r, 700))
  pwdSaving.value = false
  Object.assign(pwdForm, { current: '', next: '', confirm: '' })
  ElMessage.success(t('profile.passwordChanged'))
}

/* ── preference ── */
const prefForm = reactive({
  language: appStore.systemConfig.language || 'zh-CN',
  theme: 'light',
  pageSize: 10,
  notifyEmail: true,
  notifyBrowser: false,
})

watch(() => prefForm.language, (lang) => {
  if (lang) {
    setI18nLanguage(lang)
    appStore.updateSystemConfig({ language: lang })
  }
})

watch(() => appStore.systemConfig.language, (lang) => {
  if (lang && lang !== prefForm.language) {
    prefForm.language = lang
  }
})

const prefSaving = ref(false)
const savePref = async () => {
  prefSaving.value = true
  await new Promise((r) => setTimeout(r, 500))
  prefSaving.value = false
  ElMessage.success(t('profile.prefSaved'))
}

/* ── activity log (simulated) ── */
const activityLog = computed(() => [
  { time: '2026-04-22 15:30:12', action: t('profile.actionModifyForward'), ip: '10.10.1.5', result: t('profile.resultSuccess') },
  { time: '2026-04-22 14:18:07', action: t('profile.actionClearCache'), ip: '10.10.1.5', result: t('profile.resultSuccess') },
  { time: '2026-04-21 09:44:33', action: t('profile.actionAddRule'), ip: '10.10.1.5', result: t('profile.resultSuccess') },
  { time: '2026-04-20 17:02:51', action: t('profile.actionLogin'), ip: '10.10.1.5', result: t('profile.resultSuccess') },
  { time: '2026-04-19 11:29:04', action: t('profile.actionExportCache'), ip: '10.10.1.5', result: t('profile.resultSuccess') },
])

const activeSection = ref('profile')
</script>

<template>
  <div class="page-shell profile-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('profile.title') }}</h1>
        <p class="page-subtitle">{{ $t('profile.subtitle') }}</p>
      </div>
    </div>

    <div class="profile-layout">
      <!-- Left: avatar card + nav -->
      <div class="profile-sidebar">
        <div class="avatar-card">
          <div class="avatar-circle">{{ avatarInitial }}</div>
          <div class="avatar-name">{{ appStore.user.name }}</div>
          <div class="avatar-role">{{ $t('profile.sysAdmin') }}</div>
          <div class="avatar-meta">
            <span class="avatar-meta-item">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="12" height="12"><path d="M20 10c0 6-8 12-8 12S4 16 4 10a8 8 0 1 1 16 0z"/><circle cx="12" cy="10" r="3"/></svg>
              admin@modern-dns.local
            </span>
            <span class="avatar-meta-item">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="12" height="12"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
              {{ $t('profile.verifiedAccount') }}
            </span>
          </div>
        </div>

        <nav class="profile-nav">
          <button
            v-for="item in [
              { key: 'profile', icon: 'M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2', icon2: 'circle cx=12 cy=7 r=4', label: $t('profile.basicInfo') },
              { key: 'security', icon: 'M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z', label: $t('profile.securitySettings') },
              { key: 'preference', icon: 'M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5', label: $t('profile.preference') },
              { key: 'activity', icon: 'M12 20h9M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z', label: $t('profile.activityLog') },
            ]"
            :key="item.key"
            :class="['profile-nav-item', { 'profile-nav-item--active': activeSection === item.key }]"
            @click="activeSection = item.key"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="15" height="15">
              <path :d="item.icon"/>
            </svg>
            {{ item.label }}
          </button>
        </nav>
      </div>

      <!-- Right: sections -->
      <div class="profile-main">

        <!-- 基本资料 -->
        <div v-show="activeSection === 'profile'" class="profile-section">
          <div class="section-header">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>
            <span>{{ $t('profile.basicInfo') }}</span>
          </div>
          <el-form :model="profileForm" label-position="top" class="profile-form">
            <div class="profile-form-grid">
              <el-form-item :label="$t('profile.displayName')">
                <el-input v-model="profileForm.name" :placeholder="$t('profile.displayNamePlaceholder')" />
              </el-form-item>
              <el-form-item :label="$t('profile.rolePermission')">
                <el-input v-model="profileForm.role" disabled />
              </el-form-item>
              <el-form-item :label="$t('profile.emailAddress')">
                <el-input v-model="profileForm.email" :placeholder="$t('profile.emailPlaceholder')" />
              </el-form-item>
              <el-form-item :label="$t('profile.phoneNumber')">
                <el-input v-model="profileForm.phone" :placeholder="$t('profile.phonePlaceholder')" />
              </el-form-item>
              <el-form-item :label="$t('profile.department')">
                <el-input v-model="profileForm.department" :placeholder="$t('profile.departmentPlaceholder')" />
              </el-form-item>
            </div>
            <el-form-item :label="$t('profile.bio')">
              <el-input v-model="profileForm.bio" type="textarea" :rows="3" :placeholder="$t('profile.bioPlaceholder')" />
            </el-form-item>
            <div class="section-actions">
              <el-button type="primary" :loading="profileSaving" @click="saveProfile">{{ $t('profile.saveProfile') }}</el-button>
            </div>
          </el-form>
        </div>

        <!-- 账户安全 -->
        <div v-show="activeSection === 'security'" class="profile-section">
          <div class="section-header">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
            <span>{{ $t('profile.securitySettings') }}</span>
          </div>

          <!-- Security info strip -->
          <div class="security-strip">
            <div class="sec-item sec-item--ok">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16"><polyline points="20 6 9 17 4 12"/></svg>
              <div>
                <div class="sec-item-title">{{ $t('profile.passwordStrength') }}</div>
                <div class="sec-item-sub">{{ $t('profile.lastChanged30Days') }}</div>
              </div>
            </div>
            <div class="sec-item sec-item--warn">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
              <div>
                <div class="sec-item-title">{{ $t('profile.twoFactor') }}</div>
                <div class="sec-item-sub">{{ $t('profile.notEnabled') }}</div>
              </div>
            </div>
            <div class="sec-item sec-item--ok">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16"><polyline points="20 6 9 17 4 12"/></svg>
              <div>
                <div class="sec-item-title">{{ $t('profile.loginIpBinding') }}</div>
                <div class="sec-item-sub">10.10.1.5</div>
              </div>
            </div>
          </div>

          <div class="section-sub-title">{{ $t('profile.changePassword') }}</div>
          <el-form :model="pwdForm" label-position="top" class="profile-form">
            <div class="profile-form-grid">
              <el-form-item :label="$t('profile.oldPassword')">
                <el-input v-model="pwdForm.current" type="password" show-password :placeholder="$t('profile.oldPasswordPlaceholder')" />
              </el-form-item>
              <el-form-item :label="$t('profile.newPassword')">
                <el-input v-model="pwdForm.next" type="password" show-password :placeholder="$t('profile.newPasswordPlaceholder')" />
                <div v-if="pwdForm.next" class="pwd-strength-wrap">
                  <div class="pwd-strength-bar">
                    <div :class="['pwd-bar-fill', pwdStrengthClass]" :style="{ width: (pwdStrength * 25) + '%' }"></div>
                  </div>
                  <span :class="['pwd-strength-label', pwdStrengthClass]">{{ pwdStrengthLabel }}</span>
                </div>
              </el-form-item>
              <el-form-item :label="$t('profile.confirmPassword')">
                <el-input v-model="pwdForm.confirm" type="password" show-password :placeholder="$t('profile.confirmPasswordPlaceholder')" />
                <div v-if="pwdForm.confirm && pwdForm.confirm !== pwdForm.next" class="pwd-mismatch">{{ $t('profile.passwordMismatch') }}</div>
              </el-form-item>
            </div>
            <div class="section-actions">
              <el-button type="primary" :loading="pwdSaving" @click="changePwd">{{ $t('profile.changePassword') }}</el-button>
            </div>
          </el-form>
        </div>

        <!-- 偏好设置 -->
        <div v-show="activeSection === 'preference'" class="profile-section">
          <div class="section-header">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16"><circle cx="12" cy="12" r="3"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14M4.93 4.93a10 10 0 0 0 0 14.14"/></svg>
            <span>{{ $t('profile.preference') }}</span>
          </div>
          <el-form :model="prefForm" label-position="top" class="profile-form">
            <div class="profile-form-grid">
              <el-form-item :label="$t('profile.interfaceLang')">
                <el-select v-model="prefForm.language" style="width:100%">
                  <el-option :label="$t('setting.langZhCN')" value="zh-CN" />
                  <el-option :label="$t('setting.langEnUS')" value="en-US" />
                </el-select>
              </el-form-item>
              <el-form-item :label="$t('profile.themeMode')">
                <el-select v-model="prefForm.theme" style="width:100%">
                  <el-option :label="$t('profile.lightMode')" value="light" />
                  <el-option :label="$t('profile.darkMode')" value="dark" />
                  <el-option :label="$t('profile.followSystem')" value="system" />
                </el-select>
              </el-form-item>
              <el-form-item :label="$t('profile.defaultPageSize')">
                <el-select v-model="prefForm.pageSize" style="width:100%">
                  <el-option :label="$t('profile.perPage', { n })" :value="n" v-for="n in [10, 20, 50]" :key="n" />
                </el-select>
              </el-form-item>
            </div>
            <div class="pref-switches">
              <div class="pref-switch-row">
                <div>
                  <div class="pref-switch-label">{{ $t('profile.emailNotify') }}</div>
                  <div class="pref-switch-sub">{{ $t('profile.emailNotifyDesc') }}</div>
                </div>
                <el-switch v-model="prefForm.notifyEmail" />
              </div>
              <div class="pref-switch-row">
                <div>
                  <div class="pref-switch-label">{{ $t('profile.browserNotify') }}</div>
                  <div class="pref-switch-sub">{{ $t('profile.browserNotifyDesc') }}</div>
                </div>
                <el-switch v-model="prefForm.notifyBrowser" />
              </div>
            </div>
            <div class="section-actions">
              <el-button type="primary" :loading="prefSaving" @click="savePref">{{ $t('profile.savePref') }}</el-button>
            </div>
          </el-form>
        </div>

        <!-- 操作记录 -->
        <div v-show="activeSection === 'activity'" class="profile-section">
          <div class="section-header">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
            <span>{{ $t('profile.recentActivity') }}</span>
          </div>
          <div class="activity-list">
            <div v-for="(item, idx) in activityLog" :key="idx" class="activity-row">
              <div class="activity-dot"></div>
              <div class="activity-content">
                <div class="activity-action">{{ item.action }}</div>
                <div class="activity-meta">
                  <span class="mn-mono">{{ item.time }}</span>
                  <span class="activity-ip mn-mono">{{ item.ip }}</span>
                  <span class="activity-result activity-result--ok">{{ item.result }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>

      </div>
    </div>
  </div>
</template>

<style scoped>
.profile-shell { gap: 20px; }

/* ── Layout ── */
.profile-layout {
  display: grid;
  grid-template-columns: 240px minmax(0, 1fr);
  gap: 20px;
  align-items: start;
}

/* ── Sidebar ── */
.profile-sidebar { display: flex; flex-direction: column; gap: 16px; }

.avatar-card {
  border: 1px solid var(--app-border);
  border-radius: 12px;
  background: var(--app-bg);
  box-shadow: var(--app-shadow-soft);
  padding: 24px 16px 20px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  text-align: center;
}

.avatar-circle {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--app-accent) 0%, #6c9cff 100%);
  color: #fff;
  font-size: 28px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 4px;
  box-shadow: 0 4px 16px rgba(22,93,255,0.25);
}

.avatar-name {
  font-size: 16px;
  font-weight: 700;
  color: var(--app-text-primary, #1d2129);
}

.avatar-role {
  display: inline-flex;
  align-items: center;
  padding: 2px 10px;
  border-radius: 20px;
  font-size: 11px;
  font-weight: 600;
  background: rgba(22,93,255,0.08);
  color: var(--app-accent);
  border: 1px solid rgba(22,93,255,0.2);
}

.avatar-meta {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 6px;
  width: 100%;
}

.avatar-meta-item {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  font-size: 11px;
  color: var(--app-text-secondary);
}

/* ── Profile nav ── */
.profile-nav {
  border: 1px solid var(--app-border);
  border-radius: 10px;
  background: var(--app-bg);
  box-shadow: var(--app-shadow-soft);
  overflow: hidden;
}

.profile-nav-item {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  padding: 11px 16px;
  font-size: 13px;
  color: var(--app-text-regular);
  background: none;
  border: none;
  cursor: pointer;
  text-align: left;
  border-left: 3px solid transparent;
  transition: background 0.15s, color 0.15s;
}

.profile-nav-item:hover { background: var(--app-bg-secondary); color: var(--app-text-primary, #1d2129); }

.profile-nav-item--active {
  background: rgba(22,93,255,0.05);
  color: var(--app-accent);
  border-left-color: var(--app-accent);
  font-weight: 600;
}

/* ── Main panel ── */
.profile-main {
  border: 1px solid var(--app-border);
  border-radius: 12px;
  background: var(--app-bg);
  box-shadow: var(--app-shadow-soft);
  min-height: 480px;
}

.profile-section { padding: 24px 28px; }

.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 700;
  color: var(--app-text-primary, #1d2129);
  padding-bottom: 16px;
  margin-bottom: 20px;
  border-bottom: 1px solid var(--app-border);
}

.section-header svg { color: var(--app-accent); flex-shrink: 0; }

.section-sub-title {
  font-size: 13px;
  font-weight: 700;
  color: var(--app-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  margin: 20px 0 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--app-border);
}

.section-actions {
  display: flex;
  justify-content: flex-end;
  padding-top: 16px;
  border-top: 1px solid var(--app-border);
  margin-top: 20px;
}

/* ── Profile form grid ── */
.profile-form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 20px;
}

.profile-form :deep(.el-form-item__label) {
  color: var(--app-text-secondary);
  font-size: 12px;
  font-weight: 600;
  padding-bottom: 4px;
}

/* ── Security strip ── */
.security-strip {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  margin-bottom: 20px;
}

.sec-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  border-radius: 8px;
  border: 1px solid var(--app-border);
}

.sec-item--ok {
  background: rgba(0,180,42,0.05);
  border-color: rgba(0,180,42,0.2);
}
.sec-item--ok svg { color: var(--app-success); }

.sec-item--warn {
  background: rgba(255,125,0,0.05);
  border-color: rgba(255,125,0,0.2);
}
.sec-item--warn svg { color: var(--app-warning); }

.sec-item-title { font-size: 12px; font-weight: 600; color: var(--app-text-primary, #1d2129); }
.sec-item-sub   { font-size: 11px; color: var(--app-text-secondary); margin-top: 1px; }

/* ── Password strength ── */
.pwd-strength-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 5px;
}

.pwd-strength-bar {
  flex: 1;
  height: 4px;
  border-radius: 2px;
  background: var(--app-border);
  overflow: hidden;
}

.pwd-bar-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 0.25s, background 0.25s;
}

.pwd-weak   { background: var(--app-danger); color: var(--app-danger); }
.pwd-fair   { background: var(--app-warning); color: var(--app-warning); }
.pwd-strong { background: #10b981; color: #10b981; }
.pwd-max    { background: var(--app-accent); color: var(--app-accent); }

.pwd-strength-label { font-size: 11px; font-weight: 600; min-width: 28px; }

.pwd-mismatch {
  font-size: 11px;
  color: var(--app-danger);
  margin-top: 4px;
}

/* ── Preference switches ── */
.pref-switches {
  display: flex;
  flex-direction: column;
  gap: 0;
  border: 1px solid var(--app-border);
  border-radius: 8px;
  overflow: hidden;
  margin-bottom: 4px;
}

.pref-switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--app-border);
}

.pref-switch-row:last-child { border-bottom: none; }

.pref-switch-label { font-size: 13px; font-weight: 600; color: var(--app-text-primary, #1d2129); }
.pref-switch-sub   { font-size: 11px; color: var(--app-text-secondary); margin-top: 2px; }

/* ── Activity log ── */
.activity-list { display: flex; flex-direction: column; gap: 0; }

.activity-row {
  display: flex;
  align-items: flex-start;
  gap: 14px;
  padding: 12px 0;
  border-bottom: 1px solid rgba(0,0,0,0.04);
  position: relative;
}

.activity-row:last-child { border-bottom: none; }

.activity-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--app-accent);
  flex-shrink: 0;
  margin-top: 5px;
}

.activity-content { flex: 1; min-width: 0; }

.activity-action {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary, #1d2129);
  margin-bottom: 3px;
}

.activity-meta {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 11px;
  color: var(--app-text-secondary);
}

.activity-ip { color: var(--app-text-secondary); }

.activity-result--ok {
  color: var(--app-success);
  font-weight: 600;
}

.mn-mono {
  font-family: var(--app-font-mono, 'JetBrains Mono', monospace);
}

/* ── Responsive ── */
@media (max-width: 960px) {
  .profile-layout { grid-template-columns: 1fr; }
  .profile-form-grid { grid-template-columns: 1fr; }
  .security-strip { grid-template-columns: 1fr; }
}
</style>
