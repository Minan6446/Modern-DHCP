<template>
  <el-card shadow="hover" class="panel">
    <div class="panel-header">{{ t('pool.bindingTitle') }}</div>
    <el-form label-width="160px">
      <el-form-item :label="t('pool.vlanId')"><el-input-number v-model="vlanId" /></el-form-item>
      <el-form-item :label="t('pool.physicalPorts')">
        <el-select v-model="ports" multiple filterable allow-create>
          <el-option v-for="p in portOptions" :key="p" :label="p" :value="p" />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('pool.wirelessMapping')">
        <el-select v-model="ssids" multiple filterable allow-create>
          <el-option v-for="s in ssidOptions" :key="s" :label="s" :value="s" />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('pool.location')"><el-input v-model="location" /></el-form-item>
      <el-button type="primary" @click="save">{{ t('pool.saveBinding') }}</el-button>
    </el-form>
  </el-card>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { showSuccess } from '@/shared/errors/messageToast';
import { useI18n } from 'vue-i18n';

const props = defineProps<{
  modelValue?: number;
  portOptions?: string[];
  ssidOptions?: string[];
}>();
const emits = defineEmits<{
  (
    e: 'save',
    payload: { vlanId?: number; ports: string[]; ssids: string[]; location: string }
  ): void;
}>();

const vlanId = ref(props.modelValue);
const ports = ref<string[]>([]);
const ssids = ref<string[]>([]);
const location = ref('');

const portOptions = props.portOptions || ['eth1', 'eth2', 'eth3'];
const ssidOptions = props.ssidOptions || ['Office', 'Guest'];
const { t } = useI18n();

const save = () => {
  emits('save', {
    vlanId: vlanId.value,
    ports: ports.value,
    ssids: ssids.value,
    location: location.value
  });
  showSuccess(t('pool.bindingSaved'));
};
</script>
