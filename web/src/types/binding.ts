export type BindingStatus = 'online' | 'offline' | 'warning';

export interface Binding {
  id: string;
  identifier?: string;
  identifierType?: string;
  mac: string;
  ip: string;
  ipAddress?: string;
  hostname?: string;
  description?: string;
  options?: Record<string, string>;
  metadata?: Record<string, unknown>;
  poolId?: string;
  leaseId?: string;
  leaseProfileId?: string;
  lastSeenAt?: string;
  statusSource?: string;
  status: BindingStatus;
  clientId?: string;
  radiusUser?: string;
  serialNumber?: string;
  vendorAttrs?: Record<string, string>;
  createdAt?: string;
}

export interface BindingFilter {
  mac?: string;
  ip?: string;
  hostname?: string;
  status?: BindingStatus[];
  poolId?: string;
  keyword?: string;
  clientId?: string;
  radiusUser?: string;
  serialNumber?: string;
}

export interface BatchImportItem {
  mac: string;
  ip: string;
  hostname?: string;
  description?: string;
  valid: boolean;
  error?: string;
}
