<template>
  <div class="page-wrap" v-loading="loading">
    <section class="page-header surface-card">
      <div class="page-header-main">
        <div>
          <h3>{{ t('cluster.config.title') }}</h3>
          <p>{{ t('cluster.config.desc') }}</p>
        </div>
        <div class="page-header-actions">
          <el-button @click="resetForm">{{ t('cluster.config.reset') }}</el-button>
          <el-tooltip :content="writeGuardReason" placement="top" :disabled="!writeGuardBlocked">
            <span class="guard-disabled-wrap">
              <el-button :loading="checking" :disabled="writeGuardBlocked" @click="precheck">{{ t('cluster.config.precheck') }}</el-button>
            </span>
          </el-tooltip>
          <el-tooltip :content="writeGuardReason" placement="top" :disabled="!writeGuardBlocked">
            <span class="guard-disabled-wrap">
              <el-button type="primary" :disabled="!prechecked || writeGuardBlocked" :loading="saving" @click="saveConfig">{{ t('cluster.config.save') }}</el-button>
            </span>
          </el-tooltip>
        </div>
      </div>
      <div v-if="writeGuardBlocked" class="guard-tip">
        <el-alert :title="t('cluster.config.writeGuardAlert')" type="warning" :closable="false" />
        <el-button size="small" type="warning" plain @click="showWriteGuardReason">{{ t('cluster.config.writeGuardWhy') }}</el-button>
      </div>
    </section>

    <section class="surface-card section-card">
      <div class="section-head">
        <div>
          <div class="section-title">{{ t('cluster.config.controlSection') }}</div>
          <div class="field-help">{{ t('cluster.config.controlHelp') }}</div>
        </div>
        <div class="cluster-control-actions">
          <el-button size="small" class="action-button" :icon="Refresh" :loading="controlLoading" @click="loadClusterControl">{{ t('cluster.config.refreshControl') }}</el-button>
          <el-button
            v-if="!clusterControl?.initialized"
            size="small"
            type="primary"
            class="action-button"
            :icon="Plus"
            :loading="controlSubmitting"
            @click="initializeControlPlane"
          >
            {{ t('cluster.config.initControl') }}
          </el-button>
          <el-button
            v-if="clusterControl?.initialized"
            size="small"
            type="primary"
            class="action-button"
            :icon="Connection"
            :disabled="writeGuardBlocked"
            @click="openJoinDialog"
          >
            {{ t('cluster.config.joinSecondary') }}
          </el-button>
          <el-button
            v-if="clusterControl?.initialized"
            size="small"
            type="danger"
            class="action-button danger-button"
            :icon="Delete"
            :loading="controlDeleting"
            @click="deleteControlPlane"
          >
            {{ t('cluster.config.deleteControl') }}
          </el-button>
        </div>
      </div>

      <div class="control-summary-grid">
        <div class="control-summary-item summary-neutral">
          <span class="summary-label">{{ t('cluster.config.initStatus') }}</span>
          <el-tag class="status-chip" :class="clusterControl?.initialized ? 'status-success' : 'status-muted'">{{ clusterControl?.initialized ? t('cluster.config.initYes') : t('cluster.config.initNo') }}</el-tag>
        </div>
        <div class="control-summary-item" :class="clusterControl?.apiTlsActive ? 'summary-success' : clusterControl?.apiTlsEnabled ? 'summary-warning' : 'summary-neutral'">
          <span class="summary-label">{{ t('cluster.config.tlsRuntime') }}</span>
          <el-tag class="status-chip" :class="clusterControl?.apiTlsActive ? 'status-success' : clusterControl?.apiTlsEnabled ? 'status-warning' : 'status-muted'">
            {{ clusterControlTlsLabel }}
          </el-tag>
        </div>
        <div class="control-summary-item summary-primary">
          <span class="summary-label">{{ t('cluster.config.primaryNode') }}</span>
          <strong>{{ clusterControl?.primaryNodeId || '-' }}</strong>
        </div>
        <div class="control-summary-item summary-neutral">
          <span class="summary-label">{{ t('cluster.config.configVersion') }}</span>
          <strong>{{ clusterControl?.configVersion ?? 0 }}</strong>
        </div>
      </div>

      <el-alert
        v-if="clusterControl?.restartRequired"
        :title="t('cluster.config.tlsRestartTitle')"
        type="warning"
        :closable="false"
        show-icon
        class="control-alert collapsible-alert"
      >
        <template #default>
          <div class="alert-head-row">
            <span>{{ t('cluster.config.tlsRestartBody') }}</span>
            <div class="alert-head-actions">
              <el-button v-if="clusterControl?.apiTlsCertFile" text size="small" :icon="DocumentCopy" @click="copyText(clusterControl?.apiTlsCertFile || '', t('cluster.config.certPathCopied'))">{{ t('cluster.config.copyCertPath') }}</el-button>
              <el-button v-if="clusterControl?.apiTlsClientCaFile" text size="small" :icon="DocumentCopy" @click="copyText(clusterControl?.apiTlsClientCaFile || '', t('cluster.config.caPathCopied'))">{{ t('cluster.config.copyCaPath') }}</el-button>
              <el-button text size="small" :icon="tlsNoticeExpanded ? ArrowUp : ArrowDown" @click="tlsNoticeExpanded = !tlsNoticeExpanded">
                {{ tlsNoticeExpanded ? t('cluster.config.collapse') : t('cluster.config.expand') }}
              </el-button>
            </div>
          </div>
          <div v-if="tlsNoticeExpanded" class="restart-guide">
            <p>{{ t('cluster.config.restartGuide') }}</p>
            <div class="path-copy-grid">
              <div class="path-copy-item">
                <span class="path-copy-label">{{ t('cluster.config.serverCert') }}</span>
                <code>{{ clusterControl?.apiTlsCertFile || '-' }}</code>
              </div>
              <div class="path-copy-item">
                <span class="path-copy-label">{{ t('cluster.config.clientCa') }}</span>
                <code>{{ clusterControl?.apiTlsClientCaFile || t('cluster.config.notConfigured') }}</code>
              </div>
            </div>
          </div>
        </template>
      </el-alert>
      <el-alert
        v-if="latestClusterToken"
        :title="t('cluster.config.clusterTokenTitle')"
        type="success"
        :closable="true"
        show-icon
        class="control-alert"
        @close="latestClusterToken = ''"
      >
        <template #default>
          <div class="token-row">
            <code>{{ latestClusterToken }}</code>
          </div>
        </template>
      </el-alert>
      <el-alert
        v-if="latestNodeToken"
        :title="t('cluster.config.nodeTokenTitle', { nodeId: latestJoinedNodeId || '' })"
        type="success"
        :closable="true"
        show-icon
        class="control-alert"
        @close="closeLatestNodeToken"
      >
        <template #default>
          <div class="token-row">
            <code>{{ latestNodeToken }}</code>
          </div>
        </template>
      </el-alert>
      <el-alert
        v-if="latestJoinJobId"
        :title="t('cluster.config.joinJobTitle')"
        type="info"
        :closable="true"
        show-icon
        class="control-alert"
        @close="latestJoinJobId = ''"
      >
        <template #default>
          <div class="token-row">
            <code>{{ latestJoinJobId }}</code>
          </div>
        </template>
      </el-alert>

      <el-form
        v-if="!clusterControl?.initialized"
        :model="controlForm"
        label-width="140px"
        label-position="left"
        require-asterisk-position="left"
        class="page-form"
      >
        <div class="control-form-grid">
          <el-form-item :label="t('cluster.config.formClusterDomain')" required class="grid-span-1">
            <el-input v-model="controlForm.clusterDomain" :placeholder="t('cluster.config.formClusterDomainPh')" />
          </el-form-item>
          <el-form-item :label="t('cluster.config.formPrimaryNodeId')" class="grid-span-1">
            <el-input v-model="controlForm.primaryNodeId" :placeholder="t('cluster.config.formPrimaryNodeIdPh')" />
          </el-form-item>
          <el-form-item :label="t('cluster.config.formPrimaryUrl')" class="grid-span-1">
            <el-input v-model="controlForm.primaryNodeUrl" :placeholder="t('cluster.config.formPrimaryUrlPh')" />
          </el-form-item>
          <el-form-item :label="t('cluster.config.formPrimaryIpList')" required class="grid-span-1">
            <el-input
              v-model="controlForm.primaryNodeIpAddressesText"
              type="textarea"
              :rows="3"
              :placeholder="t('cluster.config.formPrimaryIpListPh')"
            />
          </el-form-item>
          <el-form-item :label="t('cluster.config.formHeartbeatInterval')" class="grid-span-1">
            <el-input-number v-model="controlForm.heartbeatIntervalSeconds" :min="1" :max="300" :controls="false" />
          </el-form-item>
          <el-form-item :label="t('cluster.config.formHeartbeatRetry')" class="grid-span-1">
            <el-input-number v-model="controlForm.heartbeatRetryIntervalSeconds" :min="1" :max="300" :controls="false" />
          </el-form-item>
          <el-form-item :label="t('cluster.config.formConfigRefresh')" class="grid-span-1">
            <el-input-number v-model="controlForm.configRefreshIntervalSeconds" :min="1" :max="600" :controls="false" />
          </el-form-item>
          <el-form-item :label="t('cluster.config.formConfigRetry')" class="grid-span-1">
            <el-input-number v-model="controlForm.configRetryIntervalSeconds" :min="1" :max="300" :controls="false" />
          </el-form-item>
        </div>
      </el-form>

      <div v-else class="control-detail-grid">
        <div class="control-info-grid">
          <div class="control-info-item">
            <span class="control-info-label">{{ t('cluster.config.infoClusterDomain') }}</span>
            <div class="control-info-value">
              <span>{{ clusterControl?.clusterDomain || '-' }}</span>
              <el-button text size="small" :icon="DocumentCopy" @click="copyText(clusterControl?.clusterDomain || '', t('cluster.config.domainCopied'))">{{ t('cluster.config.copy') }}</el-button>
            </div>
          </div>
          <div class="control-info-item">
            <span class="control-info-label">Primary URL</span>
            <div class="control-info-value">
              <span>{{ clusterControl?.primaryNodeUrl || '-' }}</span>
              <el-button text size="small" :icon="DocumentCopy" @click="copyText(clusterControl?.primaryNodeUrl || '', t('cluster.config.primaryUrlCopied'))">{{ t('cluster.config.copy') }}</el-button>
            </div>
          </div>
          <div class="control-info-item">
            <span class="control-info-label">Primary IP</span>
            <div class="control-info-value">
              <span>{{ (clusterControl?.primaryNodeIpAddresses || []).join(', ') || '-' }}</span>
              <el-button text size="small" :icon="DocumentCopy" @click="copyText((clusterControl?.primaryNodeIpAddresses || []).join(', '), t('cluster.config.primaryIpCopied'))">{{ t('cluster.config.copy') }}</el-button>
            </div>
          </div>
          <div class="control-info-item">
            <span class="control-info-label">{{ t('cluster.config.requireTls') }}</span>
            <div class="control-info-value">
              <span>{{ clusterControl?.requireTls ? t('cluster.config.requireTlsYes') : t('cluster.config.requireTlsNo') }}</span>
            </div>
          </div>
          <div class="control-info-item">
            <span class="control-info-label">{{ t('cluster.config.certFile') }}</span>
            <div class="control-info-value">
              <span>{{ clusterControl?.apiTlsCertFile || '-' }}</span>
              <el-button text size="small" :icon="DocumentCopy" @click="copyText(clusterControl?.apiTlsCertFile || '', t('cluster.config.certFileCopied'))">{{ t('cluster.config.copy') }}</el-button>
            </div>
          </div>
          <div class="control-info-item">
            <span class="control-info-label">{{ t('cluster.config.infoClientCa') }}</span>
            <div class="control-info-value">
              <span>{{ clusterControl?.apiTlsClientCaFile || '-' }}</span>
              <el-button text size="small" :icon="DocumentCopy" @click="copyText(clusterControl?.apiTlsClientCaFile || '', t('cluster.config.clientCaCopied'))">{{ t('cluster.config.copy') }}</el-button>
            </div>
          </div>
          <div class="control-info-item">
            <span class="control-info-label">{{ t('cluster.config.heartbeatRetryLabel') }}</span>
            <div class="control-info-value">
              <span>{{ clusterControl?.heartbeatIntervalSeconds || 0 }}s / {{ clusterControl?.heartbeatRetryIntervalSeconds || 0 }}s</span>
              <el-button text size="small" :icon="DocumentCopy" @click="copyText(`${clusterControl?.heartbeatIntervalSeconds || 0}s / ${clusterControl?.heartbeatRetryIntervalSeconds || 0}s`, t('cluster.config.heartbeatRetryCopied'))">{{ t('cluster.config.copy') }}</el-button>
            </div>
          </div>
          <div class="control-info-item">
            <span class="control-info-label">{{ t('cluster.config.configRefreshRetryLabel') }}</span>
            <div class="control-info-value">
              <span>{{ clusterControl?.configRefreshIntervalSeconds || 0 }}s / {{ clusterControl?.configRetryIntervalSeconds || 0 }}s</span>
              <el-button text size="small" :icon="DocumentCopy" @click="copyText(`${clusterControl?.configRefreshIntervalSeconds || 0}s / ${clusterControl?.configRetryIntervalSeconds || 0}s`, t('cluster.config.configRefreshRetryCopied'))">{{ t('cluster.config.copy') }}</el-button>
            </div>
          </div>
        </div>

        <el-table :data="clusterControlNodes" border stripe>
          <el-table-column prop="id" :label="t('cluster.config.colNodeId')" min-width="140" />
          <el-table-column prop="type" :label="t('cluster.config.colNodeType')" width="110">
            <template #default="{ row }">{{ controlNodeTypeLabel(row.type) }}</template>
          </el-table-column>
          <el-table-column prop="state" :label="t('cluster.config.colNodeState')" width="120">
            <template #default="{ row }">
              <el-tag :type="controlNodeStateTag(row.state)">{{ controlNodeStateLabel(row.state) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('cluster.config.colAccessAddr')" min-width="220">
            <template #default="{ row }">{{ row.url || row.ipAddresses.join(', ') || '-' }}</template>
          </el-table-column>
          <el-table-column :label="t('cluster.config.colLastHeartbeat')" min-width="170">
            <template #default="{ row }">{{ formatTime(row.lastSeen) }}</template>
          </el-table-column>
          <el-table-column :label="t('cluster.config.colActions')" width="180" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" :icon="View" @click="openMemberDetail(row.id)">{{ t('cluster.config.detail') }}</el-button>
              <el-button
                v-if="row.type === 'secondary'"
                link
                type="danger"
                :icon="WarningFilled"
                :loading="memberLeaveLoading === row.id"
                @click="leaveMember(row)"
              >
                {{ t('cluster.config.safeEvict') }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>

      </div>
    </section>

    <section class="surface-card section-card">
      <div class="section-head">
        <div>
          <div class="section-title">{{ t('cluster.config.commandSection') }}</div>
          <div class="field-help">{{ t('cluster.config.commandHelp') }}</div>
        </div>
        <el-button size="small" class="action-button" :loading="commandLoading" @click="loadClusterCommands">{{ t('cluster.config.refreshCommands') }}</el-button>
      </div>

      <template v-if="clusterControl?.initialized">
        <div class="command-form-grid command-form-grid-tight">
          <el-form-item :label="t('cluster.config.commandType')" required class="command-form-item">
            <el-select v-model="commandForm.commandType" style="width: 100%">
              <el-option :label="t('cluster.config.commandVerify')" value="verify" />
              <el-option :label="t('cluster.config.commandSync')" value="sync" />
              <el-option :label="t('cluster.config.commandLeave')" value="leave" />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('cluster.config.targetMember')" required class="command-form-item">
            <el-select v-model="commandForm.targetNodeId" style="width: 100%" :placeholder="t('cluster.config.targetMemberPh')">
              <el-option
                v-for="candidate in commandTargetOptions"
                :key="candidate.id"
                :label="`${candidate.id} / ${candidate.url || candidate.ipAddresses.join(', ') || '-'}`"
                :value="candidate.id"
              />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('cluster.config.commandRemark')" class="command-form-item command-form-item-wide">
            <el-input v-model="commandForm.reason" :placeholder="t('cluster.config.commandRemarkPh')" />
          </el-form-item>
          <div class="command-submit-wrap command-submit-wrap-inline">
            <el-button type="primary" class="action-button" :icon="Promotion" :loading="commandSubmitting" @click="submitClusterCommand">{{ t('cluster.config.submitCommand') }}</el-button>
          </div>
        </div>

        <el-table :data="clusterCommands" border stripe class="cluster-command-table" :empty-text="t('cluster.config.commandEmpty')">
          <el-table-column prop="commandType" :label="t('cluster.config.colCommand')" width="110">
            <template #default="{ row }">{{ clusterCommandTypeLabel(row.commandType) }}</template>
          </el-table-column>
          <el-table-column prop="targetNodeId" :label="t('cluster.config.colTargetMember')" min-width="140" />
          <el-table-column prop="status" :label="t('cluster.config.colStatus')" width="120">
            <template #default="{ row }">
              <el-tag class="status-chip" :class="tagClassByType(clusterCommandStatusTag(row.status))">{{ clusterCommandStatusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('cluster.config.colReceipt')" min-width="420">
            <template #default="{ row }">
              <div class="command-receipt-list">
                <div v-for="receipt in row.receipts || []" :key="`${row.commandId}-${receipt.nodeId}`" class="command-receipt-item command-receipt-card">
                  <strong>{{ receipt.nodeId }}</strong>
                  <span>{{ clusterCommandStatusLabel(receipt.status) }}</span>
                  <span>{{ receipt.detail || receipt.error || '-' }}</span>
                </div>
              </div>
            </template>
          </el-table-column>
          <el-table-column :label="t('cluster.config.colUpdateTime')" min-width="170">
            <template #default="{ row }">{{ formatTime(row.updatedAt || row.requestedAt) }}</template>
          </el-table-column>
        </el-table>
      </template>
      <el-empty v-else :description="t('cluster.config.commandEmptyInit')" :image-size="52" />
    </section>

    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-width="120px"
      label-position="left"
      require-asterisk-position="left"
      class="page-form"
    >
      <section class="surface-card section-card">
        <div class="section-title">{{ t('cluster.config.protocolSection') }}</div>
        <div class="protocol-shell">
          <div class="protocol-subsection">
            <div class="subsection-title">{{ t('cluster.config.protocolSubsection') }}</div>
            <div class="config-grid-two">
              <el-form-item :label="t('cluster.config.clusterMode')" class="grid-span-1">
                <el-input :model-value="t('cluster.config.clusterModeValue')" disabled />
                <div class="field-help">{{ t('cluster.config.clusterModeHelp') }}</div>
              </el-form-item>

              <el-form-item :label="t('cluster.config.commPort')" prop="commPort" required class="grid-span-1">
                <el-input-number v-model="form.commPort" :min="1" :max="65535" :controls="false" />
                <el-tooltip :content="t('cluster.config.commPortTip')" placement="top">
                  <el-tag size="small" class="tip-tag">{{ t('cluster.config.commPortTipLabel') }}</el-tag>
                </el-tooltip>
                <div class="field-help">{{ t('cluster.config.commPortHelp') }}</div>
              </el-form-item>

              <el-form-item :label="t('cluster.config.switchMode')" prop="switchMode" required class="grid-span-2 switch-mode-item">
                <el-radio-group v-model="form.switchMode" class="switch-mode-group">
                  <el-radio-button value="auto">{{ t('cluster.config.switchAuto') }}</el-radio-button>
                  <el-radio-button value="manual">{{ t('cluster.config.switchManual') }}</el-radio-button>
                </el-radio-group>
                <div class="field-help switch-mode-help">{{ t('cluster.config.switchModeHelp') }}</div>
              </el-form-item>
            </div>
          </div>

          <div class="protocol-subsection">
            <div class="subsection-title">{{ t('cluster.config.heartbeatSection') }}</div>
            <div class="strategy-form config-grid-two">
              <el-form-item prop="heartbeatSec" required class="compact-item grid-span-1">
                <template #label>
                  <span class="nowrap-label">{{ t('cluster.config.heartbeatInterval') }}</span>
                </template>
                <div class="form-control-group">
                  <el-input-number v-model="form.heartbeatSec" :min="1" :max="60" :controls="false" class="number-field" />
                  <el-tooltip :content="t('cluster.config.heartbeatIntervalTip')" placement="top">
                    <el-tag size="small" class="tip-tag">{{ t('cluster.config.commPortTipLabel') }}</el-tag>
                  </el-tooltip>
                </div>
              </el-form-item>

              <el-form-item prop="heartbeatFailThreshold" required class="compact-item grid-span-1">
                <template #label>
                  <span class="nowrap-label">{{ t('cluster.config.heartbeatFailThreshold') }}</span>
                </template>
                <div class="form-control-group">
                  <el-input-number v-model="form.heartbeatFailThreshold" :min="1" :max="10" :controls="false" class="number-field" />
                  <el-tooltip :content="t('cluster.config.heartbeatFailThresholdTip')" placement="top">
                    <el-tag size="small" class="tip-tag">{{ t('cluster.config.commPortTipLabel') }}</el-tag>
                  </el-tooltip>
                </div>
              </el-form-item>

              <el-form-item :label="t('cluster.config.splitBrain')" prop="splitBrain" required class="grid-span-1">
                <div class="form-control-group">
                  <el-select v-model="form.splitBrain" class="select-field">
                    <el-option :label="t('cluster.config.splitBrainPriorityLock')" value="priority-lock" />
                    <el-option :label="t('cluster.config.splitBrainArbiter')" value="arbiter" />
                  </el-select>
                  <el-tooltip :content="t('cluster.config.splitBrainTip')" placement="top">
                    <el-tag size="small" class="tip-tag">{{ t('cluster.config.commPortTipLabel') }}</el-tag>
                  </el-tooltip>
                </div>
              </el-form-item>

              <el-form-item :label="t('cluster.config.autoFailback')" prop="autoFailback" class="grid-span-1">
                <div class="form-control-group">
                  <el-switch v-model="form.autoFailback" />
                  <el-tooltip :content="t('cluster.config.autoFailbackTip')" placement="top">
                    <el-tag size="small" class="tip-tag">{{ t('cluster.config.commPortTipLabel') }}</el-tag>
                  </el-tooltip>
                </div>
              </el-form-item>

              <el-form-item v-if="form.autoFailback" :label="t('cluster.config.failbackWait')" prop="failbackWaitMin" required class="grid-span-1">
                <div class="form-control-group">
                  <el-input-number v-model="form.failbackWaitMin" :min="1" :max="1440" :controls="false" class="number-field" />
                  <el-tooltip :content="t('cluster.config.failbackWaitTip')" placement="top">
                    <el-tag size="small" class="tip-tag">{{ t('cluster.config.commPortTipLabel') }}</el-tag>
                  </el-tooltip>
                </div>
              </el-form-item>
            </div>
          </div>
        </div>
      </section>
    </el-form>

    <section class="surface-card section-card">
      <div class="section-head">
        <span class="section-title">{{ t('cluster.config.nodeListSection') }}</span>
        <el-tooltip :content="writeGuardReason" placement="top" :disabled="!writeGuardBlocked">
          <span class="guard-disabled-wrap">
            <el-button type="primary" :loading="nodeLoading" :disabled="writeGuardBlocked" @click="openCreate">{{ t('cluster.config.addNode') }}</el-button>
          </span>
        </el-tooltip>
      </div>

      <el-empty v-if="!nodeRows.length" :description="t('cluster.config.nodeListEmpty')" :image-size="52" />
      <el-table v-else :data="nodeRows" border stripe>
        <el-table-column prop="address" :label="t('cluster.config.colNodeIp')" min-width="150" />
        <el-table-column :label="t('cluster.config.colNodeRole')" width="110">
          <template #default="{ row }">{{ roleLabel(row.role) }}</template>
        </el-table-column>
        <el-table-column prop="priority" :label="t('cluster.config.colPriority')" width="90" />
        <el-table-column :label="t('cluster.config.colAuthStatus')" width="110">
          <template #default="{ row }">
            <el-tooltip :content="t('cluster.config.authTip')" placement="top">
              <el-tag class="status-chip" :class="row.hasAuth ? 'status-success' : 'status-muted'">{{ row.hasAuth ? t('cluster.config.authYes') : t('cluster.config.authNo') }}</el-tag>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column :label="t('cluster.config.colRunStatus')" width="100">
          <template #default="{ row }"><el-tag class="status-chip" :class="tagClassByType(healthTag(row.health))">{{ healthLabel(row.health) }}</el-tag></template>
        </el-table-column>
        <el-table-column :label="t('cluster.config.colHeartbeatStatus')" width="100">
          <template #default="{ row }"><el-tag class="status-chip" :class="tagClassByType(heartbeatTag(row.lastHeartbeat))">{{ heartbeatLabel(row.lastHeartbeat) }}</el-tag></template>
        </el-table-column>
        <el-table-column :label="t('cluster.config.colActions')" min-width="280" fixed="right">
          <template #default="{ row }">
            <el-tooltip :content="writeGuardReason" placement="top" :disabled="!writeGuardBlocked">
              <span class="guard-disabled-wrap node-action-wrap">
                <el-button size="small" class="node-action-btn primary-action" :icon="Edit" :disabled="writeGuardBlocked || !isManageableClusterNode(row)" @click="openEdit(row)">{{ t('cluster.config.editBtn') }}</el-button>
              </span>
            </el-tooltip>
            <el-tooltip :content="writeGuardReason" placement="top" :disabled="!writeGuardBlocked">
              <span class="guard-disabled-wrap node-action-wrap">
                <el-button size="small" class="node-action-btn primary-action" :icon="Key" :disabled="writeGuardBlocked || !isManageableClusterNode(row)" @click="openAuthEdit(row)">{{ t('cluster.config.authBtn') }}</el-button>
              </span>
            </el-tooltip>
            <el-tooltip :content="writeGuardReason" placement="top" :disabled="!writeGuardBlocked">
              <span class="guard-disabled-wrap node-action-wrap">
                <el-button size="small" class="node-action-btn warning-action" :icon="WarningFilled" :loading="rowActionLoading === row.id" :disabled="writeGuardBlocked || !isManageableClusterNode(row)" @click="toggleNode(row)">
                  {{ row.disabled ? t('cluster.config.enableBtn') : t('cluster.config.disableBtn') }}
                </el-button>
              </span>
            </el-tooltip>
            <el-tooltip :content="writeGuardReason" placement="top" :disabled="!writeGuardBlocked">
              <span class="guard-disabled-wrap node-action-wrap">
                <el-button size="small" class="node-action-btn danger-action" :icon="Delete" :loading="rowActionLoading === row.id" :disabled="writeGuardBlocked || isSelfClusterNode(row) || !isManageableClusterNode(row)" @click="removeNode(row)">
                  {{ t('cluster.config.deleteBtn') }}
                </el-button>
              </span>
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <section class="surface-card section-card">
      <div class="section-head">
        <span class="section-title">{{ t('cluster.config.membershipSection') }}</span>
        <el-button size="small" class="action-button" :loading="loading" @click="loadData">{{ t('cluster.config.refreshCommands') }}</el-button>
      </div>
      <el-empty v-if="!membershipEvents.length" :description="t('cluster.config.membershipEmpty')" :image-size="52" />
      <template v-else>
        <el-table :data="pagedMembershipEvents" border stripe>
          <el-table-column prop="time" :label="t('cluster.config.colTime')" width="170" />
          <el-table-column prop="title" :label="t('cluster.config.colEvent')" width="150" />
          <el-table-column prop="detail" :label="t('cluster.config.colDetail')" min-width="260" show-overflow-tooltip />
          <el-table-column :label="t('cluster.config.colStatus')" width="110">
            <template #default="{ row }">
              <el-tag class="status-chip" :class="row.status === 'failed' ? 'status-danger' : row.status === 'running' ? 'status-warning' : 'status-success'">
                {{ row.statusLabel || (row.status === 'failed' ? t('cluster.config.memberStatusFailed') : row.status === 'running' ? t('cluster.config.memberStatusRunning') : t('cluster.config.memberStatusDone')) }}
              </el-tag>
            </template>
          </el-table-column>
        </el-table>
        <div class="table-pagination-wrap">
          <el-pagination
            small
            background
            layout="prev, pager, next"
            :current-page="membershipPage"
            :page-size="membershipPageSize"
            :total="membershipEvents.length"
            @current-change="membershipPage = $event"
          />
        </div>
      </template>
    </section>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="600px" destroy-on-close>
      <el-form ref="nodeFormRef" :model="nodeForm" :rules="nodeRules" label-width="120px" label-position="left" require-asterisk-position="left">
        <el-form-item :label="t('cluster.config.formNodeIp')" prop="address" required>
          <el-input v-model="nodeForm.address" :disabled="authOnlyMode" :placeholder="t('cluster.config.formNodeIpPh')" />
          <div class="field-help">{{ t('cluster.config.formNodeIpHelp') }}</div>
        </el-form-item>

        <el-form-item :label="t('cluster.config.formNodeRole')" prop="role" required>
          <el-select v-model="nodeForm.role" :disabled="authOnlyMode" style="width: 220px">
            <el-option :label="t('cluster.config.formNodeRoleActive')" value="active" />
            <el-option :label="t('cluster.config.formNodeRoleStandby')" value="standby" />
          </el-select>
        </el-form-item>

        <el-form-item :label="t('cluster.config.formNodePriority')" prop="priority" required>
          <el-input-number v-model="nodeForm.priority" :disabled="authOnlyMode" :min="1" :max="100" :controls="false" />
        </el-form-item>

        <el-form-item :label="t('cluster.config.formNodeAuth')" prop="auth" required>
          <el-input v-model="nodeForm.auth" type="password" show-password :placeholder="editing ? t('cluster.config.formNodeAuthEditPh') : t('cluster.config.formNodeAuthPh')" />
          <div class="field-help">{{ t('cluster.config.formNodeAuthHelp') }}</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ t('cluster.config.cancel') }}</el-button>
        <el-tooltip :content="writeGuardReason" placement="top" :disabled="!writeGuardBlocked">
          <span class="guard-disabled-wrap">
            <el-button type="primary" :disabled="writeGuardBlocked" :loading="nodeSaving" @click="submitNode">{{ t('cluster.config.submit') }}</el-button>
          </span>
        </el-tooltip>
      </template>
    </el-dialog>

    <el-dialog
      v-model="joinDialogVisible"
      :title="t('cluster.config.joinDialogTitle')"
      width="600px"
      class="cluster-join-dialog"
      destroy-on-close
      align-center
      :close-on-click-modal="true"
      :close-on-press-escape="true"
      @closed="handleJoinDialogClosed"
    >
      <el-form
        ref="joinFormRef"
        :model="joinForm"
        :rules="joinRules"
        label-width="128px"
        label-position="left"
        require-asterisk-position="left"
        status-icon
        class="join-dialog-form"
      >
        <section class="join-form-section">
          <div class="join-form-section-title">{{ t('cluster.config.joinSectionBasic') }}</div>
          <div class="join-form-grid">
            <el-form-item :label="t('cluster.config.joinCandidate')" prop="selectedNodeId" class="grid-span-2 join-candidate-item">
              <el-select v-model="joinForm.selectedNodeId" clearable filterable :placeholder="t('cluster.config.joinCandidatePh')" style="width: 100%" @change="applyJoinCandidate">
                <el-option
                  v-for="candidate in joinCandidates"
                  :key="candidate.id"
                  :label="`${candidate.address} (${roleLabel(candidate.role)})`"
                  :value="candidate.id"
                />
              </el-select>
              <div class="field-help join-field-help">{{ t('cluster.config.joinCandidateHelp') }}</div>
              <el-alert v-if="!joinCandidates.length" type="info" :closable="false" show-icon :title="t('cluster.config.joinCandidateEmpty')" class="join-inline-alert" />
            </el-form-item>

            <el-form-item :label="t('cluster.config.joinSecondaryId')" prop="secondaryNodeId" required class="grid-span-1 join-required-item">
              <el-input v-model="joinForm.secondaryNodeId" clearable :placeholder="t('cluster.config.joinSecondaryIdPh')" />
            </el-form-item>

            <el-form-item :label="t('cluster.config.joinSecondaryName')" prop="secondaryNodeName" class="grid-span-1">
              <el-input v-model="joinForm.secondaryNodeName" clearable :placeholder="t('cluster.config.joinSecondaryNamePh')" />
            </el-form-item>

            <el-form-item :label="t('cluster.config.joinSecondaryUrl')" prop="secondaryNodeUrl" required class="grid-span-1 join-required-item">
              <el-input v-model="joinForm.secondaryNodeUrl" clearable :placeholder="t('cluster.config.joinSecondaryUrlPh')" />
            </el-form-item>

            <el-form-item :label="t('cluster.config.joinSecondaryIpList')" prop="secondaryNodeIpAddressesText" required class="grid-span-1 join-required-item">
              <el-input v-model="joinForm.secondaryNodeIpAddressesText" clearable type="textarea" :rows="3" :placeholder="t('cluster.config.joinSecondaryIpListPh')" />
              <div class="field-help join-field-help">{{ t('cluster.config.joinSecondaryIpListHelp') }}</div>
            </el-form-item>
          </div>
        </section>

        <section class="join-form-section">
          <div class="join-form-section-title">{{ t('cluster.config.joinSectionAuth') }}</div>
          <div class="join-form-grid">
            <el-form-item label="Cluster Token" prop="clusterToken" required class="grid-span-1 join-required-item">
              <el-input v-model="joinForm.clusterToken" clearable type="password" show-password :placeholder="t('cluster.config.joinClusterTokenPh')" />
            </el-form-item>

            <el-form-item :label="t('cluster.config.joinCertLabel')" prop="secondaryNodeCertificate" class="grid-span-1">
              <div class="certificate-field-shell">
                <el-radio-group v-model="joinCertificateMode" class="certificate-mode-switch">
                  <el-radio-button value="paste">{{ t('cluster.config.joinCertPaste') }}</el-radio-button>
                  <el-radio-button value="upload">{{ t('cluster.config.joinCertUpload') }}</el-radio-button>
                </el-radio-group>

                <template v-if="joinCertificateMode === 'paste'">
                  <el-input
                    v-model="joinForm.secondaryNodeCertificate"
                    clearable
                    type="textarea"
                    :rows="4"
                    :placeholder="t('cluster.config.joinCertPastePh')"
                  />
                </template>

                <template v-else>
                  <div class="certificate-upload-panel">
                    <input ref="joinCertificateInputRef" type="file" accept=".pem,.crt,.cer,.txt" class="certificate-file-input" @change="handleJoinCertificateFileChange" />
                    <div class="certificate-upload-actions">
                      <el-button class="action-button" native-type="button" @click="triggerJoinCertificateUpload">{{ t('cluster.config.joinCertSelectFile') }}</el-button>
                      <span v-if="joinCertificateFileName" class="certificate-file-name">{{ joinCertificateFileName }}</span>
                      <el-button v-if="joinForm.secondaryNodeCertificate" text class="danger-text-button" native-type="button" @click="clearJoinCertificateFile">{{ t('cluster.config.joinCertClearFile') }}</el-button>
                    </div>
                    <div class="field-help join-field-help">{{ t('cluster.config.joinCertUploadHelp') }}</div>
                    <el-input v-model="joinForm.secondaryNodeCertificate" type="textarea" :rows="4" readonly :placeholder="t('cluster.config.joinCertUploadPh')" />
                  </div>
                </template>
              </div>
            </el-form-item>
          </div>
        </section>
      </el-form>
      <template #footer>
        <div class="join-dialog-footer">
          <el-button class="action-button" @click="joinDialogVisible = false">{{ t('cluster.config.cancel') }}</el-button>
          <el-button type="primary" class="action-button" :loading="joinSubmitting" :disabled="joinSubmitting" @click="submitJoin">{{ t('cluster.config.joinSubmit') }}</el-button>
        </div>
      </template>
    </el-dialog>

    <el-drawer v-model="memberDetailVisible" :title="t('cluster.config.memberDrawerTitle')" size="560px" destroy-on-close>
      <div v-loading="memberDetailLoading" class="member-detail-grid">
        <template v-if="memberDetail">
          <el-descriptions :column="1" border>
            <el-descriptions-item :label="t('cluster.config.memberIdLabel')">{{ memberDetail.node.id }}</el-descriptions-item>
            <el-descriptions-item :label="t('cluster.config.memberTypeLabel')">{{ controlNodeTypeLabel(memberDetail.node.type) }}</el-descriptions-item>
            <el-descriptions-item :label="t('cluster.config.memberStateLabel')">
              <el-tag :type="controlNodeStateTag(memberDetail.node.state)">{{ controlNodeStateLabel(memberDetail.node.state) }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item :label="t('cluster.config.memberAddrLabel')">{{ memberDetail.node.url || memberDetail.node.ipAddresses.join(', ') || '-' }}</el-descriptions-item>
            <el-descriptions-item :label="t('cluster.config.memberJoinedAt')">{{ formatTime(memberDetail.node.joinedAt) }}</el-descriptions-item>
            <el-descriptions-item :label="t('cluster.config.memberLastSeen')">{{ formatTime(memberDetail.node.lastSeen) }}</el-descriptions-item>
          </el-descriptions>

          <div class="member-detail-card" v-if="memberDetail.runtimeNode">
            <div class="section-title">{{ t('cluster.config.memberRuntimeSection') }}</div>
            <el-descriptions :column="1" border>
              <el-descriptions-item :label="t('cluster.config.memberRoleLabel')">{{ roleLabel(memberDetail.runtimeNode.role) }}</el-descriptions-item>
              <el-descriptions-item :label="t('cluster.config.memberHealthLabel')">{{ healthLabel(memberDetail.runtimeNode.health) }}</el-descriptions-item>
              <el-descriptions-item :label="t('cluster.config.memberAddrLabel2')">{{ memberDetail.runtimeNode.address || '-' }}</el-descriptions-item>
              <el-descriptions-item :label="t('cluster.config.memberSyncLag')">{{ memberDetail.runtimeNode.syncLagMs ?? 0 }} ms</el-descriptions-item>
              <el-descriptions-item :label="t('cluster.config.memberLastHeartbeat')">{{ formatTime(memberDetail.runtimeNode.lastHeartbeat) }}</el-descriptions-item>
            </el-descriptions>
          </div>

          <div class="member-detail-card" v-if="memberDetail.latestJoinJob">
            <div class="section-title">{{ t('cluster.config.memberLatestJoinJob') }}</div>
            <el-descriptions :column="1" border>
              <el-descriptions-item :label="t('cluster.config.memberJobId')">{{ memberDetail.latestJoinJob.jobId }}</el-descriptions-item>
              <el-descriptions-item :label="t('cluster.config.memberJobStatus')">{{ memberDetail.latestJoinJob.status }}</el-descriptions-item>
              <el-descriptions-item :label="t('cluster.config.memberJobProgress')">{{ memberDetail.latestJoinJob.progress }}%</el-descriptions-item>
              <el-descriptions-item :label="t('cluster.config.memberJobUpdated')">{{ formatTime(memberDetail.latestJoinJob.updatedAt) }}</el-descriptions-item>
            </el-descriptions>
          </div>

          <div class="member-detail-card">
            <div class="section-title">{{ t('cluster.config.memberRecentCommands') }}</div>
            <el-table :data="memberDetail.recentCommands || []" border stripe>
              <el-table-column :label="t('cluster.config.memberCmdCol')" width="110">
                <template #default="{ row }">{{ clusterCommandTypeLabel(row.commandType) }}</template>
              </el-table-column>
              <el-table-column :label="t('cluster.config.memberCmdStatusCol')" width="120">
                <template #default="{ row }">
                  <el-tag :type="clusterCommandStatusTag(row.status)">{{ clusterCommandStatusLabel(row.status) }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column :label="t('cluster.config.memberCmdReceiptCol')" min-width="220">
                <template #default="{ row }">
                  <div class="command-receipt-list">
                    <div v-for="receipt in row.receipts || []" :key="`${row.commandId}-${receipt.nodeId}`" class="command-receipt-item">
                      <strong>{{ receipt.nodeId }}</strong>
                      <span>{{ clusterCommandStatusLabel(receipt.status) }}</span>
                      <span>{{ receipt.detail || receipt.error || '-' }}</span>
                    </div>
                  </div>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </template>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { FormInstance, FormRules } from 'element-plus';
import { ElMessageBox } from 'element-plus';
import { ArrowDown, ArrowUp, Connection, Delete, DocumentCopy, Edit, Key, Plus, Promotion, Refresh, View, WarningFilled } from '@element-plus/icons-vue';
import {
  createClusterCommand,
  createClusterNode,
  deleteClusterControl,
  getHAMembershipEvents,
  getClusterCommands,
  getClusterControl,
  getClusterHaConfig,
  getClusterMemberDetail,
  getClusterOverview,
  initializeClusterControl,
  joinClusterControl,
  leaveClusterMember,
  removeHAMember,
  saveClusterHaConfig,
  updateClusterNode
} from '@/api/cluster';
import type {
  ClusterCommand,
  ClusterControl,
  ClusterControlNode,
  ClusterFailoverEvent,
  ClusterHaConfig,
  ClusterInitializeResponse,
  ClusterJoinResponse,
  ClusterMemberDetail,
  ClusterNode
} from '@/types/cluster';
import { showHttpError } from '@/shared/errors/errorToast';
import { showSuccess, showWarning } from '@/shared/errors/messageToast';
import { isIPv4, isIPv6 } from '@/utils/ip';

const { t } = useI18n();

interface NodeView extends ClusterNode {
  priority: number;
  disabled?: boolean;
}

const loading = ref(false);
const controlLoading = ref(false);
const controlSubmitting = ref(false);
const controlDeleting = ref(false);
const commandLoading = ref(false);
const commandSubmitting = ref(false);
const joinDialogVisible = ref(false);
const joinSubmitting = ref(false);
const joinCertificateMode = ref<'paste' | 'upload'>('paste');
const nodeLoading = ref(false);
const rowActionLoading = ref('');
const memberLeaveLoading = ref('');
const saving = ref(false);
const checking = ref(false);
const prechecked = ref(false);

const formRef = ref<FormInstance>();
const nodeFormRef = ref<FormInstance>();
const joinFormRef = ref<FormInstance>();

const dialogVisible = ref(false);
const editing = ref(false);
const authOnlyMode = ref(false);
const nodeSaving = ref(false);
const currentEditId = ref('');
const dialogTitle = computed(() => {
  if (!editing.value) return t('cluster.config.dialogAddNode');
  if (authOnlyMode.value) return t('cluster.config.dialogEditAuth');
  return t('cluster.config.dialogEditNode');
});

const form = reactive({
  commPort: 647,
  switchMode: 'auto',
  heartbeatSec: 5,
  heartbeatFailThreshold: 3,
  splitBrain: 'priority-lock',
  autoFailback: false,
  failbackWaitMin: 30
});

const nodeRows = ref<NodeView[]>([]);
const membershipEvents = ref<ClusterFailoverEvent[]>([]);
const membershipPage = ref(1);
const membershipPageSize = 6;
const clusterRole = ref('unknown');
const clusterHealth = ref('unknown');
const clusterWriteGateOpen = ref<boolean | undefined>(undefined);
const clusterControl = ref<ClusterControl | null>(null);
const clusterCommands = ref<ClusterCommand[]>([]);
const memberDetailVisible = ref(false);
const memberDetailLoading = ref(false);
const memberDetail = ref<ClusterMemberDetail | null>(null);
const tlsNoticeExpanded = ref(false);
const latestClusterToken = ref('');
const latestNodeToken = ref('');
const latestJoinedNodeId = ref('');
const latestJoinJobId = ref('');
const joinCertificateInputRef = ref<HTMLInputElement>();
const joinCertificateFileName = ref('');

const controlForm = reactive({
  clusterDomain: '',
  primaryNodeId: '',
  primaryNodeUrl: '',
  primaryNodeIpAddressesText: '',
  heartbeatIntervalSeconds: 5,
  heartbeatRetryIntervalSeconds: 15,
  configRefreshIntervalSeconds: 30,
  configRetryIntervalSeconds: 10
});

const joinForm = reactive({
  selectedNodeId: '',
  secondaryNodeId: '',
  secondaryNodeName: '',
  secondaryNodeUrl: '',
  secondaryNodeIpAddressesText: '',
  clusterToken: '',
  secondaryNodeCertificate: ''
});

const validateJoinUrl = (_rule: unknown, value: unknown, callback: (error?: Error) => void) => {
  const url = String(value || '').trim();
  if (!url) {
    callback(new Error(t('cluster.config.validateUrlEmpty')));
    return;
  }
  if (!/^https:\/\//i.test(url)) {
    callback(new Error(t('cluster.config.validateUrlHttps')));
    return;
  }
  try {
    const parsed = new URL(url);
    if (!parsed.hostname) {
      callback(new Error(t('cluster.config.validateUrlInvalid')));
      return;
    }
  } catch {
    callback(new Error(t('cluster.config.validateUrlInvalid')));
    return;
  }
  callback();
};

const validateJoinIpList = (_rule: unknown, value: unknown, callback: (error?: Error) => void) => {
  const raw = String(value || '').trim();
  if (!raw) {
    callback(new Error(t('cluster.config.validateIpEmpty')));
    return;
  }
  const values = parseIpList(raw);
  if (!values.length) {
    callback(new Error(t('cluster.config.validateIpAtLeast')));
    return;
  }
  const invalid = values.find((item) => !isIPv4(item) && !isIPv6(item));
  if (invalid) {
    callback(new Error(t('cluster.config.validateIpInvalid', { ip: invalid })));
    return;
  }
  callback();
};

const joinRules: FormRules = {
  secondaryNodeId: [{ required: true, message: () => t('cluster.config.validateSecondaryIdReq'), trigger: ['blur', 'change'] }],
  secondaryNodeUrl: [{ validator: validateJoinUrl, trigger: ['blur', 'change'] }],
  secondaryNodeIpAddressesText: [{ validator: validateJoinIpList, trigger: ['blur', 'change'] }],
  clusterToken: [{ required: true, message: () => t('cluster.config.validateClusterTokenReq'), trigger: ['blur', 'change'] }]
};

const commandForm = reactive({
  commandType: 'verify',
  targetNodeId: '',
  reason: ''
});

const nodeForm = reactive({
  address: '',
  role: 'standby',
  priority: 10,
  auth: ''
});

const rules: FormRules = {
  commPort: [{ required: true, message: () => t('cluster.config.validateCommPort'), trigger: ['change', 'blur'] }],
  switchMode: [{ required: true, message: () => t('cluster.config.validateSwitchMode'), trigger: 'change' }],
  heartbeatSec: [
    { required: true, message: () => t('cluster.config.validateHeartbeatReq'), trigger: ['change', 'blur'] },
    {
      validator: (_rule: unknown, value: unknown, callback: (error?: Error) => void) => {
        const n = Number(value);
        if (!Number.isFinite(n) || n < 1 || n > 60) {
          callback(new Error(t('cluster.config.validateHeartbeatRange')));
          return;
        }
        callback();
      },
      trigger: ['change', 'blur']
    }
  ],
  heartbeatFailThreshold: [
    { required: true, message: () => t('cluster.config.validateFailThresholdReq'), trigger: ['change', 'blur'] },
    {
      validator: (_rule: unknown, value: unknown, callback: (error?: Error) => void) => {
        const n = Number(value);
        if (!Number.isFinite(n) || n < 1 || n > 10) {
          callback(new Error(t('cluster.config.validateFailThresholdRange')));
          return;
        }
        callback();
      },
      trigger: ['change', 'blur']
    }
  ],
  splitBrain: [{ required: true, message: () => t('cluster.config.validateSplitBrain'), trigger: 'change' }],
  failbackWaitMin: [
    {
      validator: (_rule: unknown, value: unknown, callback: (error?: Error) => void) => {
        if (!form.autoFailback) return callback();
        if (!value || Number(value) < 1) {
          callback(new Error(t('cluster.config.validateFailbackWait')));
          return;
        }
        callback();
      },
      trigger: ['change', 'blur']
    }
  ]
};

const nodeRules: FormRules = {
  address: [
    {
      validator: (_rule: unknown, value: unknown, callback: (error?: Error) => void) => {
        const ip = String(value || '').trim();
        if (!ip) return callback(new Error(t('cluster.config.validateNodeIpReq')));
        if (!isIPv4(ip) && !isIPv6(ip)) return callback(new Error(t('cluster.config.validateNodeIpInvalid')));
        const duplicated = nodeRows.value.some((n) => n.address === ip && n.id !== currentEditId.value);
        if (duplicated) return callback(new Error(t('cluster.config.validateNodeIpDup')));
        callback();
      },
      trigger: ['blur', 'change']
    }
  ],
  role: [{ required: true, message: () => t('cluster.config.validateNodeRole'), trigger: 'change' }],
  priority: [{ required: true, message: () => t('cluster.config.validateNodePriority'), trigger: 'change' }],
  auth: [
    {
      validator: (_rule: unknown, value: unknown, callback: (error?: Error) => void) => {
        if (editing.value && !authOnlyMode.value && !String(value || '').trim()) {
          callback();
          return;
        }
        if (!String(value || '').trim()) {
          callback(new Error(t('cluster.config.validateNodeAuth')));
          return;
        }
        callback();
      },
      trigger: ['change', 'blur']
    }
  ]
};

const unwrap = <T>(resp: any): T => (resp?.data?.data ?? resp?.data ?? resp) as T;

const clusterControlNodes = computed<ClusterControlNode[]>(() => clusterControl.value?.nodes || []);
const joinCandidates = computed(() => nodeRows.value.filter((item) => item.role !== 'active'));
const commandTargetOptions = computed(() => clusterControlNodes.value.filter((item) => item.type === 'secondary'));
const pagedMembershipEvents = computed(() => {
  const start = (membershipPage.value - 1) * membershipPageSize;
  return membershipEvents.value.slice(start, start + membershipPageSize);
});
const clusterControlTlsLabel = computed(() => {
  if (!clusterControl.value?.apiTlsEnabled) return t('cluster.config.tlsOff');
  if (clusterControl.value.apiTlsActive) return t('cluster.config.tlsActive');
  return t('cluster.config.tlsPendingRestart');
});

const writeGuardReason = computed(() => {
  const role = String(clusterRole.value || '').toLowerCase();
  const health = String(clusterHealth.value || '').toLowerCase();
  if (health.includes('draining')) {
    return t('cluster.config.writeGuardDraining');
  }
  if (clusterWriteGateOpen.value === true) {
    return '';
  }
  if (role !== 'primary' && role !== 'active') {
    return t('cluster.config.writeGuardNotPrimary');
  }
  return '';
});

const writeGuardBlocked = computed(() => writeGuardReason.value !== '');

const showWriteGuardReason = async () => {
  if (!writeGuardBlocked.value) return;
  await ElMessageBox.alert(writeGuardReason.value, t('cluster.config.writeGuardTitle'), {
    type: 'warning',
    confirmButtonText: t('cluster.config.writeGuardOk')
  });
};

const ensureWritable = async () => {
  if (!writeGuardBlocked.value) return true;
  await showWriteGuardReason();
  return false;
};

const parseIpList = (value: string) =>
  Array.from(
    new Set(
      String(value || '')
        .split(/[\s,;]+/)
        .map((item) => item.trim())
        .filter(Boolean)
    )
  );

const resetJoinForm = () => {
  joinForm.selectedNodeId = '';
  joinForm.secondaryNodeId = '';
  joinForm.secondaryNodeName = '';
  joinForm.secondaryNodeUrl = '';
  joinForm.secondaryNodeIpAddressesText = '';
  joinForm.clusterToken = latestClusterToken.value || '';
  joinForm.secondaryNodeCertificate = '';
  joinCertificateMode.value = 'paste';
  joinCertificateFileName.value = '';
  if (joinCertificateInputRef.value) {
    joinCertificateInputRef.value.value = '';
  }
};

const applyJoinCandidate = (value?: string) => {
  const candidate = joinCandidates.value.find((item) => item.id === value);
  if (!candidate) return;
  joinForm.secondaryNodeId = candidate.id;
  joinForm.secondaryNodeName = candidate.id;
  joinForm.secondaryNodeIpAddressesText = candidate.address || '';
};

const openJoinDialog = async () => {
  if (!(await ensureWritable())) return;
  resetJoinForm();
  joinDialogVisible.value = true;
  await nextTick();
  joinFormRef.value?.clearValidate();
};

const handleJoinDialogClosed = () => {
  resetJoinForm();
  joinFormRef.value?.clearValidate();
};

const triggerJoinCertificateUpload = () => {
  joinCertificateInputRef.value?.click();
};

const clearJoinCertificateFile = () => {
  joinForm.secondaryNodeCertificate = '';
  joinCertificateFileName.value = '';
  if (joinCertificateInputRef.value) {
    joinCertificateInputRef.value.value = '';
  }
};

const handleJoinCertificateFileChange = async (event: Event) => {
  const input = event.target as HTMLInputElement | null;
  const file = input?.files?.[0];
  if (!file) return;
  try {
    const content = await file.text();
    joinForm.secondaryNodeCertificate = content;
    joinCertificateFileName.value = file.name;
    showSuccess(t('cluster.config.certFileLoaded', { name: file.name }));
  } catch {
    showWarning(t('cluster.config.certFileReadFail'));
  }
};

const closeLatestNodeToken = () => {
  latestNodeToken.value = '';
  latestJoinedNodeId.value = '';
  latestJoinJobId.value = '';
};

const applyClusterControl = (control: ClusterControl | null | undefined) => {
  clusterControl.value = control || null;
  if (!control?.initialized) {
    commandForm.targetNodeId = '';
    return;
  }
  if (!control?.initialized) return;
  controlForm.clusterDomain = control.clusterDomain || controlForm.clusterDomain;
  controlForm.primaryNodeId = control.primaryNodeId || controlForm.primaryNodeId;
  controlForm.primaryNodeUrl = control.primaryNodeUrl || controlForm.primaryNodeUrl;
  controlForm.primaryNodeIpAddressesText = (control.primaryNodeIpAddresses || []).join(', ');
  controlForm.heartbeatIntervalSeconds = control.heartbeatIntervalSeconds || controlForm.heartbeatIntervalSeconds;
  controlForm.heartbeatRetryIntervalSeconds =
    control.heartbeatRetryIntervalSeconds || controlForm.heartbeatRetryIntervalSeconds;
  controlForm.configRefreshIntervalSeconds =
    control.configRefreshIntervalSeconds || controlForm.configRefreshIntervalSeconds;
  controlForm.configRetryIntervalSeconds =
    control.configRetryIntervalSeconds || controlForm.configRetryIntervalSeconds;
  if (!commandTargetOptions.value.some((item) => item.id === commandForm.targetNodeId)) {
    commandForm.targetNodeId = commandTargetOptions.value[0]?.id || '';
  }
};

const loadClusterControl = async () => {
  controlLoading.value = true;
  try {
    const resp = await getClusterControl();
    applyClusterControl(unwrap<ClusterControl>(resp));
  } catch (err) {
    showHttpError(err, t('cluster.config.loadControlFail'));
  } finally {
    controlLoading.value = false;
  }
};

const loadClusterCommands = async () => {
  commandLoading.value = true;
  try {
    const resp = await getClusterCommands();
    const payload = unwrap<{ items?: ClusterCommand[] }>(resp) || {};
    clusterCommands.value = payload.items || [];
  } catch (err) {
    showHttpError(err, t('cluster.config.loadCommandsFail'));
  } finally {
    commandLoading.value = false;
  }
};

const loadData = async () => {
  loading.value = true;
  try {
    const [cfgResp, overviewResp, eventsResp, controlResp, commandsResp] = await Promise.all([
      getClusterHaConfig(),
      getClusterOverview(),
      getHAMembershipEvents(),
      getClusterControl(),
      getClusterCommands()
    ]);
    const cfg = unwrap<Partial<ClusterHaConfig>>(cfgResp) || {};
    const overview = unwrap<{ nodes?: ClusterNode[]; dhcpRole?: string; dhcpHealth?: string; writeGateOpen?: boolean }>(overviewResp) || {};
    const nodes = overview.nodes || [];
    const eventsPayload = unwrap<{ items?: ClusterFailoverEvent[] }>(eventsResp) || {};
    const control = unwrap<ClusterControl>(controlResp);
    const commandsPayload = unwrap<{ items?: ClusterCommand[] }>(commandsResp) || {};

    clusterRole.value = String(overview.dhcpRole || 'unknown');
    clusterHealth.value = String(overview.dhcpHealth || 'unknown');
    clusterWriteGateOpen.value = overview.writeGateOpen;

    form.heartbeatSec = Math.max(1, Math.round((cfg.failover?.intervalMs || 5000) / 1000));
    form.heartbeatFailThreshold = cfg.failover?.failureThreshold || 3;
    form.splitBrain = cfg.splitBrain?.strategy === 'arbiter' ? 'arbiter' : 'priority-lock';
    form.switchMode = cfg.recovery?.manualAction === 'manual' ? 'manual' : 'auto';
    form.autoFailback = cfg.recovery?.manualAction === 'auto-failback';

    nodeRows.value = nodes.filter((item) => item.managed || item.self || String(item.id || '').trim().toLowerCase() === 'self').map((item, idx) => ({
      ...item,
      priority: Math.max(1, 100 - idx * 10),
      hasAuth: !!item.hasAuth,
      disabled: !!item.disabled
    }));
    membershipEvents.value = (eventsPayload.items || []).map((item) => ({
      ...item,
      time: formatTime(item.time)
    }));
    membershipPage.value = 1;
    applyClusterControl(control);
    clusterCommands.value = commandsPayload.items || [];

    prechecked.value = false;
  } catch (err) {
    showHttpError(err, t('cluster.config.loadConfigFail'));
  } finally {
    loading.value = false;
  }
};

const initializeControlPlane = async () => {
  const clusterDomain = String(controlForm.clusterDomain || '').trim();
  const primaryNodeUrl = String(controlForm.primaryNodeUrl || '').trim();
  const primaryNodeIpAddresses = parseIpList(controlForm.primaryNodeIpAddressesText);

  if (!clusterDomain) {
    showWarning(t('cluster.config.clusterDomainEmpty'));
    return;
  }
  if (!primaryNodeIpAddresses.length) {
    showWarning(t('cluster.config.primaryIpRequired'));
    return;
  }
  if (primaryNodeUrl && !/^https:\/\//i.test(primaryNodeUrl)) {
    showWarning(t('cluster.config.primaryUrlHttps'));
    return;
  }

  controlSubmitting.value = true;
  try {
    const resp = await initializeClusterControl({
      clusterDomain,
      primaryNodeId: String(controlForm.primaryNodeId || '').trim() || undefined,
      primaryNodeUrl: primaryNodeUrl || undefined,
      primaryNodeIpAddresses,
      heartbeatIntervalSeconds: controlForm.heartbeatIntervalSeconds,
      heartbeatRetryIntervalSeconds: controlForm.heartbeatRetryIntervalSeconds,
      configRefreshIntervalSeconds: controlForm.configRefreshIntervalSeconds,
      configRetryIntervalSeconds: controlForm.configRetryIntervalSeconds
    });
    const payload = unwrap<ClusterInitializeResponse>(resp);
    latestClusterToken.value = payload.clusterToken || '';
    closeLatestNodeToken();
    tlsNoticeExpanded.value = true;
    applyClusterControl(payload.cluster);
    showSuccess(payload.restartRequired ? t('cluster.config.initSuccessTls') : t('cluster.config.initSuccess'));
    await loadData();
  } catch (err) {
    showHttpError(err, t('cluster.config.initFail'));
  } finally {
    controlSubmitting.value = false;
  }
};

const deleteControlPlane = async () => {
  await ElMessageBox.confirm(t('cluster.config.deleteConfirm'), t('cluster.config.deleteConfirmTitle'), {
    type: 'warning',
    confirmButtonText: t('cluster.config.deleteConfirmBtn'),
    cancelButtonText: t('cluster.config.cancel')
  });

  controlDeleting.value = true;
  try {
    const resp = await deleteClusterControl({ forceDelete: true });
    const payload = unwrap<{ deleted: boolean; cluster: ClusterControl }>(resp);
    latestClusterToken.value = '';
    closeLatestNodeToken();
    applyClusterControl(payload.cluster);
    showSuccess(payload.deleted ? t('cluster.config.deleteDeleted') : t('cluster.config.deleteNotDeleted'));
    await loadData();
  } catch (err) {
    showHttpError(err, t('cluster.config.deleteFail'));
  } finally {
    controlDeleting.value = false;
  }
};

const submitClusterCommand = async () => {
  if (!(await ensureWritable())) return;
  const commandType = String(commandForm.commandType || '').trim();
  const targetNodeId = String(commandForm.targetNodeId || '').trim();
  if (!clusterControl.value?.initialized) {
    showWarning(t('cluster.config.cmdInitFirst'));
    return;
  }
  if (!targetNodeId) {
    showWarning(t('cluster.config.cmdSelectTarget'));
    return;
  }
  await ElMessageBox.confirm(
    commandType === 'leave' ? t('cluster.config.cmdConfirmLeave', { id: targetNodeId }) : t('cluster.config.cmdConfirmOther', { id: targetNodeId, cmd: clusterCommandTypeLabel(commandType) }),
    t('cluster.config.cmdConfirmTitle'),
    {
      type: commandType === 'leave' ? 'warning' : 'info',
      confirmButtonText: t('cluster.config.cmdConfirmBtn'),
      cancelButtonText: t('cluster.config.cancel')
    }
  );
  commandSubmitting.value = true;
  try {
    const resp = await createClusterCommand({
      commandType,
      targetNodeId,
      payload: commandForm.reason ? { reason: commandForm.reason.trim() } : undefined
    });
    const payload = unwrap<ClusterCommand>(resp);
    showSuccess(t('cluster.config.cmdSuccess', { cmd: clusterCommandTypeLabel(payload.commandType), status: clusterCommandStatusLabel(payload.status) }));
    commandForm.reason = '';
    await loadData();
    if (memberDetailVisible.value && memberDetail.value?.node?.id === targetNodeId) {
      await openMemberDetail(targetNodeId);
    }
  } catch (err) {
    showHttpError(err, t('cluster.config.cmdFail'));
  } finally {
    commandSubmitting.value = false;
  }
};

const openMemberDetail = async (memberId: string) => {
  memberDetailVisible.value = true;
  memberDetailLoading.value = true;
  try {
    const resp = await getClusterMemberDetail(memberId);
    memberDetail.value = unwrap<ClusterMemberDetail>(resp);
  } catch (err) {
    memberDetail.value = null;
    showHttpError(err, t('cluster.config.memberDetailFail'));
  } finally {
    memberDetailLoading.value = false;
  }
};

const leaveMember = async (row: ClusterControlNode) => {
  if (!(await ensureWritable())) return;
  await ElMessageBox.confirm(t('cluster.config.leaveConfirm', { id: row.id }), t('cluster.config.leaveConfirmTitle'), {
    type: 'warning',
    confirmButtonText: t('cluster.config.leaveConfirmBtn'),
    cancelButtonText: t('cluster.config.cancel')
  });
  memberLeaveLoading.value = row.id;
  try {
    await leaveClusterMember(row.id, { reason: 'manual-evict' });
    showSuccess(t('cluster.config.leaveSuccess', { id: row.id }));
    if (memberDetailVisible.value && memberDetail.value?.node?.id === row.id) {
      memberDetailVisible.value = false;
      memberDetail.value = null;
    }
    await loadData();
  } catch (err) {
    showHttpError(err, t('cluster.config.leaveFail'));
  } finally {
    memberLeaveLoading.value = '';
  }
};

const submitJoin = async () => {
  if (!(await ensureWritable())) return;
  const valid = await joinFormRef.value?.validate().then(() => true).catch(() => false);
  if (!valid) return;

  const secondaryNodeId = String(joinForm.secondaryNodeId || '').trim();
  const secondaryNodeUrl = String(joinForm.secondaryNodeUrl || '').trim();
  const clusterToken = String(joinForm.clusterToken || '').trim();
  const secondaryNodeIpAddresses = parseIpList(joinForm.secondaryNodeIpAddressesText);

  if (!clusterControl.value?.initialized) {
    showWarning(t('cluster.config.joinInitFirst'));
    return;
  }
  if (!secondaryNodeId) {
    showWarning(t('cluster.config.joinIdEmpty'));
    return;
  }
  if (!secondaryNodeUrl || !/^https:\/\//i.test(secondaryNodeUrl)) {
    showWarning(t('cluster.config.joinUrlHttps'));
    return;
  }
  if (!secondaryNodeIpAddresses.length) {
    showWarning(t('cluster.config.joinIpRequired'));
    return;
  }
  if (!clusterToken) {
    showWarning(t('cluster.config.joinTokenEmpty'));
    return;
  }

  const confirmed = await ElMessageBox.confirm(
    t('cluster.config.joinConfirm', { id: secondaryNodeId }),
    t('cluster.config.joinConfirmTitle'),
    {
      type: 'warning',
      confirmButtonText: t('cluster.config.joinConfirmBtn'),
      cancelButtonText: t('cluster.config.cancel')
    }
  ).then(() => true).catch(() => false);
  if (!confirmed) return;

  joinSubmitting.value = true;
  try {
    const resp = await joinClusterControl({
      secondaryNodeId,
      secondaryNodeName: String(joinForm.secondaryNodeName || '').trim() || undefined,
      secondaryNodeUrl,
      secondaryNodeIpAddresses,
      secondaryNodeCertificate: String(joinForm.secondaryNodeCertificate || '').trim() || undefined,
      clusterToken
    });
    const payload = unwrap<ClusterJoinResponse>(resp);
    latestJoinedNodeId.value = payload.nodeId || secondaryNodeId;
    latestNodeToken.value = payload.nodeToken || '';
    latestJoinJobId.value = payload.joinJobId || '';
    applyClusterControl(payload.cluster);
    joinDialogVisible.value = false;
    showSuccess(t('cluster.config.joinSuccess', { id: payload.nodeId || secondaryNodeId }) + (payload.joinJobId ? t('cluster.config.joinSuccessJob', { jobId: payload.joinJobId }) : ''));
    await loadData();
  } catch (err) {
    showHttpError(err, t('cluster.config.joinFail'));
  } finally {
    joinSubmitting.value = false;
  }
};

const precheck = async () => {
  if (!(await ensureWritable())) return;
  checking.value = true;
  try {
    const valid = await formRef.value?.validate().then(() => true).catch(() => false);
    if (!valid) return;

    if (form.commPort < 1 || form.commPort > 65535) {
      showWarning(t('cluster.config.precheckPortRange'));
      return;
    }

    if (!nodeRows.value.length) {
      showWarning(t('cluster.config.precheckNodeRequired'));
      return;
    }

    const activeCount = nodeRows.value.filter((n) => n.role === 'active' && !n.disabled).length;
    if (activeCount !== 1) {
      showWarning(t('cluster.config.precheckSingleActive'));
      return;
    }

    if (form.switchMode === 'auto' && form.heartbeatFailThreshold < 2) {
      showWarning(t('cluster.config.precheckThreshold'));
      return;
    }

    await new Promise((resolve) => setTimeout(resolve, 300));
    prechecked.value = true;
    showSuccess(t('cluster.config.precheckPass'));
  } finally {
    checking.value = false;
  }
};

const saveConfig = async () => {
  if (!(await ensureWritable())) return;
  if (!prechecked.value) return;
  saving.value = true;
  try {
    const active = nodeRows.value.find((n) => n.role === 'active' && !n.disabled);
    const standby = nodeRows.value.filter((n) => n.role === 'standby' && !n.disabled).map((n) => n.id);

    await saveClusterHaConfig({
      mode: 'active-passive',
      primary: active?.id || '',
      standbyNodes: standby,
      loadBalancing: { enabled: false },
      failover: {
        intervalMs: form.heartbeatSec * 1000,
        timeoutMs: form.heartbeatSec * form.heartbeatFailThreshold * 1000,
        failureThreshold: form.heartbeatFailThreshold
      },
      replication: {
        mechanism: 'bndupd-bndack',
        snapshotIntervalSec: 30,
        lagAlertMs: 150,
        syncMode: 'strong'
      },
      splitBrain: {
        strategy: form.splitBrain === 'arbiter' ? 'arbiter' : 'priority-lock'
      },
      recovery: {
        manualAction: form.switchMode === 'manual' ? 'manual' : form.autoFailback ? 'auto-failback' : 'manual-ack'
      }
    });

    showSuccess(t('cluster.config.saveSuccess'));
    prechecked.value = false;
  } catch (err) {
    showHttpError(err, t('cluster.config.saveFail'));
  } finally {
    saving.value = false;
  }
};

const resetForm = () => {
  form.commPort = 647;
  form.switchMode = 'auto';
  form.heartbeatSec = 5;
  form.heartbeatFailThreshold = 3;
  form.splitBrain = 'priority-lock';
  form.autoFailback = false;
  form.failbackWaitMin = 30;
  prechecked.value = false;
};

const openCreate = () => {
  if (writeGuardBlocked.value) {
    showWriteGuardReason();
    return;
  }
  editing.value = false;
  authOnlyMode.value = false;
  currentEditId.value = '';
  nodeForm.address = '';
  nodeForm.role = 'standby';
  nodeForm.priority = 10;
  nodeForm.auth = '';
  dialogVisible.value = true;
};

const openEdit = (row: NodeView) => {
  if (!isManageableClusterNode(row)) {
    showWarning(t('cluster.config.nodeNotEditable'));
    return;
  }
  if (writeGuardBlocked.value) {
    showWriteGuardReason();
    return;
  }
  editing.value = true;
  authOnlyMode.value = false;
  currentEditId.value = row.id;
  nodeForm.address = row.address;
  nodeForm.role = row.role === 'active' ? 'active' : 'standby';
  nodeForm.priority = row.priority;
  nodeForm.auth = '';
  dialogVisible.value = true;
};

const openAuthEdit = (row: NodeView) => {
  if (!isManageableClusterNode(row)) {
    showWarning(t('cluster.config.nodeNotAuthEditable'));
    return;
  }
  openEdit(row);
  authOnlyMode.value = true;
};

const submitNode = async () => {
  if (!(await ensureWritable())) return;
  const valid = await nodeFormRef.value?.validate().then(() => true).catch(() => false);
  if (!valid) return;

  nodeSaving.value = true;
  try {
    let joinJobId = '';
    if (editing.value) {
      const resp = await updateClusterNode(currentEditId.value, {
        address: nodeForm.address,
        role: nodeForm.role,
        auth: nodeForm.auth
      });
      joinJobId = String(resp?.headers?.['x-join-job-id'] || resp?.headers?.['X-Join-Job-Id'] || '').trim();
      const target = nodeRows.value.find((n) => n.id === currentEditId.value);
      if (target) {
        target.address = nodeForm.address;
        target.role = nodeForm.role;
        target.priority = nodeForm.priority;
        target.hasAuth = target.hasAuth || !!nodeForm.auth;
      }
    } else {
      const id = nodeForm.address;
      const resp = await createClusterNode({ id, address: nodeForm.address, role: nodeForm.role, auth: nodeForm.auth });
      joinJobId = String(resp?.headers?.['x-join-job-id'] || resp?.headers?.['X-Join-Job-Id'] || '').trim();
      nodeRows.value.push({
        id,
        address: nodeForm.address,
        role: nodeForm.role,
        hasAuth: !!nodeForm.auth,
        version: '-',
        health: 'healthy',
        cpuPercent: 0,
        memoryPercent: 0,
        activeLeasesRedis: 0,
        totalLeasesMySQL: 0,
        syncLagMs: 0,
        lastHeartbeat: new Date().toISOString(),
        priority: nodeForm.priority,
        disabled: false
      });
    }
    const msg = editing.value ? t('cluster.config.nodeUpdatedMsg') : t('cluster.config.nodeCreatedMsg');
    showSuccess(joinJobId ? t('cluster.config.nodeUpdatedJob', { msg, jobId: joinJobId }) : msg);
    dialogVisible.value = false;
    await loadData();
    prechecked.value = false;
  } catch (err) {
    showHttpError(err, editing.value ? t('cluster.config.nodeUpdateFail') : t('cluster.config.nodeCreateFail'));
  } finally {
    nodeSaving.value = false;
  }
};

const toggleNode = async (row: NodeView) => {
  if (!isManageableClusterNode(row)) {
    showWarning(t('cluster.config.nodeNotToggleable'));
    return;
  }
  if (!(await ensureWritable())) return;
  const action = row.disabled ? t('cluster.config.enableBtn') : t('cluster.config.disableBtn');
  await ElMessageBox.confirm(t('cluster.config.toggleConfirm', { action, addr: row.address }), t('cluster.config.toggleConfirmTitle'), {
    type: 'warning',
    confirmButtonText: t('cluster.config.toggleConfirmBtn', { action }),
    cancelButtonText: t('cluster.config.cancel')
  });
  rowActionLoading.value = row.id;
  try {
    await updateClusterNode(row.id, { disabled: !row.disabled });
    row.disabled = !row.disabled;
    showSuccess(row.disabled ? t('cluster.config.toggleDisabled', { addr: row.address }) : t('cluster.config.toggleEnabled', { addr: row.address }));
    await loadData();
    prechecked.value = false;
  } catch (err) {
    showHttpError(err, t('cluster.config.toggleFail'));
  } finally {
    rowActionLoading.value = '';
  }
};

const removeNode = async (row: NodeView) => {
  if (!(await ensureWritable())) return;
  if (!isManageableClusterNode(row)) {
    showWarning(t('cluster.config.nodeNotDeletable'));
    return;
  }
  if (isSelfClusterNode(row)) {
    showWarning(t('cluster.config.selfNotDeletable'));
    return;
  }
  await ElMessageBox.confirm(t('cluster.config.deleteNodeConfirm', { addr: row.address }), t('cluster.config.deleteNodeTitle'), {
    type: 'warning',
    confirmButtonText: t('cluster.config.deleteNodeBtn'),
    cancelButtonText: t('cluster.config.cancel')
  });

  rowActionLoading.value = row.id;
  try {
    await removeHAMember(row.id || row.address);
    nodeRows.value = nodeRows.value.filter((n) => n.id !== row.id);
    showSuccess(t('cluster.config.nodeDeletedMsg'));
    await loadData();
    prechecked.value = false;
  } catch (err) {
    showHttpError(err, t('cluster.config.nodeDeleteFail'));
  } finally {
    rowActionLoading.value = '';
  }
};

const roleLabel = (role: string) => (role === 'active' ? t('cluster.config.roleActive') : role === 'standby' ? t('cluster.config.roleStandby') : t('cluster.config.roleWorker'));
const isManageableClusterNode = (row: NodeView) => !!row?.managed;
const isSelfClusterNode = (row: NodeView) => {
  const id = String(row?.id || '').trim().toLowerCase();
  const address = String(row?.address || '').trim().toLowerCase();
  return id === 'self' || address === 'self';
};
const controlNodeTypeLabel = (value: string) => (value === 'primary' ? t('cluster.config.controlTypePrimary') : value === 'secondary' ? t('cluster.config.controlTypeSecondary') : value || '-');
const controlNodeStateLabel = (value: string) => {
  if (value === 'self') return t('cluster.config.controlStateSelf');
  if (value === 'connected') return t('cluster.config.controlStateConnected');
  if (value === 'unreachable') return t('cluster.config.controlStateUnreachable');
  if (value === 'unknown') return t('cluster.config.controlStateUnknown');
  return value || '-';
};
const controlNodeStateTag = (value: string) => {
  if (value === 'self' || value === 'connected') return 'success';
  if (value === 'unreachable') return 'danger';
  return 'info';
};
const clusterCommandTypeLabel = (value: string) => {
  if (value === 'verify') return t('cluster.config.cmdTypeVerify');
  if (value === 'sync') return t('cluster.config.cmdTypeSync');
  if (value === 'leave') return t('cluster.config.cmdTypeLeave');
  return value || '-';
};
const clusterCommandStatusLabel = (value: string) => {
  if (value === 'QUEUED') return t('cluster.config.cmdStatusQueued');
  if (value === 'RUNNING') return t('cluster.config.cmdStatusRunning');
  if (value === 'COMPLETED') return t('cluster.config.cmdStatusCompleted');
  if (value === 'FAILED') return t('cluster.config.cmdStatusFailed');
  return value || '-';
};
const clusterCommandStatusTag = (value: string) => {
  if (value === 'COMPLETED') return 'success';
  if (value === 'RUNNING' || value === 'QUEUED') return 'warning';
  if (value === 'FAILED') return 'danger';
  return 'info';
};
const tagClassByType = (type?: string) => {
  if (type === 'success') return 'status-success';
  if (type === 'warning') return 'status-warning';
  if (type === 'danger') return 'status-danger';
  return 'status-muted';
};
const normHealth = (value: unknown) => {
  const v = String(value || '').toLowerCase();
  if (v.includes('healthy') || v.includes('正常') || v.includes('健康')) return 'healthy';
  if (v.includes('warning') || v.includes('告警')) return 'warning';
  return 'critical';
};
const healthLabel = (value: unknown) => (normHealth(value) === 'healthy' ? t('cluster.config.healthHealthy') : normHealth(value) === 'warning' ? t('cluster.config.healthWarning') : t('cluster.config.healthCritical'));
const healthTag = (value: unknown) => (normHealth(value) === 'healthy' ? 'success' : normHealth(value) === 'warning' ? 'warning' : 'danger');
const heartbeatTag = (value?: string) => (Date.now() - new Date(value || 0).getTime() < 20000 ? 'success' : 'danger');
const heartbeatLabel = (value?: string) => (Date.now() - new Date(value || 0).getTime() < 20000 ? t('cluster.config.heartbeatOk') : t('cluster.config.heartbeatDown'));
const formatTime = (value?: string) => {
  if (!value) return '-';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString();
};

const copyText = async (value: string, message: string) => {
  const content = String(value || '').trim();
  if (!content) {
    showWarning(t('cluster.config.noCopyContent'));
    return;
  }
  try {
    await navigator.clipboard.writeText(content);
    showSuccess(message);
  } catch {
    const textArea = document.createElement('textarea');
    textArea.value = content;
    textArea.style.position = 'fixed';
    textArea.style.opacity = '0';
    document.body.appendChild(textArea);
    textArea.select();
    document.execCommand('copy');
    document.body.removeChild(textArea);
    showSuccess(message);
  }
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

.page-header-main {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.page-header h3 {
  margin: 0;
}

.page-header p {
  margin: 4px 0 0;
  color: var(--text-secondary);
}

.guard-tip {
  margin-top: 12px;
  display: flex;
  align-items: center;
  gap: 10px;
}

.guard-disabled-wrap {
  display: inline-block;
}

.page-header-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: wrap;
}

.section-card {
  padding: 22px;
}

.section-title {
  margin-bottom: 14px;
  font-weight: 600;
}

.strategy-card {
  padding: 20px;
}

.strategy-card .section-title {
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
}

.strategy-form {
  display: grid;
}

.form-control-group {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.number-field,
.select-field {
  width: 240px;
}

.nowrap-label {
  white-space: nowrap;
}

.section-head {
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.cluster-control-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.control-summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}

.control-summary-item {
  padding: 16px;
  border: 1px solid var(--surface-border);
  border-radius: 12px;
  background: color-mix(in srgb, var(--surface-bg) 94%, var(--brand-primary) 6%);
  display: grid;
  gap: 6px;
}

.summary-neutral {
  border-color: color-mix(in srgb, var(--surface-border) 80%, var(--state-muted) 20%);
}

.summary-primary {
  border-color: color-mix(in srgb, var(--brand-primary) 35%, var(--surface-border));
}

.summary-success {
  border-color: color-mix(in srgb, var(--state-success) 38%, var(--surface-border));
}

.summary-warning {
  border-color: color-mix(in srgb, var(--state-warning) 45%, var(--surface-border));
}

.summary-label {
  font-size: 12px;
  color: var(--text-secondary);
}

.status-chip {
  width: fit-content;
  padding-inline: 10px;
  border-radius: 999px;
  border: none;
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

.control-alert {
  margin-bottom: 16px;
}

.collapsible-alert :deep(.el-alert__content) {
  width: 100%;
}

.alert-head-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.alert-head-actions {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.restart-guide p {
  margin: 4px 0;
}

.path-copy-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-top: 12px;
}

.path-copy-item {
  display: grid;
  gap: 6px;
  padding: 12px;
  border: 1px dashed var(--surface-border);
  border-radius: 10px;
}

.path-copy-label {
  font-size: 12px;
  color: var(--text-secondary);
}

.path-copy-item code {
  word-break: break-all;
}

.control-form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 18px;
}

.control-detail-grid {
  display: grid;
  gap: 16px;
}

.control-info-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.control-info-item {
  display: grid;
  gap: 8px;
  min-height: 92px;
  padding: 14px 16px;
  border: 1px solid color-mix(in srgb, var(--surface-border) 88%, var(--brand-primary) 12%);
  border-radius: 12px;
  background: color-mix(in srgb, var(--surface-bg) 96%, var(--brand-primary) 4%);
}

.control-info-label {
  font-size: 12px;
  color: var(--text-secondary);
}

.control-info-value {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.control-info-value span:first-child {
  flex: 1;
  word-break: break-word;
}

.config-grid-two {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 18px;
}

.protocol-shell {
  display: grid;
  gap: 18px;
}

.protocol-subsection {
  padding: 18px;
  border: 1px solid color-mix(in srgb, var(--surface-border) 90%, var(--brand-primary) 10%);
  border-radius: 12px;
  background: color-mix(in srgb, var(--surface-bg) 97%, var(--brand-primary) 3%);
}

.subsection-title {
  margin-bottom: 14px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.grid-span-1 {
  grid-column: span 1;
}

.grid-span-2 {
  grid-column: span 2;
}

.command-center-card {
  display: grid;
  gap: 16px;
  padding-top: 8px;
  border-top: 1px solid color-mix(in srgb, var(--surface-border) 88%, var(--brand-primary) 12%);
}

.command-form-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  align-items: end;
}

.command-form-grid-tight {
  margin-bottom: 16px;
}

.command-form-item {
  margin-bottom: 0;
}

.command-form-item-wide {
  grid-column: span 2;
}

.command-submit-wrap {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  height: 100%;
}

.command-submit-wrap-inline {
  min-width: 160px;
}

.command-receipt-list {
  display: grid;
  gap: 6px;
}

.command-receipt-item {
  display: grid;
  gap: 2px;
  font-size: 12px;
}

.command-receipt-card {
  padding: 10px 12px;
  border: 1px solid color-mix(in srgb, var(--surface-border) 86%, var(--brand-primary) 14%);
  border-radius: 10px;
  background: color-mix(in srgb, var(--surface-bg) 96%, var(--brand-primary) 4%);
}

.member-detail-grid {
  display: grid;
  gap: 16px;
}

.member-detail-card {
  display: grid;
  gap: 12px;
}

.control-footer-actions {
  position: static;
  padding: 0;
  margin-top: 4px;
}

.token-row {
  margin-top: 8px;
  overflow-x: auto;
}

.token-row code {
  font-family: 'Cascadia Code', 'Consolas', monospace;
  font-size: 12px;
  word-break: break-all;
}

.field-help {
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.6;
}

.switch-mode-help {
  margin-top: 10px;
}

.switch-mode-item :deep(.el-form-item__content) {
  display: block;
}

.switch-mode-group {
  display: inline-flex;
  gap: 8px;
  padding: 4px;
  border-radius: 12px;
  background: color-mix(in srgb, var(--surface-bg) 84%, var(--surface-border) 16%);
}

.switch-mode-group :deep(.el-radio-button__inner) {
  min-width: 132px;
  height: 38px;
  border-radius: 10px;
  border: 1px solid var(--surface-border);
  background: color-mix(in srgb, var(--surface-bg) 94%, var(--surface-border) 6%);
  color: var(--text-secondary);
  box-shadow: none;
  font-weight: 600;
}

.switch-mode-group :deep(.el-radio-button.is-active .el-radio-button__inner) {
  background: var(--brand-primary);
  border-color: var(--brand-primary);
  color: #fff;
  box-shadow: 0 10px 20px rgba(22, 119, 255, 0.22);
}

.action-button {
  border-radius: 10px;
}

.danger-button {
  background: color-mix(in srgb, var(--state-danger) 12%, var(--surface-bg));
  border-color: color-mix(in srgb, var(--state-danger) 45%, var(--surface-border));
  color: var(--state-danger);
}

.node-action-wrap {
  margin-right: 4px;
}

.node-action-btn {
  min-width: 72px;
  border-radius: 10px;
}

.primary-action {
  color: var(--brand-primary);
  border-color: color-mix(in srgb, var(--brand-primary) 36%, var(--surface-border));
  background: color-mix(in srgb, var(--brand-primary) 8%, var(--surface-bg));
}

.warning-action {
  color: #9a6700;
  border-color: color-mix(in srgb, var(--state-warning) 46%, var(--surface-border));
  background: color-mix(in srgb, var(--state-warning) 10%, var(--surface-bg));
}

.danger-action {
  color: var(--state-danger);
  border-color: color-mix(in srgb, var(--state-danger) 45%, var(--surface-border));
  background: color-mix(in srgb, var(--state-danger) 8%, var(--surface-bg));
}

.join-dialog-form {
  display: grid;
  gap: 16px;
}

.join-form-section {
  display: grid;
  gap: 14px;
  padding: 16px;
  border: 1px solid color-mix(in srgb, var(--surface-border) 88%, var(--brand-primary) 12%);
  border-radius: 12px;
  background: color-mix(in srgb, var(--surface-bg) 97%, var(--brand-primary) 3%);
}

.join-form-section-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-primary);
}

.join-form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
}

.join-candidate-item,
.join-dialog-form :deep(.el-form-item__content),
.certificate-field-shell,
.certificate-upload-panel {
  align-items: flex-start;
}

.join-field-help {
  margin-top: 6px;
}

.join-inline-alert {
  margin-top: 10px;
}

.certificate-field-shell {
  display: grid;
  gap: 12px;
  width: 100%;
}

.certificate-mode-switch {
  width: fit-content;
}

.certificate-mode-switch :deep(.el-radio-button__inner) {
  min-width: 118px;
  border-radius: 10px;
}

.certificate-upload-panel {
  display: grid;
  gap: 10px;
  width: 100%;
}

.certificate-upload-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.certificate-file-name {
  max-width: 100%;
  font-size: 12px;
  color: var(--text-secondary);
  word-break: break-all;
}

.certificate-file-input {
  display: none;
}

.danger-text-button {
  color: var(--state-danger);
}

.join-dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

:deep(.cluster-join-dialog .el-dialog) {
  border-radius: 16px;
  overflow: hidden;
}

:deep(.cluster-join-dialog .el-dialog__body) {
  padding-top: 8px;
}

:deep(.cluster-join-dialog .el-dialog__footer) {
  padding-top: 8px;
}

:deep(.join-dialog-form .el-form-item__label) {
  width: 128px;
  text-align: left;
  padding-right: 12px;
}

:deep(.join-dialog-form .el-form-item__content) {
  display: flex;
  flex-direction: column;
  justify-content: flex-start;
}

:deep(.join-dialog-form .el-input__wrapper),
:deep(.join-dialog-form .el-textarea__inner),
:deep(.join-dialog-form .el-select__wrapper) {
  width: 100%;
  min-height: 40px;
}

:deep(.join-required-item .el-input__wrapper),
:deep(.join-required-item .el-textarea__inner),
:deep(.join-required-item .el-select__wrapper) {
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--brand-primary) 18%, transparent) inset;
}

:deep(.join-dialog-form .el-form-item.is-error .el-input__wrapper),
:deep(.join-dialog-form .el-form-item.is-error .el-select__wrapper),
:deep(.join-dialog-form .el-form-item.is-error .el-textarea__inner) {
  box-shadow: 0 0 0 1px var(--state-danger) inset;
}

:deep(.join-dialog-form .el-form-item__error) {
  color: var(--state-danger);
}

.tip-tag {
  margin-left: 8px;
}

.table-pagination-wrap {
  margin-top: 14px;
  display: flex;
  justify-content: flex-end;
}

:deep(.el-form-item) {
  margin-bottom: 18px;
}

:deep(.el-form-item__label) {
  text-align: right;
  padding-right: 12px;
  width: 120px;
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

:deep(.strategy-form .el-input-number .el-input__wrapper),
:deep(.strategy-form .el-select .el-select__wrapper),
:deep(.control-form-grid .el-input-number .el-input__wrapper),
:deep(.control-form-grid .el-select .el-select__wrapper),
:deep(.command-form-grid .el-select .el-select__wrapper) {
  min-height: 38px;
}

:deep(.strategy-form .compact-item .el-form-item__label) {
  padding-right: 8px;
}

:deep(.cluster-command-table .cell) {
  white-space: normal;
  word-break: break-word;
}

:deep(.el-input__wrapper),
:deep(.el-textarea__inner),
:deep(.el-select__wrapper),
:deep(.el-input-number .el-input__wrapper) {
  border-radius: 10px;
}

@media (max-width: 1200px) {
  .page-wrap {
    padding: 16px;
  }

  .control-summary-grid,
  .control-info-grid,
  .control-form-grid,
  .command-form-grid,
  .join-form-grid,
  .config-grid-two,
  .path-copy-grid {
    grid-template-columns: 1fr;
  }

  .command-form-item-wide {
    grid-column: span 1;
  }

  .grid-span-2 {
    grid-column: span 1;
  }

  .form-control-group {
    width: 100%;
  }

  .number-field,
  .select-field {
    width: min(280px, 100%);
  }

  .page-header-main {
    flex-direction: column;
    align-items: stretch;
  }

  .page-header-actions {
    justify-content: flex-start;
  }

  .alert-head-row {
    flex-direction: column;
  }
}
</style>
