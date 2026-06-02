<template>
  <el-dialog
    v-model="visible"
    title="新增节点"
    width="600px"
    class="add-node-dialog"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :before-close="handleBeforeClose"
    destroy-on-close
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="110px" class="node-form">
      <el-form-item prop="nodeIp">
        <template #label>
          <span class="field-label required-label"><span class="required">*</span>节点IP</span>
        </template>
        <div class="field-wrap">
          <el-input v-model="form.nodeIp" placeholder="示例：192.168.1.10 或 2001:db8::10" @input="onNodeIpInput" />
          <div class="field-hint">支持 IPv4/IPv6，保存前会检测是否与现有节点 IP 冲突。</div>
        </div>
      </el-form-item>

      <el-form-item prop="role">
        <template #label>
          <span class="field-label required-label"><span class="required">*</span>节点角色</span>
        </template>
        <div class="field-wrap">
          <el-select v-model="form.role" placeholder="选择节点角色">
            <el-option v-for="item in roleOptions" :key="item.value" :value="item.value" :label="item.label">
              <el-tooltip :content="item.hint" placement="right">
                <span>{{ item.label }}</span>
              </el-tooltip>
            </el-option>
          </el-select>
        </div>
      </el-form-item>

      <el-form-item prop="name">
        <template #label>
          <span class="field-label optional-label">节点名称 <span class="optional">可选</span></span>
        </template>
        <div class="field-wrap">
          <el-input v-model="form.name" placeholder="示例：edge-node-01" @input="onNameInput" />
          <div class="field-hint">若填写名称将执行重名检测。</div>
        </div>
      </el-form-item>

      <el-form-item prop="heartbeatIp">
        <template #label>
          <span class="field-label required-label"><span class="required">*</span>心跳地址</span>
        </template>
        <div class="field-wrap">
          <el-input v-model="form.heartbeatIp" placeholder="默认与节点IP同步" @input="onHeartbeatInput" />
          <div class="field-hint">默认与节点IP同步，可手动改为独立心跳网卡地址。</div>
        </div>
      </el-form-item>

      <el-form-item prop="sshPort">
        <template #label>
          <span class="field-label required-label"><span class="required">*</span>SSH端口</span>
        </template>
        <div class="field-wrap">
          <el-input-number v-model="form.sshPort" :min="1" :max="65535" :controls="false" @change="validateField('sshPort')" />
          <div class="field-hint">范围 1-65535，默认 22。</div>
        </div>
      </el-form-item>

      <el-form-item prop="authType">
        <template #label>
          <span class="field-label required-label"><span class="required">*</span>认证方式</span>
        </template>
        <div class="field-wrap">
          <el-radio-group v-model="form.authType">
            <el-radio-button label="password">密码认证</el-radio-button>
            <el-radio-button label="key">密钥认证</el-radio-button>
          </el-radio-group>
        </div>
      </el-form-item>

      <el-form-item v-if="form.authType === 'password'" prop="password">
        <template #label>
          <span class="field-label required-label"><span class="required">*</span>认证密码</span>
        </template>
        <div class="field-wrap">
          <el-input v-model="form.password" type="password" show-password placeholder="请输入节点SSH密码" @input="validateField('password')" />
        </div>
      </el-form-item>

      <el-form-item v-else prop="privateKey">
        <template #label>
          <span class="field-label required-label"><span class="required">*</span>密钥上传</span>
        </template>
        <div class="field-wrap">
          <el-upload
            class="key-upload"
            :auto-upload="false"
            :limit="1"
            :show-file-list="false"
            accept=".pem,.key,.txt"
            :on-change="onKeySelected"
          >
            <el-button>选择密钥文件</el-button>
          </el-upload>
          <div class="field-hint">{{ keyFileName || '未选择文件，支持 .pem / .key / .txt' }}</div>
        </div>
      </el-form-item>

      <el-form-item prop="remark">
        <template #label>
          <span class="field-label optional-label">备注 <span class="optional">可选</span></span>
        </template>
        <div class="field-wrap">
          <el-input v-model="form.remark" type="textarea" :autosize="false" placeholder="记录节点用途、机房位置等信息" />
        </div>
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button :disabled="submitting" @click="requestClose">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">{{ submitting ? '保存中...' : '保存' }}</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import type { FormInstance, FormRules } from 'element-plus';
import { ElMessageBox } from 'element-plus';
import { createClusterNode, getClusterOverview } from '@/api/cluster';
import { showHttpError } from '@/shared/errors/errorToast';
import { showSuccess } from '@/shared/errors/messageToast';
import { isIPv4, isIPv6 } from '@/utils/ip';
import type { ClusterNode, ClusterOverview } from '@/types/cluster';

