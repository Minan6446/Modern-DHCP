// Pure utility functions for the DHCP option memory store.
// Extracted from optionMemory.ts for separation of concerns.
import type { DhcpOption, DhcpScope, DhcpTemplateOption, DhcpTemplateVersion, DhcpTemplate, DhcpScopePayload } from './optionTypes';
import type { DhcpOptionDTO, DhcpTemplateDTO, DhcpScopeDTO } from '@/api/dhcpOptions';

const uuid = () => crypto.randomUUID();

export const normalizeScope = (input: Partial<DhcpScope> & { id?: string }): DhcpScope => ({
  id: input.id || uuid(),
  name: input.name || '',
  subnet: input.subnet || '',
  range: input.range || '',
  gateway: input.gateway || '',
  status: input.status === 'inactive' ? 'inactive' : 'active',
  options: Array.isArray(input.options) ? input.options.filter((item): item is string => typeof item === 'string') : [],
  templateId: input.templateId || '',
  notes: input.notes || ''
});

export const defaultScopes = (): DhcpScope[] => [];

export const loadPersistedScopes = (): DhcpScope[] => defaultScopes();

export const persistScopes = (_scopes: DhcpScope[]) => {
  // Scope data must be backend/database authoritative.
};

export const mapScopeDto = (dto: DhcpScopeDTO): DhcpScope =>
  normalizeScope({
    id: dto.id,
    name: dto.name,
    subnet: dto.subnet || dto.target || '',
    range: dto.range || '',
    gateway: dto.gateway || '',
    status: dto.status === 'inactive' ? 'inactive' : 'active',
    options: dto.optionIds || [],
    templateId: dto.templateId || '',
    notes: dto.notes || dto.description || ''
  });

export const toScopePayload = (scope: Partial<DhcpScope>): DhcpScopePayload => ({
  name: scope.name,
  subnet: scope.subnet,
  range: scope.range,
  gateway: scope.gateway,
  status: scope.status,
  target: scope.subnet,
  optionIds: scope.options,
  templateId: scope.templateId,
  notes: scope.notes,
  description: scope.notes
});

export const toScopePatchPayload = (patch: Partial<DhcpScope>): DhcpScopePayload => {
  const payload: DhcpScopePayload = {};
  if (patch.name !== undefined) payload.name = patch.name;
  if (patch.subnet !== undefined) {
    payload.subnet = patch.subnet;
    payload.target = patch.subnet;
  }
  if (patch.range !== undefined) payload.range = patch.range;
  if (patch.gateway !== undefined) payload.gateway = patch.gateway;
  if (patch.status !== undefined) payload.status = patch.status;
  if (patch.options !== undefined) payload.optionIds = patch.options;
  if (patch.templateId !== undefined) payload.templateId = patch.templateId;
  if (patch.notes !== undefined) {
    payload.notes = patch.notes;
    payload.description = patch.notes;
  }
  return payload;
};

export const mapOptionDto = (dto: DhcpOptionDTO): DhcpOption => ({
  id: dto.id || uuid(),
  code: dto.code,
  name: dto.name,
  description: dto.description || '',
  value: dto.value || dto.sampleValue || dto.valueExample || '',
  category: 'custom',
  persisted: true
});

export const templateOptionFromOption = (opt: DhcpOption): DhcpTemplateOption => ({
  optionId: opt.id,
  code: opt.code,
  name: opt.name,
  value: opt.value,
  description: opt.description
});

export const snapshotTemplateVersion = (tpl: DhcpTemplate, note?: string): DhcpTemplateVersion => {
  const prev = tpl.versions.at(-1)?.version || 0;
  return {
    id: uuid(),
    version: prev + 1,
    updatedAt: new Date().toISOString(),
    note,
    name: tpl.name,
    description: tpl.description,
    icon: tpl.icon,
    options: tpl.options.map((item) => ({ ...item }))
  };
};

export const normalizeTemplateOptions = (options: DhcpTemplateOption[], existingOptions: DhcpOption[]): DhcpTemplateOption[] => {
  const seen = new Set<number>();
  const result: DhcpTemplateOption[] = [];
  for (const item of options) {
    if (!item || !item.code || seen.has(item.code)) continue;
    seen.add(item.code);
    const existing = existingOptions.find((opt) => opt.id === item.optionId || opt.code === item.code);
    result.push({
      optionId: existing?.id || item.optionId || uuid(),
      code: item.code,
      name: item.name || existing?.name || `Option ${item.code}`,
      value: item.value ?? existing?.value ?? '',
      description: item.description || existing?.description || ''
    });
  }
  return result;
};

export const toTemplatePayloadOptions = (options: DhcpTemplateOption[]): DhcpOptionDTO[] =>
  options.map((item) => ({
    id: item.optionId,
    code: item.code,
    name: item.name,
    value: item.value,
    description: item.description,
    dataType: 'string',
    format: 'string',
    scope: 'GLOBAL'
  }));
