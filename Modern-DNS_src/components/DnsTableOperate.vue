<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { confirmRiskAction } from '../utils/interaction'

export interface DnsOperatePermissions {
  enable?: boolean
  disable?: boolean
  delete?: boolean
  more?: boolean
}

export interface DnsMoreAction {
  label: string
  command: string | number
  danger?: boolean
  disabled?: boolean
  visible?: boolean
}

const props = withDefaults(
  defineProps<{
    enabled?: boolean
    loading?: boolean
    compact?: boolean
    showSwitch?: boolean
    deleteConfirm?: boolean
    deleteConfirmTitle?: string
    deleteConfirmText?: string
    permissions?: DnsOperatePermissions
    moreActions?: DnsMoreAction[]
  }>(),
  {
    enabled: true,
    loading: false,
    compact: false,
    showSwitch: true,
    deleteConfirm: true,
    permissions: () => ({ enable: true, disable: true, delete: true, more: true }),
    moreActions: () => [],
  },
)

const { t } = useI18n()

const emit = defineEmits<{
  (e: 'enable'): void
  (e: 'disable'): void
  (e: 'toggle', value: boolean): void
  (e: 'delete'): void
  (e: 'more-command', command: string | number): void
}>()

const canEnable = computed(() => props.permissions.enable !== false)
const canDisable = computed(() => props.permissions.disable !== false)
const canDelete = computed(() => props.permissions.delete !== false)
const canMore = computed(() => props.permissions.more !== false)

const deleteConfirmTitleText = computed(() => props.deleteConfirmTitle ?? t('record.deleteConfirmTitle'))
const deleteConfirmTextValue = computed(() => props.deleteConfirmText ?? t('record.deleteConfirm'))

const visibleMoreActions = computed(() => props.moreActions.filter((item) => item.visible !== false))

const handleToggle = (next: boolean) => {
  emit('toggle', next)
  if (next) {
    emit('enable')
    return
  }
  emit('disable')
}

const handleDelete = async () => {
  if (props.deleteConfirm) {
    const actionText = deleteConfirmTextValue.value.replace(/[？?]$/, '').replace(/^确认/, '')
    await confirmRiskAction({
      title: deleteConfirmTitleText.value,
      action: actionText,
      risk: t('record.deleteConfirmRisk'),
    })
  }
  emit('delete')
}
</script>

<template>
  <div class="dns-table-operate" :class="{ 'dns-table-operate--compact': compact }">
    <el-switch
      v-if="showSwitch && ((enabled && canDisable) || (!enabled && canEnable))"
      :model-value="enabled"
      inline-prompt
      :active-text="$t('common.enabled')"
      :inactive-text="$t('common.disabled')"
      :disabled="loading"
      @change="handleToggle"
    />

    <template v-else>
      <el-button
        v-if="enabled ? canDisable : canEnable"
        link
        :type="enabled ? 'warning' : 'success'"
        :disabled="loading"
        @click="handleToggle(!enabled)"
      >
        {{ enabled ? $t('common.disable') : $t('common.enable') }}
      </el-button>
    </template>

    <el-button v-if="canDelete" link type="danger" :disabled="loading" @click="handleDelete">{{ $t('common.delete') }}</el-button>

    <el-dropdown v-if="canMore && visibleMoreActions.length" @command="(command) => emit('more-command', command)">
      <el-button link type="primary" :disabled="loading">{{ $t('common.more') }}</el-button>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item
            v-for="item in visibleMoreActions"
            :key="String(item.command)"
            :command="item.command"
            :disabled="item.disabled"
            :class="{ 'dns-table-operate__danger-item': item.danger }"
          >
            {{ item.label }}
          </el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
  </div>
</template>

<style scoped>
.dns-table-operate {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}

.dns-table-operate--compact {
  gap: 6px;
}

.dns-table-operate :deep(.el-switch__core) {
  border-radius: var(--dns-radius-base, 8px);
}

.dns-table-operate__danger-item {
  color: var(--dns-color-danger, var(--el-color-danger));
}
</style>
