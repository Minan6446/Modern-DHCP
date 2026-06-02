import type { OptionAssignment, OptionTemplate, OptionTemplateGraphNode } from '@/types/option';

export const buildInheritanceGraph = (templates: OptionTemplate[]): OptionTemplateGraphNode[] =>
  templates.map((t) => ({
    id: t.id,
    name: t.name,
    version: t.version,
    inheritsFrom: t.inheritsFrom
  }));

export const detectInheritanceCycle = (templates: OptionTemplate[]): boolean => {
  const map = new Map<string, string | undefined>();
  templates.forEach((t) => map.set(t.id, t.inheritsFrom));

  const visiting = new Set<string>();
  const visited = new Set<string>();

  const dfs = (id: string): boolean => {
    if (visited.has(id)) return false;
    if (visiting.has(id)) return true;
    visiting.add(id);
    const parent = map.get(id);
    if (parent && map.has(parent) && dfs(parent)) return true;
    visiting.delete(id);
    visited.add(id);
    return false;
  };

  for (const id of map.keys()) {
    if (dfs(id)) return true;
  }
  return false;
};

export const resolveTemplate = (templates: OptionTemplate[], id: string): OptionAssignment[] => {
  const map = new Map<string, OptionTemplate>();
  templates.forEach((t) => map.set(t.id, t));
  const visited = new Set<string>();

  const walk = (tid?: string): OptionAssignment[] => {
    if (!tid) return [];
    if (visited.has(tid)) return [];
    visited.add(tid);
    const t = map.get(tid);
    if (!t) return [];
    const parent = walk(t.inheritsFrom);
    const merged = new Map<number, OptionAssignment>();
    parent.forEach((p) => merged.set(p.optionCode, p));
    t.options.forEach((o) => merged.set(o.optionCode, o));
    return Array.from(merged.values());
  };

  return walk(id);
};

export const diffTemplates = (base: OptionAssignment[], next: OptionAssignment[]) => {
  const baseMap = new Map(base.map((b) => [b.optionCode, b] as const));
  const nextMap = new Map(next.map((n) => [n.optionCode, n] as const));
  const added: OptionAssignment[] = [];
  const removed: OptionAssignment[] = [];
  const changed: OptionAssignment[] = [];

  nextMap.forEach((val, code) => {
    if (!baseMap.has(code)) added.push(val);
    else if (baseMap.get(code)?.value !== val.value) changed.push(val);
  });
  baseMap.forEach((val, code) => {
    if (!nextMap.has(code)) removed.push(val);
  });
  return { added, removed, changed };
};

export const parseTemplateText = (text: string): OptionAssignment[] => {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const [codeStr, value] = line.split(':').map((p) => p.trim());
      return { optionCode: Number(codeStr), value, scope: 'global' as const };
    })
    .filter((item) => !Number.isNaN(item.optionCode));
};
