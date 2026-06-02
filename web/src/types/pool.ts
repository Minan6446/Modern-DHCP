export interface PoolSummary {
  id: string;
  name: string;
  version: 4 | 6;
  cidr: string;
  allocated: number;
  capacity: number;
  utilization: number;
  status: 'active' | 'disabled' | 'warning';
  vlanId?: number;
  tenantId?: string;
}

export interface PoolOverviewStats {
  total: number;
  enabledCount: number;
  avgUsage: number;
  highUsageCount: number;
}

export interface SubnetDraft {
  name: string;
  cidr: string;
  gateway?: string;
  option43?: string;
  dns?: string[];
  rangeStart?: string;
  rangeEnd?: string;
  leaseTime: number;
  maxLeaseTime?: number;
  exclude: string[];
  strategy: AllocationStrategy;
  vlanId?: number;
  location?: string;
  tags?: string[];
  leaseProfileId?: string;
}

export interface AllocationStrategy {
  mode: 'round-robin' | 'sequential' | 'random';
}

export interface PrefixDelegation {
  prefix: string;
  length: number;
  delegated: number;
  capacity: number;
}

export interface ConflictReport {
  poolId: string;
  cidr: string;
  conflicts: Array<{ withPool: string; cidr: string; reason: string }>;
}

export interface BindingConflict {
  id: string;
  poolId: string;
  ipAddress: string;
  identifier: string;
  identifierType: string;
  leaseProfileId?: string;
  note?: string;
  updatedAt?: string;
}

export interface BindingConflictResponse {
  count: number;
  items: BindingConflict[];
}

// Alias used by stores and list APIs
export type Pool = PoolSummary;
