<template>
  <el-dialog
    v-model="visible"
    class="mac-binding-dialog"
    width="600px"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :before-close="handleBeforeClose"
    destroy-on-close
    align-center
  >
    <template #header>
      <div class="dialog-title">新建 MAC 地址绑定</div>
    </template>

    <div class="dialog-body">
      <div class="form-row">
        <label class="field-label required-label"><span class="required">*</span>{{ t('binding.formMac') }}</label>
        <div class="field-control">
          <el-input
            v-model="form.mac"
            placeholder="示例：00:11:22:33:44:55"
            @input="validateMac"
            @blur="validateMac"
          />
          <div class="field-hint">支持冒号或短横线分隔，保存时自动标准化。</div>
          <p v-if="errors.mac" class="field-error">{{ errors.mac }}</p>
        </div>
      </div>

      <div class="form-row">
        <label class="field-label required-label"><span class="required">*</span>{{ t('binding.formIp') }}</label>
        <div class="field-control">
          <el-input
            v-model="form.ip"
            placeholder="示例：192.168.10.20"
            @input="validateIp"
            @blur="validateIp"
          />
          <div class="field-hint">支持 IPv4/IPv6，若选择 IPv4 地址池会校验网段范围。</div>
          <p v-if="errors.ip" class="field-error">{{ errors.ip }}</p>
        </div>
      </div>

      <div class="form-row">
        <label class="field-label required-label"><span class="required">*</span>地址池</label>
        <div class="field-control">
          <el-select
            v-model="form.poolId"
            filterable
            clearable
            :loading="poolsLoading"
            placeholder="请选择地址池"
            @change="validatePool"
          >
            <el-option
              v-for="pool in poolOptions"
              :key="pool.id"
              :label="`${pool.name || pool.cidr} (${pool.cidr})`"
              :value="pool.id"
            >
              <el-tooltip :content="`网段：${pool.cidr}`" placement="right">
                <span>{{ `${pool.name || pool.cidr} (${pool.cidr})` }}</span>
              </el-tooltip>
            </el-option>
          </el-select>
          <div class="field-hint">IP 地址需落在所选地址池网段内。</div>
          <p v-if="errors.poolId" class="field-error">{{ errors.poolId }}</p>
        </div>
      </div>

      <div class="form-row">
        <label class="field-label optional-label">{{ t('binding.formHostname') }} <span class="optional">可选</span></label>
        <div class="field-control">
          <el-input v-model="form.hostname" placeholder="示例：host-a01" @blur="validateHostname" />
          <div class="field-hint">支持中文、字母、数字与 ._-，最长 64 个字符。</div>
          <p v-if="errors.hostname" class="field-error">{{ errors.hostname }}</p>
        </div>
      </div>

      <div class="form-row">
        <label class="field-label optional-label">{{ t('binding.formDesc') }} <span class="optional">可选</span></label>
        <div class="field-control">
          <el-input
            v-model="form.description"
            type="textarea"
            :autosize="{ minRows: 3, maxRows: 5 }"
            placeholder="示例：核心交换机上联设备固定绑定"
            maxlength="200"
            show-word-limit
          />
          <div class="field-hint">用于记录设备用途、位置等辅助说明。</div>
        </div>
      </div>

      <div class="form-row options-group-row">
        <label class="field-label"></label>
        <div class="field-control">
          <div class="group-title">DHCP 选项配置（可选）</div>
          <div class="option-editor" role="group" aria-label="DHCP 选项配置">
            <el-select v-model="selectedOption" filterable placeholder="DHCP 选项" class="option-key">
              <el-option
                v-for="item in optionCatalog"
                :key="item.key"
                :label="item.label"
                :value="item.key"
              >
                <el-tooltip :content="item.hint" placement="right">
                  <span>{{ item.label }}</span>
                </el-tooltip>
              </el-option>
            </el-select>
            <el-input v-model="optionValue" class="option-value" placeholder="选项值" />
            <el-button class="option-add" @click="addOrUpdateOption">添加</el-button>
          </div>
          <div class="field-hint">已添加选项支持编辑、删除。</div>
          <div class="option-list" v-if="optionEntries.length">
            <div class="option-item" v-for="item in optionEntries" :key="item.key">
              <div class="option-kv">{{ item.key }} - {{ item.value }}</div>
              <div class="option-actions">
                <el-button size="small" text @click="editOption(item.key)">编辑</el-button>
                <el-button size="small" text type="danger" @click="removeOption(item.key)">删除</el-button>
              </div>
            </div>
          </div>
          <p v-if="errors.options" class="field-error">{{ errors.options }}</p>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <el-button :disabled="submitting" @click="requestClose">{{ t('common.cancel') }}</el-button>
        <el-button :disabled="submitting" @click="runPrecheck">配置预校验</el-button>
        <el-button :disabled="submitting" @click="openPreview">配置预览</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">{{ submitting ? '保存中...' : t('common.save') }}</el-button>
      </div>
    </template>
  </el-dialog>

  <el-dialog v-model="previewVisible" title="配置预览" width="560px">
    <el-descriptions :column="1" border>
      <el-descriptions-item label="MAC 地址">{{ form.mac || '-' }}</el-descriptions-item>
      <el-descriptions-item label="IP 地址">{{ form.ip || '-' }}</el-descriptions-item>
      <el-descriptions-item label="地址池">
        {{ selectedPoolLabel || '-' }}
      </el-descriptions-item>
      <el-descriptions-item label="主机名">{{ form.hostname || '-' }}</el-descriptions-item>
      <el-descriptions-item label="描述">{{ form.description || '-' }}</el-descriptions-item>
      <el-descriptions-item label="DHCP 选项">
        {{ optionEntries.map((it) => `${it.key}=${it.value}`).join('；') || '-' }}
      </el-descriptions-item>
    </el-descriptions>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import type { Binding } from '@/types/binding';
