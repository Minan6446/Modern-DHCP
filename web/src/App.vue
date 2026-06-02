<template>
  <el-config-provider :locale="elementLocale">
    <RouterView />
  </el-config-provider>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { ElConfigProvider } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { getElementLocale } from '@/i18n/useLocale';
import { resolveLocale } from '@/i18n';

const { locale } = useI18n({ useScope: 'global' });
const elementLocale = computed(() => getElementLocale(resolveLocale(locale.value).code));
</script>
