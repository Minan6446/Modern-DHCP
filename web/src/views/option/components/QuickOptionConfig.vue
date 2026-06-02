<template>
  <el-card shadow="never">
    <template #header>
      <div class="card-header">
        <span>{{ t('option.quickTitle') }}</span>
        <span class="hint">{{ t('option.quickHint') }}</span>
      </div>
    </template>
    <el-form ref="formRef" :model="form" :rules="rules" label-width="140px" label-position="left">
      <el-row :gutter="12">
        <el-col :span="12">
          <el-form-item label="Gateway (Opt 3)" prop="gateway">
            <el-input v-model="form.gateway" :placeholder="t('option.quickGatewayRequired')" />
          </el-form-item>
          <el-form-item label="DNS (Opt 6)" prop="dns">
            <el-input v-model="form.dns" :placeholder="t('option.quickDnsRequired')" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="Domain (Opt 15)" prop="domain">
            <el-input v-model="form.domain" placeholder="corp.example.com" />
          </el-form-item>
          <el-form-item label="Vendor (Opt 43)" prop="vendor">
            <el-input v-model="form.vendor" placeholder="0104DEADBEEF" />
          </el-form-item>
        </el-col>
      </el-row>
      <el-form-item>
        <el-button type="primary" @click="emitSubmit">{{ t('option.quickGenerate') }}</el-button>
        <el-button @click="reset">{{ t('option.quickReset') }}</el-button>
      </el-form-item>
    </el-form>
    <el-alert
      v-if="output"
      :title="t('option.quickOutput')"
      type="success"
      :closable="false"
      class="result"
    >
      <div class="result-line">Option 3: {{ output.gateway }}</div>
      <div class="result-line">Option 6: {{ output.dns }}</div>
      <div class="result-line">Option 15: {{ output.domain }}</div>
      <div class="result-line">Option 43: {{ output.vendor }}</div>
    </el-alert>
  </el-card>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue';
import { isIPv4 } from '@/utils/ip';
import { validateByType } from '@/utils/optionValidator';
import { useI18n } from 'vue-i18n';

const form = reactive({ gateway: '', dns: '', domain: '', vendor: '' });
const output = ref<{ gateway: string; dns: string; domain: string; vendor: string }>();

const formRef = ref();
const { t } = useI18n();

const rules = {
  gateway: [
    {
      validator: (_: unknown, val: string, cb: (err?: Error) => void) => {
        const ips = (val || '')
          .split(',')
          .map((i) => i.trim())
          .filter(Boolean);
        if (!ips.length) return cb(new Error(t('option.quickGatewayRequired')));
        const invalid = ips.find((i) => !isIPv4(i));
        if (invalid) return cb(new Error(t('option.quickInvalidIp', { ip: invalid })));
        cb();
      },
      trigger: 'blur'
    }
  ],
  dns: [
    {
      validator: (_: unknown, val: string, cb: (err?: Error) => void) => {
        const ips = (val || '')
          .split(',')
          .map((i) => i.trim())
          .filter(Boolean);
        if (!ips.length) return cb(new Error(t('option.quickDnsRequired')));
        const invalid = ips.find((i) => !isIPv4(i));
        if (invalid) return cb(new Error(t('option.quickInvalidIp', { ip: invalid })));
        cb();
      },
      trigger: 'blur'
    }
  ],
  domain: [
    {
      validator: (_: unknown, val: string, cb: (err?: Error) => void) => {
        const res = validateByType('domain', val || '');
        res.valid ? cb() : cb(new Error(res.message || t('option.quickInvalidDomain')));
      },
      trigger: 'blur'
    }
  ],
  vendor: [
    {
      validator: (_: unknown, val: string, cb: (err?: Error) => void) => {
        const res = validateByType('hex', val || '');
        res.valid ? cb() : cb(new Error(res.message || t('option.quickInvalidHex')));
      },
      trigger: 'blur'
    }
  ]
};

const emitSubmit = () => {
  formRef.value?.validate((valid: boolean) => {
    if (!valid) return;
    output.value = {
      gateway: form.gateway.replace(/\s+/g, ''),
      dns: form.dns.replace(/\s+/g, ''),
      domain: form.domain.trim(),
      vendor: form.vendor.toUpperCase()
    };
  });
};

const reset = () => {
  form.gateway = '';
  form.dns = '';
  form.domain = '';
  form.vendor = '';
  output.value = undefined;
};
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.hint {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.result {
  margin-top: 12px;
}

.result-line {
  font-family:
    ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New',
    monospace;
}
</style>
