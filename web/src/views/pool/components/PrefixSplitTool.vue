<template>
  <el-card shadow="hover" class="panel">
    <div class="panel-header">{{ t('pool.splitToolTitle') }}</div>
    <el-form label-width="140px">
      <el-form-item :label="t('pool.splitParent')">
        <el-input v-model="parent" placeholder="2001:db8::/48" />
      </el-form-item>
      <el-form-item :label="t('pool.splitTargetLen')">
        <el-input-number v-model="length" :min="0" :max="128" />
      </el-form-item>
      <el-button type="primary" @click="split">{{ t('pool.splitCalc') }}</el-button>
    </el-form>
    <el-table :data="results" height="240" border class="mt">
      <el-table-column prop="prefix" :label="t('pool.splitChildPrefix')" />
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { showError } from '@/shared/errors/messageToast';
import { validateIPv6Prefix } from '@/utils/ip';

const { t } = useI18n();
const parent = ref('');
const length = ref(56);
const results = ref<{ prefix: string }[]>([]);

const split = () => {
  if (!validateIPv6Prefix(parent.value)) {
    showError(t('pool.splitInvalidParent'));
    return;
  }
  const [addr, parentLenStr] = parent.value.split('/');
  const parentLen = Number(parentLenStr);
  if (length.value <= parentLen) {
    showError(t('pool.splitLenError'));
    return;
  }
  // Simplified: generate 4 child prefixes as demo
  const subnets = Array.from({ length: 4 }, (_, i) => `${addr}/${length.value}#${i}`).map((p) => ({
    prefix: p.replace('#', '')
  }));
  results.value = subnets;
};
</script>

<style scoped>
.mt {
  margin-top: 12px;
}
</style>