const props = defineProps<{ modelValue: boolean }>();
const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void;
  (e: 'saved'): void;
}>();

const formRef = ref<FormInstance>();
const submitting = ref(false);
const keyFileName = ref('');
const heartbeatTouched = ref(false);
const existingNodes = ref<ClusterNode[]>([]);
const snapshot = ref('');

const roleOptions = [
  { label: '主节点', value: 'active', hint: '承担主写流量与核心调度。' },
  { label: '备节点', value: 'standby', hint: '用于主节点故障切换与容灾。' },
  { label: '工作节点', value: 'worker', hint: '承载业务处理与扩展任务。' }
];

const form = reactive({
  nodeIp: '',
  role: 'worker',
  name: '',
  heartbeatIp: '',
  sshPort: 22,
  authType: 'password' as 'password' | 'key',
  password: '',
  privateKey: '',
  remark: ''
});

const visible = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit('update:modelValue', value)
});

const resetForm = () => {
  form.nodeIp = '';
  form.role = 'worker';
  form.name = '';
  form.heartbeatIp = '';
  form.sshPort = 22;
  form.authType = 'password';
  form.password = '';
  form.privateKey = '';
  form.remark = '';
  keyFileName.value = '';
  heartbeatTouched.value = false;
  formRef.value?.clearValidate();
  snapshot.value = JSON.stringify(form);
};

const unwrap = <T>(resp: any): T => (resp?.data?.data ?? resp?.data ?? resp) as T;

const loadExistingNodes = async () => {
  try {
    const response = await getClusterOverview();
    const payload = unwrap<ClusterOverview>(response);
    existingNodes.value = payload?.nodes || [];
  } catch {
    existingNodes.value = [];
  }
};

watch(
  () => props.modelValue,
  async (value) => {
    if (!value) return;
    resetForm();
    await loadExistingNodes();
    snapshot.value = JSON.stringify(form);
  }
);

const hasIpDuplicate = (ip: string) =>
  existingNodes.value.some((node) => String(node.address || '').trim() === ip.trim());

const hasNameDuplicate = (name: string) => {
  const target = String(name || '').trim().toLowerCase();
  if (!target) return false;
  return existingNodes.value.some((node) => String(node.id || '').trim().toLowerCase() === target);
};

const validateIp = (_: unknown, value: string, callback: (err?: Error) => void) => {
  const text = String(value || '').trim();
  if (!text) return callback(new Error('节点IP必填'));
  if (!isIPv4(text) && !isIPv6(text)) return callback(new Error('IP 格式不合法'));
  if (hasIpDuplicate(text)) return callback(new Error('节点IP已存在，不能重复'));
  callback();
};

const validateHeartbeat = (_: unknown, value: string, callback: (err?: Error) => void) => {
  const text = String(value || '').trim();
  if (!text) return callback(new Error('心跳地址必填'));
  if (!isIPv4(text) && !isIPv6(text)) return callback(new Error('心跳地址格式不合法'));
  callback();
};

const validateName = (_: unknown, value: string, callback: (err?: Error) => void) => {
  const text = String(value || '').trim();
  if (!text) return callback();
  if (hasNameDuplicate(text)) return callback(new Error('节点名称已存在'));
  callback();
};

const validateSshPort = (_: unknown, value: number, callback: (err?: Error) => void) => {
  const port = Number(value);
  if (!Number.isFinite(port) || port < 1 || port > 65535) {
    callback(new Error('SSH端口范围必须在 1-65535'));
    return;
  }
  callback();
};

const validatePassword = (_: unknown, value: string, callback: (err?: Error) => void) => {
  if (form.authType !== 'password') return callback();
  if (!String(value || '').trim()) return callback(new Error('请输入认证密码'));
  callback();
};

const validatePrivateKey = (_: unknown, value: string, callback: (err?: Error) => void) => {
  if (form.authType !== 'key') return callback();
  if (!String(value || '').trim()) return callback(new Error('请上传或粘贴私钥内容'));
  callback();
};

