export type OptionDataType =
  | 'ip'
  | 'ip-list'
  | 'domain'
  | 'string'
  | 'hex'
  | 'uint8'
  | 'uint16'
  | 'uint32'
  | 'boolean'
  | 'fqdn';

export type OptionCategory = 'basic' | 'network' | 'time' | 'security' | 'vendor' | 'custom';

export interface OptionDefinition {
  id?: string;
  code: number;
  name: string;
  dataType: OptionDataType;
  category: OptionCategory;
  description?: string;
  minLength?: number;
  maxLength?: number;
  recommendedLength?: number;
  valueExample?: string;
  value?: string;
  sampleValue?: string;
  standard?: boolean;
  vendorId?: string;
  allowedValues?: Array<string | number>;
  range?: { min: number; max: number };
  pattern?: string;
  version?: string;
  lastModifiedAt?: string;
}

export interface OptionTemplate {
  id: string;
  name: string;
  version: string;
  inheritsFrom?: string;
  description?: string;
  options: OptionAssignment[];
  status: 'draft' | 'active' | 'archived';
  createdAt?: string;
  updatedAt?: string;
}

export interface OptionTemplateGraphNode {
  id: string;
  name: string;
  version: string;
  inheritsFrom?: string;
}

export interface OptionAssignment {
  optionCode: number;
  value: string;
  scope: 'global' | 'pool' | 'binding' | 'profile';
  targetId?: string;
  validators?: string[];
}

export interface OptionUsageMetric {
  optionCode: number;
  name: string;
  frequency: number;
  references: number;
  lastUsedAt?: string;
}

export interface OptionUsageSnapshot {
  heat: OptionUsageMetric[];
  relations: Array<{ from: number; to: number; weight: number }>;
  history: Array<{
    timestamp: string;
    optionCode: number;
    action: string;
    operator?: string;
    value?: string;
  }>;
}

export interface OptionTestRequest {
  clientIp?: string;
  clientMac?: string;
  requestedOptions?: number[];
  templateId?: string;
  inlineOptions?: OptionAssignment[];
  vendorClassId?: string;
}

export interface OptionTestResult {
  offeredOptions: OptionAssignment[];
  rawPacketHex: string;
  logs: string[];
  latencyMs?: number;
}