import { formatMac as fmtMac, isMac } from '@/utils/mac';
import { isIPv4, isIPv6, validateRangeWithin } from '@/utils/ip';
import { showError, showSuccess } from '@/shared/errors/messageToast';
import { listPools } from '@/api/pools';
import { useTenantStore } from '@/store/tenant';
import { useI18n } from 'vue-i18n';
import { ElMessageBox } from 'element-plus';

const props = defineProps<{
  modelValue: boolean;
  value: Binding | null;
  existingBindings?: Binding[];
  submitting?: boolean;
  conflictChecker?: (ip: string) => Promise<boolean>;
}>();
const emits = defineEmits<{
  (e: 'update:modelValue', v: boolean): void;
  (e: 'submit', v: Partial<Binding>): void;
}>();

const { t } = useI18n();
const tenantStore = useTenantStore();

const defaultForm = (): Partial<Binding> => ({
  mac: '',
  ip: '',
  hostname: '',
  description: '',
  poolId: '',
  options: {}
});

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emits('update:modelValue', v)
});

const submitting = computed(() => Boolean(props.submitting));
const previewVisible = ref(false);
const snapshot = ref('');

const form = reactive<Partial<Binding>>(defaultForm());
const errors = reactive<Record<string, string>>({});
const optionCatalog = [
  { key: '3', label: '3 - Router', hint: '默认网关地址' },
  { key: '6', label: '6 - DNS Server', hint: 'DNS 服务器地址' },
  { key: '15', label: '15 - Domain Name', hint: '域名后缀' },
  { key: '42', label: '42 - NTP Server', hint: 'NTP 时间服务器' },
  { key: '66', label: '66 - TFTP Server Name', hint: 'TFTP 服务器名' },
  { key: '67', label: '67 - Bootfile Name', hint: 'PXE 引导文件名' }
];
const selectedOption = ref('');
const optionValue = ref('');

const poolOptions = ref<Array<{ id: string; name: string; cidr: string }>>([]);
const poolsLoading = ref(false);
let ipValidationToken = 0;

const optionEntries = computed(() =>
  Object.entries(form.options || {}).map(([key, value]) => ({ key, value: String(value || '') }))
);

const selectedPoolLabel = computed(() => {
  const selected = poolOptions.value.find((pool) => pool.id === form.poolId);
  if (!selected) return '';
  return `${selected.name || selected.cidr} (${selected.cidr})`;
});

const isDirty = computed(() => snapshot.value !== JSON.stringify(form));

const clearErrors = () => Object.keys(errors).forEach((key) => delete errors[key]);

const resetForm = (val?: Binding | null) => {
  Object.assign(form, defaultForm(), val ? { ...val, options: { ...(val.options || {}) } } : {});
  selectedOption.value = '';
  optionValue.value = '';
  clearErrors();
  snapshot.value = JSON.stringify(form);
};

const loadPools = async () => {
  poolsLoading.value = true;
  try {
    const { data } = await listPools({
      page: 1,
      pageSize: 200,
      version: 4,
      tenantId: tenantStore.currentTenantId || 'global'
    });
    const payload: any = (data as any)?.data ?? data;
    const items = Array.isArray(payload?.items) ? payload.items : Array.isArray(payload) ? payload : [];
    poolOptions.value = items.map((p: any) => ({ id: p.id, name: p.name, cidr: p.cidr }));
  } catch {
    poolOptions.value = [];
  } finally {
    poolsLoading.value = false;
  }
};

