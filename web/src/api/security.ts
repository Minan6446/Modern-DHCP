import type { ApiResponse, PageQuery, PageResult } from '@/types/system';
import type {
  TrustPort,
  SnoopingBinding,
  ViolationPolicy,
  RateLimitRule,
  RogueServerRecord,
  DAIConfig,
  SourceGuardConfig,
  PortSecurityProfile,
  Dot1xProfile,
  MacAuthProfile,
  RadiusMapping,
  ThreatEvent,
  ThreatRuleConfig,
  TopologyNode,
  TopologyLink,
  MacListEntry
} from '@/types/security';
import { httpClient } from '@/shared/api-client/http';

type RogueStoreRecord = RogueServerRecord & {
  tenantId?: string;
};

type TrustPortStoreRecord = TrustPort & {
  tenantId?: string;
};

type RateLimitStoreRecord = RateLimitRule & {
  tenantId?: string;
};

type ViolationPolicyStoreRecord = ViolationPolicy & {
  id: string;
  name: string;
  updatedAt: string;
  tenantId?: string;
};

const SECURITY_ROGUE_STORE_KEY = 'mdhcp.security.rogue.servers.v1';
const SECURITY_TRUST_PORT_STORE_KEY = 'mdhcp.security.trust-ports.v1';
const SECURITY_RATE_LIMIT_STORE_KEY = 'mdhcp.security.rate-limits.v1';
const SECURITY_VIOLATION_POLICY_STORE_KEY = 'mdhcp.security.violation-policy.v1';

const getBrowserStorage = () => {
  if (typeof window === 'undefined') return null;
  return window.localStorage;
};

