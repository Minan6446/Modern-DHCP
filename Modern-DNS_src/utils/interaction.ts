import { ElMessageBox } from 'element-plus'
import type { FormInstance } from 'element-plus'

interface ConfirmRiskOptions {
  title?: string
  action: string
  target?: string
  risk?: string
}

interface ValidateFailure {
  [key: string]: unknown
  fields?: Record<string, unknown>
}

const ensureRiskText = (text: string, risk: string): string => {
  if (!text.trim()) {
    return risk
  }
  if (text.includes('不可恢复') || text.includes('高风险') || text.includes('风险')) {
    return text
  }
  return `${text}。${risk}`
}

const focusByProp = (prop: string): void => {
  const item = document.querySelector(`.el-form-item[prop="${prop}"]`) as HTMLElement | null
  if (!item) {
    return
  }

  const field = item.querySelector(
    '.el-input__inner, .el-textarea__inner, .el-select__input, .el-switch__core, .el-checkbox__input, .el-radio__input',
  ) as HTMLElement | null

  if (field) {
    field.focus()
    return
  }

  item.focus()
}

export const confirmRiskAction = async ({ title = '风险确认', action, target, risk = '该操作存在风险，请确认后继续。' }: ConfirmRiskOptions): Promise<void> => {
  const detail = target ? `【${target}】` : ''
  const message = ensureRiskText(`确认执行${action}${detail}吗？`, risk)
  await ElMessageBox.confirm(message, title, {
    type: 'warning',
    confirmButtonText: '确认执行',
    cancelButtonText: '取消',
    closeOnClickModal: false,
    closeOnPressEscape: true,
    distinguishCancelAndClose: true,
  })
}

export const validateFormAndFocus = async (form: FormInstance | undefined): Promise<boolean> => {
  if (!form) {
    return false
  }

  try {
    await form.validate()
    return true
  } catch (error) {
    const failure = error as ValidateFailure
    const firstField = Object.keys(failure?.fields || {})[0]
    if (firstField) {
      form.scrollToField(firstField)
      window.setTimeout(() => {
        focusByProp(firstField)
      }, 20)
    }
    return false
  }
}

export const normalizeMultiValue = (value: string): string => {
  const tokens = String(value || '')
    .split(/[\n\r\t\s,;，；]+/)
    .map((item) => item.trim())
    .filter(Boolean)

  return Array.from(new Set(tokens)).join(',')
}
