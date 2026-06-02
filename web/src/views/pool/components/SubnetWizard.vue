<template>
  <el-dialog
    v-model="visible"
    width="680px"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    destroy-on-close
    class="subnet-wizard-dialog"
    @close="handleDialogClose"
    :before-close="handleBeforeClose"
  >
    <template #header>
      <div class="wizard-title">{{ t('pool.wizardDialogTitle') }}</div>
    </template>

    <div class="wizard-shell">
      <div class="wizard-steps">
        <button
          v-for="(item, index) in stepItems"
          :key="item.key"
          type="button"
          class="step-item"
          :class="stepClass(index)"
          :disabled="!canJumpTo(index)"
          @click="jumpStep(index)"
        >
          <span class="step-icon">{{ index < currentStep ? '✓' : index + 1 }}</span>
          <span class="step-title">{{ item.title }}</span>
        </button>
      </div>

      <AppErrorCallout v-if="errors" :error="errors" class="mb-12" />

      <el-form :model="draft" label-width="120px" label-position="right" class="wizard-form">
        <template v-if="currentStep === 0">
          <div class="quick-fill-block">
            <div class="quick-fill-title">{{ t('pool.wizardQuickFill') }} <span class="optional">{{ t('pool.wizardOptional') }}</span></div>
            <el-form-item :error="fieldErrors.templateSource">
              <template #label>
                <span class="field-label optional-label">{{ t('pool.wizardTemplateFill') }} <span class="optional">{{ t('pool.wizardOptional') }}</span></span>
              </template>
              <div class="field-wrap">
                <div class="field-row">
                  <el-select v-model="selectedTemplateName" class="field-control" :placeholder="t('pool.wizardFromTemplate')" clearable>
                    <el-option v-for="item in templates" :key="item.name" :label="item.name" :value="item.name" />
                  </el-select>
                  <el-button @click="applyTemplate">{{ t('pool.wizardApplyTemplate') }}</el-button>
                </div>
                <div class="field-row">
                  <el-select v-model="selectedSubnetId" class="field-control" :placeholder="t('pool.wizardFromSubnet')" clearable>
                    <el-option v-for="item in normalizedSubnets" :key="item.id" :label="`${item.name} (${item.cidr})`" :value="item.id" />
                  </el-select>
                  <el-button @click="applySubnet">{{ t('pool.wizardApplySubnet') }}</el-button>
                </div>
                <div class="field-hint">{{ t('pool.wizardQuickFillHint') }}</div>
              </div>
            </el-form-item>
          </div>

          <el-divider class="block-divider" />

          <el-form-item :error="fieldErrors.name">
            <template #label>
              <span class="field-label required-label"><span class="required">*</span>{{ t('pool.labelName') }}</span>
            </template>
            <div class="field-wrap">
              <el-input v-model="draft.name" class="field-control" :placeholder="t('pool.wizardNamePlaceholder')" @input="clearFieldError('name')" />
              <div class="field-hint">{{ t('pool.wizardNameHint') }}</div>
            </div>
          </el-form-item>

          <el-form-item :error="fieldErrors.cidr">
            <template #label>
              <span class="field-label required-label"><span class="required">*</span>CIDR</span>
            </template>
            <div class="field-wrap">
              <el-input v-model="draft.cidr" class="field-control" :placeholder="t('pool.wizardCidrPlaceholder')" @input="onCidrInput" />
              <div class="field-hint">{{ t('pool.wizardCidrHint') }}</div>
              <div v-if="computedRange" class="cidr-hint-bar">{{ rangeText }}</div>
            </div>
          </el-form-item>
        </template>

        <template v-else-if="currentStep === 1">
          <el-form-item :error="fieldErrors.gateway">
            <template #label>
              <span class="field-label required-label"><span class="required">*</span>{{ t('pool.labelGateway') }}</span>
            </template>
            <div class="field-wrap">
              <el-input v-model="draft.gateway" class="field-control" :placeholder="t('pool.wizardGatewayPlaceholder')" @input="validateGatewayRealtime" />
              <div class="field-hint">{{ t('pool.wizardGatewayHint') }}</div>
            </div>
          </el-form-item>

          <el-form-item :error="fieldErrors.rangeStart">
            <template #label>
              <span class="field-label required-label"><span class="required">*</span>{{ t('pool.labelRangeStart') }}</span>
            </template>
            <div class="field-wrap">
              <el-input v-model="draft.rangeStart" class="field-control" :placeholder="t('pool.rangeStartPlaceholder')" @input="validateRangeRealtime" />
              <div class="field-hint">{{ t('pool.wizardRangeStartHint') }}</div>
            </div>
          </el-form-item>

          <el-form-item :error="fieldErrors.rangeEnd">
            <template #label>
              <span class="field-label required-label"><span class="required">*</span>{{ t('pool.labelRangeEnd') }}</span>
            </template>
            <div class="field-wrap">
              <el-input v-model="draft.rangeEnd" class="field-control" :placeholder="t('pool.rangeEndPlaceholder')" @input="validateRangeRealtime" />
              <div class="field-hint">{{ t('pool.wizardRangeEndHint') }}</div>
            </div>
          </el-form-item>

          <el-form-item :error="fieldErrors.dns">
            <template #label>
              <span class="field-label optional-label">DNS <span class="optional">{{ t('pool.wizardOptional') }}</span></span>
            </template>
            <div class="field-wrap optional-zone">
              <div class="field-row dns-row">
                <el-input v-model="dnsPrimary" class="field-control" :placeholder="t('pool.dnsPrimaryPlaceholder')" @input="clearFieldError('dns')" />
                <el-input v-model="dnsSecondary" class="field-control" :placeholder="t('pool.dnsSecondaryPlaceholder')" @input="clearFieldError('dns')" />
              </div>
              <div class="field-hint">{{ t('pool.wizardDnsHint') }}</div>
            </div>
          </el-form-item>

          <el-form-item :error="fieldErrors.exclude">
            <template #label>
              <span class="field-label optional-label">{{ t('pool.labelExclude') }} <span class="optional">{{ t('pool.wizardOptional') }}</span></span>
            </template>
            <div class="field-wrap optional-zone">
              <el-input
                v-model="excludeInput"
                class="field-control"
                type="textarea"
                :autosize="false"
                :placeholder="t('pool.excludePlaceholder')"
                @input="clearFieldError('exclude')"
              />
              <div class="field-hint">{{ t('pool.wizardExcludeHint') }}</div>
              <div class="tags" v-if="draft.exclude.length">
                <el-tag v-for="ip in draft.exclude" :key="ip" closable @close="removeExclude(ip)">{{ ip }}</el-tag>
              </div>
            </div>
          </el-form-item>

          <el-form-item :error="fieldErrors.option43">
            <template #label>
              <span class="field-label optional-label">Option43 <span class="optional">{{ t('pool.wizardOptional') }}</span></span>
            </template>
            <div class="field-wrap optional-zone">
              <el-input v-model="draft.option43" class="field-control" :placeholder="t('pool.wizardOption43Placeholder')" @input="clearFieldError('option43')" />
              <div class="field-hint">{{ t('pool.wizardOption43Hint') }}</div>
            </div>
          </el-form-item>
        </template>

        <template v-else-if="currentStep === 2">
          <el-form-item :error="fieldErrors['strategy.mode']">
            <template #label>
              <span class="field-label required-label"><span class="required">*</span>{{ t('pool.allocationTitle') }}</span>
            </template>
            <div class="field-wrap">
              <el-select v-model="draft.strategy.mode" class="field-control" :placeholder="t('pool.wizardStrategyPlaceholder')" @change="clearFieldError('strategy.mode')">
                <el-option value="round-robin" :label="t('pool.allocationRoundRobin')" />
                <el-option value="sequential" :label="t('pool.allocationSequential')" />
                <el-option value="random" :label="t('pool.wizardWeightedRandom')" />
              </el-select>
              <div class="field-hint">{{ t('pool.wizardStrategyHint') }}</div>
            </div>
          </el-form-item>

          <el-form-item :error="fieldErrors.leaseTime">
            <template #label>
              <span class="field-label required-label"><span class="required">*</span>{{ t('pool.wizardMinLeaseLabel') }}</span>
            </template>
            <div class="field-wrap">
              <el-input
                v-model.number="draft.leaseTime"
                class="field-control"
                type="number"
                min="300"
                max="604800"
                :placeholder="t('pool.wizardLeaseTimePlaceholder')"
                @input="validateLeaseTime"
              />
              <div class="field-hint">{{ t('pool.wizardLeaseTimeHint') }}</div>
            </div>
          </el-form-item>

          <el-form-item :error="fieldErrors.maxLeaseTime">
            <template #label>
              <span class="field-label required-label"><span class="required">*</span>{{ t('pool.wizardMaxLeaseLabel') }}</span>
            </template>
            <div class="field-wrap">
              <el-input
                v-model.number="draft.maxLeaseTime"
                class="field-control"
                type="number"
                min="300"
                max="604800"
                :placeholder="t('pool.wizardMaxLeaseTimePlaceholder')"
                @input="validateMaxLeaseTime"
              />
              <div class="field-hint">{{ t('pool.wizardMaxLeaseTimeHint') }}</div>
            </div>
          </el-form-item>

          <el-form-item :error="fieldErrors.vlanId">
            <template #label>
              <span class="field-label optional-label">VLAN <span class="optional">{{ t('pool.wizardOptional') }}</span></span>
            </template>
            <div class="field-wrap optional-zone">
              <el-input-number
                v-model="draft.vlanId"
                class="field-control"
                :min="1"
                :max="4094"
                :controls="false"
                @change="validateVlan"
              />
              <div class="field-hint">{{ t('pool.wizardVlanHint') }}</div>
            </div>
          </el-form-item>

          <el-form-item :error="fieldErrors.location">
            <template #label>
              <span class="field-label optional-label">{{ t('pool.labelLocation') }} <span class="optional">{{ t('pool.wizardOptional') }}</span></span>
            </template>
            <div class="field-wrap optional-zone">
              <el-input v-model="draft.location" class="field-control" :placeholder="t('pool.wizardLocationPlaceholder')" @input="clearFieldError('location')" />
              <div class="field-hint">{{ t('pool.wizardLocationHint') }}</div>
            </div>
          </el-form-item>
        </template>

        <template v-else>
          <div class="preview-head">
            <h4>{{ t('pool.wizardPreview') }}</h4>
            <el-button size="small" @click="saveTemplate">{{ t('pool.wizardSaveTemplate') }}</el-button>
          </div>
          <div class="preview-group">
            <div class="preview-title">{{ t('pool.wizardBasicInfo') }}</div>
            <el-descriptions :column="1" border class="preview-box">
              <el-descriptions-item :label="t('pool.labelName')">{{ draft.name }}</el-descriptions-item>
              <el-descriptions-item label="CIDR">{{ draft.cidr }}</el-descriptions-item>
            </el-descriptions>
          </div>
          <div class="preview-group">
            <div class="preview-title">{{ t('pool.wizardNetworkConfig') }}</div>
            <el-descriptions :column="1" border class="preview-box">
              <el-descriptions-item :label="t('pool.labelGateway')">{{ draft.gateway || '-' }}</el-descriptions-item>
              <el-descriptions-item :label="t('pool.wizardAddressRange')">{{ draft.rangeStart || '-' }} ~ {{ draft.rangeEnd || '-' }}</el-descriptions-item>
              <el-descriptions-item label="DNS">{{ (draft.dns || []).join(', ') || '-' }}</el-descriptions-item>
              <el-descriptions-item :label="t('pool.labelExclude')">{{ draft.exclude.join(', ') || '-' }}</el-descriptions-item>
            </el-descriptions>
          </div>
          <div class="preview-group">
            <div class="preview-title">{{ t('pool.wizardLeaseStrategy') }}</div>
            <el-descriptions :column="1" border class="preview-box">
              <el-descriptions-item :label="t('pool.allocationTitle')">{{ strategyLabel }}</el-descriptions-item>
              <el-descriptions-item :label="t('pool.wizardMinLeaseLabel')">{{ t('pool.wizardLeaseTimeSec', { value: draft.leaseTime }) }}</el-descriptions-item>
              <el-descriptions-item :label="t('pool.wizardMaxLeaseLabel')">{{ draft.maxLeaseTime ? t('pool.wizardLeaseTimeSec', { value: draft.maxLeaseTime }) : '-' }}</el-descriptions-item>
              <el-descriptions-item label="VLAN">{{ draft.vlanId ?? '-' }}</el-descriptions-item>
              <el-descriptions-item :label="t('pool.labelLocation')">{{ draft.location || '-' }}</el-descriptions-item>
            </el-descriptions>
          </div>
        </template>
      </el-form>
    </div>

    <template #footer>
      <div class="wizard-footer">
        <template v-if="currentStep < 3">
          <el-button :loading="cancelLoading" :disabled="submitting || nextLoading || prevLoading" @click="closeWizard">{{ t('common.cancel') }}</el-button>
          <el-button :loading="prevLoading" :disabled="currentStep === 0 || submitting || nextLoading" @click="prevStep">{{ t('pool.wizardPrev') }}</el-button>
          <el-button v-if="currentStep < 3" type="primary" :loading="nextLoading" :disabled="submitting || prevLoading" @click="nextStep">{{ t('pool.wizardNext') }}</el-button>
        </template>
        <template v-else>
          <el-button :loading="prevLoading" :disabled="submitting" @click="prevStep">{{ t('pool.wizardPrev') }}</el-button>
          <el-button type="primary" :loading="submitting" :disabled="submitting || nextLoading || prevLoading" @click="submitWizard">{{ t('pool.wizardSubmit') }}</el-button>
        </template>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, reactive, ref, watch } from 'vue';
