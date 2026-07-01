// Type definitions for the DHCP option memory store.
// Extracted from optionMemory.ts for separation of concerns.

export type OptionCategory = 'standard' | 'custom' | 'vendor' | 'template';

export interface DhcpOption {
  id: string;
  code: number;
  name: string;
  description: string;
  value: string;
  category: OptionCategory;
  persisted?: boolean;
}

export interface DhcpScope {
  id: string;
  name: string;
  subnet: string;
  range: string;
  gateway: string;
  status: 'active' | 'inactive';
  options: string[];
  templateId?: string;
  notes?: string;
}

export interface DhcpTemplateOption {
  optionId: string;
  code: number;
  name: string;
  value: string;
  description?: string;
}

export interface DhcpTemplateVersion {
  id: string;
  version: number;
  updatedAt: string;
  note?: string;
  name: string;
  description: string;
  icon?: string;
  options: DhcpTemplateOption[];
}

export interface DhcpTemplate {
  id: string;
  name: string;
  description: string;
  icon?: string;
  options: DhcpTemplateOption[];
  updatedAt: string;
  references: string[];
  versions: DhcpTemplateVersion[];
}

export interface DhcpScopePayload {
  name?: string;
  subnet?: string;
  target?: string;
  range?: string;
  gateway?: string;
  status?: string;
  optionIds?: string[];
  templateId?: string;
  notes?: string;
  description?: string;
}
