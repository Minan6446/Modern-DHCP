<template>
  <el-drawer v-model="visible" :title="t('lease.detailTitle', { ip: lease?.ip || '' })" size="40%">
    <div v-if="lease" class="detail">
      <el-descriptions :column="1" border>
        <el-descriptions-item :label="t('lease.ip')">{{ lease.ip }}</el-descriptions-item>
        <el-descriptions-item :label="t('lease.mac')">{{ lease.mac }}</el-descriptions-item>
        <el-descriptions-item :label="t('lease.detailClient')">{{
          lease.clientId || '-'
        }}</el-descriptions-item>
        <el-descriptions-item :label="t('lease.detailHostname')">{{
          lease.hostname || '-'
        }}</el-descriptions-item>
        <el-descriptions-item :label="t('lease.detailPool')">{{
          lease.poolName || lease.poolId
        }}</el-descriptions-item>
        <el-descriptions-item :label="t('lease.detailState')">
          <el-tag :type="stateMeta.color" :title="stateMeta.desc">{{ stateMeta.label }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item :label="t('lease.detailStart')">{{
          lease.startsAt
        }}</el-descriptions-item>
        <el-descriptions-item :label="t('lease.detailEnd')">{{
          lease.endsAt
        }}</el-descriptions-item>
        <el-descriptions-item :label="t('lease.detailT1T2')"
          >{{ lease.t1 || '-' }} / {{ lease.t2 || '-' }}</el-descriptions-item
        >
      </el-descriptions>
      <h4 class="section">{{ t('lease.timeline') }}</h4>
      <el-timeline>
        <el-timeline-item
          v-for="e in events"
          :key="e.id"
          class="el-timeline-item"
          :timestamp="e.ts"
          type="info"
        >
          {{ e.action }} {{ e.actor ? t('lease.timelineBy', { actor: e.actor }) : '' }}
        </el-timeline-item>
      </el-timeline>
    </div>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { Lease, LeaseEvent, LeaseState } from '@/types/lease';
import { getLeaseEvents } from '@/api/leases';
import { showError } from '@/shared/errors/messageToast';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { useI18n } from 'vue-i18n';
import { leaseStateMeta } from '@/shared/utils/leaseState';

const props = defineProps<{ modelValue: boolean; lease: Lease | null }>();
const emits = defineEmits<{ (e: 'update:modelValue', v: boolean): void }>();

const visible = ref(props.modelValue);
watch(
  () => props.modelValue,
  (v) => (visible.value = v)
);
watch(visible, (v) => emits('update:modelValue', v));

const events = ref<LeaseEvent[]>([]);
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const { t } = useI18n();
const can = (key: string) => (permissionStore.can ? permissionStore.can(key) : true);

watch(
  () => props.lease?.id,
  async (id) => {
    if (id) {
      if (!can('lease.view')) return;
      if (!events.value.length) {
        events.value = [{ id: 'placeholder', ts: '', action: '', actor: '' } as any];
      }
      try {
        const { data } = await getLeaseEvents(id, { tenantId: tenantStore.currentTenantId });
        events.value = data.data;
      } catch (e) {
        showError(t('lease.eventsLoadFail'));
      }
    } else {
      events.value = [];
    }
  },
  { immediate: true }
);

const stateMeta = computed(() => leaseStateMeta((props.lease?.state as LeaseState) || 'OFFLINE'));
</script>

<style scoped>
.detail {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.section {
  margin: 8px 0;
}
</style>
