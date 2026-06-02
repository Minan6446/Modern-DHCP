<template>
  <el-dialog
    :model-value="modelValue"
    width="760px"
    class="agreement-dialog"
    :show-close="!required"
    :close-on-click-modal="!required"
    :close-on-press-escape="!required"
    :before-close="handleBeforeClose"
    destroy-on-close
    append-to-body
    @update:model-value="emit('update:modelValue', $event)"
  >
    <template #header>
      <div class="dialog-header">
        <h3>服务协议与隐私政策</h3>
        <div class="meta">
          <span>版本：v2.2.0</span>
          <span>发布时间：2026-01-05</span>
          <span>生效时间：2026-01-10</span>
        </div>
      </div>
    </template>

    <el-tabs v-model="activeTab" class="agreement-tabs">
      <el-tab-pane label="用户协议" name="agreement">
        <div class="dialog-content">
          <h4>1. 服务说明</h4>
          <p>Modern DHCP（以下简称“本系统”）为企业提供 DHCP 地址管理、租约监控、选项配置与高可用能力。用户需遵守所在地法律法规及组织内部管理规范。</p>
          <h4>2. 账户与权限</h4>
          <p>用户应妥善保管账户信息并对账户下所有操作负责。管理员可基于最小权限原则配置角色与访问范围。</p>
          <h4>3. 数据与审计</h4>
          <p>系统会记录必要的登录、配置与操作审计信息，用于安全追踪、问题定位及合规审查。</p>
          <h4>4. 责任限制</h4>
          <p>因不可抗力、第三方服务异常或超出合理控制范围的原因造成的服务中断，平台将在合理范围内协助恢复，但不承担额外赔偿责任。</p>
        </div>
      </el-tab-pane>
      <el-tab-pane label="隐私政策" name="privacy">
        <div class="dialog-content">
          <h4>1. 信息收集</h4>
          <p>为提供服务与保障安全，本系统会处理登录标识、操作日志、设备与网络元数据等必要信息。</p>
          <h4>2. 信息使用</h4>
          <p>收集的信息用于身份校验、权限控制、风险识别、故障诊断与服务优化，不用于无关用途。</p>
          <h4>3. 信息保护</h4>
          <p>我们采用访问控制、传输加密、审计留痕等措施保护数据安全，并按最小化原则保留数据。</p>
          <h4>4. 用户权利</h4>
          <p>您可通过管理员或技术支持渠道申请访问、更正、删除账户相关信息。涉及法律法规要求保留的数据将按规定处理。</p>
        </div>
      </el-tab-pane>
    </el-tabs>

    <div class="history-link">
      <el-link type="primary" :underline="false">查看历史版本</el-link>
    </div>

    <template #footer>
      <el-button v-if="!required" @click="emit('update:modelValue', false)">关闭</el-button>
      <el-button type="primary" @click="handleAgree">同意并继续</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import type { DialogProps } from 'element-plus';

const props = withDefaults(
  defineProps<{
    modelValue: boolean;
    initialTab?: 'agreement' | 'privacy';
    required?: boolean;
  }>(),
  {
    initialTab: 'agreement',
    required: false
  }
);

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void;
  (e: 'agreed'): void;
}>();

const activeTab = ref<'agreement' | 'privacy'>(props.initialTab);

watch(
  () => props.initialTab,
  (value) => {
    activeTab.value = value;
  }
);

watch(
  () => props.modelValue,
  (opened) => {
    if (opened) {
      activeTab.value = props.initialTab;
    }
  }
);

const handleBeforeClose: DialogProps['beforeClose'] = (done) => {
  if (props.required) return;
  done();
};

const handleAgree = () => {
  emit('agreed');
  emit('update:modelValue', false);
};
</script>

<style scoped lang="scss">
.agreement-dialog {
  :deep(.el-dialog) {
    border-radius: 8px;
  }

  :deep(.el-dialog__header) {
    padding: 20px 20px 8px;
  }

  :deep(.el-dialog__body) {
    padding: 8px 20px 16px;
  }

  .dialog-header {
    display: flex;
    flex-direction: column;
    gap: 8px;

    h3 {
      margin: 0;
      font-size: 18px;
      font-weight: 700;
      color: #0f172a;
    }

    .meta {
      display: flex;
      gap: 12px;
      flex-wrap: wrap;
      color: #64748b;
      font-size: 12px;
    }
  }

  .agreement-tabs {
    margin-top: 2px;
  }

  .dialog-content {
    max-height: 46vh;
    overflow-y: auto;
    padding-right: 6px;

    h4 {
      margin: 0 0 8px;
      font-size: 14px;
      font-weight: 600;
      color: #1e293b;
    }

    h4:not(:first-child) {
      margin-top: 14px;
    }

    p {
      margin: 0;
      font-size: 14px;
      line-height: 1.8;
      color: #334155;
      text-align: justify;
    }
  }

  .history-link {
    margin-top: 8px;
    display: flex;
    justify-content: flex-end;
  }
}
</style>
