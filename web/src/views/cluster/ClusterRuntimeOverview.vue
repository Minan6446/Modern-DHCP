<template>
  <div class="page-wrap" v-loading="loading">
    <section class="page-header surface-card">
      <h3>{{ t('cluster.runtime.title') }}</h3>
      <p class="desc">{{ t('cluster.runtime.desc') }}</p>
    </section>

    <section class="control-brief-grid">
      <el-card shadow="never" class="surface-card control-brief-card">
        <div class="control-brief-head">
          <span class="control-brief-title">{{ t('cluster.runtime.controlInit') }}</span>
          <el-tag class="status-chip" :class="clusterControl?.initialized ? 'status-success' : 'status-muted'">
            {{ clusterControl?.initialized ? t('cluster.runtime.controlInitYes') : t('cluster.runtime.controlInitNo') }}
          </el-tag>
        </div>
        <strong class="control-brief-value">{{ clusterControl?.clusterDomain || t('cluster.runtime.controlNoDomain') }}</strong>
        <p class="control-brief-sub">Primary: {{ clusterControl?.primaryNodeId || '-' }}</p>
      </el-card>

      <el-card shadow="never" class="surface-card control-brief-card">
        <div class="control-brief-head">
          <span class="control-brief-title">{{ t('cluster.runtime.apiTlsTitle') }}</span>
          <el-tag class="status-chip" :class="clusterControlTlsClass">{{ clusterControlTlsLabel }}</el-tag>
        </div>
        <strong class="control-brief-value">{{ clusterControl?.apiTlsEnabled ? t('cluster.runtime.apiTlsEnabled') : t('cluster.runtime.apiTlsDisabled') }}</strong>
        <p class="control-brief-sub">{{ clusterControl?.restartRequired ? t('cluster.runtime.apiTlsRestartHint') : t('cluster.runtime.apiTlsConsistent') }}</p>
      </el-card>

      <el-card shadow="never" class="surface-card control-brief-card">
        <div class="control-brief-head">
          <span class="control-brief-title">{{ t('cluster.runtime.controlNodes') }}</span>
          <el-tag class="status-chip status-primary">{{ t('cluster.runtime.controlNodesCount', { count: clusterControlNodes.length }) }}</el-tag>
        </div>
        <strong class="control-brief-value">{{ t('cluster.runtime.controlNodesConnected', { count: connectedControlNodes }) }}</strong>
        <p class="control-brief-sub">{{ t('cluster.runtime.controlNodesSub', { unreachable: unreachableControlNodes, version: clusterControl?.configVersion ?? 0 }) }}</p>
      </el-card>

      <el-card shadow="never" class="surface-card control-brief-card">
        <div class="control-brief-head">
          <span class="control-brief-title">{{ t('cluster.runtime.joinOrchTitle') }}</span>
          <el-tag class="status-chip" :class="joinJobSummary.failed > 0 ? 'status-danger' : joinJobSummary.running > 0 ? 'status-warning' : 'status-success'">
            {{ joinJobSummary.running > 0 ? t('cluster.runtime.joinOrchRunning') : joinJobSummary.failed > 0 ? t('cluster.runtime.joinOrchNeedAction') : t('cluster.runtime.joinOrchStable') }}
          </el-tag>
        </div>
        <strong class="control-brief-value">{{ t('cluster.runtime.joinOrchValue', { running: joinJobSummary.running, failed: joinJobSummary.failed }) }}</strong>
        <p class="control-brief-sub">{{ t('cluster.runtime.joinOrchSub', { completed: joinJobSummary.completed, time: latestJoinJobUpdate }) }}</p>
      </el-card>
    </section>

    <el-alert
      v-if="clusterControl?.restartRequired"
      :title="t('cluster.runtime.tlsRestartAlert')"
      type="warning"
      :closable="false"
      show-icon
    />

    <section class="stats-grid">
      <el-card
        v-for="card in statCards"
        :key="card.key"
        shadow="never"
        class="surface-card stat-card"
        :class="[
          card.level === 'danger' ? 'is-danger' : '',
          card.level === 'warning' ? 'is-warning' : ''
        ]"
      >
        <p class="stat-label">{{ card.label }}</p>
        <strong class="stat-value">{{ card.value }}</strong>
        <el-tag v-if="card.badge" size="small" class="status-chip stat-badge" :class="badgeClass(card.badge.type)">{{ card.badge.text }}</el-tag>
        <small class="stat-sub">{{ card.sub }}</small>
      </el-card>
    </section>

    <section class="grid-2-1">
      <el-card shadow="never" class="surface-card table-card">
        <template #header><div class="card-title">{{ t('cluster.runtime.topologyTitle') }}</div></template>
        <div class="topology-wrap">
          <div class="node-panel" :class="nodePanelClass(activeNode)">
            <h4>{{ t('cluster.runtime.activeNode') }}</h4>
            <p>IP: {{ activeNode?.address || '-' }}</p>
            <p>{{ t('cluster.runtime.nodeRunState') }}: <span :class="statusClass(activeNode?.health)">{{ healthLabel(activeNode?.health) }}</span></p>
            <div class="control-binding-row">
              <span class="control-binding-label">{{ t('cluster.runtime.controlBinding') }}</span>
              <el-tag size="small" :type="controlBindingTag(activeNode)">{{ controlBindingLabel(activeNode) }}</el-tag>
            </div>
            <p class="control-binding-meta">{{ controlBindingMeta(activeNode) }}</p>
            <div class="control-binding-row">
              <span class="control-binding-label">{{ t('cluster.runtime.joinOrch') }}</span>
              <el-tag size="small" :type="joinJobTag(activeNode)">{{ joinJobLabel(activeNode) }}</el-tag>
            </div>
            <p class="control-binding-meta">{{ joinJobMeta(activeNode) }}</p>
            <p class="control-binding-meta" v-if="joinJobForRuntimeNode(activeNode)">{{ joinSnapshotBrief(activeNode) }}</p>
            <p class="control-binding-meta" v-if="joinJobForRuntimeNode(activeNode)">{{ joinCatchUpBrief(activeNode) }}</p>
            <div class="service-inline-grid">
              <div v-for="svc in serviceViewsForNode(activeNode)" :key="`active-${svc.key}`" class="service-inline-item" :class="consistencyClass(svc.consistency)">
                <div class="service-inline-head">
                  <div class="service-title-wrap">
                    <span class="service-badge" :class="`service-badge-${svc.key}`">{{ serviceBadgeText(svc.key) }}</span>
                    <strong>{{ svc.label }}</strong>
                  </div>
                  <el-tag size="small" class="status-chip" :class="consistencyClassName(svc.consistency)">{{ consistencyText(svc.consistency) }}</el-tag>
                </div>
                <p>{{ t('cluster.runtime.roleLabel') }}: {{ storageRoleLabel(svc.role) }} / {{ t('cluster.runtime.expectedLabel') }}{{ storageRoleLabel(svc.expectedRole) }}</p>
                <p>{{ t('cluster.runtime.healthLabel') }}: <span :class="statusClass(svc.health)">{{ storageHealthLabel(svc.health) }}</span></p>
                <p v-if="svc.lagText">{{ t('cluster.runtime.lagLabel') }}: {{ svc.lagText }}</p>
              </div>
            </div>
            <p class="consistency-row">
              {{ t('cluster.runtime.consistencyLabel') }}:
              <el-tag size="small" class="status-chip" :class="consistencyClassName(nodeConsistency(activeNode))">{{ consistencyText(nodeConsistency(activeNode)) }}</el-tag>
            </p>
            <div class="node-meter-grid">
              <div class="node-meter-item">
                <div class="meter-head"><span>CPU</span><strong>{{ pct(activeNode?.cpuPercent) }}</strong></div>
                <el-progress :percentage="Number(activeNode?.cpuPercent || 0)" :stroke-width="8" :show-text="false" :color="metricColor(Number(activeNode?.cpuPercent || 0))" />
              </div>
              <div class="node-meter-item">
                <div class="meter-head"><span>{{ t('cluster.runtime.memLabel') }}</span><strong>{{ pct(activeNode?.memoryPercent) }}</strong></div>
                <el-progress :percentage="Number(activeNode?.memoryPercent || 0)" :stroke-width="8" :show-text="false" :color="metricColor(Number(activeNode?.memoryPercent || 0))" />
              </div>
            </div>
            <div class="panel-actions">
              <el-button size="small" @click="goConfig">{{ t('cluster.runtime.nodeConfig') }}</el-button>
              <el-button
                size="small"
                type="primary"
                :loading="syncingId === (activeNode?.id || '')"
                :disabled="!canNodeSync(activeNode)"
                @click="manualSync(activeNode)"
              >
                {{ t('cluster.runtime.manualSync') }}
              </el-button>
            </div>
          </div>

          <div class="link-panel">
            <el-tag size="small" class="status-chip" :class="syncException ? 'status-danger' : 'status-success'">TCP 647</el-tag>
            <div class="sync-line" :class="syncException ? 'is-bad' : 'is-good'">BNDUPD -> BNDACK</div>
            <small :class="syncException ? 'bad-text' : 'good-text'">
              {{ syncException ? t('cluster.runtime.syncException') : t('cluster.runtime.syncNormal') }}
            </small>
          </div>

          <div class="node-panel" :class="nodePanelClass(standbyNode)">
            <h4>{{ t('cluster.runtime.standbyNode') }}</h4>
            <p>IP: {{ standbyNode?.address || '-' }}</p>
            <p>{{ t('cluster.runtime.nodeRunState') }}: <span :class="statusClass(standbyNode?.health)">{{ healthLabel(standbyNode?.health) }}</span></p>
            <div class="control-binding-row">
              <span class="control-binding-label">{{ t('cluster.runtime.controlBinding') }}</span>
              <el-tag size="small" :type="controlBindingTag(standbyNode)">{{ controlBindingLabel(standbyNode) }}</el-tag>
            </div>
            <p class="control-binding-meta">{{ controlBindingMeta(standbyNode) }}</p>
            <div class="control-binding-row">
              <span class="control-binding-label">{{ t('cluster.runtime.joinOrch') }}</span>
              <el-tag size="small" :type="joinJobTag(standbyNode)">{{ joinJobLabel(standbyNode) }}</el-tag>
            </div>
            <p class="control-binding-meta">{{ joinJobMeta(standbyNode) }}</p>
            <p class="control-binding-meta" v-if="joinJobForRuntimeNode(standbyNode)">{{ joinSnapshotBrief(standbyNode) }}</p>
            <p class="control-binding-meta" v-if="joinJobForRuntimeNode(standbyNode)">{{ joinCatchUpBrief(standbyNode) }}</p>
            <div class="service-inline-grid">
              <div v-for="svc in serviceViewsForNode(standbyNode)" :key="`standby-${svc.key}`" class="service-inline-item" :class="consistencyClass(svc.consistency)">
                <div class="service-inline-head">
                  <div class="service-title-wrap">
                    <span class="service-badge" :class="`service-badge-${svc.key}`">{{ serviceBadgeText(svc.key) }}</span>
                    <strong>{{ svc.label }}</strong>
                  </div>
                  <el-tag size="small" class="status-chip" :class="consistencyClassName(svc.consistency)">{{ consistencyText(svc.consistency) }}</el-tag>
                </div>
                <p>{{ t('cluster.runtime.roleLabel') }}: {{ storageRoleLabel(svc.role) }} / {{ t('cluster.runtime.expectedLabel') }}{{ storageRoleLabel(svc.expectedRole) }}</p>
                <p>{{ t('cluster.runtime.healthLabel') }}: <span :class="statusClass(svc.health)">{{ storageHealthLabel(svc.health) }}</span></p>
                <p v-if="svc.lagText">{{ t('cluster.runtime.lagLabel') }}: {{ svc.lagText }}</p>
              </div>
            </div>
            <p class="consistency-row">
              {{ t('cluster.runtime.consistencyLabel') }}:
              <el-tag size="small" class="status-chip" :class="consistencyClassName(nodeConsistency(standbyNode))">{{ consistencyText(nodeConsistency(standbyNode)) }}</el-tag>
            </p>
            <div class="node-meter-grid">
              <div class="node-meter-item">
                <div class="meter-head"><span>CPU</span><strong>{{ pct(standbyNode?.cpuPercent) }}</strong></div>
                <el-progress :percentage="Number(standbyNode?.cpuPercent || 0)" :stroke-width="8" :show-text="false" :color="metricColor(Number(standbyNode?.cpuPercent || 0))" />
              </div>
              <div class="node-meter-item">
                <div class="meter-head"><span>{{ t('cluster.runtime.memLabel') }}</span><strong>{{ pct(standbyNode?.memoryPercent) }}</strong></div>
                <el-progress :percentage="Number(standbyNode?.memoryPercent || 0)" :stroke-width="8" :show-text="false" :color="metricColor(Number(standbyNode?.memoryPercent || 0))" />
              </div>
            </div>
            <div class="panel-actions">
              <el-button size="small" @click="goConfig">{{ t('cluster.runtime.nodeConfig') }}</el-button>
              <el-button
                size="small"
                type="primary"
                :loading="syncingId === (standbyNode?.id || '')"
                :disabled="!canNodeSync(standbyNode)"
                @click="manualSync(standbyNode)"
              >
                {{ t('cluster.runtime.manualSync') }}
              </el-button>
            </div>
          </div>
        </div>
      </el-card>

      <el-card shadow="never" class="surface-card table-card">
        <template #header><div class="card-title">{{ t('cluster.runtime.pendingAlertTitle') }}</div></template>
        <el-empty v-if="!pendingEvents.length" :description="t('cluster.runtime.pendingAlertEmpty')" :image-size="52" />
        <div v-else class="event-list">
          <div v-for="evt in pendingEvents" :key="evt.id" class="event-item" :class="evt.type === 'danger' ? 'is-danger' : 'is-warning'">
            <div class="event-main">
              <strong>{{ evt.title }}</strong>
              <p>{{ evt.detail }}</p>
              <small>{{ formatTime(evt.time) }}</small>
            </div>
            <el-button size="small" type="primary" :loading="handlingEventId === evt.id" @click="handleEvent(evt)">{{ t('cluster.runtime.oneClickHandle') }}</el-button>
          </div>
        </div>
      </el-card>
    </section>

    <section class="surface-card table-card">
      <div class="filter-bar">
        <div class="bar-left">
          <el-select v-model="statusFilter" clearable size="small" :placeholder="t('cluster.runtime.filterStatusPlaceholder')" style="width: 120px">
            <el-option :label="t('cluster.runtime.filterStatusHealthy')" value="healthy" />
            <el-option :label="t('cluster.runtime.filterStatusWarning')" value="warning" />
            <el-option :label="t('cluster.runtime.filterStatusCritical')" value="critical" />
            <el-option :label="t('cluster.runtime.filterStatusUnknown')" value="unknown" />
          </el-select>
          <el-select v-model="roleFilter" clearable size="small" :placeholder="t('cluster.runtime.filterRolePlaceholder')" style="width: 120px">
            <el-option :label="t('cluster.runtime.filterRoleActive')" value="active" />
            <el-option :label="t('cluster.runtime.filterRoleStandby')" value="standby" />
            <el-option :label="t('cluster.runtime.filterRoleWorker')" value="worker" />
          </el-select>
          <el-input v-model="keyword" clearable size="small" :placeholder="t('cluster.runtime.filterKeywordPlaceholder')" style="width: 180px" />
        </div>
        <div class="bar-right">
          <el-button size="small" :loading="loading" @click="loadData">{{ t('cluster.runtime.refresh') }}</el-button>
          <el-button size="small" type="primary" @click="exportNodes">{{ t('cluster.runtime.export') }}</el-button>
        </div>
      </div>

      <el-table :data="tableRows" border stripe>
        <el-table-column prop="address" :label="t('cluster.runtime.colNodeIp')" min-width="150" />
        <el-table-column :label="t('cluster.runtime.colRole')" width="110">
          <template #default="{ row }">{{ roleLabel(row.role) }}</template>
        </el-table-column>
        <el-table-column :label="t('cluster.runtime.colHealth')" width="110">
          <template #default="{ row }"><el-tag :type="healthTag(row.health)">{{ healthLabel(row.health) }}</el-tag></template>
        </el-table-column>
        <el-table-column :label="t('cluster.runtime.colControlState')" min-width="180">
          <template #default="{ row }">
            <div class="control-cell">
              <el-tag size="small" :type="controlBindingTag(row)">{{ controlBindingLabel(row) }}</el-tag>
              <small>{{ controlBindingShortMeta(row) }}</small>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="t('cluster.runtime.colJoinOrch')" min-width="180">
          <template #default="{ row }">
            <div class="control-cell">
              <el-tag size="small" :type="joinJobTag(row)">{{ joinJobLabel(row) }}</el-tag>
              <small>{{ joinJobShortMeta(row) }}</small>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="t('cluster.runtime.colCpu')" width="110">
          <template #default="{ row }">
            <div class="metric-cell">
              <strong :class="alertValueClass(row.cpuPercent)">{{ pct(row.cpuPercent) }}</strong>
              <el-progress :percentage="Number(row.cpuPercent || 0)" :stroke-width="6" :show-text="false" :color="metricColor(Number(row.cpuPercent || 0))" />
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="t('cluster.runtime.colMem')" width="110">
          <template #default="{ row }">
            <div class="metric-cell">
              <strong :class="alertValueClass(row.memoryPercent)">{{ pct(row.memoryPercent) }}</strong>
              <el-progress :percentage="Number(row.memoryPercent || 0)" :stroke-width="6" :show-text="false" :color="metricColor(Number(row.memoryPercent || 0))" />
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="t('cluster.runtime.colActiveLeases')" width="140">
          <template #default="{ row }"><span :class="alertValueClass(row.cpuPercent)">{{ activeLeaseCount(row) }}</span></template>
        </el-table-column>
        <el-table-column :label="t('cluster.runtime.colTotalLeases')" width="140">
          <template #default="{ row }"><span :class="alertValueClass(row.memoryPercent)">{{ totalLeaseCount(row) }}</span></template>
        </el-table-column>
        <el-table-column :label="t('cluster.runtime.colLastSync')" min-width="170">
          <template #default="{ row }">{{ formatTime(row.lastHeartbeat) }}</template>
        </el-table-column>
        <el-table-column :label="t('cluster.runtime.colAlignment')" width="110">
          <template #default>
            <el-tag :type="alignmentTag()">{{ alignmentLabel() }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('cluster.runtime.colWriteGate')" width="100">
          <template #default>
            <el-tag :type="writeGateTag()">{{ writeGateLabel() }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('cluster.runtime.colActions')" width="180" fixed="right">
          <template #default="{ row }">
            <div class="row-actions">
              <el-button size="small" @click="showDetail(row)">{{ t('cluster.runtime.viewDetail') }}</el-button>
              <el-button
                size="small"
                type="primary"
                plain
                :loading="syncingId === row.id"
                :disabled="!canNodeSync(row)"
                @click="manualSync(row)"
              >
                {{ t('cluster.runtime.manualSync') }}
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <section class="grid-2-1 grid-single-side">
      <el-card shadow="never" class="surface-card table-card">
        <template #header>
          <div class="card-title row-between">
            <span>{{ t('cluster.runtime.recentEvents') }}</span>
            <el-button link type="primary" @click="goAudit">{{ t('cluster.runtime.viewAllAudit') }}</el-button>
          </div>
        </template>

        <el-empty v-if="!sortedEvents.length" :description="t('cluster.runtime.recentEventsEmpty')" :image-size="52" />
        <el-table v-else :data="sortedEvents.slice(0, 10)" size="small" border>
          <el-table-column prop="time" :label="t('cluster.runtime.eventColTime')" width="150" />
          <el-table-column prop="title" :label="t('cluster.runtime.eventColType')" width="120" />
          <el-table-column prop="detail" :label="t('cluster.runtime.eventColDetail')" min-width="170" show-overflow-tooltip />
          <el-table-column :label="t('cluster.runtime.eventColActions')" width="90">
            <template #default="{ row }">
              <el-button size="small" type="primary" link @click="handleEvent(row)">{{ t('cluster.runtime.oneClickHandle') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </section>

    <el-dialog v-model="detailVisible" :title="t('cluster.runtime.detailTitle')" width="640px">
      <div v-if="detailNode" class="detail-service-grid">
        <div
          v-for="svc in serviceViewsForNode(detailNode)"
          :key="`detail-${svc.key}`"
          class="detail-service-item"
          :class="consistencyClass(svc.consistency)"
        >
          <div class="service-inline-head">
            <div class="service-title-wrap">
              <span class="service-badge" :class="`service-badge-${svc.key}`">{{ serviceBadgeText(svc.key) }}</span>
              <strong>{{ svc.label }}</strong>
            </div>
            <el-tag size="small" class="status-chip" :class="consistencyClassName(svc.consistency)">{{ consistencyText(svc.consistency) }}</el-tag>
          </div>
          <p>{{ t('cluster.runtime.roleLabel') }}: {{ storageRoleLabel(svc.role) }} / {{ t('cluster.runtime.expectedLabel') }}{{ storageRoleLabel(svc.expectedRole) }}</p>
          <p>{{ t('cluster.runtime.healthLabel') }}: {{ storageHealthLabel(svc.health) }}</p>
          <p v-if="svc.lagText">{{ t('cluster.runtime.lagLabel') }}: {{ svc.lagText }}</p>
        </div>
      </div>
      <el-descriptions v-if="detailNode" :column="2" border class="detail-kv">
        <el-descriptions-item :label="t('cluster.runtime.detailConsistency')">
          <el-tag :type="consistencyTag(nodeConsistency(detailNode))">{{ consistencyText(nodeConsistency(detailNode)) }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailControlBinding')">
          <el-tag :type="controlBindingTag(detailNode)">{{ controlBindingLabel(detailNode) }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailDhcpRole')">{{ storageRoleLabel(overview?.dhcpRole) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailDhcpHealth')">{{ storageHealthLabel(overview?.dhcpHealth) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailMysqlRole')">{{ storageRoleLabel(overview?.mysqlRole) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailMysqlHealth')">{{ storageHealthLabel(overview?.mysqlHealth) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailRedisRole')">{{ storageRoleLabel(overview?.redisRole) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailRedisHealth')">{{ storageHealthLabel(overview?.redisHealth) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailAlignment')">{{ alignmentLabel() }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailWriteGate')">{{ writeGateLabel() }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailControlEndpoint')">{{ controlBindingMeta(detailNode) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailJoinOrch')">{{ joinJobMeta(detailNode) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailJoinVerify')" v-if="joinJobForRuntimeNode(detailNode)">
          <el-tag :type="joinVerificationTag(detailNode)">{{ joinVerificationLabel(detailNode) }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailVerifyTime')" v-if="joinJobForRuntimeNode(detailNode)">{{ joinVerificationCheckedAt(detailNode) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailReplicaMode')" v-if="joinJobForRuntimeNode(detailNode)">{{ joinVerificationMode(detailNode) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailReplicaSource')" v-if="joinJobForRuntimeNode(detailNode)">{{ joinVerificationSource(detailNode) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailReplicaCheck')" v-if="joinJobForRuntimeNode(detailNode)">{{ joinVerificationReplication(detailNode) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailReplicaPhase')" v-if="joinJobForRuntimeNode(detailNode)">{{ joinVerificationState(detailNode) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailLastSyncTx')" v-if="joinJobForRuntimeNode(detailNode)">{{ joinVerificationSync(detailNode) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailDataSummary')" :span="2" v-if="joinJobForRuntimeNode(detailNode)">{{ joinVerificationDataset(detailNode) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailJoinSnapshot')" :span="2" v-if="joinJobForRuntimeNode(detailNode)">{{ joinSnapshotSummary(detailNode) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailCatchUp')" :span="2" v-if="joinJobForRuntimeNode(detailNode)">{{ joinCatchUpSummary(detailNode) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailCheckpoint')" :span="2" v-if="joinJobForRuntimeNode(detailNode)">{{ joinCheckpointSummary(detailNode) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailVerifyNote')" :span="2" v-if="joinJobForRuntimeNode(detailNode)">{{ joinVerificationMessage(detailNode) }}</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailMysqlLag')">{{ overview?.mysqlLagMs ?? overview?.replicationLagMs ?? 0 }} ms</el-descriptions-item>
        <el-descriptions-item :label="t('cluster.runtime.detailRedisLag')">{{ overview?.redisOffsetLag ?? 0 }}</el-descriptions-item>
        <el-descriptions-item label="Fencing Epoch" :span="2">{{ overview?.fencingEpoch || '-' }}</el-descriptions-item>
      </el-descriptions>
      <pre class="detail-pre">{{ JSON.stringify(detailNode, null, 2) }}</pre>
      <template #footer><el-button @click="detailVisible = false">{{ t('cluster.runtime.detailClose') }}</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import {
  getClusterControl,
  getClusterFailoverHistory,
  getClusterJoinJobs,
  getClusterOverview,
  getClusterSyncStatus,
  triggerClusterSync
} from '@/api/cluster';
import type {
  ClusterControl,
  ClusterControlNode,
  ClusterFailoverEvent,
  ClusterJoinJob,
  ClusterNode,
  ClusterOverview,
  ClusterSyncStatus
} from '@/types/cluster';
import { useI18n } from 'vue-i18n';
import { showHttpError } from '@/shared/errors/errorToast';
import { showSuccess } from '@/shared/errors/messageToast';

type CardKey = 'runtime' | 'sync' | 'storage' | 'pending';
type ServiceKey = 'dhcp' | 'mysql' | 'redis';
type ConsistencyState = 'good' | 'warn' | 'bad' | 'unknown';
type AlignmentState = 'aligned' | 'misaligned' | 'unknown';

const { t } = useI18n();
const router = useRouter();
const loading = ref(false);
const syncingId = ref('');
const handlingEventId = ref('');
const detailVisible = ref(false);
const detailNode = ref<ClusterNode | null>(null);

const statusFilter = ref('');
const roleFilter = ref('');
const keyword = ref('');

const overview = ref<ClusterOverview | null>(null);
const clusterControl = ref<ClusterControl | null>(null);
const syncRows = ref<ClusterSyncStatus[]>([]);
const events = ref<ClusterFailoverEvent[]>([]);
const joinJobs = ref<ClusterJoinJob[]>([]);

const unwrap = <T>(resp: any): T => (resp?.data?.data ?? resp?.data ?? resp) as T;

const nodes = computed(() => overview.value?.nodes || []);
const clusterControlNodes = computed<ClusterControlNode[]>(() => clusterControl.value?.nodes || []);
const connectedControlNodes = computed(
  () => clusterControlNodes.value.filter((item) => item.state === 'connected' || item.state === 'self').length
);
const unreachableControlNodes = computed(
  () => clusterControlNodes.value.filter((item) => item.state === 'unreachable').length
);
const clusterControlTlsLabel = computed(() => {
  if (!clusterControl.value?.apiTlsEnabled) return t('cluster.runtime.apiTlsLabelOff');
  if (clusterControl.value.apiTlsActive) return t('cluster.runtime.apiTlsLabelActive');
  return t('cluster.runtime.apiTlsLabelPending');
});
const clusterControlTlsTag = computed(() => {
  if (!clusterControl.value?.apiTlsEnabled) return 'info';
  if (clusterControl.value.apiTlsActive) return 'success';
  return 'warning';
});
const clusterControlTlsClass = computed(() => {
  if (!clusterControl.value?.apiTlsEnabled) return 'status-muted';
  if (clusterControl.value.apiTlsActive) return 'status-success';
  return 'status-warning';
});
const joinJobSummary = computed(() => ({
  running: joinJobs.value.filter((job) => isJoinJobRunning(job.status)).length,
  failed: joinJobs.value.filter((job) => String(job.status || '').toUpperCase() === 'FAILED').length,
  completed: joinJobs.value.filter((job) => ['WARM_STANDBY', 'ACTIVE'].includes(String(job.status || '').toUpperCase())).length
}));
const latestJoinJobUpdate = computed(() => {
  if (!joinJobs.value.length) return t('cluster.runtime.joinOrchNoTask');
  const latest = [...joinJobs.value].sort((a, b) => new Date(b.updatedAt || 0).getTime() - new Date(a.updatedAt || 0).getTime())[0];
  return formatTime(latest?.updatedAt);
});
const normalizeIdentity = (value: unknown) => String(value || '').trim().toLowerCase();
const hostFromUrl = (value?: string) => {
  const raw = String(value || '').trim();
  if (!raw) return '';
  try {
    return new URL(raw).hostname.toLowerCase();
  } catch {
    return '';
  }
};
const controlNodeIdentities = (item: ClusterControlNode) => {
  const values = [item.id, item.name, item.url, hostFromUrl(item.url), ...(item.ipAddresses || [])];
  return new Set(values.map((entry) => normalizeIdentity(entry)).filter(Boolean));
};
const expectedControlType = (node?: ClusterNode) => {
  if (!node) return '';
  if (node.role === 'active') return 'primary';
  if (node.role === 'standby') return 'secondary';
  return '';
};
const controlNodeForRuntimeNode = (node?: ClusterNode | null) => {
  if (!node || !clusterControl.value?.initialized) return null;
  const runtimeKeys = [node.id, node.address].map((entry) => normalizeIdentity(entry)).filter(Boolean);
  if (runtimeKeys.length) {
    const exact = clusterControlNodes.value.find((item) => runtimeKeys.some((key) => controlNodeIdentities(item).has(key)));
    if (exact) return exact;
  }
  const expectedType = expectedControlType(node);
  if (!expectedType) return null;
  const typed = clusterControlNodes.value.filter((item) => item.type === expectedType);
  return typed.length === 1 ? typed[0] : null;
};
const joinJobForRuntimeNode = (node?: ClusterNode | null) => {
  if (!node) return null;
  const runtimeKeys = new Set(
    [node.id, node.address, controlNodeForRuntimeNode(node)?.url, ...(controlNodeForRuntimeNode(node)?.ipAddresses || [])]
      .map((entry) => normalizeIdentity(entry))
      .filter(Boolean)
  );
  const matched = joinJobs.value.filter((job) => {
    const keys = [job.nodeId, job.peerAddress].map((entry) => normalizeIdentity(entry)).filter(Boolean);
    return keys.some((key) => runtimeKeys.has(key));
  });
  if (!matched.length) return null;
  return matched.sort((a, b) => new Date(b.updatedAt || 0).getTime() - new Date(a.updatedAt || 0).getTime())[0];
};
const isVirtualStandbyNode = (item?: ClusterNode) => {
  if (!item) return false;
  const id = String(item.id || '').toLowerCase();
  const addr = String(item.address || '').toLowerCase();
  return id === 'peer' && (addr === '' || addr === 'peer' || addr === '-');
};
const activeNode = computed(() => nodes.value.find((item) => item.role === 'active'));
const standbyNode = computed(() =>
  nodes.value.find((item) => item.role === 'standby' && !isVirtualStandbyNode(item))
);

const storageAlignmentState = computed<AlignmentState>(() => {
  const dhcp = normalizeRole(overview.value?.dhcpRole);
  const mysql = normalizeRole(overview.value?.mysqlRole);
  const redis = normalizeRole(overview.value?.redisRole);
  if (dhcp === 'unknown' || mysql === 'unknown' || redis === 'unknown') return 'unknown';
  const expected = dhcp === 'primary' ? 'primary' : 'replica';
  return mysql === expected && redis === expected ? 'aligned' : 'misaligned';
});

const avgLatency = computed(() => {
  if (!syncRows.value.length) return 0;
  const values = syncRows.value.map((item) => Number(String(item.latency || '').replace(/[^\d.]/g, '')) || 0);
  return Math.round(values.reduce((a, b) => a + b, 0) / values.length);
});

const syncException = computed(() => {
  const hasCriticalNode = nodes.value.some((item) => normHealth(item.health) === 'critical');
  return hasCriticalNode || avgLatency.value > 180;
});

const pendingEvents = computed(() =>
  sortedEvents.value.filter((item) => item.status !== 'success').slice(0, 6)
);

const statCards = computed(() => {
  const abnormalNode = nodes.value.some((item) => normHealth(item.health) === 'critical');
  const warnNode = nodes.value.some((item) => normHealth(item.health) === 'warning');
  const alignment = storageAlignmentState.value;
  const activeConsistency = nodeConsistency(activeNode.value);
  const runtimeLevel = abnormalNode ? 'danger' : warnNode ? 'warning' : 'normal';
  const syncLevel = syncException.value ? 'danger' : avgLatency.value > 120 ? 'warning' : 'normal';
  const storageLevel = alignment === 'misaligned' ? 'danger' : alignment === 'unknown' ? 'normal' : !overview.value?.writeGateOpen ? 'warning' : 'normal';
  const pendingLevel = pendingEvents.value.length > 0 ? 'warning' : 'normal';

  return [
    {
      key: 'runtime' as CardKey,
      label: t('cluster.runtime.statRuntime'),
      value: `DHCP ${storageRoleLabel(overview.value?.dhcpRole)}`,
      sub: `${storageHealthLabel(overview.value?.dhcpHealth)} · Active ${activeNode.value?.address || '-'}`,
      badge: {
        text: activeConsistency === 'bad' ? t('cluster.runtime.badgeMisaligned') : activeConsistency === 'warn' ? t('cluster.runtime.badgeAttention') : t('cluster.runtime.badgeConsistent'),
        type: consistencyTag(activeConsistency)
      },
      level: runtimeLevel
    },
    {
      key: 'sync' as CardKey,
      label: t('cluster.runtime.statSync'),
      value: t('cluster.runtime.statSyncValue', { ms: avgLatency.value }),
      sub: t('cluster.runtime.statSyncSub', { time: formatTime(syncRows.value[0]?.last) }),
      level: syncLevel
    },
    {
      key: 'storage' as CardKey,
      label: t('cluster.runtime.statStorage'),
      value: alignment === 'aligned' ? t('cluster.runtime.statStorageAligned') : alignment === 'misaligned' ? t('cluster.runtime.statStorageMisaligned') : t('cluster.runtime.statStorageUnknown'),
      sub: `${overview.value?.writeGateOpen ? t('cluster.runtime.statWriteGateOpen') : t('cluster.runtime.statWriteGateClosed')} · Fencing ${overview.value?.fencingEpoch || '-'}`,
      badge: {
        text: alignment === 'aligned' ? t('cluster.runtime.statAligned') : alignment === 'misaligned' ? t('cluster.runtime.statMisaligned') : t('cluster.runtime.statUnknown'),
        type: alignment === 'aligned' ? 'success' : alignment === 'misaligned' ? 'danger' : 'info'
      },
      level: storageLevel
    },
    {
      key: 'pending' as CardKey,
      label: t('cluster.runtime.statPending'),
      value: String(pendingEvents.value.length),
      sub: t('cluster.runtime.statPendingSub'),
      level: pendingLevel
    }
  ];
});

const tableRows = computed(() =>
  nodes.value.filter((item) => {
    const byStatus = statusFilter.value ? normHealth(item.health) === statusFilter.value : true;
    const byRole = roleFilter.value ? item.role === roleFilter.value : true;
    const byKeyword = keyword.value ? String(item.address || '').includes(keyword.value.trim()) : true;
    return byStatus && byRole && byKeyword;
  })
);

const sortedEvents = computed(() =>
  [...events.value]
    .sort((a, b) => {
      const score = (x: ClusterFailoverEvent['type']) => (x === 'danger' ? 3 : x === 'warning' ? 2 : 1);
      const severity = score(b.type) - score(a.type);
      if (severity !== 0) return severity;
      return new Date(b.time || '').getTime() - new Date(a.time || '').getTime();
    })
    .map((item) => ({ ...item, time: formatTime(item.time) }))
);

const loadData = async () => {
  loading.value = true;
  try {
    const [overviewResp, syncResp, eventResp, controlResp, joinJobsResp] = await Promise.all([
      getClusterOverview(),
      getClusterSyncStatus(),
      getClusterFailoverHistory(),
      getClusterControl(),
      getClusterJoinJobs()
    ]);
    overview.value = unwrap<ClusterOverview>(overviewResp);
    syncRows.value = unwrap<ClusterSyncStatus[]>(syncResp) || [];
    events.value = unwrap<ClusterFailoverEvent[]>(eventResp) || [];
    clusterControl.value = unwrap<ClusterControl>(controlResp);
    joinJobs.value = unwrap<{ items: ClusterJoinJob[]; count: number }>(joinJobsResp)?.items || [];
  } catch (err) {
    showHttpError(err, t('cluster.runtime.loadFail'));
  } finally {
    loading.value = false;
  }
};

const canNodeSync = (node: ClusterNode | undefined) => Boolean(node && normHealth(node.health) !== 'critical');

const manualSync = async (node: ClusterNode | undefined) => {
  if (!node || !canNodeSync(node)) return;
  syncingId.value = node.id;
  try {
    const sourceNode = node.address || node.id || 'active';
    const targetNode =
      node.role === 'active'
        ? standbyNode.value?.address || standbyNode.value?.id || 'standby'
        : activeNode.value?.address || activeNode.value?.id || 'active';
    const resp = await triggerClusterSync({
      type: 'lease',
      sourceNode,
      targetNode
    });
    const evt = unwrap<ClusterFailoverEvent>(resp);
    const suffix = evt?.txId ? t('cluster.runtime.syncTxSuffix', { txId: evt.txId }) : '';
    showSuccess(t('cluster.runtime.syncTriggered', { addr: node.address, suffix }));
    await loadData();
  } catch (err) {
    showHttpError(err, t('cluster.runtime.syncFail'));
  } finally {
    syncingId.value = '';
  }
};

const handleEvent = async (evt: ClusterFailoverEvent) => {
  handlingEventId.value = evt.id;
  try {
    await new Promise((resolve) => setTimeout(resolve, 300));
    showSuccess(t('cluster.runtime.eventHandled'));
    events.value = events.value.map((item) => (item.id === evt.id ? { ...item, status: 'success', type: 'info' } : item));
  } finally {
    handlingEventId.value = '';
  }
};

const showDetail = (node: ClusterNode) => {
  detailNode.value = node;
  detailVisible.value = true;
};

const exportNodes = () => {
  const head = [t('cluster.runtime.csvNodeIp'), t('cluster.runtime.csvRole'), t('cluster.runtime.csvHealth'), t('cluster.runtime.csvCpu'), t('cluster.runtime.csvMem'), t('cluster.runtime.csvActiveLeases'), t('cluster.runtime.csvTotalLeases'), t('cluster.runtime.csvLastSync')];
  const rows = tableRows.value.map((item) => [
    item.address,
    roleLabel(item.role),
    healthLabel(item.health),
    pct(item.cpuPercent),
    pct(item.memoryPercent),
    String(activeLeaseCount(item)),
    String(totalLeaseCount(item)),
    formatTime(item.lastHeartbeat)
  ]);
  const csv = [head, ...rows].map((line) => line.map((cell) => `"${String(cell).replace(/"/g, '""')}"`).join(',')).join('\n');
  const blob = new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `cluster_runtime_${Date.now()}.csv`;
  a.click();
  URL.revokeObjectURL(url);
};

const goConfig = () => router.push('/cluster/config');
const goAudit = () => router.push('/cluster/sync-audit-dr');

const normHealth = (value: unknown) => {
  const v = String(value || '').toLowerCase();
  if (!v || v.includes('unknown')) return 'unknown';
  if (v.includes('healthy')) return 'healthy';
  if (v.includes('warning')) return 'warning';
  return 'critical';
};

const healthLabel = (value: unknown) => {
  const state = normHealth(value);
  if (state === 'healthy') return t('cluster.runtime.healthHealthy');
  if (state === 'warning') return t('cluster.runtime.healthWarning');
  if (state === 'unknown') return t('cluster.runtime.healthUnknown');
  return t('cluster.runtime.healthCritical');
};

const healthTag = (value: unknown) => {
  const state = normHealth(value);
  if (state === 'healthy') return 'success';
  if (state === 'warning') return 'warning';
  if (state === 'unknown') return 'info';
  return 'danger';
};

const roleLabel = (value: string) => {
  if (value === 'active') return t('cluster.runtime.roleActive');
  if (value === 'standby') return t('cluster.runtime.roleStandby');
  return t('cluster.runtime.roleWorker');
};

const storageRoleLabel = (value: unknown) => {
  const v = String(value || '').toLowerCase();
  if (v === 'primary' || v === 'active') return t('cluster.runtime.storagePrimary');
  if (v === 'replica' || v === 'standby') return t('cluster.runtime.storageReplica');
  return t('cluster.runtime.storageUnknown');
};

const storageHealthLabel = (value: unknown) => {
  const v = String(value || '').toLowerCase();
  if (v.includes('healthy')) return t('cluster.runtime.storageHealthy');
  if (v.includes('warning') || v.includes('degraded')) return t('cluster.runtime.storageWarning');
  if (v.includes('error') || v.includes('critical')) return t('cluster.runtime.storageCritical');
  return t('cluster.runtime.storageHealthUnknown');
};

const normalizeRole = (value: unknown) => {
  const v = String(value || '').toLowerCase();
  if (v === 'primary' || v === 'active' || v === 'master') return 'primary';
  if (v === 'replica' || v === 'standby' || v === 'secondary' || v === 'slave') return 'replica';
  return 'unknown';
};

const normalizeStorageHealth = (value: unknown) => {
  const v = String(value || '').toLowerCase();
  if (v.includes('healthy')) return 'healthy';
  if (v.includes('warning') || v.includes('degraded')) return 'warning';
  if (v.includes('critical') || v.includes('error')) return 'critical';
  return 'unknown';
};

const expectedRoleByNode = (nodeRole?: string) => {
  if (nodeRole === 'active') return 'primary';
  if (nodeRole === 'standby') return 'replica';
  return 'unknown';
};

const serviceMetricText = (service: ServiceKey) => {
  if (service === 'mysql') return `${overview.value?.mysqlLagMs ?? overview.value?.replicationLagMs ?? 0} ms`;
  if (service === 'redis') return String(overview.value?.redisOffsetLag ?? 0);
  return '';
};

const serviceViewsForNode = (node?: ClusterNode | null) => {
  if (!node) {
    return [
      { key: 'dhcp' as ServiceKey, label: 'DHCP', role: 'unknown', health: 'unknown', lagText: '', expectedRole: 'unknown', consistency: 'unknown' as ConsistencyState },
      { key: 'mysql' as ServiceKey, label: 'MySQL', role: 'unknown', health: 'unknown', lagText: '', expectedRole: 'unknown', consistency: 'unknown' as ConsistencyState },
      { key: 'redis' as ServiceKey, label: 'Redis', role: 'unknown', health: 'unknown', lagText: '', expectedRole: 'unknown', consistency: 'unknown' as ConsistencyState }
    ];
  }
  const expectedRole = expectedRoleByNode(node?.role);
  const records: Array<{ key: ServiceKey; label: string; role: unknown; health: unknown; lagText: string }> = [
    { key: 'dhcp', label: 'DHCP', role: overview.value?.dhcpRole, health: overview.value?.dhcpHealth, lagText: '' },
    { key: 'mysql', label: 'MySQL', role: overview.value?.mysqlRole, health: overview.value?.mysqlHealth, lagText: serviceMetricText('mysql') },
    { key: 'redis', label: 'Redis', role: overview.value?.redisRole, health: overview.value?.redisHealth, lagText: serviceMetricText('redis') }
  ];
  return records.map((item) => {
    const role = normalizeRole(item.role);
    const health = normalizeStorageHealth(item.health);
    const roleMismatch = expectedRole !== 'unknown' && role !== 'unknown' && role !== expectedRole;
    const consistency: ConsistencyState = roleMismatch || health === 'critical' ? 'bad' : health === 'warning' ? 'warn' : health === 'unknown' ? 'unknown' : 'good';
    return {
      ...item,
      role,
      health,
      expectedRole,
      consistency
    };
  });
};

const nodeConsistency = (node?: ClusterNode | null): ConsistencyState => {
  if (!node) return 'unknown';
  const services = serviceViewsForNode(node);
  if (services.some((item) => item.consistency === 'bad')) return 'bad';
  if (services.some((item) => item.consistency === 'warn')) return 'warn';
  if (services.some((item) => item.consistency === 'unknown')) return 'unknown';
  return 'good';
};

const consistencyTag = (value: ConsistencyState) => {
  if (value === 'good') return 'success';
  if (value === 'warn') return 'warning';
  if (value === 'unknown') return 'info';
  return 'danger';
};

const consistencyText = (value: ConsistencyState) => {
  if (value === 'good') return t('cluster.runtime.consistencyGood');
  if (value === 'warn') return t('cluster.runtime.consistencyWarn');
  if (value === 'unknown') return t('cluster.runtime.consistencyUnknown');
  return t('cluster.runtime.consistencyBad');
};

const consistencyClass = (value: ConsistencyState) => {
  if (value === 'good') return 'consistency-good';
  if (value === 'warn') return 'consistency-warn';
  if (value === 'unknown') return 'consistency-unknown';
  return 'consistency-bad';
};

const alignmentLabel = () =>
  storageAlignmentState.value === 'aligned' ? t('cluster.runtime.alignAligned') : storageAlignmentState.value === 'misaligned' ? t('cluster.runtime.alignMisaligned') : t('cluster.runtime.alignUnknown');
const alignmentTag = () =>
  storageAlignmentState.value === 'aligned' ? 'success' : storageAlignmentState.value === 'misaligned' ? 'danger' : 'info';
const writeGateLabel = () => (overview.value?.writeGateOpen ? t('cluster.runtime.writeGateOpen') : t('cluster.runtime.writeGateClosed'));
const writeGateTag = () => (overview.value?.writeGateOpen ? 'success' : 'warning');
const controlNodeStateLabel = (value: string) => {
  if (value === 'self') return t('cluster.runtime.controlSelf');
  if (value === 'connected') return t('cluster.runtime.controlConnected');
  if (value === 'unreachable') return t('cluster.runtime.controlUnreachable');
  if (value === 'unknown') return t('cluster.runtime.controlUnknownState');
  return value || t('cluster.runtime.controlUnknownState');
};
const controlNodeStateTag = (value: string) => {
  if (value === 'self' || value === 'connected') return 'success';
  if (value === 'unreachable') return 'danger';
  return 'info';
};
const controlBindingTag = (node?: ClusterNode | null) => {
  if (!clusterControl.value?.initialized) return 'info';
  const mapped = controlNodeForRuntimeNode(node);
  if (!mapped) return 'warning';
  return controlNodeStateTag(mapped.state);
};
const controlBindingLabel = (node?: ClusterNode | null) => {
  if (!clusterControl.value?.initialized) return t('cluster.runtime.controlNotInit');
  const mapped = controlNodeForRuntimeNode(node);
  if (!mapped) return t('cluster.runtime.controlUnmapped');
  const prefix = mapped.type === 'primary' ? t('cluster.runtime.controlPrimaryLabel') : mapped.type === 'secondary' ? t('cluster.runtime.controlSecondaryLabel') : mapped.type;
  return `${prefix} / ${controlNodeStateLabel(mapped.state)}`;
};
const controlBindingMeta = (node?: ClusterNode | null) => {
  if (!clusterControl.value?.initialized) return t('cluster.runtime.controlNotInitMeta');
  const mapped = controlNodeForRuntimeNode(node);
  if (!mapped) return t('cluster.runtime.controlUnmappedMeta');
  const endpoint = mapped.url || mapped.ipAddresses.join(', ') || mapped.id;
  const heartbeat = mapped.lastSeen ? ` · ${formatTime(mapped.lastSeen)}` : '';
  return `${endpoint || '-'}${heartbeat}`;
};
const controlBindingShortMeta = (node?: ClusterNode | null) => {
  if (!clusterControl.value?.initialized) return t('cluster.runtime.controlShortNotInit');
  const mapped = controlNodeForRuntimeNode(node);
  if (!mapped) return t('cluster.runtime.controlShortUnmapped');
  return mapped.url || mapped.ipAddresses[0] || mapped.id;
};
const isJoinJobRunning = (status?: string) => ['REGISTERED', 'SNAPSHOTTING', 'CATCHING_UP', 'VERIFYING'].includes(String(status || '').toUpperCase());
const joinJobStatusLabel = (status?: string) => {
  const value = String(status || '').toUpperCase();
  if (value === 'REGISTERED') return t('cluster.runtime.joinRegistered');
  if (value === 'SNAPSHOTTING') return t('cluster.runtime.joinSnapshotting');
  if (value === 'CATCHING_UP') return t('cluster.runtime.joinCatchingUp');
  if (value === 'VERIFYING') return t('cluster.runtime.joinVerifying');
  if (value === 'WARM_STANDBY') return t('cluster.runtime.joinWarmStandby');
  if (value === 'ACTIVE') return t('cluster.runtime.joinActive');
  if (value === 'FAILED') return t('cluster.runtime.joinFailed');
  if (value === 'CANCELED') return t('cluster.runtime.joinCanceled');
  return t('cluster.runtime.joinNotOrchestrated');
};
const joinJobStatusTag = (status?: string) => {
  const value = String(status || '').toUpperCase();
  if (value === 'FAILED') return 'danger';
  if (value === 'WARM_STANDBY' || value === 'ACTIVE') return 'success';
  if (value === 'CANCELED') return 'info';
  if (isJoinJobRunning(value)) return 'warning';
  return 'info';
};
const joinJobLabel = (node?: ClusterNode | null) => {
  const job = joinJobForRuntimeNode(node);
  if (!job) return t('cluster.runtime.joinNotTriggered');
  return joinJobStatusLabel(job.status);
};
const joinJobTag = (node?: ClusterNode | null) => {
  const job = joinJobForRuntimeNode(node);
  if (!job) return 'info';
  return joinJobStatusTag(job.status);
};
const joinJobMeta = (node?: ClusterNode | null) => {
  const job = joinJobForRuntimeNode(node);
  if (!job) return t('cluster.runtime.joinNotTriggeredMeta');
  const latestPhase = [...(job.phases || [])].reverse().find((phase) => phase);
  const phaseText = latestPhase ? `${joinJobStatusLabel(latestPhase.phase)} / ${latestPhase.status}` : joinJobStatusLabel(job.status);
  const errorText = latestPhase?.error || job.error;
  const checkpoint = job.checkpoint?.phase ? ` · ${joinJobStatusLabel(job.checkpoint.phase)}${job.checkpoint.resumeCount ? `(${job.checkpoint.resumeCount})` : ''}` : '';
  const updatedText = job.updatedAt ? ` · ${formatTime(job.updatedAt)}` : '';
  return `${phaseText}${checkpoint}${errorText ? ` · ${errorText}` : ''}${updatedText}`;
};
const joinJobShortMeta = (node?: ClusterNode | null) => {
  const job = joinJobForRuntimeNode(node);
  if (!job) return t('cluster.runtime.joinNotOrchestrated');
  if (job.catchUp?.targetOffset) {
    return `${job.jobId} · ${t('cluster.runtime.catchUpTarget', { target: job.catchUp.targetOffset })}`;
  }
  if (job.snapshot?.consistencyChecksum) {
    return `${job.jobId} · ${job.snapshot.consistencyChecksum}`;
  }
  return job.jobId;
};

const joinSnapshotBrief = (node?: ClusterNode | null) => {
	const snapshot = joinJobForRuntimeNode(node)?.snapshot;
	if (!snapshot) return t('cluster.runtime.snapshotNotGenerated');
	const captured = snapshot.capturedAt ? ` · ${formatTime(snapshot.capturedAt)}` : '';
	return t('cluster.runtime.snapshotBrief', { status: snapshot.status || '-', pool: snapshot.poolCount ?? 0, binding: snapshot.bindingCount ?? 0, lease: snapshot.leaseCount ?? 0, captured });
};

const joinCatchUpBrief = (node?: ClusterNode | null) => {
	const catchUp = joinJobForRuntimeNode(node)?.catchUp;
	if (!catchUp) return t('cluster.runtime.catchUpNotStarted');
	const target = catchUp.targetOffset ? t('cluster.runtime.catchUpTarget', { target: catchUp.targetOffset }) : t('cluster.runtime.catchUpTarget', { target: '-' });
	const current = catchUp.currentOffset ? t('cluster.runtime.catchUpCurrent', { current: catchUp.currentOffset }) : t('cluster.runtime.catchUpCurrent', { current: '-' });
	return t('cluster.runtime.catchUpBrief', { status: catchUp.status || '-', target, current });
};

const joinVerificationLabel = (node?: ClusterNode | null) => {
  const verification = joinJobForRuntimeNode(node)?.verification;
  if (!verification) return t('cluster.runtime.verifyNotGenerated');
  return String(verification.status || '').toUpperCase() === 'VERIFIED' ? t('cluster.runtime.verifyPassed') : t('cluster.runtime.verifyFailed');
};

const joinVerificationTag = (node?: ClusterNode | null) => {
  const verification = joinJobForRuntimeNode(node)?.verification;
  if (!verification) return 'info';
  return String(verification.status || '').toUpperCase() === 'VERIFIED' ? 'success' : 'danger';
};

const joinVerificationCheckedAt = (node?: ClusterNode | null) => {
  const verification = joinJobForRuntimeNode(node)?.verification;
  return formatTime(verification?.checkedAt);
};

const joinVerificationMode = (node?: ClusterNode | null) => {
  const verification = joinJobForRuntimeNode(node)?.verification;
  return verification?.replicationMode || '-';
};

const joinVerificationSource = (node?: ClusterNode | null) => {
  const verification = joinJobForRuntimeNode(node)?.verification;
  return verification?.replicationSource || '-';
};

const joinVerificationReplication = (node?: ClusterNode | null) => {
  const verification = joinJobForRuntimeNode(node)?.verification;
  if (!verification) return '-';
  const state = verification.replicationHealthy ? t('cluster.runtime.replicaHealthy') : t('cluster.runtime.replicaUnhealthy');
  const lag = `${verification.replicationLagMs ?? 0} ms`;
  const offset = verification.replicationOffset ? ` · ${verification.replicationOffset}` : '';
  return `${state} · Lag ${lag}${offset}`;
};

const joinVerificationState = (node?: ClusterNode | null) => {
  const verification = joinJobForRuntimeNode(node)?.verification;
  return verification?.replicationState || '-';
};

const joinVerificationSync = (node?: ClusterNode | null) => {
  const verification = joinJobForRuntimeNode(node)?.verification;
  if (!verification?.lastSyncTxId) return '-';
  return `${verification.lastSyncType || 'sync'} / ${verification.lastSyncTxId}`;
};

const joinVerificationDataset = (node?: ClusterNode | null) => {
  const verification = joinJobForRuntimeNode(node)?.verification;
  if (!verification) return '-';
  const pools = verification.poolCount ?? 0;
  const bindings = verification.bindingCount ?? 0;
  const leases = verification.leaseCount ?? 0;
  const activeLeases = verification.activeLeaseCount ?? 0;
  const created = verification.createdLeaseCount24h ?? 0;
  const conflicts = verification.conflictLeaseCount24h ?? 0;
  const checksum = verification.consistencyChecksum ? ` · Checksum ${verification.consistencyChecksum}` : '';
  return `Pool ${pools} / Binding ${bindings} / Lease ${leases} / ${t('cluster.runtime.datasetActiveLease', { count: activeLeases })} / ${t('cluster.runtime.dataset24hNew', { count: created })} / ${t('cluster.runtime.dataset24hConflict', { count: conflicts })}${checksum}`;
};

const joinSnapshotSummary = (node?: ClusterNode | null) => {
  const snapshot = joinJobForRuntimeNode(node)?.snapshot;
  if (!snapshot) return '-';
  const source = snapshot.sourceAddress || snapshot.sourceNode || '-';
  const tx = snapshot.lastSyncTxId ? ` · Tx ${snapshot.lastSyncType || 'sync'} / ${snapshot.lastSyncTxId}` : '';
  const checksum = snapshot.consistencyChecksum ? ` · Checksum ${snapshot.consistencyChecksum}` : '';
  const offset = snapshot.replicationOffset ? ` · Offset ${snapshot.replicationOffset}` : '';
  return `${snapshot.status || '-'} · ${t('cluster.runtime.snapshotSource', { source })} · Pool ${snapshot.poolCount ?? 0} / Binding ${snapshot.bindingCount ?? 0} / Lease ${snapshot.leaseCount ?? 0}${offset}${tx}${checksum}`;
};

const joinCatchUpSummary = (node?: ClusterNode | null) => {
  const catchUp = joinJobForRuntimeNode(node)?.catchUp;
  if (!catchUp) return '-';
  const target = catchUp.targetOffset || '-';
  const current = catchUp.currentOffset || '-';
  const timing = catchUp.completedAt ? ` · ${t('cluster.runtime.catchUpCompleted', { time: formatTime(catchUp.completedAt) })}` : catchUp.updatedAt ? ` · ${t('cluster.runtime.catchUpUpdated', { time: formatTime(catchUp.updatedAt) })}` : '';
  const watermark = catchUp.highWatermarkReached ? t('cluster.runtime.catchUpReached') : t('cluster.runtime.catchUpNotReached');
  return `${catchUp.status || '-'} · ${watermark} · Baseline ${catchUp.baselineOffset || '-'} · Target ${target} · Current ${current}${timing}`;
};

const joinCheckpointSummary = (node?: ClusterNode | null) => {
  const checkpoint = joinJobForRuntimeNode(node)?.checkpoint;
  if (!checkpoint) return '-';
  const lastCompleted = checkpoint.lastCompletedPhase ? ` · ${t('cluster.runtime.checkpointLastCompleted', { phase: joinJobStatusLabel(checkpoint.lastCompletedPhase) })}` : '';
  const resumable = checkpoint.resumable ? t('cluster.runtime.checkpointResumable') : t('cluster.runtime.checkpointNotResumable');
  const resumeCount = checkpoint.resumeCount ? ` · ${t('cluster.runtime.checkpointResumed', { count: checkpoint.resumeCount })}` : '';
  const lastError = checkpoint.lastError ? ` · ${checkpoint.lastError}` : '';
  return `${joinJobStatusLabel(checkpoint.phase)} / ${checkpoint.status || '-'} · ${resumable}${lastCompleted}${resumeCount}${lastError}`;
};

const joinVerificationMessage = (node?: ClusterNode | null) => {
  const verification = joinJobForRuntimeNode(node)?.verification;
  if (!verification) return '-';
  return verification.warning || verification.failureReason || '-';
};

const formatTime = (value?: string) => {
  if (!value) return '-';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString();
};

const pct = (value: unknown) => `${Math.max(0, Math.round(Number(value || 0)))}%`;
const alertValueClass = (value: unknown) => (Number(value || 0) > 90 ? 'danger-text' : Number(value || 0) >= 70 ? 'warn-text' : 'good-text');
const metricColor = (value: number) => {
  if (value > 90) return '#F44336';
  if (value >= 70) return '#FFB300';
  return '#00C853';
};
const statusClass = (value: unknown) =>
  normHealth(value) === 'critical'
    ? 'danger-text'
    : normHealth(value) === 'warning'
      ? 'warn-text'
      : normHealth(value) === 'unknown'
        ? 'mute-text'
        : 'good-text';
const nodePanelClass = (node: ClusterNode | undefined) => {
  if (!node) return 'is-unknown';
  if (nodeConsistency(node) === 'unknown') return 'is-unknown';
  if (nodeConsistency(node) === 'bad' || normHealth(node.health) === 'critical') return 'is-danger';
  if (nodeConsistency(node) === 'warn' || normHealth(node.health) === 'warning') return 'is-warning';
  return '';
};
const activeLeaseCount = (node: ClusterNode) => Math.max(0, Math.round(Number(node.activeLeasesRedis) || 0));
const totalLeaseCount = (node: ClusterNode) => Math.max(activeLeaseCount(node), Math.round(Number(node.totalLeasesMySQL) || 0));
const badgeClass = (type?: string) => {
  if (type === 'danger') return 'status-danger';
  if (type === 'warning') return 'status-warning';
  if (type === 'success') return 'status-success';
  return 'status-primary';
};
const consistencyClassName = (value: ConsistencyState) => {
  if (value === 'good') return 'status-success';
  if (value === 'warn') return 'status-warning';
  if (value === 'bad') return 'status-danger';
  return 'status-muted';
};
const serviceBadgeText = (value: ServiceKey) => {
  if (value === 'dhcp') return 'D';
  if (value === 'mysql') return 'M';
  return 'R';
};

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

.control-brief-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.control-brief-card {
  padding: 16px;
  min-height: 132px;
}

.control-brief-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.control-brief-title {
  font-size: 13px;
  color: var(--text-secondary);
}

.control-brief-value {
  display: block;
  margin-top: 10px;
  font-size: 20px;
  line-height: 1.3;
}

.control-brief-sub {
  margin: 8px 0 0;
  color: var(--text-secondary);
  font-size: 12px;
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

:deep(.status-primary.el-tag) {
  background: var(--brand-primary);
}

.card-title {
  font-weight: 600;
}

.row-between {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.stat-card {
  padding: 16px;
  min-height: 150px;
}

.stat-card.is-danger {
  border-color: var(--state-danger);
}

.stat-card.is-warning {
  border-color: var(--state-warning);
}

.stat-label {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
}

.stat-value {
  display: block;
  margin-top: 8px;
  font-size: 22px;
}

.stat-sub {
  display: block;
  margin-top: 8px;
  color: var(--text-secondary);
  font-size: 12px;
}

.stat-badge {
  margin-top: 8px;
}

.grid-2-1 {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: 16px;
}

.topology-wrap {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  gap: 16px;
  align-items: center;
}

.node-panel {
  border: 1px solid var(--surface-border);
  border-radius: 14px;
  padding: 18px;
}

.node-panel.is-danger {
  border-color: var(--state-danger);
}

.node-panel.is-warning {
  border-color: var(--state-warning);
}

.node-panel.is-unknown {
  border-color: var(--state-muted);
}

.node-panel h4 {
  margin: 0 0 8px;
}

.node-panel p {
  margin: 4px 0;
  color: var(--text-secondary);
}

.control-binding-row {
  margin: 8px 0 4px;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.control-binding-label {
  color: var(--text-secondary);
  font-size: 12px;
}

.control-binding-meta {
  min-height: 18px;
  font-size: 12px;
  color: var(--text-secondary);
}

.service-inline-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin: 8px 0;
}

.service-inline-item {
  border: 1px solid var(--surface-border);
  border-radius: 12px;
  padding: 8px;
  background: color-mix(in srgb, var(--surface-bg) 92%, #94a3b8 8%);
}

.service-title-wrap {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.service-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  font-size: 12px;
  font-weight: 700;
  color: #fff;
}

.service-badge-dhcp {
  background: var(--brand-primary);
}

.service-badge-mysql {
  background: var(--state-success);
}

.service-badge-redis {
  background: var(--state-warning);
}

.service-inline-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}

.service-inline-item p {
  margin: 2px 0;
  font-size: 12px;
}

.consistency-good {
  border-color: var(--state-success);
}

.consistency-warn {
  border-color: var(--state-warning);
}

.consistency-bad {
  border-color: var(--state-danger);
}

.consistency-unknown {
  border-color: var(--state-muted);
}

.consistency-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.node-meter-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-top: 12px;
}

.node-meter-item {
  display: grid;
  gap: 6px;
}

.meter-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
  color: var(--text-secondary);
}

.meter-head strong {
  font-size: 13px;
}

.panel-actions {
  margin-top: 12px;
  display: flex;
  gap: 8px;
}

.link-panel {
  text-align: center;
  display: grid;
  gap: 8px;
  justify-items: center;
}

.sync-line {
  margin-top: 8px;
  padding: 8px 10px;
  border-radius: 4px;
  font-weight: 600;
}

.sync-line.is-good {
  border: 1px solid var(--state-success);
  color: var(--state-success);
}

.sync-line.is-bad {
  border: 1px dashed var(--state-danger);
  color: var(--state-danger);
}

.event-list {
  display: grid;
  gap: 10px;
}

.event-item {
  border: 1px solid var(--surface-border);
  border-radius: 12px;
  padding: 10px;
  display: flex;
  justify-content: space-between;
  gap: 10px;
}

.event-item.is-danger {
  border-color: var(--state-danger);
}

.event-item.is-warning {
  border-color: var(--state-warning);
}

.event-main p {
  margin: 4px 0;
  color: var(--text-secondary);
}

.event-main small {
  color: var(--text-secondary);
}

.filter-bar {
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.bar-left,
.bar-right {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.row-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: nowrap;
  white-space: nowrap;
}

.control-cell {
  display: grid;
  gap: 4px;
}

.control-cell small {
  color: var(--text-secondary);
}

.metric-cell {
  display: grid;
  gap: 6px;
}

.metric-cell strong {
  font-size: 12px;
}

.row-actions :deep(.el-button + .el-button) {
  margin-left: 0;
}

.grid-single-side {
  grid-template-columns: minmax(0, 1fr);
}

.detail-pre {
  margin: 0;
  margin-top: 12px;
  padding: 12px;
  border: 1px solid var(--surface-border);
  border-radius: 12px;
  background: color-mix(in srgb, var(--surface-bg) 88%, #94a3b8 12%);
  white-space: pre-wrap;
}

.detail-kv {
  margin-bottom: 8px;
}

.detail-service-grid {
  margin-bottom: 12px;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.detail-service-item {
  border: 1px solid var(--surface-border);
  border-radius: 12px;
  padding: 8px;
}

.detail-service-item p {
  margin: 3px 0;
  color: var(--text-secondary);
  font-size: 12px;
}

.good-text {
  color: var(--state-success);
}

.warn-text {
  color: var(--state-warning);
}

.bad-text,
.danger-text {
  color: var(--state-danger);
}

.mute-text {
  color: var(--state-muted);
}

:deep(.el-form-item) {
  margin-bottom: 18px;
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
  .control-brief-grid,
  .stats-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .grid-2-1 {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 1200px) {
  .page-wrap {
    padding: 16px;
  }

  .control-brief-grid {
    grid-template-columns: 1fr;
  }

  .topology-wrap {
    grid-template-columns: 1fr;
  }

  .node-meter-grid {
    grid-template-columns: 1fr;
  }

  .filter-bar {
    flex-direction: column;
    align-items: stretch;
  }

  .service-inline-grid,
  .detail-service-grid {
    grid-template-columns: 1fr;
  }
}
</style>
