import { isIPv4, isIPv6 } from './ip';
import { isMac } from './mac';
import { validateOptionValue } from './optionValidator';
import type { OptionDefinition } from '@/types/option';

export const requiredRule = (message = '必填项'): FormItemRule => ({
  required: true,
  message,
  trigger: 'blur'
});

export const ipRule = (message = '无效的 IPv4 地址'): FormItemRule => ({
  validator: (_: unknown, val: string, cb: (err?: Error) => void) => {
    if (isIPv4(val || '')) cb();
    else cb(new Error(message));
  },
  trigger: 'blur'
});

export const ipv6Rule = (message = '无效的 IPv6 地址'): FormItemRule => ({
  validator: (_: unknown, val: string, cb: (err?: Error) => void) => {
    if (isIPv6(val || '')) cb();
    else cb(new Error(message));
  },
  trigger: 'blur'
});

export const macRule = (message = '无效的 MAC 地址'): FormItemRule => ({
  validator: (_: unknown, val: string, cb: (err?: Error) => void) => {
    if (isMac(val || '')) cb();
    else cb(new Error(message));
  },
  trigger: 'blur'
});

export const optionRule = (definition: OptionDefinition): FormItemRule => ({
  validator: (_: unknown, val: string, cb: (err?: Error) => void) => {
    const res = validateOptionValue(definition, val || '');
    res.valid ? cb() : cb(new Error(res.message || '值不合法'));
  },
  trigger: 'blur'
});

export const lengthRule = (min: number, max: number, message = '长度不合法'): FormItemRule => ({
  validator: (_: unknown, val: string, cb: (err?: Error) => void) => {
    if (!val) return cb();
    if (val.length < min || val.length > max) return cb(new Error(message));
    cb();
  },
  trigger: 'blur'
});

export const numberRangeRule = (min: number, max: number, message = '超出范围'): FormItemRule => ({
  validator: (_: unknown, val: number, cb: (err?: Error) => void) => {
    if (val === undefined || val === null) return cb();
    if (val < min || val > max) return cb(new Error(message));
    cb();
  },
  trigger: 'change'
});

export const businessRule = (predicate: () => boolean, message: string): FormItemRule => ({
  validator: (_: unknown, _val: unknown, cb: (err?: Error) => void) => {
    predicate() ? cb() : cb(new Error(message));
  },
  trigger: 'blur'
});
