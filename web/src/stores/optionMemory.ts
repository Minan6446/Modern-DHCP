import { reactive, computed } from 'vue';
import { isAxiosError } from 'axios';
import {
  fetchDhcpOptions,
  createDhcpOption,
  updateDhcpOption,
  deleteDhcpOption,
  fetchDhcpTemplates,
  createDhcpTemplate,
  updateDhcpTemplate,
  deleteDhcpTemplate,
  applyDhcpTemplate,
  fetchDhcpScopes,
  createDhcpScope,
  updateDhcpScope,
  deleteDhcpScope,
  syncDhcpTemplate,
  type DhcpOptionDTO,
  type DhcpTemplateDTO,
  type DhcpScopeDTO
} from '@/api/dhcpOptions';

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

const uuid = () => crypto.randomUUID();

const normalizeScope = (input: Partial<DhcpScope> & { id?: string }): DhcpScope => ({
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

const defaultScopes = (): DhcpScope[] => [];

const loadPersistedScopes = (): DhcpScope[] => defaultScopes();

const persistScopes = (_scopes: DhcpScope[]) => {
  // Scope data must be backend/database authoritative.
};

const mapScopeDto = (dto: DhcpScopeDTO): DhcpScope =>
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

const toScopePayload = (scope: Partial<DhcpScope>) => ({
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

const toScopePatchPayload = (patch: Partial<DhcpScope>) => {
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

const state = reactive<{ options: DhcpOption[]; scopes: DhcpScope[]; templates: DhcpTemplate[] }>({
  options: [],
  scopes: defaultScopes(),
  templates: []
});

const resolveApiErrorMessage = (error: unknown, fallback: string) => {
  if (!isAxiosError(error)) return fallback;
  const payload = error.response?.data as { message?: string } | undefined;
  const message = payload?.message;
  if (typeof message === 'string' && message.trim()) return message.trim();
  return fallback;
};

export const useOptionMemory = () => {
  const hydrated = reactive({ loaded: false });

  const list = computed(() => state.options);
  const scopes = computed(() => state.scopes);
  const templates = computed(() => state.templates);

  const mapOptionDto = (dto: DhcpOptionDTO): DhcpOption => ({
    id: dto.id || uuid(),
    code: dto.code,
    name: dto.name,
    description: dto.description || '',
    value: dto.value || dto.sampleValue || dto.valueExample || '',
    category: 'custom',
    persisted: true
  });

  const templateOptionFromOption = (opt: DhcpOption): DhcpTemplateOption => ({
    optionId: opt.id,
    code: opt.code,
    name: opt.name,
    value: opt.value,
    description: opt.description
  });

  const snapshotTemplateVersion = (tpl: DhcpTemplate, note?: string): DhcpTemplateVersion => {
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

  const normalizeTemplateOptions = (items: DhcpTemplateOption[]): DhcpTemplateOption[] => {
    const seen = new Set<number>();
    const result: DhcpTemplateOption[] = [];
    for (const item of items) {
      if (!item || !item.code || seen.has(item.code)) continue;
      seen.add(item.code);
      const existing = state.options.find((opt) => opt.id === item.optionId || opt.code === item.code);
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

  const mapTemplateDto = (dto: DhcpTemplateDTO): DhcpTemplate => {
    const options = (dto.options || []).map((item) => {
      const mapped = mapOptionDto(item);
      return templateOptionFromOption(mapped);
    });
    const now = dto.updatedAt || new Date().toISOString();
    const template: DhcpTemplate = {
      id: dto.id,
      name: dto.name,
      description: dto.description || '',
      icon: dto.icon,
      options: normalizeTemplateOptions(options),
      updatedAt: now,
      references: [],
      versions: []
    };
    template.versions = [snapshotTemplateVersion(template, 'initial')];
    return template;
  };

  const toTemplatePayloadOptions = (options: DhcpTemplateOption[]): DhcpOptionDTO[] =>
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

  const load = async () => {
    if (hydrated.loaded) return;
    try {
      const [optResp, tplResp, scopeResp] = await Promise.allSettled([
        fetchDhcpOptions(),
        fetchDhcpTemplates(),
        fetchDhcpScopes()
      ]);

      if (optResp.status === 'fulfilled') {
        const optData = optResp.value.data.data as DhcpOptionDTO[] | { items?: DhcpOptionDTO[] };
        const opts = Array.isArray(optData) ? optData : optData.items || [];
        state.options.splice(0, state.options.length, ...opts.map(mapOptionDto));
      }

      if (tplResp.status === 'fulfilled') {
        const tplData = tplResp.value.data.data as DhcpTemplateDTO[] | { items?: DhcpTemplateDTO[] };
        const templatesData = Array.isArray(tplData) ? tplData : tplData.items || [];
        state.templates.splice(
          0,
          state.templates.length,
          ...templatesData.map(mapTemplateDto)
        );
      }

      if (scopeResp.status === 'fulfilled') {
        const scopeData = scopeResp.value.data.data as DhcpScopeDTO[] | { items?: DhcpScopeDTO[] };
        const scopesData = Array.isArray(scopeData) ? scopeData : scopeData.items || [];
        const normalizedScopes = scopesData.map(mapScopeDto);
        state.scopes.splice(0, state.scopes.length, ...normalizedScopes);
        persistScopes(state.scopes);
      } else {
        state.scopes.splice(0, state.scopes.length);
      }
    } finally {
      hydrated.loaded = true;
    }
  };

  const addOption = async (option: Omit<DhcpOption, 'id'>): Promise<boolean> => {
    const created = { ...option, id: uuid(), persisted: false };
    state.options.push(created);
    const idx = state.options.length - 1;
    try {
      const resp = await createDhcpOption({
        code: option.code,
        name: option.name,
        value: option.value,
        dataType: 'string',
        description: option.description,
        scope: 'GLOBAL',
        format: 'string'
      });
      const id = (resp.data.data as DhcpOptionDTO | undefined)?.id;
      if (idx >= 0 && idx < state.options.length) {
        if (id) state.options[idx].id = id;
        state.options[idx].persisted = true;
      }
      return true;
    } catch {
      if (idx >= 0 && idx < state.options.length) {
        state.options[idx].persisted = false;
      }
      return false;
    }
  };

  const updateOption = (id: string, patch: Partial<DhcpOption>) => {
    const idx = state.options.findIndex((o) => o.id === id);
    if (idx >= 0) {
      const merged = { ...state.options[idx], ...patch };
      state.options[idx] = merged;
      void updateDhcpOption(id, {
        code: merged.code,
        name: merged.name,
        value: merged.value,
        description: merged.description,
        dataType: 'string',
        scope: 'GLOBAL',
        format: 'string'
      }).catch(() => undefined);
    }
  };

  const removeOption = async (id: string) => {
    const idx = state.options.findIndex((o) => o.id === id);
    if (idx < 0) {
      return { ok: false, message: '选项不存在或已删除' };
    }
    const removed = state.options[idx];
    state.options.splice(idx, 1);
    try {
      await deleteDhcpOption(id);
      return { ok: true };
    } catch (error) {
      state.options.splice(idx, 0, removed);
      return {
        ok: false,
        message: resolveApiErrorMessage(error, '删除失败，请稍后重试')
      };
    }
  };

  const codeExists = (code: number) => state.options.some((o) => o.code === code);

  const addScope = (scope: Omit<DhcpScope, 'id'>) => {
    const created = normalizeScope({ ...scope, id: uuid() });
    state.scopes.push(created);
    persistScopes(state.scopes);
    void createDhcpScope(toScopePayload(created))
      .then((resp) => {
        const id = (resp.data.data as DhcpScopeDTO | undefined)?.id;
        if (!id) return;
        const idx = state.scopes.findIndex((item) => item.id === created.id);
        if (idx >= 0) {
          state.scopes[idx].id = id;
          persistScopes(state.scopes);
        }
      })
      .catch(() => undefined);
    return created;
  };

  const addScopeSafe = async (scope: Omit<DhcpScope, 'id'>) => {
    const draft = normalizeScope({ ...scope, id: uuid() });
    try {
      const resp = await createDhcpScope(toScopePayload(draft));
      const createdDto = resp.data.data as DhcpScopeDTO | undefined;
      const created = createdDto ? mapScopeDto(createdDto) : draft;
      if (!created.id) created.id = draft.id;
      state.scopes.push(created);
      persistScopes(state.scopes);
      return { ok: true, scope: created };
    } catch (error) {
      return {
        ok: false,
        message: resolveApiErrorMessage(error, '创建失败，请稍后重试')
      };
    }
  };
  const updateScope = (id: string, patch: Partial<DhcpScope>) => {
    const idx = state.scopes.findIndex((s) => s.id === id);
    if (idx >= 0) {
      state.scopes[idx] = normalizeScope({ ...state.scopes[idx], ...patch, id });
      persistScopes(state.scopes);
      const changed = state.scopes[idx];
      void updateDhcpScope(id, toScopePayload(changed)).catch(() => undefined);
      return state.scopes[idx];
    }
    return undefined;
  };
  const updateScopeSafe = async (id: string, patch: Partial<DhcpScope>) => {
    const idx = state.scopes.findIndex((s) => s.id === id);
    if (idx < 0) {
      return { ok: false, message: '作用域不存在或已删除' };
    }
    const previous = normalizeScope({ ...state.scopes[idx] });
    const next = normalizeScope({ ...state.scopes[idx], ...patch, id });
    state.scopes[idx] = next;
    persistScopes(state.scopes);
    try {
      await updateDhcpScope(id, toScopePatchPayload(patch));
      return { ok: true, scope: next };
    } catch (error) {
      state.scopes[idx] = previous;
      persistScopes(state.scopes);
      return {
        ok: false,
        message: resolveApiErrorMessage(error, '更新失败，请稍后重试')
      };
    }
  };

  const removeScope = async (id: string) => {
    const idx = state.scopes.findIndex((s) => s.id === id);
    if (idx < 0) {
      return { ok: false, message: '作用域不存在或已删除' };
    }
    const removed = state.scopes[idx];
    state.scopes.splice(idx, 1);
    persistScopes(state.scopes);
    try {
      await deleteDhcpScope(id);
      return { ok: true };
    } catch (error) {
      state.scopes.splice(idx, 0, removed);
      persistScopes(state.scopes);
      return {
        ok: false,
        message: resolveApiErrorMessage(error, '删除失败，请稍后重试')
      };
    }
  };

  const addTemplate = (tpl: Omit<DhcpTemplate, 'id' | 'updatedAt' | 'references' | 'versions'>) => {
    const created: DhcpTemplate = {
      ...tpl,
      options: normalizeTemplateOptions(tpl.options),
      id: uuid(),
      updatedAt: new Date().toISOString(),
      references: [],
      versions: []
    };
    created.versions = [snapshotTemplateVersion(created, 'create')];
    state.templates.push(created);
    const idx = state.templates.length - 1;
    void createDhcpTemplate({
      name: created.name,
      description: created.description,
      icon: created.icon,
      options: toTemplatePayloadOptions(created.options)
    })
      .then((resp) => {
        const id = (resp.data.data as DhcpTemplateDTO | undefined)?.id;
        if (id) state.templates[idx].id = id;
      })
      .catch(() => undefined);
  };

  const addTemplateSafe = async (
    tpl: Omit<DhcpTemplate, 'id' | 'updatedAt' | 'references' | 'versions'>
  ) => {
    const normalizedOptions = normalizeTemplateOptions(tpl.options);
    try {
      const resp = await createDhcpTemplate({
        name: tpl.name,
        description: tpl.description,
        icon: tpl.icon,
        options: toTemplatePayloadOptions(normalizedOptions)
      });
      const dto = resp.data.data as DhcpTemplateDTO | undefined;
      const created: DhcpTemplate = {
        id: dto?.id || uuid(),
        name: tpl.name,
        description: tpl.description,
        icon: tpl.icon,
        options: normalizedOptions,
        updatedAt: dto?.updatedAt || new Date().toISOString(),
        references: [],
        versions: []
      };
      created.versions = [snapshotTemplateVersion(created, 'create')];
      state.templates.push(created);
      return { ok: true, template: created };
    } catch (error) {
      return {
        ok: false,
        message: resolveApiErrorMessage(error, '模板创建失败，请稍后重试')
      };
    }
  };

  const updateTemplate = (
    id: string,
    patch: Partial<Omit<DhcpTemplate, 'id' | 'versions' | 'references'>> &
      Pick<DhcpTemplate, 'name' | 'description' | 'options'>
  ) => {
    const idx = state.templates.findIndex((t) => t.id === id);
    if (idx >= 0) {
      const previous = state.templates[idx];
      const merged: DhcpTemplate = {
        ...previous,
        ...patch,
        id,
        options: normalizeTemplateOptions(patch.options),
        updatedAt: new Date().toISOString(),
        references: [...previous.references],
        versions: [...previous.versions]
      };
      merged.versions.push(snapshotTemplateVersion(merged, 'update'));
      state.templates[idx] = merged;
      void updateDhcpTemplate(id, {
        name: merged.name,
        description: merged.description,
        icon: merged.icon,
        options: toTemplatePayloadOptions(merged.options)
      }).catch(() => undefined);
    }
  };

  const updateTemplateSafe = async (
    id: string,
    patch: Partial<Omit<DhcpTemplate, 'id' | 'versions' | 'references'>> &
      Pick<DhcpTemplate, 'name' | 'description' | 'options'>
  ) => {
    const idx = state.templates.findIndex((t) => t.id === id);
    if (idx < 0) {
      return { ok: false, message: '模板不存在或已删除' };
    }
    const previous = state.templates[idx];
    const merged: DhcpTemplate = {
      ...previous,
      ...patch,
      id,
      options: normalizeTemplateOptions(patch.options),
      updatedAt: new Date().toISOString(),
      references: [...previous.references],
      versions: [...previous.versions]
    };
    try {
      await updateDhcpTemplate(id, {
        name: merged.name,
        description: merged.description,
        icon: merged.icon,
        options: toTemplatePayloadOptions(merged.options)
      });
      merged.versions.push(snapshotTemplateVersion(merged, 'update'));
      state.templates[idx] = merged;
      return { ok: true, template: merged };
    } catch (error) {
      return {
        ok: false,
        message: resolveApiErrorMessage(error, '模板更新失败，请稍后重试')
      };
    }
  };

  const removeTemplate = async (id: string) => {
    const idx = state.templates.findIndex((t) => t.id === id);
    if (idx < 0) {
      return { ok: false, message: '模板不存在或已删除' };
    }
    const removed = state.templates[idx];
    state.templates.splice(idx, 1);
    try {
      await deleteDhcpTemplate(id);
      return { ok: true };
    } catch (error) {
      state.templates.splice(idx, 0, removed);
      return {
        ok: false,
        message: resolveApiErrorMessage(error, '删除失败，请稍后重试')
      };
    }
  };

  const createTemplateFromScope = (
    scopeID: string,
    payload: { name: string; description?: string; icon?: string }
  ) => {
    const scope = state.scopes.find((item) => item.id === scopeID);
    if (!scope) return false;
    const options = scope.options
      .map((id) => state.options.find((opt) => opt.id === id))
      .filter((opt): opt is DhcpOption => !!opt)
      .map(templateOptionFromOption);
    addTemplate({
      name: payload.name,
      description: payload.description || '',
      icon: payload.icon,
      options
    });
    return true;
  };

  const createTemplateFromOptionIDs = (
    optionIDs: string[],
    payload: { name: string; description?: string; icon?: string }
  ) => {
    const options = optionIDs
      .map((id) => state.options.find((item) => item.id === id))
      .filter((item): item is DhcpOption => !!item)
      .map(templateOptionFromOption);
    addTemplate({
      name: payload.name,
      description: payload.description || '',
      icon: payload.icon,
      options
    });
  };

  const ensureOptionForTemplateItem = (item: DhcpTemplateOption): DhcpOption => {
    const byID = state.options.find((opt) => opt.id === item.optionId);
    if (byID) {
      byID.value = item.value;
      if (item.name) byID.name = item.name;
      if (item.description) byID.description = item.description;
      return byID;
    }
    const byCode = state.options.find((opt) => opt.code === item.code);
    if (byCode) {
      byCode.value = item.value;
      if (item.name) byCode.name = item.name;
      if (item.description) byCode.description = item.description;
      return byCode;
    }
    const created: DhcpOption = {
      id: item.optionId || uuid(),
      code: item.code,
      name: item.name,
      description: item.description || '',
      value: item.value,
      category: 'template',
      persisted: true
    };
    state.options.push(created);
    return created;
  };

  const applyTemplateToScopes = (
    templateID: string,
    scopeIDs: string[],
    mode: 'merge' | 'overwrite',
    conflict: 'template_wins' | 'keep_scope' = 'template_wins'
  ) => {
    const tpl = state.templates.find((item) => item.id === templateID);
    if (!tpl) return { applied: 0 };
    let applied = 0;
    for (const scopeID of scopeIDs) {
      const scope = state.scopes.find((item) => item.id === scopeID);
      if (!scope) continue;
      const incomingOptions = tpl.options.map(ensureOptionForTemplateItem);
      const incomingIDs = incomingOptions.map((opt) => opt.id);
      if (mode === 'overwrite') {
        scope.options = incomingIDs;
      } else {
        const existingIDs = [...scope.options];
        const existingByCode = new Map<number, string>();
        for (const id of existingIDs) {
          const opt = state.options.find((item) => item.id === id);
          if (opt) existingByCode.set(opt.code, id);
        }
        for (const incoming of incomingOptions) {
          const existID = existingByCode.get(incoming.code);
          if (!existID) {
            existingIDs.push(incoming.id);
            existingByCode.set(incoming.code, incoming.id);
            continue;
          }
          if (conflict === 'template_wins') {
            const idx = existingIDs.indexOf(existID);
            if (idx >= 0) existingIDs[idx] = incoming.id;
            existingByCode.set(incoming.code, incoming.id);
          }
        }
        scope.options = Array.from(new Set(existingIDs));
      }
      scope.templateId = templateID;
      if (!tpl.references.includes(scopeID)) tpl.references.push(scopeID);
      persistScopes(state.scopes);
      applied++;
    }
    tpl.updatedAt = new Date().toISOString();
    return { applied };
  };

  const applyTemplateToGlobal = (
    templateID: string,
    mode: 'merge' | 'overwrite',
    conflict: 'template_wins' | 'keep_scope' = 'template_wins'
  ) => {
    const tpl = state.templates.find((item) => item.id === templateID);
    if (!tpl) return false;
    const incoming = tpl.options.map(ensureOptionForTemplateItem);
    if (mode === 'overwrite') {
      const keepCodes = new Set(incoming.map((item) => item.code));
      const retained = state.options.filter((item) => !keepCodes.has(item.code));
      state.options.splice(0, state.options.length, ...retained, ...incoming);
      return true;
    }
    for (const item of incoming) {
      const exist = state.options.find((opt) => opt.code === item.code);
      if (!exist) {
        state.options.push(item);
        continue;
      }
      if (conflict === 'template_wins') {
        exist.name = item.name;
        exist.value = item.value;
        exist.description = item.description;
      }
    }
    return true;
  };

  const removeTemplates = async (
    ids: string[],
    opts?: { force?: boolean; unlinkScopes?: boolean }
  ) => {
    const blocked: string[] = [];
    const deleted: string[] = [];
    const failed: Array<{ id: string; message: string }> = [];
    for (const id of ids) {
      const tpl = state.templates.find((item) => item.id === id);
      if (!tpl) continue;
      const refs = tpl.references.filter((scopeID) => state.scopes.some((scope) => scope.id === scopeID));
      if (refs.length > 0 && !opts?.force && !opts?.unlinkScopes) {
        blocked.push(id);
        continue;
      }
      if (opts?.unlinkScopes) {
        const optionIDs = new Set(tpl.options.map((item) => item.optionId));
        for (const scope of state.scopes) {
          if (!refs.includes(scope.id)) continue;
          scope.options = scope.options.filter((oid) => !optionIDs.has(oid));
          if (scope.templateId === id) scope.templateId = '';
        }
        persistScopes(state.scopes);
      }
      const result = await removeTemplate(id);
      if (result.ok) {
        deleted.push(id);
      } else {
        failed.push({ id, message: result.message || '删除失败' });
      }
    }
    return { deleted, blocked, failed };
  };

  const restoreTemplateVersion = (templateID: string, versionID: string) => {
    const tpl = state.templates.find((item) => item.id === templateID);
    if (!tpl) return false;
    const version = tpl.versions.find((item) => item.id === versionID);
    if (!version) return false;
    tpl.name = version.name;
    tpl.description = version.description;
    tpl.icon = version.icon;
    tpl.options = normalizeTemplateOptions(version.options);
    tpl.updatedAt = new Date().toISOString();
    tpl.versions.push(snapshotTemplateVersion(tpl, `restore:v${version.version}`));
    void updateDhcpTemplate(tpl.id, {
      name: tpl.name,
      description: tpl.description,
      icon: tpl.icon,
      options: toTemplatePayloadOptions(tpl.options)
    }).catch(() => undefined);
    return true;
  };

  const applyTemplate = async (tplId: string) => {
    const tpl = state.templates.find((t) => t.id === tplId);
    if (!tpl) {
      return { ok: false, message: '模板不存在或已删除' };
    }
    try {
      await applyDhcpTemplate(tplId);
    } catch (error) {
      return {
        ok: false,
        message: resolveApiErrorMessage(error, '模板应用失败，请稍后重试')
      };
    }
    tpl.options.forEach((opt) => {
      const existing = state.options.find((o) => o.code === opt.code);
      if (existing) {
        Object.assign(existing, {
          code: opt.code,
          name: opt.name,
          description: opt.description,
          value: opt.value,
          category: 'template'
        });
      } else {
        state.options.push({
          id: opt.optionId || uuid(),
          code: opt.code,
          name: opt.name,
          description: opt.description || '',
          value: opt.value,
          category: 'template',
          persisted: true
        });
      }
    });
    return { ok: true };
  };

  const templateUsage = (templateID: string) => {
    const tpl = state.templates.find((item) => item.id === templateID);
    if (!tpl) return { count: 0, scopes: [] as DhcpScope[] };
    const scopeIdSet = new Set<string>(tpl.references);
    state.scopes.forEach((scope) => {
      if (scope.templateId === templateID) scopeIdSet.add(scope.id);
    });
    const scopesUsing = state.scopes.filter((scope) => scopeIdSet.has(scope.id));
    return { count: scopesUsing.length, scopes: scopesUsing };
  };

  const optionReferenceCount = (optionID: string) => {
    const byID = state.options.find((item) => item.id === optionID);
    if (!byID) return { scopeCount: 0, templateCount: 0, total: 0 };
    const templateCount = state.templates.filter((tpl) =>
      tpl.options.some((item) => item.optionId === optionID || item.code === byID.code)
    ).length;
    const scopeCount = state.scopes.filter((scope) =>
      scope.options.some((id) => {
        if (id === optionID) return true;
        const opt = state.options.find((item) => item.id === id);
        return !!opt && opt.code === byID.code;
      })
    ).length;
    return { scopeCount, templateCount, total: scopeCount + templateCount };
  };

  const syncTemplateToReferencedScopes = async (
    templateID: string,
    mode: 'merge' | 'overwrite' = 'merge',
    conflict: 'template_wins' | 'keep_scope' = 'template_wins'
  ) => {
    const usage = templateUsage(templateID);
    if (!usage.count) return { ok: true, applied: 0 };
    const result = applyTemplateToScopes(
      templateID,
      usage.scopes.map((scope) => scope.id),
      mode,
      conflict
    );
    try {
      await syncDhcpTemplate(templateID, mode === 'overwrite' ? 'replace' : 'merge');
      return { ok: true, applied: result.applied };
    } catch (error) {
      return {
        ok: false,
        applied: result.applied,
        message: resolveApiErrorMessage(error, '模板同步失败，请稍后重试')
      };
    }
  };

  return {
    load,
    list,
    scopes,
    templates,
    addOption,
    updateOption,
    removeOption,
    codeExists,
    addScope,
    addScopeSafe,
    updateScope,
    updateScopeSafe,
    removeScope,
    addTemplate,
    addTemplateSafe,
    createTemplateFromScope,
    createTemplateFromOptionIDs,
    updateTemplate,
    updateTemplateSafe,
    removeTemplate,
    removeTemplates,
    applyTemplate,
    applyTemplateToScopes,
    applyTemplateToGlobal,
    syncTemplateToReferencedScopes,
    restoreTemplateVersion,
    templateUsage,
    optionReferenceCount
  };
};
