<template>
  <div class="page surface-card login-audit">
    <div class="header-row">
      <div>
        <h3>{{ t('system.audit.pageTitle') }}</h3>
        <p class="subtitle">{{ t('system.audit.pageSubtitle') }}</p>
      </div>
    </div>

    <AuditStats :rows="rows" :is-risk-row="isRiskRow" />

    <div class="filter-bar">
      <div class="bar-left">
        <el-radio-group v-model="timePreset" size="small" @change="onTimePresetChange">
          <el-radio-button label="24h">{{ t('system.audit.preset24h') }}</el-radio-button>
          <el-radio-button label="7d">{{ t('system.audit.preset7d') }}</el-radio-button>
          <el-radio-button label="30d">{{ t('system.audit.preset30d') }}</el-radio-button>
          <el-radio-button label="custom">{{ t('system.audit.presetCustom') }}</el-radio-button>
        </el-radio-group>

        <el-date-picker
          v-if="timePreset === 'custom'"
          v-model="range"
          type="datetimerange"
          range-separator="-"
          :start-placeholder="t('system.audit.startTime')"
          :end-placeholder="t('system.audit.endTime')"
          :unlink-panels="true"
          class="filter-item range-item"
          value-format="YYYY-MM-DD HH:mm:ss"
        />

        <el-input v-model="filters.username" :placeholder="t('system.audit.username')" clearable class="filter-item w-160" />
        <el-input v-model="filters.ip" :placeholder="t('system.audit.ip')" clearable class="filter-item w-150" />
        <el-select v-model="filters.result" clearable class="filter-item w-130" :placeholder="t('system.audit.result')">
          <el-option :label="t('system.audit.success')" value="success" />
          <el-option :label="t('system.audit.failed')" value="failed" />
          <el-option :label="t('system.audit.risk')" value="risk" />
        </el-select>
        <el-select v-model="filters.operationType" clearable class="filter-item w-160" :placeholder="t('system.audit.opType')">
          <el-option v-for="item in operationTypeOptions" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
        <el-input v-model="filters.resourceKeyword" :placeholder="t('system.audit.resourceName')" clearable class="filter-item w-180" />
        <el-input v-model="filters.idKeyword" :placeholder="t('system.audit.idSearch')" clearable class="filter-item w-180" />
      </div>

      <div class="bar-right">
        <el-button :disabled="loading" @click="resetFilters">{{ t('system.audit.resetBtn') }}</el-button>
        <el-button :loading="loading" @click="refresh">{{ t('system.audit.refreshBtn') }}</el-button>
        <el-button type="primary" :loading="exporting" @click="exportAuditLogs">{{ t('system.audit.exportBtn') }}</el-button>
      </div>
    </div>

    <div class="bulk-row">
      <span>{{ t('system.audit.selectedN', { n: selectedRows.length }) }}</span>
      <el-button plain size="small" :disabled="!selectedRows.length || exporting" @click="exportSelected">{{ t('system.audit.exportSelected') }}</el-button>
      <el-button plain size="small" :disabled="!selectedRows.length" @click="clearSelection">{{ t('system.audit.clearSelection') }}</el-button>
    </div>

    <div class="table-wrapper">
      <el-table
        ref="tableRef"
        v-loading="loading"
        :data="displayRows"
        border
        stripe
        highlight-current-row
        class="data-table"
        @selection-change="onSelectionChange"
        @sort-change="onSortChange"
      >
        <el-table-column type="selection" width="52" />
        <el-table-column prop="createdAt" :label="t('system.audit.time')" width="180" sortable="custom">
          <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column :label="t('system.audit.colOpType')" min-width="150">
          <template #default="{ row }">
            <el-tag size="small" type="info">{{ operationTypeLabel(row) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('system.audit.colResource')" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <a v-if="row.resourceLink" class="resource-link" @click.prevent="openResource(row)">{{ resourceNameLabel(row) }}</a>
            <span v-else>{{ resourceNameLabel(row) }}</span>
            <div class="sub" v-if="row.resourceId">ID: <span class="mono">{{ row.resourceId }}</span></div>
          </template>
        </el-table-column>
        <el-table-column prop="username" :label="t('system.audit.username')" min-width="130">
          <template #default="{ row }">{{ normalizedUsername(row.username) }}</template>
        </el-table-column>
        <el-table-column prop="ip" :label="t('system.audit.ip')" min-width="150">
          <template #default="{ row }">
            <div class="mono">{{ row.ip || '-' }}</div>
            <div v-if="row.location" class="sub">{{ row.location }}</div>
          </template>
        </el-table-column>
        <el-table-column prop="result" :label="t('system.audit.result')" width="110">
          <template #default="{ row }">
            <el-tag :type="resultTagType(row)">{{ resultText(row) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="reason" :label="t('system.audit.reason')" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <span :class="['reason-text', { danger: isRiskRow(row) || row.result === 'failed' }]">{{ localizedReason(row.reason) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="requestId" :label="t('system.audit.requestId')" min-width="180" show-overflow-tooltip>
          <template #default="{ row }"><span class="mono">{{ row.requestId || '-' }}</span></template>
        </el-table-column>
        <el-table-column prop="sessionId" :label="t('system.audit.sessionId')" min-width="180" show-overflow-tooltip>
          <template #default="{ row }"><span class="mono">{{ row.sessionId || '-' }}</span></template>
        </el-table-column>
        <el-table-column :label="t('system.audit.colAction')" width="110" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" plain size="small" @click="openDetail(row)">{{ t('system.audit.viewDetail') }}</el-button>
          </template>
        </el-table-column>

        <template #empty>
          <el-empty :description="t('system.audit.emptyLog')">
            <el-button @click="resetFilters">{{ t('system.audit.resetFilter') }}</el-button>
          </el-empty>
        </template>
      </el-table>
    </div>

    <div class="pager-row">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :total="total"
        layout="total, sizes, prev, pager, next, jumper"
        :page-sizes="[10, 20, 50, 100]"
        @current-change="handlePageChange"
        @size-change="handleSizeChange"
      />
    </div>

    <el-drawer v-model="detailDrawerVisible" :title="t('system.audit.drawerTitle')" size="42%">
      <el-descriptions :column="1" border>
        <el-descriptions-item :label="t('system.audit.detailTime')">{{ formatTime(detailRow?.createdAt) }}</el-descriptions-item>
        <el-descriptions-item :label="t('system.audit.detailOpType')">{{ detailRow ? operationTypeLabel(detailRow) : '-' }}</el-descriptions-item>
        <el-descriptions-item :label="t('system.audit.detailResult')">{{ detailRow ? resultText(detailRow) : '-' }}</el-descriptions-item>
        <el-descriptions-item :label="t('system.audit.detailUsername')">{{ normalizedUsername(detailRow?.username) }}</el-descriptions-item>
        <el-descriptions-item :label="t('system.audit.detailIp')">{{ detailRow?.ip || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="t('system.audit.detailLocation')">{{ detailRow?.location || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="t('system.audit.detailRequestId')">{{ detailRow?.requestId || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="t('system.audit.detailSessionId')">{{ detailRow?.sessionId || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="t('system.audit.detailTraceId')">{{ detailRow?.traceId || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="t('system.audit.detailReason')">{{ localizedReason(detailRow?.reason) }}</el-descriptions-item>
        <el-descriptions-item :label="t('system.audit.detailUserAgent')">{{ detailRow?.userAgent || '-' }}</el-descriptions-item>
      </el-descriptions>
      <div class="detail-block">
        <div class="detail-title">{{ t('system.audit.resourceDetail') }}</div>
        <el-descriptions :column="1" border>
          <el-descriptions-item :label="t('system.audit.detailResType')">{{ detailRow?.resourceType || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.audit.detailResName')">
            <a v-if="detailRow?.resourceLink" class="resource-link" @click.prevent="openResource(detailRow)">{{ resourceNameLabel(detailRow) }}</a>
            <span v-else>{{ resourceNameLabel(detailRow) }}</span>
          </el-descriptions-item>
          <el-descriptions-item :label="t('system.audit.detailResId')"><span class="mono">{{ detailRow?.resourceId || '-' }}</span></el-descriptions-item>
          <el-descriptions-item :label="t('system.audit.detailResSummary')">{{ detailRow?.resourceSummary || '-' }}</el-descriptions-item>
        </el-descriptions>
        <el-table v-if="resourceDetailRows.length" :data="resourceDetailRows" border size="small" class="diff-table">
          <el-table-column prop="key" :label="t('system.audit.fieldCol')" min-width="180" />
          <el-table-column :label="t('system.audit.valueCol')" min-width="260" />
        </el-table>
      </div>
      <div class="detail-block">
        <div class="detail-title">{{ t('system.audit.compareTitle') }}</div>
        <el-table :data="compareRows" border size="small" class="diff-table">
          <el-table-column prop="key" :label="t('system.audit.fieldCol')" min-width="180" />
          <el-table-column :label="t('system.audit.beforeCol')" min-width="220">
            <template #default="{ row }">
              <span :class="row.kind === 'removed' ? 'diff-removed' : ''">{{ row.beforeText }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('system.audit.afterCol')" min-width="220">
            <template #default="{ row }">
              <span :class="row.kind === 'added' || row.kind === 'changed' ? 'diff-added' : ''">{{ row.afterText }}</span>
            </template>
          </el-table-column>
        </el-table>
      </div>
      <div class="detail-block">
        <div class="detail-title title-with-action">
          <span>{{ t('system.audit.rawTitle') }}</span>
          <el-button link size="small" @click="copyRawJson">{{ t('system.audit.copyBtn') }}</el-button>
        </div>
        <pre class="detail-json" v-html="highlightedDetailJson"></pre>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import dayjs from 'dayjs';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { useDebounceFn } from '@vueuse/core';
import type { LoginAuditRecord } from '@/types/system';
import { listLoginAudits } from '@/api/system/audit';
import { useSystemViewStore } from '@/store/systemView';
import { usePermissionStore } from '@/store/permission';
import { showHttpError } from '@/shared/errors/errorToast';
import { showSuccess, showWarning } from '@/shared/errors/messageToast';
import AuditStats from '@/components/audit/AuditStats.vue';

type TimePreset = '24h' | '7d' | '30d' | 'custom';
type AuditResultFilter = LoginAuditRecord['result'] | 'risk' | '';
type AuditRecordExt = LoginAuditRecord & {
  operationType?: string;
  riskLevel?: string;
};

const { t } = useI18n();
const router = useRouter();
const viewStore = useSystemViewStore();
const permissionStore = usePermissionStore();

const loading = ref(false);
const exporting = ref(false);
const rows = ref<AuditRecordExt[]>([]);
const selectedRows = ref<AuditRecordExt[]>([]);
const total = ref(0);
const tableRef = ref<any>();
const page = ref(viewStore.loginAudit.page || 1);
const pageSize = ref(viewStore.loginAudit.pageSize || 10);
const timePreset = ref<TimePreset>((viewStore.loginAudit.timePreset as TimePreset) || 'custom');
const range = ref<[Date, Date] | []>(
  viewStore.loginAudit.startAt && viewStore.loginAudit.endAt
    ? [new Date(viewStore.loginAudit.startAt), new Date(viewStore.loginAudit.endAt)]
    : []
);
const sortOrder = ref<'ascending' | 'descending' | null>('descending');
const detailDrawerVisible = ref(false);
const detailRow = ref<AuditRecordExt | null>(null);

const filters = reactive({
  username: viewStore.loginAudit.username || '',
  ip: viewStore.loginAudit.ip || '',
  result: (viewStore.loginAudit.result || '') as AuditResultFilter,
  operationType: viewStore.loginAudit.operationType || '',
  resourceKeyword: (viewStore.loginAudit as any).resourceKeyword || '',
  idKeyword: viewStore.loginAudit.idKeyword || '',
  requestId: viewStore.loginAudit.requestId || '',
  sessionId: viewStore.loginAudit.sessionId || ''
});

if (filters.operationType === 'admin.activity') {
  filters.operationType = '';
}

const now = () => new Date();
const buildPresetRange = (preset: TimePreset): [Date, Date] | [] => {
  if (preset === 'custom') return range.value;
  const end = now();
  if (preset === '24h') return [dayjs(end).subtract(24, 'hour').toDate(), end];
  if (preset === '7d') return [dayjs(end).subtract(7, 'day').toDate(), end];
  return [dayjs(end).subtract(30, 'day').toDate(), end];
};

if (!range.value.length && timePreset.value !== 'custom') {
  range.value = buildPresetRange(timePreset.value);
}

const opTypeLabelMap = computed<Record<string, string>>(() => ({
  login: t('system.audit.opLogin'),
  'auth.login': t('system.audit.opLogin'),
  'auth.login.failed': t('system.audit.opLoginFailed'),
  'auth.login.abnormal': t('system.audit.opLoginAbnormal'),
  'auth.logout': t('system.audit.opLogout'),
  'auth.session.create': t('system.audit.opSessionCreate'),
  'auth.session.refresh': t('system.audit.opSessionRefresh'),
  'auth.session.revoke': t('system.audit.opSessionRevoke'),
  'auth.session.timeout': t('system.audit.opSessionTimeout'),
  'auth.user.create': t('system.audit.opUserCreate'),
  'auth.user.update': t('system.audit.opUserUpdate'),
  'auth.user.enable': t('system.audit.opUserEnable'),
  'auth.user.disable': t('system.audit.opUserDisable'),
  'auth.user.password.reset': t('system.audit.opUserPwdReset'),
  'auth.user.delete': t('system.audit.opUserDelete'),
  'auth.user.roles.replace': t('system.audit.opUserRolesReplace'),
  'rbac.role.create': t('system.audit.opRoleCreate'),
  'rbac.role.update': t('system.audit.opRoleUpdate'),
  'rbac.role.delete': t('system.audit.opRoleDelete'),
  'rbac.assignment.create': t('system.audit.opAssignCreate'),
  'rbac.assignment.delete': t('system.audit.opAssignDelete'),
  'pool.create': t('system.audit.opPoolCreate'),
  'pool.update': t('system.audit.opPoolUpdate'),
  'pool.import': t('system.audit.opPoolImport'),
  'pool.export': t('system.audit.opPoolExport'),
  'pool.delete': t('system.audit.opPoolDelete'),
  'lease.allocate': t('system.audit.opLeaseAllocate'),
  'lease.renew': t('system.audit.opLeaseRenew'),
  'lease.release': t('system.audit.opLeaseRelease'),
  'lease.reclaim': t('system.audit.opLeaseReclaim'),
  'lease.cooldown_clear': t('system.audit.opLeaseCooldown'),
  'lease.security_state': t('system.audit.opLeaseSecurity'),
  'binding.create': t('system.audit.opBindingCreate'),
  'binding.update': t('system.audit.opBindingUpdate'),
  'binding.delete': t('system.audit.opBindingDelete'),
  'scope.create': t('system.audit.opScopeCreate'),
  'scope.update': t('system.audit.opScopeUpdate'),
  'scope.delete': t('system.audit.opScopeDelete'),
  'scope.sync': t('system.audit.opScopeSync'),
  'option.create': t('system.audit.opOptionCreate'),
  'option.update': t('system.audit.opOptionUpdate'),
  'option.delete': t('system.audit.opOptionDelete'),
  'template.create': t('system.audit.opTemplateCreate'),
  'template.update': t('system.audit.opTemplateUpdate'),
  'template.delete': t('system.audit.opTemplateDelete'),
  'template.apply': t('system.audit.opTemplateApply'),
  'prefix.release': t('system.audit.opPrefixRelease'),
  'prefix.decline': t('system.audit.opPrefixDecline'),
  'cluster.node.create': t('system.audit.opClusterNodeCreate'),
  'cluster.node.update': t('system.audit.opClusterNodeUpdate'),
  'cluster.node.delete': t('system.audit.opClusterNodeDelete'),
  'cluster.initialize': t('system.audit.opClusterInit'),
  'cluster.join': t('system.audit.opClusterJoin'),
  'cluster.delete': t('system.audit.opClusterDelete'),
  'cluster.leave': t('system.audit.opClusterLeave'),
  'ha.member.upsert': t('system.audit.opHaMemberUpsert'),
  'ha.member.remove': t('system.audit.opHaMemberRemove'),
  'ha.failover': t('system.audit.opHaFailover'),
  'ha.load_balancer': t('system.audit.opHaLoadBalancer'),
  'ha.join_job.create': t('system.audit.opHaJoinJobCreate'),
  'ha.join_job.cancel': t('system.audit.opHaJoinJobCancel'),
  'ha.join_job.retry': t('system.audit.opHaJoinJobRetry'),
  'security.policy.rule.create': t('system.audit.opSecPolicyCreate'),
  'security.policy.rule.update': t('system.audit.opSecPolicyUpdate'),
  'security.policy.rule.delete': t('system.audit.opSecPolicyDelete'),
  'security.mac_list.create': t('system.audit.opMacListCreate'),
  'security.mac_list.update': t('system.audit.opMacListUpdate'),
  'security.mac_list.delete': t('system.audit.opMacListDelete'),
  'security.mac_list.enable': t('system.audit.opMacListEnable'),
  'security.mac_list.disable': t('system.audit.opMacListDisable'),
  'security.mac_list.import': t('system.audit.opMacListImport'),
  'security.mac_list.export': t('system.audit.opMacListExport'),
  'config.update': t('system.audit.opConfigUpdate'),
  'backup.run': t('system.audit.opBackupRun'),
  'backup.restore': t('system.audit.opBackupRestore'),
  'ops.system.update': t('system.audit.opSysUpdate'),
  'ops.system.theme': t('system.audit.opSysTheme'),
  'ops.system.locale': t('system.audit.opSysLocale'),
  'ops.system.maintenance': t('system.audit.opSysMaintenance'),
  'ops.transfer.job.start': t('system.audit.opTransferStart'),
  'ops.script_run.start': t('system.audit.opScriptStart'),
  'ops.script_run.approve': t('system.audit.opScriptApprove'),
  'ops.script_run.reject': t('system.audit.opScriptReject'),
  'system.service.start': t('system.audit.opServiceStart'),
  'system.service.stop': t('system.audit.opServiceStop'),
  'system.reload': t('system.audit.opSysReload'),
  'system.upgrade': t('system.audit.opSysUpgrade'),
  'audit.config.update': t('system.audit.opAuditConfigUpdate'),
  'audit.flush': t('system.audit.opAuditFlush'),
  'auth.api_key.create': t('system.audit.opApiKeyCreate'),
  'auth.api_key.enable': t('system.audit.opApiKeyEnable'),
  'auth.api_key.disable': t('system.audit.opApiKeyDisable'),
  'auth.api_key.revoke': t('system.audit.opApiKeyRevoke'),
  'auth.api_key.exchange': t('system.audit.opApiKeyExchange'),
  'auth.provider.create': t('system.audit.opAuthProviderCreate'),
  'auth.provider.update': t('system.audit.opAuthProviderUpdate'),
  'tenant.quota.update': t('system.audit.opTenantQuotaUpdate'),
  'automation.schedule.update': t('system.audit.opAutoScheduleUpdate'),
  'automation.approval.approve': t('system.audit.opAutoApprove'),
  'automation.approval.reject': t('system.audit.opAutoReject'),
  'automation.approval.apply': t('system.audit.opAutoApply'),
  'notification.channel.upsert': t('system.audit.opNotifyChannelUpsert'),
  'notification.webhook.update': t('system.audit.opWebhookUpdate'),
  'notification.smtp.update': t('system.audit.opSmtpUpdate'),
  'alert.rule.create': t('system.audit.opAlertRuleCreate'),
  'alert.rule.update': t('system.audit.opAlertRuleUpdate'),
  'alert.rule.delete': t('system.audit.opAlertRuleDelete'),
  'alert.config.thresholds.update': t('system.audit.opAlertThresholds'),
  'alert.config.notify.update': t('system.audit.opAlertNotify'),
  'alert.template.create': t('system.audit.opAlertTplCreate'),
  'alert.template.update': t('system.audit.opAlertTplUpdate'),
  'alert.template.delete': t('system.audit.opAlertTplDelete'),
  'alert.receiver.create': t('system.audit.opAlertReceiverCreate'),
  'alert.receiver.update': t('system.audit.opAlertReceiverUpdate'),
  'alert.receiver.delete': t('system.audit.opAlertReceiverDelete'),
  'alert.receiver.import': t('system.audit.opAlertReceiverImport'),
  'alert.receiver.export': t('system.audit.opAlertReceiverExport'),
  'alert.route.update': t('system.audit.opAlertRouteUpdate'),
  'alert.roster.update': t('system.audit.opAlertRosterUpdate'),
  'alert.action.ack': t('system.audit.opAlertAck'),
  'alert.action.suppress': t('system.audit.opAlertSuppress'),
  'alert.action.silence': t('system.audit.opAlertSilence'),
  'alert.action.escalate': t('system.audit.opAlertEscalate'),
  'migration.pool.import': t('system.audit.opMigPoolImport'),
  'migration.pool.export': t('system.audit.opMigPoolExport'),
  'migration.binding.import': t('system.audit.opMigBindingImport'),
  'migration.binding.export': t('system.audit.opMigBindingExport'),
  'migration.lease.import': t('system.audit.opMigLeaseImport'),
  'migration.lease.export': t('system.audit.opMigLeaseExport'),
  'migration.archive.import': t('system.audit.opMigArchiveImport'),
  'migration.archive.export': t('system.audit.opMigArchiveExport'),
  'migration.cleanup.import': t('system.audit.opMigCleanupImport'),
  'migration.cleanup.export': t('system.audit.opMigCleanupExport'),
  'migration.transfer.import': t('system.audit.opMigTransferImport'),
  'migration.transfer.export': t('system.audit.opMigTransferExport'),
  logout: t('system.audit.opLegacyLogout'),
  password_reset: t('system.audit.opLegacyPwdReset'),
  token_refresh: t('system.audit.opLegacyTokenRefresh'),
  user_update: t('system.audit.opLegacyUserUpdate'),
  user_delete: t('system.audit.opLegacyUserDelete'),
  role_update: t('system.audit.opLegacyRoleUpdate'),
  role_delete: t('system.audit.opLegacyRoleDelete'),
  session_revoke: t('system.audit.opLegacySessionRevoke'),
  apikey_create: t('system.audit.opLegacyApiKeyCreate'),
  apikey_delete: t('system.audit.opLegacyApiKeyDelete')
}));

const opTypeAliasMap: Record<string, string> = {
  clusterinitialize: 'cluster.initialize',
  clusterjoin: 'cluster.join',
  clusterdelete: 'cluster.delete',
  clusterleave: 'cluster.leave',
  clusternodecreate: 'cluster.node.create',
  clusternodeupdate: 'cluster.node.update',
  clusternodedelete: 'cluster.node.delete',
  hamemberupsert: 'ha.member.upsert',
  hamemberremove: 'ha.member.remove',
  hajoinjobcreate: 'ha.join_job.create',
  hajoinjobcancel: 'ha.join_job.cancel',
  hajoinjobretry: 'ha.join_job.retry'
};

const normalizeOperationType = (value: unknown) => {
  const raw = String(value || '').trim();
  if (!raw) return '';
  const lowered = raw.toLowerCase();
  return opTypeAliasMap[lowered] || lowered;
};

const inferOperationType = (row: AuditRecordExt) => {
  const raw = normalizeOperationType((row as any).operationType || (row as any).action);
  if (raw) {
    if (raw === 'pool.update') {
      const beforeStatus = String((row.before || {})['status'] || '').trim().toLowerCase();
      const afterStatus = String((row.after || {})['status'] || '').trim().toLowerCase();
      if (beforeStatus !== afterStatus) {
        if (afterStatus === 'disabled') return t('system.audit.opPoolDisable');
        if (afterStatus === 'active') return t('system.audit.opPoolEnable');
      }
    }
    if (raw === 'scope.update') {
      const beforeStatus = String((row.before || {})['status'] || '').trim().toLowerCase();
      const afterStatus = String((row.after || {})['status'] || '').trim().toLowerCase();
      if (beforeStatus !== afterStatus) {
        if (afterStatus === 'inactive') return t('system.audit.opScopeDisable');
        if (afterStatus === 'active') return t('system.audit.opScopeEnable');
      }

      const beforeTemplateId = String((row.before || {})['templateId'] || '').trim();
      const afterTemplateId = String((row.after || {})['templateId'] || '').trim();
      if (beforeTemplateId !== afterTemplateId) {
        if (afterTemplateId) return t('system.audit.opScopeApplyTpl');
        return t('system.audit.opScopeUnbindTpl');
      }
    }
    if (raw === 'auth.user.update') {
      const payload = (row.rawPayload || {}) as Record<string, any>;
      const normalizedStatus = String(payload.status || (row.after || {})['status'] || '')
        .trim()
        .toLowerCase();
      if (['disabled', 'inactive', 'blocked', 'suspended'].includes(normalizedStatus)) return t('system.audit.opUserDisable');
      if (['active', 'enabled'].includes(normalizedStatus)) return t('system.audit.opUserEnable');
      if (payload.mustChangePassword === true || String(payload.password || '').trim() !== '') return t('system.audit.opUserPwdReset');
    }
    return opTypeLabelMap.value[raw] || raw;
  }
  const reason = String(row.reason || '').toLowerCase();
  if (reason.includes('login') || reason.includes('登录')) return t('system.audit.opInferLogin');
  if (reason.includes('logout') || reason.includes('退出')) return t('system.audit.opInferLogout');
  if (reason.includes('password') || reason.includes('密码')) return t('system.audit.opInferPwdReset');
  if (reason.includes('token')) return t('system.audit.opInferTokenRefresh');
  if (reason.includes('role') || reason.includes('角色')) return t('system.audit.opInferRoleChange');
  if (reason.includes('user') || reason.includes('用户')) return t('system.audit.opInferUserChange');
  if (reason.includes('session') || reason.includes('会话')) return t('system.audit.opInferSessionChange');
  if (reason.includes('api key') || reason.includes('apikey') || reason.includes('密钥')) return t('system.audit.opInferApiKey');
  return t('system.audit.opInferSystem');
};

const isRiskRow = (row: AuditRecordExt) => {
  if (row.result === 'failed') return true;
  const op = inferOperationType(row).toLowerCase();
  const reason = String(row.reason || '').toLowerCase();
  const riskWords = ['异常', '风险', 'blocked', 'forbidden', 'attack', 'invalid', 'denied', '冲突', 'risk', 'abnormal', 'conflict'];
  const riskOpWords = ['delete', 'revoke', 'reset', '删除', '吊销', '重置密码'];
  return (
    riskWords.some((item) => reason.includes(item)) ||
    riskOpWords.some((item) => op.includes(item))
  );
};

const operationTypeLabel = (row: AuditRecordExt) => inferOperationType(row);

const resourceNameLabel = (row?: AuditRecordExt | null) => {
  if (!row) return '-';
  return String(row.resourceName || row.resourceSummary || row.resourceId || '-');
};

const operationTypeOptions = computed(() => {
  const m = opTypeLabelMap.value;
  const options = new Map<string, string>([
    ['auth.login', m['auth.login']],
    ['auth.login.failed', m['auth.login.failed']],
    ['auth.login.abnormal', m['auth.login.abnormal']],
    ['auth.logout', m['auth.logout']],
    ['auth.session.create', m['auth.session.create']],
    ['auth.session.revoke', m['auth.session.revoke']],
    ['auth.user.create', m['auth.user.create']],
    ['auth.user.update', m['auth.user.update']],
    ['auth.user.enable', m['auth.user.enable']],
    ['auth.user.disable', m['auth.user.disable']],
    ['auth.user.password.reset', m['auth.user.password.reset']],
    ['auth.user.delete', m['auth.user.delete']],
    ['auth.user.roles.replace', m['auth.user.roles.replace']],
    ['rbac.role.create', m['rbac.role.create']],
    ['rbac.role.update', m['rbac.role.update']],
    ['rbac.role.delete', m['rbac.role.delete']],
    ['pool.create', m['pool.create']],
    ['pool.update', m['pool.update']],
    ['pool.import', m['pool.import']],
    ['pool.export', m['pool.export']],
    ['pool.enable', t('system.audit.opPoolEnable')],
    ['pool.disable', t('system.audit.opPoolDisable')],
    ['scope.enable', t('system.audit.opScopeEnable')],
    ['scope.disable', t('system.audit.opScopeDisable')],
    ['pool.delete', m['pool.delete']],
    ['cluster.initialize', m['cluster.initialize']],
    ['cluster.join', m['cluster.join']],
    ['cluster.delete', m['cluster.delete']],
    ['cluster.leave', m['cluster.leave']],
    ['lease.allocate', m['lease.allocate']],
    ['lease.renew', m['lease.renew']],
    ['lease.release', m['lease.release']],
    ['binding.create', m['binding.create']],
    ['binding.update', m['binding.update']],
    ['binding.delete', m['binding.delete']],
    ['security.policy.rule.update', m['security.policy.rule.update']],
    ['config.update', m['config.update']],
    ['cluster.node.create', m['cluster.node.create']],
    ['cluster.node.update', m['cluster.node.update']],
    ['cluster.node.delete', m['cluster.node.delete']]
  ]);
  rows.value.forEach((row) => {
    const value = normalizeOperationType((row as any).operationType || (row as any).action);
    if (!value) return;
    if (value === 'admin.activity') return;
    options.set(value, m[value] || inferOperationType(row));
  });
  return Array.from(options.entries()).map(([value, label]) => ({ label, value }));
});

const resultText = (row: AuditRecordExt) => {
  if (row.result === 'failed') return t('system.audit.failed');
  if (isRiskRow(row)) return t('system.audit.risk');
  return t('system.audit.success');
};

const resultTagType = (row: AuditRecordExt) => {
  if (row.result === 'failed') return 'danger';
  if (isRiskRow(row)) return 'warning';
  return 'success';
};

const reasonI18nRules = computed<Array<[RegExp, string]>>(() => [
  [/refresh token invalid or expired/i, t('system.audit.reasonRefreshTokenExpired')],
  [/refresh token invalid/i, t('system.audit.reasonRefreshTokenInvalid')],
  [/refreshToken is required/i, t('system.audit.reasonRefreshTokenRequired')],
  [/invalid username or password/i, t('system.audit.reasonInvalidCredentials')],
  [/authentication required/i, t('system.audit.reasonAuthRequired')],
  [/principal not found|principal not resolved/i, t('system.audit.reasonPrincipalNotFound')],
  [/verification code invalid/i, t('system.audit.reasonVerifyCodeInvalid')],
  [/verification challenge not found/i, t('system.audit.reasonVerifyChallengeNotFound')],
  [/verification required/i, t('system.audit.reasonVerifyRequired')],
  [/reset challenge expired/i, t('system.audit.reasonResetExpired')],
  [/account verification failed/i, t('system.audit.reasonAccountVerifyFail')],
  [/failed to send verification code/i, t('system.audit.reasonSendVerifyFail')],
  [/invalid captcha/i, t('system.audit.reasonInvalidCaptcha')],
  [/captcha required/i, t('system.audit.reasonCaptchaRequired')],
  [/token expired/i, t('system.audit.reasonTokenExpired')],
  [/token invalid/i, t('system.audit.reasonTokenInvalid')],
  [/csrf token missing/i, t('system.audit.reasonCsrfMissing')],
  [/csrf token invalid/i, t('system.audit.reasonCsrfInvalid')],
  [/unauthorized|forbidden|permission denied/i, t('system.audit.reasonForbidden')],
  [/rbac resolution failed/i, t('system.audit.reasonRbacFail')],
  [/insufficient role/i, t('system.audit.reasonInsufficientRole')],
  [/user not found/i, t('system.audit.reasonUserNotFound')],
  [/role not found/i, t('system.audit.reasonRoleNotFound')],
  [/pool not found/i, t('system.audit.reasonPoolNotFound')],
  [/binding not found/i, t('system.audit.reasonBindingNotFound')],
  [/option not found/i, t('system.audit.reasonOptionNotFound')],
  [/profile not found/i, t('system.audit.reasonProfileNotFound')],
  [/device not found/i, t('system.audit.reasonDeviceNotFound')],
  [/session not found/i, t('system.audit.reasonSessionNotFound')],
  [/lease not found|prefix lease not found/i, t('system.audit.reasonLeaseNotFound')],
  [/not found/i, t('system.audit.reasonNotFound')],
  [/already exists|duplicate/i, t('system.audit.reasonAlreadyExists')],
  [/is required/i, t('system.audit.reasonRequired')],
  [/must be RFC3339|must be Go duration string|must be a positive integer|must be a non-negative integer|must be a valid UUID|must be in the future/i, t('system.audit.reasonFormatInvalid')],
  [/cannot be negative|at least one .+ required|no supported mutation specified|unsupported action|unsupported .* supported/i, t('system.audit.reasonParamInvalid')],
  [/bad request|invalid request/i, t('system.audit.reasonBadRequest')],
  [/service unavailable|disabled|not implemented|unavailable|not initialized/i, t('system.audit.reasonServiceUnavailable')],
  [/conflict|already exists/i, t('system.audit.reasonConflict')],
  [/failed to build/i, t('system.audit.reasonBuildFail')],
  [/internal server error/i, t('system.audit.reasonInternalError')],
  [/timeout/i, t('system.audit.reasonTimeout')],
  [/network error/i, t('system.audit.reasonNetworkError')]
]);

const localizedReason = (reason?: string) => {
  const text = String(reason || '').trim();
  if (!text) return '-';
  for (const [pattern, label] of reasonI18nRules.value) {
    if (pattern.test(text)) return label;
  }
  return text;
};

const normalizedUsername = (username?: string) => {
  const raw = String(username || '').trim();
  if (!raw) return '-';
  if (/^session\s*:/i.test(raw)) {
    const value = raw.replace(/^session\s*:/i, '').trim();
    return value || raw;
  }
  return raw;
};

const idMatched = (row: AuditRecordExt) => {
  const key = String(filters.idKeyword || '').trim().toLowerCase();
  if (!key) return true;
  return [row.id, row.requestId, row.sessionId, row.traceId]
    .map((item) => String(item || '').toLowerCase())
    .some((text) => text.includes(key));
};

const opTypeMatched = (row: AuditRecordExt) => {
  if (!filters.operationType) return true;
  if (filters.operationType === 'pool.enable') return operationTypeLabel(row) === t('system.audit.opPoolEnable');
  if (filters.operationType === 'pool.disable') return operationTypeLabel(row) === t('system.audit.opPoolDisable');
  if (filters.operationType === 'scope.enable') return operationTypeLabel(row) === t('system.audit.opScopeEnable');
  if (filters.operationType === 'scope.disable') return operationTypeLabel(row) === t('system.audit.opScopeDisable');
  if (filters.operationType === 'auth.user.enable') return operationTypeLabel(row) === t('system.audit.opUserEnable');
  if (filters.operationType === 'auth.user.disable') return operationTypeLabel(row) === t('system.audit.opUserDisable');
  if (filters.operationType === 'auth.user.password.reset') return operationTypeLabel(row) === t('system.audit.opUserPwdReset');
  return normalizeOperationType((row as any).operationType || (row as any).action) === filters.operationType;
};

const resultMatched = (row: AuditRecordExt) => {
  if (!filters.result) return true;
  if (filters.result === 'risk') return isRiskRow(row) && row.result !== 'failed';
  return row.result === filters.result;
};

const sortedRows = computed(() => {
  const cloned = [...rows.value];
  if (!sortOrder.value) return cloned;
  return cloned.sort((a, b) => {
    const av = dayjs(a.createdAt).valueOf();
    const bv = dayjs(b.createdAt).valueOf();
    return sortOrder.value === 'ascending' ? av - bv : bv - av;
  });
});

const displayRows = computed(() => sortedRows.value.filter((row) => resultMatched(row) && opTypeMatched(row) && idMatched(row)));

const detailJson = computed(() => JSON.stringify(detailRow.value?.rawPayload || detailRow.value || {}, null, 2));

const compareRows = computed(() => {
  const before = (detailRow.value?.before || {}) as Record<string, any>;
  const after = (detailRow.value?.after || {}) as Record<string, any>;
  const keys = Array.from(new Set([...Object.keys(before), ...Object.keys(after)])).sort();
  return keys.map((key) => {
    const beforeVal = before[key];
    const afterVal = after[key];
    const beforeText = formatDiffValue(beforeVal);
    const afterText = formatDiffValue(afterVal);
    let kind: 'added' | 'removed' | 'changed' | 'same' = 'same';
    if (beforeVal === undefined && afterVal !== undefined) kind = 'added';
    else if (beforeVal !== undefined && afterVal === undefined) kind = 'removed';
    else if (beforeText !== afterText) kind = 'changed';
    return { key, beforeText, afterText, kind };
  });
});

const resourceDetailRows = computed(() => {
  if (!detailRow.value) return [] as Array<{ key: string; value: string }>;
  const row = detailRow.value;
  const before = row.before || {};
  const after = row.after || {};
  const resourceType = String(row.resourceType || '').toLowerCase();
  const result: Array<{ key: string; value: string }> = [];
  const pushIf = (key: string, value: any) => {
    const text = formatDiffValue(value);
    if (text && text !== '-') result.push({ key, value: text });
  };
  if (resourceType.includes('pool') || String(row.operationType || '').startsWith('pool.')) {
    pushIf(t('system.audit.resPoolName'), row.resourceName || after['name'] || before['name']);
    pushIf(t('system.audit.resCidr'), after['cidr'] || before['cidr']);
    pushIf(t('system.audit.resRange'), `${after['rangeStart'] || before['rangeStart'] || '-'} ~ ${after['rangeEnd'] || before['rangeEnd'] || '-'}`);
    pushIf(t('system.audit.resGateway'), after['gateway'] || before['gateway']);
    pushIf(t('system.audit.resDns'), (after['dns'] || before['dns']) as any);
  } else if (resourceType.includes('cluster') || resourceType.includes('ha') || String(row.operationType || '').startsWith('cluster.')) {
    pushIf(t('system.audit.resNodeIp'), (row.rawPayload || {})['ip'] || after['ip'] || before['ip']);
    pushIf(t('system.audit.resRoleChange'), `${before['role'] || '-'} -> ${after['role'] || '-'}`);
    pushIf(t('system.audit.resStatusChange'), `${before['status'] || '-'} -> ${after['status'] || '-'}`);
  } else if (resourceType.includes('config') || String(row.operationType || '').startsWith('ops.system') || String(row.operationType || '').startsWith('system.')) {
    const changes = ((row.rawPayload || {})['changes'] || {}) as Record<string, any>;
    const keys = Object.keys(changes);
    if (keys.length) {
      keys.forEach((key) => {
        const change = changes[key] || {};
        pushIf(t('system.audit.resConfigItem', { key }), `${formatDiffValue(change.before)} -> ${formatDiffValue(change.after)}`);
      });
    }
  }
  return result;
});

const highlightedDetailJson = computed(() => highlightJson(detailJson.value, detailRow.value));

const persistState = (startAt: string, endAt: string) => {
  viewStore.setLoginAudit({
    ...filters,
    keyword: filters.idKeyword,
    startAt,
    endAt,
    page: page.value,
    pageSize: pageSize.value,
    timePreset: timePreset.value
  });
};

const getRangeISO = () => {
  if (!Array.isArray(range.value) || range.value.length < 2) return { start: '', end: '' };
  return {
    start: dayjs(range.value[0]).toISOString(),
    end: dayjs(range.value[1]).toISOString()
  };
};

const fetch = async () => {
  if (!permissionStore.can('audit.read')) {
    rows.value = [];
    total.value = 0;
    return;
  }
  if (timePreset.value !== 'custom') {
    range.value = buildPresetRange(timePreset.value);
  }
  const { start, end } = getRangeISO();
  loading.value = true;
  try {
    persistState(start, end);
    const { data } = await listLoginAudits({
      page: page.value,
      pageSize: pageSize.value,
      username: filters.username || undefined,
      ip: filters.ip || undefined,
      result: filters.result === 'risk' ? undefined : (filters.result as LoginAuditRecord['result'] | ''),
      requestId: filters.requestId || undefined,
      sessionId: filters.sessionId || undefined,
      keyword: filters.idKeyword || undefined,
      resourceKeyword: filters.resourceKeyword || undefined,
      startAt: start || undefined,
      endAt: end || undefined,
      ...(filters.operationType && ![
        'pool.enable',
        'pool.disable',
        'scope.enable',
        'scope.disable',
        'auth.user.enable',
        'auth.user.disable',
        'auth.user.password.reset'
      ].includes(filters.operationType)
        ? ({ operationType: filters.operationType } as any)
        : {})
    } as any);
    const items = (data.data.items || []) as AuditRecordExt[];
    rows.value = items;
    total.value = Number(data.data.total || 0);
  } catch (e) {
    const status = (e as any)?.response?.status;
    if (status === 403) {
      rows.value = [];
      total.value = 0;
      return;
    }
    showHttpError(e, t('system.audit.loadFail'));
  } finally {
    loading.value = false;
  }
};

const refresh = useDebounceFn(async () => {
  page.value = 1;
  await fetch();
  showSuccess(t('system.audit.refreshOk'));
}, 200);

const resetFilters = async () => {
  filters.username = '';
  filters.ip = '';
  filters.result = '';
  filters.operationType = '';
  filters.resourceKeyword = '';
  filters.idKeyword = '';
  filters.requestId = '';
  filters.sessionId = '';
  timePreset.value = 'custom';
  range.value = [];
  page.value = 1;
  await fetch();
  showSuccess(t('system.audit.filterResetOk'));
};

const onTimePresetChange = () => {
  if (timePreset.value !== 'custom') {
    range.value = buildPresetRange(timePreset.value);
  }
  page.value = 1;
  fetch();
};

const handlePageChange = (p: number) => {
  page.value = p;
  fetch();
};

const handleSizeChange = (size: number) => {
  pageSize.value = size;
  page.value = 1;
  fetch();
};

const onSortChange = ({ prop, order }: { prop: string; order: 'ascending' | 'descending' | null }) => {
  if (prop !== 'createdAt') return;
  sortOrder.value = order;
};

const onSelectionChange = (selection: AuditRecordExt[]) => {
  selectedRows.value = selection;
};

const clearSelection = () => {
  selectedRows.value = [];
  tableRef.value?.clearSelection?.();
};

const toCsv = (list: AuditRecordExt[]) => {
  const headers = [t('system.audit.csvTime'), t('system.audit.csvOpType'), t('system.audit.csvResName'), t('system.audit.csvResId'), t('system.audit.csvSummary'), t('system.audit.csvResult'), t('system.audit.csvUsername'), t('system.audit.csvIp'), t('system.audit.csvReason'), t('system.audit.csvRequestId'), t('system.audit.csvSessionId'), t('system.audit.csvTraceId')];
  const lines = list.map((row) => {
    const cols = [
      formatTime(row.createdAt),
      operationTypeLabel(row),
      resourceNameLabel(row),
      row.resourceId || '',
      row.operationSummary || row.resourceSummary || row.reason || '',
      resultText(row),
      normalizedUsername(row.username),
      row.ip || '',
      localizedReason(row.reason),
      row.requestId || '',
      row.sessionId || '',
      row.traceId || ''
    ].map((value) => `"${String(value).replace(/"/g, '""')}"`);
    return cols.join(',');
  });
  return ['\uFEFF' + headers.join(','), ...lines].join('\n');
};

const downloadCsv = (name: string, csv: string) => {
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
};

const exportAuditLogs = async () => {
  exporting.value = true;
  try {
    const data = displayRows.value;
    if (!data.length) {
      showWarning(t('system.audit.noExportData'));
      return;
    }
    downloadCsv(`audit-logs-${dayjs().format('YYYYMMDDHHmmss')}.csv`, toCsv(data));
    showSuccess(t('system.audit.exportOk'));
  } finally {
    exporting.value = false;
  }
};

const exportSelected = async () => {
  if (!selectedRows.value.length) {
    showWarning(t('system.audit.noSelectedData'));
    return;
  }
  exporting.value = true;
  try {
    downloadCsv(`audit-selected-${dayjs().format('YYYYMMDDHHmmss')}.csv`, toCsv(selectedRows.value));
    showSuccess(t('system.audit.exportSelectedOk'));
  } finally {
    exporting.value = false;
  }
};

const openDetail = (row: AuditRecordExt) => {
  detailRow.value = row;
  detailDrawerVisible.value = true;
};

const openResource = (row?: AuditRecordExt | null) => {
  if (!row?.resourceLink) return;
  router.push(row.resourceLink);
};

const copyRawJson = async () => {
  try {
    await navigator.clipboard.writeText(detailJson.value);
    showSuccess(t('system.audit.copyOk'));
  } catch {
    showWarning(t('system.audit.copyFail'));
  }
};

const formatDiffValue = (value: any) => {
  if (value === undefined) return '-';
  if (value === null) return 'null';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  return JSON.stringify(value);
};

const escapeHtml = (text: string) =>
  text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');

const highlightJson = (jsonText: string, row?: AuditRecordExt | null) => {
  const withEscaped = escapeHtml(jsonText);
  return withEscaped.replace(
    /(\"([^\"]+)\"\s*:)|(\"([^\"]*)\")|(\btrue\b|\bfalse\b|null)|(\b-?\d+(?:\.\d+)?\b)/g,
    (match, keyToken, keyName, _strToken, strVal, boolNull, numVal) => {
      if (keyToken) {
        const hintKey = keyName || '';
        let title = '';
        if (row && (hintKey === 'poolId' || hintKey === 'nodeId' || hintKey === 'configKey' || hintKey === 'identifier')) {
          title = row.resourceName || row.resourceSummary || '';
        }
        const titleAttr = title ? ` title="${escapeHtml(title)}"` : '';
        return `<span class="json-key"${titleAttr}>${match}</span>`;
      }
      if (strVal !== undefined) return `<span class="json-string">${match}</span>`;
      if (boolNull) return `<span class="json-bool">${match}</span>`;
      if (numVal) return `<span class="json-number">${match}</span>`;
      return match;
    }
  );
};

const formatTime = (value?: string) => (value ? dayjs(value).format('YYYY-MM-DD HH:mm:ss') : '-');

onMounted(fetch);
</script>

<style scoped>
.login-audit {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  padding: 14px 16px 16px;
  box-sizing: border-box;
}

.header-row h3 {
  margin: 0;
}

.subtitle {
  margin: 6px 0 0;
  color: var(--el-text-color-secondary);
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.overview-card {
  border-radius: 10px;
  min-height: 94px;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.overview-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.overview-value {
  margin-top: 8px;
  font-size: 24px;
  line-height: 1;
  font-weight: 600;
}

.filter-bar {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 10px;
  background: var(--el-fill-color-blank);
}

.bar-left {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.bar-right {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.filter-item {
  width: 160px;
}

.range-item {
  width: 330px;
}

.w-130 {
  width: 130px;
}

.w-150 {
  width: 150px;
}

.w-160 {
  width: 160px;
}

.w-180 {
  width: 180px;
}

.bulk-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.table-wrapper {
  min-height: 380px;
}

.data-table :deep(.el-table__row:hover > td) {
  background: var(--el-fill-color-light) !important;
}

.mono {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
}

.sub {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.2;
}

.reason-text.danger {
  color: var(--el-color-danger);
  font-weight: 500;
}

.pager-row {
  display: flex;
  justify-content: flex-end;
}

.detail-block {
  margin-top: 12px;
}

.detail-title {
  margin-bottom: 8px;
  color: var(--el-text-color-secondary);
}

.title-with-action {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.resource-link {
  color: var(--el-color-primary);
  cursor: pointer;
  text-decoration: none;
}

.resource-link:hover {
  text-decoration: underline;
}

.diff-table {
  margin-top: 8px;
}

.diff-added {
  color: var(--el-color-success);
  font-weight: 600;
}

.diff-removed {
  color: var(--el-color-danger);
  font-weight: 600;
}

.detail-json {
  margin: 0;
  padding: 10px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  background: var(--el-fill-color-light);
  max-height: 260px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

.detail-json :deep(.json-key) {
  color: #6f42c1;
}

.detail-json :deep(.json-string) {
  color: #0b7a75;
}

.detail-json :deep(.json-number) {
  color: #0a58ca;
}

.detail-json :deep(.json-bool) {
  color: #d63384;
}
</style>
