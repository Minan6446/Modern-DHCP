<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  listNoticeTemplatesApi,
  previewNoticeTemplateApi,
  resetNoticeTemplateApi,
  updateNoticeTemplateApi,
} from '../../api/notification'
import type { NoticeTemplate, NotificationChannel } from '../../types/notification'
import { NOTIFICATION_CHANNELS } from '../../types/notification'

// NoticeTemplateDrawer is the operator-facing editor for the per-channel
// rendering templates. We intentionally keep the editor *very* simple
// (just two textareas + a variable cheatsheet) because: (1) operators
// edit these maybe once a quarter so investing in a Monaco-grade UX is
// not worth the bundle weight; (2) Go text/template syntax is small
// enough that syntax highlighting doesn't pay for itself; (3) the
// Preview button on every save is the real safety net.
//
// Each channel gets its own `el-tab-pane`. When the operator switches
// tabs we lazily run the channel's draft through `previewNoticeTemplateApi`
// only when they click "预览" — running it on every keystroke would
// hammer the backend with template parses for no real benefit.

interface Props {
  modelValue: boolean
}
const props = defineProps<Props>()
const emit = defineEmits<{ (e: 'update:modelValue', v: boolean): void }>()

const { t } = useI18n()

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit('update:modelValue', v),
})

// Reactive store keyed by channel. Each entry tracks the current draft
// (subject/body), the server-side defaults (so "reset" can be a local
// action when the operator hasn't yet saved), and a few UI flags.
interface Draft {
  subject: string
  body: string
  defaultSubject: string
  defaultBody: string
  isCustom: boolean
  updatedAt?: string
  saving: boolean
  resetting: boolean
  previewing: boolean
  preview: { subject: string; body: string } | null
}
const drafts = reactive<Record<NotificationChannel, Draft>>(
  Object.fromEntries(
    NOTIFICATION_CHANNELS.map((ch) => [
      ch,
      {
        subject: '',
        body: '',
        defaultSubject: '',
        defaultBody: '',
        isCustom: false,
        saving: false,
        resetting: false,
        previewing: false,
        preview: null,
      } satisfies Draft,
    ]),
  ) as Record<NotificationChannel, Draft>,
)

const activeChannel = ref<NotificationChannel>('email')
const loading = ref(false)

// Channel display labels — pulled from the same i18n keys the tabs in
// NotificationSettings.vue use, so a rename in one place flows
// automatically to the other.
const channelTitleMap = computed<Record<NotificationChannel, string>>(() => ({
  email: t('notice.channelEmail'),
  webhook: t('notice.webhook'),
  sms: t('notice.channelSms'),
  dingtalk: t('notice.dingtalk.tabLabel'),
  feishu: t('notice.feishu.tabLabel'),
  wecom: t('notice.wecom.tabLabel'),
  slack: t('notice.slack.tabLabel'),
  voice: t('notice.voice.tabLabel'),
}))

// Variable cheatsheet shown on the right rail. Keep this list in sync
// with backend/pkg/notify/template.go::TemplateData — adding a field
// there should also surface here so operators discover it without
// reading the source.
const variableTokens: { token: string; descKey: string }[] = [
  { token: '{{.Title}}', descKey: 'noticeTemplate.varTitle' },
  { token: '{{.Level}}', descKey: 'noticeTemplate.varLevel' },
  { token: '{{.LevelLabel}}', descKey: 'noticeTemplate.varLevelLabel' },
  { token: '{{.Type}}', descKey: 'noticeTemplate.varType' },
  { token: '{{.Domain}}', descKey: 'noticeTemplate.varDomain' },
  { token: '{{.Message}}', descKey: 'noticeTemplate.varMessage' },
  { token: '{{.Time}}', descKey: 'noticeTemplate.varTime' },
  { token: '{{.Timestamp}}', descKey: 'noticeTemplate.varTimestamp' },
]

// Per-channel hint about what `subject` and `body` actually map to on
// the wire. The renderer uses both fields for some channels (email
// subject vs body) and only one for others (webhook body, sms body).
// Surfacing this hint inline is how we avoid the FAQ "I edited the
// subject but nothing changed" for SMS.
const channelHints: Record<NotificationChannel, { subjectFor: string; bodyFor: string }> = {
  email: { subjectFor: 'noticeTemplate.hint.email.subject', bodyFor: 'noticeTemplate.hint.email.body' },
  webhook: { subjectFor: 'noticeTemplate.hint.webhook.subject', bodyFor: 'noticeTemplate.hint.webhook.body' },
  sms: { subjectFor: 'noticeTemplate.hint.sms.subject', bodyFor: 'noticeTemplate.hint.sms.body' },
  dingtalk: { subjectFor: 'noticeTemplate.hint.chatbot.subject', bodyFor: 'noticeTemplate.hint.chatbot.body' },
  feishu: { subjectFor: 'noticeTemplate.hint.chatbot.subject', bodyFor: 'noticeTemplate.hint.chatbot.body' },
  wecom: { subjectFor: 'noticeTemplate.hint.chatbot.subject', bodyFor: 'noticeTemplate.hint.chatbot.body' },
  slack: { subjectFor: 'noticeTemplate.hint.chatbot.subject', bodyFor: 'noticeTemplate.hint.chatbot.body' },
  voice: { subjectFor: 'noticeTemplate.hint.voice.subject', bodyFor: 'noticeTemplate.hint.voice.body' },
}

