<template>
  <div class="page-wrap">
    <el-card shadow="never" class="surface-card config-shell" v-loading="pageLoading">
      <div class="title-block">
        <h3>{{ t('system.smtp.pageTitle') }}</h3>
        <p class="desc">{{ t('system.smtp.pageDesc') }}</p>
      </div>

      <el-tabs v-model="activeTab" class="config-tabs" :before-leave="handleBeforeLeave">
        <el-tab-pane :label="t('system.smtp.tabEmail')" name="email">
          <div class="tab-body">
            <div class="status-row">
              <span class="tab-summary">{{ t('system.smtp.emailSummary') }}</span>
              <el-tag :type="emailStatus.type" size="small">{{ emailStatus.text }}</el-tag>
            </div>
            <el-form
              ref="formRef"
              :model="form"
              :rules="rules"
              label-position="left"
              label-width="120px"
              require-asterisk-position="left"
              class="config-form"
              :disabled="loading || saving || testing"
            >
              <el-form-item :label="t('system.smtp.host')" prop="host" required>
                <el-input v-model="form.host" class="field-control" placeholder="smtp.example.com" clearable />
              </el-form-item>
              <el-form-item :label="t('system.smtp.port')" prop="port" required>
                <el-input-number v-model="form.port" class="field-control" :min="1" :max="65535" :step="1" controls-position="right" />
              </el-form-item>
              <el-form-item :label="t('system.smtp.username')" prop="username" required>
                <el-input v-model="form.username" class="field-control" placeholder="smtp-user" clearable />
              </el-form-item>
              <el-form-item :label="t('system.smtp.password')" prop="password" required>
                <el-input v-model="form.password" class="field-control" type="password" show-password placeholder="••••••" />
              </el-form-item>
              <el-form-item :label="t('system.smtp.from')" prop="from" required>
                <el-input v-model="form.from" class="field-control" placeholder="noreply@example.com" clearable />
              </el-form-item>
              <el-form-item :label="t('system.smtp.tls')" prop="useTLS">
                <div class="inline-field">
                  <el-switch v-model="form.useTLS" />
                  <span class="hint">{{ t('system.smtp.tlsHint') }}</span>
                </div>
              </el-form-item>
              <el-form-item :label="t('system.smtp.testRecipient')" prop="testRecipient" required>
                <el-input v-model="form.testRecipient" class="field-control" placeholder="test@example.com" clearable />
              </el-form-item>
            </el-form>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('system.smtp.tabSms')" name="sms">
          <div class="tab-body">
            <div class="status-row">
              <span class="tab-summary">{{ t('system.smtp.smsSummary') }}</span>
              <el-tag :type="smsStatus.type" size="small">{{ smsStatus.text }}</el-tag>
            </div>
            <el-form
              ref="smsFormRef"
              :model="smsForm"
              :rules="smsRules"
              label-position="left"
              label-width="120px"
              require-asterisk-position="left"
              class="config-form"
              :disabled="smsLoading || smsSaving || smsTesting"
            >
              <el-form-item :label="t('system.smtp.smsProvider')" prop="provider" required>
                <el-select v-model="smsForm.provider" class="field-control" :placeholder="t('system.smtp.smsProviderPh')" filterable>
                  <el-option :label="t('system.smtp.smsAliyun')" value="aliyun" />
                  <el-option :label="t('system.smtp.smsTencent')" value="tencent" />
                  <el-option :label="t('system.smtp.smsHuawei')" value="huawei" />
                  <el-option label="AWS SNS" value="aws-sns" />
                  <el-option label="Twilio" value="twilio" />
                  <el-option :label="t('system.smtp.smsVolcengine')" value="volcengine" />
                </el-select>
              </el-form-item>
              <el-form-item label="Access Key" prop="accessKeyId" required>
                <el-input v-model="smsForm.accessKeyId" class="field-control" type="password" show-password placeholder="AK / API Key" />
              </el-form-item>
              <el-form-item label="Access Secret" prop="accessKeySecret" required>
                <el-input v-model="smsForm.accessKeySecret" class="field-control" type="password" show-password placeholder="Secret / API Secret" />
              </el-form-item>
              <el-form-item :label="t('system.smtp.smsSignName')" prop="signName" required>
                <el-input v-model="smsForm.signName" class="field-control" :placeholder="t('system.smtp.smsSignPh')" clearable />
              </el-form-item>
              <el-form-item :label="t('system.smtp.smsTemplateCode')" prop="templateCode" required>
                <el-input v-model="smsForm.templateCode" class="field-control" placeholder="SMS_123456" clearable />
              </el-form-item>
              <el-form-item :label="t('system.smtp.smsTestPhone')" prop="testPhone" required>
                <el-input v-model="smsForm.testPhone" class="field-control" placeholder="+8613800000000" clearable />
              </el-form-item>
            </el-form>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('system.smtp.tabWebhook')" name="webhook">
          <div class="tab-body">
            <div class="status-row">
              <span class="tab-summary">{{ t('system.smtp.webhookSummary') }}</span>
              <el-tag :type="webhookStatus.type" size="small">{{ webhookStatus.text }}</el-tag>
            </div>
            <el-form
              ref="webhookFormRef"
              :model="webhookForm"
              :rules="webhookRules"
              label-position="left"
              label-width="120px"
              require-asterisk-position="left"
              class="config-form"
              :disabled="webhookLoading || webhookSaving || webhookTesting"
            >
              <el-form-item label="Webhook URL" prop="url" required>
                <el-input v-model="webhookForm.url" class="field-control" placeholder="https://example.com/hooks/alerts" clearable />
              </el-form-item>
              <el-form-item :label="t('system.smtp.webhookMethod')" prop="method" required>
                <div class="method-field">
                  <el-select v-model="webhookForm.method" class="field-control" :placeholder="t('system.smtp.webhookMethodPh')">
                    <el-option label="POST" value="POST" />
                    <el-option label="PUT" value="PUT" />
                    <el-option label="PATCH" value="PATCH" />
                    <el-option label="DELETE" value="DELETE" />
                  </el-select>
                  <div v-if="!webhookForm.method" class="required-tip">{{ t('system.smtp.webhookMethodRequired') }}</div>
                </div>
              </el-form-item>
              <el-form-item :label="t('system.smtp.webhookToken')" prop="token">
                <el-input v-model="webhookForm.token" class="field-control" type="password" show-password :placeholder="t('system.smtp.webhookTokenPh')" />
              </el-form-item>
              <el-form-item :label="t('system.smtp.webhookHeaderKey')" prop="headerKey">
                <el-input v-model="webhookForm.headerKey" class="field-control" placeholder="X-Auth-Token" clearable />
              </el-form-item>
              <el-form-item :label="t('system.smtp.webhookPayload')" prop="payload">
                <el-input
                  v-model="webhookForm.payload"
                  class="field-control"
                  type="textarea"
                  :rows="4"
                  placeholder='{"alert":"DHCP alert","severity":"critical"}'
                />
              </el-form-item>
            </el-form>

            <el-alert v-if="webhookTestDetail" class="detail-alert" type="info" :closable="false" :title="t('system.smtp.webhookTestTitle')">
              <div class="detail-time">{{ webhookTestDetail.at }}</div>
              <div class="detail-grid">
                <div>
                  <div class="detail-title">{{ t('system.smtp.webhookTestRequest') }}</div>
                  <pre class="detail-pre">{{ webhookTestDetail.request }}</pre>
                </div>
                <div>
                  <div class="detail-title">{{ t('system.smtp.webhookTestResponse') }}</div>
                  <pre class="detail-pre">{{ webhookTestDetail.response }}</pre>
                </div>
              </div>
            </el-alert>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('system.smtp.channelDingTalk')" name="dingtalk">
          <div class="tab-body">
            <div class="status-row">
              <span class="tab-summary">{{ t('system.smtp.channelDingTalkSummary') }}</span>
              <el-tag :type="channelStatus('dingtalk').type" size="small">{{ channelStatus('dingtalk').text }}</el-tag>
            </div>
            <el-alert class="channel-tip" type="info" :closable="false" :title="t('system.smtp.channelAutoDetectTip')" />
            <el-form
              :model="webhookForm"
              label-position="left"
              label-width="120px"
              class="config-form"
              :disabled="webhookLoading || webhookSaving"
            >
              <el-form-item :label="t('system.smtp.channelDingTalkUrl')" prop="dingTalkUrl">
                <el-input v-model="webhookForm.dingTalkUrl" class="field-control" :placeholder="t('system.smtp.channelDingTalkUrlPh')" clearable />
              </el-form-item>
            </el-form>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('system.smtp.channelFeishu')" name="feishu">
          <div class="tab-body">
            <div class="status-row">
              <span class="tab-summary">{{ t('system.smtp.channelFeishuSummary') }}</span>
              <el-tag :type="channelStatus('feishu').type" size="small">{{ channelStatus('feishu').text }}</el-tag>
            </div>
            <el-alert class="channel-tip" type="info" :closable="false" :title="t('system.smtp.channelAutoDetectTip')" />
            <el-form
              :model="webhookForm"
              label-position="left"
              label-width="120px"
              class="config-form"
              :disabled="webhookLoading || webhookSaving"
            >
              <el-form-item :label="t('system.smtp.channelFeishuUrl')" prop="feishuUrl">
                <el-input v-model="webhookForm.feishuUrl" class="field-control" :placeholder="t('system.smtp.channelFeishuUrlPh')" clearable />
              </el-form-item>
            </el-form>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('system.smtp.channelWecom')" name="wecom">
          <div class="tab-body">
            <div class="status-row">
              <span class="tab-summary">{{ t('system.smtp.channelWecomSummary') }}</span>
              <el-tag :type="channelStatus('wecom').type" size="small">{{ channelStatus('wecom').text }}</el-tag>
            </div>
            <el-alert class="channel-tip" type="info" :closable="false" :title="t('system.smtp.channelAutoDetectTip')" />
            <el-form
              :model="webhookForm"
              label-position="left"
              label-width="120px"
              class="config-form"
              :disabled="webhookLoading || webhookSaving"
            >
              <el-form-item :label="t('system.smtp.channelWecomUrl')" prop="wecomUrl">
                <el-input v-model="webhookForm.wecomUrl" class="field-control" :placeholder="t('system.smtp.channelWecomUrlPh')" clearable />
              </el-form-item>
            </el-form>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('system.smtp.channelSlack')" name="slack">
          <div class="tab-body">
            <div class="status-row">
              <span class="tab-summary">{{ t('system.smtp.channelSlackSummary') }}</span>
              <el-tag :type="channelStatus('slack').type" size="small">{{ channelStatus('slack').text }}</el-tag>
            </div>
            <el-alert class="channel-tip" type="info" :closable="false" :title="t('system.smtp.channelAutoDetectTip')" />
            <el-form
              :model="webhookForm"
              label-position="left"
              label-width="120px"
              class="config-form"
              :disabled="webhookLoading || webhookSaving"
            >
              <el-form-item :label="t('system.smtp.channelSlackUrl')" prop="slackUrl">
                <el-input v-model="webhookForm.slackUrl" class="field-control" :placeholder="t('system.smtp.channelSlackUrlPh')" clearable />
              </el-form-item>
            </el-form>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('system.smtp.channelPhone')" name="phone">
          <div class="tab-body">
            <div class="status-row">
              <span class="tab-summary">{{ t('system.smtp.channelPhoneSummary') }}</span>
              <el-tag :type="channelStatus('phone').type" size="small">{{ channelStatus('phone').text }}</el-tag>
            </div>
            <el-alert class="channel-tip" type="info" :closable="false" :title="t('system.smtp.channelAutoDetectTip')" />
            <el-form
              :model="webhookForm"
              label-position="left"
              label-width="120px"
              class="config-form"
              :disabled="webhookLoading || webhookSaving"
            >
              <el-form-item :label="t('system.smtp.channelPhones')" prop="phoneNumbers">
                <el-input v-model="webhookForm.phoneNumbers" class="field-control" :placeholder="t('system.smtp.channelPhonesPh')" clearable />
              </el-form-item>
            </el-form>
          </div>
        </el-tab-pane>
      </el-tabs>

      <div class="action-bar">
        <el-button :loading="activeTesting" :disabled="activeLoading" @click="handleTest">{{ t('system.smtp.btnTest') }}</el-button>
        <el-button :disabled="activeLoading" @click="handleReset">{{ t('system.smtp.btnReset') }}</el-button>
        <el-button type="primary" :loading="activeSaving" :disabled="activeLoading" @click="handleSave">{{ t('system.smtp.btnSave') }}</el-button>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { ElMessageBox } from 'element-plus';
