<template>
  <div class="pool-page">
    <el-card v-loading="tableLoading" class="surface-card table-card">
      <section class="page-header">
        <h3>{{ t('pool.ipv6Title') }}</h3>
        <p class="desc">{{ t('pool.addPrefixInfo') }}</p>
      </section>

      <div class="filter-bar surface-card">
        <div class="bar-left">
          <el-form :model="filters" inline class="filters" @submit.prevent>
            <el-form-item :label="t('pool.searchV6')">
              <el-input v-model="filters.keyword" :placeholder="t('pool.searchV6')" clearable size="small" />
            </el-form-item>
            <el-form-item :label="t('pool.v6Status')">
              <el-select v-model="filters.status" clearable :placeholder="t('pool.v6StatusAll')" size="small" style="width: 130px">
                <el-option :label="t('pool.v6StatusActive')" value="active" />
                <el-option :label="t('pool.v6StatusWarning')" value="warning" />
                <el-option :label="t('pool.v6StatusDisabled')" value="disabled" />
              </el-select>
            </el-form-item>
            <el-form-item :label="t('pool.v6PrefixLength')">
              <el-select v-model="filters.prefixLength" clearable :placeholder="t('pool.v6PrefixAll')" size="small" style="width: 120px">
                <el-option label="/48" :value="48" />
                <el-option label="/52" :value="52" />
                <el-option label="/56" :value="56" />
                <el-option label="/60" :value="60" />
                <el-option label="/64" :value="64" />
              </el-select>
            </el-form-item>
            <el-form-item :label="t('pool.v6UsageRange')">
              <el-select v-model="filters.usageRange" clearable :placeholder="t('pool.v6PrefixAll')" size="small" style="width: 150px">
                <el-option label="0% - 30%" value="0-30" />
                <el-option label="30% - 60%" value="30-60" />
                <el-option label="60% - 80%" value="60-80" />
                <el-option label="80% - 100%" value="80-100" />
              </el-select>
            </el-form-item>
            <el-form-item class="action-btns">
              <el-button size="small" type="primary" @click="handleSearch">{{ t('common.search') }}</el-button>
              <el-button size="small" @click="handleReset">{{ t('common.reset') }}</el-button>
            </el-form-item>
          </el-form>
        </div>
        <div class="bar-right">
          <el-button size="small" @click="downloadTemplate">{{ t('pool.v6DownloadTemplate') }}</el-button>
          <el-button size="small" :loading="exporting" @click="exportCsv">{{ t('pool.v6ExportCsv') }}</el-button>
          <el-button size="small" type="primary" :loading="importing" :disabled="!canManage" @click="openImport"
            >{{ t('pool.v6ImportCsv') }}</el-button
          >
          <el-button size="small" :icon="Refresh" :disabled="tableLoading" @click="fetchPools">{{
            t('common.refresh')
          }}</el-button>
          <el-button size="small" type="primary" :icon="Plus" :disabled="!canManage" @click="openCreate">
            {{ t('pool.create') }}
          </el-button>
        </div>
      </div>

      <section class="overview-grid">
        <el-card shadow="never" class="overview-card">
          <div class="overview-label">{{ t('pool.v6OverviewTotal') }}</div>
          <div class="overview-value">{{ overviewTotal }}</div>
        </el-card>
        <el-card shadow="never" class="overview-card">
          <div class="overview-label">{{ t('pool.v6OverviewEnabled') }}</div>
          <div class="overview-value">{{ enabledCount }}</div>
        </el-card>
        <el-card shadow="never" class="overview-card">
          <div class="overview-label">{{ t('pool.v6OverviewAvgUsage') }}</div>
          <div class="overview-value">{{ overviewAvgUsage.toFixed(1) }}%</div>
        </el-card>
        <el-card shadow="never" class="overview-card">
          <div class="overview-label">{{ t('pool.v6OverviewHighUsage') }}</div>
          <div class="overview-value">{{ overviewHighUsageCount }}</div>
        </el-card>
      </section>

      <div v-if="exporting || importing" class="progress-wrap">
        <div v-if="exporting" class="progress-item">
          <span>{{ t('pool.v6ExportProgress') }}</span>
          <el-progress :percentage="exportProgress" />
        </div>
        <div v-if="importing" class="progress-item">
          <span>{{ t('pool.v6ImportProgress') }}</span>
          <el-progress :percentage="importProgress" status="success" />
        </div>
      </div>

      <div class="selection-tools">
        <div class="selection-info">{{ t('pool.v6Selected', { selected: selectedIds.length, total: pools.length }) }}</div>
        <div class="selection-actions">
          <el-button size="small" :disabled="!pools.length" @click="selectAllCurrent"
            >{{ t('pool.v6SelectAll') }}</el-button
          >
          <el-button size="small" :disabled="!pools.length" @click="invertSelection"
            >{{ t('pool.v6Invert') }}</el-button
          >
          <el-button size="small" :disabled="!selectedIds.length" @click="clearSelection"
            >{{ t('pool.v6Clear') }}</el-button
          >
          <el-button
            size="small"
            type="warning"
            :disabled="!selectedIds.length || !canManage"
            @click="batchSetStatus('disabled')"
            >{{ t('pool.v6BatchDisable') }}</el-button
          >
          <el-button
            size="small"
            type="success"
            :disabled="!selectedIds.length || !canManage"
            @click="batchSetStatus('active')"
            >{{ t('pool.v6BatchEnable') }}</el-button
          >
          <el-button
            size="small"
            type="danger"
            :disabled="!selectedIds.length || !canManage"
            @click="batchDelete"
            >{{ t('pool.v6BatchDelete') }}</el-button
          >
        </div>
      </div>
      <AppErrorCallout v-if="pageError" :error="pageError" class="mb-12" />
      <PoolTable
        :rows="pools"
        :loading="tableLoading"
        :pagination="pagination"
        :show-actions="true"
        :actions-disabled="!canManage"
        :virtual="false"
        :virtual-threshold="200"
        :table-height="920"
        selectable
        :selected-keys="selectedIds"
        @edit="openEdit"
        @remove="handleDelete"
        @select="handleSelect"
        @selection-change="handleSelectionChange"
        @page-change="handlePageChange"
        @size-change="handleSizeChange"
        @toggle-status="handleToggleStatus"
      />
    </el-card>

    <el-dialog
      v-model="createVisible"
      class="ipv6-create-dialog"
      width="720px"
      :before-close="handleBeforeCreateClose"
      @close="handleCreateDialogClose"
      destroy-on-close
      align-center
    >
      <template #header>
        <div class="dialog-title">{{ t('pool.v6CreateTitle') }}</div>
      </template>

      <div class="ipv6-dialog-body">
        <el-collapse v-model="activeSections" class="group-collapse" @change="handleCollapseChange">
          <el-collapse-item name="base" :title="t('pool.v6BaseSection')" class="panel-base">
            <div class="form-row">
              <label class="form-label"><span class="required">*</span> {{ t('pool.v6PoolName') }}</label>
              <div class="form-control">
                <el-input
                  v-model="createForm.name"
                  :placeholder="t('pool.v6PoolNamePlaceholder')"
                  maxlength="64"
                  @blur="validateField('name')"
                />
                <p v-if="fieldErrors.name" class="field-error">{{ fieldErrors.name }}</p>
              </div>
            </div>

            <div class="form-row">
              <label class="form-label"><span class="required">*</span> {{ t('pool.v6Prefix') }}</label>
              <div class="form-control">
                <el-input
                  v-model="createForm.cidr"
                  :placeholder="t('pool.v6PrefixPlaceholder')"
                  @blur="validateField('cidr')"
                />
                <p v-if="fieldErrors.cidr" class="field-error">{{ fieldErrors.cidr }}</p>
              </div>
            </div>

            <div class="form-row">
              <label class="form-label">{{ t('pool.v6Gateway') }} <span class="optional">{{ t('pool.wizardOptional') }}</span></label>
              <div class="form-control">
                <el-input
                  v-model="createForm.gateway"
                  :placeholder="t('pool.v6GatewayPlaceholder')"
                  @blur="validateField('gateway')"
                />
                <p v-if="fieldErrors.gateway" class="field-error">{{ fieldErrors.gateway }}</p>
              </div>
            </div>

            <div class="form-row">
              <label class="form-label">{{ t('pool.v6Location') }} <span class="optional">{{ t('pool.wizardOptional') }}</span></label>
              <div class="form-control">
                <el-input
                  v-model="createForm.location"
                  :placeholder="t('pool.v6LocationPlaceholder')"
                  maxlength="64"
                  @blur="validateField('location')"
                />
                <p v-if="fieldErrors.location" class="field-error">{{ fieldErrors.location }}</p>
              </div>
            </div>
          </el-collapse-item>

          <el-collapse-item name="mode" :title="t('pool.v6ModeSection')" class="panel-mode">
            <div class="form-row">
              <label class="form-label"><span class="required">*</span> {{ t('pool.v6DhcpMode') }}</label>
              <div class="form-control">
                <el-radio-group v-model="createForm.dhcpMode" @change="validateField('dhcpMode')">
                  <el-radio-button label="stateful">{{ t('pool.v6Stateful') }}</el-radio-button>
                  <el-radio-button label="stateless">{{ t('pool.v6Stateless') }}</el-radio-button>
                </el-radio-group>
                <div class="field-hint">{{ t('pool.v6StatelessHint') }}</div>
              </div>
            </div>

            <div class="form-row">
              <label class="form-label"
                >{{ t('pool.v6RaMode') }}
                <el-tooltip :content="t('pool.v6RaTooltip')" placement="top">
                  <el-icon><QuestionFilled /></el-icon>
                </el-tooltip>
              </label>
              <div class="form-control">
                <el-select
                  v-model="createForm.raMode"
                  filterable
                  :placeholder="t('pool.v6RaPlaceholder')"
                  @change="validateField('raMode')"
                >
                  <el-option value="managed" label="Managed (M=1,O=0)">
                    <el-tooltip :content="t('pool.v6RaManagedTip')" placement="right">
                      <span>Managed (M=1,O=0)</span>
                    </el-tooltip>
                  </el-option>
                  <el-option value="assisted" label="Assisted (M=1,O=1)">
                    <el-tooltip :content="t('pool.v6RaAssistedTip')" placement="right">
                      <span>Assisted (M=1,O=1)</span>
                    </el-tooltip>
                  </el-option>
                  <el-option value="stateless" label="Stateless (M=0,O=1)">
                    <el-tooltip :content="t('pool.v6RaStatelessTip')" placement="right">
                      <span>Stateless (M=0,O=1)</span>
                    </el-tooltip>
                  </el-option>
                </el-select>
                <p v-if="fieldErrors.raMode" class="field-error">{{ fieldErrors.raMode }}</p>
              </div>
            </div>

            <div class="form-row">
              <label class="form-label"
                ><span class="required">*</span> {{ t('pool.v6AllocMode') }}
                <el-tooltip :content="t('pool.v6AllocModeTooltip')" placement="top">
                  <el-icon><QuestionFilled /></el-icon>
                </el-tooltip>
              </label>
              <div class="form-control">
                <el-tabs v-model="createForm.allocationMode" type="card" class="alloc-tabs" @tab-change="validateField('allocationMode')">
                  <el-tab-pane name="sequential" :label="t('pool.v6Sequential')" />
                  <el-tab-pane name="round-robin" :label="t('pool.v6RoundRobin')" />
                  <el-tab-pane name="random" :label="t('pool.v6WeightedRandom')" />
                </el-tabs>
                <div class="field-hint">{{ t('pool.v6AllocHint') }}</div>
                <p v-if="fieldErrors.allocationMode" class="field-error">{{ fieldErrors.allocationMode }}</p>
              </div>
            </div>

            <div v-if="createForm.allocationMode === 'random'" class="form-row">
              <label class="form-label"><span class="required">*</span> {{ t('pool.v6Weight') }}</label>
              <div class="form-control">
                <el-input-number
                  v-model="createForm.priorityWeight"
                  :min="1"
                  :max="100"
                  :step="1"
                  :controls="false"
                  @change="validateField('priorityWeight')"
                />
                <p v-if="fieldErrors.priorityWeight" class="field-error">{{ fieldErrors.priorityWeight }}</p>
              </div>
            </div>
          </el-collapse-item>

          <el-collapse-item name="lease" :title="t('pool.v6LeaseSection')" class="panel-lease">
            <div class="template-toolbar">
              <el-select v-model="selectedTemplateName" filterable :placeholder="t('pool.v6FromTemplate')" clearable class="narrow-select">
                <el-option v-for="tpl in templates" :key="tpl.name" :label="tpl.name" :value="tpl.name">
                  <el-tooltip :content="t('pool.v6TemplateTip', { name: tpl.name })" placement="right">
                    <span>{{ tpl.name }}</span>
                  </el-tooltip>
                </el-option>
              </el-select>
              <el-button @click="applyTemplate">{{ t('pool.v6ApplyTemplate') }}</el-button>
              <el-select v-model="selectedPoolId" filterable :placeholder="t('pool.v6FromPool')" clearable class="mid-select">
                <el-option v-for="pool in poolFillOptions" :key="pool.id" :label="pool.name" :value="pool.id">
                  <el-tooltip :content="pool.cidr" placement="right">
                    <span>{{ pool.name }}</span>
                  </el-tooltip>
                </el-option>
              </el-select>
              <el-button @click="applyFromPool">{{ t('pool.v6ApplyFromPool') }}</el-button>
              <el-button type="primary" plain @click="saveTemplate">{{ t('pool.v6SaveAsTemplate') }}</el-button>
            </div>

            <div v-if="createForm.dhcpMode === 'stateful'" class="form-row two-col">
              <div class="col-item">
                <label class="form-label">
                  <span class="required">*</span> {{ t('pool.v6MinLease') }}
                  <el-tooltip :content="t('pool.v6MinLeaseTooltip')" placement="top">
                    <el-icon><QuestionFilled /></el-icon>
                  </el-tooltip>
                </label>
                <div class="form-control">
                  <el-input-number
                    v-model="createForm.leaseTime"
                    :min="300"
                    :max="604800"
                    :step="300"
                    :controls="false"
                    :placeholder="t('pool.v6MinLeasePlaceholder')"
                    @change="validateField('leaseTime')"
                  />
                  <div class="field-hint">{{ t('pool.v6MinLeaseHint') }}</div>
                  <p v-if="fieldErrors.leaseTime" class="field-error">{{ fieldErrors.leaseTime }}</p>
                </div>
              </div>
              <div class="col-item">
                <label class="form-label">
                  <span class="required">*</span> {{ t('pool.v6MaxLease') }}
                  <el-tooltip :content="t('pool.v6MaxLeaseTooltip')" placement="top">
                    <el-icon><QuestionFilled /></el-icon>
                  </el-tooltip>
                </label>
                <div class="form-control">
                  <el-input-number
                    v-model="createForm.maxLeaseTime"
                    :min="300"
                    :max="604800"
                    :step="300"
                    :controls="false"
                    :placeholder="t('pool.v6MaxLeasePlaceholder')"
                    @change="validateField('maxLeaseTime')"
                  />
                  <div class="field-hint">{{ t('pool.v6MaxLeaseHint') }}</div>
                  <p v-if="fieldErrors.maxLeaseTime" class="field-error">{{ fieldErrors.maxLeaseTime }}</p>
                </div>
              </div>
            </div>
          </el-collapse-item>

          <el-collapse-item name="dns" :title="t('pool.v6DnsSection')" class="panel-dns">
            <div class="form-row">
              <label class="form-label">{{ t('pool.v6DnsPrimary') }} <span class="optional">{{ t('pool.wizardOptional') }}</span></label>
              <div class="form-control">
                <el-input
                  v-model="createForm.dnsPrimary"
                  :placeholder="t('pool.v6DnsPrimaryPlaceholder')"
                  @blur="validateField('dnsPrimary')"
                />
                <p v-if="fieldErrors.dnsPrimary" class="field-error">{{ fieldErrors.dnsPrimary }}</p>
              </div>
            </div>

            <div class="form-row">
              <label class="form-label">{{ t('pool.v6DnsSecondary') }} <span class="optional">{{ t('pool.wizardOptional') }}</span></label>
              <div class="form-control">
                <el-input
                  v-model="createForm.dnsSecondary"
                  :placeholder="t('pool.v6DnsSecondaryPlaceholder')"
                  @blur="validateField('dnsSecondary')"
                />
                <p v-if="fieldErrors.dnsSecondary" class="field-error">{{ fieldErrors.dnsSecondary }}</p>
              </div>
            </div>

            <div class="form-row">
              <label class="form-label">{{ t('pool.v6DomainSearch') }} <span class="optional">{{ t('pool.wizardOptional') }}</span></label>
              <div class="form-control">
                <el-input
                  v-model="createForm.domainSearch"
                  :placeholder="t('pool.v6DomainSearchPlaceholder')"
                  @blur="validateField('domainSearch')"
                />
                <p v-if="fieldErrors.domainSearch" class="field-error">{{ fieldErrors.domainSearch }}</p>
              </div>
            </div>
          </el-collapse-item>

          <el-collapse-item name="pd" :title="t('pool.v6PdSection')" class="panel-pd">
            <div class="form-row">
              <label class="form-label">{{ t('pool.v6PdEnable') }}</label>
              <div class="form-control">
                <el-switch v-model="createForm.pdEnabled" />
                <div class="field-hint">{{ t('pool.v6PdEnableHint') }}</div>
              </div>
            </div>

            <template v-if="createForm.pdEnabled">
              <div class="form-row two-col">
                <div class="col-item">
                  <label class="form-label"><span class="required">*</span> {{ t('pool.v6PdPrefixLength') }}</label>
                  <div class="form-control">
                    <el-input-number
                      v-model="createForm.pdPrefixLength"
                      :min="48"
                      :max="124"
                      :step="1"
                      :controls="false"
                      :placeholder="t('pool.v6PdPrefixLengthPlaceholder')"
                      @change="validateField('pdPrefixLength')"
                    />
                    <p v-if="fieldErrors.pdPrefixLength" class="field-error">{{ fieldErrors.pdPrefixLength }}</p>
                  </div>
                </div>
                <div class="col-item">
                  <label class="form-label"><span class="required">*</span> {{ t('pool.v6PdMaxDepth') }}</label>
                  <div class="form-control">
                    <el-input-number
                      v-model="createForm.pdMaxDepth"
                      :min="1"
                      :max="8"
                      :step="1"
                      :controls="false"
                      :placeholder="t('pool.v6PdMaxDepthPlaceholder')"
                      @change="validateField('pdMaxDepth')"
                    />
                    <p v-if="fieldErrors.pdMaxDepth" class="field-error">{{ fieldErrors.pdMaxDepth }}</p>
                  </div>
                </div>
              </div>

              <div class="form-row two-col">
                <div class="col-item">
                  <label class="form-label"><span class="required">*</span> {{ t('pool.v6PdConcurrent') }}</label>
                  <div class="form-control">
                    <el-input-number
                      v-model="createForm.pdConcurrent"
                      :min="1"
                      :max="200"
                      :step="1"
                      :controls="false"
                      :placeholder="t('pool.v6PdConcurrentPlaceholder')"
                      @change="validateField('pdConcurrent')"
                    />
                    <p v-if="fieldErrors.pdConcurrent" class="field-error">{{ fieldErrors.pdConcurrent }}</p>
                  </div>
                </div>
                <div class="col-item">
                  <label class="form-label">{{ t('pool.v6PdNotify') }}</label>
                  <div class="form-control">
                    <el-switch v-model="createForm.pdNotify" />
                  </div>
                </div>
              </div>
            </template>
          </el-collapse-item>

          <el-collapse-item name="alert" :title="t('pool.v6AlertSection')" class="panel-alert">
            <div class="form-row">
              <label class="form-label"><span class="required">*</span> {{ t('pool.v6WarnThreshold') }}</label>
              <div class="form-control">
                <el-input-number
                  v-model="createForm.warnThreshold"
                  :min="1"
                  :max="100"
                  :step="1"
                  :controls="false"
                  :placeholder="t('pool.v6WarnThresholdPlaceholder')"
                  @change="validateField('warnThreshold')"
                />
                <div class="field-hint">{{ t('pool.v6WarnThresholdHint') }}</div>
                <p v-if="fieldErrors.warnThreshold" class="field-error">{{ fieldErrors.warnThreshold }}</p>
              </div>
            </div>
          </el-collapse-item>
        </el-collapse>
      </div>

      <template #footer>
        <div class="dialog-footer">
          <el-button :disabled="createPending" @click="requestCloseCreateDialog">{{ t('pool.v6Cancel') }}</el-button>
          <el-button :disabled="createPending" @click="runPrecheck">{{ t('pool.v6Precheck') }}</el-button>
          <el-button :disabled="createPending" @click="openPreview">{{ t('pool.v6Preview') }}</el-button>
          <el-button type="primary" :loading="createPending" @click="handleSubmit">{{ t('pool.v6Submit') }}</el-button>
        </div>
      </template>
    </el-dialog>

    <el-dialog v-model="previewVisible" :title="t('pool.v6PreviewTitle')" width="640px">
      <el-descriptions :column="1" border class="preview-desc">
        <el-descriptions-item :label="t('pool.v6PreviewName')">{{ createForm.name || '-' }}</el-descriptions-item>
        <el-descriptions-item label="CIDR">{{ createForm.cidr || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="t('pool.v6PreviewGateway')">{{ createForm.gateway || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="t('pool.v6PreviewDhcpMode')">{{ createForm.dhcpMode }}</el-descriptions-item>
        <el-descriptions-item :label="t('pool.v6PreviewRaMode')">{{ createForm.raMode }}</el-descriptions-item>
        <el-descriptions-item :label="t('pool.v6PreviewAllocMode')">{{ createForm.allocationMode }}</el-descriptions-item>
        <el-descriptions-item :label="t('pool.v6PreviewMinLease')">{{ createForm.leaseTime ? t('pool.v6PreviewLeaseSec', { value: createForm.leaseTime }) : '-' }}</el-descriptions-item>
        <el-descriptions-item :label="t('pool.v6PreviewMaxLease')">{{ createForm.maxLeaseTime ? t('pool.v6PreviewLeaseSec', { value: createForm.maxLeaseTime }) : '-' }}</el-descriptions-item>
        <el-descriptions-item label="DNS">
          {{ [createForm.dnsPrimary, createForm.dnsSecondary].filter(Boolean).join(', ') || '-' }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('pool.v6PreviewPd')">
          {{ createForm.pdEnabled ? `/${createForm.pdPrefixLength}, depth=${createForm.pdMaxDepth}` : t('pool.v6PreviewPdOff') }}
        </el-descriptions-item>
      </el-descriptions>
    </el-dialog>

    <el-dialog v-model="importDialog" :title="t('pool.v6ImportTitle')" width="520px">
      <div class="import-body">
        <p class="tip">{{ t('pool.v6ImportTip') }}</p>
        <el-upload
          drag
          :auto-upload="false"
          :show-file-list="false"
          accept=".csv"
          @change="handleImportFile"
        >
          <el-icon class="el-icon--upload"><UploadFilled /></el-icon>
          <div class="el-upload__text">{{ t('pool.v6ImportDragText') }}</div>
        </el-upload>

        <el-alert
          v-if="importError"
          type="error"
          show-icon
          :closable="false"
          class="mt-12"
          :title="importError"
        />

        <div v-if="importStats.total > 0" class="mt-12">
          <el-descriptions :column="2" border>
            <el-descriptions-item :label="t('pool.v6ImportTotal')">{{ importStats.total }}</el-descriptions-item>
            <el-descriptions-item :label="t('pool.v6ImportSuccess')">{{ importStats.success }}</el-descriptions-item>
            <el-descriptions-item :label="t('pool.v6ImportFailed')">{{ importStats.failed }}</el-descriptions-item>
          </el-descriptions>
          <el-scrollbar v-if="failedRows.length" height="160" class="mt-8">
            <div v-for="item in failedRows" :key="item.row" class="fail-list">
              {{ t('pool.v6ImportRowError', { row: item.row, error: item.error }) }}
            </div>
          </el-scrollbar>
        </div>
      </div>
      <template #footer>
        <el-button :disabled="importing" @click="importDialog = false">{{ t('pool.v6Cancel') }}</el-button>
        <el-button type="primary" :loading="importing" :disabled="!importFile" @click="submitImport"
          >{{ t('pool.v6ImportStart') }}</el-button
        >
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { Plus, QuestionFilled, Refresh, UploadFilled } from '@element-plus/icons-vue';
import { useI18n } from 'vue-i18n';
import { ElMessageBox } from 'element-plus';
import { showError, showInfo, showSuccess } from '@/shared/errors/messageToast';
import { listPools, getPoolStats, updatePool, createPool, deletePool, getPool, importPoolsCsv, exportPoolsCsv } from '@/api/pools';
import type { PoolSummary, SubnetDraft } from '@/types/pool';
import PoolTable from './components/PoolTable.vue';
import AppErrorCallout from '@/components/common/AppErrorCallout.vue';
import { getApiError, createInlineError } from '@/shared/errors/apiError';
import type { ApiErrorDescriptor } from '@/shared/errors/apiError';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { usePoolApiGuard } from './usePoolApiGuard';
import { isIPv6, validateIPv6Prefix } from '@/utils/ip';

const { t } = useI18n();
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const { wrapApi } = usePoolApiGuard();

type ImportRow = Partial<
  SubnetDraft & {
    scope?: string;
    parentId?: string;
    interfaceId?: string;
    ssid?: string;
    reservePercent?: string;
    leaseProfileId?: string;
    tags?: string;
    vlanId?: string;
    location?: string;
    gateway?: string;
    dns?: string;
    exclusions?: string;
    allocationMode?: string;
    priorityWeight?: string;
    rangeStart?: string;
    rangeEnd?: string;
    network?: string;
    netmask?: string;
  }
>;

const filters = reactive<{
  keyword: string;
  status: '' | 'active' | 'warning' | 'disabled';
  prefixLength: number | null;
  usageRange: '' | '0-30' | '30-60' | '60-80' | '80-100';
}>({
  keyword: '',
  status: '',
  prefixLength: null,
  usageRange: ''
});
const pagination = reactive({ page: 1, pageSize: 10, total: 0 });
const pools = ref<PoolSummary[]>([]);
const tableLoading = ref(false);
const pageError = ref<ApiErrorDescriptor | null>(null);
const selectedIds = ref<string[]>([]);
const editingPoolId = ref<string | null>(null);
const createVisible = ref(false);
const createPending = ref(false);
const previewVisible = ref(false);
const activeSections = ref<string[]>(['base', 'mode']);
const selectedTemplateName = ref('');
const selectedPoolId = ref('');
const snapshot = ref('');

type AllocationMode = 'sequential' | 'round-robin' | 'random';
type DHCPMode = 'stateful' | 'stateless';
type RAMode = 'managed' | 'assisted' | 'stateless';

type IPv6CreateForm = {
  name: string;
  cidr: string;
  gateway: string;
  location: string;
  dhcpMode: DHCPMode;
  raMode: RAMode;
  allocationMode: AllocationMode;
  priorityWeight: number;
  leaseTime: number;
  maxLeaseTime: number;
  dnsPrimary: string;
  dnsSecondary: string;
  domainSearch: string;
  pdEnabled: boolean;
  pdPrefixLength: number;
  pdMaxDepth: number;
  pdConcurrent: number;
  pdNotify: boolean;
  warnThreshold: number;
};

const createDefaultForm = (): IPv6CreateForm => ({
  name: '',
  cidr: '',
  gateway: '',
  location: '',
  dhcpMode: 'stateful',
  raMode: 'managed',
  allocationMode: 'round-robin',
  priorityWeight: 1,
  leaseTime: 1800,
  maxLeaseTime: 7200,
  dnsPrimary: '',
  dnsSecondary: '',
  domainSearch: '',
  pdEnabled: true,
  pdPrefixLength: 56,
  pdMaxDepth: 2,
  pdConcurrent: 20,
  pdNotify: true,
  warnThreshold: 80
});

const createForm = reactive<IPv6CreateForm>(createDefaultForm());
const fieldErrors = reactive<Record<string, string>>({});
const templates = ref<Array<{ name: string; payload: IPv6CreateForm }>>([]);
const allPoolOptions = ref<Array<{ id: string; name: string; cidr: string }>>([]);
const poolFillOptions = computed(() => allPoolOptions.value.filter((item) => item.id !== editingPoolId.value));

const isDirty = computed(() => snapshot.value !== JSON.stringify(createForm));

const exporting = ref(false);
const importing = ref(false);
const importDialog = ref(false);
const importFile = ref<File | null>(null);
const importError = ref('');
const importStats = reactive({ total: 0, success: 0, failed: 0 });
const failedRows = ref<{ row: number; error: string }[]>([]);
const exportProgress = ref(0);
const importProgress = ref(0);
const overviewTotal = ref(0);
const enabledCount = ref(0);
const overviewAvgUsage = ref(0);
const overviewHighUsageCount = ref(0);
const canManage = computed(() => permissionStore.can('pool.ipv6.manage'));

let controller: AbortController | null = null;
let requestToken = 0;

const getTenantId = () => tenantStore.currentTenantId || 'global';

const downloadTemplate = () => {
  const header =
    'name,cidr,scope,parentId,vlanId,interfaceId,ssid,location,reservePercent,leaseProfileId,tags,gateway,dns,rangeStart,rangeEnd,exclusions,allocationMode,priorityWeight';
  const comment = [
    t('pool.v6CsvName'),
    t('pool.v6CsvCidr'),
    t('pool.v6CsvScope'),
    t('pool.v6CsvParentId'),
    t('pool.v6CsvVlanId'),
    t('pool.v6CsvInterfaceId'),
    t('pool.v6CsvSsid'),
    t('pool.v6CsvLocation'),
    t('pool.v6CsvReserve'),
    t('pool.v6CsvLeaseProfile'),
    t('pool.v6CsvTags'),
    t('pool.v6CsvGateway'),
    t('pool.v6CsvDns'),
    t('pool.v6CsvRangeStart'),
    t('pool.v6CsvRangeEnd'),
    t('pool.v6CsvExclusions'),
    t('pool.v6CsvAllocMode'),
    t('pool.v6CsvWeight')
  ].join(',');
  const example = [
    t('pool.v6CsvExample'),
    '2001:db8::/48',
    'GLOBAL',
    '',
    '',
    '',
    '',
    '',
    '10',
    'default-lease-profile',
    'role=core;env=prod',
    '2001:db8::1',
    '2001:4860:4860::8888;2001:4860:4860::8844',
    '2001:db8::10',
    '2001:db8::ffff',
    '2001:db8::50-2001:db8::60',
    'ROUND_ROBIN',
    '1'
  ].join(',');
  const bom = '\ufeff';
  const blob = new Blob([`${bom}${header}\n${comment}\n${example}\n`], {
    type: 'text/csv;charset=utf-8;'
  });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'ipv6-pool-template.csv';
  a.click();
  URL.revokeObjectURL(url);
};

const exportCsv = async () => {
  try {
    exporting.value = true;
    exportProgress.value = 10;
    const response = await wrapApi(() => exportPoolsCsv({ version: 6 }), t('pool.loadFail'));
    exportProgress.value = 70;
    const blob =
      response.data instanceof Blob
        ? response.data
        : new Blob([response.data as any], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'ipv6-pools-export.csv';
    a.click();
    exportProgress.value = 100;
    URL.revokeObjectURL(url);
  } finally {
    exporting.value = false;
    window.setTimeout(() => {
      exportProgress.value = 0;
    }, 300);
  }
};

const openImport = () => {
  importDialog.value = true;
  importError.value = '';
  importFile.value = null;
  importStats.total = 0;
  importStats.success = 0;
  importStats.failed = 0;
  failedRows.value = [];
};

const parsePrefixLength = (cidr: string): number | null => {
  const value = String(cidr || '').trim();
  const idx = value.lastIndexOf('/');
  if (idx < 0) return null;
  const parsed = Number(value.slice(idx + 1));
  return Number.isFinite(parsed) ? parsed : null;
};

const inUsageRange = (usage: number, range: '' | '0-30' | '30-60' | '60-80' | '80-100') => {
  if (!range) return true;
  const value = Math.max(0, Math.min(100, Number(usage || 0)));
  if (range === '0-30') return value >= 0 && value < 30;
  if (range === '30-60') return value >= 30 && value < 60;
  if (range === '60-80') return value >= 60 && value < 80;
  return value >= 80 && value <= 100;
};

const applyLocalFilters = (rows: PoolSummary[]) =>
  rows.filter((item) => {
    if (filters.prefixLength !== null) {
      const prefixLength = parsePrefixLength(item.cidr);
      if (prefixLength !== filters.prefixLength) return false;
    }
    if (!inUsageRange(Number(item.utilization || 0), filters.usageRange)) return false;
    return true;
  });

const fetchPools = async () => {
  if (!permissionStore.can('pool.ipv6.view')) return;
  const token = ++requestToken;
  const tenantId = getTenantId();
  controller?.abort();
  controller = new AbortController();
  tableLoading.value = true;
  pageError.value = null;
  try {
    const localFilterEnabled = filters.prefixLength !== null || !!filters.usageRange;
    if (!localFilterEnabled) {
      const [pageResponse, statsResponse] = await Promise.allSettled([
        wrapApi(
          () =>
            listPools(
              {
                page: pagination.page,
                pageSize: pagination.pageSize,
                keyword: filters.keyword || undefined,
                status: filters.status || undefined,
                version: 6,
                tenantId
              },
              controller?.signal
            ),
          t('pool.loadFail')
        ),
        getPoolStats(
          {
            version: 6,
            keyword: filters.keyword || undefined,
            status: filters.status || undefined,
            tenantId
          },
          controller?.signal
        )
      ]);
      if (pageResponse.status !== 'fulfilled') {
        throw pageResponse.reason;
      }
      if (token !== requestToken) return;
      const data = pageResponse.value.data;
      pools.value = (data.data.items || []).slice(0, pagination.pageSize);
      pagination.total = data.data.total;
      if (statsResponse.status === 'fulfilled') {
        overviewTotal.value = Number(statsResponse.value.data?.data?.total ?? 0);
        enabledCount.value = Number(statsResponse.value.data?.data?.enabledCount ?? 0);
        overviewAvgUsage.value = Number(statsResponse.value.data?.data?.avgUsage ?? 0);
        overviewHighUsageCount.value = Number(statsResponse.value.data?.data?.highUsageCount ?? 0);
      }
    } else {
      let page = 1;
      const pageSize = 500;
      const all: PoolSummary[] = [];
      const seen = new Set<string>();
      while (true) {
        const { data } = await wrapApi(
          () =>
            listPools(
              {
                page,
                pageSize,
                keyword: filters.keyword || undefined,
                status: filters.status || undefined,
                version: 6,
                tenantId
              },
              controller?.signal
            ),
          t('pool.loadFail')
        );
        const items = data.data.items || [];
        const total = data.data.total || items.length;
        items.forEach((item) => {
          if (item?.id && !seen.has(item.id)) {
            seen.add(item.id);
            all.push(item);
          }
        });
        if (!items.length || items.length < pageSize || all.length >= total || page >= 200) break;
        page += 1;
      }
      const filtered = applyLocalFilters(all);
      const offset = (pagination.page - 1) * pagination.pageSize;
      if (offset >= filtered.length && pagination.page > 1) {
        pagination.page = 1;
      }
      const start = (pagination.page - 1) * pagination.pageSize;
      pools.value = filtered.slice(start, start + pagination.pageSize);
      pagination.total = filtered.length;
      overviewTotal.value = filtered.length;
      enabledCount.value = filtered.filter((item) => item.status === 'active').length;
      const sumUsage = filtered.reduce((sum, item) => sum + Number(item.utilization || 0), 0);
      overviewAvgUsage.value = filtered.length ? sumUsage / filtered.length : 0;
      overviewHighUsageCount.value = filtered.filter((item) => Number(item.utilization || 0) >= 80).length;
    }
    selectedIds.value = selectedIds.value.filter((id) => pools.value.some((p) => p.id === id));
  } catch (error: any) {
    if (error?.code === 'ERR_CANCELED') return;
    if (error?.name === 'AbortError' || error?.name === 'CanceledError') return;
    pageError.value = getApiError(error) || createInlineError(t('pool.loadFail'));
  } finally {
    if (token === requestToken) {
      tableLoading.value = false;
    }
    controller = null;
  }
};

const handleSearch = () => {
  pagination.page = 1;
  selectedIds.value = [];
  fetchPools();
};

const handleReset = () => {
  filters.keyword = '';
  filters.status = '';
  filters.prefixLength = null;
  filters.usageRange = '';
  handleSearch();
};

const handlePageChange = (page: number) => {
  pagination.page = page;
  fetchPools();
};

const handleSizeChange = (size: number) => {
  pagination.pageSize = size;
  pagination.page = 1;
  fetchPools();
};

const handleSelectionChange = (selection: PoolSummary[]) => {
  selectedIds.value = selection.map((item) => item.id);
};

const selectAllCurrent = () => {
  selectedIds.value = pools.value.map((p) => p.id);
};

const invertSelection = () => {
  const set = new Set(selectedIds.value);
  selectedIds.value = pools.value.filter((p) => !set.has(p.id)).map((p) => p.id);
};

const clearSelection = () => {
  selectedIds.value = [];
};

const selectedRows = computed(() => pools.value.filter((row) => selectedIds.value.includes(row.id)));

const batchSetStatus = async (status: 'active' | 'disabled') => {
  if (!selectedRows.value.length || !canManage.value) return;
  const actionText = status === 'active' ? t('pool.v6StatusActive') : t('pool.v6StatusDisabled');
  try {
    await ElMessageBox.confirm(t('pool.v6BatchConfirm', { action: actionText, count: selectedRows.value.length }), t('pool.v6BatchTitle'), {
      type: 'warning'
    });
    const tenantId = getTenantId();
    for (const row of selectedRows.value) {
      await wrapApi(() => updatePool(row.id, { status, tenantId }), t('pool.saveFail'));
    }
    showSuccess(t('pool.v6BatchDone', { action: actionText }));
    await fetchPools();
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      showError(t('pool.v6BatchFail', { action: actionText }));
    }
  }
};

const batchDelete = async () => {
  if (!selectedRows.value.length || !canManage.value) return;
  try {
    await ElMessageBox.confirm(
      t('pool.v6BatchDeleteConfirm', { count: selectedRows.value.length }),
      t('pool.v6BatchDeleteTitle'),
      {
        type: 'warning'
      }
    );
    const tenantId = getTenantId();
    for (const row of selectedRows.value) {
      await wrapApi(() => deletePool(row.id, { tenantId }), t('pool.deleteFail'));
    }
    showSuccess(t('pool.v6BatchDeleteDone'));
    selectedIds.value = [];
    await fetchPools();
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      showError(t('pool.v6BatchDeleteFail'));
    }
  }
};

const handleSelect = () => {
  // keep row-click behavior consistent with IPv4; no extra detail panel in IPv6 currently.
};

const clearFieldError = (key: string) => {
  delete fieldErrors[key];
};

const resetFieldErrors = () => {
  Object.keys(fieldErrors).forEach((key) => delete fieldErrors[key]);
};

const safeClone = (value: IPv6CreateForm): IPv6CreateForm => JSON.parse(JSON.stringify(value));

const resetCreateForm = () => {
  Object.assign(createForm, createDefaultForm());
  selectedTemplateName.value = '';
  selectedPoolId.value = '';
  activeSections.value = ['base', 'mode'];
  resetFieldErrors();
};

const fieldSectionMap: Record<string, string> = {
  name: 'base',
  cidr: 'base',
  gateway: 'base',
  location: 'base',
  dhcpMode: 'mode',
  raMode: 'mode',
  allocationMode: 'mode',
  priorityWeight: 'mode',
  leaseTime: 'lease',
  maxLeaseTime: 'lease',
  dnsPrimary: 'dns',
  dnsSecondary: 'dns',
  domainSearch: 'dns',
  pdPrefixLength: 'pd',
  pdMaxDepth: 'pd',
  pdConcurrent: 'pd',
  warnThreshold: 'alert'
};

const ensureSectionOpened = (field: string) => {
  const section = fieldSectionMap[field];
  if (!section) return;
  if (!activeSections.value.includes(section)) {
    activeSections.value = [...activeSections.value, section];
  }
};

const scrollToPanel = async (section: string) => {
  await nextTick();
  const panel = document.querySelector(`.ipv6-create-dialog .panel-${section}`) as HTMLElement | null;
  panel?.scrollIntoView({ behavior: 'smooth', block: 'start' });
};

const scrollToFirstError = async () => {
  await nextTick();
  const firstError = Array.from(document.querySelectorAll('.ipv6-create-dialog .field-error')).find((item) => {
    const el = item as HTMLElement;
    return !!el.offsetParent;
  }) as HTMLElement | undefined;
  firstError?.scrollIntoView({ behavior: 'smooth', block: 'center' });
};

const handleCollapseChange = async (value: string[] | string) => {
  const current = Array.isArray(value) ? value : [value];
  const active = current[current.length - 1];
  if (active) await scrollToPanel(active);
};

function loadTemplates() {
  try {
    const raw = localStorage.getItem('ipv6-pool-templates');
    templates.value = raw ? JSON.parse(raw) : [];
  } catch {
    templates.value = [];
  }
}

const readPrefixLength = (cidr: string): number | null => {
  if (!validateIPv6Prefix(cidr)) return null;
  const [, len] = cidr.split('/');
  const parsed = Number(len);
  return Number.isFinite(parsed) ? parsed : null;
};

const expandIPv6 = (ip: string): string[] | null => {
  const value = ip.trim().toLowerCase();
  if (!isIPv6(value)) return null;
  const parts = value.split('::');
  if (parts.length > 2) return null;
  const left = parts[0] ? parts[0].split(':').filter(Boolean) : [];
  const right = parts[1] ? parts[1].split(':').filter(Boolean) : [];
  const missing = 8 - left.length - right.length;
  if (missing < 0) return null;
  const body = parts.length === 1 ? left : [...left, ...Array(missing).fill('0'), ...right];
  if (body.length !== 8) return null;
  return body.map((h) => h.padStart(4, '0'));
};

const ipv6ToBigInt = (ip: string): bigint | null => {
  const expanded = expandIPv6(ip);
  if (!expanded) return null;
  let value = 0n;
  for (const item of expanded) {
    value = (value << 16n) + BigInt(parseInt(item, 16));
  }
  return value;
};

const cidrToRange = (cidr: string): { start: bigint; end: bigint } | null => {
  if (!validateIPv6Prefix(cidr)) return null;
  const [ip, prefixText] = cidr.split('/');
  const prefix = Number(prefixText);
  const address = ipv6ToBigInt(ip);
  if (address === null) return null;
  const hostBits = BigInt(128 - prefix);
  const mask = prefix === 0 ? 0n : ((1n << BigInt(prefix)) - 1n) << hostBits;
  const start = address & mask;
  const end = prefix === 128 ? start : start | ((1n << hostBits) - 1n);
  return { start, end };
};

const ipv6CidrOverlap = (a: string, b: string) => {
  const ra = cidrToRange(a);
  const rb = cidrToRange(b);
  if (!ra || !rb) return false;
  const maxStart = ra.start > rb.start ? ra.start : rb.start;
  const minEnd = ra.end < rb.end ? ra.end : rb.end;
  return maxStart <= minEnd;
};

const validateDomainList = (value: string) => {
  if (!value.trim()) return true;
  const entries = value
    .split(/[;,\s]+/)
    .map((item) => item.trim())
    .filter(Boolean);
  const domainRegex = /^(?=.{1,253}$)([a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)(\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/;
  return entries.every((entry) => domainRegex.test(entry));
};

const validateField = (field: string) => {
  clearFieldError(field);
  if (field === 'name') {
    const value = createForm.name.trim();
    if (!value) fieldErrors.name = t('pool.v6ErrName');
    else if (!/^[\w\u4e00-\u9fa5\s\-_.()/#]{1,64}$/.test(value)) {
      fieldErrors.name = t('pool.v6ErrNameFormat');
    }
    return !fieldErrors.name;
  }
  if (field === 'cidr') {
    const value = createForm.cidr.trim();
    if (!value) {
      fieldErrors.cidr = t('pool.v6ErrCidr');
      return false;
    }
    if (!validateIPv6Prefix(value)) {
      fieldErrors.cidr = t('pool.v6ErrCidrFormat');
      return false;
    }
    const conflict = allPoolOptions.value.some(
      (item) => item.id !== editingPoolId.value && item.cidr && ipv6CidrOverlap(value, item.cidr)
    );
    if (conflict) {
      fieldErrors.cidr = t('pool.v6ErrCidrConflict');
      return false;
    }
    return true;
  }
  if (field === 'gateway') {
    const value = createForm.gateway.trim();
    if (!value) return true;
    if (!isIPv6(value)) {
      fieldErrors.gateway = t('pool.v6ErrGateway');
      return false;
    }
    return true;
  }
  if (field === 'location') {
    const value = createForm.location.trim();
    if (!value) return true;
    if (!/^[\w\u4e00-\u9fa5\s\-_.()/#]{1,64}$/.test(value)) {
      fieldErrors.location = t('pool.v6ErrLocation');
      return false;
    }
    return true;
  }
  if (field === 'allocationMode') {
    if (!createForm.allocationMode) {
      fieldErrors.allocationMode = t('pool.v6ErrAllocMode');
      return false;
    }
    return true;
  }
  if (field === 'priorityWeight') {
    if (createForm.allocationMode !== 'random') return true;
    if (!Number.isFinite(createForm.priorityWeight) || createForm.priorityWeight < 1 || createForm.priorityWeight > 100) {
      fieldErrors.priorityWeight = t('pool.v6ErrWeight');
      return false;
    }
    return true;
  }
  if (field === 'leaseTime') {
    if (createForm.dhcpMode !== 'stateful') return true;
    if (!Number.isFinite(createForm.leaseTime) || createForm.leaseTime < 300 || createForm.leaseTime > 604800) {
      fieldErrors.leaseTime = t('pool.v6ErrMinLease');
      return false;
    }
    if (createForm.maxLeaseTime < createForm.leaseTime) {
      fieldErrors.leaseTime = t('pool.v6ErrMinLeaseMax');
      return false;
    }
    return true;
  }
  if (field === 'maxLeaseTime') {
    if (createForm.dhcpMode !== 'stateful') return true;
    if (!Number.isFinite(createForm.maxLeaseTime) || createForm.maxLeaseTime < 300 || createForm.maxLeaseTime > 604800) {
      fieldErrors.maxLeaseTime = t('pool.v6ErrMaxLease');
      return false;
    }
    if (createForm.maxLeaseTime < createForm.leaseTime) {
      fieldErrors.maxLeaseTime = t('pool.v6ErrMaxLeaseMin');
      return false;
    }
    return true;
  }
  if (field === 'dnsPrimary' || field === 'dnsSecondary') {
    const value = (createForm as any)[field].trim();
    if (!value) return true;
    if (!isIPv6(value)) {
      fieldErrors[field] = t('pool.v6ErrDns');
      return false;
    }
    return true;
  }
  if (field === 'domainSearch') {
    if (!validateDomainList(createForm.domainSearch)) {
      fieldErrors.domainSearch = t('pool.v6ErrDomainSearch');
      return false;
    }
    return true;
  }
  if (field === 'pdPrefixLength') {
    if (!createForm.pdEnabled) return true;
    const cidrPrefix = readPrefixLength(createForm.cidr);
    if (!Number.isFinite(createForm.pdPrefixLength) || createForm.pdPrefixLength < 48 || createForm.pdPrefixLength > 124) {
      fieldErrors.pdPrefixLength = t('pool.v6ErrPdPrefixLength');
      return false;
    }
    if (cidrPrefix !== null && createForm.pdPrefixLength < cidrPrefix) {
      fieldErrors.pdPrefixLength = t('pool.v6ErrPdPrefixLengthMin');
      return false;
    }
    return true;
  }
  if (field === 'pdMaxDepth') {
    if (!createForm.pdEnabled) return true;
    if (!Number.isFinite(createForm.pdMaxDepth) || createForm.pdMaxDepth < 1 || createForm.pdMaxDepth > 8) {
      fieldErrors.pdMaxDepth = t('pool.v6ErrPdMaxDepth');
      return false;
    }
    return true;
  }
  if (field === 'pdConcurrent') {
    if (!createForm.pdEnabled) return true;
    if (!Number.isFinite(createForm.pdConcurrent) || createForm.pdConcurrent < 1 || createForm.pdConcurrent > 200) {
      fieldErrors.pdConcurrent = t('pool.v6ErrPdConcurrent');
      return false;
    }
    return true;
  }
  if (field === 'warnThreshold') {
    if (!Number.isFinite(createForm.warnThreshold) || createForm.warnThreshold < 1 || createForm.warnThreshold > 100) {
      fieldErrors.warnThreshold = t('pool.v6ErrWarnThreshold');
      return false;
    }
    return true;
  }
  if (field === 'dhcpMode') return createForm.dhcpMode === 'stateful' || createForm.dhcpMode === 'stateless';
  if (field === 'raMode') return !!createForm.raMode;
  return true;
};

const validateAll = () => {
  const keys = [
    'name',
    'cidr',
    'gateway',
    'location',
    'allocationMode',
    'priorityWeight',
    'leaseTime',
    'maxLeaseTime',
    'dnsPrimary',
    'dnsSecondary',
    'domainSearch',
    'pdPrefixLength',
    'pdMaxDepth',
    'pdConcurrent',
    'warnThreshold'
  ];
  let firstInvalid = '';
  for (const key of keys) {
    if (!validateField(key)) {
      if (!firstInvalid) firstInvalid = key;
    }
  }
  if (firstInvalid) {
    ensureSectionOpened(firstInvalid);
    void scrollToFirstError();
    return false;
  }
  return true;
};

const getAllPoolOptions = async () => {
  const tenantId = getTenantId();
  const pageSize = 500;
  let page = 1;
  const next: Array<{ id: string; name: string; cidr: string }> = [];
  const seen = new Set<string>();
  while (page <= 200) {
    const { data } = await wrapApi(
      () =>
        listPools(
          {
            page,
            pageSize,
            version: 6,
            tenantId
          },
          undefined
        ),
      t('pool.loadFail')
    );
    const items = data.data.items || [];
    const total = data.data.total || items.length;
    for (const item of items) {
      if (!item?.id || seen.has(item.id)) continue;
      seen.add(item.id);
      next.push({ id: item.id, name: item.name || item.cidr, cidr: item.cidr || '' });
    }
    if (!items.length || items.length < pageSize || next.length >= total) break;
    page += 1;
  }
  allPoolOptions.value = next;
};

const applyForm = (value: Partial<IPv6CreateForm>) => {
  Object.assign(createForm, createDefaultForm(), value);
  resetFieldErrors();
};

const markSnapshot = () => {
  snapshot.value = JSON.stringify(createForm);
};

const saveTemplate = () => {
  const name = createForm.name.trim();
  if (!name) {
    showError(t('pool.v6SaveTemplateNameRequired'));
    return;
  }
  const next = templates.value.filter((tpl) => tpl.name !== name);
  next.unshift({ name, payload: safeClone(createForm) });
  templates.value = next.slice(0, 30);
  localStorage.setItem('ipv6-pool-templates', JSON.stringify(templates.value));
  showSuccess(t('pool.v6TemplateSaved'));
};

const applyTemplate = () => {
  if (!selectedTemplateName.value) return;
  const item = templates.value.find((tpl) => tpl.name === selectedTemplateName.value);
  if (!item) return;
  applyForm(item.payload);
  validateField('cidr');
};

const applyFromPool = async () => {
  if (!selectedPoolId.value) return;
  try {
    const { data } = await wrapApi(
      () => getPool(selectedPoolId.value, { tenantId: getTenantId() }),
      t('pool.loadFail')
    );
    const detail = data.data;
    applyForm({
      name: `${detail.name || t('pool.v6PoolDefault')}${t('pool.v6PoolCopySuffix')}`,
      cidr: detail.cidr || '',
      gateway: (detail as any).gateway || '',
      location: (detail as any).location || '',
      leaseTime: Number(detail.leaseTime || 1800),
      maxLeaseTime: Number(detail.maxLeaseTime || 7200),
      allocationMode: detail.strategy?.mode || 'round-robin',
      dnsPrimary: Array.isArray(detail.dns) ? detail.dns[0] || '' : '',
      dnsSecondary: Array.isArray(detail.dns) ? detail.dns[1] || '' : ''
    });
    validateField('cidr');
  } catch (error) {
    console.error(error);
  }
};

const runPrecheck = () => {
  if (!validateAll()) {
    showError(t('pool.v6PrecheckFail'));
    return;
  }
  showSuccess(t('pool.v6PrecheckPass'));
};

const openPreview = () => {
  if (!validateAll()) {
    showError(t('pool.v6PreviewValidation'));
    return;
  }
  previewVisible.value = true;
};

const requestCloseCreateDialog = async () => {
  if (!isDirty.value || createPending.value) {
    createVisible.value = false;
    return;
  }
  try {
    await ElMessageBox.confirm(t('pool.v6UnsavedConfirm'), t('pool.v6UnsavedTitle'), {
      type: 'warning',
      confirmButtonText: t('pool.v6ConfirmClose'),
      cancelButtonText: t('pool.v6ContinueEdit')
    });
    createVisible.value = false;
  } catch {
    return;
  }
};

const handleBeforeCreateClose = async (done: () => void) => {
  if (!isDirty.value || createPending.value) {
    done();
    return;
  }
  try {
    await ElMessageBox.confirm(t('pool.v6UnsavedConfirm'), t('pool.v6UnsavedTitle'), {
      type: 'warning',
      confirmButtonText: t('pool.v6ConfirmClose'),
      cancelButtonText: t('pool.v6ContinueEdit')
    });
    done();
  } catch {
    return;
  }
};

const handleCreateDialogClose = () => {
  previewVisible.value = false;
  selectedTemplateName.value = '';
  selectedPoolId.value = '';
  resetFieldErrors();
};

const openCreate = async () => {
  if (!canManage.value) return;
  editingPoolId.value = null;
  createPending.value = false;
  resetCreateForm();
  loadTemplates();
  await getAllPoolOptions();
  markSnapshot();
  createVisible.value = true;
};

const openEdit = async (row: PoolSummary) => {
  if (!canManage.value) return;
  editingPoolId.value = row.id;
  createPending.value = false;
  resetCreateForm();
  loadTemplates();
  await getAllPoolOptions();
  createVisible.value = true;
  try {
    const { data } = await wrapApi(
      () => getPool(row.id, { tenantId: getTenantId() }),
      t('pool.loadFail')
    );
    const detail = data.data;
    applyForm({
      name: detail.name || '',
      cidr: detail.cidr || '',
      gateway: (detail as any).gateway || '',
      location: (detail as any).location || '',
      leaseTime: Number(detail.leaseTime || 1800),
      maxLeaseTime: Number(detail.maxLeaseTime || 7200),
      allocationMode: detail.strategy?.mode || 'round-robin',
      dnsPrimary: Array.isArray(detail.dns) ? detail.dns[0] || '' : '',
      dnsSecondary: Array.isArray(detail.dns) ? detail.dns[1] || '' : ''
    });
    markSnapshot();
  } catch (error) {
    console.error(error);
  }
};

const handleDelete = async (row: PoolSummary) => {
  if (!canManage.value) return;
  try {
    const tenantId = getTenantId();
    await wrapApi(() => deletePool(row.id, { tenantId }), t('pool.deleteFail'));
    showSuccess(t('pool.deleted'));
    selectedIds.value = selectedIds.value.filter((id) => id !== row.id);
    if (pagination.page > 1 && pools.value.length <= 1) {
      pagination.page -= 1;
    }
    await fetchPools();
  } catch (error) {
    console.error(error);
  }
};

const handleToggleStatus = async (row: PoolSummary) => {
  if (!canManage.value) return;
  const tenantId = getTenantId();
  const nextStatus: PoolSummary['status'] = row.status === 'active' ? 'disabled' : 'active';
  try {
    await wrapApi(
      () => updatePool(row.id, { status: nextStatus, tenantId }),
      t('pool.saveFail')
    );
    showSuccess(nextStatus === 'active' ? t('pool.v6Enabled') : t('pool.v6Disabled'));
    await fetchPools();
  } catch (error) {
    console.error(error);
  }
};

const handleSubmit = async () => {
  if (!canManage.value || createPending.value) return;
  if (!validateAll()) {
    showError(t('pool.v6SubmitError'));
    return;
  }
  createPending.value = true;
  try {
    const tenantId = getTenantId();
    const dns = [createForm.dnsPrimary, createForm.dnsSecondary].map((item) => item.trim()).filter(Boolean);
    const payload: SubnetDraft & { tenantId?: string; version?: 4 | 6 } = {
      name: createForm.name.trim(),
      cidr: createForm.cidr.trim(),
      gateway: createForm.gateway.trim() || undefined,
      dns,
      leaseTime: createForm.leaseTime,
      maxLeaseTime: createForm.maxLeaseTime,
      exclude: [],
      strategy: { mode: createForm.allocationMode },
      vlanId: undefined,
      location: createForm.location.trim() || undefined,
      tags: []
    };
    if (editingPoolId.value) {
      await wrapApi(
        () => updatePool(editingPoolId.value!, { ...payload, tenantId }),
        t('pool.saveFail')
      );
      showSuccess(t('pool.updated'));
    } else {
      await wrapApi(() => createPool({ ...payload, version: 6, tenantId }), t('pool.saveFail'));
      showSuccess(t('pool.created'));
    }
    createVisible.value = false;
    handleCreateDialogClose();
    editingPoolId.value = null;
    await fetchPools();
  } catch (error) {
    const message = getApiError(error)?.message || t('pool.saveFail');
    showError(message);
    console.error(error);
  } finally {
    createPending.value = false;
  }
};

const splitCsv = (content: string) => {
  const rows: string[][] = [];
  let current = '';
  let inQuotes = false;
  let row: string[] = [];
  for (let i = 0; i < content.length; i += 1) {
    const ch = content[i];
    if (ch === '"') {
      inQuotes = !inQuotes;
      current += ch;
      continue;
    }
    if (!inQuotes && ch === ',') {
      row.push(current);
      current = '';
      continue;
    }
    if (!inQuotes && (ch === '\n' || ch === '\r')) {
      if (current !== '' || row.length) {
        row.push(current);
        rows.push(row);
      }
      current = '';
      row = [];
      continue;
    }
    current += ch;
  }
  row.push(current);
  rows.push(row);
  return rows;
};

const parseCsv = (content: string): ImportRow[] => {
  const rawRows = splitCsv(content)
    .map((r) => r.map((cell) => cell.trim()))
    .filter((r) => r.some((cell) => cell !== ''))
    .filter((r) => !(r[0] || '').startsWith('#'));
  if (rawRows.length < 2) throw new Error(t('pool.v6CsvContentError'));
  const header = rawRows[0].map((h) =>
    h
      .replace(/^\ufeff/, '')
      .split('(')[0]
      .trim()
      .toLowerCase()
  );
  const fieldMap: Record<string, keyof ImportRow> = {
    name: 'name',
    cidr: 'cidr',
    scope: 'scope',
    parentid: 'parentId',
    vlanid: 'vlanId',
    interfaceid: 'interfaceId',
    ssid: 'ssid',
    location: 'location',
    reservepercent: 'reservePercent',
    leaseprofileid: 'leaseProfileId',
    tags: 'tags',
    gateway: 'gateway',
    dns: 'dns',
    rangestart: 'rangeStart',
    rangeend: 'rangeEnd',
    exclusions: 'exclusions',
    allocationmode: 'allocationMode',
    priorityweight: 'priorityWeight',
    network: 'network',
    netmask: 'netmask'
  };
  ['name', 'cidr'].forEach((key) => {
    if (!header.includes(key)) throw new Error(t('pool.v6CsvFieldMissing', { field: key }));
  });
  const headerKeys = header.map((key) => fieldMap[key] || null);
  const rows: ImportRow[] = [];
  for (let i = 1; i < rawRows.length; i += 1) {
    const cols = rawRows[i];
    const record: ImportRow = {};
    headerKeys.forEach((mapped, idx) => {
      if (!mapped) return;
      (record as Record<string, string>)[mapped as string] = (cols[idx] || '').trim();
    });
    if (!record.name && !record.cidr) continue;
    rows.push(record);
  }
  return rows;
};

const handleImportFile = (file: any) => {
  importError.value = '';
  failedRows.value = [];
  const picked = file?.raw || file;
  importFile.value = picked instanceof File ? picked : null;
  const reader = new FileReader();
  reader.onload = () => {
    try {
      const text = String(reader.result || '');
      const parsed = parseCsv(text);
      importStats.total = parsed.length;
      importStats.success = 0;
      importStats.failed = 0;
    } catch (error: any) {
      importError.value = error?.message || t('pool.v6CsvParseError');
      importFile.value = null;
      importStats.total = 0;
    }
  };
  reader.readAsText(picked);
};

const normalizeImportResult = (payload: any) => {
  const data = payload?.data ?? payload;
  const inner = data?.data ?? data;
  return {
    total: Number(inner?.total ?? 0),
    success: Number(inner?.success ?? 0),
    failed: Number(inner?.failed ?? 0),
    errors: Array.isArray(inner?.errors) ? inner.errors : []
  } as {
    total: number;
    success: number;
    failed: number;
    errors: { row?: number; message?: string; error?: string }[];
  };
};

const submitImport = async () => {
  if (!importFile.value) return;
  importing.value = true;
  importProgress.value = 10;
  const timer = window.setInterval(() => {
    if (importProgress.value < 90) {
      importProgress.value += 8;
    }
  }, 250);
  importError.value = '';
  failedRows.value = [];
  importStats.success = 0;
  importStats.failed = 0;
  try {
    const response = await wrapApi(
      () => importPoolsCsv(importFile.value as File, { version: 6 }),
      t('pool.saveFail')
    );
    const result = normalizeImportResult(response?.data);
    importStats.total = result.total;
    importStats.success = result.success;
    importStats.failed = result.failed;
    failedRows.value = (result.errors || []).map((item) => {
      const rowNumber = Number((item as any).row ?? (item as any).Row ?? 0);
      return { row: rowNumber, error: item.message || (item as any).error || t('pool.v6ImportFail') };
    });
    if (!importStats.failed) {
      showSuccess(t('pool.v6ImportDone'));
    }
    importProgress.value = 100;
    await fetchPools();
  } catch (error: any) {
    importError.value = getApiError(error)?.message || error?.message || t('pool.v6ImportFail');
  } finally {
    window.clearInterval(timer);
    importing.value = false;
    window.setTimeout(() => {
      importProgress.value = 0;
    }, 300);
  }
};

watch(
  () => createForm.cidr,
  () => {
    if (!createVisible.value) return;
    validateField('cidr');
    validateField('pdPrefixLength');
  }
);

watch(
  () => createForm.dhcpMode,
  () => {
    if (!createVisible.value) return;
    if (createForm.dhcpMode === 'stateless') {
      clearFieldError('leaseTime');
      clearFieldError('maxLeaseTime');
    } else {
      validateField('leaseTime');
      validateField('maxLeaseTime');
    }
  }
);

watch(
  () => createForm.allocationMode,
  () => {
    if (!createVisible.value) return;
    if (createForm.allocationMode !== 'random') {
      clearFieldError('priorityWeight');
    } else {
      validateField('priorityWeight');
    }
  }
);

watch(
  () => [createForm.name, createForm.gateway, createForm.location],
  () => {
    if (!createVisible.value) return;
    validateField('name');
    validateField('gateway');
    validateField('location');
  }
);

watch(
  () => [createForm.dnsPrimary, createForm.dnsSecondary, createForm.domainSearch],
  () => {
    if (!createVisible.value) return;
    validateField('dnsPrimary');
    validateField('dnsSecondary');
    validateField('domainSearch');
  }
);

watch(
  () => [createForm.pdEnabled, createForm.pdPrefixLength, createForm.pdMaxDepth, createForm.pdConcurrent],
  () => {
    if (!createVisible.value) return;
    validateField('pdPrefixLength');
    validateField('pdMaxDepth');
    validateField('pdConcurrent');
  }
);

watch(
  () => [createForm.leaseTime, createForm.maxLeaseTime, createForm.warnThreshold, createForm.priorityWeight],
  () => {
    if (!createVisible.value) return;
    validateField('leaseTime');
    validateField('maxLeaseTime');
    validateField('warnThreshold');
    validateField('priorityWeight');
  }
);

watch(
  () => tenantStore.currentTenantId,
  () => {
    pagination.page = 1;
    selectedIds.value = [];
    fetchPools();
  }
);

onMounted(() => {
  fetchPools();
});

onBeforeUnmount(() => {
  controller?.abort();
  controller = null;
});
</script>

<style scoped>
.pool-page {
  --surface-bg: #ffffff;
  --surface-border: #e5e7eb;
  --text-primary: #111827;
  --text-secondary: #6b7280;
  --text-muted: #4b5563;
  --subtle-bg: #f9fafb;
  --chip-bg: #f3f4f6;
  --danger-text: #b91c1c;
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
  max-width: 2400px;
  margin: 0 auto;
  min-height: calc(100vh - 24px);
}

.table-card {
  width: 100%;
  min-height: calc(100vh - 140px);
}

.table-card :deep(.el-card__body) {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.page-header {
  margin-bottom: 12px;
}

.page-header h3 {
  margin: 0;
}

.filter-bar {
  min-height: 56px;
  margin-bottom: 12px;
  padding: 12px 16px;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 16px;
  border: 1px solid var(--surface-border);
  border-radius: 8px;
  background: #f5f7fa;
}

.bar-left,
.bar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.bar-left {
  min-width: 0;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;
}

.bar-right {
  justify-content: flex-end;
  white-space: nowrap;
}

.desc {
  margin: 4px 0 0;
  color: var(--text-secondary);
}

.filters {
  margin: 0;
  display: flex;
  align-items: center;
  flex-wrap: nowrap;
  gap: 16px;
  white-space: nowrap;
}

.filters :deep(.el-form-item) {
  margin-bottom: 0;
}

.action-btns :deep(.el-form-item__content) {
  display: flex;
  flex-wrap: nowrap;
  gap: 8px;
}

.filter-bar :deep(.el-input__wrapper),
.filter-bar :deep(.el-select__wrapper),
.filter-bar :deep(.el-button) {
  min-height: 32px;
  height: 32px;
  border-radius: 8px;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}

.overview-card {
  border-radius: 10px;
}

.overview-label {
  color: var(--text-secondary);
  font-size: 13px;
}

.overview-value {
  margin-top: 8px;
  font-size: 24px;
  font-weight: 700;
  color: var(--text-primary);
}

.progress-wrap {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 12px;
}

.progress-item {
  display: grid;
  grid-template-columns: 80px 1fr;
  align-items: center;
  gap: 10px;
}

.selection-tools {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  background: var(--subtle-bg);
  border: 1px solid var(--surface-border);
  border-radius: 8px;
  margin-bottom: 12px;
}

.selection-actions {
  display: flex;
  gap: 8px;
}

.selection-info {
  color: var(--text-muted);
}

.ipv6-create-dialog :deep(.el-dialog) {
  width: 720px;
  border-radius: 8px;
}

.ipv6-create-dialog :deep(.el-dialog__header) {
  padding: 24px 32px 8px;
}

.ipv6-create-dialog :deep(.el-dialog__body) {
  padding: 0 32px 0;
}

.ipv6-create-dialog :deep(.el-dialog__footer) {
  padding: 12px 32px 24px;
}

.dialog-title {
  font-size: 16px;
  font-weight: 700;
  line-height: 24px;
  color: var(--text-primary);
}

.ipv6-dialog-body {
  min-height: 850px;
  max-height: calc(100vh - 200px);
  overflow-y: auto;
  padding-bottom: 8px;
}

.template-toolbar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
}

.mid-select {
  width: 200px;
}

.group-collapse :deep(.el-collapse-item__header) {
  font-weight: 600;
  font-size: 14px;
}

.group-collapse :deep(.el-collapse-item__wrap) {
  border-bottom: 1px solid var(--surface-border);
}

.group-collapse :deep(.el-collapse-item__content) {
  padding: 12px 0 18px;
}

.form-row {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 14px;
}

.form-row.two-col {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.col-item {
  display: flex;
  gap: 12px;
}

.form-label {
  width: 120px;
  min-width: 120px;
  text-align: right;
  line-height: 32px;
  color: var(--text-primary);
  font-size: 13px;
}

.form-control {
  flex: 1;
  min-width: 0;
}

.form-control :deep(.el-input),
.form-control :deep(.el-select),
.form-control :deep(.el-input-number) {
  width: 100%;
}

.inline-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.narrow-select {
  width: 150px;
}

.with-top-gap {
  margin-top: 8px;
}

.required {
  color: var(--el-color-danger);
  margin-right: 4px;
}

.optional {
  color: var(--text-secondary);
  font-size: 12px;
}

.field-error {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--el-color-danger);
}

.alloc-tabs :deep(.el-tabs__item) {
  min-width: 88px;
  text-align: center;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.preview-desc {
  margin-top: 6px;
}

.form-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.panel-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 8px;
}

.mt-12 {
  margin-top: 12px;
}

.mb-12 {
  margin-bottom: 12px;
}

.mt-8 {
  margin-top: 8px;
}

.import-body .tip {
  color: var(--text-muted);
  margin-bottom: 12px;
}

.field-tip {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.field-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 10px;
}

.field-list span {
  padding: 2px 8px;
  background: var(--chip-bg);
  border-radius: 4px;
  font-size: 12px;
}

.fail-list {
  padding: 4px 0;
  color: var(--danger-text);
  font-size: 13px;
}

@media (max-width: 1200px) {
  .filter-bar {
    grid-template-columns: 1fr;
    gap: 12px;
  }

  .bar-right {
    justify-content: flex-start;
  }

  .overview-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .selection-tools {
    flex-direction: column;
    align-items: flex-start;
  }
}

@media (max-width: 768px) {
  .ipv6-create-dialog :deep(.el-dialog) {
    width: calc(100vw - 24px) !important;
  }

  .ipv6-dialog-body {
    min-height: 680px;
    max-height: calc(100vh - 160px);
  }

  .template-toolbar {
    align-items: stretch;
  }

  .mid-select,
  .narrow-select {
    width: 100%;
  }

  .form-row,
  .col-item {
    flex-direction: column;
    gap: 6px;
  }

  .form-row.two-col {
    grid-template-columns: 1fr;
  }

  .form-label {
    width: 100%;
    min-width: 100%;
    text-align: left;
    line-height: 20px;
  }

  .overview-grid {
    grid-template-columns: 1fr;
  }
}

</style>
