export interface TrustPort {
  id: string;
  device: string;
  port: string;
  vlan?: number;
  trusted: boolean;
  rateLimitPps?: number;
  lastUpdated?: string;
}

export interface SnoopingBinding {
  id: string;
  mac: string;
  ip: string;
  vlan?: number;
  port: string;
  leaseExpiry?: string;
  source: 'snooping' | 'static' | 'radius';
}

export interface ViolationPolicy {
  id?: string;
  name?: string;
  updatedAt?: string;
  action: 'drop' | 'shutdown' | 'alert' | 'rate-limit';
  blockDurationSeconds?: number;
  alertChannels?: string[];
}

export interface RateLimitRule {
  id: string;
  scope: 'port' | 'mac';
  target: string;
  limitPps: number;
  burst?: number;
  dynamic?: boolean;
  vlan?: number;
  status: 'active' | 'disabled';
}

export type MacListType = 'whitelist' | 'blacklist' | 'graylist';
export type MacListAction = 'allow' | 'block' | 'monitor';

export interface MacListEntry {
  id: string;
  tenantId?: string;
  mac: string;
  type: MacListType;
  action: MacListAction;
  description?: string;
  source?: string;
  priority?: number;
  enabled: boolean;
  validFrom?: string;
  validUntil?: string;
  metadata?: Record<string, unknown>;
  createdAt?: string;
  updatedAt?: string;
}

export interface RogueServerRecord {
  id: string;
  ip: string;
  mac: string;
  vlan?: number;
  detectedAt: string;
  severity: 'low' | 'medium' | 'high';
  actions?: string[];
}

export interface DAIConfig {
  enabled: boolean;
  validateMac: boolean;
  validateIp: boolean;
  rateLimitPps?: number;
}

export interface SourceGuardConfig {
  enabled: boolean;
  defaultAction: 'permit' | 'deny';
  exceptions: Array<{ mac: string; ip: string; vlan?: number; port?: string }>;
}

export interface PortSecurityProfile {
  id: string;
  name: string;
  maxMacs: number;
  sticky: boolean;
  shutdownOnViolation: boolean;
  agingMinutes?: number;
}

export interface Dot1xProfile {
  id: string;
  name: string;
  mode: 'single' | 'multi-auth';
  reauthInterval?: number;
  eapTypes?: string[];
}

export interface MacAuthProfile {
  id: string;
  name: string;
  authList: string;
  fallbackVlan?: number;
}

export interface RadiusMapping {
  id: string;
  attribute: string;
  localField: string;
  transform?: string;
}

export interface ThreatEvent {
  id: string;
  type: 'spoofing' | 'exhaustion' | 'rogue-server' | 'anomaly';
  sourceIp?: string;
  sourceMac?: string;
  port?: string;
  vlan?: number;
  score: number;
  occurredAt: string;
  description?: string;
}

export interface ThreatSpoofingRuleConfig {
  enabled: boolean;
  strictMode: boolean;
  blockThreshold: number;
  autoBlock: boolean;
}

export interface ThreatExhaustionRuleConfig {
  enabled: boolean;
  windowSeconds: number;
  attemptLimit: number;
  autoRateLimit: boolean;
}

export interface ThreatRogueRuleConfig {
  enabled: boolean;
  detectInterval: number;
  confidenceThreshold: number;
  autoQuarantine: boolean;
}

export interface ThreatRuleConfig {
  spoofing: ThreatSpoofingRuleConfig;
  exhaustion: ThreatExhaustionRuleConfig;
  rogue: ThreatRogueRuleConfig;
  updatedAt?: string;
}

export interface TopologyNode {
  id: string;
  label: string;
  type: 'core' | 'distribution' | 'access' | 'server' | 'rogue';
  status: 'normal' | 'warning' | 'critical';
}

export interface TopologyLink {
  source: string;
  target: string;
  status: 'up' | 'down' | 'degraded';
}
