<template>
  <div class="page surface-card">
    <el-card shadow="never" class="module-card">
      <div class="module-header">
        <div>
          <div class="module-title">{{ t('system.migration.pageTitle') }}</div>
          <div class="module-subtitle">{{ t('system.migration.pageSubtitle') }}</div>
        </div>
        <div v-if="hasParsedResult" class="summary-tags">
          <el-tag type="info">{{ t('system.migration.tagSubnets', { n: subnetRows.length }) }}</el-tag>
          <el-tag type="success">{{ t('system.migration.tagReservations', { n: reservationRows.length }) }}</el-tag>
          <el-tag type="warning">{{ t('system.migration.tagDenied', { n: deniedRows.length }) }}</el-tag>
          <el-tag :type="exceptionCount > 0 ? 'danger' : 'success'">{{ t('system.migration.tagExceptions', { n: exceptionCount }) }}</el-tag>
        </div>
      </div>
    </el-card>

    <el-card shadow="never" class="module-card">
      <template #header>
        <div class="section-title">{{ t('system.migration.sectionUpload') }}</div>
      </template>

      <div v-if="parseState === 'idle'" class="upload-idle">
        <el-upload
          drag
          :auto-upload="false"
          :show-file-list="false"
          accept=".conf,.txt"
          :on-change="handleIscUpload"
        >
          <el-icon class="el-icon--upload"><UploadFilled /></el-icon>
          <div class="el-upload__text">{{ t('system.migration.uploadDrag') }}</div>
          <div class="el-upload__tip">{{ t('system.migration.uploadTip') }}</div>
        </el-upload>
      </div>

      <div v-else-if="parseState === 'parsing'" class="parse-loading">
        <el-progress :percentage="parseProgress" :status="parseProgress === 100 ? 'success' : undefined" />
        <div class="loading-tip">{{ t('system.migration.parsingTip') }}</div>
      </div>

      <div v-else-if="parseState === 'success'" class="parse-success">
        <el-descriptions :column="3" border>
          <el-descriptions-item :label="t('system.migration.descFileName')">{{ parseMeta.fileName || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.migration.descParseStatus')">
            <el-tag type="success">{{ t('system.migration.parseOk') }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="t('system.migration.descCompleted')">{{ formatTime(parseMeta.completedAt) }}</el-descriptions-item>
        </el-descriptions>
        <div class="inline-actions">
          <el-button type="primary" @click="resetToUpload">{{ t('system.migration.btnReupload') }}</el-button>
          <el-button @click="openLogDialog">{{ t('system.migration.btnViewLog') }}</el-button>
        </div>
      </div>

      <div v-else class="parse-failed">
        <el-alert type="error" :closable="false" show-icon>
          <template #title>
            {{ t('system.migration.parseFail', { reason: parseError.reason || t('system.migration.parseFailDefault') }) }}
          </template>
          <div class="error-detail">
            <div>{{ t('system.migration.errorPos', { pos: parseError.position || t('system.migration.errorPosDefault') }) }}</div>
            <div>{{ t('system.migration.errorGuidance', { guide: parseError.guidance || t('system.migration.errorGuidanceDefault') }) }}</div>
          </div>
        </el-alert>
        <div class="inline-actions">
          <el-button type="primary" @click="resetToUpload">{{ t('system.migration.btnReupload') }}</el-button>
          <el-button @click="openLogDialog">{{ t('system.migration.btnViewLog') }}</el-button>
        </div>
      </div>
    </el-card>

    <el-card v-if="hasParsedResult" shadow="never" class="module-card">
      <template #header>
        <div class="section-title">{{ t('system.migration.sectionResult') }}</div>
      </template>

      <el-collapse v-model="activePanels">
        <el-collapse-item name="subnets">
          <template #title>
            <div class="panel-title-wrap">
              <span class="panel-title">{{ t('system.migration.panelSubnets') }}</span>
              <el-tag type="info" size="small">{{ subnetRows.length }}</el-tag>
            </div>
          </template>
          <div class="panel-toolbar">
            <el-input v-model="subnetKeyword" :placeholder="t('system.migration.searchSubnet')" clearable class="w-260" />
            <div class="toolbar-right">
              <el-button @click="selectAllSubnets">{{ t('system.migration.selectAll') }}</el-button>
              <el-button @click="invertSubnets">{{ t('system.migration.invertSelection') }}</el-button>
              <el-button @click="exportSubnetsCsv(false)">{{ t('system.migration.exportCsv') }}</el-button>
            </div>
          </div>
          <el-table
            ref="subnetTableRef"
            :data="pagedSubnets"
            border
            stripe
            class="data-table"
            :row-class-name="subnetRowClass"
            @selection-change="onSubnetSelectionChange"
          >
            <el-table-column type="selection" width="52" :selectable="isSubnetSelectable" />
            <el-table-column :label="t('system.migration.colParseStatus')" width="160">
              <template #default="{ row }">
                <el-tag :type="statusTagType(row.parseStatus)">{{ parseStatusLabel(row.parseStatus) }}</el-tag>
                <div v-if="row.parseReason" class="reason">{{ row.parseReason }}</div>
              </template>
            </el-table-column>
            <el-table-column prop="cidr" :label="t('system.migration.colCidr')" min-width="150" />
            <el-table-column prop="gateway" :label="t('system.migration.colGateway')" min-width="130" />
            <el-table-column :label="t('system.migration.colDns')" min-width="180">
              <template #default="{ row }">{{ (row.dns || []).join(', ') || '--' }}</template>
            </el-table-column>
            <el-table-column :label="t('system.migration.colPool')" min-width="240">
              <template #default="{ row }">
                {{ row.rangesText || '--' }}
              </template>
            </el-table-column>
            <el-table-column :label="t('system.migration.colExclusions')" min-width="200">
              <template #default="{ row }">{{ row.exclusionsText || '--' }}</template>
            </el-table-column>
            <el-table-column :label="t('system.migration.colOptions')" min-width="220" show-overflow-tooltip>
              <template #default="{ row }">{{ row.optionsText || '--' }}</template>
            </el-table-column>
          </el-table>
          <div class="pager-row">
            <el-pagination
              v-model:current-page="subnetPage"
              v-model:page-size="subnetPageSize"
              :total="filteredSubnets.length"
              layout="total, sizes, prev, pager, next, jumper"
              :page-sizes="[10, 20, 50, 100]"
            />
          </div>
        </el-collapse-item>

        <el-collapse-item name="reservations">
          <template #title>
            <div class="panel-title-wrap">
              <span class="panel-title">{{ t('system.migration.panelReservations') }}</span>
              <el-tag type="info" size="small">{{ reservationRows.length }}</el-tag>
            </div>
          </template>
          <div class="panel-toolbar">
            <el-input v-model="reservationKeyword" :placeholder="t('system.migration.searchReservation')" clearable class="w-260" />
            <div class="toolbar-right">
              <el-button @click="selectAllReservations">{{ t('system.migration.selectAll') }}</el-button>
              <el-button @click="invertReservations">{{ t('system.migration.invertSelection') }}</el-button>
              <el-button @click="exportReservationsCsv(false)">{{ t('system.migration.exportCsv') }}</el-button>
            </div>
          </div>
          <el-table
            ref="reservationTableRef"
            :data="pagedReservations"
            border
            stripe
            class="data-table"
            :row-class-name="reservationRowClass"
            @selection-change="onReservationSelectionChange"
          >
            <el-table-column type="selection" width="52" :selectable="isReservationSelectable" />
            <el-table-column :label="t('system.migration.colParseStatus')" width="180">
              <template #default="{ row }">
                <el-tag :type="statusTagType(row.parseStatus)">{{ parseStatusLabel(row.parseStatus) }}</el-tag>
                <div v-if="row.parseReason" class="reason">{{ row.parseReason }}</div>
              </template>
            </el-table-column>
            <el-table-column prop="hostname" :label="t('system.migration.colHostname')" min-width="150" />
            <el-table-column prop="mac" :label="t('system.migration.colMac')" min-width="180" />
            <el-table-column prop="ip" :label="t('system.migration.colIp')" min-width="150" />
          </el-table>
          <div class="pager-row">
            <el-pagination
              v-model:current-page="reservationPage"
              v-model:page-size="reservationPageSize"
              :total="filteredReservations.length"
              layout="total, sizes, prev, pager, next, jumper"
              :page-sizes="[10, 20, 50, 100]"
            />
          </div>
        </el-collapse-item>

        <el-collapse-item name="denied">
          <template #title>
            <div class="panel-title-wrap">
              <span class="panel-title">{{ t('system.migration.panelDenied') }}</span>
              <el-tag type="info" size="small">{{ deniedRows.length }}</el-tag>
            </div>
          </template>
          <div class="panel-toolbar">
            <el-input v-model="deniedKeyword" :placeholder="t('system.migration.searchDenied')" clearable class="w-260" />
            <div class="toolbar-right">
              <el-button @click="selectAllDenied">{{ t('system.migration.selectAll') }}</el-button>
              <el-button @click="invertDenied">{{ t('system.migration.invertSelection') }}</el-button>
              <el-button @click="exportDeniedHostsCsv(false)">{{ t('system.migration.exportCsv') }}</el-button>
            </div>
          </div>
          <el-table
            ref="deniedTableRef"
            :data="pagedDenied"
            border
            stripe
            class="data-table"
            :row-class-name="deniedRowClass"
            @selection-change="onDeniedSelectionChange"
          >
            <el-table-column type="selection" width="52" :selectable="isDeniedSelectable" />
            <el-table-column :label="t('system.migration.colParseStatus')" width="180">
              <template #default="{ row }">
                <el-tag :type="statusTagType(row.parseStatus)">{{ parseStatusLabel(row.parseStatus) }}</el-tag>
                <div v-if="row.parseReason" class="reason">{{ row.parseReason }}</div>
              </template>
            </el-table-column>
            <el-table-column prop="hostname" :label="t('system.migration.colHostname')" min-width="160" />
            <el-table-column prop="mac" :label="t('system.migration.colMac')" min-width="190" />
            <el-table-column prop="reason" :label="t('system.migration.colReason')" min-width="200" />
          </el-table>
          <div class="pager-row">
            <el-pagination
              v-model:current-page="deniedPage"
              v-model:page-size="deniedPageSize"
              :total="filteredDenied.length"
              layout="total, sizes, prev, pager, next, jumper"
              :page-sizes="[10, 20, 50, 100]"
            />
          </div>
        </el-collapse-item>
      </el-collapse>
    </el-card>

    <el-card v-if="hasParsedResult" shadow="never" class="module-card">
      <template #header>
        <div class="section-title">{{ t('system.migration.sectionActions') }}</div>
      </template>
      <div class="flow-actions">
        <el-button type="primary" :loading="validateLoading" @click="validateSelection">{{ t('system.migration.btnValidate') }}</el-button>
        <el-button :disabled="selectedTotalCount === 0" @click="exportSelectedCsv">{{ t('system.migration.btnExportSelected') }}</el-button>
        <el-button :disabled="!canStartImport" type="primary" :loading="importLoading" @click="confirmImport"
          >{{ t('system.migration.btnStartImport') }}</el-button
        >
        <el-button @click="resetAll">{{ t('system.migration.btnCancelReset') }}</el-button>
        <el-button v-if="lastImportReport" type="warning" plain @click="rollbackImport">{{ t('system.migration.btnRollback') }}</el-button>
      </div>
      <el-alert
        v-if="validationReport"
        :type="validationReport.pass ? 'success' : 'error'"
        :closable="false"
        class="mt-12"
      >
        <template #title>
          {{ validationReport.pass ? t('system.migration.valPass') : t('system.migration.valFail') }}
        </template>
        <div>
          {{ t('system.migration.valSummary', { total: validationReport.total, invalid: validationReport.invalid, conflict: validationReport.conflict }) }}
        </div>
      </el-alert>
      <el-progress v-if="importLoading" class="mt-12" :percentage="importProgress" />
      <el-alert v-if="lastImportReport" type="success" :closable="false" class="mt-12">
        <template #title>{{ t('system.migration.importReport') }}</template>
        <div>{{ lastImportReport }}</div>
      </el-alert>
    </el-card>

    <el-card shadow="never" class="module-card">
      <template #header>
        <div class="section-title">{{ t('system.migration.sectionHelp') }}</div>
      </template>
      <el-collapse>
        <el-collapse-item :title="t('system.migration.checklistTitle')" name="checklist">
          <el-timeline>
            <el-timeline-item v-for="item in checklist" :key="item.title" :timestamp="item.stage">
              <div class="item-title">{{ item.title }}</div>
              <p class="item-desc">{{ item.desc }}</p>
              <ul class="item-list">
                <li v-for="tip in item.tips" :key="tip">{{ tip }}</li>
              </ul>
            </el-timeline-item>
          </el-timeline>
        </el-collapse-item>
        <el-collapse-item :title="t('system.migration.mappingTitle')" name="mapping">
          <el-table :data="mapping" border>
            <el-table-column prop="source" :label="t('system.migration.mapColSource')" min-width="150" />
            <el-table-column prop="target" :label="t('system.migration.mapColTarget')" min-width="150" />
            <el-table-column prop="notes" :label="t('system.migration.mapColNotes')" min-width="260" />
          </el-table>
        </el-collapse-item>
        <el-collapse-item :title="t('system.migration.bestTitle')" name="best">
          <ul class="item-list">
            <li>{{ t('system.migration.bestTip1') }}</li>
            <li>{{ t('system.migration.bestTip2') }}</li>
            <li>{{ t('system.migration.bestTip3') }}</li>
          </ul>
        </el-collapse-item>
      </el-collapse>
    </el-card>

    <el-dialog v-model="logDialogVisible" :title="t('system.migration.logTitle')" width="700px">
      <el-alert
        v-if="iscResult?.warnings?.length"
        type="warning"
        :closable="false"
:title="t('system.migration.logWarnTitle')"
      >
        <ul class="item-list">
          <li v-for="w in iscResult?.warnings || []" :key="w">{{ w }}</li>
        </ul>
      </el-alert>
      <el-empty v-else :description="t('system.migration.logEmpty')" />
      <template #footer>
        <el-button @click="logDialogVisible = false">{{ t('system.migration.logClose') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { UploadFilled } from '@element-plus/icons-vue';
import { ElMessageBox } from 'element-plus';
import { importIscDhcp, validateMigrationSelection, startMigrationImport, rollbackMigrationImport } from '@/api/tools';
import type { ISCImportResult } from '@/api/tools';
import { useI18n } from 'vue-i18n';
import { showHttpError } from '@/shared/errors/errorToast';
import { showSuccess, showWarning } from '@/shared/errors/messageToast';

const { t } = useI18n();

type ParseState = 'idle' | 'parsing' | 'success' | 'failed';
type ParseStatus = 'ok' | 'error' | 'conflict';

interface ParsedSubnetRow {
  id: string;
  cidr: string;
  gateway?: string;
  dns?: string[];
  rangesText: string;
  exclusionsText: string;
  optionsText: string;
  parseStatus: ParseStatus;
  parseReason: string;
  selectable: boolean;
}

interface ParsedReservationRow {
  id: string;
  hostname?: string;
  mac?: string;
  ip?: string;
  parseStatus: ParseStatus;
  parseReason: string;
  selectable: boolean;
}

interface ParsedDeniedRow {
  id: string;
  hostname?: string;
  mac?: string;
  reason?: string;
  parseStatus: ParseStatus;
  parseReason: string;
  selectable: boolean;
}

const checklist = computed(() => [
  {
    stage: t('system.migration.checkStage1'),
    title: t('system.migration.checkTitle1'),
    desc: t('system.migration.checkDesc1'),
    tips: [t('system.migration.checkTip1a'), t('system.migration.checkTip1b'), t('system.migration.checkTip1c')]
  },
  {
    stage: t('system.migration.checkStage2'),
    title: t('system.migration.checkTitle2'),
    desc: t('system.migration.checkDesc2'),
    tips: [t('system.migration.checkTip2a'), t('system.migration.checkTip2b'), t('system.migration.checkTip2c')]
  },
  {
    stage: t('system.migration.checkStage3'),
    title: t('system.migration.checkTitle3'),
    desc: t('system.migration.checkDesc3'),
    tips: [t('system.migration.checkTip3a'), t('system.migration.checkTip3b'), t('system.migration.checkTip3c')]
  }
]);

const mapping = computed(() => [
  { source: t('system.migration.mapRow1Source'), target: t('system.migration.mapRow1Target'), notes: t('system.migration.mapRow1Notes') },
  { source: t('system.migration.mapRow2Source'), target: t('system.migration.mapRow2Target'), notes: t('system.migration.mapRow2Notes') },
  { source: t('system.migration.mapRow3Source'), target: t('system.migration.mapRow3Target'), notes: t('system.migration.mapRow3Notes') },
  { source: t('system.migration.mapRow4Source'), target: t('system.migration.mapRow4Target'), notes: t('system.migration.mapRow4Notes') }
]);

const parseState = ref<ParseState>('idle');
const parseProgress = ref(0);
const parseMeta = reactive({ fileName: '', completedAt: '' });
const parseError = reactive({ position: '', reason: '', guidance: '' });
const iscResult = ref<ISCImportResult | null>(null);

const logDialogVisible = ref(false);
const activePanels = ref<string[]>(['subnets', 'reservations', 'denied']);

const subnetTableRef = ref<any>();
const reservationTableRef = ref<any>();
const deniedTableRef = ref<any>();

const subnetKeyword = ref('');
const reservationKeyword = ref('');
const deniedKeyword = ref('');

const subnetPage = ref(1);
const subnetPageSize = ref(10);
const reservationPage = ref(1);
const reservationPageSize = ref(10);
const deniedPage = ref(1);
const deniedPageSize = ref(10);

const selectedSubnets = ref<ParsedSubnetRow[]>([]);
const selectedReservations = ref<ParsedReservationRow[]>([]);
const selectedDenied = ref<ParsedDeniedRow[]>([]);

const validateLoading = ref(false);
const importLoading = ref(false);
const importProgress = ref(0);
const validationReport = ref<{ pass: boolean; total: number; invalid: number; conflict: number } | null>(null);
const lastImportReport = ref('');
const rollbackToken = ref('');

const hasParsedResult = computed(() => !!iscResult.value && parseState.value === 'success');

const subnetRows = computed<ParsedSubnetRow[]>(() => {
  const source = iscResult.value?.subnets || [];
  const cidrCount = new Map<string, number>();
  source.forEach((s) => {
    const cidr = String(s.cidr || '').trim();
    if (!cidr) return;
    cidrCount.set(cidr, (cidrCount.get(cidr) || 0) + 1);
  });
  return source.map((s, index) => {
    const ranges = s.ranges || (s.rangeStart ? [{ start: s.rangeStart, end: s.rangeEnd || '' }] : []);
    const rangesText = ranges.map((r: any) => `${r.start || ''}-${r.end || ''}`).filter(Boolean).join('; ');
    const exclusionsText = (s.exclusions || []).join('; ');
    const optionsText = s.options
      ? Object.entries(s.options)
          .map(([key, value]) => `${key}: ${value}`)
          .join('; ')
      : '';

    const cidr = String(s.cidr || '').trim();
    let parseStatus: ParseStatus = 'ok';
    let parseReason = '';
    if (!cidr || !rangesText) {
      parseStatus = 'error';
      parseReason = t('system.migration.errCidrMissing');
    } else if ((cidrCount.get(cidr) || 0) > 1) {
      parseStatus = 'conflict';
      parseReason = t('system.migration.errCidrConflict');
    }

    return {
      id: `subnet-${index}`,
      cidr,
      gateway: s.gateway || '',
      dns: s.dns || [],
      rangesText,
      exclusionsText,
      optionsText,
      parseStatus,
      parseReason,
      selectable: parseStatus === 'ok'
    };
  });
});

const deniedMacSet = computed(() => new Set(deniedRows.value.map((d) => normalizeMac(d.mac || '')).filter(Boolean)));

const reservationRows = computed<ParsedReservationRow[]>(() => {
  const source = iscResult.value?.reservations || [];
  const ipCount = new Map<string, number>();
  const macCount = new Map<string, number>();

  source.forEach((r) => {
    const ip = String(r.ip || '').trim();
    const mac = normalizeMac(r.mac || '');
    if (ip) ipCount.set(ip, (ipCount.get(ip) || 0) + 1);
    if (mac) macCount.set(mac, (macCount.get(mac) || 0) + 1);
  });

  return source.map((r, index) => {
    const ip = String(r.ip || '').trim();
    const mac = normalizeMac(r.mac || '');
    let parseStatus: ParseStatus = 'ok';
    let parseReason = '';

    if (!ip || !mac) {
      parseStatus = 'error';
      parseReason = t('system.migration.errMacIpMissing');
    } else if (!isValidIPv4(ip)) {
      parseStatus = 'error';
      parseReason = t('system.migration.errIpInvalid');
    } else if (!isValidMac(mac)) {
      parseStatus = 'error';
      parseReason = t('system.migration.errMacInvalid');
    } else if ((ipCount.get(ip) || 0) > 1 || (macCount.get(mac) || 0) > 1) {
      parseStatus = 'conflict';
      parseReason = t('system.migration.errIpMacConflict');
    } else if (deniedMacSet.value.has(mac)) {
      parseStatus = 'conflict';
      parseReason = t('system.migration.errMacDenied');
    }

    return {
      id: `reservation-${index}`,
      hostname: r.hostname || '',
      mac,
      ip,
      parseStatus,
      parseReason,
      selectable: parseStatus === 'ok'
    };
  });
});

const deniedRows = computed<ParsedDeniedRow[]>(() => {
  const source = iscResult.value?.deniedHosts || [];
  const macCount = new Map<string, number>();
  source.forEach((d) => {
    const mac = normalizeMac(d.mac || '');
    if (mac) macCount.set(mac, (macCount.get(mac) || 0) + 1);
  });

  return source.map((d, index) => {
    const mac = normalizeMac(d.mac || '');
    let parseStatus: ParseStatus = 'ok';
    let parseReason = '';
    if (!mac) {
      parseStatus = 'error';
      parseReason = t('system.migration.errMacMissing');
    } else if (!isValidMac(mac)) {
      parseStatus = 'error';
      parseReason = t('system.migration.errMacInvalid');
    } else if ((macCount.get(mac) || 0) > 1) {
      parseStatus = 'conflict';
      parseReason = t('system.migration.errMacDupConflict');
    }

    return {
      id: `denied-${index}`,
      hostname: d.hostname || '',
      mac,
      reason: d.reason || 'deny booting',
      parseStatus,
      parseReason,
      selectable: parseStatus === 'ok'
    };
  });
});

const exceptionCount = computed(() => {
  const s = subnetRows.value.filter((r) => r.parseStatus !== 'ok').length;
  const r = reservationRows.value.filter((row) => row.parseStatus !== 'ok').length;
  const d = deniedRows.value.filter((row) => row.parseStatus !== 'ok').length;
  const warnings = iscResult.value?.warnings?.length || 0;
  return s + r + d + warnings;
});

const filteredSubnets = computed(() => {
  const q = subnetKeyword.value.trim().toLowerCase();
  if (!q) return subnetRows.value;
  return subnetRows.value.filter((row) => {
    const text = [row.cidr, row.gateway, (row.dns || []).join(' '), row.rangesText, row.optionsText]
      .join(' ')
      .toLowerCase();
    return text.includes(q);
  });
});

const filteredReservations = computed(() => {
  const q = reservationKeyword.value.trim().toLowerCase();
  if (!q) return reservationRows.value;
  return reservationRows.value.filter((row) => {
    const text = [row.hostname, row.mac, row.ip].join(' ').toLowerCase();
    return text.includes(q);
  });
});

const filteredDenied = computed(() => {
  const q = deniedKeyword.value.trim().toLowerCase();
  if (!q) return deniedRows.value;
  return deniedRows.value.filter((row) => {
    const text = [row.hostname, row.mac, row.reason].join(' ').toLowerCase();
    return text.includes(q);
  });
});

const pagedSubnets = computed(() => {
  const start = (subnetPage.value - 1) * subnetPageSize.value;
  return filteredSubnets.value.slice(start, start + subnetPageSize.value);
});

const pagedReservations = computed(() => {
  const start = (reservationPage.value - 1) * reservationPageSize.value;
  return filteredReservations.value.slice(start, start + reservationPageSize.value);
});

const pagedDenied = computed(() => {
  const start = (deniedPage.value - 1) * deniedPageSize.value;
  return filteredDenied.value.slice(start, start + deniedPageSize.value);
});

const selectedTotalCount = computed(
  () => selectedSubnets.value.length + selectedReservations.value.length + selectedDenied.value.length
);

const canStartImport = computed(() => {
  return !!validationReport.value?.pass && selectedTotalCount.value > 0 && !importLoading.value;
});

const buildSelectionPayload = () => ({
  subnets: selectedSubnets.value.map((item) => ({
    id: item.id,
    cidr: item.cidr,
    gateway: item.gateway,
    parseStatus: item.parseStatus,
    parseReason: item.parseReason
  })),
  reservations: selectedReservations.value.map((item) => ({
    id: item.id,
    hostname: item.hostname,
    mac: item.mac,
    ip: item.ip,
    parseStatus: item.parseStatus,
    parseReason: item.parseReason
  })),
  deniedHosts: selectedDenied.value.map((item) => ({
    id: item.id,
    hostname: item.hostname,
    mac: item.mac,
    reason: item.reason,
    parseStatus: item.parseStatus,
    parseReason: item.parseReason
  }))
});

const parseStatusLabel = (status: ParseStatus) => {
  if (status === 'ok') return t('system.migration.statusOk');
  if (status === 'conflict') return t('system.migration.statusConflict');
  return t('system.migration.statusError');
};

const statusTagType = (status: ParseStatus) => {
  if (status === 'ok') return 'success';
  if (status === 'conflict') return 'warning';
  return 'danger';
};

const rowClassByStatus = (status: ParseStatus) => (status === 'ok' ? '' : 'error-row');

const subnetRowClass = ({ row }: { row: ParsedSubnetRow }) => rowClassByStatus(row.parseStatus);
const reservationRowClass = ({ row }: { row: ParsedReservationRow }) => rowClassByStatus(row.parseStatus);
const deniedRowClass = ({ row }: { row: ParsedDeniedRow }) => rowClassByStatus(row.parseStatus);
const isSubnetSelectable = (row: ParsedSubnetRow) => row.selectable;
const isReservationSelectable = (row: ParsedReservationRow) => row.selectable;
const isDeniedSelectable = (row: ParsedDeniedRow) => row.selectable;

const onSubnetSelectionChange = (rows: ParsedSubnetRow[]) => {
  selectedSubnets.value = rows;
};
const onReservationSelectionChange = (rows: ParsedReservationRow[]) => {
  selectedReservations.value = rows;
};
const onDeniedSelectionChange = (rows: ParsedDeniedRow[]) => {
  selectedDenied.value = rows;
};

const selectRows = (tableRef: any, rows: Array<{ selectable: boolean }>) => {
  if (!tableRef?.clearSelection) return;
  tableRef.clearSelection();
  rows.forEach((row: any) => {
    if (row.selectable) tableRef.toggleRowSelection(row, true);
  });
};

const invertRows = (tableRef: any, rows: Array<{ id: string; selectable: boolean }>, selected: Array<{ id: string }>) => {
  if (!tableRef?.clearSelection) return;
  const selectedSet = new Set(selected.map((item) => item.id));
  tableRef.clearSelection();
  rows.forEach((row: any) => {
    if (!row.selectable) return;
    tableRef.toggleRowSelection(row, !selectedSet.has(row.id));
  });
};

const selectAllSubnets = () => selectRows(subnetTableRef.value, pagedSubnets.value);
const invertSubnets = () => invertRows(subnetTableRef.value, pagedSubnets.value, selectedSubnets.value);
const selectAllReservations = () => selectRows(reservationTableRef.value, pagedReservations.value);
const invertReservations = () => invertRows(reservationTableRef.value, pagedReservations.value, selectedReservations.value);
const selectAllDenied = () => selectRows(deniedTableRef.value, pagedDenied.value);
const invertDenied = () => invertRows(deniedTableRef.value, pagedDenied.value, selectedDenied.value);

const handleIscUpload = async (file: any) => {
  const picked = file?.raw || file;
  if (!(picked instanceof File)) return false;

  parseMeta.fileName = picked.name;
  parseMeta.completedAt = '';
  parseProgress.value = 12;
  parseState.value = 'parsing';
  validationReport.value = null;
  lastImportReport.value = '';
  parseError.position = '';
  parseError.reason = '';
  parseError.guidance = '';

  const timer = setInterval(() => {
    if (parseProgress.value < 88) parseProgress.value += 8;
  }, 180);

  try {
    const { data } = await importIscDhcp(picked);
    iscResult.value = data.data as ISCImportResult;
    parseProgress.value = 100;
    parseState.value = 'success';
    parseMeta.completedAt = new Date().toISOString();
    showSuccess(t('system.migration.parseComplete'));
  } catch (error: any) {
    const msg = error?.response?.data?.message || t('system.migration.parseFailMsg');
    parseState.value = 'failed';
    fillParseError(msg);
    showHttpError(error, msg);
  } finally {
    clearInterval(timer);
  }
  return false;
};

const fillParseError = (message: string) => {
  const text = String(message || '').trim();
  const lineMatch = text.match(/line\s*[:=]?\s*(\d+)/i);
  const colMatch = text.match(/column\s*[:=]?\s*(\d+)/i);
  const line = lineMatch?.[1] || '';
  const col = colMatch?.[1] || '';
  parseError.position = line ? (col ? t('system.migration.errPosLineCol', { line, col }) : t('system.migration.errPosLine', { line })) : t('system.migration.errPosUnknown');
  parseError.reason = text || t('system.migration.errDefaultReason');
  parseError.guidance = t('system.migration.errDefaultGuidance');
};

const openLogDialog = () => {
  logDialogVisible.value = true;
};

const resetToUpload = () => {
  parseState.value = 'idle';
  parseProgress.value = 0;
  parseMeta.fileName = '';
  parseMeta.completedAt = '';
  iscResult.value = null;
  selectedSubnets.value = [];
  selectedReservations.value = [];
  selectedDenied.value = [];
  validationReport.value = null;
  lastImportReport.value = '';
  rollbackToken.value = '';
};

const resetAll = async () => {
  try {
    await ElMessageBox.confirm(t('system.migration.resetConfirm'), t('system.migration.resetTitle'), { type: 'warning' });
  } catch {
    return;
  }
  resetToUpload();
  showSuccess(t('system.migration.resetDone'));
};

const validateSelection = async () => {
  if (selectedTotalCount.value === 0) {
    showWarning(t('system.migration.valWarn'));
    return;
  }
  validateLoading.value = true;
  try {
    const { data } = await validateMigrationSelection(buildSelectionPayload());
    const result = (data as any)?.data || data;
    validationReport.value = {
      pass: !!result.pass,
      total: Number(result.total || 0),
      invalid: Number(result.invalid || 0),
      conflict: Number(result.conflict || 0)
    };
    if (validationReport.value.pass) {
      showSuccess(t('system.migration.valOk'));
    } else {
      showWarning(t('system.migration.valNotPass'));
    }
  } catch (e) {
    showHttpError(e, t('system.migration.valFailed'));
  } finally {
    validateLoading.value = false;
  }
};

const confirmImport = async () => {
  if (!canStartImport.value) return;
  const msg = t('system.migration.importConfirm', { subnets: selectedSubnets.value.length, reservations: selectedReservations.value.length, denied: selectedDenied.value.length });
  try {
    await ElMessageBox.confirm(msg, t('system.migration.importTitle'), { type: 'warning' });
  } catch {
    return;
  }

  importLoading.value = true;
  importProgress.value = 0;
  lastImportReport.value = '';
  try {
    for (let i = 1; i <= 7; i += 1) {
      await wait(220);
      importProgress.value = i * 10;
    }
    const { data } = await startMigrationImport(buildSelectionPayload());
    const result = (data as any)?.data || data;
    rollbackToken.value = String(result.rollbackToken || '');
    importProgress.value = 100;
    lastImportReport.value = String(result.report || t('system.migration.importDefault'));
    showSuccess(t('system.migration.importComplete'));
  } catch (e) {
    showHttpError(e, t('system.migration.importFail'));
  } finally {
    importLoading.value = false;
  }
};

const rollbackImport = async () => {
  if (!rollbackToken.value) {
    showWarning(t('system.migration.rollbackWarn'));
    return;
  }
  try {
    await ElMessageBox.confirm(t('system.migration.rollbackConfirm'), t('system.migration.rollbackTitle'), { type: 'warning' });
  } catch {
    return;
  }
  importLoading.value = true;
  try {
    const { data } = await rollbackMigrationImport(rollbackToken.value);
    const result = (data as any)?.data || data;
    lastImportReport.value = String(result.message || t('system.migration.rollbackDefault'));
    rollbackToken.value = '';
    showSuccess(t('system.migration.rollbackDone'));
  } catch (e) {
    showHttpError(e, t('system.migration.rollbackFail'));
  } finally {
    importLoading.value = false;
  }
};

const selectedSubnetRaw = () => {
  const selected = new Set(selectedSubnets.value.map((r) => r.id));
  return subnetRows.value.filter((row) => selected.has(row.id));
};
const selectedReservationRaw = () => {
  const selected = new Set(selectedReservations.value.map((r) => r.id));
  return reservationRows.value.filter((row) => selected.has(row.id));
};
const selectedDeniedRaw = () => {
  const selected = new Set(selectedDenied.value.map((r) => r.id));
  return deniedRows.value.filter((row) => selected.has(row.id));
};

const exportSelectedCsv = () => {
  if (selectedTotalCount.value === 0) {
    showWarning(t('system.migration.exportWarn'));
    return;
  }
  exportSubnetsCsv(true);
  exportReservationsCsv(true);
  exportDeniedHostsCsv(true);
  showSuccess(t('system.migration.exportDone'));
};

const exportSubnetsCsv = (selectedOnly: boolean) => {
  const rowsSource = selectedOnly ? selectedSubnetRaw() : subnetRows.value;
  if (!rowsSource.length) return;
  const header = ['cidr', 'gateway', 'dns', 'range', 'exclusions', 'options', 'parseStatus', 'parseReason'];
  const rows = [header];
  rowsSource.forEach((s) => {
    rows.push([
      s.cidr || '',
      s.gateway || '',
      (s.dns || []).join(';'),
      s.rangesText || '',
      s.exclusionsText || '',
      s.optionsText || '',
      parseStatusLabel(s.parseStatus),
      s.parseReason || ''
    ]);
  });
  downloadCsv(rows, selectedOnly ? 'selected-subnets.csv' : 'all-subnets.csv');
};

const exportReservationsCsv = (selectedOnly: boolean) => {
  const rowsSource = selectedOnly ? selectedReservationRaw() : reservationRows.value;
  if (!rowsSource.length) return;
  const rows = [['hostname', 'mac', 'ip', 'parseStatus', 'parseReason']];
  rowsSource.forEach((r) => {
    rows.push([r.hostname || '', r.mac || '', r.ip || '', parseStatusLabel(r.parseStatus), r.parseReason || '']);
  });
  downloadCsv(rows, selectedOnly ? 'selected-reservations.csv' : 'all-reservations.csv');
};

const exportDeniedHostsCsv = (selectedOnly: boolean) => {
  const rowsSource = selectedOnly ? selectedDeniedRaw() : deniedRows.value;
  if (!rowsSource.length) return;
  const rows = [['hostname', 'mac', 'reason', 'parseStatus', 'parseReason']];
  rowsSource.forEach((h) => {
    rows.push([
      h.hostname || '',
      h.mac || '',
      h.reason || 'deny booting',
      parseStatusLabel(h.parseStatus),
      h.parseReason || ''
    ]);
  });
  downloadCsv(rows, selectedOnly ? 'selected-denied-hosts.csv' : 'all-denied-hosts.csv');
};

const downloadCsv = (rows: string[][], filename: string) => {
  const csv = rows
    .map((row) => row.map((field) => `"${(field || '').replace(/"/g, '""')}"`).join(','))
    .join('\n');
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
};

const normalizeMac = (mac: string) => String(mac || '').trim().toLowerCase().replace(/-/g, ':');

const isValidMac = (mac: string) => /^([0-9a-f]{2}:){5}[0-9a-f]{2}$/i.test(mac);

const isValidIPv4 = (ip: string) => {
  const parts = ip.split('.').map((v) => Number(v));
  return parts.length === 4 && parts.every((n) => Number.isInteger(n) && n >= 0 && n <= 255);
};

const formatTime = (value?: string) => {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`;
};

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));
</script>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
  box-sizing: border-box;
}

.module-card {
  border-radius: 10px;
}

.module-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
}

.module-title {
  font-weight: 600;
  font-size: 16px;
  line-height: 24px;
}

.module-subtitle {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  margin-top: 2px;
}

.summary-tags {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.section-title {
  font-weight: 600;
}

.parse-loading .loading-tip {
  margin-top: 8px;
  color: var(--el-text-color-secondary);
}

.inline-actions {
  margin-top: 12px;
  display: flex;
  gap: 8px;
}

.error-detail {
  margin-top: 8px;
  line-height: 1.8;
}

.panel-title-wrap {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.panel-title {
  font-weight: 600;
}

.panel-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
  flex-wrap: wrap;
}

.toolbar-right {
  display: flex;
  gap: 8px;
}

.data-table {
  width: 100%;
}

.data-table :deep(.el-table__body-wrapper) {
  overflow-x: auto;
}

.data-table :deep(.error-row > td) {
  background: var(--el-color-danger-light-9) !important;
}

.reason {
  margin-top: 4px;
  color: var(--el-color-danger);
  font-size: 12px;
}

.flow-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.mt-12 {
  margin-top: 12px;
}

.item-title {
  font-weight: 600;
}

.item-desc {
  color: var(--el-text-color-secondary);
  margin: 4px 0;
}

.item-list {
  padding-left: 16px;
  color: var(--el-text-color-regular);
}

.pager-row {
  margin-top: 12px;
  display: flex;
  justify-content: flex-end;
}

.w-260 {
  width: 260px;
}
</style>
