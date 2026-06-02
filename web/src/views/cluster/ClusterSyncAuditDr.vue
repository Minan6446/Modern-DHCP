<template>
  <div class="page-wrap" v-loading="loading">
    <section class="page-header surface-card">
      <h3>{{ t('cluster.syncAudit.title') }}</h3>
      <p class="desc">{{ t('cluster.syncAudit.desc') }}</p>
    </section>

    <section class="stats-grid">
      <el-card
        v-for="card in statCards"
        :key="card.key"
        shadow="never"
        class="surface-card stat-card"
        :class="[activeCard === card.key ? 'is-active' : '', card.level === 'danger' ? 'is-danger' : card.level === 'warning' ? 'is-warning' : '']"
        @click="applyCardFilter(card.key)"
      >
        <p class="stat-label">{{ card.label }}</p>
        <strong class="stat-value">{{ card.value }}</strong>
        <el-tag size="small" class="status-chip stat-chip" :class="levelToStatusClass(card.level)">{{ card.level === 'danger' ? t('cluster.syncAudit.needAction') : card.level === 'warning' ? t('cluster.syncAudit.watching') : t('cluster.syncAudit.stable') }}</el-tag>
        <small class="stat-sub">{{ card.sub }}</small>
      </el-card>
    </section>

    <section class="surface-card table-card">
      <div class="row-between join-job-header">
        <div>
          <span class="card-title">{{ t('cluster.syncAudit.joinJobSection') }}</span>
          <p class="section-desc">{{ t('cluster.syncAudit.joinJobDesc') }}</p>
        </div>
        <div class="join-job-summary">
          <el-tag class="status-chip status-warning">{{ t('cluster.syncAudit.joinJobRunning', { n: joinJobSummary.running }) }}</el-tag>
          <el-tag class="status-chip status-success">{{ t('cluster.syncAudit.joinJobCompleted', { n: joinJobSummary.completed }) }}</el-tag>
          <el-tag class="status-chip status-danger">{{ t('cluster.syncAudit.joinJobFailed', { n: joinJobSummary.failed }) }}</el-tag>
          <el-tag class="status-chip status-muted">{{ t('cluster.syncAudit.joinJobCanceled', { n: joinJobSummary.canceled }) }}</el-tag>
        </div>
      </div>

      <el-table :data="joinJobs" border stripe :empty-text="t('cluster.syncAudit.joinJobEmpty')">
        <el-table-column prop="jobId" :label="t('cluster.syncAudit.colJobId')" min-width="210" show-overflow-tooltip />
        <el-table-column prop="nodeId" :label="t('cluster.syncAudit.colNode')" width="130" />
        <el-table-column prop="peerAddress" :label="t('cluster.syncAudit.colPeer')" min-width="180" show-overflow-tooltip />
        <el-table-column :label="t('cluster.syncAudit.colStatus')" width="120">
          <template #default="{ row }">
            <el-tag class="status-chip" :class="tagTypeClass(joinJobStatusTag(row.status))">{{ joinJobStatusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('cluster.syncAudit.colProgress')" width="180">
          <template #default="{ row }">
            <el-progress :percentage="Math.max(0, Math.min(100, row.progress || 0))" :stroke-width="8" />
          </template>
        </el-table-column>
        <el-table-column :label="t('cluster.syncAudit.colPhaseSummary')" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">{{ joinJobPhaseSummary(row) }}</template>
        </el-table-column>
        <el-table-column :label="t('cluster.syncAudit.colUpdateTime')" width="170">
          <template #default="{ row }">{{ formatTime(row.updatedAt) }}</template>
        </el-table-column>
        <el-table-column :label="t('cluster.syncAudit.colActions')" width="220" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openJoinJobDetail(row)">{{ t('cluster.syncAudit.detail') }}</el-button>
            <el-button
              v-if="joinJobCanRetry(row.status)"
              size="small"
              type="warning"
              plain
              :loading="joinJobBusyId === row.jobId"
              @click="retryJoinJob(row)"
            >
              {{ t('cluster.syncAudit.retry') }}
            </el-button>
            <el-button
              v-if="joinJobCanCancel(row.status)"
              size="small"
              type="danger"
              plain
              :loading="joinJobBusyId === row.jobId"
              @click="cancelJoinJob(row)"
            >
              {{ t('cluster.syncAudit.cancel') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <section class="surface-card table-card">
      <div class="filter-bar">
        <div class="bar-left filter-group">
          <el-select v-model="syncType" clearable size="small" :placeholder="t('cluster.syncAudit.syncTypePh')" style="width: 130px">
            <el-option :label="t('cluster.syncAudit.syncTypeLease')" value="lease" />
            <el-option :label="t('cluster.syncAudit.syncTypeConfig')" value="config" />
          </el-select>
          <el-select v-model="nodeFilter" clearable size="small" :placeholder="t('cluster.syncAudit.nodePh')" style="width: 170px">
            <el-option v-for="node in nodeOptions" :key="node" :label="node" :value="node" />
          </el-select>
          <el-select v-model="syncPhase" clearable size="small" :placeholder="t('cluster.syncAudit.syncPhasePh')" style="width: 130px">
            <el-option :label="t('cluster.syncAudit.phaseBndupd')" value="bndupd" />
            <el-option :label="t('cluster.syncAudit.phaseBndack')" value="bndack" />
            <el-option :label="t('cluster.syncAudit.phaseCommitted')" value="committed" />
            <el-option :label="t('cluster.syncAudit.phaseFailed')" value="failed" />
          </el-select>
          <el-select v-model="timeRange" size="small" :placeholder="t('cluster.syncAudit.timeRangePh')" style="width: 130px">
            <el-option :label="t('cluster.syncAudit.timeRange1h')" value="1h" />
            <el-option :label="t('cluster.syncAudit.timeRange24h')" value="24h" />
            <el-option :label="t('cluster.syncAudit.timeRange7d')" value="7d" />
          </el-select>
          <el-input v-model="nodeKeyword" clearable size="small" :placeholder="t('cluster.syncAudit.nodeKeywordPh')" style="width: 180px" />
        </div>
        <div class="bar-right filter-group filter-group-actions">
          <el-select v-model="retryStrategy" size="small" style="width: 120px" :placeholder="t('cluster.syncAudit.retryStrategyPh')">
            <el-option :label="t('cluster.syncAudit.retryNewTx')" value="new_tx" />
            <el-option :label="t('cluster.syncAudit.retrySameTx')" value="same_tx" />
          </el-select>
          <el-button size="small" type="warning" :loading="failoverPlanning" @click="runFailoverPlan">{{ t('cluster.syncAudit.runFailoverPlan') }}</el-button>
          <el-button size="small" :loading="loading" @click="loadData">{{ t('cluster.syncAudit.refresh') }}</el-button>
          <el-button size="small" :loading="syncing" @click="triggerFullSync">{{ t('cluster.syncAudit.triggerFullSync') }}</el-button>
          <el-button size="small" type="primary" :loading="checking" @click="runConsistencyCheck">{{ t('cluster.syncAudit.consistencyCheck') }}</el-button>
        </div>
      </div>
      <div class="filter-hint">{{ t('cluster.syncAudit.filterHintPrefix') }} {{ filterHint }}</div>

      <el-table :data="syncTable" border stripe>
        <el-table-column prop="id" :label="t('cluster.syncAudit.colSyncId')" min-width="150" />
        <el-table-column prop="syncType" :label="t('cluster.syncAudit.colSyncType')" width="110" />
        <el-table-column prop="triggerTime" :label="t('cluster.syncAudit.colTriggerTime')" width="170" />
        <el-table-column prop="activeNode" :label="t('cluster.syncAudit.colActiveNode')" width="130" />
        <el-table-column prop="standbyNode" :label="t('cluster.syncAudit.colStandbyNode')" width="130" />
        <el-table-column :label="t('cluster.syncAudit.colSyncStatus')" width="130">
          <template #default="{ row }">
            <el-tag class="status-chip" :class="tagTypeClass(stateTag(row.status))">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('cluster.syncAudit.colTxPhase')" width="120">
          <template #default="{ row }">
            <el-tag size="small" class="status-chip" :class="tagTypeClass(phaseTag(row.phase))">{{ phaseLabel(row.phase) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="duration" :label="t('cluster.syncAudit.colDuration')" width="90" />
        <el-table-column prop="detail" :label="t('cluster.syncAudit.colDetail')" min-width="200" show-overflow-tooltip />
        <el-table-column :label="t('cluster.syncAudit.colActions')" width="150" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="viewSyncDetail(row)">{{ t('cluster.syncAudit.detail') }}</el-button>
            <el-button
              v-if="row.status === t('cluster.syncAudit.syncStatusFailed')"
              size="small"
              type="danger"
              plain
              :loading="retryingId === row.id"
              @click="retrySync(row)"
            >
              {{ t('cluster.syncAudit.retrySync') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <section class="surface-card table-card">
      <div class="row-between">
        <span class="card-title">{{ t('cluster.syncAudit.latencySection') }}</span>
        <div class="mini-filters">
          <el-tag size="small" class="status-chip" :class="tagTypeClass(levelTag(comparisonSummary.ack.level))">ACK {{ comparisonSummary.ack.levelText }}</el-tag>
          <el-tag size="small" class="status-chip" :class="tagTypeClass(levelTag(comparisonSummary.mysql.level))">MySQL {{ comparisonSummary.mysql.levelText }}</el-tag>
          <el-tag size="small" class="status-chip" :class="tagTypeClass(levelTag(comparisonSummary.redis.level))">Redis {{ comparisonSummary.redis.levelText }}</el-tag>
          <el-button size="small" @click="exportLatencyComparison">{{ t('cluster.syncAudit.exportComparison') }}</el-button>
        </div>
      </div>

      <div class="trend-grid">
        <div class="trend-card">
          <div class="trend-title">{{ t('cluster.syncAudit.ackTrend') }}</div>
          <div v-for="point in trendPoints" :key="`ack-${point.key}`" class="trend-row">
            <span class="trend-time">{{ point.label }}</span>
            <div class="trend-bar-wrap">
              <div class="trend-bar" :class="levelClass(point.ackLevel)" :style="{ width: `${scalePercent(point.ackMs, ackScaleMax)}%` }" />
            </div>
            <span class="trend-value">{{ point.ackMs }} ms</span>
          </div>
        </div>

        <div class="trend-card">
          <div class="trend-title">{{ t('cluster.syncAudit.mysqlTrend') }}</div>
          <div v-for="point in trendPoints" :key="`mysql-${point.key}`" class="trend-row">
            <span class="trend-time">{{ point.label }}</span>
            <div class="trend-bar-wrap">
              <div class="trend-bar" :class="levelClass(point.mysqlLevel)" :style="{ width: `${scalePercent(point.mysqlMs, mysqlScaleMax)}%` }" />
            </div>
            <span class="trend-value">{{ point.mysqlMs }} ms</span>
          </div>
        </div>

        <div class="trend-card">
          <div class="trend-title">{{ t('cluster.syncAudit.redisTrend') }}</div>
          <div v-for="point in trendPoints" :key="`redis-${point.key}`" class="trend-row">
            <span class="trend-time">{{ point.label }}</span>
            <div class="trend-bar-wrap">
              <div class="trend-bar" :class="levelClass(point.redisLevel)" :style="{ width: `${scalePercent(point.redisLag, redisScaleMax)}%` }" />
            </div>
            <span class="trend-value">{{ point.redisLag }}</span>
          </div>
        </div>
      </div>

      <div class="compare-grid">
        <div class="compare-item" :class="levelClass(comparisonSummary.ack.level)">
          <strong>{{ t('cluster.syncAudit.ackLatency') }}</strong>
          <p>{{ t('cluster.syncAudit.beforeSwitch', { v: comparisonSummary.ack.before }) }}</p>
          <p>{{ t('cluster.syncAudit.afterSwitch', { v: comparisonSummary.ack.after }) }}</p>
          <p>{{ t('cluster.syncAudit.deltaSwitch', { v: signedDelta(comparisonSummary.ack.delta) }) }}</p>
        </div>
        <div class="compare-item" :class="levelClass(comparisonSummary.mysql.level)">
          <strong>{{ t('cluster.syncAudit.mysqlLatency') }}</strong>
          <p>{{ t('cluster.syncAudit.beforeSwitch', { v: comparisonSummary.mysql.before }) }}</p>
          <p>{{ t('cluster.syncAudit.afterSwitch', { v: comparisonSummary.mysql.after }) }}</p>
          <p>{{ t('cluster.syncAudit.deltaSwitch', { v: signedDelta(comparisonSummary.mysql.delta) }) }}</p>
        </div>
        <div class="compare-item" :class="levelClass(comparisonSummary.redis.level)">
          <strong>{{ t('cluster.syncAudit.redisOffset') }}</strong>
          <p>{{ t('cluster.syncAudit.beforeSwitchPlain', { v: comparisonSummary.redis.before }) }}</p>
          <p>{{ t('cluster.syncAudit.afterSwitchPlain', { v: comparisonSummary.redis.after }) }}</p>
          <p>{{ t('cluster.syncAudit.deltaSwitchPlain', { v: signedDelta(comparisonSummary.redis.delta) }) }}</p>
        </div>
      </div>
    </section>

    <section class="grid-2">
      <el-card shadow="never" class="surface-card table-card half-card">
        <template #header>
          <div class="row-between">
            <span class="card-title">{{ t('cluster.syncAudit.failoverHistory') }}</span>
            <div class="mini-filters">
              <el-select v-model="switchTypeFilter" clearable size="small" :placeholder="t('cluster.syncAudit.typePh')" style="width: 100px">
                <el-option :label="t('cluster.syncAudit.typeSuccess')" :value="t('cluster.syncAudit.resultSuccess')" />
                <el-option :label="t('cluster.syncAudit.typeFail')" :value="t('cluster.syncAudit.resultFailed')" />
              </el-select>
              <el-input v-model="switchKeyword" clearable size="small" :placeholder="t('cluster.syncAudit.operatorPh')" style="width: 110px" />
              <el-button size="small" @click="exportFailover">{{ t('cluster.syncAudit.export') }}</el-button>
            </div>
          </div>
        </template>

        <el-table :data="paginatedFailoverRows" border size="small" height="310" :empty-text="t('cluster.syncAudit.failoverEmpty')">
          <el-table-column prop="time" :label="t('cluster.syncAudit.colSwitchTime')" width="150" />
          <el-table-column prop="planId" :label="t('cluster.syncAudit.colPlanId')" width="140" show-overflow-tooltip />
          <el-table-column prop="reason" :label="t('cluster.syncAudit.colReason')" min-width="130" show-overflow-tooltip />
          <el-table-column prop="oldActive" :label="t('cluster.syncAudit.colOldActive')" width="110" />
          <el-table-column prop="newActive" :label="t('cluster.syncAudit.colNewActive')" width="110" />
          <el-table-column :label="t('cluster.syncAudit.colResult')" width="90">
            <template #default="{ row }">
              <el-tag class="status-chip" :class="tagTypeClass(planResultTag(row.result))">{{ row.result }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="failedStep" :label="t('cluster.syncAudit.colFailedStep')" width="110" show-overflow-tooltip />
          <el-table-column prop="rollback" :label="t('cluster.syncAudit.colRollback')" width="100" />
          <el-table-column prop="operator" :label="t('cluster.syncAudit.colOperator')" width="90" />
          <el-table-column :label="t('cluster.syncAudit.colActions')" width="100" fixed="right">
            <template #default="{ row }">
              <el-button size="small" @click="openPlanProgress(row)">{{ t('cluster.syncAudit.stepProgress') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <div class="table-pagination">
          <el-pagination
            small
            background
            layout="prev, pager, next"
            :current-page="failoverPage"
            :page-size="historyPageSize"
            :total="filteredFailoverRows.length"
            @current-change="failoverPage = $event"
          />
        </div>
      </el-card>

      <el-card shadow="never" class="surface-card table-card half-card">
        <template #header>
          <div class="row-between">
            <span class="card-title">{{ t('cluster.syncAudit.auditSection') }}</span>
            <div class="mini-filters">
              <el-input v-model="auditKeyword" clearable size="small" :placeholder="t('cluster.syncAudit.auditKeywordPh')" style="width: 140px" />
              <el-button size="small" @click="exportAudit">{{ t('cluster.syncAudit.export') }}</el-button>
            </div>
          </div>
        </template>

        <el-table :data="paginatedAuditRows" border size="small" height="310" :empty-text="t('cluster.syncAudit.auditEmpty')">
          <el-table-column prop="operator" :label="t('cluster.syncAudit.colAuditOperator')" width="100" />
          <el-table-column prop="time" :label="t('cluster.syncAudit.colAuditTime')" width="150" />
          <el-table-column prop="action" :label="t('cluster.syncAudit.colAuditAction')" min-width="160" show-overflow-tooltip />
          <el-table-column prop="ip" :label="t('cluster.syncAudit.colAuditIp')" width="120" />
        </el-table>
        <div class="table-pagination">
          <el-pagination
            small
            background
            layout="prev, pager, next"
            :current-page="auditPage"
            :page-size="historyPageSize"
            :total="filteredAuditRows.length"
            @current-change="auditPage = $event"
          />
        </div>
      </el-card>
    </section>

    <section class="surface-card table-card">
      <div class="row-between">
        <span class="card-title">{{ t('cluster.syncAudit.backupSection') }}</span>
        <el-button type="primary" :loading="backuping" @click="runBackup">{{ t('cluster.syncAudit.runBackup') }}</el-button>
      </div>

      <el-form ref="backupFormRef" :model="backupPlan" :rules="backupRules" label-width="120px" label-position="left" class="backup-form">
        <el-form-item :label="t('cluster.syncAudit.backupFreq')" prop="freq" required class="backup-span-1">
          <el-select v-model="backupPlan.freq" style="width: 160px">
            <el-option :label="t('cluster.syncAudit.freqDaily')" value="daily" />
            <el-option :label="t('cluster.syncAudit.freqWeekly')" value="weekly" />
            <el-option :label="t('cluster.syncAudit.freqMonthly')" value="monthly" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('cluster.syncAudit.backupRetain')" prop="retain" required class="backup-span-1">
          <el-select v-model="backupPlan.retain" style="width: 160px">
            <el-option :label="t('cluster.syncAudit.retain7')" value="7" />
            <el-option :label="t('cluster.syncAudit.retain30')" value="30" />
            <el-option :label="t('cluster.syncAudit.retain90')" value="90" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('cluster.syncAudit.backupContents')" prop="contents" required class="backup-span-2">
          <el-checkbox-group v-model="backupPlan.contents">
            <el-checkbox :label="t('cluster.syncAudit.contentConfig')" />
            <el-checkbox :label="t('cluster.syncAudit.contentLease')" />
            <el-checkbox :label="t('cluster.syncAudit.contentAudit')" />
          </el-checkbox-group>
        </el-form-item>
        <el-form-item :label="t('cluster.syncAudit.backupStorage')" prop="storage" required class="backup-span-1">
          <el-select v-model="backupPlan.storage" style="width: 180px">
            <el-option :label="t('cluster.syncAudit.storageLocal')" value="local" />
            <el-option label="NAS" value="nas" />
            <el-option :label="t('cluster.syncAudit.storageCloud')" value="cloud" />
            <el-option :label="t('cluster.syncAudit.storageHybrid')" value="hybrid" />
          </el-select>
        </el-form-item>
      </el-form>

      <el-progress v-if="progress > 0" :percentage="progress" :stroke-width="10" class="backup-progress" />

      <el-table :data="backupRows" border stripe>
        <el-table-column prop="id" :label="t('cluster.syncAudit.colBackupId')" width="140" />
        <el-table-column prop="time" :label="t('cluster.syncAudit.colBackupTime')" width="170" />
        <el-table-column prop="content" :label="t('cluster.syncAudit.colBackupContent')" min-width="180" />
        <el-table-column prop="size" :label="t('cluster.syncAudit.colBackupSize')" width="100" />
        <el-table-column prop="status" :label="t('cluster.syncAudit.colBackupStatus')" width="100" />
        <el-table-column :label="t('cluster.syncAudit.colActions')" width="220" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="downloadBackup(row)">{{ t('cluster.syncAudit.download') }}</el-button>
            <el-button size="small" type="warning" plain @click="restoreBackup(row)">{{ t('cluster.syncAudit.restore') }}</el-button>
            <el-button size="small" type="danger" plain @click="deleteBackup(row)">{{ t('cluster.syncAudit.delete') }}</el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <el-drawer v-model="drawerVisible" :title="t('cluster.syncAudit.drawerSyncDetail')" size="42%">
      <pre class="detail-pre">{{ JSON.stringify(detailRow, null, 2) }}</pre>
    </el-drawer>

    <el-drawer v-model="planDrawerVisible" :title="t('cluster.syncAudit.drawerPlanSteps')" size="46%">
      <template v-if="selectedPlan">
        <el-descriptions :column="2" border class="plan-meta">
          <el-descriptions-item :label="t('cluster.syncAudit.planId')">{{ selectedPlan.planId }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.planStatus')">
            <el-tag class="status-chip" :class="tagTypeClass(planStatusTag(selectedPlan.status))">{{ selectedPlan.status }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.sourceNode')">{{ selectedPlan.sourceNode || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.targetNode')">{{ selectedPlan.targetNode || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.failedStep')">{{ selectedPlan.failedStep || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.rollbackStatus')">{{ selectedPlan.rollbackStatus || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.createdAt')">{{ formatTime(selectedPlan.createdAt) }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.updatedAt')">{{ formatTime(selectedPlan.updatedAt) }}</el-descriptions-item>
        </el-descriptions>

        <el-progress :percentage="planProgress(selectedPlan)" :stroke-width="10" class="plan-progress" />

        <el-table :data="selectedPlan.steps || []" border size="small" class="plan-step-table">
          <el-table-column prop="title" :label="t('cluster.syncAudit.colStep')" min-width="140" />
          <el-table-column :label="t('cluster.syncAudit.colStepStatus')" width="100">
            <template #default="{ row }">
              <el-tag size="small" class="status-chip" :class="tagTypeClass(planStepTag(row.status))">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('cluster.syncAudit.colDurationMs')" width="100">
            <template #default="{ row }">{{ row.durationMs || 0 }}</template>
          </el-table-column>
          <el-table-column :label="t('cluster.syncAudit.colError')" min-width="180" show-overflow-tooltip>
            <template #default="{ row }">{{ row.error || row.detail || '-' }}</template>
          </el-table-column>
        </el-table>
      </template>
      <el-empty v-else :description="t('cluster.syncAudit.noStepData')" :image-size="52" />
    </el-drawer>

    <el-drawer v-model="joinJobDrawerVisible" :title="t('cluster.syncAudit.drawerJoinDetail')" size="42%">
      <template v-if="selectedJoinJob">
        <el-descriptions :column="2" border class="plan-meta">
          <el-descriptions-item :label="t('cluster.syncAudit.joinJobId')">{{ selectedJoinJob.jobId }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.colStatus')">
            <el-tag class="status-chip" :class="tagTypeClass(joinJobStatusTag(selectedJoinJob.status))">{{ joinJobStatusLabel(selectedJoinJob.status) }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.joinNode')">{{ selectedJoinJob.nodeId }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.joinPeer')">{{ selectedJoinJob.peerAddress || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.joinCreated')">{{ formatTime(selectedJoinJob.createdAt) }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.joinUpdated')">{{ formatTime(selectedJoinJob.updatedAt) }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.joinCheckpoint')">{{ joinJobCheckpointHeadline(selectedJoinJob) }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.joinCatchUp')">{{ joinJobCatchUpHeadline(selectedJoinJob) }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.joinError')" :span="2">{{ selectedJoinJob.error || '-' }}</el-descriptions-item>
        </el-descriptions>

        <el-descriptions v-if="selectedJoinJob.snapshot" :column="2" border class="plan-meta join-verify-meta">
          <el-descriptions-item :label="t('cluster.syncAudit.snapshotStatus')">{{ selectedJoinJob.snapshot.status || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.snapshotTime')">{{ formatTime(selectedJoinJob.snapshot.capturedAt) }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.snapshotSourceNode')">{{ selectedJoinJob.snapshot.sourceNode || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.snapshotSourceAddr')">{{ selectedJoinJob.snapshot.sourceAddress || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.snapshotPoolBinding')">{{ `${selectedJoinJob.snapshot.poolCount ?? 0} / ${selectedJoinJob.snapshot.bindingCount ?? 0}` }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.snapshotLeaseCount')">{{ selectedJoinJob.snapshot.leaseCount ?? 0 }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.snapshotOffset')">{{ selectedJoinJob.snapshot.replicationOffset || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.snapshotSyncSummary')">{{ joinJobSnapshotSyncSummary(selectedJoinJob) }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.snapshotChecksum')" :span="2">{{ joinJobSnapshotChecksumSummary(selectedJoinJob) }}</el-descriptions-item>
        </el-descriptions>

        <el-descriptions v-if="selectedJoinJob.catchUp" :column="2" border class="plan-meta join-verify-meta">
          <el-descriptions-item :label="t('cluster.syncAudit.catchUpStatus')">{{ selectedJoinJob.catchUp.status || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.catchUpResult')">{{ selectedJoinJob.catchUp.highWatermarkReached ? t('cluster.syncAudit.catchUpReached') : t('cluster.syncAudit.catchUpNotReached') }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.catchUpBaseline')">{{ selectedJoinJob.catchUp.baselineOffset || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.catchUpTarget')">{{ selectedJoinJob.catchUp.targetOffset || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.catchUpCurrent')">{{ selectedJoinJob.catchUp.currentOffset || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.catchUpStarted')">{{ formatTime(selectedJoinJob.catchUp.startedAt) }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.catchUpUpdated')">{{ formatTime(selectedJoinJob.catchUp.updatedAt) }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.catchUpCompleted')">{{ formatTime(selectedJoinJob.catchUp.completedAt) }}</el-descriptions-item>
        </el-descriptions>

        <el-descriptions v-if="selectedJoinJob.checkpoint" :column="2" border class="plan-meta join-verify-meta">
          <el-descriptions-item :label="t('cluster.syncAudit.checkpointPhase')">{{ joinJobStatusLabel(selectedJoinJob.checkpoint.phase) }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.checkpointStatus')">{{ selectedJoinJob.checkpoint.status || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.checkpointLastPhase')">{{ joinJobStatusLabel(selectedJoinJob.checkpoint.lastCompletedPhase) }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.checkpointResumable')">{{ selectedJoinJob.checkpoint.resumable ? t('cluster.syncAudit.checkpointResumableYes') : t('cluster.syncAudit.checkpointResumableNo') }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.checkpointResumeCount')">{{ selectedJoinJob.checkpoint.resumeCount ?? 0 }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.checkpointUpdated')">{{ formatTime(selectedJoinJob.checkpoint.updatedAt) }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.checkpointError')" :span="2">{{ selectedJoinJob.checkpoint.lastError || '-' }}</el-descriptions-item>
        </el-descriptions>

        <el-descriptions v-if="selectedJoinJob.verification" :column="2" border class="plan-meta join-verify-meta">
          <el-descriptions-item :label="t('cluster.syncAudit.verifyConclusion')">
            <el-tag class="status-chip" :class="selectedJoinJob.verification.status === 'VERIFIED' ? 'status-success' : 'status-danger'">
              {{ selectedJoinJob.verification.status === 'VERIFIED' ? t('cluster.syncAudit.verifyPassed') : t('cluster.syncAudit.verifyFailed') }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifyTime')">{{ formatTime(selectedJoinJob.verification.checkedAt) }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifyPoolCount')">{{ selectedJoinJob.verification.poolCount ?? 0 }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifyBindingCount')">{{ selectedJoinJob.verification.bindingCount ?? 0 }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifyLeaseCount')">{{ selectedJoinJob.verification.leaseCount ?? 0 }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifyActiveLeaseCount')">{{ selectedJoinJob.verification.activeLeaseCount ?? 0 }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifyConflict24h')">{{ selectedJoinJob.verification.conflictLeaseCount24h ?? 0 }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifyCreated24h')">{{ selectedJoinJob.verification.createdLeaseCount24h ?? 0 }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifyReplication')">
            <el-tag class="status-chip" :class="selectedJoinJob.verification.replicationHealthy ? 'status-success' : 'status-danger'">
              {{ selectedJoinJob.verification.replicationHealthy ? t('cluster.syncAudit.verifyRepHealthy') : t('cluster.syncAudit.verifyRepUnhealthy') }}
            </el-tag>
            <span class="metric-inline">Lag {{ selectedJoinJob.verification.replicationLagMs ?? 0 }} ms</span>
          </el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifyRepMode')">{{ selectedJoinJob.verification.replicationMode || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifyRepSource')">{{ selectedJoinJob.verification.replicationSource || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifyRepState')">{{ selectedJoinJob.verification.replicationState || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifyRepOffset')">{{ selectedJoinJob.verification.replicationOffset || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifyFencingEpoch')">{{ selectedJoinJob.verification.fencingEpoch || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifySyncSummary')">{{ joinJobSyncSummary(selectedJoinJob) }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifyChecksum')">{{ selectedJoinJob.verification.consistencyChecksum || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('cluster.syncAudit.verifyWarning')" :span="2">{{ selectedJoinJob.verification.warning || selectedJoinJob.verification.failureReason || '-' }}</el-descriptions-item>
        </el-descriptions>

        <el-table :data="selectedJoinJob.phases || []" border size="small" class="plan-step-table">
          <el-table-column prop="phase" :label="t('cluster.syncAudit.colPhase')" min-width="140">
            <template #default="{ row }">{{ joinJobStatusLabel(row.phase) }}</template>
          </el-table-column>
          <el-table-column :label="t('cluster.syncAudit.colStepStatus')" width="100">
            <template #default="{ row }">
              <el-tag size="small" class="status-chip" :class="tagTypeClass(joinPhaseTag(row.status))">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('cluster.syncAudit.colStartTime')" min-width="150">
            <template #default="{ row }">{{ formatTime(row.startedAt) }}</template>
          </el-table-column>
          <el-table-column :label="t('cluster.syncAudit.colDurationMs')" width="100">
            <template #default="{ row }">{{ row.durationMs || 0 }}</template>
          </el-table-column>
          <el-table-column :label="t('cluster.syncAudit.colError')" min-width="180" show-overflow-tooltip>
            <template #default="{ row }">{{ row.error || '-' }}</template>
          </el-table-column>
        </el-table>
      </template>
      <el-empty v-else :description="t('cluster.syncAudit.noJobDetail')" :image-size="52" />
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { FormInstance, FormRules } from 'element-plus';
import { ElMessageBox } from 'element-plus';
import {
  cancelClusterJoinJob,
  createClusterFailoverPlan,
  getClusterBackupHistory,
  getClusterBackupPlan,
  getClusterFailoverHistory,
  getClusterFailoverPlan,
  getClusterFailoverPlans,
  getClusterJoinJob,
  getClusterJoinJobs,
  getClusterScaleEvents,
  getClusterSyncStats,
  getClusterSyncTransaction,
  getClusterSyncStatus,
  getClusterSyncTransactions,
  retryClusterJoinJob,
  retryClusterSyncTransaction,
  runClusterBackup,
  runClusterRestore,
  saveClusterBackupPlan,
  triggerClusterSync
} from '@/api/cluster';
import type {
  ClusterBackupPlan,
  ClusterBackupRecord,
  ClusterFailoverEvent,
  ClusterFailoverPlan,
  ClusterJoinJob,
  ClusterSyncStats,
  ClusterSyncStatus,
  ClusterSyncTransaction
} from '@/types/cluster';
import { showHttpError } from '@/shared/errors/errorToast';
import { showSuccess, showWarning } from '@/shared/errors/messageToast';

const { t } = useI18n();

interface SyncRow {
  id: string;
  txId?: string;
  syncType: string;
  triggerTime: string;
  activeNode: string;
  standbyNode: string;
  status: string;
  duration: string;
  phase?: string;
  bndupdAt?: string;
  bndackAt?: string;
  ackAt?: string;
  errorCode?: string;
  sourceNode?: string;
  targetNode?: string;
  reconcileLastRun?: string;
  reconcileLastErr?: string;
  reconcileSuccessTotal?: number;
  reconcileFailureTotal?: number;
  reconcileFailureStreak?: number;
  reconcileIntervalSec?: number;
  reconcileAlertThreshold?: number;
  mysqlLagMs?: number;
  mysqlLagLevel?: MetricLevel;
  mysqlLagWarnMs?: number;
  mysqlLagCriticalMs?: number;
  redisOffsetLag?: number;
  redisOffsetLagLevel?: MetricLevel;
  redisOffsetLagWarn?: number;
  redisOffsetLagCritical?: number;
  rfc6853AckLatencyMs?: number;
  rfc6853AckLatencyLevel?: MetricLevel;
  rfc6853AckWarnMs?: number;
  rfc6853AckCriticalMs?: number;
  detail: string;
}

type StatKey = 'lease' | 'config' | 'tp' | 'reconcile';
type MetricLevel = 'healthy' | 'warning' | 'critical';

const loading = ref(false);
const syncing = ref(false);
const checking = ref(false);
const failoverPlanning = ref(false);
const backuping = ref(false);
const retryingId = ref('');
const joinJobBusyId = ref('');
const progress = ref(0);

const syncType = ref('');
const syncPhase = ref('');
const nodeFilter = ref('');
const retryStrategy = ref<'new_tx' | 'same_tx'>('new_tx');
const timeRange = ref('24h');
const nodeKeyword = ref('');
const activeCard = ref<StatKey>('lease');

const switchTypeFilter = ref('');
const switchKeyword = ref('');
const auditKeyword = ref('');
const failoverPage = ref(1);
const auditPage = ref(1);
const historyPageSize = 6;

const syncRows = ref<SyncRow[]>([]);
const syncTransactions = ref<ClusterSyncTransaction[]>([]);
const syncStats = ref<ClusterSyncStats>({
  windowHours: 24,
  total: 0,
  committed: 0,
  failed: 0,
  ackTimeoutCount: 0,
  successRatePercent: 100,
  avgAckLatencyMs: 0,
  p95AckLatencyMs: 0
});
const failoverEvents = ref<ClusterFailoverEvent[]>([]);
const failoverPlans = ref<ClusterFailoverPlan[]>([]);
const auditEvents = ref<ClusterFailoverEvent[]>([]);
const backupRowsRaw = ref<ClusterBackupRecord[]>([]);
const joinJobs = ref<ClusterJoinJob[]>([]);

const drawerVisible = ref(false);
const detailRow = ref<SyncRow | null>(null);
const planDrawerVisible = ref(false);
const selectedPlan = ref<ClusterFailoverPlan | null>(null);
const joinJobDrawerVisible = ref(false);
const selectedJoinJob = ref<ClusterJoinJob | null>(null);

const backupFormRef = ref<FormInstance>();
const backupPlan = reactive<ClusterBackupPlan>({
  freq: 'daily',
  retain: '30',
  storage: 'local',
  contents: ['config_data', 'lease_data']
});

const backupRules: FormRules = {
  freq: [{ required: true, message: () => t('cluster.syncAudit.backupRuleFreq'), trigger: 'change' }],
  retain: [{ required: true, message: () => t('cluster.syncAudit.backupRuleRetain'), trigger: 'change' }],
  storage: [{ required: true, message: () => t('cluster.syncAudit.backupRuleStorage'), trigger: 'change' }],
  contents: [{ type: 'array', required: true, message: () => t('cluster.syncAudit.backupRuleContents'), trigger: 'change' }]
};

const unwrap = <T>(resp: any): T => (resp?.data?.data ?? resp?.data ?? resp) as T;

const statsHours = computed(() => {
  if (timeRange.value === '1h') return 1;
  if (timeRange.value === '7d') return 24 * 7;
  return 24;
});

const syncTable = computed(() =>
  syncRows.value.filter((row) => {
    const byType = syncType.value ? row.syncType.includes(syncType.value === 'lease' ? t('cluster.syncAudit.leaseSyncType') : t('cluster.syncAudit.configSyncType')) : true;
    const byPhase = syncPhase.value ? String(row.phase || '').toLowerCase().includes(syncPhase.value) : true;
    const byNodeInput = nodeKeyword.value ? `${row.activeNode} ${row.standbyNode}`.includes(nodeKeyword.value.trim()) : true;
    const byNodeSelect = nodeFilter.value ? `${row.activeNode} ${row.standbyNode}`.includes(nodeFilter.value) : true;
    const byNode = byNodeInput && byNodeSelect;
    return byType && byPhase && byNode;
  })
);

const nodeOptions = computed(() => {
  const set = new Set<string>();
  syncRows.value.forEach((row) => {
    if (row.activeNode && row.activeNode !== '-') set.add(row.activeNode);
    if (row.standbyNode && row.standbyNode !== '-') set.add(row.standbyNode);
  });
  return Array.from(set);
});

const trendBaseRows = computed(() => syncTable.value.slice(0, 20).reverse());

const trendPoints = computed(() =>
  trendBaseRows.value.map((row, idx) => ({
    key: `${row.id}-${idx}`,
    label: row.triggerTime,
    phase: row.phase || '-',
    source: row.activeNode,
    target: row.standbyNode,
    ackMs: Math.max(0, Number(row.rfc6853AckLatencyMs ?? parseLatencyMs(row.duration))),
    ackLevel: normalizeMetricLevel(row.rfc6853AckLatencyLevel),
    mysqlMs: Math.max(0, Number(row.mysqlLagMs ?? 0)),
    mysqlLevel: normalizeMetricLevel(row.mysqlLagLevel),
    redisLag: Math.max(0, Number(row.redisOffsetLag ?? 0)),
    redisLevel: normalizeMetricLevel(row.redisOffsetLagLevel)
  }))
);

const ackScaleMax = computed(() => Math.max(100, ...trendPoints.value.map((p) => p.ackMs)));
const mysqlScaleMax = computed(() => Math.max(100, ...trendPoints.value.map((p) => p.mysqlMs)));
const redisScaleMax = computed(() => Math.max(100, ...trendPoints.value.map((p) => p.redisLag)));

const latestSwitchAt = computed(() => {
  const point = filteredFailoverRows.value[0]?.time;
  if (!point || point === '-') return 0;
  const ts = new Date(point).getTime();
  return Number.isFinite(ts) ? ts : 0;
});

const comparisonSummary = computed(() => {
  const cut = latestSwitchAt.value;
  const beforeRows = trendPoints.value.filter((p) => {
    const ts = new Date(p.label).getTime();
    return Number.isFinite(ts) && ts <= cut;
  });
  const afterRows = trendPoints.value.filter((p) => {
    const ts = new Date(p.label).getTime();
    return Number.isFinite(ts) && ts > cut;
  });
  const fallback = trendPoints.value;
  const before = beforeRows.length ? beforeRows : fallback;
  const after = afterRows.length ? afterRows : fallback;

  const ackBefore = avgOf(before.map((p) => p.ackMs));
  const ackAfter = avgOf(after.map((p) => p.ackMs));
  const mysqlBefore = avgOf(before.map((p) => p.mysqlMs));
  const mysqlAfter = avgOf(after.map((p) => p.mysqlMs));
  const redisBefore = avgOf(before.map((p) => p.redisLag));
  const redisAfter = avgOf(after.map((p) => p.redisLag));

  return {
    ack: buildCompareMetric(ackBefore, ackAfter, trendPoints.value.map((p) => p.ackLevel)),
    mysql: buildCompareMetric(mysqlBefore, mysqlAfter, trendPoints.value.map((p) => p.mysqlLevel)),
    redis: buildCompareMetric(redisBefore, redisAfter, trendPoints.value.map((p) => p.redisLevel))
  };
});

const leaseSynced = computed(() => syncRows.value.filter((item) => item.status === t('cluster.syncAudit.syncStatusComplete')).length);
const leasePending = computed(() => syncRows.value.filter((item) => item.status !== t('cluster.syncAudit.syncStatusComplete')).length);
const latestSyncTime = computed(() => syncRows.value[0]?.triggerTime || '-');

const avgLatency = computed(() => {
  if (syncStats.value.avgAckLatencyMs > 0) return syncStats.value.avgAckLatencyMs;
  if (!syncRows.value.length) return 0;
  const nums = syncRows.value.map((item) => Number(item.duration.replace('ms', '')) || 0);
  return Math.round(nums.reduce((a, b) => a + b, 0) / nums.length);
});

const configState = computed(() => (syncRows.value.some((item) => item.status === t('cluster.syncAudit.syncStatusFailed')) ? t('cluster.syncAudit.statConfigInconsistent') : t('cluster.syncAudit.statConfigConsistent')));
const tpFailCount = computed(() => syncRows.value.filter((item) => item.status === t('cluster.syncAudit.syncStatusFailed')).length);
const tpSuccessRate = computed(() => {
  if (syncStats.value.total > 0) return Math.round(syncStats.value.successRatePercent);
  if (!syncRows.value.length) return 100;
  return Math.round(((syncRows.value.length - tpFailCount.value) / syncRows.value.length) * 100);
});

const reconcileMeta = computed(() => {
  const row = syncRows.value[0];
  return {
    success: row?.reconcileSuccessTotal ?? 0,
    failure: row?.reconcileFailureTotal ?? 0,
    streak: row?.reconcileFailureStreak ?? 0,
    intervalSec: row?.reconcileIntervalSec ?? 0,
    threshold: row?.reconcileAlertThreshold ?? 0,
    lastRun: row?.reconcileLastRun || '-',
    lastErr: row?.reconcileLastErr || ''
  };
});

const statCards = computed(() => [
  {
    key: 'lease' as StatKey,
    label: t('cluster.syncAudit.statLeaseSyncLabel'),
    value: t('cluster.syncAudit.statLeaseSyncValue', { synced: leaseSynced.value, pending: leasePending.value }),
    sub: t('cluster.syncAudit.statLeaseSyncSub', { time: latestSyncTime.value, ms: avgLatency.value }),
    level: leasePending.value > 0 ? 'warning' : 'normal'
  },
  {
    key: 'config' as StatKey,
    label: t('cluster.syncAudit.statConfigSyncLabel'),
    value: configState.value,
    sub: t('cluster.syncAudit.statConfigSyncSub'),
    level: configState.value === t('cluster.syncAudit.statConfigInconsistent') ? 'danger' : 'normal'
  },
  {
    key: 'tp' as StatKey,
    label: t('cluster.syncAudit.statTpLabel'),
    value: `${tpSuccessRate.value}%`,
    sub: t('cluster.syncAudit.statTpSub', { hours: syncStats.value.windowHours || statsHours.value, failed: syncStats.value.failed || tpFailCount.value, timeout: syncStats.value.ackTimeoutCount, p95: syncStats.value.p95AckLatencyMs || 0 }),
    level: (syncStats.value.failed || tpFailCount.value) > 0 ? 'danger' : 'normal'
  },
  {
    key: 'reconcile' as StatKey,
    label: t('cluster.syncAudit.statReconcileLabel'),
    value: t('cluster.syncAudit.statReconcileValue', { success: reconcileMeta.value.success, failure: reconcileMeta.value.failure }),
    sub: t('cluster.syncAudit.statReconcileSub', { streak: reconcileMeta.value.streak, threshold: reconcileMeta.value.threshold || '-', interval: reconcileMeta.value.intervalSec || '-', lastRun: formatTime(reconcileMeta.value.lastRun), lastErr: reconcileMeta.value.lastErr ? t('cluster.syncAudit.statReconcileErr', { err: reconcileMeta.value.lastErr }) : '' }),
    level: reconcileMeta.value.streak > 0 ? 'warning' : 'normal'
  }
]);

const joinJobSummary = computed(() => ({
  running: joinJobs.value.filter((job) => joinJobCanCancel(job.status)).length,
  completed: joinJobs.value.filter((job) => ['WARM_STANDBY', 'ACTIVE'].includes(String(job.status || '').toUpperCase())).length,
  failed: joinJobs.value.filter((job) => String(job.status || '').toUpperCase() === 'FAILED').length,
  canceled: joinJobs.value.filter((job) => String(job.status || '').toUpperCase() === 'CANCELED').length
}));

const failoverRows = computed(() =>
  failoverPlans.value.map((plan) => ({
    time: formatTime(plan.updatedAt || plan.createdAt),
    reason: plan.reason || t('cluster.syncAudit.failoverReason'),
    oldActive: plan.sourceNode || '-',
    newActive: plan.targetNode || '-',
    result: plan.status === 'SUCCESS' || plan.status === 'ROLLED_BACK' ? t('cluster.syncAudit.resultSuccess') : plan.status === 'RUNNING' || plan.status === 'PENDING' ? t('cluster.syncAudit.resultRunning') : t('cluster.syncAudit.resultFailed'),
    operator: '-',
    failedStep: plan.failedStep || '-',
    rollback: plan.rollbackTriggered ? plan.rollbackStatus || 'TRIGGERED' : '-',
    planId: plan.planId
  }))
);

const planProgress = (plan?: ClusterFailoverPlan | null) => {
  if (!plan?.steps?.length) return 0;
  const done = plan.steps.filter((s) => ['SUCCESS', 'FAILED', 'SKIPPED'].includes(String(s.status || '').toUpperCase())).length;
  return Math.max(0, Math.min(100, Math.round((done / plan.steps.length) * 100)));
};

const filteredFailoverRows = computed(() =>
  failoverRows.value.filter((item) => {
    const byType = switchTypeFilter.value ? item.result === switchTypeFilter.value : true;
    const byKeyword = switchKeyword.value ? item.operator.includes(switchKeyword.value.trim()) : true;
    return byType && byKeyword;
  })
);
const paginatedFailoverRows = computed(() => {
  const start = (failoverPage.value - 1) * historyPageSize;
  return filteredFailoverRows.value.slice(start, start + historyPageSize);
});

const auditRows = computed(() =>
  auditEvents.value.slice(0, 30).map((item) => ({
    operator: parseDetailValue(item.detail, ['operator', 'actor', 'user']) || '-',
    time: formatTime(item.time),
    action: item.detail || item.title,
    ip: parseDetailValue(item.detail, ['ip', 'clientIp', 'sourceIp']) || '-'
  }))
);

const filteredAuditRows = computed(() =>
  auditRows.value.filter((item) => {
    if (!auditKeyword.value) return true;
    const kw = auditKeyword.value.trim();
    return item.operator.includes(kw) || item.action.includes(kw) || item.ip.includes(kw);
  })
);
const paginatedAuditRows = computed(() => {
  const start = (auditPage.value - 1) * historyPageSize;
  return filteredAuditRows.value.slice(start, start + historyPageSize);
});

const backupRows = computed(() =>
  backupRowsRaw.value.map((item, idx) => ({
    id: item.txId || item.time || `row-${idx + 1}`,
    time: formatTime(item.time),
    rawTime: item.time,
    txId: item.txId,
    content: item.type,
    size: item.size,
    status: item.status,
    phase: item.phase,
    errorCode: item.errorCode
  }))
);

const filterHint = computed(() => {
  const scope = timeRange.value === '1h' ? t('cluster.syncAudit.timeRange1h') : timeRange.value === '7d' ? t('cluster.syncAudit.timeRange7d') : t('cluster.syncAudit.timeRange24h');
  const type = syncType.value === 'lease' ? t('cluster.syncAudit.syncTypeLease') : syncType.value === 'config' ? t('cluster.syncAudit.syncTypeConfig') : t('cluster.syncAudit.filterScopeAll');
  const phase = syncPhase.value === 'bndupd' ? t('cluster.syncAudit.phaseBndupd') : syncPhase.value === 'bndack' ? t('cluster.syncAudit.phaseBndack') : syncPhase.value === 'committed' ? t('cluster.syncAudit.phaseCommitted') : syncPhase.value === 'failed' ? t('cluster.syncAudit.filterFailedPhase') : t('cluster.syncAudit.filterPhaseAll');
  const node = nodeFilter.value || t('cluster.syncAudit.filterNodeAll');
  return `${scope} · ${type} · ${phase} · ${node}`;
});

const applyCardFilter = (key: StatKey) => {
  activeCard.value = key;
  if (key === 'lease') {
    syncType.value = 'lease';
    syncPhase.value = '';
    if (timeRange.value !== '1h') {
      timeRange.value = '1h';
      return;
    }
    loadData();
  }
  if (key === 'config') {
    syncType.value = 'config';
    syncPhase.value = '';
    if (timeRange.value !== '24h') {
      timeRange.value = '24h';
      return;
    }
    loadData();
  }
  if (key === 'tp') {
    syncType.value = '';
    syncPhase.value = 'failed';
    if (timeRange.value !== '24h') {
      timeRange.value = '24h';
      return;
    }
    loadData();
  }
  if (key === 'reconcile') {
    syncType.value = '';
    syncPhase.value = '';
    if (timeRange.value !== '24h') {
      timeRange.value = '24h';
      return;
    }
    loadData();
  }
};

watch([switchTypeFilter, switchKeyword], () => {
  failoverPage.value = 1;
});

watch(auditKeyword, () => {
  auditPage.value = 1;
});

const loadData = async () => {
  loading.value = true;
  try {
    const [syncResp, statsResp, txResp, eventsResp, failoverResp, failoverPlansResp, planResp, backupsResp, joinJobsResp] = await Promise.allSettled([
      getClusterSyncStatus(),
      getClusterSyncStats(statsHours.value),
      getClusterSyncTransactions(),
      getClusterScaleEvents(),
      getClusterFailoverHistory(),
      getClusterFailoverPlans(),
      getClusterBackupPlan(),
      getClusterBackupHistory(),
      getClusterJoinJobs()
    ]);

    const failedBlocks: string[] = [];
    const unwrapSettled = <T>(resp: PromiseSettledResult<any>, fallback: T, label: string): T => {
      if (resp.status === 'fulfilled') {
        return (unwrap<T>(resp.value) || fallback) as T;
      }
      failedBlocks.push(label);
      return fallback;
    };

    const statuses = unwrapSettled<ClusterSyncStatus[]>(syncResp, [], 'SyncStatus');
    syncStats.value = unwrapSettled<ClusterSyncStats>(statsResp, syncStats.value, 'SyncStats');
    syncTransactions.value = unwrapSettled<ClusterSyncTransaction[]>(txResp, [], 'SyncTx');
    const latestTx = syncTransactions.value[0];
    syncRows.value = statuses.map((item, idx) => ({
      id: `SYNC-${idx + 1}`,
      txId: item.lastTxId || latestTx?.txId,
      syncType: String(item.type || '').includes('config') ? t('cluster.syncAudit.configSyncType') : t('cluster.syncAudit.leaseSyncType'),
      triggerTime: formatTime(item.last),
      activeNode: item.sourceNode || latestTx?.sourceNode || '-',
      standbyNode: item.targetNode || latestTx?.targetNode || '-',
      status: mapSyncStatus(item, latestTx),
      duration: `${Math.max(1, Number(String(item.latency).replace(/[^\d]/g, '')) || 0)}ms`,
      phase: item.phase || latestTx?.phase,
      bndupdAt: item.bndupdAt || latestTx?.bndupdAt,
      bndackAt: item.bndackAt || latestTx?.bndackAt,
      ackAt: item.ackAt || latestTx?.ackAt || item.bndackAt || latestTx?.bndackAt,
      errorCode: item.errorCode || latestTx?.errorCode,
      sourceNode: item.sourceNode || latestTx?.sourceNode,
      targetNode: item.targetNode || latestTx?.targetNode,
      reconcileLastRun: item.reconcileLastRun,
      reconcileLastErr: item.reconcileLastErr,
      reconcileSuccessTotal: item.reconcileSuccessTotal,
      reconcileFailureTotal: item.reconcileFailureTotal,
      reconcileFailureStreak: item.reconcileFailureStreak,
      reconcileIntervalSec: item.reconcileIntervalSec,
      reconcileAlertThreshold: item.reconcileAlertThreshold,
      mysqlLagMs: item.mysqlLagMs,
      mysqlLagLevel: normalizeMetricLevel(item.mysqlLagLevel),
      mysqlLagWarnMs: item.mysqlLagWarnMs,
      mysqlLagCriticalMs: item.mysqlLagCriticalMs,
      redisOffsetLag: item.redisOffsetLag,
      redisOffsetLagLevel: normalizeMetricLevel(item.redisOffsetLagLevel),
      redisOffsetLagWarn: item.redisOffsetLagWarn,
      redisOffsetLagCritical: item.redisOffsetLagCritical,
      rfc6853AckLatencyMs: item.rfc6853AckLatencyMs,
      rfc6853AckLatencyLevel: normalizeMetricLevel(item.rfc6853AckLatencyLevel),
      rfc6853AckWarnMs: item.rfc6853AckWarnMs,
      rfc6853AckCriticalMs: item.rfc6853AckCriticalMs,
      detail: buildSyncDetail(item, latestTx)
    }));

    auditEvents.value = unwrapSettled<ClusterFailoverEvent[]>(eventsResp, [], 'Audit');
    failoverEvents.value = unwrapSettled<ClusterFailoverEvent[]>(failoverResp, [], 'FailoverHistory');
    failoverPlans.value = unwrapSettled<{ items: ClusterFailoverPlan[]; count: number }>(failoverPlansResp, { items: [], count: 0 }, 'FailoverPlans').items || [];
    Object.assign(backupPlan, unwrapSettled<Partial<ClusterBackupPlan>>(planResp, {}, 'BackupPlan'));
    backupRowsRaw.value = unwrapSettled<ClusterBackupRecord[]>(backupsResp, [], 'BackupHistory');
    joinJobs.value = unwrapSettled<{ items: ClusterJoinJob[]; count: number }>(joinJobsResp, { items: [], count: 0 }, 'JoinJobs').items || [];

    if (failedBlocks.length > 0) {
      showWarning(t('cluster.syncAudit.partialLoadFail', { blocks: failedBlocks.join(', ') }));
    }
  } catch (err) {
    showHttpError(err, t('cluster.syncAudit.loadFail'));
  } finally {
    loading.value = false;
  }
};

const runFailoverPlan = async () => {
  await ElMessageBox.confirm(t('cluster.syncAudit.failoverConfirm'), t('cluster.syncAudit.failoverConfirmTitle'), {
    type: 'warning',
    confirmButtonText: t('cluster.syncAudit.failoverConfirmBtn'),
    cancelButtonText: t('cluster.syncAudit.cancel')
  });
  failoverPlanning.value = true;
  try {
    const resp = await createClusterFailoverPlan({ reason: 'manual orchestrated failover' });
    const plan = unwrap<ClusterFailoverPlan>(resp);
    showSuccess(t('cluster.syncAudit.failoverPlanCreated', { id: plan?.planId || '-' }));
    await loadData();
  } catch (err) {
    showHttpError(err, t('cluster.syncAudit.failoverPlanFail'));
  } finally {
    failoverPlanning.value = false;
  }
};

const openPlanProgress = async (row: { planId?: string }) => {
  if (!row.planId) return;
  const cached = failoverPlans.value.find((p) => p.planId === row.planId) || null;
  selectedPlan.value = cached;
  planDrawerVisible.value = true;
  try {
    const resp = await getClusterFailoverPlan(row.planId);
    selectedPlan.value = unwrap<ClusterFailoverPlan>(resp) || cached;
  } catch {
    // Keep cached content if detail fetch fails.
  }
};

const openJoinJobDetail = async (row: ClusterJoinJob) => {
  selectedJoinJob.value = row;
  joinJobDrawerVisible.value = true;
  try {
    const resp = await getClusterJoinJob(row.jobId);
    selectedJoinJob.value = unwrap<ClusterJoinJob>(resp) || row;
  } catch {
    // Keep table row snapshot when detail fetch fails.
  }
};

const cancelJoinJob = async (row: ClusterJoinJob) => {
  await ElMessageBox.confirm(t('cluster.syncAudit.cancelJobConfirm', { id: row.jobId }), t('cluster.syncAudit.cancelJobTitle'), {
    type: 'warning',
    confirmButtonText: t('cluster.syncAudit.cancelJobBtn'),
    cancelButtonText: t('cluster.syncAudit.cancel')
  });
  joinJobBusyId.value = row.jobId;
  try {
    await cancelClusterJoinJob(row.jobId);
    showSuccess(t('cluster.syncAudit.cancelJobSuccess', { id: row.jobId }));
    await loadData();
  } catch (err) {
    showHttpError(err, t('cluster.syncAudit.cancelJobFail'));
  } finally {
    joinJobBusyId.value = '';
  }
};

const retryJoinJob = async (row: ClusterJoinJob) => {
  await ElMessageBox.confirm(t('cluster.syncAudit.retryJobConfirm', { id: row.jobId }), t('cluster.syncAudit.retryJobTitle'), {
    type: 'warning',
    confirmButtonText: t('cluster.syncAudit.retryJobBtn'),
    cancelButtonText: t('cluster.syncAudit.cancel')
  });
  joinJobBusyId.value = row.jobId;
  try {
    await retryClusterJoinJob(row.jobId);
    showSuccess(t('cluster.syncAudit.retryJobSuccess', { id: row.jobId }));
    await loadData();
  } catch (err) {
    showHttpError(err, t('cluster.syncAudit.retryJobFail'));
  } finally {
    joinJobBusyId.value = '';
  }
};

const triggerFullSync = async () => {
  syncing.value = true;
  try {
    await triggerClusterSync();
    showSuccess(t('cluster.syncAudit.fullSyncSuccess'));
    await loadData();
  } catch (err) {
    showHttpError(err, t('cluster.syncAudit.fullSyncFail'));
  } finally {
    syncing.value = false;
  }
};

const runConsistencyCheck = async () => {
  checking.value = true;
  try {
    const [statusResp, statsResp] = await Promise.all([
      getClusterSyncStatus(),
      getClusterSyncStats(statsHours.value)
    ]);
    const statuses = unwrap<ClusterSyncStatus[]>(statusResp) || [];
    const stats = unwrap<ClusterSyncStats>(statsResp) || syncStats.value;
    const hasFailed = statuses.some((item) => mapSyncStatus(item) === t('cluster.syncAudit.syncStatusFailed')) || Number(stats.failed || 0) > 0;
    if (hasFailed) {
      showWarning(t('cluster.syncAudit.consistencyHasFailed'));
      return;
    }
    showSuccess(t('cluster.syncAudit.consistencyAllGood'));
  } finally {
    checking.value = false;
  }
};

const viewSyncDetail = (row: SyncRow) => {
  const openWithLocal = () => {
    const tx = row.txId ? syncTransactions.value.find((item) => item.txId === row.txId) : undefined;
    detailRow.value = {
      ...row,
      transaction: tx || null
    } as unknown as SyncRow;
    drawerVisible.value = true;
  };

  if (!row.txId) {
    openWithLocal();
    return;
  }

  getClusterSyncTransaction(row.txId)
    .then((resp) => {
      const tx = unwrap<ClusterSyncTransaction>(resp);
      detailRow.value = {
        ...row,
        transaction: tx || null
      } as unknown as SyncRow;
      drawerVisible.value = true;
    })
    .catch(() => {
      openWithLocal();
    });
};

const retrySync = async (row: SyncRow) => {
  await ElMessageBox.confirm(t('cluster.syncAudit.retrySyncConfirm', { id: row.id }), t('cluster.syncAudit.retrySyncTitle'), {
    type: 'warning',
    confirmButtonText: t('cluster.syncAudit.retrySyncBtn'),
    cancelButtonText: t('cluster.syncAudit.cancel')
  });
  retryingId.value = row.id;
  try {
    if (row.txId) {
      await retryClusterSyncTransaction(row.txId, { strategy: retryStrategy.value });
    } else {
      await triggerClusterSync();
    }
    showSuccess(t('cluster.syncAudit.retrySyncSuccess'));
    await loadData();
  } catch (err) {
    showHttpError(err, t('cluster.syncAudit.retrySyncFail'));
  } finally {
    retryingId.value = '';
  }
};

const runBackup = async () => {
  const valid = await backupFormRef.value?.validate().then(() => true).catch(() => false);
  if (!valid) return;

  await ElMessageBox.confirm(t('cluster.syncAudit.backupConfirm'), t('cluster.syncAudit.backupConfirmTitle'), {
    type: 'warning',
    confirmButtonText: t('cluster.syncAudit.backupConfirmBtn'),
    cancelButtonText: t('cluster.syncAudit.cancel')
  });

  backuping.value = true;
  const timer = window.setInterval(() => {
    progress.value = Math.min(95, progress.value + 10);
  }, 150);

  try {
    await saveClusterBackupPlan(backupPlan);
    const resp = await runClusterBackup();
    const record = unwrap<ClusterBackupRecord>(resp);
    progress.value = 100;
    const suffix = record?.txId ? t('cluster.syncAudit.backupSuccessJob', { txId: record.txId }) : '';
    showSuccess(t('cluster.syncAudit.backupSuccess', { suffix }));
    await loadData();
  } catch (err) {
    showHttpError(err, t('cluster.syncAudit.backupFail'));
  } finally {
    window.clearInterval(timer);
    window.setTimeout(() => {
      progress.value = 0;
    }, 500);
    backuping.value = false;
  }
};

const restoreBackup = async (row: { time: string; rawTime?: string; txId?: string }) => {
  await ElMessageBox.confirm(t('cluster.syncAudit.restoreConfirm', { time: row.time }), t('cluster.syncAudit.restoreConfirmTitle'), {
    type: 'warning',
    confirmButtonText: t('cluster.syncAudit.restoreConfirmBtn'),
    cancelButtonText: t('cluster.syncAudit.cancel')
  });
  try {
    const restorePoint = row.txId || row.rawTime || row.time;
    const resp = await runClusterRestore({ time: restorePoint });
    const record = unwrap<ClusterBackupRecord>(resp);
    if (record?.errorCode) {
      showWarning(t('cluster.syncAudit.restoreErrorCode', { code: record.errorCode }));
      return;
    }
    const suffix = record?.txId ? t('cluster.syncAudit.restoreSuccessJob', { txId: record.txId }) : '';
    showSuccess(t('cluster.syncAudit.restoreSuccess', { suffix }));
  } catch (err) {
    showHttpError(err, t('cluster.syncAudit.restoreFail'));
  }
};

const deleteBackup = async (row: { id: string }) => {
  showWarning(t('cluster.syncAudit.deleteNotSupported', { id: row.id }));
};

const downloadBackup = (row: { id: string; time: string; content: string; size: string; status: string }) => {
  const csv = [
    t('cluster.syncAudit.csvBackupHeader'),
    `${row.id},${row.time},${row.content},${row.size},${row.status}`
  ].join('\n');
  const blob = new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `${row.id}.csv`;
  a.click();
  URL.revokeObjectURL(url);
};

const exportFailover = () => {
  const csv = [t('cluster.syncAudit.csvFailoverHeader'), ...filteredFailoverRows.value.map((item) => `${item.time},${item.reason},${item.oldActive},${item.newActive},${item.result},${item.failedStep || '-'},${item.rollback || '-'},${item.planId || '-'}`)].join('\n');
  const blob = new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `failover_history_${Date.now()}.csv`;
  a.click();
  URL.revokeObjectURL(url);
};

const exportAudit = () => {
  const csv = [t('cluster.syncAudit.csvAuditHeader'), ...filteredAuditRows.value.map((item) => `${item.operator},${item.time},${item.action},${item.ip}`)].join('\n');
  const blob = new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `audit_log_${Date.now()}.csv`;
  a.click();
  URL.revokeObjectURL(url);
};

const exportLatencyComparison = () => {
  const header = t('cluster.syncAudit.csvLatencyHeader');
  const rows = trendPoints.value.map((p) => `${p.label},${phaseLabel(p.phase)},${p.source},${p.target},${p.ackMs},${p.ackLevel},${p.mysqlMs},${p.mysqlLevel},${p.redisLag},${p.redisLevel}`);
  const summary = [
    '',
    t('cluster.syncAudit.csvCompareHeader'),
    `${t('cluster.syncAudit.csvAckLatency')},${comparisonSummary.value.ack.before},${comparisonSummary.value.ack.after},${signedDelta(comparisonSummary.value.ack.delta)},${comparisonSummary.value.ack.level}`,
    `${t('cluster.syncAudit.csvMysqlLatency')},${comparisonSummary.value.mysql.before},${comparisonSummary.value.mysql.after},${signedDelta(comparisonSummary.value.mysql.delta)},${comparisonSummary.value.mysql.level}`,
    `${t('cluster.syncAudit.csvRedisOffset')},${comparisonSummary.value.redis.before},${comparisonSummary.value.redis.after},${signedDelta(comparisonSummary.value.redis.delta)},${comparisonSummary.value.redis.level}`
  ];
  const csv = [header, ...rows, ...summary].join('\n');
  const blob = new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `sync_latency_compare_${Date.now()}.csv`;
  a.click();
  URL.revokeObjectURL(url);
};

const formatTime = (value?: string) => {
  if (!value) return '-';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString();
};

const parseDetailValue = (detail?: string, keys: string[] = []) => {
  const source = String(detail || '');
  if (!source.trim()) return '';
  for (const key of keys) {
    const matcher = new RegExp(`${key}\\s*[=:]\\s*([^;,\\n]+)`, 'i');
    const hit = source.match(matcher);
    if (hit?.[1]) {
      const value = hit[1].trim();
      if (value) return value;
    }
  }
  return '';
};

const mapSyncStatus = (item: ClusterSyncStatus, tx?: ClusterSyncTransaction): SyncRow['status'] => {
  if (String(item.errorCode || tx?.errorCode || '').trim()) return t('cluster.syncAudit.syncStatusFailed') as SyncRow['status'];
  const state = String(item.health || '').toLowerCase();
  if (state.includes('critical') || state.includes('failed') || state.includes('异常')) return t('cluster.syncAudit.syncStatusFailed') as SyncRow['status'];
  const phase = String(item.phase || '').toLowerCase();
  if (phase.includes('bndupd')) return t('cluster.syncAudit.syncStatusBndupdSent') as SyncRow['status'];
  if (phase.includes('bndack')) return t('cluster.syncAudit.syncStatusBndackRecv') as SyncRow['status'];
  if (phase.includes('commit')) return t('cluster.syncAudit.syncStatusComplete') as SyncRow['status'];
  if (phase.includes('error')) return t('cluster.syncAudit.syncStatusFailed') as SyncRow['status'];
  return t('cluster.syncAudit.syncStatusComplete') as SyncRow['status'];
};

const buildSyncDetail = (item: ClusterSyncStatus, tx?: ClusterSyncTransaction) => {
  const phase = item.phase || tx?.phase || '-';
  const txId = item.lastTxId || tx?.txId || '-';
  const bndupdAt = item.bndupdAt || tx?.bndupdAt || '-';
  const bndackAt = item.bndackAt || tx?.bndackAt || '-';
  const ackAt = item.ackAt || tx?.ackAt || bndackAt;
  const errorCode = item.errorCode || tx?.errorCode || '-';
  const sourceNode = item.sourceNode || tx?.sourceNode || '-';
  const targetNode = item.targetNode || tx?.targetNode || '-';
  const transport = item.transport || '-';
  const peer = item.peerAddress || '-';
  const responderUp = item.responderUp === undefined ? '-' : item.responderUp ? 'up' : 'down';
  const responderErr = item.responderErr || '-';
  const reconcileLastRun = item.reconcileLastRun || '-';
  const reconcileLastErr = item.reconcileLastErr || '-';
  const reconcileSuccessTotal = item.reconcileSuccessTotal ?? 0;
  const reconcileFailureTotal = item.reconcileFailureTotal ?? 0;
  const reconcileFailureStreak = item.reconcileFailureStreak ?? 0;
  const reconcileIntervalSec = item.reconcileIntervalSec ?? 0;
  const reconcileAlertThreshold = item.reconcileAlertThreshold ?? 0;
  return `txId=${txId}; phase=${phase}; ackAt=${ackAt}; errorCode=${errorCode}; transport=${transport}; peer=${peer}; responder=${responderUp}; responderErr=${responderErr}; reconcileLastRun=${reconcileLastRun}; reconcileLastErr=${reconcileLastErr}; reconcileSuccessTotal=${reconcileSuccessTotal}; reconcileFailureTotal=${reconcileFailureTotal}; reconcileFailureStreak=${reconcileFailureStreak}; reconcileIntervalSec=${reconcileIntervalSec}; reconcileAlertThreshold=${reconcileAlertThreshold}; source=${sourceNode}; target=${targetNode}; bndupdAt=${bndupdAt}; bndackAt=${bndackAt}`;
};

const stateTag = (status: SyncRow['status']) => {
  if (status === t('cluster.syncAudit.syncStatusFailed')) return 'danger';
  if (status === t('cluster.syncAudit.syncStatusComplete')) return 'success';
  return 'warning';
};
const tagTypeClass = (type?: string) => {
  if (type === 'danger') return 'status-danger';
  if (type === 'warning') return 'status-warning';
  if (type === 'success') return 'status-success';
  return 'status-muted';
};
const levelToStatusClass = (level?: string) => {
  if (level === 'danger') return 'status-danger';
  if (level === 'warning') return 'status-warning';
  return 'status-success';
};

const phaseLabel = (phase?: string) => {
  const p = String(phase || '').toLowerCase();
  if (p.includes('bndupd')) return 'BNDUPD';
  if (p.includes('bndack')) return 'BNDACK';
  if (p.includes('commit')) return 'COMMIT';
  if (p.includes('fail')) return 'FAILED';
  return '-';
};

const joinJobStatusLabel = (status?: string) => {
  const value = String(status || '').toUpperCase();
  if (value === 'REGISTERED') return t('cluster.syncAudit.joinStatusRegistered');
  if (value === 'SNAPSHOTTING') return t('cluster.syncAudit.joinStatusSnapshotting');
  if (value === 'CATCHING_UP') return t('cluster.syncAudit.joinStatusCatchingUp');
  if (value === 'VERIFYING') return t('cluster.syncAudit.joinStatusVerifying');
  if (value === 'WARM_STANDBY') return t('cluster.syncAudit.joinStatusWarmStandby');
  if (value === 'ACTIVE') return t('cluster.syncAudit.joinStatusActive');
  if (value === 'FAILED') return t('cluster.syncAudit.joinStatusFailed');
  if (value === 'CANCELED') return t('cluster.syncAudit.joinStatusCanceled');
  return status || '-';
};

const joinJobStatusTag = (status?: string) => {
  const value = String(status || '').toUpperCase();
  if (value === 'FAILED') return 'danger';
  if (value === 'WARM_STANDBY' || value === 'ACTIVE') return 'success';
  if (value === 'CANCELED') return 'info';
  return 'warning';
};

const joinPhaseTag = (status?: string) => {
  const value = String(status || '').toUpperCase();
  if (value === 'FAILED') return 'danger';
  if (value === 'SUCCESS') return 'success';
  if (value === 'CANCELED') return 'info';
  return 'warning';
};

const joinJobCanCancel = (status?: string) => ['REGISTERED', 'SNAPSHOTTING', 'CATCHING_UP', 'VERIFYING'].includes(String(status || '').toUpperCase());

const joinJobCanRetry = (status?: string) => ['FAILED', 'CANCELED'].includes(String(status || '').toUpperCase());

const joinJobPhaseSummary = (job: ClusterJoinJob) => {
  const latest = [...(job.phases || [])].reverse().find((phase) => phase);
  if (!latest) {
    return job.error || t('cluster.syncAudit.waitingOrchestrator');
  }
  const latestStatus = latest.status ? ` · ${latest.status}` : '';
  const latestError = latest.error ? ` · ${latest.error}` : '';
  const checkpoint = job.checkpoint?.phase ? t('cluster.syncAudit.checkpointPrefix', { phase: joinJobStatusLabel(job.checkpoint.phase) }) : '';
  const target = job.catchUp?.targetOffset ? t('cluster.syncAudit.targetPrefix', { offset: job.catchUp.targetOffset }) : '';
  return `${joinJobStatusLabel(latest.phase)}${latestStatus}${checkpoint}${target}${latestError}`;
};

const joinJobSyncSummary = (job: ClusterJoinJob) => {
  const verification = job.verification;
  if (!verification?.lastSyncTxId) {
    return '-';
  }
  return `${verification.lastSyncType || 'sync'} / ${verification.lastSyncTxId}`;
};

const joinJobSnapshotSyncSummary = (job: ClusterJoinJob) => {
  if (!job.snapshot?.lastSyncTxId) {
    return '-';
  }
  return `${job.snapshot.lastSyncType || 'sync'} / ${job.snapshot.lastSyncTxId}`;
};

const joinJobSnapshotChecksumSummary = (job: ClusterJoinJob) => {
  if (!job.snapshot) {
    return '-';
  }
  const pool = job.snapshot.poolChecksum ? `Pool ${job.snapshot.poolChecksum}` : 'Pool -';
  const binding = job.snapshot.bindingChecksum ? `Binding ${job.snapshot.bindingChecksum}` : 'Binding -';
  const lease = job.snapshot.leaseChecksum ? `Lease ${job.snapshot.leaseChecksum}` : 'Lease -';
  const all = job.snapshot.consistencyChecksum ? ` · Summary ${job.snapshot.consistencyChecksum}` : '';
  return `${pool} · ${binding} · ${lease}${all}`;
};

const joinJobCheckpointHeadline = (job: ClusterJoinJob) => {
  if (!job.checkpoint) {
    return '-';
  }
  const resumable = job.checkpoint.resumable ? t('cluster.syncAudit.checkpointResumableYes') : t('cluster.syncAudit.checkpointResumableNo');
  return `${joinJobStatusLabel(job.checkpoint.phase)} / ${job.checkpoint.status || '-'} · ${resumable}`;
};

const joinJobCatchUpHeadline = (job: ClusterJoinJob) => {
  if (!job.catchUp) {
    return '-';
  }
  return `${job.catchUp.status || '-'} · ${job.catchUp.highWatermarkReached ? t('cluster.syncAudit.catchUpReached') : t('cluster.syncAudit.catchUpNotReached')}`;
};

const normalizeMetricLevel = (level?: string): MetricLevel => {
  const v = String(level || '').toLowerCase();
  if (v.includes('critical') || v.includes('error') || v.includes('danger')) return 'critical';
  if (v.includes('warning') || v.includes('warn')) return 'warning';
  return 'healthy';
};

const parseLatencyMs = (duration?: string) => Math.max(0, Number(String(duration || '').replace(/[^\d]/g, '')) || 0);

const levelTag = (level: MetricLevel) => {
  if (level === 'critical') return 'danger';
  if (level === 'warning') return 'warning';
  return 'success';
};

const levelClass = (level: MetricLevel) => {
  if (level === 'critical') return 'level-critical';
  if (level === 'warning') return 'level-warning';
  return 'level-healthy';
};

const scalePercent = (value: number, max: number) => {
  if (max <= 0) return 0;
  return Math.max(4, Math.min(100, Math.round((value / max) * 100)));
};

const avgOf = (values: number[]) => {
  if (!values.length) return 0;
  return Math.round(values.reduce((sum, item) => sum + item, 0) / values.length);
};

const signedDelta = (value: number) => (value > 0 ? `+${value}` : `${value}`);

const worstLevel = (levels: MetricLevel[]): MetricLevel => {
  if (levels.some((l) => l === 'critical')) return 'critical';
  if (levels.some((l) => l === 'warning')) return 'warning';
  return 'healthy';
};

const buildCompareMetric = (before: number, after: number, levels: MetricLevel[]) => {
  const delta = after - before;
  const level = worstLevel(levels);
  return {
    before,
    after,
    delta,
    level,
    levelText: level === 'critical' ? t('cluster.syncAudit.levelRed') : level === 'warning' ? t('cluster.syncAudit.levelYellow') : t('cluster.syncAudit.levelGreen')
  };
};

const phaseTag = (phase?: string) => {
  const p = String(phase || '').toLowerCase();
  if (p.includes('commit')) return 'success';
  if (p.includes('fail')) return 'danger';
  if (p.includes('bnd')) return 'warning';
  return 'info';
};

const planResultTag = (result: string) => {
  if (result === t('cluster.syncAudit.resultSuccess')) return 'success';
  if (result === t('cluster.syncAudit.resultRunning')) return 'warning';
  return 'danger';
};

const planStatusTag = (status: string) => {
  const v = String(status || '').toUpperCase();
  if (v === 'SUCCESS' || v === 'ROLLED_BACK') return 'success';
  if (v === 'RUNNING' || v === 'PENDING') return 'warning';
  return 'danger';
};

const planStepTag = (status: string) => {
  const v = String(status || '').toUpperCase();
  if (v === 'SUCCESS') return 'success';
  if (v === 'RUNNING' || v === 'PENDING') return 'warning';
  if (v === 'SKIPPED') return 'info';
  return 'danger';
};

watch(timeRange, () => {
  loadData();
});

onMounted(loadData);
</script>

<style scoped>
.page-wrap {
  --brand-primary: #1677ff;
  --state-success: #00c853;
  --state-warning: #ffb300;
  --state-danger: #f44336;
  --state-muted: #909399;
  --page-bg: #f5f7fb;
  --surface-bg: #ffffff;
  --surface-border: #d9e1ec;
  --surface-shadow: 0 10px 24px rgba(15, 23, 42, 0.06);
  --text-primary: #1f2d3d;
  --text-secondary: #687586;
  padding: 24px;
  background: var(--page-bg);
  color: var(--text-primary);
  display: grid;
  gap: 24px;
}

.surface-card {
  border: 1px solid var(--surface-border);
  border-radius: 14px;
  background: var(--surface-bg);
  box-shadow: var(--surface-shadow);
}

.page-header {
  padding: 20px;
}

.page-header h3 {
  margin: 0;
}

.desc {
  margin: 4px 0 0;
  color: var(--text-secondary);
}

.section-desc {
  margin: 6px 0 0;
  color: var(--text-secondary);
  font-size: 13px;
}

.table-card {
  padding: 20px;
}

.status-chip {
  width: fit-content;
  padding-inline: 10px;
  border: none;
  border-radius: 999px;
  font-weight: 600;
}

:deep(.status-chip.el-tag) {
  color: #fff;
}

:deep(.status-success.el-tag) {
  background: var(--state-success);
}

:deep(.status-warning.el-tag) {
  background: var(--state-warning);
}

:deep(.status-danger.el-tag) {
  background: var(--state-danger);
}

:deep(.status-muted.el-tag) {
  background: var(--state-muted);
}

.card-title {
  font-weight: 600;
}

.join-job-header {
  margin-bottom: 16px;
}

.join-job-summary {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.row-between {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.stat-card {
  cursor: pointer;
  padding: 16px;
  min-height: 148px;
}

.stat-card.is-active {
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--brand-primary) 45%, transparent), var(--surface-shadow);
}

.stat-card.is-danger {
  border-color: var(--state-danger);
}

.stat-card.is-warning {
  border-color: var(--state-warning);
}

.stat-label {
  margin: 0;
  font-size: 13px;
  color: var(--text-secondary);
}

.stat-value {
  display: block;
  margin-top: 8px;
  font-size: 20px;
  line-height: 1.3;
}

.stat-chip {
  margin-top: 10px;
}

.stat-sub {
  display: block;
  margin-top: 8px;
  font-size: 12px;
  color: var(--text-secondary);
}

.filter-bar {
  margin-bottom: 16px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.filter-group {
  padding: 12px;
  border: 1px solid var(--surface-border);
  border-radius: 12px;
  background: color-mix(in srgb, var(--surface-bg) 92%, #94a3b8 8%);
}

.filter-group-actions {
  justify-content: flex-end;
}

.filter-hint {
  margin: 0 0 12px;
  font-size: 12px;
  color: var(--text-secondary);
}

.bar-left,
.bar-right,
.mini-filters {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.grid-2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.trend-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  margin-top: 12px;
}

.trend-card {
  border: 1px solid var(--surface-border);
  border-radius: 14px;
  padding: 14px;
  min-height: 260px;
}

.trend-title {
  font-weight: 600;
  margin-bottom: 8px;
}

.trend-row {
  display: grid;
  grid-template-columns: 92px 1fr 70px;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}

.trend-time {
  font-size: 12px;
  color: var(--text-secondary);
}

.trend-bar-wrap {
  height: 8px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--surface-bg) 84%, #94a3b8 16%);
  overflow: hidden;
}

.trend-bar {
  height: 8px;
  border-radius: 999px;
}

.trend-value {
  text-align: right;
  font-size: 12px;
}

.compare-grid {
  margin-top: 12px;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.compare-item {
  border: 1px solid var(--surface-border);
  border-radius: 14px;
  padding: 14px;
}

.compare-item p {
  margin: 4px 0;
  color: var(--text-secondary);
  font-size: 13px;
}

.level-healthy {
  border-color: var(--state-success);
  background: color-mix(in srgb, var(--state-success) 8%, var(--surface-bg));
}

.level-warning {
  border-color: var(--state-warning);
  background: color-mix(in srgb, var(--state-warning) 8%, var(--surface-bg));
}

.level-critical {
  border-color: var(--state-danger);
  background: color-mix(in srgb, var(--state-danger) 8%, var(--surface-bg));
}

.half-card {
  min-height: 390px;
}

.backup-form {
  margin-top: 16px;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 20px;
}

.backup-span-1 {
  grid-column: span 1;
}

.backup-span-2 {
  grid-column: 1 / -1;
}

.backup-progress {
  margin-bottom: 12px;
}

.detail-pre {
  margin: 0;
  padding: 12px;
  border: 1px solid var(--surface-border);
  border-radius: 12px;
  background: color-mix(in srgb, var(--surface-bg) 88%, #94a3b8 12%);
  white-space: pre-wrap;
}

.plan-meta {
  margin-bottom: 12px;
}

.plan-progress {
  margin-bottom: 12px;
}

.plan-step-table {
  width: 100%;
}

.table-pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 14px;
}

:deep(.el-form-item) {
  margin-bottom: 18px;
}

:deep(.el-form-item__label) {
  text-align: right;
  padding-right: 12px;
}

:deep(.el-button) {
  height: 36px;
  transition: transform 0.18s ease, box-shadow 0.18s ease, background-color 0.18s ease, border-color 0.18s ease;
}

:deep(.el-button:hover:not(.is-disabled)) {
  transform: translateY(-1px);
}

:deep(.el-button.is-disabled) {
  opacity: 0.58;
}

@media (max-width: 1400px) {
  .stats-grid,
  .grid-2 {
    grid-template-columns: 1fr;
  }

  .trend-grid,
  .compare-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 1200px) {
  .page-wrap {
    padding: 16px;
  }

  .backup-form {
    grid-template-columns: 1fr;
  }

  .backup-span-1,
  .backup-span-2 {
    grid-column: auto;
  }

  .filter-bar,
  .row-between {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