const loadAll = async () => {
  loading.value = true
  try {
    const { data } = await listNoticeTemplatesApi()
    for (const item of data as NoticeTemplate[]) {
      const draft = drafts[item.channel]
      if (!draft) continue
      draft.subject = item.subject
      draft.body = item.body
      draft.defaultSubject = item.defaultSubject
      draft.defaultBody = item.defaultBody
      draft.isCustom = item.isCustom
      draft.updatedAt = item.updatedAt
      draft.preview = null
    }
  } catch (e) {
    ElMessage.error(t('noticeTemplate.loadFailed'))
  } finally {
    loading.value = false
  }
}

// Re-fetch whenever the drawer re-opens. We refuse to trust an in-memory
// snapshot from a previous open because another admin in another tab
// may have edited templates in the meantime.
watch(
  () => props.modelValue,
  (open) => {
    if (open) loadAll()
  },
)

const onSave = async (channel: NotificationChannel) => {
  const draft = drafts[channel]
  draft.saving = true
  try {
    await updateNoticeTemplateApi(channel, { subject: draft.subject, body: draft.body })
    draft.isCustom = true
    draft.updatedAt = new Date().toISOString().replace('T', ' ').slice(0, 19)
    ElMessage.success(t('noticeTemplate.saved', { channel: channelTitleMap.value[channel] }))
  } catch (e: any) {
    // Backend returns a parse error message for malformed templates;
    // surface it verbatim so the operator can fix syntax.
    ElMessage.error(e?.message || t('noticeTemplate.saveFailed'))
  } finally {
    draft.saving = false
  }
}

const onReset = async (channel: NotificationChannel) => {
  try {
    await ElMessageBox.confirm(
      t('noticeTemplate.resetConfirm', { channel: channelTitleMap.value[channel] }),
      t('noticeTemplate.resetConfirmTitle'),
      { type: 'warning', confirmButtonText: t('noticeTemplate.resetConfirmOk'), cancelButtonText: t('common.cancel') },
    )
  } catch {
    return
  }
  const draft = drafts[channel]
  draft.resetting = true
  try {
    await resetNoticeTemplateApi(channel)
    draft.subject = draft.defaultSubject
    draft.body = draft.defaultBody
    draft.isCustom = false
    draft.updatedAt = undefined
    draft.preview = null
    ElMessage.success(t('noticeTemplate.resetDone', { channel: channelTitleMap.value[channel] }))
  } catch (e: any) {
    ElMessage.error(e?.message || t('noticeTemplate.resetFailed'))
  } finally {
    draft.resetting = false
  }
}

const onPreview = async (channel: NotificationChannel) => {
  const draft = drafts[channel]
  draft.previewing = true
  try {
    const { data } = await previewNoticeTemplateApi(channel, {
      subject: draft.subject,
      body: draft.body,
    })
    draft.preview = { subject: data.subject, body: data.body }
  } catch (e: any) {
    draft.preview = null
    ElMessage.error(e?.message || t('noticeTemplate.previewFailed'))
  } finally {
    draft.previewing = false
  }
}

// Shortcut to insert a variable token at the body cursor — a small UX
// nicety so operators don't have to type `{{.LevelLabel}}` exactly.
// Falls back to appending when there's no focus context.
const bodyTextareas = ref<Record<NotificationChannel, HTMLTextAreaElement | null>>({} as any)
const insertVariable = (channel: NotificationChannel, token: string) => {
  const el = bodyTextareas.value[channel]
  const draft = drafts[channel]
  if (el && typeof el.selectionStart === 'number') {
    const start = el.selectionStart
    const end = el.selectionEnd ?? start
    draft.body = draft.body.slice(0, start) + token + draft.body.slice(end)
    // Restore caret to *after* the inserted token on next tick so the
    // operator can continue typing without re-focusing.
    requestAnimationFrame(() => {
      el.focus()
      el.selectionStart = el.selectionEnd = start + token.length
    })
  } else {
    draft.body = draft.body + token
  }
}
</script>