import type { SubnetDraft } from '@/types/pool';
import {
  calcIPv4Range,
  cidrOverlap,
  dedupeIps,
  ipv4ToInt,
  isIPv4,
  validateRangeWithin
} from '@/utils/ip';
import { showError } from '@/shared/errors/messageToast';
import { useI18n } from 'vue-i18n';
import AppErrorCallout from '@/components/common/AppErrorCallout.vue';
import { mapFieldErrors } from '@/shared/errors/apiError';
import type { ApiErrorDescriptor } from '@/shared/errors/apiError';
import { ElMessageBox } from 'element-plus';
import { showSuccess } from '@/shared/errors/messageToast';

const props = defineProps<{
  modelValue: boolean;
  value: SubnetDraft | null;
  existingCidrs?: string[];
  existingSubnets?: Array<Record<string, any>>;
  errors?: ApiErrorDescriptor | null;
  submitting?: boolean;
}>();
const emits = defineEmits<{
  (e: 'update:modelValue', val: boolean): void;
  (e: 'submit', draft: SubnetDraft): void;
}>();

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emits('update:modelValue', v)
});

const { t } = useI18n();

const submitting = computed(() => Boolean(props.submitting));
const errors = computed(() => props.errors || null);
const fieldErrors = reactive<Record<string, string>>({});
const fieldAlias: Record<string, string> = {
  name: 'name',
  cidr: 'cidr',
  gateway: 'gateway',
  dns: 'dns',
  option43: 'option43',
  exclude: 'exclude',
  rangeStart: 'rangeStart',
  rangeEnd: 'rangeEnd',
  'strategy.mode': 'strategy.mode',
  leaseTime: 'leaseTime',
  maxLeaseTime: 'maxLeaseTime',
  capacityThreshold: 'capacityThreshold',
  vlanId: 'vlanId',
  location: 'location'
};

