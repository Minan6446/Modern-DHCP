<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import BlackWhitePage from './BlackWhitePage.vue'
import RpzPage from './RpzPage.vue'
import { useSecurityStore } from '../../stores/security'

const { t } = useI18n()
const activeTab = ref('blackwhite')

// Per-IP rate-limit panel reuses the DDoS global config endpoint
// (same model, additive fields). We load it here independently of
// the BlackWhitePage so this page works standalone — the user lands
// on it from the menu without first opening the DDoS view.
const securityStore = useSecurityStore()
const perIpForm = reactive({ qps: 0, burst: 0 })
const perIpSaving = ref(false)

const syncPerIpForm = () => {
  perIpForm.qps = securityStore.ddosGlobal.perIpQps || 0
  perIpForm.burst = securityStore.ddosGlobal.perIpBurst || 0
}

onMounted(async () => {
  if (!securityStore.lastUpdated) {
    await securityStore.fetchSecurityData()
  }
  syncPerIpForm()
})

const savePerIpRateLimit = async () => {
  // Reject mismatched configs early — burst without sustained QPS is
  // ill-defined (the bucket would never refill once drained), and
  // sustained QPS without burst would lock everyone out at the very
  // first query. The "all zero" path explicitly disables the feature.
  if ((perIpForm.qps > 0) !== (perIpForm.burst > 0)) {
    ElMessage.warning(t('security.perIpRateLimitMismatch'))
    return
  }
  perIpSaving.value = true
  try {
    await securityStore.saveDdosGlobalConfig({
      ...securityStore.ddosGlobal,
      perIpQps: perIpForm.qps,
      perIpBurst: perIpForm.burst,
    })
    ElMessage.success(t('security.perIpRateLimitSaved'))
  } catch {
    ElMessage.error(t('security.saveFailedRetry'))
  } finally {
    perIpSaving.value = false
  }
}

const resetPerIpRateLimit = () => {
  perIpForm.qps = 0
  perIpForm.burst = 0
}
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('security.accessControl') }}</h1>
        <p class="page-subtitle">{{ $t('security.accessControlSubtitle') }}</p>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="access-tabs">
      <el-tab-pane name="blackwhite">
        <template #label>
          <span class="tab-label">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="13" height="13"><circle cx="12" cy="12" r="10"/><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/></svg>
            {{ $t('security.blackWhiteTab') }}
          </span>
        </template>
        <div class="tab-desc">{{ $t('security.blackWhiteTabDesc') }}</div>
        <BlackWhitePage view="black-white" />
      </el-tab-pane>
      <el-tab-pane name="rpz">
        <template #label>
          <span class="tab-label">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="13" height="13"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
            {{ $t('security.rpzTab') }}
          </span>
        </template>
        <div class="tab-desc">{{ $t('security.rpzTabDesc') }}</div>
        <RpzPage />
      </el-tab-pane>
      <!-- Per-IP rate limit moved into a sibling tab so the access-
           control workspace exposes one consistent navigation idiom
           (every category is a tab, not a mix of permanent panels
           and tabs). -->
      <el-tab-pane name="perip">
        <template #label>
          <span class="tab-label">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="13" height="13"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></svg>
            {{ $t('security.perIpRateLimitTitle') }}
          </span>
        </template>
        <div class="tab-desc">{{ $t('security.perIpRateLimitSubtitle') }}</div>
        <el-card class="per-ip-card">
          <div class="per-ip-grid">
            <el-form label-width="120px" class="per-ip-form">
              <el-form-item :label="$t('security.perIpQps')">
                <el-input-number v-model="perIpForm.qps" :min="0" :max="100000" style="width:100%" />
                <div class="per-ip-hint">{{ $t('security.perIpQpsHint') }}</div>
              </el-form-item>
              <el-form-item :label="$t('security.perIpBurst')">
                <el-input-number v-model="perIpForm.burst" :min="0" :max="100000" style="width:100%" />
                <div class="per-ip-hint">{{ $t('security.perIpBurstHint') }}</div>
              </el-form-item>
            </el-form>
          </div>
          <div class="per-ip-actions">
            <el-button type="primary" :loading="perIpSaving" @click="savePerIpRateLimit">{{ $t('security.saveConfig') }}</el-button>
            <el-button :disabled="perIpSaving" @click="resetPerIpRateLimit">{{ $t('security.resetConfig') }}</el-button>
          </div>
        </el-card>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped>
.access-tabs :deep(.el-tabs__header) {
  margin-bottom: 0;
  padding: 0 4px;
  background: var(--app-bg);
  border-bottom: 1px solid var(--app-border);
}
.access-tabs :deep(.el-tabs__content) {
  padding: 0;
}
.tab-label {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.tab-desc {
  font-size: 12px;
  color: var(--app-text-secondary);
  padding: 10px 2px 14px;
}
.access-tabs :deep(.page-shell) {
  padding: 0;
  gap: 12px;
}
.access-tabs :deep(.page-header) {
  display: none;
}

.per-ip-card {
  margin-bottom: 12px;
}
.per-ip-grid {
  padding: 16px 16px 0;
}
.per-ip-form :deep(.el-form-item) {
  margin-bottom: 18px;
  max-width: 480px;
}
.per-ip-hint {
  margin-top: 4px;
  font-size: 12px;
  color: var(--app-text-secondary);
  line-height: 1.5;
}
.per-ip-actions {
  display: flex;
  gap: 8px;
  padding: 0 16px 16px;
}
</style>