<template>
  <el-drawer
    v-model="visible"
    :title="$t('noticeTemplate.title')"
    direction="rtl"
    size="78%"
    :destroy-on-close="false"
  >
    <div class="ntpl-drawer" v-loading="loading">
      <p class="ntpl-subtitle">{{ $t('noticeTemplate.subtitle') }}</p>

      <el-tabs v-model="activeChannel" tab-position="left" class="ntpl-tabs">
        <el-tab-pane v-for="ch in NOTIFICATION_CHANNELS" :key="ch" :name="ch">
          <template #label>
            <span class="ntpl-tab-label">
              {{ channelTitleMap[ch] }}
              <span v-if="drafts[ch].isCustom" class="ntpl-badge">{{ $t('noticeTemplate.customBadge') }}</span>
            </span>
          </template>

          <div class="ntpl-pane">
            <div class="ntpl-pane-main">
              <el-form label-position="top">
                <el-form-item :label="$t('noticeTemplate.subjectLabel')">
                  <el-input
                    v-model="drafts[ch].subject"
                    type="text"
                    :placeholder="$t(channelHints[ch].subjectFor)"
                  />
                </el-form-item>

                <el-form-item :label="$t('noticeTemplate.bodyLabel')">
                  <el-input
                    :ref="(el: any) => (bodyTextareas[ch] = el?.textarea ?? null)"
                    v-model="drafts[ch].body"
                    type="textarea"
                    :rows="14"
                    :placeholder="$t(channelHints[ch].bodyFor)"
                    class="ntpl-body"
                  />
                </el-form-item>

                <div class="ntpl-actions">
                  <el-button
                    type="primary"
                    :loading="drafts[ch].saving"
                    @click="onSave(ch)"
                  >{{ $t('noticeTemplate.save') }}</el-button>
                  <el-button :loading="drafts[ch].previewing" @click="onPreview(ch)">
                    {{ $t('noticeTemplate.preview') }}
                  </el-button>
                  <el-button
                    type="warning"
                    plain
                    :loading="drafts[ch].resetting"
                    @click="onReset(ch)"
                  >{{ $t('noticeTemplate.reset') }}</el-button>
                  <span v-if="drafts[ch].updatedAt" class="ntpl-meta">
                    {{ $t('noticeTemplate.updatedAt', { time: drafts[ch].updatedAt }) }}
                  </span>
                </div>
              </el-form>

              <div v-if="drafts[ch].preview" class="ntpl-preview">
                <div class="ntpl-preview-title">{{ $t('noticeTemplate.previewTitle') }}</div>
                <div v-if="drafts[ch].preview!.subject" class="ntpl-preview-subject">
                  <span class="ntpl-preview-label">{{ $t('noticeTemplate.subjectLabel') }}：</span>
                  <span>{{ drafts[ch].preview!.subject }}</span>
                </div>
                <pre class="ntpl-preview-body">{{ drafts[ch].preview!.body }}</pre>
              </div>
            </div>

            <div class="ntpl-pane-side">
              <div class="ntpl-side-title">{{ $t('noticeTemplate.variablesTitle') }}</div>
              <p class="ntpl-side-hint">{{ $t('noticeTemplate.variablesHint') }}</p>
              <ul class="ntpl-vars">
                <li v-for="v in variableTokens" :key="v.token">
                  <button type="button" class="ntpl-var-token" @click="insertVariable(ch, v.token)">
                    {{ v.token }}
                  </button>
                  <span class="ntpl-var-desc">{{ $t(v.descKey) }}</span>
                </li>
              </ul>
            </div>
          </div>
        </el-tab-pane>
      </el-tabs>
    </div>
  </el-drawer>
</template>

<style scoped>
.ntpl-drawer {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: 100%;
}
.ntpl-subtitle {
  margin: 0 0 8px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.ntpl-tabs {
  flex: 1;
  min-height: 0;
}
.ntpl-tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.ntpl-badge {
  display: inline-block;
  padding: 0 6px;
  height: 18px;
  line-height: 18px;
  font-size: 11px;
  border-radius: 9px;
  background: var(--el-color-primary-light-8);
  color: var(--el-color-primary);
}
.ntpl-pane {
  display: grid;
  grid-template-columns: 1fr 280px;
  gap: 18px;
  align-items: start;
}
.ntpl-pane-main {
  min-width: 0;
}
.ntpl-body :deep(.el-textarea__inner) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
  line-height: 1.55;
}
.ntpl-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.ntpl-meta {
  margin-left: auto;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.ntpl-preview {
  margin-top: 14px;
  padding: 12px 14px;
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  background: var(--el-fill-color-lighter);
}
.ntpl-preview-title {
  font-weight: 600;
  margin-bottom: 8px;
}
.ntpl-preview-subject {
  margin-bottom: 6px;
  font-size: 13px;
}
.ntpl-preview-label {
  color: var(--el-text-color-secondary);
}
.ntpl-preview-body {
  margin: 0;
  padding: 8px 10px;
  border-radius: 6px;
  background: var(--el-bg-color);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12.5px;
  white-space: pre-wrap;
  word-break: break-word;
}
.ntpl-pane-side {
  position: sticky;
  top: 0;
  padding: 14px;
  border: 1px solid var(--el-border-color);
  border-radius: 10px;
  background: var(--el-fill-color-lighter);
}
.ntpl-side-title {
  font-weight: 600;
  margin-bottom: 6px;
}
.ntpl-side-hint {
  margin: 0 0 10px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.ntpl-vars {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.ntpl-vars li {
  display: flex;
  align-items: center;
  gap: 8px;
}
.ntpl-var-token {
  flex-shrink: 0;
  padding: 2px 8px;
  border-radius: 6px;
  border: 1px solid var(--el-border-color);
  background: var(--el-bg-color);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  cursor: pointer;
  color: var(--el-color-primary);
}
.ntpl-var-token:hover {
  background: var(--el-color-primary-light-9);
}
.ntpl-var-desc {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