const syncFieldErrors = () => {
  Object.keys(fieldErrors).forEach((key) => delete fieldErrors[key]);
  const mapped = mapFieldErrors(errors.value, fieldAlias);
  Object.assign(fieldErrors, mapped);
};

watch(
  () => props.errors,
  () => syncFieldErrors(),
  { immediate: true }
);

const clearFieldError = (key: string) => {
  if (fieldErrors[key]) delete fieldErrors[key];
};

const existingCidrs = computed(() => props.existingCidrs || []);
const normalizedSubnets = computed(() => (props.existingSubnets || []).map((item) => ({
  id: String(item.id || item.cidr || Math.random()),
  name: String(item.name || t('pool.wizardUnnamed')),
  cidr: String(item.cidr || ''),
  gateway: item.gateway,
  rangeStart: item.rangeStart,
  rangeEnd: item.rangeEnd,
  vlanId: item.vlanId,
  location: item.location
})));

const draft = reactive<SubnetDraft>({
  name: props.value?.name || '',
  cidr: props.value?.cidr || '',
  gateway: props.value?.gateway,
  option43: props.value?.option43,
  dns: props.value?.dns || [],
  leaseTime: props.value?.leaseTime || 1800,
  maxLeaseTime: props.value?.maxLeaseTime || 7200,
  exclude: props.value?.exclude || [],
  rangeStart: props.value?.rangeStart,
  rangeEnd: props.value?.rangeEnd,
  strategy: props.value?.strategy || { mode: 'round-robin' },
  vlanId: props.value?.vlanId,
  location: props.value?.location
});