const rules: FormRules = {
  nodeIp: [{ validator: validateIp, trigger: ['blur', 'change'] }],
  role: [{ required: true, message: '请选择节点角色', trigger: 'change' }],
  name: [{ validator: validateName, trigger: ['blur', 'change'] }],
  heartbeatIp: [{ validator: validateHeartbeat, trigger: ['blur', 'change'] }],
  sshPort: [{ validator: validateSshPort, trigger: ['blur', 'change'] }],
  authType: [{ required: true, message: '请选择认证方式', trigger: 'change' }],
  password: [{ validator: validatePassword, trigger: ['blur', 'change'] }],
  privateKey: [{ validator: validatePrivateKey, trigger: ['blur', 'change'] }]
};

const validateField = (field: 'nodeIp' | 'name' | 'heartbeatIp' | 'sshPort' | 'password' | 'privateKey') => {
  formRef.value?.validateField(field);
};

const onNodeIpInput = () => {
  if (!heartbeatTouched.value) {
    form.heartbeatIp = form.nodeIp;
    formRef.value?.validateField('heartbeatIp');
  }
  validateField('nodeIp');
};

const onHeartbeatInput = () => {
  heartbeatTouched.value = true;
  validateField('heartbeatIp');
};

const onNameInput = () => validateField('name');

const onKeySelected = async (file: any) => {
  const selected = file?.raw || file;
  if (!selected) return;
  keyFileName.value = selected.name || '已选择密钥文件';
  try {
    form.privateKey = await selected.text();
  } catch {
    form.privateKey = '';
  }
  validateField('privateKey');
};

const requestClose = async () => {
  if (submitting.value) return;
  const dirty = JSON.stringify(form) !== snapshot.value;
  if (!dirty) {
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
  if (submitting.value) return;
  const dirty = JSON.stringify(form) !== snapshot.value;
  if (!dirty) {
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
  const valid = await formRef.value?.validate().then(() => true).catch(() => false);
  if (!valid) return;
  submitting.value = true;
  try {
    await createClusterNode({
      id: form.name.trim() || form.nodeIp.trim(),
      address: form.nodeIp.trim(),
      role: form.role,
      heartbeatAddress: form.heartbeatIp.trim(),
      sshPort: Number(form.sshPort),
      authType: form.authType,
      password: form.authType === 'password' ? form.password : undefined,
      privateKey: form.authType === 'key' ? form.privateKey : undefined,
      remark: form.remark.trim() || undefined
    } as any);
    showSuccess('节点新增成功');
    visible.value = false;
    emit('saved');
  } catch (error) {
    showHttpError(error, '节点新增失败');
  } finally {
    submitting.value = false;
  }
};
</script>

<style scoped>
.add-node-dialog :deep(.el-dialog) {
  border-radius: 8px;
}

.add-node-dialog :deep(.el-dialog__header) {
  padding: 32px 32px 0;
}

.add-node-dialog :deep(.el-dialog__title) {
  font-size: 16px;
  font-weight: 500;
}

.add-node-dialog :deep(.el-dialog__body) {
  padding: 32px;
}

.add-node-dialog :deep(.el-dialog__footer) {
  padding: 0 32px 32px;
}

.node-form :deep(.el-form-item) {
  margin-bottom: 18px;
}

.node-form :deep(.el-form-item__label) {
  width: 110px !important;
  justify-content: flex-end;
  padding-right: 12px;
  white-space: nowrap;
}

.node-form :deep(.el-form-item__content) {
  margin-left: 0 !important;
}

.field-label {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
  font-size: 14px;
}

.required-label {
  font-weight: 500;
}

.optional-label {
  font-weight: 400;
}

.required {
  color: var(--el-color-danger);
}

.optional {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.field-wrap {
  width: 440px;
}

.field-wrap :deep(.el-input),
.field-wrap :deep(.el-select),
.field-wrap :deep(.el-input-number),
.field-wrap :deep(.el-textarea),
.key-upload {
  width: 440px;
}

.field-wrap :deep(.el-input__wrapper),
.field-wrap :deep(.el-select__wrapper),
.field-wrap :deep(.el-input-number),
.field-wrap :deep(.el-textarea__inner) {
  min-height: 36px;
  border-radius: 4px;
}

.field-wrap :deep(.el-textarea__inner) {
  height: 80px;
}

.field-hint {
  margin-top: 4px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
