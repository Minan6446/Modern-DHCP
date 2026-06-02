<template>
  <el-form
    ref="formRef"
    :model="form"
    :rules="rules"
    label-width="120px"
    label-position="left"
    class="option-editor"
  >
    <el-row :gutter="12">
      <el-col :span="12">
        <el-form-item :label="t('option.editorCode')" prop="code">
          <el-input-number v-model="form.code" :min="1" :max="254" controls-position="right" />
        </el-form-item>
      </el-col>
      <el-col :span="12">
        <el-form-item :label="t('option.editorName')" prop="name">
          <el-input v-model="form.name" :placeholder="t('option.editorName')" />
        </el-form-item>
      </el-col>
      <el-col :span="12">
        <el-form-item :label="t('option.editorType')" prop="dataType">
          <el-select v-model="form.dataType" :placeholder="t('option.editorType')">
            <el-option v-for="t in dataTypes" :key="t" :label="t" :value="t" />
          </el-select>
        </el-form-item>
      </el-col>
      <el-col :span="12">
        <el-form-item :label="t('option.editorCategory')" prop="category">
          <el-select v-model="form.category">
            <el-option :label="t('option.categoryBasic')" value="basic" />
            <el-option :label="t('option.categoryNetwork')" value="network" />
            <el-option :label="t('option.categoryTime')" value="time" />
            <el-option :label="t('option.categorySecurity')" value="security" />
            <el-option :label="t('option.categoryVendor')" value="vendor" />
            <el-option :label="t('option.tabCustom')" value="custom" />
          </el-select>
        </el-form-item>
      </el-col>
      <el-col :span="12">
        <el-form-item :label="t('option.editorLength')">
          <div class="inline">
            <el-input-number v-model="form.minLength" :min="0" placeholder="最小" />
            <span class="sep">-</span>
            <el-input-number v-model="form.maxLength" :min="0" placeholder="最大" />
          </div>
        </el-form-item>
      </el-col>
      <el-col :span="12">
        <el-form-item :label="t('option.editorLengthRec')">
          <el-input-number v-model="form.recommendedLength" :min="0" />
        </el-form-item>
      </el-col>
      <el-col :span="24">
        <el-form-item :label="t('option.editorDesc')">
          <el-input v-model="form.description" type="textarea" :rows="2" />
        </el-form-item>
      </el-col>
      <el-col :span="24">
        <el-form-item :label="t('option.editorExample')">
          <el-input v-model="form.valueExample" />
        </el-form-item>
      </el-col>
      <el-col :span="12">
        <el-form-item :label="t('option.editorAllowed')">
          <el-input
            v-model="form.allowedValuesText"
            type="textarea"
            :rows="3"
            :placeholder="t('option.editorAllowed')"
          />
        </el-form-item>
      </el-col>
      <el-col :span="12">
        <el-form-item :label="t('option.editorPattern')">
          <el-input v-model="form.pattern" :placeholder="t('option.editorPattern')" />
        </el-form-item>
      </el-col>
      <el-col :span="24">
        <el-form-item :label="t('option.editorSample')" prop="sampleValue">
          <hex-editor v-if="form.dataType === 'hex'" v-model="form.sampleValue" />
          <el-input v-else v-model="form.sampleValue" placeholder="用于实时验证" />
          <el-alert
            v-if="sampleValidation"
            :title="sampleValidation"
            :type="sampleValid ? 'success' : 'error'"
            show-icon
            :closable="false"
            class="mt-4"
          />
        </el-form-item>
      </el-col>
    </el-row>
    <el-form-item>
      <el-button type="primary" @click="submit">{{ t('common.save') }}</el-button>
      <el-button @click="reset">{{ t('common.reset') }}</el-button>
    </el-form-item>
  </el-form>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import type { OptionDefinition } from '@/types/option';
import { validateOptionValue } from '@/utils/optionValidator';
import HexEditor from './HexEditor.vue';
import { useI18n } from 'vue-i18n';

const props = defineProps<{ modelValue?: OptionDefinition; existingCodes?: number[] }>();
const emit = defineEmits<{ (e: 'submit', payload: Partial<OptionDefinition>): void }>();
const { t } = useI18n();