const stepItems = computed(() => [
  { key: 'basic', title: t('pool.wizardStepBasic') },
  { key: 'network', title: t('pool.wizardStepNetwork') },
  { key: 'strategy', title: t('pool.wizardStepStrategy') },
  { key: 'preview', title: t('pool.wizardStepPreview') }
]);

const currentStep = ref(0);
const maxVisitedStep = ref(0);
const computedRange = ref<{ firstHost: string; lastHost: string; hostCount: number } | null>(null);
const excludeInput = ref('');
const dnsPrimary = ref('');
const dnsSecondary = ref('');
const selectedTemplateName = ref('');
const selectedSubnetId = ref('');
const templates = ref<Array<{ name: string; payload: SubnetDraft }>>([]);
const snapshot = ref('');
const nextLoading = ref(false);
const prevLoading = ref(false);
const cancelLoading = ref(false);

const rangeText = computed(() =>
  computedRange.value
    ? t('pool.rangeText', {
        count: computedRange.value.hostCount,
        first: computedRange.value.firstHost,
        last: computedRange.value.lastHost
      })
    : ''
);

const strategyLabel = computed(() => {
  if (draft.strategy.mode === 'sequential') return t('pool.allocationSequential');
  if (draft.strategy.mode === 'random') return t('pool.wizardWeightedRandom');
  return t('pool.allocationRoundRobin');
});

const stepClass = (index: number) => ({
  done: index < currentStep.value,
  current: index === currentStep.value,
  upcoming: index > currentStep.value
});

const canJumpTo = (index: number) => index <= maxVisitedStep.value;

const isDirty = computed(() => snapshot.value !== JSON.stringify(draft));