import { useI18n } from 'vue-i18n';
import type { FormInstance, FormRules } from 'element-plus';
import {
  getSmtpConfig,
  saveSmtpConfig,
  sendSmtpTest,
  getSmsConfig,
  saveSmsConfig,
  sendSmsTest,
  getWebhookConfig,
  saveWebhookConfig,
  sendWebhookTest
} from '@/api/system/smtp';
import type { SmsConfig, SmtpConfig, WebhookConfig } from '@/api/system/smtp';
import { showHttpError } from '@/shared/errors/errorToast';
import { showSuccess, showWarning } from '@/shared/errors/messageToast';

type ChannelTab = 'dingtalk' | 'feishu' | 'wecom' | 'slack' | 'phone';
type ConfigTab = 'email' | 'sms' | 'webhook' | ChannelTab;

const { t } = useI18n();
const activeTab = ref<ConfigTab>('email');
const pageLoading = ref(false);

const formRef = ref<FormInstance>();
const loading = ref(false);
const saving = ref(false);
const testing = ref(false);
const form = reactive<SmtpConfig>({
  host: '',
  port: 587,
  username: '',
  password: '',
  from: '',
  useTLS: true,
  testRecipient: ''
});
const emailSnapshot = ref<SmtpConfig>({ ...form });

const smsFormRef = ref<FormInstance>();
const smsLoading = ref(false);
const smsSaving = ref(false);
const smsTesting = ref(false);
const smsForm = reactive<SmsConfig>({
  provider: '',
  accessKeyId: '',
  accessKeySecret: '',
  signName: '',
  templateCode: '',
  testPhone: ''
});
const smsSnapshot = ref<SmsConfig>({ ...smsForm });

