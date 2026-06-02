import { flushPromises, mount } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ClusterSyncAuditDr from './ClusterSyncAuditDr.vue';

const toastSpies = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  httpError: vi.fn()
}));

vi.mock('@/shared/errors/messageToast', () => ({
  showSuccess: toastSpies.success,
  showWarning: toastSpies.warning
}));

vi.mock('@/shared/errors/errorToast', () => ({
  showHttpError: toastSpies.httpError
}));

const mockState = vi.hoisted(() => ({
  joinJobs: {
    items: [
      {
        jobId: 'job-running',
        nodeId: 'node-b',
        peerAddress: 'https://10.0.0.2:8443',
        status: 'SNAPSHOTTING',
        progress: 20,
        createdAt: '2026-04-10T00:00:00Z',
        updatedAt: '2026-04-10T00:01:00Z',
        phases: [
          {
            phase: 'SNAPSHOTTING',
            status: 'RUNNING',
            startedAt: '2026-04-10T00:00:00Z'
          }
        ]
      },
      {
        jobId: 'job-failed',
        nodeId: 'node-c',
        peerAddress: 'https://10.0.0.3:8443',
        status: 'FAILED',
        progress: 85,
        createdAt: '2026-04-10T00:02:00Z',
        updatedAt: '2026-04-10T00:03:00Z',
        error: 'verify failed',
        phases: [
          {
            phase: 'VERIFYING',
            status: 'FAILED',
            startedAt: '2026-04-10T00:02:00Z',
            finishedAt: '2026-04-10T00:03:00Z',
            durationMs: 1000,
            error: 'verify failed'
          }
        ]
      }
    ],
    count: 2
  },
  joinJobDetail: {
    jobId: 'job-failed',
    nodeId: 'node-c',
    peerAddress: 'https://10.0.0.3:8443',
    status: 'FAILED',
    progress: 85,
    createdAt: '2026-04-10T00:02:00Z',
    updatedAt: '2026-04-10T00:03:00Z',
    error: 'verify failed',
    snapshot: {
      status: 'IMPORTED',
      capturedAt: '2026-04-10T00:02:20Z',
      sourceNode: 'node-a',
      sourceAddress: 'https://10.0.0.1:8443',
      poolCount: 12,
      bindingCount: 4,
      leaseCount: 87,
      poolChecksum: 'pool1234',
      bindingChecksum: 'bind5678',
      leaseChecksum: 'lease9999',
      consistencyChecksum: 'snapabc12',
      replicationOffset: 'binlog.000012:3000',
      lastSyncTxId: 'tx-snapshot-1',
      lastSyncType: 'config'
    },
    catchUp: {
      status: 'RUNNING',
      baselineOffset: 'binlog.000012:3000',
      targetOffset: 'binlog.000012:3456',
      currentOffset: 'binlog.000012:3200',
      startedAt: '2026-04-10T00:02:30Z',
      updatedAt: '2026-04-10T00:02:55Z',
      highWatermarkReached: false
    },
    checkpoint: {
      phase: 'VERIFYING',
      status: 'FAILED',
      lastCompletedPhase: 'CATCHING_UP',
      resumeCount: 2,
      resumable: true,
      updatedAt: '2026-04-10T00:03:00Z',
      lastError: 'verify failed'
    },
    verification: {
      status: 'FAILED',
      checkedAt: '2026-04-10T00:03:00Z',
      poolCount: 12,
      bindingCount: 4,
      leaseCount: 87,
      activeLeaseCount: 87,
      conflictLeaseCount24h: 2,
      createdLeaseCount24h: 21,
      fencingEpoch: 'epoch-42',
      replicationHealthy: false,
      replicationLagMs: 210,
      replicationMode: 'binlog',
      replicationSource: 'mysql-primary:3306/mysql-bin.000012',
      replicationState: 'disconnected',
      replicationOffset: 'binlog.000012:3456',
      lastSyncTxId: 'tx-verify-1',
      lastSyncType: 'lease',
      consistencyChecksum: 'abc12345',
      failureReason: 'verify failed'
    },
    phases: [
      {
        phase: 'VERIFYING',
        status: 'FAILED',
        startedAt: '2026-04-10T00:02:00Z',
        finishedAt: '2026-04-10T00:03:00Z',
        durationMs: 1000,
        error: 'verify failed'
      }
    ]
  }
}));

