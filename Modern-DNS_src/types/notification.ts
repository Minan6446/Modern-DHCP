// Wire-level channel codes. Must match the backend `pkg/notify` Channel
// constants verbatim — the value is sent in URL paths
// (PUT /setting/notice/:channel) and persisted to operation_logs.
export type NotificationChannel =
  | 'email'
  | 'webhook'
  | 'sms'
  | 'dingtalk'
  | 'feishu'
  | 'wecom'
  | 'slack'
  | 'voice'

// All channel codes in display order. Importable so tab strips and
// permission lists stay in sync without re-typing.
export const NOTIFICATION_CHANNELS: NotificationChannel[] = [
  'email',
  'webhook',
  'sms',
  'dingtalk',
  'feishu',
  'wecom',
  'slack',
  'voice',
]

export interface NotificationEmailConfig {
  enabled: boolean
  smtpHost: string
  smtpPort: number
  sender: string
  authCode: string
  receivers: string
}

export interface NotificationWebhookConfig {
  enabled: boolean
  url: string
  method: 'GET' | 'POST'
  secret: string
  alertTypes: string[]
}

// Aliyun SMS-specific shape. The backend `pkg/notify` SMS sender signs
// against Aliyun's POP API directly, so the keys here mirror what their
// SendSms request expects.
export interface NotificationSmsConfig {
  enabled: boolean
  accessKeyId: string
  accessKeySecret: string
  signName: string
  templateCode: string
  regionId: string
  phones: string
}

// Shared severity floor type. Matches what the backend's level filter
// understands; `''` means "no floor" so empty selects pass through.
export type NotificationMinLevel = '' | 'info' | 'warning' | 'critical'

// Common knobs shared by every chat-bot channel. Splitting them out
// makes the per-channel interfaces declarative.
export interface ChatBotBase {
  enabled: boolean
  url: string
  secret: string
  alertTypes: string[]
  minLevel: NotificationMinLevel
  allowPrivateNetwork: boolean
}

// DingTalk custom-bot. URL accepts the access_token query param; secret
// activates DingTalk's HMAC "加签" mode (the bot URL must also be
// configured to require signed requests).
export interface NotificationDingTalkConfig extends ChatBotBase {
  atMobiles: string[]
  atUserIds: string[]
  atAll: boolean
}

// Feishu (Lark) custom-bot. `useCard` opts into the interactive card
// renderer (colored header + structured fields).
export interface NotificationFeishuConfig extends ChatBotBase {
  useCard: boolean
  atUsers: string[]
  atAll: boolean
}

// WeCom (企业微信) custom-bot. The URL key serves as the credential;
// no separate secret is required, but the field exists for parity with
// the other tabs.
export interface NotificationWecomConfig extends ChatBotBase {
  atMobiles: string[]
  atUsers: string[]
}

// Slack incoming webhook. `mentions` is a free-form list of @-tokens
// (e.g. "<!channel>", "<@U123>") that operators paste verbatim.
export interface NotificationSlackConfig extends ChatBotBase {
  channel: string
  username: string
  iconEmoji: string
  mentions: string[]
}

// Aliyun voice (TTS outbound call). Independent of SMS because Aliyun
// bills voice per-call and rate-limits to one phone per request.
export interface NotificationVoiceConfig {
  enabled: boolean
  accessKeyId: string
  accessKeySecret: string
  ttsCode: string
  calledShowNumber: string
  regionId: string
  phones: string
  ttsParam: string
  playTimes: number
  volume: number
  maxCalls: number
  alertTypes: string[]
  minLevel: NotificationMinLevel
}

export interface NotificationConfig {
  email: NotificationEmailConfig
  webhook: NotificationWebhookConfig
  sms: NotificationSmsConfig
  dingtalk: NotificationDingTalkConfig
  feishu: NotificationFeishuConfig
  wecom: NotificationWecomConfig
  slack: NotificationSlackConfig
  voice: NotificationVoiceConfig
}

export interface NotificationChannelPayloadMap {
  email: NotificationEmailConfig
  webhook: NotificationWebhookConfig
  sms: NotificationSmsConfig
  dingtalk: NotificationDingTalkConfig
  feishu: NotificationFeishuConfig
  wecom: NotificationWecomConfig
  slack: NotificationSlackConfig
  voice: NotificationVoiceConfig
}

export interface NotificationTabExpose {
  validate: () => Promise<boolean>
  clearValidate: () => void
}

export interface NotificationTestResult {
  success: boolean
  channel: NotificationChannel
  message: string
}

// One row per channel. The backend always returns all 8 channels —
// channels without an operator override echo the package default in
// both `subject`/`body` and `defaultSubject`/`defaultBody`, with
// `isCustom=false` so the UI can show a "currently using default" hint
// instead of an "edited" badge.
export interface NoticeTemplate {
  channel: NotificationChannel
  subject: string
  body: string
  defaultSubject: string
  defaultBody: string
  isCustom: boolean
  updatedAt?: string
}

export interface NoticeTemplatePreview {
  channel: NotificationChannel
  subject: string
  body: string
}

export const notificationAlertTypeOptions = ['DDoS防护', '解析失败', 'DNSSEC异常', '配置变更'] as const

// Common defaults shared by chat-bot channels. Inlined back into each
// factory below so adding a new field touches only the relevant
// channel default rather than this aggregate.
const defaultChatBotBase = (): ChatBotBase => ({
  enabled: false,
  url: '',
  secret: '',
  alertTypes: [],
  minLevel: '',
  allowPrivateNetwork: false,
})

export const createDefaultNotificationConfig = (): NotificationConfig => ({
  email: {
    enabled: false,
    smtpHost: '',
    smtpPort: 465,
    sender: '',
    authCode: '',
    receivers: '',
  },
  webhook: {
    enabled: false,
    url: '',
    method: 'POST',
    secret: '',
    alertTypes: [],
  },
  sms: {
    enabled: false,
    accessKeyId: '',
    accessKeySecret: '',
    signName: '',
    templateCode: '',
    regionId: 'cn-hangzhou',
    phones: '',
  },
  dingtalk: {
    ...defaultChatBotBase(),
    atMobiles: [],
    atUserIds: [],
    atAll: false,
  },
  feishu: {
    ...defaultChatBotBase(),
    useCard: true,
    atUsers: [],
    atAll: false,
  },
  wecom: {
    ...defaultChatBotBase(),
    atMobiles: [],
    atUsers: [],
  },
  slack: {
    ...defaultChatBotBase(),
    channel: '',
    username: '',
    iconEmoji: '',
    mentions: [],
  },
  voice: {
    enabled: false,
    accessKeyId: '',
    accessKeySecret: '',
    ttsCode: '',
    calledShowNumber: '',
    regionId: 'cn-hangzhou',
    phones: '',
    ttsParam: '',
    playTimes: 1,
    volume: 100,
    maxCalls: 5,
    alertTypes: [],
    minLevel: '',
  },
})