const webhookFormRef = ref<FormInstance>();
const webhookLoading = ref(false);
const webhookSaving = ref(false);
const webhookTesting = ref(false);
const webhookForm = reactive<WebhookConfig>({
  url: '',
  method: 'POST',
  token: '',
  headerKey: 'X-Auth-Token',
  payload: '{"alert":"DHCP alert","severity":"critical"}',
  channels: [],
  dingTalkUrl: '',
  feishuUrl: '',
  wecomUrl: '',
  slackUrl: '',
  phoneNumbers: ''
});
const webhookSnapshot = ref<WebhookConfig>({ ...webhookForm });
const webhookTestDetail = ref<{ request: string; response: string; at: string } | null>(null);

const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const phoneRegex = /^\+?\d{7,15}$/;
const normalizeWebhookChannels = (channels?: string[]) => {
  const allowed = new Set(['dingtalk', 'feishu', 'wecom', 'slack', 'phone']);
  const seen = new Set<string>();
  const result: string[] = [];
  for (const channel of channels || []) {
    const item = String(channel || '').trim().toLowerCase();
    if (!allowed.has(item) || seen.has(item)) continue;
    seen.add(item);
    result.push(item);
  }
  return result;
};
const channelTabs: ChannelTab[] = ['dingtalk', 'feishu', 'wecom', 'slack', 'phone'];
const isChannelTab = (tab: string): tab is ChannelTab => channelTabs.includes(tab as ChannelTab);
const parsePhoneNumbers = (value?: string) =>
  String(value || '')
    .split(/[\n,，;]/)
    .map((item) => item.trim())
    .filter(Boolean);
