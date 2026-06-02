import { isIPv4 } from './ip';
import { formatMac } from './mac';
import type { OptionDefinition, OptionDataType } from '@/types/option';

const domainRegex = /^(?!-)([a-zA-Z0-9-]{1,63}\.)+[a-zA-Z]{2,}$/;
const hexRegex = /^(?:[0-9a-fA-F]{2})+$/;

const withinRange = (num: number, min?: number, max?: number) => {
  if (min !== undefined && num < min) return false;
  if (max !== undefined && num > max) return false;
  return true;
};

const lengthOk = (val: string, min?: number, max?: number) => {
  if (min !== undefined && val.length < min) return false;
  if (max !== undefined && val.length > max) return false;
  return true;
};

export const validateByType = (
  type: OptionDataType,
  value: string
): { valid: boolean; message?: string } => {
  const trimmed = value.trim();
  if (!trimmed) return { valid: false, message: '值不能为空' };
  switch (type) {
    case 'ip':
      return isIPv4(trimmed)
        ? { valid: true }
        : { valid: false, message: '必须是有效的 IPv4 地址' };
    case 'ip-list': {
      const ips = trimmed
        .split(',')
        .map((i) => i.trim())
        .filter(Boolean);
      if (!ips.length) return { valid: false, message: '至少提供一个 IP' };
      const invalid = ips.find((i) => !isIPv4(i));
      return invalid ? { valid: false, message: `无效 IP: ${invalid}` } : { valid: true };
    }
    case 'domain':
    case 'fqdn':
      return domainRegex.test(trimmed)
        ? { valid: true }
        : { valid: false, message: '必须是有效的域名/FQDN' };
    case 'hex':
      return hexRegex.test(trimmed)
        ? { valid: true }
        : { valid: false, message: '需要偶数字节的十六进制字符串' };
    case 'string':
      return { valid: true };
    case 'boolean':
      return ['true', 'false', '1', '0'].includes(trimmed.toLowerCase())
        ? { valid: true }
        : { valid: false, message: '布尔值应为 true/false/1/0' };
    case 'uint8':
    case 'uint16':
    case 'uint32': {
      const num = Number(trimmed);
      if (Number.isNaN(num)) return { valid: false, message: '需要数字' };
      const max = type === 'uint8' ? 255 : type === 'uint16' ? 65535 : 4294967295;
      return num >= 0 && num <= max
        ? { valid: true }
        : { valid: false, message: `超出 ${type} 范围` };
    }
    default:
      return { valid: true };
  }
};

export const validateOptionValue = (definition: OptionDefinition, value: string) => {
  const typeResult = validateByType(definition.dataType, value);
  if (!typeResult.valid) return typeResult;
  if (!lengthOk(value, definition.minLength, definition.maxLength)) {
    return { valid: false, message: '长度不在允许范围内' };
  }
  if (definition.allowedValues && definition.allowedValues.length) {
    if (!definition.allowedValues.map(String).includes(value)) {
      return { valid: false, message: '值不在允许列表内' };
    }
  }
  if (definition.range) {
    const num = Number(value);
    if (!withinRange(num, definition.range.min, definition.range.max)) {
      return { valid: false, message: '值超出配置范围' };
    }
  }
  if (definition.pattern) {
    const reg = new RegExp(definition.pattern);
    if (!reg.test(value)) return { valid: false, message: '未匹配自定义规则' };
  }
  return { valid: true };
};

export const normalizeValue = (definition: OptionDefinition, value: string) => {
  if (definition.dataType === 'ip-list') {
    return value
      .split(',')
      .map((i) => i.trim())
      .filter(Boolean)
      .join(',');
  }
  if (definition.dataType === 'hex') return value.toUpperCase();
  if (definition.dataType === 'boolean')
    return ['true', '1'].includes(value.toLowerCase()) ? '1' : '0';
  if (definition.dataType === 'string') return value.trim();
  if (definition.dataType === 'domain' || definition.dataType === 'fqdn')
    return value.toLowerCase();
  if (
    definition.dataType === 'uint8' ||
    definition.dataType === 'uint16' ||
    definition.dataType === 'uint32'
  )
    return String(Number(value));
  if (definition.dataType === 'ip') return value.trim();
  if (definition.dataType === 'ip-list') return value.trim();
  return value;
};

export const maskClientId = (mac: string) => formatMac(mac).replace(/:[0-9A-F]{2}$/g, ':**');