const readRogueStore = (): RogueStoreRecord[] => {
  const storage = getBrowserStorage();
  if (!storage) return [];
  try {
    const raw = storage.getItem(SECURITY_ROGUE_STORE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed as RogueStoreRecord[];
  } catch {
    return [];
  }
};

const writeRogueStore = (records: RogueStoreRecord[]) => {
  const storage = getBrowserStorage();
  if (!storage) return;
  storage.setItem(SECURITY_ROGUE_STORE_KEY, JSON.stringify(records));
};

const nowISO = () => new Date().toISOString();

const readStoreList = <T>(key: string): T[] => {
  const storage = getBrowserStorage();
  if (!storage) return [];
  try {
    const raw = storage.getItem(key);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? (parsed as T[]) : [];
  } catch {
    return [];
  }
};

const writeStoreList = <T>(key: string, items: T[]) => {
  const storage = getBrowserStorage();
  if (!storage) return;
  storage.setItem(key, JSON.stringify(items));
};

const readStoreObject = <T>(key: string): T | null => {
  const storage = getBrowserStorage();
  if (!storage) return null;
  try {
    const raw = storage.getItem(key);
    if (!raw) return null;
    return JSON.parse(raw) as T;
  } catch {
    return null;
  }
};

const writeStoreObject = <T>(key: string, value: T) => {
  const storage = getBrowserStorage();
  if (!storage) return;
  storage.setItem(key, JSON.stringify(value));
};

const normalizeRogue = (record: Partial<RogueStoreRecord>): RogueStoreRecord => ({
  id: String(record.id || `rogue-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`),
  ip: String(record.ip || '').trim(),
  mac: String(record.mac || '').trim(),
  vlan: record.vlan,
  detectedAt: record.detectedAt || nowISO(),
  severity: (record.severity as RogueServerRecord['severity']) || 'medium',
  actions: Array.isArray(record.actions) ? record.actions : [],
  tenantId: record.tenantId
});

const normalizeTrustPort = (record: Partial<TrustPortStoreRecord>): TrustPortStoreRecord => ({
  id: String(record.id || `trust-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`),
  device: String(record.device || '').trim(),
  port: String(record.port || '').trim(),
  vlan: record.vlan,
  trusted: record.trusted !== false,
  rateLimitPps: record.rateLimitPps,
  lastUpdated: record.lastUpdated || nowISO(),
  tenantId: record.tenantId
});

const normalizeRateLimit = (record: Partial<RateLimitStoreRecord>): RateLimitStoreRecord => ({
  id: String(record.id || `rate-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`),
  scope: (record.scope as RateLimitRule['scope']) || 'port',
  target: String(record.target || '').trim(),
  limitPps: Number(record.limitPps || 0) || 0,
  burst: record.burst,
  dynamic: record.dynamic,
  vlan: record.vlan,
  status: (record.status as RateLimitRule['status']) || 'active',
  tenantId: record.tenantId
});

const normalizeViolationPolicy = (
  record: Partial<ViolationPolicyStoreRecord>
): ViolationPolicyStoreRecord => ({
  id: String(record.id || `vp-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`),
  name: String(record.name || '默认违规策略').trim() || '默认违规策略',
  updatedAt: record.updatedAt || nowISO(),
  action: (record.action as ViolationPolicy['action']) || 'alert',
  blockDurationSeconds: record.blockDurationSeconds ?? 0,
  alertChannels: Array.isArray(record.alertChannels) ? record.alertChannels : [],
  tenantId: record.tenantId
});

const readViolationPolicyStore = (): ViolationPolicyStoreRecord[] => {
  const list = readStoreList<ViolationPolicyStoreRecord>(SECURITY_VIOLATION_POLICY_STORE_KEY);
  if (list.length > 0) return list.map((item) => normalizeViolationPolicy(item));
  const legacy = readStoreObject<Record<string, ViolationPolicyStoreRecord>>(
    SECURITY_VIOLATION_POLICY_STORE_KEY
  );
  if (!legacy || Array.isArray(legacy)) return [];
  return Object.entries(legacy).map(([tenantId, policy]) =>
    normalizeViolationPolicy({ ...policy, tenantId: tenantId === '__global__' ? undefined : tenantId })
  );
};

const filterRoguesByTenant = (records: RogueStoreRecord[], tenantId?: string) => {
  if (!tenantId) return records;
  return records.filter((item) => !item.tenantId || item.tenantId === tenantId);
};

const emptyPage = <T>(params?: PageQuery) => ({
  data: {
    code: 0,
    data: {
      items: [] as T[],
      total: 0,
      page: params?.page ?? 1,
      pageSize: params?.pageSize ?? 10,
      hasMore: false
    }
  }
});

export const listTrustPorts = async (_params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<TrustPort[]>>('/core/security/trust-ports', { params: _params });
export const saveTrustPorts = async (ports: TrustPort[], params?: { tenantId?: string }) => {
  return httpClient.put<ApiResponse<void>>('/core/security/trust-ports', ports, { params });
};
export const listSnoopingBindings = async (params: PageQuery & { tenantId?: string }) =>
  emptyPage<SnoopingBinding>(params);
export const saveViolationPolicy = async (_policy: ViolationPolicy & { tenantId?: string }) =>
  httpClient.post<ApiResponse<ViolationPolicy>>('/core/security/violation-policies', _policy, {
    params: _policy.tenantId ? { tenantId: _policy.tenantId } : undefined
  });

export const listViolationPolicies = async (params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<ViolationPolicy[]>>('/core/security/violation-policies', { params });

export const deleteViolationPolicy = async (id: string, params?: { tenantId?: string }) =>
  httpClient.delete<ApiResponse<void>>(`/core/security/violation-policies/${id}`, { params });

export const listRateLimits = async (params: PageQuery & { tenantId?: string }) =>
  httpClient.get<ApiResponse<PageResult<RateLimitRule>>>('/core/security/rate-limits', { params });
export const saveRateLimit = async (payload: Partial<RateLimitRule> & { tenantId?: string }) => {
  return httpClient.post<ApiResponse<RateLimitRule>>('/core/security/rate-limits', payload, {
    params: payload.tenantId ? { tenantId: payload.tenantId } : undefined
  });
};
export const deleteRateLimit = async (id: string, params?: { tenantId?: string }) =>
  httpClient.delete<ApiResponse<void>>(`/core/security/rate-limits/${id}`, { params });

export const listRogueServers = async (params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<RogueServerRecord[]>>('/core/security/rogue-servers', { params });

export const createRogueServer = async (
  payload: Partial<RogueServerRecord> & { tenantId?: string }
) => {
  return httpClient.post<ApiResponse<RogueServerRecord>>('/core/security/rogue-servers', payload, {
    params: payload.tenantId ? { tenantId: payload.tenantId } : undefined
  });
};

export const updateRogueServer = async (
  id: string,
  payload: Partial<RogueServerRecord> & { tenantId?: string }
) => {
  return httpClient.put<ApiResponse<RogueServerRecord>>(`/core/security/rogue-servers/${id}`, payload, {
    params: payload.tenantId ? { tenantId: payload.tenantId } : undefined
  });
};

export const deleteRogueServer = async (id: string, params?: { tenantId?: string }) =>
  httpClient.delete<ApiResponse<void>>(`/core/security/rogue-servers/${id}`, { params });

export const quarantineRogue = async (id: string, payload?: { tenantId?: string }) => {
  return httpClient.post<ApiResponse<RogueServerRecord>>(
    `/core/security/rogue-servers/${id}/quarantine`,
    {},
    { params: payload }
  );
};

export const blockRogue = async (id: string, payload?: { tenantId?: string }) => {
  return httpClient.post<ApiResponse<RogueServerRecord>>(
    `/core/security/rogue-servers/${id}/block`,
    {},
    { params: payload }
  );
};

export const getDAIConfig = async (_params?: { tenantId?: string }) => ({
  data: { code: 0, data: {} as DAIConfig }
});
export const updateDAIConfig = async (_payload: Partial<DAIConfig> & { tenantId?: string }) => ({
  data: { code: 0, data: undefined }
});

export const getSourceGuard = async (_params?: { tenantId?: string }) => ({
  data: { code: 0, data: {} as SourceGuardConfig }
});
export const updateSourceGuard = async (
  _payload: Partial<SourceGuardConfig> & { tenantId?: string }
) => ({ data: { code: 0, data: undefined } });

export const listPortProfiles = async (_params?: { tenantId?: string }) => ({
  data: { code: 0, data: [] as PortSecurityProfile[] }
});
export const savePortProfile = async (
  _payload: Partial<PortSecurityProfile> & { tenantId?: string }
) => ({ data: { code: 0, data: {} as PortSecurityProfile } });
export const deletePortProfile = async (_id: string, _params?: { tenantId?: string }) => ({
  data: { code: 0, data: undefined }
});

export const listDot1xProfiles = async (_params?: { tenantId?: string }) => ({
  data: { code: 0, data: [] as Dot1xProfile[] }
});
export const saveDot1xProfile = async (_payload: Partial<Dot1xProfile>) => ({
  data: { code: 0, data: {} as Dot1xProfile }
});

export const listMacAuthProfiles = async (_params?: { tenantId?: string }) => ({
  data: { code: 0, data: [] as MacAuthProfile[] }
});
export const saveMacAuthProfile = async (_payload: Partial<MacAuthProfile>) => ({
  data: { code: 0, data: {} as MacAuthProfile }
});

export const listRadiusMappings = async (_params?: { tenantId?: string }) => ({
  data: { code: 0, data: [] as RadiusMapping[] }
});
export const saveRadiusMapping = async (_payload: Partial<RadiusMapping>) => ({
  data: { code: 0, data: {} as RadiusMapping }
});
export const deleteRadiusMapping = async (_id: string, _params?: { tenantId?: string }) => ({
  data: { code: 0, data: undefined }
});

export const listMacLists = async (
  params: PageQuery & {
    tenantId?: string;
    type?: string;
    action?: string;
    enabled?: boolean | string;
  }
) => httpClient.get<ApiResponse<PageResult<MacListEntry>>>('/core/security/mac-lists', { params });
export const createMacList = async (payload: Partial<MacListEntry> & { tenantId?: string }) => {
  const params = payload.tenantId ? { tenantId: payload.tenantId } : undefined;
  return httpClient.post<ApiResponse<MacListEntry>>('/core/security/mac-lists', payload, { params });
};
export const updateMacList = async (
  id: string,
  payload: Partial<MacListEntry> & { tenantId?: string }
) => {
  const params = payload.tenantId ? { tenantId: payload.tenantId } : undefined;
  return httpClient.patch<ApiResponse<MacListEntry>>(`/core/security/mac-lists/${id}`, payload, {
    params
  });
};
export const deleteMacList = async (id: string, params?: { tenantId?: string }) =>
  httpClient.delete<ApiResponse<void>>(`/core/security/mac-lists/${id}`, { params });

export const recordMacListAuditAction = async (
  payload: { action: 'security.mac_list.import' | 'security.mac_list.export'; count?: number } & { tenantId?: string }
) => {
  const params = payload.tenantId ? { tenantId: payload.tenantId } : undefined;
  return httpClient.post<ApiResponse<void>>('/core/security/mac-lists/audit-actions', payload, { params });
};

export const listThreatEvents = async (_params?: { tenantId?: string }) => ({
  data: { code: 0, data: [] as ThreatEvent[] }
});
export const getThreatRules = async (params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<ThreatRuleConfig>>('/core/security/threat-rules', { params });

export const updateThreatRules = async (
  payload: ThreatRuleConfig & { tenantId?: string }
) => {
  const params = payload.tenantId ? { tenantId: payload.tenantId } : undefined;
  const body: ThreatRuleConfig = {
    spoofing: payload.spoofing,
    exhaustion: payload.exhaustion,
    rogue: payload.rogue,
    updatedAt: payload.updatedAt
  };
  return httpClient.put<ApiResponse<ThreatRuleConfig>>('/core/security/threat-rules', body, {
    params
  });
};

export const getTopology = async (_params?: { tenantId?: string }) => ({
  data: { code: 0, data: { nodes: [] as TopologyNode[], links: [] as TopologyLink[] } }
});
