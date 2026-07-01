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
import type { DhcpOption, DhcpScope, DhcpTemplate, DhcpTemplateOption, DhcpTemplateVersion } from './optionTypes';
import {
  normalizeScope,
  defaultScopes,
  mapScopeDto,
  toScopePayload,
  toScopePatchPayload,
  mapOptionDto,
  templateOptionFromOption,
  snapshotTemplateVersion,
  normalizeTemplateOptions,
  toTemplatePayloadOptions
} from './optionHelpers';
// Re-export types for existing consumers
export type { DhcpOption, DhcpScope, OptionCategory, DhcpTemplate, DhcpTemplateOption, DhcpTemplateVersion } from './optionTypes';

const uuid = () => crypto.randomUUID();

const resolveApiErrorMessage = (error: unknown, fallback: string) => {
  if (!isAxiosError(error)) return fallback;
  const payload = error.response?.data as { message?: string } | undefined;
  const message = payload?.message;
  if (typeof message === 'string' && message.trim()) return message.trim();
  return fallback;
};

const state = reactive<{ options: DhcpOption[]; scopes: DhcpScope[]; templates: DhcpTemplate[] }>({
  options: [],
  scopes: defaultScopes(),
  templates: []
});

export const useOptionMemory = () => {
  const hydrated = reactive({ loaded: false });

  const list = computed(() => state.options);
  const scopes = computed(() => state.scopes);
  const templates = computed(() => state.templates);

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
      options: normalizeTemplateOptions(options, state.options),
      updatedAt: now,
      references: [],
      versions: []
    };
    template.versions = [snapshotTemplateVersion(template, 'initial')];
    return template;
  };

  const load = async () => {
    if (hydrated.loaded) return;
    try {
      const [optResp, tplResp, scopeResp] = await Promise.allSettled([
        fetchDhcpOptions().catch(() => null),
        fetchDhcpTemplates().catch(() => null),
        fetchDhcpScopes().catch(() => null)
      ]);

      if (optResp.status === 'fulfilled' && optResp.value) {
        try {
          const optData = optResp.value?.data?.data as DhcpOptionDTO[] | { items?: DhcpOptionDTO[] } | undefined;
          const opts = Array.isArray(optData) ? optData : (optData as any)?.items || [];
          state.options.splice(0, state.options.length, ...opts.map(mapOptionDto));
        } catch { /* skip malformed response */ }
      }

      if (tplResp.status === 'fulfilled' && tplResp.value) {
        try {
          const tplData = tplResp.value?.data?.data as DhcpTemplateDTO[] | { items?: DhcpTemplateDTO[] } | undefined;
          const templatesData = Array.isArray(tplData) ? tplData : (tplData as any)?.items || [];
          state.templates.splice(0, state.templates.length, ...templatesData.map(mapTemplateDto));
        } catch { /* skip malformed response */ }
      }

      if (scopeResp.status === 'fulfilled' && scopeResp.value) {
        try {
          const scopeData = scopeResp.value?.data?.data as DhcpScopeDTO[] | { items?: DhcpScopeDTO[] } | undefined;
          const scopesData = Array.isArray(scopeData) ? scopeData : (scopeData as any)?.items || [];
          const normalizedScopes = scopesData.map(mapScopeDto);
          state.scopes.splice(0, state.scopes.length, ...normalizedScopes);
        } catch { /* skip malformed response */ }
      } else if (scopeResp.status === 'rejected') {
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
    void createDhcpScope(toScopePayload(created))
      .then((resp) => {
        const id = (resp.data.data as DhcpScopeDTO | undefined)?.id;
        if (id) {
          const idx = state.scopes.findIndex((s) => s.id === created.id);
          if (idx >= 0) state.scopes[idx].id = id;
        }
      })
      .catch(() => {
        const idx = state.scopes.findIndex((s) => s.id === created.id);
        if (idx >= 0) state.scopes.splice(idx, 1);
      });
  };

  const updateScope = (id: string, patch: Partial<DhcpScope>) => {
    const idx = state.scopes.findIndex((s) => s.id === id);
    if (idx < 0) return;
    state.scopes[idx] = { ...state.scopes[idx], ...patch };
    void updateDhcpScope(id, toScopePatchPayload(patch)).catch(() => undefined);
  };

  const removeScope = async (id: string) => {
    const idx = state.scopes.findIndex((s) => s.id === id);
    if (idx < 0) return { ok: false, message: '作用域不存在或已删除' };
    const removed = state.scopes[idx];
    state.scopes.splice(idx, 1);
    try {
      await deleteDhcpScope(id);
      return { ok: true };
    } catch (error) {
      state.scopes.splice(idx, 0, removed);
      return {
        ok: false,
        message: resolveApiErrorMessage(error, '删除失败，请稍后重试')
      };
    }
  };

  const addTemplate = async (template: { name: string; description?: string; icon?: string; options: DhcpTemplateOption[] }) => {
    const created: DhcpTemplate = {
      id: uuid(),
      name: template.name,
      description: template.description || '',
      icon: template.icon,
      options: normalizeTemplateOptions(template.options, state.options),
      updatedAt: new Date().toISOString(),
      references: [],
      versions: [snapshotTemplateVersion({ name: template.name, description: template.description || '', options: template.options } as DhcpTemplate, 'initial')]
    };
    state.templates.push(created);
    try {
      const resp = await createDhcpTemplate({
        name: created.name,
        description: created.description,
        icon: created.icon,
        options: toTemplatePayloadOptions(created.options)
      });
      const returnedId = (resp.data.data as DhcpTemplateDTO | undefined)?.id;
      const idx = state.templates.findIndex((t) => t.id === created.id);
      if (idx >= 0) {
        if (returnedId) state.templates[idx].id = returnedId;
      }
      return { ok: true, template: idx >= 0 ? state.templates[idx] : created };
    } catch (error) {
      const idx = state.templates.findIndex((t) => t.id === created.id);
      if (idx >= 0) state.templates.splice(idx, 1);
      return { ok: false, message: resolveApiErrorMessage(error, '创建模板失败') };
    }
  };

  const updateTemplate = async (id: string, patch: Partial<DhcpTemplate>) => {
    const idx = state.templates.findIndex((t) => t.id === id);
    if (idx < 0) return { ok: false, message: '模板不存在' };
    const before = state.templates[idx];
    const merged = { ...before, ...patch };
    if (patch.options) {
      merged.options = normalizeTemplateOptions(patch.options, state.options);
    }
    state.templates[idx] = merged;
    try {
      await updateDhcpTemplate(id, {
        name: merged.name,
        description: merged.description,
        icon: merged.icon,
        options: toTemplatePayloadOptions(merged.options)
      });
      return { ok: true };
    } catch (error) {
      state.templates[idx] = before;
      return { ok: false, message: resolveApiErrorMessage(error, '更新模板失败') };
    }
  };

  const removeTemplate = async (id: string) => {
    const idx = state.templates.findIndex((t) => t.id === id);
    if (idx < 0) return { ok: false, message: '模板不存在或已删除' };
    const removed = state.templates[idx];
    state.templates.splice(idx, 1);
    try {
      await deleteDhcpTemplate(id);
      return { ok: true };
    } catch (error) {
      state.templates.splice(idx, 0, removed);
      return { ok: false, message: resolveApiErrorMessage(error, '删除模板失败') };
    }
  };

  const applyTemplate = async (templateId: string, scopeId: string) => {
    await applyDhcpTemplate(templateId, scopeId);
  };

  const syncTemplate = async (templateId: string, note?: string) => {
    const idx = state.templates.findIndex((t) => t.id === templateId);
    if (idx < 0) return { ok: false, message: '模板不存在' };
    try {
      const resp = await syncDhcpTemplate(templateId);
      const serverTemplate = (resp.data.data as DhcpTemplateDTO | undefined);
      if (serverTemplate) {
        const updated = mapTemplateDto(serverTemplate);
        state.templates[idx] = {
          ...updated,
          versions: [...state.templates[idx].versions, snapshotTemplateVersion(updated, note || `sync ${new Date().toISOString()}`)]
        };
      }
      return { ok: true };
    } catch (error) {
      return { ok: false, message: resolveApiErrorMessage(error, '同步模板失败') };
    }
  };

  const getOptionByCode = (code: number) => state.options.find((o) => o.code === code);

  const optionReferenceCount = (optionID: string) => {
    let count = 0;
    for (const scope of state.scopes) {
      if (scope.options.includes(optionID)) count++;
    }
    for (const tpl of state.templates) {
      if (tpl.options.some((o) => o.optionId === optionID)) count++;
    }
    return count;
  };

  const templateUsage = (templateID: string) => {
    let count = 0;
    for (const scope of state.scopes) {
      if (scope.templateId === templateID) count++;
    }
    return { count, templateId: templateID };
  };

  return {
    hydrated,
    list,
    scopes,
    templates,
    load,
    addOption,
    updateOption,
    removeOption,
    codeExists,
    addScope,
    updateScope,
    removeScope,
    addTemplate,
    updateTemplate,
    removeTemplate,
    applyTemplate,
    syncTemplate,
    getOptionByCode,
    optionReferenceCount,
    templateUsage
  };
};
