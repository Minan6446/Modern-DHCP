export interface BusinessErrorMap {
  [code: string]: string;
}

const defaultMap: BusinessErrorMap = {
  TENANT_REQUIRED: '请选择租户后重试',
  APPROVAL_REQUIRED: '该操作需要审批，请提交审批流程',
  QUOTA_EXCEEDED: '已超出配额限制，请调整后再试'
};

export const mapBusinessError = (code?: string, override?: BusinessErrorMap) => {
  if (!code) return '';
  const table = { ...defaultMap, ...(override || {}) };
  return table[code] || '';
};
