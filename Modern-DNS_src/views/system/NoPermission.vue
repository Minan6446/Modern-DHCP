<template>
  <div class="page-shell">
    <el-result icon="error" :title="t('error.noPermission')" :sub-title="t('error.noPermissionDesc')">
      <template #extra>
        <el-button type="primary" @click="$router.push('/dashboard/overview')">{{ t('error.backHome') }}</el-button>
      </template>
    </el-result>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
</script>