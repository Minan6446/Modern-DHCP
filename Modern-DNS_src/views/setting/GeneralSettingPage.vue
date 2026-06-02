<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import GeneralSettingsPanel from '../../components/system/GeneralSettingsPanel.vue'
import { useGeneralConfig } from '../../composables/system/useGeneralConfig'

const { t } = useI18n()

const {
  configForm,
  loading,
  saveConfig,
  resetConfig,
} = useGeneralConfig()
</script>

<template>
  <div class="general-setting-page page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ t('setting.general') }}</h1>
        <p class="page-subtitle">{{ t('setting.generalSubtitle') }}</p>
      </div>
    </div>

    <GeneralSettingsPanel
      :model="configForm"
      :loading="loading"
      @update="saveConfig"
      @reset="resetConfig"
    />
  </div>
</template>

<style scoped>
.general-setting-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
</style>