const calcRange = () => {
  const r = calcIPv4Range(draft.cidr);
  computedRange.value = r;
};

const onCidrInput = () => {
  clearFieldError('cidr');
  calcRange();
  if (!draft.cidr.trim()) {
    fieldErrors.cidr = t('pool.wizardEnterCidr');
    return;
  }
  if (!computedRange.value) {
    fieldErrors.cidr = t('pool.wizardCidrInvalid');
    return;
  }
  if (existingCidrs.value.some((cidr) => cidrOverlap(draft.cidr, cidr))) {
    fieldErrors.cidr = t('pool.wizardCidrConflict');
    return;
  }
  const start = ipv4ToInt(computedRange.value.firstHost);
  const end = ipv4ToInt(computedRange.value.lastHost);
  draft.gateway = computedRange.value.firstHost;
  draft.rangeStart = intToIPv4Safe(Math.min(start + 1, end));
  draft.rangeEnd = computedRange.value.lastHost;
  clearFieldError('gateway');
  clearFieldError('rangeStart');
  clearFieldError('rangeEnd');
};

const validateGatewayRealtime = () => {
  clearFieldError('gateway');
  if (!draft.gateway) return;
  if (!isIPv4(draft.gateway) || !validateRangeWithin(draft.cidr, draft.gateway)) {
    fieldErrors.gateway = t('pool.wizardGatewayInvalid');
  }
};

const validateRangeRealtime = () => {
  clearFieldError('rangeStart');
  clearFieldError('rangeEnd');
  validateRange();
};

const intToIPv4Safe = (value: number) => {
  const v = Math.max(0, Math.min(0xffffffff, value));
  return [24, 16, 8, 0].map((shift) => (v >>> shift) & 255).join('.');
};

const applyDraft = (value: SubnetDraft | null) => {
  draft.name = value?.name || '';
  draft.cidr = value?.cidr || '';
  draft.gateway = value?.gateway;
  draft.option43 = value?.option43;
  draft.dns = value?.dns || [];
  draft.leaseTime = value?.leaseTime || 1800;
  draft.maxLeaseTime = value?.maxLeaseTime || 7200;
  draft.exclude = value?.exclude || [];
  draft.rangeStart = value?.rangeStart;
  draft.rangeEnd = value?.rangeEnd;
  draft.strategy = value?.strategy || { mode: 'round-robin' };
  draft.vlanId = value?.vlanId;
  draft.location = value?.location;
  excludeInput.value = '';
  dnsPrimary.value = draft.dns[0] || '';
  dnsSecondary.value = draft.dns[1] || '';
  currentStep.value = 0;
  maxVisitedStep.value = 0;
  loadTemplates();
  snapshot.value = JSON.stringify(draft);
  calcRange();
};

watch(
  () => props.value,
  (val) => {
    applyDraft(val || null);
  },
  { immediate: true }
);

const applyDns = () => {
  clearFieldError('dns');
  const primary = dnsPrimary.value.split(/\s|,|\r?\n/)[0]?.trim() || '';
  const secondary = dnsSecondary.value.split(/\s|,|\r?\n/)[0]?.trim() || '';
  const items = [primary, secondary].filter(Boolean);
  draft.dns = dedupeIps(items);
};

const applyExclude = () => {
  clearFieldError('exclude');
  const items = excludeInput.value
    .split(/\r?\n|,/)
    .map((i: string) => i.trim())
    .filter(Boolean);
  const ok: string[] = [];
  for (const token of items) {
    if (token.includes('-')) {
      const [startRaw, endRaw] = token.split('-').map((part) => part.trim());
      if (!isIPv4(startRaw) || !isIPv4(endRaw)) continue;
      if (
        computedRange.value &&
        (!validateRangeWithin(draft.cidr, startRaw) || !validateRangeWithin(draft.cidr, endRaw))
      ) {
        continue;
      }
      if (ipv4ToInt(startRaw) > ipv4ToInt(endRaw)) continue;
      ok.push(startRaw === endRaw ? startRaw : `${startRaw}-${endRaw}`);
      continue;
    }
    if (!isIPv4(token)) continue;
    if (computedRange.value && !validateRangeWithin(draft.cidr, token)) continue;
    ok.push(token);
  }
  draft.exclude = dedupeIps([...draft.exclude, ...ok]);
};

const removeExclude = (ip: string) => {
  draft.exclude = draft.exclude.filter((x: string) => x !== ip);
};