const validateChannelValue = (channel: ChannelTab) => {
  if (channel === 'phone') {
    const numbers = parsePhoneNumbers(webhookForm.phoneNumbers);
    if (!numbers.length) return false;
    return numbers.every((item) => phoneRegex.test(item));
  }
  const value =
    channel === 'dingtalk'
      ? webhookForm.dingTalkUrl
      : channel === 'feishu'
        ? webhookForm.feishuUrl
        : channel === 'wecom'
          ? webhookForm.wecomUrl
          : webhookForm.slackUrl;
  const raw = String(value || '').trim();
  if (!raw) return false;
  try {
    const u = new URL(raw);
    return u.protocol === 'http:' || u.protocol === 'https:';
  } catch {
    return false;
  }
};
const hasChannelValue = (channel: ChannelTab) => {
  if (channel === 'phone') return parsePhoneNumbers(webhookForm.phoneNumbers).length > 0;
  return Boolean(
    String(
      channel === 'dingtalk'
        ? webhookForm.dingTalkUrl
        : channel === 'feishu'
          ? webhookForm.feishuUrl
          : channel === 'wecom'
            ? webhookForm.wecomUrl
            : webhookForm.slackUrl
    ).trim()
  );
};
const channelStatus = (channel: ChannelTab) => {
  const currentValue =
    channel === 'dingtalk'
      ? String(webhookForm.dingTalkUrl || '').trim()
      : channel === 'feishu'
        ? String(webhookForm.feishuUrl || '').trim()
        : channel === 'wecom'
          ? String(webhookForm.wecomUrl || '').trim()
          : channel === 'slack'
            ? String(webhookForm.slackUrl || '').trim()
            : parsePhoneNumbers(webhookForm.phoneNumbers).join(',');
  const snapshotValue =
    channel === 'dingtalk'
      ? String(webhookSnapshot.value.dingTalkUrl || '').trim()
      : channel === 'feishu'
        ? String(webhookSnapshot.value.feishuUrl || '').trim()
        : channel === 'wecom'
          ? String(webhookSnapshot.value.wecomUrl || '').trim()
          : channel === 'slack'
            ? String(webhookSnapshot.value.slackUrl || '').trim()
            : parsePhoneNumbers(webhookSnapshot.value.phoneNumbers).join(',');
  return makeStatus(
    hasChannelValue(channel) && validateChannelValue(channel),
    currentValue !== snapshotValue,
    webhookLoading.value || webhookSaving.value || webhookTesting.value
  );
};
const deriveChannelsFromForm = (raw?: Partial<WebhookConfig>) => {
  const source = raw || webhookForm;
  const merged = normalizeWebhookChannels(source.channels);
  const add = (name: ChannelTab, value?: string) => {
    if (String(value || '').trim() && !merged.includes(name)) merged.push(name);
  };
  add('dingtalk', source.dingTalkUrl);
  add('feishu', source.feishuUrl);
  add('wecom', source.wecomUrl);
  add('slack', source.slackUrl);
  add('phone', source.phoneNumbers);
  return merged;
};
const hasAnyWebhookTarget = () => {
  const channels = deriveChannelsFromForm();
  if (String(webhookForm.url || '').trim() !== '') return true;
  if (channels.includes('dingtalk') && String(webhookForm.dingTalkUrl || '').trim() !== '') return true;
  if (channels.includes('feishu') && String(webhookForm.feishuUrl || '').trim() !== '') return true;
  if (channels.includes('wecom') && String(webhookForm.wecomUrl || '').trim() !== '') return true;
  if (channels.includes('slack') && String(webhookForm.slackUrl || '').trim() !== '') return true;
  if (channels.includes('phone') && String(webhookForm.phoneNumbers || '').trim() !== '') return true;
  return false;
};
const emailValidator = (_rule: any, value: string, callback: (err?: Error) => void) => {
  const raw = String(value || '').trim();
  if (!raw) {
    callback(new Error(t('system.smtp.valEmailRecipient')));
    return;
  }
  if (!emailRegex.test(raw)) {
    callback(new Error(t('system.smtp.valEmailFormat')));
    return;
  }
  callback();
};
const phoneValidator = (_rule: any, value: string, callback: (err?: Error) => void) => {
  const raw = String(value || '').trim();
  if (!raw) {
    callback(new Error(t('system.smtp.valPhone')));
    return;
  }
  if (!phoneRegex.test(raw)) {
    callback(new Error(t('system.smtp.valPhoneFormat')));
    return;
  }
  callback();
};
const urlValidator = (_rule: any, value: string, callback: (err?: Error) => void) => {
  const raw = String(value || '').trim();
  if (!raw) {
    if (hasAnyWebhookTarget()) {
      callback();
      return;
    }
    callback(new Error(t('system.smtp.valWebhookTarget')));
    return;
  }
  try {
    const u = new URL(raw);
    if (u.protocol !== 'http:' && u.protocol !== 'https:') {
      callback(new Error(t('system.smtp.valUrlProtocol')));
      return;
    }
    callback();
  } catch {
    callback(new Error(t('system.smtp.valUrlFormat')));
  }
};
const payloadValidator = (_rule: any, value: string, callback: (err?: Error) => void) => {
  const raw = String(value || '').trim();
  if (!raw) {
    callback();
    return;
  }
  try {
    JSON.parse(raw);
    callback();
  } catch {
    callback(new Error(t('system.smtp.valPayloadJson')));
  }
};