const dataTypes = [
  'ip',
  'ip-list',
  'domain',
  'fqdn',
  'string',
  'hex',
  'uint8',
  'uint16',
  'uint32',
  'boolean'
];

const form = reactive({
  code: 0,
  name: '',
  dataType: 'string',
  category: 'custom',
  minLength: undefined as number | undefined,
  maxLength: undefined as number | undefined,
  recommendedLength: undefined as number | undefined,
  description: '',
  valueExample: '',
  allowedValuesText: '',
  pattern: '',
  sampleValue: ''
});

const formRef = ref();

watch(
  () => props.modelValue,
  (val) => {
    if (!val) return;
    form.code = val.code;
    form.name = val.name;
    form.dataType = val.dataType;
    form.category = val.category;
    form.minLength = val.minLength;
    form.maxLength = val.maxLength;
    form.recommendedLength = val.recommendedLength;
    form.description = val.description || '';
    form.valueExample = val.valueExample || '';
    form.allowedValuesText = (val.allowedValues || []).join('\n');
    form.pattern = val.pattern || '';
    form.sampleValue = val.sampleValue || val.value || val.valueExample || '';
  },
  { immediate: true }
);

const rules = {
  code: [
    { required: true, message: t('option.editorRequired'), trigger: 'blur' },
    {
      validator: (_: unknown, val: number, cb: (err?: Error) => void) => {
        if (!val || val < 1 || val > 254) return cb(new Error('1-254'));
        const dup =
          props.existingCodes?.includes(val) &&
          (!props.modelValue || props.modelValue.code !== val);
        if (dup) return cb(new Error(t('option.editorCodeExists')));
        cb();
      },
      trigger: 'blur'
    }
  ],
  name: [{ required: true, message: t('option.editorRequired'), trigger: 'blur' }],
  dataType: [{ required: true, message: t('option.editorRequired'), trigger: 'change' }],
  category: [{ required: true, message: t('option.editorRequired'), trigger: 'change' }],
  sampleValue: [
    {
      validator: (_: unknown, val: string, cb: (err?: Error) => void) => {
        const res = validateOptionValue(defFromForm(), val || '');
        res.valid ? cb() : cb(new Error(res.message || t('option.editorSampleInvalid')));
      },
      trigger: 'blur'
    }
  ]
};

const defFromForm = (): OptionDefinition => {
  const allowedValues = form.allowedValuesText
    .split('\n')
    .map((v) => v.trim())
    .filter(Boolean);
  const primaryValue = form.sampleValue || form.valueExample || allowedValues[0] || '';
  return {
    code: form.code,
    name: form.name,
    dataType: form.dataType as OptionDefinition['dataType'],
    category: form.category as OptionDefinition['category'],
    minLength: form.minLength,
    maxLength: form.maxLength,
    recommendedLength: form.recommendedLength,
    description: form.description,
    valueExample: form.valueExample,
    sampleValue: form.sampleValue,
    value: primaryValue,
    allowedValues,
    pattern: form.pattern
  };
};

const sampleCheck = computed(() => validateOptionValue(defFromForm(), form.sampleValue || ''));
const sampleValid = computed(() => sampleCheck.value.valid);
const sampleValidation = computed(() =>
  form.sampleValue ? sampleCheck.value.message || t('option.editorSample') : ''
);

const submit = () => {
  formRef.value?.validate((valid: boolean) => {
    if (!valid) return;
    emit('submit', defFromForm());
  });
};

const reset = () => {
  form.code = 0;
  form.name = '';
  form.dataType = 'string';
  form.category = 'custom';
  form.minLength = undefined;
  form.maxLength = undefined;
  form.recommendedLength = undefined;
  form.description = '';
  form.valueExample = '';
  form.allowedValuesText = '';
  form.pattern = '';
  form.sampleValue = '';
};
</script>

<style scoped>
.option-editor {
  padding: 4px 8px;
}

.inline {
  display: flex;
  align-items: center;
  gap: 6px;
}

.sep {
  color: var(--el-text-color-secondary);
}

.mt-4 {
  margin-top: 8px;
}
</style>
