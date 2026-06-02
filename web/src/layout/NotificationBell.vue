<template>
  <el-popover
    v-model:visible="visible"
    trigger="click"
    placement="bottom-end"
    width="360"
    :hide-after="0"
    @show="handleShow"
  >
    <template #reference>
      <el-badge
        class="notification-trigger"
        :value="badgeCount"
        :max="99"
        :hidden="badgeCount === 0"
      >
        <el-button text class="notification-btn">
          <Bell class="notification-icon" />
        </el-button>
      </el-badge>
    </template>
    <div class="notification-panel">
      <div class="panel-header">
        <span>{{ t('notifications.title') }}</span>
        <el-button text size="small" :disabled="!canViewAlerts" @click="goAlertCenter">
          {{ t('notifications.viewAll') }}
        </el-button>
      </div>
      <div v-loading="loading && canViewAlerts" class="panel-body">
        <el-result v-if="!canViewAlerts" icon="warning" :title="t('notifications.noPermission')" />
        <AppErrorCallout v-else-if="error" :error="error" class="mb-8" />
        <el-empty v-else-if="!events.length" :description="t('notifications.empty')" />
        <ul v-else class="alert-list">
          <li v-for="event in events" :key="event.id" class="alert-item">
            <div class="item-head">
              <el-tag size="small" :type="severityTag(event.severity)">{{ event.severity }}</el-tag>
              <el-tag size="small" effect="plain" :type="statusTag(event.status)">{{
                event.status
              }}</el-tag>
            </div>
            <p class="item-message">{{ event.message }}</p>
            <div class="item-meta">
              <span>{{ formatRelativeTime(event.createdAt) }}</span>
            </div>
          </li>
        </ul>
      </div>
      <div v-if="canViewAlerts" class="panel-footer">
        <el-button text size="small" :loading="loading" @click="handleRefresh">
          {{ t('notifications.refresh') }}
        </el-button>
      </div>
    </div>
  </el-popover>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { Bell } from '@element-plus/icons-vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { listAlertEvents } from '@/api/monitoring';
import type { AlertEvent } from '@/types/monitoring';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { getApiError, createInlineError } from '@/shared/errors/apiError';
import type { ApiErrorDescriptor } from '@/shared/errors/apiError';
import { formatRelative } from '@/utils/time';
import AppErrorCallout from '@/components/common/AppErrorCallout.vue';

const { t } = useI18n();
const router = useRouter();
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();

const visible = ref(false);
const loading = ref(false);
const events = ref<AlertEvent[]>([]);
const total = ref(0);
const error = ref<ApiErrorDescriptor | null>(null);
const controller = ref<AbortController | null>(null);

const canViewAlerts = computed(
  () => permissionStore.can('monitor.view') || permissionStore.can('monitor.manage')
);
const badgeCount = computed(() => (total.value > 0 ? Math.min(total.value, 99) : 0));

const resetState = () => {
  events.value = [];
  total.value = 0;
  error.value = null;
  loading.value = false;
};

const finalize = (ctrl: AbortController) => {
  if (controller.value === ctrl) {
    controller.value = null;
  }
  if (!ctrl.signal.aborted) {
    loading.value = false;
  }
};

const fetchAlerts = async () => {
  if (!canViewAlerts.value) {
    resetState();
    return;
  }
  controller.value?.abort();
  const ctrl = new AbortController();
  controller.value = ctrl;
  loading.value = true;
  error.value = null;
  try {
    const { data } = await listAlertEvents(
      {
        page: 1,
        pageSize: 5,
        status: 'firing',
        tenantId: tenantStore.currentTenantId || undefined
      },
      ctrl.signal
    );
    const payload = data.data;
    if (Array.isArray(payload)) {
      events.value = payload.slice(0, 5);
      total.value = payload.length;
    } else {
      events.value = payload.items || [];
      total.value = payload.total ?? events.value.length;
    }
  } catch (err) {
    if (!ctrl.signal.aborted) {
      // For the bell, stay silent on errors to avoid noisy UX; show empty list instead.
      events.value = [];
      total.value = 0;
      error.value = null;
    }
  } finally {
    finalize(ctrl);
  }
};

const handleShow = () => {
  if (canViewAlerts.value) {
    fetchAlerts();
  }
};

const handleRefresh = () => {
  if (!loading.value && canViewAlerts.value) {
    fetchAlerts();
  }
};

const goAlertCenter = () => {
  visible.value = false;
  router.push('/monitor/history');
};

const severityTag = (severity: AlertEvent['severity']) => {
  if (severity === 'critical') return 'danger';
  if (severity === 'warning') return 'warning';
  return 'info';
};

const statusTag = (status: AlertEvent['status']) => {
  if (status === 'firing') return 'danger';
  if (status === 'ack') return 'warning';
  return 'success';
};

const formatRelativeTime = (ts: string) => formatRelative(ts);

watch(
  () => tenantStore.currentTenantId,
  () => {
    fetchAlerts();
  }
);

watch(canViewAlerts, (allowed) => {
  if (allowed) {
    fetchAlerts();
  } else {
    resetState();
  }
});

onMounted(() => {
  fetchAlerts();
});

onBeforeUnmount(() => {
  controller.value?.abort();
});
</script>

<style scoped>
.notification-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.notification-btn {
  padding: 4px;
  min-width: 28px;
}

.notification-icon {
  width: 18px;
  height: 18px;
  color: var(--el-text-color-primary);
}

.notification-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.panel-body {
  min-height: 160px;
}

.panel-footer {
  display: flex;
  justify-content: flex-end;
}

.alert-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.alert-item {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  padding: 8px 10px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.item-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.item-message {
  margin: 0;
  font-size: 14px;
  color: var(--el-text-color-primary);
}

.item-meta {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  display: flex;
  justify-content: space-between;
}

.mb-8 {
  margin-bottom: 8px;
}
</style>
