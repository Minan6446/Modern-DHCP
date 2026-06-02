import request from './request'
import type { ApiResponse } from '../types/api'
import type {
  NotificationChannel,
  NotificationChannelPayloadMap,
  NotificationConfig,
  NotificationTestResult,
  NoticeTemplate,
  NoticeTemplatePreview,
} from '../types/notification'
import { createDefaultNotificationConfig } from '../types/notification'

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value))

let notificationCache: NotificationConfig | null = null

// mergeRaw spreads the wire-level payload over the default skeleton so
// missing fields (e.g. a backend that hasn't yet been migrated past an
// earlier schema, or a channel the operator has never opened) keep
// their default values rather than rendering as `undefined`. Each
// channel is merged individually rather than via Object.assign so the
// type information per channel is preserved.
const mergeRaw = (raw: Record<string, any>): NotificationConfig => {
  const defaults = createDefaultNotificationConfig()
  return {
    email: { ...defaults.email, ...(raw.email ?? {}) },
    webhook: {
      ...defaults.webhook,
      ...(raw.webhook ?? {}),
      method: raw.webhook?.method === 'GET' ? 'GET' : 'POST',
    },
    sms: { ...defaults.sms, ...(raw.sms ?? {}) },
    dingtalk: { ...defaults.dingtalk, ...(raw.dingtalk ?? {}) },
    feishu: { ...defaults.feishu, ...(raw.feishu ?? {}) },
    wecom: { ...defaults.wecom, ...(raw.wecom ?? {}) },
    slack: { ...defaults.slack, ...(raw.slack ?? {}) },
    voice: { ...defaults.voice, ...(raw.voice ?? {}) },
  }
}

// Operator-facing channel labels for the toast fallback in
// testNotificationChannelApi. Keep in sync with the backend's
// channelDisplayName helper so the audit log and the toast read the
// same name.
const channelLabelMap: Record<NotificationChannel, string> = {
  email: '邮件',
  webhook: 'Webhook',
  sms: '短信',
  dingtalk: '钉钉',
  feishu: '飞书',
  wecom: '企业微信',
  slack: 'Slack',
  voice: '电话',
}

// All three helpers below talk to the backend through `request`, whose
// response interceptor returns the *full* envelope `{code, message, data}`
// (see api/request.ts). Earlier the helpers treated the envelope as the
// plain `data` object, so `mergeRaw(envelope)` always saw `envelope.email`
// as undefined and silently fell back to the defaults — making save look
// successful in the toast while the form reset to placeholders on reload.
// We unwrap the `data` field explicitly here to keep that bug from
// re-appearing if a teammate copies these helpers as a template.
const unwrapEnvelope = (raw: unknown): Record<string, any> => {
  const env = raw as { data?: unknown } | null | undefined
  const inner = env && typeof env === 'object' && 'data' in env ? env.data : env
  return (inner as Record<string, any>) ?? {}
}

export const getNotificationConfigApi = async (): Promise<ApiResponse<NotificationConfig>> => {
  const raw = await request.get('/setting/notice')
  const config = mergeRaw(unwrapEnvelope(raw))
  notificationCache = config
  return { code: 0, message: 'ok', data: clone(config) }
}

export const updateNotificationChannelApi = async <T extends NotificationChannel>(
  channel: T,
  payload: NotificationChannelPayloadMap[T],
): Promise<ApiResponse<NotificationConfig>> => {
  const raw = await request.put(`/setting/notice/${channel}`, payload)
  const config = mergeRaw(unwrapEnvelope(raw))
  notificationCache = config
  return { code: 0, message: 'ok', data: clone(config) }
}

export const testNotificationChannelApi = async <T extends NotificationChannel>(
  channel: T,
  payload: NotificationChannelPayloadMap[T],
): Promise<ApiResponse<NotificationTestResult>> => {
  const raw = await request.post('/setting/notice/test', { channel, ...payload })
  const data = unwrapEnvelope(raw)
  const backendMessage = data?.message
  return {
    code: 0,
    message: 'ok',
    data: {
      success: Boolean(data?.success),
      channel,
      message:
        typeof backendMessage === 'string' && backendMessage.trim()
          ? backendMessage
          : `${channelLabelMap[channel]}测试已发送`,
    },
  }
}

// ─── Notice templates ────────────────────────────────────────────────────────
//
// Each helper unwraps the `data` envelope and tolerates the legacy
// shape where the backend returned the data field directly, mirroring
// what the channel-config helpers above do. The backend's list
// endpoint always returns the full set of channels (custom + default
// echoed back), so the caller receives a stable 8-element array even
// on a fresh deployment.
export const listNoticeTemplatesApi = async (): Promise<ApiResponse<NoticeTemplate[]>> => {
  const raw = await request.get('/setting/notice/templates')
  const env = unwrapEnvelope(raw)
  const items = Array.isArray(env?.items) ? (env.items as NoticeTemplate[]) : []
  return { code: 0, message: 'ok', data: items }
}

export const updateNoticeTemplateApi = async (
  channel: NotificationChannel,
  payload: { subject: string; body: string },
): Promise<ApiResponse<{ channel: NotificationChannel }>> => {
  const raw = await request.put(`/setting/notice/templates/${channel}`, payload)
  const env = unwrapEnvelope(raw)
  return { code: 0, message: 'ok', data: { channel: (env?.channel as NotificationChannel) ?? channel } }
}

export const resetNoticeTemplateApi = async (
  channel: NotificationChannel,
): Promise<ApiResponse<{ channel: NotificationChannel }>> => {
  const raw = await request.post(`/setting/notice/templates/${channel}/reset`)
  const env = unwrapEnvelope(raw)
  return { code: 0, message: 'ok', data: { channel: (env?.channel as NotificationChannel) ?? channel } }
}

export const previewNoticeTemplateApi = async (
  channel: NotificationChannel,
  payload: { subject: string; body: string },
): Promise<ApiResponse<NoticeTemplatePreview>> => {
  const raw = await request.post(`/setting/notice/templates/${channel}/preview`, payload)
  const env = unwrapEnvelope(raw)
  return {
    code: 0,
    message: 'ok',
    data: {
      channel: (env?.channel as NotificationChannel) ?? channel,
      subject: typeof env?.subject === 'string' ? env.subject : '',
      body: typeof env?.body === 'string' ? env.body : '',
    },
  }
}