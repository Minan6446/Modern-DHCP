<script setup lang="ts">
import { computed } from 'vue'
import en from 'element-plus/es/locale/lang/en'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import { useAppStore } from './stores/app'

const appStore = useAppStore()

const elementLocale = computed(() =>
  appStore.systemConfig.language === 'en-US' ? en : zhCn,
)
</script>

<template>
  <el-config-provider :locale="elementLocale">
    <router-view />
  </el-config-provider>
</template>
