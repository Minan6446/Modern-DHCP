<template>
  <div class="page-shell">
    <el-result icon="warning" title="404" :sub-title="t('error.notFoundDesc')">
      <template #extra>
        <el-button type="primary" @click="$router.push('/dashboard/overview')">{{ $t('error.backHome') }}</el-button>
      </template>
    </el-result>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
</script>