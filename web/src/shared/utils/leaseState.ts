import type { LeaseState, LeaseStateGroup } from '@/types/lease';

export interface LeaseStateMeta {
  label: string;
  desc: string;
  group: LeaseStateGroup;
  color: 'success' | 'warning' | 'danger' | 'info' | '';
  icon?: string;
}

const stateMap: Record<LeaseState, LeaseStateMeta> = {
  INIT: {
    label: 'INIT',
    desc: '客户端初始化，尚未发送 Discover',
    group: 'transient',
    color: 'info'
  },
  SELECTING: {
    label: 'SELECTING',
    desc: '客户端选择服务器中（收到多个 Offer）',
    group: 'transient',
    color: 'info'
  },
  REQUESTING: {
    label: 'REQUESTING',
    desc: '客户端发送 Request 等待 ACK',
    group: 'transient',
    color: 'warning'
  },
  BOUND: { label: 'BOUND', desc: '租约已绑定，正常使用中', group: 'active', color: 'success' },
  RENEWING: {
    label: 'RENEWING',
    desc: 'T1 续约中，向原服务器请求',
    group: 'active',
    color: 'success'
  },
  REBINDING: {
    label: 'REBINDING',
    desc: 'T2 重绑定中，向任意服务器请求',
    group: 'active',
    color: 'warning'
  },
  EXPIRED: { label: 'EXPIRED', desc: '租约已过期未续约', group: 'other', color: '' },
  RELEASED: { label: 'RELEASED', desc: '租约被主动释放', group: 'other', color: '' },
  DECLINED: {
    label: 'DECLINED',
    desc: '客户端拒绝租约（地址冲突）',
    group: 'error',
    color: 'danger'
  },
  CONFLICT: { label: 'CONFLICT', desc: '检测到地址冲突', group: 'error', color: 'danger' },
  ABANDONED: { label: 'ABANDONED', desc: '异常废弃的租约', group: 'error', color: 'danger' },
  OFFLINE: { label: 'OFFLINE', desc: '自定义：离线或不可用', group: 'other', color: 'info' },
  RENEW: {
    label: 'RENEW',
    desc: '兼容旧值，将映射为 RENEWING/REBINDING',
    group: 'active',
    color: 'success'
  }
};

const known = new Set<LeaseState>(Object.keys(stateMap) as LeaseState[]);

/**
 * 标准化租约状态，保持向后兼容。未知状态将原样大写，如果仍未识别则返回 OFFLINE。
 */
export const normalizeLeaseState = (raw: string | null | undefined): LeaseState => {
  if (!raw) return 'OFFLINE';
  const upper = raw.toUpperCase();
  if (upper === 'RENEW') return 'RENEWING';
  if (known.has(upper as LeaseState)) return upper as LeaseState;
  return 'OFFLINE';
};

export const leaseStateMeta = (state: LeaseState): LeaseStateMeta => {
  return stateMap[state] || stateMap.OFFLINE;
};

export const isActiveLease = (state: LeaseState) =>
  state === 'BOUND' || state === 'RENEWING' || state === 'REBINDING';
export const isTransientState = (state: LeaseState) =>
  state === 'INIT' || state === 'SELECTING' || state === 'REQUESTING';
export const isErrorState = (state: LeaseState) =>
  state === 'DECLINED' || state === 'CONFLICT' || state === 'ABANDONED';

export const leaseStates = Object.keys(stateMap) as LeaseState[];

export const groupedLeaseStates = () => {
  const groups: Record<LeaseStateGroup, LeaseState[]> = {
    active: [],
    transient: [],
    error: [],
    other: []
  };
  leaseStates.forEach((state) => {
    groups[stateMap[state].group].push(state);
  });
  return groups;
};