vi.mock('@/api/cluster', () => ({
  getClusterSyncStatus: vi.fn(async () => []),
  getClusterSyncStats: vi.fn(async () => ({
    windowHours: 24,
    total: 0,
    committed: 0,
    failed: 0,
    ackTimeoutCount: 0,
    successRatePercent: 100,
    avgAckLatencyMs: 0,
    p95AckLatencyMs: 0
  })),
  getClusterSyncTransactions: vi.fn(async () => []),
  getClusterScaleEvents: vi.fn(async () => []),
  getClusterFailoverHistory: vi.fn(async () => []),
  getClusterFailoverPlans: vi.fn(async () => ({ items: [], count: 0 })),
  getClusterFailoverPlan: vi.fn(async () => null),
  createClusterFailoverPlan: vi.fn(async () => ({ planId: 'plan-1' })),
  getClusterBackupPlan: vi.fn(async () => ({ freq: 'daily', retain: '30', storage: 'local', contents: ['配置数据'] })),
  getClusterBackupHistory: vi.fn(async () => []),
  saveClusterBackupPlan: vi.fn(async () => ({})),
  runClusterBackup: vi.fn(async () => ({})),
  runClusterRestore: vi.fn(async () => ({})),
  triggerClusterSync: vi.fn(async () => ({})),
  retryClusterSyncTransaction: vi.fn(async () => ({})),
  getClusterSyncTransaction: vi.fn(async () => null),
  getClusterJoinJobs: vi.fn(async () => mockState.joinJobs),
  getClusterJoinJob: vi.fn(async () => mockState.joinJobDetail),
  cancelClusterJoinJob: vi.fn(async () => undefined),
  retryClusterJoinJob: vi.fn(async () => mockState.joinJobDetail)
}));

describe('cluster phase2 join jobs', () => {
  beforeEach(() => {
    toastSpies.success.mockReset();
    toastSpies.warning.mockReset();
    toastSpies.httpError.mockReset();
  });

  it('renders join jobs and shows status-driven actions', async () => {
    const wrapper = mount(ClusterSyncAuditDr, {
      attachTo: document.body
    });

    await flushPromises();

    expect(wrapper.text()).toContain('节点接入编排');
    expect(wrapper.text()).toContain('job-running');
    expect(wrapper.text()).toContain('job-failed');
    expect(wrapper.text()).toContain('进行中 1');
    expect(wrapper.text()).toContain('失败 1');
    expect(wrapper.text()).toContain('快照中');
    expect(wrapper.text()).toContain('失败');

    const text = wrapper.text();
    expect(text).toContain('取消');
    expect(text).toContain('重试');

    wrapper.unmount();
  });

  it('shows structured verification summary in detail drawer', async () => {
    const wrapper = mount(ClusterSyncAuditDr, {
      attachTo: document.body
    });

    await flushPromises();

    const detailButtons = wrapper.findAll('button').filter((button) => button.text() === '详情');
    await detailButtons[1]?.trigger('click');
    await flushPromises();

    const text = wrapper.text();
    expect(text).toContain('校验结论');
    expect(text).toContain('校验失败');
    expect(text).toContain('Pool 数');
    expect(text).toContain('12');
    expect(text).toContain('binlog');
    expect(text).toContain('mysql-primary:3306/mysql-bin.000012');
    expect(text).toContain('disconnected');
    expect(text).toContain('binlog.000012:3456');
    expect(text).toContain('tx-verify-1');
    expect(text).toContain('快照状态');
    expect(text).toContain('snapabc12');
    expect(text).toContain('Target Offset');
    expect(text).toContain('binlog.000012:3200');
    expect(text).toContain('检查点阶段');
    expect(text).toContain('可恢复');
    expect(text).toContain('恢复次数');

    wrapper.unmount();
  });
});