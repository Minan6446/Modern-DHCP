import { flushPromises, mount, type VueWrapper } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ClusterConfigManagement from './ClusterConfigManagement.vue';
import ClusterRuntimeOverview from './ClusterRuntimeOverview.vue';

const routerPush = vi.fn();

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: routerPush })
}));

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

const mockState = vi.hoisted(() => {
  const overview = {
    mode: 'active-passive',
    dhcpRole: 'primary',
    dhcpHealth: 'healthy',
    failoverReady: true,
    replicationLagMs: 12,
    writeGateOpen: true,
    pendingActions: 0,
    fencingEpoch: '1',
    mysqlRole: 'primary',
    mysqlHealth: 'healthy',
    mysqlLagMs: 12,
    redisRole: 'replica',
    redisHealth: 'healthy',
    redisOffsetLag: 3,
    nodes: [
      {
        id: 'node-a',
        role: 'active',
        address: '10.0.0.1',
        hasAuth: true,
        version: 'test',
        health: 'healthy',
        cpuPercent: 12,
        memoryPercent: 18,
        activeLeasesRedis: 33,
        totalLeasesMySQL: 35,
        syncLagMs: 0,
        lastHeartbeat: '2026-04-10T00:00:00Z'
      },
      {
        id: 'node-b',
        role: 'standby',
        address: '10.0.0.2',
        hasAuth: false,
        version: 'test',
        health: 'healthy',
        cpuPercent: 8,
        memoryPercent: 15,
        activeLeasesRedis: 12,
        totalLeasesMySQL: 14,
        syncLagMs: 5,
        lastHeartbeat: '2026-04-10T00:00:00Z'
      }
    ]
  };

  const control = {
    initialized: false,
    clusterDomain: '',
    primaryNodeId: '',
    primaryNodeUrl: '',
    primaryNodeIpAddresses: [],
    heartbeatIntervalSeconds: 5,
    heartbeatRetryIntervalSeconds: 15,
    configRefreshIntervalSeconds: 30,
    configRetryIntervalSeconds: 10,
    configVersion: 0,
    requireTls: true,
    apiTlsEnabled: false,
    apiTlsActive: false,
    apiTlsCertFile: '',
    apiTlsClientCaFile: '',
    restartRequired: false,
    nodes: [] as Array<Record<string, unknown>>
  };

  return {
    haConfig: {
      mode: 'active-passive',
      primary: 'node-a',
      standbyNodes: ['node-b'],
      loadBalancing: { enabled: false },
      failover: { intervalMs: 5000, timeoutMs: 15000, failureThreshold: 3 },
      replication: { mechanism: 'bndupd-bndack', snapshotIntervalSec: 30, lagAlertMs: 150, syncMode: 'strong' },
      splitBrain: { strategy: 'priority-lock' },
      recovery: { manualAction: 'manual-ack' }
    },
    overview,
    control,
    commands: { items: [] as Array<Record<string, unknown>>, count: 0 },
    joinJobs: { items: [] as Array<Record<string, unknown>>, count: 0 },
    membershipEvents: { items: [] as Array<Record<string, unknown>> },
    syncStatus: [] as Array<Record<string, unknown>>,
    failoverHistory: [] as Array<Record<string, unknown>>
  };
});

const deepCopy = <T>(value: T): T => JSON.parse(JSON.stringify(value));