const rules: FormRules = {
  host: [{ required: true, message: t('system.smtp.valHost'), trigger: ['blur', 'change'] }],
  port: [{ required: true, message: t('system.smtp.valPort'), trigger: ['change', 'blur'] }],
  username: [{ required: true, message: t('system.smtp.valUsername'), trigger: ['blur', 'change'] }],
  password: [{ required: true, message: t('system.smtp.valPassword'), trigger: ['blur', 'change'] }],
  from: [{ required: true, message: t('system.smtp.valFrom'), trigger: ['blur', 'change'] }],
  testRecipient: [{ validator: emailValidator, trigger: ['blur', 'change'] }]
};

const smsRules: FormRules = {
  provider: [{ required: true, message: t('system.smtp.valProvider'), trigger: ['change', 'blur'] }],
  accessKeyId: [{ required: true, message: t('system.smtp.valAccessKey'), trigger: ['blur', 'change'] }],
  accessKeySecret: [{ required: true, message: t('system.smtp.valAccessSecret'), trigger: ['blur', 'change'] }],
  signName: [{ required: true, message: t('system.smtp.valSignName'), trigger: ['blur', 'change'] }],
  templateCode: [{ required: true, message: t('system.smtp.valTemplateCode'), trigger: ['blur', 'change'] }],
  testPhone: [{ validator: phoneValidator, trigger: ['blur', 'change'] }]
};

const webhookRules: FormRules = {
  url: [{ validator: urlValidator, trigger: ['blur', 'change'] }],
  method: [{ required: true, message: t('system.smtp.valHttpMethod'), trigger: ['change', 'blur'] }],
  payload: [{ validator: payloadValidator, trigger: ['blur', 'change'] }]
};

const normalizeEmail = (raw?: Partial<SmtpConfig>): SmtpConfig => ({
  host: String(raw?.host || '').trim(),
  port: Number(raw?.port || 587),
  username: String(raw?.username || '').trim(),
  password: String(raw?.password || ''),
  from: String(raw?.from || '').trim(),
  useTLS: Boolean(raw?.useTLS),
  testRecipient: String(raw?.testRecipient || '').trim()
});