watch(
  () => props.modelValue,
  (v) => {
    if (!v) return;
    resetForm(props.value);
    loadPools();
  },
  { immediate: true }
);

watch(
  () => props.value,
  (v) => {
    if (!visible.value) return;
    resetForm(v);
  }
);

watch(
  () => [form.mac, form.ip, form.poolId],
  () => {
    if (!visible.value) return;
    void validateMac();
    void validateIp();
    validatePool();
  }
);

const findLocalDuplicate = (field: 'mac' | 'ip', value: string) => {
  const currentId = props.value?.id;
  return (props.existingBindings || []).some((item) => {
    if (!item) return false;
    if (currentId && item.id === currentId) return false;
    if (field === 'mac') return String(item.mac || '').toUpperCase() === value.toUpperCase();
    return String(item.ip || item.ipAddress || '').trim() === value;
  });
};

const validateMac = async () => {
  delete errors.mac;
  const value = String(form.mac || '').trim();
  if (!value) {
    errors.mac = '请输入 MAC 地址';
    return false;
  }
  const formatted = fmtMac(value);
  form.mac = formatted;
  if (!isMac(formatted)) {
    errors.mac = 'MAC 地址格式不合法';
    return false;
  }
  if (findLocalDuplicate('mac', formatted)) {
    errors.mac = 'MAC 地址已存在绑定，不能重复';
    return false;
  }
  return true;
};

const validateIp = async () => {
  delete errors.ip;
  const value = String(form.ip || '').trim();
  if (!value) {
    errors.ip = '请输入 IP 地址';
    return false;
  }
  if (!isIPv4(value) && !isIPv6(value)) {
    errors.ip = 'IP 地址格式不合法';
    return false;
  }
  form.ip = value;
  if (findLocalDuplicate('ip', value)) {
    errors.ip = 'IP 地址已存在绑定，不能重复';
    return false;
  }
  if (props.conflictChecker) {
    const token = ++ipValidationToken;
    const conflict = await props.conflictChecker(value);
    if (token !== ipValidationToken) return !errors.ip;
    if (conflict) {
      errors.ip = 'IP 地址存在冲突绑定';
      return false;
    }
  }
  return true;
};

const validatePool = () => {
  delete errors.poolId;
  const poolId = String(form.poolId || '').trim();
  if (!poolId) {
    errors.poolId = '请选择地址池';
    return false;
  }
  if (!isIPv4(String(form.ip || '').trim())) return true;
  const selected = poolOptions.value.find((item) => item.id === poolId);
  if (!selected?.cidr) return true;
  if (!validateRangeWithin(selected.cidr, String(form.ip || '').trim())) {
    errors.poolId = 'IP 地址不在所选地址池网段范围内';
    return false;
  }
  return true;
};

const validateHostname = () => {
  delete errors.hostname;
  const value = String(form.hostname || '').trim();
  if (!value) return true;
  if (!/^[\u4e00-\u9fffa-zA-Z0-9._-]{1,64}$/.test(value)) {
    errors.hostname = '主机名仅支持中文、字母、数字和 ._-';
    return false;
  }
  form.hostname = value;
  return true;
};

const validateAll = async () => {
  const macOk = await validateMac();
  const ipOk = await validateIp();
  const poolOk = validatePool();
  const hostOk = validateHostname();
  if (!macOk || !ipOk || !poolOk || !hostOk) {
    const firstError = document.querySelector('.mac-binding-dialog .field-error') as HTMLElement | null;
    firstError?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }
  return macOk && ipOk && poolOk && hostOk;
};

const addOrUpdateOption = () => {
  delete errors.options;
  const key = selectedOption.value.trim();
  const value = optionValue.value.trim();
  if (!key) {
    errors.options = '请选择 DHCP 选项';
    return;
  }
  if (!value) {
    errors.options = '请输入选项值';
    return;
  }
  form.options = { ...(form.options || {}) };
  form.options[key] = value;
  optionValue.value = '';
  showSuccess('DHCP 选项已添加');
};

const editOption = (key: string) => {
  selectedOption.value = key;
  optionValue.value = String(form.options?.[key] || '');
};

const removeOption = (key: string) => {
  if (!form.options?.[key]) return;
  const next = { ...(form.options || {}) };
  delete next[key];
  form.options = next;
  showSuccess('DHCP 选项已删除');
};

const runPrecheck = async () => {
  const ok = await validateAll();
  if (!ok) {
    showError('预校验未通过，请先修正错误项');
    return;
  }
  showSuccess('预校验通过');
};