vi.mock('@/api/cluster', () => ({
  getClusterHaConfig: vi.fn(async () => deepCopy(mockState.haConfig)),
  getClusterOverview: vi.fn(async () => deepCopy(mockState.overview)),
  getHAMembershipEvents: vi.fn(async () => deepCopy(mockState.membershipEvents)),
  getClusterControl: vi.fn(async () => deepCopy(mockState.control)),
  getClusterCommands: vi.fn(async () => deepCopy(mockState.commands)),
  getClusterJoinJobs: vi.fn(async () => deepCopy(mockState.joinJobs)),
  getClusterSyncStatus: vi.fn(async () => deepCopy(mockState.syncStatus)),
  getClusterFailoverHistory: vi.fn(async () => deepCopy(mockState.failoverHistory)),
  getClusterMemberDetail: vi.fn(async (memberId: string) => ({
    node: deepCopy((mockState.control.nodes as Array<Record<string, any>>).find((item) => item.id === memberId)),
    runtimeNode: deepCopy((mockState.overview.nodes as Array<Record<string, any>>).find((item) => item.id === memberId)),
    latestJoinJob: deepCopy((mockState.joinJobs.items as Array<Record<string, any>>).find((item) => item.nodeId === memberId)),
    recentCommands: deepCopy(
      (mockState.commands.items as Array<Record<string, any>>).filter((item) => item.targetNodeId === memberId).slice(0, 10)
    )
  })),
  triggerClusterSync: vi.fn(async () => ({ id: 'evt-1', txId: 'tx-1', title: 'sync', detail: 'sync', time: '2026-04-10T00:00:00Z', status: 'success', type: 'info' })),
  saveClusterHaConfig: vi.fn(async () => ({})),
  createClusterNode: vi.fn(async () => ({})),
  updateClusterNode: vi.fn(async () => ({})),
  deleteClusterNode: vi.fn(async () => ({})),
  createClusterCommand: vi.fn(async (payload: Record<string, any>) => {
    const command = {
      commandId: `cmd-${payload.commandType}-${payload.targetNodeId}`,
      commandType: payload.commandType,
      targetNodeId: payload.targetNodeId,
      requestedBy: 'tester',
      status: 'COMPLETED',
      payload: payload.payload || {},
      requestedAt: '2026-04-10T00:06:00Z',
      updatedAt: '2026-04-10T00:06:01Z',
      completedAt: '2026-04-10T00:06:01Z',
      receipts: [
        {
          nodeId: payload.targetNodeId,
          status: 'COMPLETED',
          detail: payload.commandType === 'leave' ? '成员已安全退出并完成控制面清理' : '已完成成员健康与复制状态校验',
          ackedAt: '2026-04-10T00:06:01Z',
          updatedAt: '2026-04-10T00:06:01Z',
          completedAt: '2026-04-10T00:06:01Z'
        }
      ]
    };
    mockState.commands = {
      items: [command, ...(mockState.commands.items || [])],
      count: (mockState.commands.count || 0) + 1
    };
    return deepCopy(command);
  }),
  leaveClusterMember: vi.fn(async (memberId: string) => {
    const command = {
      commandId: `cmd-leave-${memberId}`,
      commandType: 'leave',
      targetNodeId: memberId,
      requestedBy: 'tester',
      status: 'COMPLETED',
      requestedAt: '2026-04-10T00:07:00Z',
      updatedAt: '2026-04-10T00:07:01Z',
      completedAt: '2026-04-10T00:07:01Z',
      receipts: [
        {
          nodeId: memberId,
          status: 'COMPLETED',
          detail: '成员已安全退出并完成控制面清理',
          ackedAt: '2026-04-10T00:07:01Z',
          updatedAt: '2026-04-10T00:07:01Z',
          completedAt: '2026-04-10T00:07:01Z'
        }
      ]
    };
    mockState.control = {
      ...mockState.control,
      nodes: (mockState.control.nodes as Array<Record<string, any>>).filter((item) => item.id !== memberId)
    };
    mockState.overview = {
      ...mockState.overview,
      nodes: ((mockState.overview.nodes as unknown as Array<Record<string, any>>).filter((item) => item.id !== memberId) as typeof mockState.overview.nodes)
    };
    mockState.commands = {
      items: [command, ...(mockState.commands.items || [])],
      count: (mockState.commands.count || 0) + 1
    };
    return { removed: true, command, cluster: deepCopy(mockState.control) };
  }),
  deleteClusterControl: vi.fn(async () => {
    mockState.control = {
      ...mockState.control,
      initialized: false,
      nodes: [],
      restartRequired: false,
      apiTlsEnabled: false,
      apiTlsActive: false,
      configVersion: 0
    };
    return { deleted: true, cluster: deepCopy(mockState.control) };
  }),
  initializeClusterControl: vi.fn(async (payload: Record<string, any>) => {
    mockState.control = {
      initialized: true,
      clusterDomain: payload.clusterDomain,
      primaryNodeId: payload.primaryNodeId || 'node-a',
      primaryNodeUrl: payload.primaryNodeUrl,
      primaryNodeIpAddresses: payload.primaryNodeIpAddresses,
      heartbeatIntervalSeconds: payload.heartbeatIntervalSeconds || 5,
      heartbeatRetryIntervalSeconds: payload.heartbeatRetryIntervalSeconds || 15,
      configRefreshIntervalSeconds: payload.configRefreshIntervalSeconds || 30,
      configRetryIntervalSeconds: payload.configRetryIntervalSeconds || 10,
      configVersion: 1,
      requireTls: true,
      apiTlsEnabled: true,
      apiTlsActive: false,
      apiTlsCertFile: 'data/cluster/pki/api-server-cert.pem',
      apiTlsClientCaFile: '',
      restartRequired: true,
      nodes: [
        {
          id: payload.primaryNodeId || 'node-a',
          name: payload.primaryNodeId || 'node-a',
          url: payload.primaryNodeUrl,
          ipAddresses: payload.primaryNodeIpAddresses,
          type: 'primary',
          state: 'self',
          version: 1,
          lastSeen: '2026-04-10T00:00:00Z'
        }
      ]
    };
    return {
      clusterToken: 'clt_test_token',
      cluster: deepCopy(mockState.control),
      tlsMaterialGenerated: true,
      restartRequired: true
    };
  }),
  joinClusterControl: vi.fn(async (payload: Record<string, any>) => {
    mockState.control = {
      ...mockState.control,
      nodes: [
        ...mockState.control.nodes,
        {
          id: payload.secondaryNodeId,
          name: payload.secondaryNodeName || payload.secondaryNodeId,
          url: payload.secondaryNodeUrl,
          ipAddresses: payload.secondaryNodeIpAddresses,
          type: 'secondary',
          state: 'connected',
          version: mockState.control.configVersion,
          lastSeen: '2026-04-10T00:05:00Z'
        }
      ]
    };
    mockState.commands = {
      items: [
        {
          commandId: 'cmd-verify-node-b',
          commandType: 'verify',
          targetNodeId: payload.secondaryNodeId,
          requestedBy: 'tester',
          status: 'COMPLETED',
          requestedAt: '2026-04-10T00:05:31Z',
          updatedAt: '2026-04-10T00:05:32Z',
          completedAt: '2026-04-10T00:05:32Z',
          receipts: [
            {
              nodeId: payload.secondaryNodeId,
              status: 'COMPLETED',
              detail: '已完成成员健康与复制状态校验',
              ackedAt: '2026-04-10T00:05:32Z',
              updatedAt: '2026-04-10T00:05:32Z',
              completedAt: '2026-04-10T00:05:32Z'
            }
          ]
        }
      ],
      count: 1
    };
    mockState.joinJobs = {
      items: [
        {
          jobId: 'job-node-b',
          nodeId: payload.secondaryNodeId,
          peerAddress: payload.secondaryNodeUrl,
          status: 'WARM_STANDBY',
          progress: 100,
          createdAt: '2026-04-10T00:05:00Z',
          updatedAt: '2026-04-10T00:05:30Z',
          snapshot: {
            status: 'IMPORTED',
            capturedAt: '2026-04-10T00:05:10Z',
            sourceNode: 'node-a',
            sourceAddress: 'https://10.0.0.1:8443',
            poolCount: 5,
            bindingCount: 3,
            leaseCount: 14,
            poolChecksum: 'pool1111',
            bindingChecksum: 'bind2222',
            leaseChecksum: 'lease3333',
            consistencyChecksum: 'snap12345',
            replicationOffset: 'binlog.000021:1024',
            lastSyncTxId: 'tx-snapshot-1',
            lastSyncType: 'config'
          },
          catchUp: {
            status: 'HIGH_WATER_REACHED',
            baselineOffset: 'binlog.000021:1000',
            targetOffset: 'binlog.000021:1024',
            currentOffset: 'binlog.000021:1024',
            updatedAt: '2026-04-10T00:05:25Z',
            completedAt: '2026-04-10T00:05:26Z',
            highWatermarkReached: true
          },
          checkpoint: {
            phase: 'VERIFYING',
            status: 'SUCCESS',
            lastCompletedPhase: 'VERIFYING',
            resumeCount: 1,
            resumable: false,
            updatedAt: '2026-04-10T00:05:30Z'
          },
          verification: {
            status: 'VERIFIED',
            checkedAt: '2026-04-10T00:05:30Z',
            poolCount: 5,
            bindingCount: 3,
            leaseCount: 14,
            activeLeaseCount: 14,
            conflictLeaseCount24h: 0,
            createdLeaseCount24h: 6,
            fencingEpoch: '1',
            replicationHealthy: true,
            replicationLagMs: 12,
            replicationMode: 'binlog',
            replicationSource: 'mysql-primary:3306/mysql-bin.000021',
            replicationState: 'applying',
            replicationOffset: 'binlog.000021:1024',
            lastSyncTxId: 'tx-join-1',
            lastSyncType: 'lease',
            consistencyChecksum: 'abc12345'
          },
          phases: [
            {
              phase: 'VERIFYING',
              status: 'SUCCESS',
              startedAt: '2026-04-10T00:05:00Z',
              finishedAt: '2026-04-10T00:05:30Z',
              durationMs: 30000
            }
          ]
        }
      ],
      count: 1
    };
    return {
      nodeId: payload.secondaryNodeId,
      nodeToken: 'ndt_test_token',
      joinJobId: 'job-node-b',
      cluster: deepCopy(mockState.control)
    };
  })
}));