const normalizeSms = (raw?: Partial<SmsConfig>): SmsConfig => ({
  provider: String(raw?.provider || '').trim(),
  accessKeyId: String(raw?.accessKeyId || ''),
  accessKeySecret: String(raw?.accessKeySecret || ''),
  signName: String(raw?.signName || '').trim(),
  templateCode: String(raw?.templateCode || '').trim(),
  testPhone: String(raw?.testPhone || '').trim()
});

const normalizeWebhook = (raw?: Partial<WebhookConfig>): WebhookConfig => ({
  url: String(raw?.url || '').trim(),
  method: String(raw?.method || 'POST').trim().toUpperCase(),
  token: String(raw?.token || ''),
  headerKey: String(raw?.headerKey || 'X-Auth-Token').trim(),
  payload: String(raw?.payload || '{"alert":"DHCP alert","severity":"critical"}'),
  channels: deriveChannelsFromForm(raw),
  dingTalkUrl: String(raw?.dingTalkUrl || '').trim(),
  feishuUrl: String(raw?.feishuUrl || '').trim(),
  wecomUrl: String(raw?.wecomUrl || '').trim(),
  slackUrl: String(raw?.slackUrl || '').trim(),
  phoneNumbers: String(raw?.phoneNumbers || '').trim()
});

const serialize = (value: unknown) => JSON.stringify(value);

const isEmailDirty = computed(() => serialize(normalizeEmail(form)) !== serialize(normalizeEmail(emailSnapshot.value)));
const isSmsDirty = computed(() => serialize(normalizeSms(smsForm)) !== serialize(normalizeSms(smsSnapshot.value)));
const isWebhookDirty = computed(
  () => serialize(normalizeWebhook(webhookForm)) !== serialize(normalizeWebhook(webhookSnapshot.value))
);

const emailReady = computed(
  () => !!form.host.trim() && !!form.username.trim() && !!form.password.trim() && !!form.from.trim() && form.port > 0
);
const smsReady = computed(
  () =>
    !!smsForm.provider.trim() &&
    !!smsForm.accessKeyId.trim() &&
    !!smsForm.accessKeySecret.trim() &&
    !!smsForm.signName.trim() &&
    !!smsForm.templateCode.trim()
);
const webhookReady = computed(() => hasAnyWebhookTarget() && !!webhookForm.method.trim());

const makeStatus = (ready: boolean, dirty: boolean, loadingState: boolean) => {
  if (loadingState) return { text: t('system.smtp.statusSyncing'), type: 'info' as const };
  if (!ready) return { text: t('system.smtp.statusIncomplete'), type: 'warning' as const };
  if (dirty) return { text: t('system.smtp.statusUnsaved'), type: 'danger' as const };
  return { text: t('system.smtp.statusConfigured'), type: 'success' as const };
};

const emailStatus = computed(() => makeStatus(emailReady.value, isEmailDirty.value, loading.value || saving.value || testing.value));
const smsStatus = computed(() => makeStatus(smsReady.value, isSmsDirty.value, smsLoading.value || smsSaving.value || smsTesting.value));
const webhookStatus = computed(
  () => makeStatus(webhookReady.value, isWebhookDirty.value, webhookLoading.value || webhookSaving.value || webhookTesting.value)
);

const activeLoading = computed(() => {
  if (activeTab.value === 'email') return loading.value || saving.value || testing.value;
  if (activeTab.value === 'sms') return smsLoading.value || smsSaving.value || smsTesting.value;
  if (isChannelTab(activeTab.value)) return webhookLoading.value || webhookSaving.value;
  return webhookLoading.value || webhookSaving.value || webhookTesting.value;
});
const activeSaving = computed(() =>
  activeTab.value === 'email' ? saving.value : activeTab.value === 'sms' ? smsSaving.value : webhookSaving.value
);
const activeTesting = computed(() =>
  activeTab.value === 'email' ? testing.value : activeTab.value === 'sms' ? smsTesting.value : webhookTesting.value
);

const isDirtyByTab = (tab: ConfigTab) => {
  if (tab === 'email') return isEmailDirty.value;
  if (tab === 'sms') return isSmsDirty.value;
  if (isChannelTab(tab)) return isWebhookDirty.value;
  return isWebhookDirty.value;
};

const handleBeforeLeave = async (nextName: string, oldName: string) => {
  const from = oldName as ConfigTab;
  if (!isDirtyByTab(from)) return true;
  try {
    await ElMessageBox.confirm(t('system.smtp.unsavedConfirm'), t('system.smtp.unsavedTitle'), {
      type: 'warning',
      confirmButtonText: t('system.smtp.unsavedOk'),
      cancelButtonText: t('system.smtp.unsavedCancel')
    });
    return true;
  } catch {
    return false;
  }
};

const loadConfig = async () => {
  loading.value = true;
  try {
    const { data } = await getSmtpConfig();
    const loaded = normalizeEmail(data.data || {});
    Object.assign(form, loaded);
    emailSnapshot.value = { ...loaded };
  } catch (err) {
    showHttpError(err, t('system.smtp.loadEmailFail'));
  } finally {
    loading.value = false;
  }
};

