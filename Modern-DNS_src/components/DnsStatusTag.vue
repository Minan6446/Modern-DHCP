<script setup lang="ts">
import { computed } from 'vue'

export type DnsStatus = '正常' | '异常' | '同步中' | '禁用'

type TagTone = 'normal' | 'danger' | 'warning' | 'disabled'

const props = withDefaults(
  defineProps<{
    status: DnsStatus
    showIcon?: boolean
  }>(),
  {
    showIcon: true,
  },
)

const toneMap: Record<DnsStatus, { tone: TagTone; icon: string }> = {
  正常: { tone: 'normal', icon: 'CircleCheckFilled' },
  异常: { tone: 'danger', icon: 'CircleCloseFilled' },
  同步中: { tone: 'warning', icon: 'WarningFilled' },
  禁用: { tone: 'disabled', icon: 'InfoFilled' },
}

const current = computed(() => toneMap[props.status])
</script>

<template>
  <el-tag :class="['dns-status-tag', `dns-status-tag--${current.tone}`]" disable-transitions>
    <el-icon v-if="showIcon" class="dns-status-tag__icon">
      <component :is="current.icon" />
    </el-icon>
    <span>{{ status }}</span>
  </el-tag>
</template>

<style scoped>
.dns-status-tag {
  border-radius: var(--dns-radius-base, 8px);
  border-color: transparent;
  color: var(--el-color-white);
}

.dns-status-tag__icon {
  margin-right: 4px;
  vertical-align: middle;
}

.dns-status-tag--normal {
  --el-tag-bg-color: var(--dns-color-success, var(--el-color-success));
  --el-tag-border-color: var(--dns-color-success, var(--el-color-success));
  --el-tag-text-color: var(--el-color-white);
}

.dns-status-tag--warning {
  --el-tag-bg-color: var(--dns-color-warning, var(--el-color-warning));
  --el-tag-border-color: var(--dns-color-warning, var(--el-color-warning));
  --el-tag-text-color: var(--el-color-white);
}

.dns-status-tag--danger {
  --el-tag-bg-color: var(--dns-color-danger, var(--el-color-danger));
  --el-tag-border-color: var(--dns-color-danger, var(--el-color-danger));
  --el-tag-text-color: var(--el-color-white);
}

.dns-status-tag--disabled {
  --el-tag-bg-color: var(--dns-color-disabled, var(--el-disabled-border-color));
  --el-tag-border-color: var(--dns-color-disabled, var(--el-disabled-border-color));
  --el-tag-text-color: var(--el-color-white);
}
</style>