const findButtonByText = (wrapper: VueWrapper<any>, text: string) => {
  const button = wrapper.findAll('button').find((item) => item.text().includes(text));
  if (!button) {
    throw new Error(`button not found: ${text}`);
  }
  return button;
};

describe('cluster phase1 pages', () => {
  beforeEach(() => {
    routerPush.mockReset();
    toastSpies.success.mockReset();
    toastSpies.warning.mockReset();
    toastSpies.httpError.mockReset();

    mockState.control = {
      initialized: false,
      clusterDomain: '',
      primaryNodeId: '',
      primaryNodeUrl: '',
      primaryNodeIpAddresses: [],
      heartbeatIntervalSeconds: 5,
      heartbeatRetryIntervalSeconds: 15,
      configRefreshIntervalSeconds: 30,
      configRetryIntervalSeconds: 10,
      configVersion: 0,
      requireTls: true,
      apiTlsEnabled: false,
      apiTlsActive: false,
      apiTlsCertFile: '',
      apiTlsClientCaFile: '',
      restartRequired: false,
      nodes: []
    };
    mockState.joinJobs = { items: [], count: 0 };
    mockState.commands = { items: [], count: 0 };
  });

  it('covers initialize -> join -> token display -> overview mapping', async () => {
    const configWrapper = mount(ClusterConfigManagement, {
      attachTo: document.body
    });

    await flushPromises();

    await configWrapper.get('input[placeholder="例如 cluster.local"]').setValue('cluster.example.internal');
    await configWrapper.get('input[placeholder="例如 node-a"]').setValue('node-a');
    await configWrapper.get('input[placeholder="例如 https://10.10.1.10:8443"]').setValue('https://10.0.0.1:8443');
    await configWrapper.get('textarea[placeholder="多个 IP 用逗号、空格或换行分隔"]').setValue('10.0.0.1');

    await findButtonByText(configWrapper, '初始化控制面').trigger('click');
    await flushPromises();

    expect(configWrapper.text()).toContain('clt_test_token');
    expect(configWrapper.text()).toContain('待重启生效');

    await findButtonByText(configWrapper, '接入 Secondary').trigger('click');
    await flushPromises();

    const setupState = (configWrapper.vm as any).$?.setupState as Record<string, any>;
    if (!setupState?.joinForm || typeof setupState.submitJoin !== 'function') {
      throw new Error('join form bindings not exposed in setup state');
    }

    setupState.joinForm.secondaryNodeId = 'node-b';
    setupState.joinForm.secondaryNodeName = 'node-b-standby';
    setupState.joinForm.secondaryNodeUrl = 'https://10.0.0.2:8443';
    setupState.joinForm.secondaryNodeIpAddressesText = '10.0.0.2';
    setupState.joinForm.clusterToken = 'clt_test_token';
    await flushPromises();

    await setupState.submitJoin();
    await flushPromises();

    expect(configWrapper.text()).toContain('ndt_test_token');
    expect(configWrapper.text()).toContain('node-b');
    expect(configWrapper.text()).toContain('命令中心');
    expect(configWrapper.text()).toContain('已完成成员健康与复制状态校验');

    const setupStateAfterJoin = (configWrapper.vm as any).$?.setupState as Record<string, any>;
    setupStateAfterJoin.commandForm.commandType = 'verify';
    setupStateAfterJoin.commandForm.targetNodeId = 'node-b';
    await setupStateAfterJoin.submitClusterCommand();
    await flushPromises();

    expect(configWrapper.text()).toContain('校验');

    await setupStateAfterJoin.openMemberDetail('node-b');
    await flushPromises();

    expect(configWrapper.text()).toContain('成员详情');
    expect(configWrapper.text()).toContain('最近接入任务');
    expect(configWrapper.text()).toContain('最近命令');

    const overviewWrapper = mount(ClusterRuntimeOverview);
    await flushPromises();

    expect(overviewWrapper.text()).toContain('主控 / 本机');
    expect(overviewWrapper.text()).toContain('从控 / 已连接');
    expect(overviewWrapper.text()).toContain('https://10.0.0.2:8443');
    expect(overviewWrapper.text()).toContain('预热完成');

    const detailButtons = overviewWrapper.findAll('button').filter((item) => item.text().includes('查看详情'));
    await detailButtons[1]?.trigger('click');
    await flushPromises();

    expect(overviewWrapper.text()).toContain('校验通过');
    expect(overviewWrapper.text()).toContain('binlog');
    expect(overviewWrapper.text()).toContain('mysql-primary:3306/mysql-bin.000021');
    expect(overviewWrapper.text()).toContain('applying');
    expect(overviewWrapper.text()).toContain('tx-join-1');
    expect(overviewWrapper.text()).toContain('Checksum abc12345');
    expect(overviewWrapper.text()).toContain('接入快照');
    expect(overviewWrapper.text()).toContain('snap12345');
    expect(overviewWrapper.text()).toContain('追平进度');
    expect(overviewWrapper.text()).toContain('HIGH_WATER_REACHED');
    expect(overviewWrapper.text()).toContain('恢复检查点');
    expect(overviewWrapper.text()).toContain('已恢复 1 次');

    configWrapper.unmount();
    overviewWrapper.unmount();
  });
});