const loadSmsConfig = async () => {
  smsLoading.value = true;
  try {
    const { data } = await getSmsConfig();
    const loaded = normalizeSms(data.data || {});
    Object.assign(smsForm, loaded);
    smsSnapshot.value = { ...loaded };
  } catch (err) {
    showHttpError(err, t('system.smtp.loadSmsFail'));
  } finally {
    smsLoading.value = false;
  }
};

const loadWebhookConfig = async () => {
  webhookLoading.value = true;
  try {
    const { data } = await getWebhookConfig();
    const loaded = normalizeWebhook(data.data || {});
    Object.assign(webhookForm, loaded);
    webhookSnapshot.value = { ...loaded };
  } catch (err) {
    showHttpError(err, t('system.smtp.loadWebhookFail'));
  } finally {
    webhookLoading.value = false;
  }
};

const validateForm = async (formInst: FormInstance | undefined) => {
  if (!formInst) return false;
  try {
    await formInst.validate();
    return true;
  } catch {
    return false;
  }
};

const handleSaveEmail = async () => {
  const valid = await validateForm(formRef.value);
  if (!valid) return;
  saving.value = true;
  try {
    const payload = normalizeEmail(form);
    await saveSmtpConfig(payload);
    emailSnapshot.value = { ...payload };
    showSuccess(t('system.smtp.saveEmailOk'));
  } catch (err) {
    showHttpError(err, t('system.smtp.saveEmailFail'));
  } finally {
    saving.value = false;
  }
};

const handleTestEmail = async () => {
  if (!String(form.testRecipient || '').trim()) {
    showWarning(t('system.smtp.testEmailWarn'));
    return;
  }
  const valid = await validateForm(formRef.value);
  if (!valid) return;
  testing.value = true;
  try {
    await sendSmtpTest({ ...normalizeEmail(form), to: String(form.testRecipient || '').trim() });
    showSuccess(t('system.smtp.testSent'));
  } catch (err) {
    showHttpError(err, t('system.smtp.testEmailFail'));
  } finally {
    testing.value = false;
  }
};

const handleSaveSms = async () => {
  const valid = await validateForm(smsFormRef.value);
  if (!valid) return;
  smsSaving.value = true;
  try {
    const payload = normalizeSms(smsForm);
    await saveSmsConfig(payload);
    smsSnapshot.value = { ...payload };
    showSuccess(t('system.smtp.saveSmsOk'));
  } catch (err) {
    showHttpError(err, t('system.smtp.saveSmsFail'));
  } finally {
    smsSaving.value = false;
  }
};

const handleTestSms = async () => {
  if (!String(smsForm.testPhone || '').trim()) {
    showWarning(t('system.smtp.testSmsWarn'));
    return;
  }
  const valid = await validateForm(smsFormRef.value);
  if (!valid) return;
  smsTesting.value = true;
  try {
    await sendSmsTest();
    showSuccess(t('system.smtp.testSmsOk'));
  } catch (err) {
    showHttpError(err, t('system.smtp.testSmsFail'));
  } finally {
    smsTesting.value = false;
  }
};

const safeJsonString = (text: string) => {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
};

const buildWebhookRequestPreview = () => {
  const headers: Record<string, string> = {};
  if (webhookForm.headerKey?.trim() && webhookForm.token?.trim()) {
    headers[webhookForm.headerKey.trim()] = '******';
  }
  return JSON.stringify(
    {
      url: webhookForm.url,
      channels: deriveChannelsFromForm(),
      dingTalkUrl: webhookForm.dingTalkUrl,
      feishuUrl: webhookForm.feishuUrl,
      wecomUrl: webhookForm.wecomUrl,
      slackUrl: webhookForm.slackUrl,
      phoneNumbers: webhookForm.phoneNumbers,
      method: webhookForm.method,
      headers,
      payload: safeJsonString(webhookForm.payload)
    },
    null,
    2
  );
};

const handleSaveWebhook = async () => {
  const valid = activeTab.value === 'webhook' ? await validateForm(webhookFormRef.value) : true;
  if (!valid) return;
  webhookSaving.value = true;
  try {
    const payload = normalizeWebhook(webhookForm);
    await saveWebhookConfig(payload);
    webhookSnapshot.value = { ...payload };
    showSuccess(t('system.smtp.saveWebhookOk'));
  } catch (err) {
    showHttpError(err, t('system.smtp.saveWebhookFail'));
  } finally {
    webhookSaving.value = false;
  }
};