const openPreview = async () => {
  const ok = await validateAll();
  if (!ok) {
    showError('请先修正错误项后再预览');
    return;
  }
  previewVisible.value = true;
};

const requestClose = async () => {
  if (!isDirty.value || submitting.value) {
    visible.value = false;
    return;
  }
  try {
    await ElMessageBox.confirm('当前有未保存配置，确认关闭吗？', '未保存提醒', {
      type: 'warning',
      confirmButtonText: '确认关闭',
      cancelButtonText: '继续编辑'
    });
    visible.value = false;
  } catch {
    return;
  }
};

const handleBeforeClose = async (done: () => void) => {
  if (!isDirty.value || submitting.value) {
    done();
    return;
  }
  try {
    await ElMessageBox.confirm('当前有未保存配置，确认关闭吗？', '未保存提醒', {
      type: 'warning',
      confirmButtonText: '确认关闭',
      cancelButtonText: '继续编辑'
    });
    done();
  } catch {
    return;
  }
};

const submit = async () => {
  if (submitting.value) return;
  const ok = await validateAll();
  if (!ok) return;
  emits('submit', {
    ...form,
    mac: String(form.mac || '').trim(),
    ip: String(form.ip || '').trim(),
    hostname: String(form.hostname || '').trim() || undefined,
    description: String(form.description || '').trim() || undefined,
    poolId: String(form.poolId || '').trim(),
    options: { ...(form.options || {}) }
  });
};
</script>

<style scoped>
.mac-binding-dialog :deep(.el-dialog) {
  width: 600px;
  min-height: 680px;
  border-radius: 8px;
}

.mac-binding-dialog :deep(.el-dialog__header) {
  padding: 32px 32px 0;
}

.mac-binding-dialog :deep(.el-dialog__body) {
  padding: 32px;
}

.mac-binding-dialog :deep(.el-dialog__footer) {
  padding: 0 32px 32px;
}

.dialog-title {
  font-size: 16px;
  font-weight: 500;
  color: var(--el-text-color-primary);
  line-height: 24px;
}

.dialog-body {
  min-height: 680px;
  max-height: calc(100vh - 200px);
  overflow-y: auto;
  padding-bottom: 0;
}

.form-row {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 18px;
}

.field-label {
  width: 100px;
  min-width: 100px;
  text-align: right;
  color: var(--el-text-color-primary);
  line-height: 36px;
  font-size: 14px;
  white-space: nowrap;
}

.required-label {
  font-weight: 500;
}

.optional-label {
  font-weight: 400;
}

.required {
  color: var(--el-color-danger);
  margin-right: 4px;
}

.optional {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.field-control {
  width: 440px;
  max-width: 440px;
  min-width: 0;
}

.field-control :deep(.el-input),
.field-control :deep(.el-select),
.field-control :deep(.el-input-number),
.field-control :deep(.el-textarea) {
  width: 440px;
}

.field-control :deep(.el-input__wrapper),
.field-control :deep(.el-select__wrapper),
.field-control :deep(.el-input-number),
.field-control :deep(.el-textarea__inner) {
  min-height: 36px;
  border-radius: 4px;
}

.field-hint {
  margin-top: 4px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.field-error {
  margin-top: 4px;
  font-size: 12px;
  color: var(--el-color-danger);
}

.option-editor {
  width: 440px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.group-title {
  font-size: 14px;
  font-weight: 500;
  line-height: 22px;
  margin-bottom: 8px;
  color: var(--el-text-color-primary);
}

.option-key {
  width: 180px;
}

.option-value {
  width: 160px;
}

.option-add {
  width: 80px;
}

.option-list {
  margin-top: 8px;
  width: 440px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 6px;
  overflow: hidden;
}

.option-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 10px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.option-item:last-child {
  border-bottom: none;
}

.option-kv {
  color: var(--el-text-color-primary);
  word-break: break-all;
}

.option-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

@media (max-width: 768px) {
  .mac-binding-dialog :deep(.el-dialog) {
    width: calc(100vw - 24px) !important;
  }

  .dialog-body {
    min-height: 520px;
  }

  .form-row {
    flex-direction: column;
    gap: 6px;
  }

  .field-label {
    width: auto;
    min-width: 0;
    text-align: left;
    line-height: 20px;
  }

  .field-control {
    width: 100%;
    max-width: none;
  }

  .field-control :deep(.el-input),
  .field-control :deep(.el-select),
  .field-control :deep(.el-input-number),
  .field-control :deep(.el-textarea),
  .option-editor,
  .option-list,
  .option-key,
  .option-value,
  .option-add {
    width: 100%;
  }

  .option-editor {
    flex-wrap: wrap;
  }
}
</style>
