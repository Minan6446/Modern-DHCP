<template>
  <div class="result-page">
    <el-result :title="t('errors.notFoundTitle')" :sub-title="t('errors.notFoundDesc')" icon="info">
      <template #extra>
        <el-button type="primary" @click="goHome">{{ t('errors.goHome') }}</el-button>
        <el-button @click="goBack">{{ t('errors.goBack') }}</el-button>
      </template>
    </el-result>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';

const router = useRouter();
const { t } = useI18n();

const goHome = () => router.push('/dashboard');
const goBack = () => router.back();
</script>

<style scoped>
.result-page {
  min-height: 60vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px;
}
</style>
