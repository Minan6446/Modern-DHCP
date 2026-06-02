import type { OptionAssignment, OptionTestRequest, OptionTestResult } from '@/types/option';
import { normalizeValue } from './optionValidator';

export const simulateOptionDelivery = (request: OptionTestRequest): OptionTestResult => {
  const options: OptionAssignment[] = [];
  const inline = request.inlineOptions || [];
  inline.forEach((o) =>
    options.push({
      ...o,
      value: normalizeValue(
        { dataType: 'string', category: 'custom', code: o.optionCode, name: '', description: '' },
        o.value
      )
    })
  );
  if (request.templateId) {
    options.push({ optionCode: 54, value: '192.168.0.1', scope: 'global' });
  }
  return {
    offeredOptions: options,
    rawPacketHex: '02010600DEADBEEF',
    logs: ['模拟发送 DHCP OFFER', '写入选项字段', '完成封包'],
    latencyMs: Math.round(Math.random() * 20) + 5
  };
};
