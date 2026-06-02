import type { PageQuery } from './system';

// RFC 2131/2132 + custom extensions
export type LeaseState =
  | 'INIT'
  | 'SELECTING'
  | 'REQUESTING'
  | 'BOUND'
  | 'RENEWING'
  | 'REBINDING'
  | 'EXPIRED'
  | 'RELEASED'
  | 'DECLINED'
  | 'CONFLICT'
  | 'ABANDONED'
  | 'OFFLINE'
  // legacy/compat
  | 'RENEW';

export type LeaseStateGroup = 'active' | 'transient' | 'error' | 'other';

export interface Lease {
  id: string;
  ip: string;
  mac: string;
  clientId?: string;
  hostname?: string;
  poolId: string;
  poolName?: string;
  state: LeaseState;
  startsAt: string;
  endsAt: string;
  t1?: string;
  t2?: string;
  renewCount?: number;
  version: 4 | 6;
  vendorClass?: string;
  relayAgent?: string;
}

export interface LeaseEvent {
  id: string;
  leaseId: string;
  action: string;
  actor?: string;
  ts: string;
  meta?: Record<string, unknown>;
}

export interface LeaseFilter extends PageQuery {
  ip?: string;
  mac?: string;
  clientId?: string;
  poolId?: string;
  state?: LeaseState[];
  version?: 4 | 6;
  startTime?: string;
  endTime?: string;
  keyword?: string;
  tenantId?: string;
}

export interface LeaseInsights {
  generatedAt: string;
  pressureIndex: number;
  renewalRate: number;
  avgLeaseDurationHours: number;
  todayNewLeases: number;
  conflictIps: number;
  expiringNext24h: number;
  declines24h: number;
  stateBreakdown: Record<string, number>;
  vendorMix: Array<{ key: string; count: number }>;
  poolsAtRisk: Array<{
    poolId: string;
    poolName: string;
    tenantId: string;
    utilization: number;
    activeLeases: number;
    exhaustionEta?: string;
  }>;
}