const handleTestWebhook = async () => {
  const valid = activeTab.value === 'webhook' ? await validateForm(webhookFormRef.value) : true;
  if (!valid) return;
  webhookTesting.value = true;
  try {
    const resp = await sendWebhookTest();
    webhookTestDetail.value = {
      at: t('system.smtp.execTime', { time: new Date().toLocaleString() }),
      request: buildWebhookRequestPreview(),
      response: JSON.stringify((resp as any)?.data ?? { message: t('system.smtp.testReqSent') }, null, 2)
    };
    showSuccess(t('system.smtp.testWebhookOk'));
  } catch (err: any) {
    webhookTestDetail.value = {
      at: t('system.smtp.execTime', { time: new Date().toLocaleString() }),
      request: buildWebhookRequestPreview(),
      response: JSON.stringify(err?.response?.data ?? { message: t('system.smtp.reqFailed') }, null, 2)
    };
    showHttpError(err, t('system.smtp.testWebhookFail'));
  } finally {
    webhookTesting.value = false;
  }
};

const resetEmail = () => {
  Object.assign(form, normalizeEmail(emailSnapshot.value));
  formRef.value?.clearValidate();
};
const resetSms = () => {
  Object.assign(smsForm, normalizeSms(smsSnapshot.value));
  smsFormRef.value?.clearValidate();
};
const resetWebhook = () => {
  Object.assign(webhookForm, normalizeWebhook(webhookSnapshot.value));
  webhookFormRef.value?.clearValidate();
};

const handleReset = async () => {
  try {
    await ElMessageBox.confirm(t('system.smtp.resetConfirm'), t('system.smtp.resetTitle'), {
      type: 'warning',
      confirmButtonText: t('system.smtp.resetOk'),
      cancelButtonText: t('system.smtp.resetCancel')
    });
  } catch {
    return;
  }

  if (activeTab.value === 'email') {
    resetEmail();
  } else if (activeTab.value === 'sms') {
    resetSms();
  } else {
    resetWebhook();
  }
  showSuccess(t('system.smtp.resetDone'));
};

const handleSave = async () => {
  if (activeTab.value === 'email') {
    await handleSaveEmail();
  } else if (activeTab.value === 'sms') {
    await handleSaveSms();
  } else {
    await handleSaveWebhook();
  }
};

const handleTest = async () => {
  if (activeTab.value === 'email') {
    await handleTestEmail();
  } else if (activeTab.value === 'sms') {
    await handleTestSms();
  } else if (isChannelTab(activeTab.value)) {
    if (!hasChannelValue(activeTab.value)) {
      showWarning(t('system.smtp.testChannelWarnEmpty'));
      return;
    }
    if (!validateChannelValue(activeTab.value)) {
      showWarning(activeTab.value === 'phone' ? t('system.smtp.valChannelPhonesFormat') : t('system.smtp.valChannelUrlFormat'));
      return;
    }
    await handleTestWebhook();
  } else {
    await handleTestWebhook();
  }
};

onMounted(async () => {
  pageLoading.value = true;
  try {
    await Promise.all([loadConfig(), loadSmsConfig(), loadWebhookConfig()]);
  } finally {
    pageLoading.value = false;
  }
});
</script>

<style scoped>
.page-wrap {
  padding: 16px;
}

.config-shell {
  border-radius: 8px;
  padding: 16px 16px 0;
}

.title-block {
  margin-bottom: 16px;
}

.title-block h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}

.desc {
  margin: 8px 0 0;
  color: var(--el-text-color-secondary);
}

.config-tabs {
  margin-top: 8px;
}

.tab-body {
  padding: 8px 0 16px;
}

.status-row {
  margin-bottom: 16px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}

.tab-summary {
  color: var(--el-text-color-secondary);
}

.config-form {
  width: 100%;
}

.config-form :deep(.el-form-item) {
  margin-bottom: 16px;
}

.config-form :deep(.el-form-item__label) {
  width: 120px !important;
}

.field-control {
  width: 100%;
  max-width: 400px;
}

.field-control :deep(.el-input__wrapper),
.field-control :deep(.el-textarea__inner),
.field-control :deep(.el-select__wrapper) {
  width: 100%;
}

.inline-field {
  display: flex;
  align-items: center;
  gap: 8px;
}

.hint {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.method-field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.required-tip {
  color: var(--el-color-danger);
  font-size: 12px;
}

.detail-alert {
  margin-top: 16px;
}

.channel-tip {
  margin-bottom: 16px;
}

.detail-time {
  margin-bottom: 8px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.detail-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.detail-title {
  margin-bottom: 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.detail-pre {
  margin: 0;
  padding: 10px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  background: var(--el-fill-color-light);
  max-height: 220px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

.action-bar {
  position: sticky;
  bottom: 0;
  z-index: 8;
  display: flex;
  justify-content: flex-start;
  gap: 8px;
  padding: 16px 0;
  border-top: 1px solid var(--el-border-color-lighter);
  background: var(--el-bg-color);
}
</style>