const validateRange = () => {
  clearFieldError('rangeStart');
  clearFieldError('rangeEnd');
  const start = (draft.rangeStart || '').trim();
  const end = (draft.rangeEnd || '').trim();
  if (!start && !end) return true;

  let ok = true;
  const startValid = !start || isIPv4(start);
  const endValid = !end || isIPv4(end);
  if (!startValid || !endValid) {
    if (!startValid) fieldErrors.rangeStart = t('pool.rangeInvalid');
    if (!endValid) fieldErrors.rangeEnd = t('pool.rangeInvalid');
    ok = false;
  }

  if (ok && computedRange.value) {
    if (start && !validateRangeWithin(draft.cidr, start)) {
      fieldErrors.rangeStart = t('pool.rangeOutOfCidr');
      ok = false;
    }
    if (end && !validateRangeWithin(draft.cidr, end)) {
      fieldErrors.rangeEnd = t('pool.rangeOutOfCidr');
      ok = false;
    }
  }

  if (ok && start && end && ipv4ToInt(start) > ipv4ToInt(end)) {
    fieldErrors.rangeStart = t('pool.rangeOrderInvalid');
    fieldErrors.rangeEnd = t('pool.rangeOrderInvalid');
    ok = false;
  }

  return ok;
};

const validateExcludeConflict = () => {
  if (!draft.exclude.length) return true;
  const ranges = draft.exclude
    .map((entry) => {
      if (entry.includes('-')) {
        const [start, end] = entry.split('-').map((it) => it.trim());
        return { start: ipv4ToInt(start), end: ipv4ToInt(end), raw: entry };
      }
      const value = ipv4ToInt(entry);
      return { start: value, end: value, raw: entry };
    })
    .sort((a, b) => a.start - b.start);
  for (let i = 1; i < ranges.length; i += 1) {
    if (ranges[i].start <= ranges[i - 1].end) {
      fieldErrors.exclude = t('pool.wizardExcludeConflict', { a: ranges[i - 1].raw, b: ranges[i].raw });
      return false;
    }
  }
  if (draft.rangeStart && draft.rangeEnd) {
    const start = ipv4ToInt(draft.rangeStart);
    const end = ipv4ToInt(draft.rangeEnd);
    if (ranges.some((item) => item.start < start || item.end > end)) {
      fieldErrors.exclude = t('pool.wizardExcludeOutOfRange');
      return false;
    }
  }
  return true;
};

const validateLeaseTime = () => {
  clearFieldError('leaseTime');
  clearFieldError('maxLeaseTime');
  const minLease = Number(draft.leaseTime);
  const maxLease = Number(draft.maxLeaseTime);
  let ok = true;
  if (!Number.isFinite(minLease) || minLease < 300 || minLease > 604800) {
    fieldErrors.leaseTime = t('pool.wizardLeaseTimeRange');
    ok = false;
  }
  if (Number.isFinite(maxLease) && maxLease > 0 && minLease > maxLease) {
    fieldErrors.leaseTime = t('pool.wizardMinLeaseTooBig');
    fieldErrors.maxLeaseTime = t('pool.wizardMaxLeaseTooSmall');
    ok = false;
  }
  return ok;
};

const validateMaxLeaseTime = () => {
  clearFieldError('maxLeaseTime');
  clearFieldError('leaseTime');
  const minLease = Number(draft.leaseTime);
  const maxLease = Number(draft.maxLeaseTime);
  let ok = true;
  if (!Number.isFinite(maxLease) || maxLease < 300 || maxLease > 604800) {
    fieldErrors.maxLeaseTime = t('pool.wizardMaxLeaseTimeRange');
    ok = false;
  }
  if (Number.isFinite(minLease) && minLease > 0 && maxLease < minLease) {
    fieldErrors.maxLeaseTime = t('pool.wizardMaxLeaseTooSmall');
    fieldErrors.leaseTime = t('pool.wizardMinLeaseTooBig');
    ok = false;
  }
  return ok;
};

const validateVlan = () => {
  clearFieldError('vlanId');
  if (draft.vlanId === undefined || draft.vlanId === null || draft.vlanId === ('' as any)) return true;
  const value = Number(draft.vlanId);
  if (!Number.isInteger(value) || value < 1 || value > 4094) {
    fieldErrors.vlanId = t('pool.wizardVlanInvalid');
    return false;
  }
  return true;
};

