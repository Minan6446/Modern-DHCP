import { httpClient } from '@/shared/api-client/http';
import type { ApiResponse } from '@/types/system';

export interface ISCImportResult {
  subnets: Array<{
    cidr: string;
    gateway?: string;
    dns?: string[];
    rangeStart?: string;
    rangeEnd?: string;
    ranges?: Array<{ start: string; end: string }>;
    exclusions?: string[];
    options?: Record<string, string>;
  }>;
  reservations: Array<{ hostname?: string; mac?: string; ip?: string }>;
  deniedHosts?: Array<{ hostname?: string; mac?: string; reason?: string }>;
  warnings: string[];
}

export interface MigrationSelectionSubnet {
  id: string;
  cidr: string;
  gateway?: string;
  parseStatus: 'ok' | 'error' | 'conflict';
  parseReason?: string;
}

export interface MigrationSelectionReservation {
  id: string;
  hostname?: string;
  mac?: string;
  ip?: string;
  parseStatus: 'ok' | 'error' | 'conflict';
  parseReason?: string;
}

export interface MigrationSelectionDeniedHost {
  id: string;
  hostname?: string;
  mac?: string;
  reason?: string;
  parseStatus: 'ok' | 'error' | 'conflict';
  parseReason?: string;
}

export interface MigrationValidatePayload {
  subnets: MigrationSelectionSubnet[];
  reservations: MigrationSelectionReservation[];
  deniedHosts: MigrationSelectionDeniedHost[];
}

export interface MigrationValidateResult {
  pass: boolean;
  total: number;
  invalid: number;
  conflict: number;
  details?: string[];
}

export interface MigrationImportResult {
  importedSubnets: number;
  importedReservations: number;
  importedDeniedHosts: number;
  rollbackToken: string;
  report: string;
}

export const importIscDhcp = (file: File) => {
  const form = new FormData();
  form.append('file', file);
  return httpClient.post<ApiResponse<ISCImportResult>>('/core/tools/isc/import', form, {
    headers: { 'Content-Type': 'multipart/form-data' }
  });
};

export const validateMigrationSelection = (payload: MigrationValidatePayload) =>
  httpClient.post<ApiResponse<MigrationValidateResult>>('/core/tools/migration/validate', payload);

export const startMigrationImport = (payload: MigrationValidatePayload) =>
  httpClient.post<ApiResponse<MigrationImportResult>>('/core/tools/migration/import', payload);

export const rollbackMigrationImport = (rollbackToken: string) =>
  httpClient.post<ApiResponse<{ rolledBack: boolean; message: string }>>('/core/tools/migration/rollback', {
    rollbackToken
  });