const validateStep = (index: number) => {
  if (index === 0) {
    if (!draft.name.trim()) fieldErrors.name = t('pool.wizardEnterName');
    if (!draft.cidr.trim()) fieldErrors.cidr = t('pool.wizardEnterCidr');
    if (draft.cidr && !calcIPv4Range(draft.cidr)) fieldErrors.cidr = t('pool.wizardCidrInvalid');
    if (draft.cidr && existingCidrs.value.some((cidr) => cidrOverlap(draft.cidr, cidr))) {
      fieldErrors.cidr = t('pool.wizardCidrConflict');
    }
    return !fieldErrors.name && !fieldErrors.cidr;
  }
  if (index === 1) {
    if (!draft.gateway) fieldErrors.gateway = t('pool.wizardEnterGateway');
    if (draft.gateway && (!isIPv4(draft.gateway) || !validateRangeWithin(draft.cidr, draft.gateway))) {
      fieldErrors.gateway = t('pool.wizardGatewayInvalid');
    }
    if (!draft.rangeStart) fieldErrors.rangeStart = t('pool.wizardEnterRangeStart');
    if (!draft.rangeEnd) fieldErrors.rangeEnd = t('pool.wizardEnterRangeEnd');
    const okRange = validateRange();
    applyDns();
    applyExclude();
    const okExclude = validateExcludeConflict();
    return !fieldErrors.gateway && okRange && okExclude;
  }
  if (index === 2) {
    if (!draft.strategy?.mode) fieldErrors['strategy.mode'] = t('pool.wizardSelectStrategy');
    const leaseOk = validateLeaseTime();
    const maxLeaseOk = validateMaxLeaseTime();
    const vlanOk = validateVlan();
    return !fieldErrors['strategy.mode'] && leaseOk && maxLeaseOk && vlanOk;
  }
  return true;
};

const nextStep = () => {
  nextLoading.value = true;
  if (!validateStep(currentStep.value)) {
    void scrollToFirstError();
    showError(t('pool.wizardStepError'));
    nextLoading.value = false;
    return;
  }
  if (currentStep.value < 3) {
    currentStep.value += 1;
    maxVisitedStep.value = Math.max(maxVisitedStep.value, currentStep.value);
  }
  nextLoading.value = false;
};

const prevStep = () => {
  prevLoading.value = true;
  if (currentStep.value > 0) currentStep.value -= 1;
  prevLoading.value = false;
};

const jumpStep = (index: number) => {
  if (canJumpTo(index)) currentStep.value = index;
};

const submitWizard = () => {
  for (let i = 0; i < 3; i += 1) {
    if (!validateStep(i)) {
      currentStep.value = i;
      void scrollToFirstError();
      showError(t('pool.wizardValidationError'));
      return;
    }
  }
  applyDns();
  applyExclude();
  emits('submit', { ...draft });
};

function loadTemplates() {
  try {
    const raw = localStorage.getItem('ipv4-subnet-templates');
    templates.value = raw ? JSON.parse(raw) : [];
  } catch {
    templates.value = [];
  }
}

const saveTemplate = async () => {
  const name = draft.name?.trim();
  if (!name) {
    showError(t('pool.wizardNameRequired'));
    return;
  }
  const payload: SubnetDraft = JSON.parse(JSON.stringify(draft));
  const next = templates.value.filter((item) => item.name !== name);
  next.unshift({ name, payload });
  templates.value = next.slice(0, 30);
  localStorage.setItem('ipv4-subnet-templates', JSON.stringify(templates.value));
  showSuccess(t('pool.wizardTemplateSaved'));
};

const applyTemplate = () => {
  if (!selectedTemplateName.value) return;
  const item = templates.value.find((tpl) => tpl.name === selectedTemplateName.value);
  if (!item) return;
  applyDraft(item.payload);
};

const applySubnet = () => {
  if (!selectedSubnetId.value) return;
  const item = normalizedSubnets.value.find((subnet) => subnet.id === selectedSubnetId.value);
  if (!item) return;
  draft.name = `${item.name}${t('pool.wizardCopySuffix')}`;
  draft.cidr = item.cidr;
  draft.gateway = item.gateway;
  draft.rangeStart = item.rangeStart;
  draft.rangeEnd = item.rangeEnd;
  draft.vlanId = item.vlanId;
  draft.location = item.location;
  draft.leaseTime = 1800;
  draft.maxLeaseTime = 7200;
  calcRange();
};

const handleBeforeClose = async (done: () => void) => {
  if (!isDirty.value || submitting.value) {
    done();
    return;
  }
  try {
    await ElMessageBox.confirm(t('pool.wizardUnsavedConfirm'), t('pool.wizardUnsavedTitle'), {
      type: 'warning',
      confirmButtonText: t('pool.wizardConfirmClose'),
      cancelButtonText: t('pool.wizardContinueEdit')
    });
    done();
  } catch {
    return;
  }
};

const closeWizard = () => {
  if (submitting.value) return;
  cancelLoading.value = true;
  visible.value = false;
  cancelLoading.value = false;
};

const scrollToFirstError = async () => {
  await nextTick();
  const firstError = document.querySelector('.subnet-wizard-dialog .field-error') as HTMLElement | null;
  firstError?.scrollIntoView({ behavior: 'smooth', block: 'center' });
};

const handleDialogClose = () => {
  currentStep.value = 0;
  maxVisitedStep.value = 0;
  selectedTemplateName.value = '';
  selectedSubnetId.value = '';
  Object.keys(fieldErrors).forEach((key) => delete fieldErrors[key]);
};
</script>

<style scoped>
.subnet-wizard-dialog :deep(.el-dialog) {
  border-radius: 8px;
  min-height: 700px;
}

.subnet-wizard-dialog :deep(.el-dialog__header) {
  padding: 32px 36px 0;
}

.subnet-wizard-dialog :deep(.el-dialog__body) {
  padding: 32px 36px;
}

.subnet-wizard-dialog :deep(.el-dialog__footer) {
  padding: 0 36px 32px;
}

.wizard-title {
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
}

.wizard-shell {
  min-height: 700px;
  display: flex;
  flex-direction: column;
}

.wizard-steps {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 16px;
}

.step-item {
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  background: var(--el-bg-color);
  color: var(--el-text-color-secondary);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px;
}

.step-item .step-icon {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  border: 1px solid currentColor;
}

.step-item.current,
.step-item.done {
  color: var(--el-color-primary);
  border-color: color-mix(in oklab, var(--el-color-primary) 45%, var(--el-border-color));
}

.step-item.done .step-icon {
  background: var(--el-color-primary);
  border-color: var(--el-color-primary);
  color: #fff;
}

.step-item.current .step-icon {
  border-color: var(--el-color-primary);
  color: var(--el-color-primary);
}

.step-item.upcoming .step-icon {
  color: var(--el-text-color-placeholder);
  border-color: var(--el-border-color);
}

.step-item:disabled {
  opacity: 0.8;
  cursor: not-allowed;
}

.wizard-form {
  flex: 1;
}

.wizard-form :deep(.el-form-item) {
  margin-bottom: 18px;
}

.wizard-form :deep(.el-form-item__content) {
  margin-left: 0 !important;
}

.field-label {
  width: 110px;
  display: inline-flex;
  justify-content: flex-end;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
  font-weight: 400;
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
  font-size: 12px;
  color: var(--el-text-color-placeholder);
}

.field-wrap {
  width: 100%;
  margin-left: 12px;
}

.field-wrap :deep(.el-input),
.field-wrap :deep(.el-select),
.field-wrap :deep(.el-input-number),
.field-wrap :deep(.el-textarea) {
  width: 420px;
}

.field-row {
  display: grid;
  grid-template-columns: 420px auto;
  gap: 8px;
  margin-bottom: 8px;
}

.dns-row {
  grid-template-columns: 206px 206px;
}

.dns-row :deep(.el-input) {
  width: 100%;
}

.field-wrap :deep(.el-input__wrapper),
.field-wrap :deep(.el-select__wrapper),
.field-wrap :deep(.el-input-number),
.field-wrap :deep(.el-textarea__inner) {
  min-height: 36px;
}

.field-wrap :deep(.el-textarea__inner) {
  height: 80px;
}

.field-hint {
  margin-top: 4px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  text-align: left;
}

.cidr-hint-bar {
  width: 420px;
  margin-top: 4px;
  padding: 8px 10px;
  border-radius: 6px;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-regular);
  font-size: 12px;
}

.quick-fill-block {
  padding: 12px;
  border-radius: 8px;
  background: var(--el-fill-color-lighter);
  margin-bottom: 6px;
}

.quick-fill-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--el-text-color-primary);
  margin-bottom: 10px;
}

.block-divider {
  margin: 6px 0 14px;
}

.optional-zone {
  padding: 10px;
  border-radius: 8px;
  background: var(--el-fill-color-lighter);
}

.hint-icon {
  margin-left: 4px;
  color: var(--el-text-color-secondary);
}

.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}

.preview-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.preview-box {
  background: var(--el-fill-color-lighter);
}

.preview-box :deep(.el-descriptions__label.el-descriptions__cell.is-bordered-label) {
  width: 136px;
  min-width: 136px;
  white-space: nowrap;
}

.preview-group {
  margin-bottom: 12px;
}

.preview-title {
  font-size: 13px;
  font-weight: 500;
  margin-bottom: 8px;
}

.wizard-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

@media (max-width: 768px) {
  .subnet-wizard-dialog :deep(.el-dialog) {
    width: 96vw !important;
  }

  .subnet-wizard-dialog :deep(.el-dialog__header),
  .subnet-wizard-dialog :deep(.el-dialog__body),
  .subnet-wizard-dialog :deep(.el-dialog__footer) {
    padding-left: 16px;
    padding-right: 16px;
  }

  .wizard-steps {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .field-row {
    grid-template-columns: 1fr;
  }

  .field-wrap :deep(.el-input),
  .field-wrap :deep(.el-select),
  .field-wrap :deep(.el-input-number),
  .field-wrap :deep(.el-textarea),
  .cidr-hint-bar {
    width: 100%;
  }
}
</style>
