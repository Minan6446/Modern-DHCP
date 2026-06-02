<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { listCertsApi, uploadCertApi, deleteCertApi, renewCertApi, type TLSCertRecord } from '../../api/security'
import StatusPill from '../../components/StatusPill.vue'

type CertRecord = TLSCertRecord

const { t } = useI18n()
const certs = ref<CertRecord[]>([])

const loading = ref(false)
const dialogVisible = ref(false)
const detailVisible = ref(false)
const detailRow = ref<CertRecord | null>(null)
const submitting = ref(false)
const keyword = ref('')
const filterStatus = ref('')

const fetchCerts = async () => {
  loading.value = true
  try {
    const res = await listCertsApi({ keyword: keyword.value, status: filterStatus.value })
    certs.value = res.data?.list ?? []
  } finally {
    loading.value = false
  }
}

onMounted(fetchCerts)

const filteredCerts = computed(() => certs.value.filter(c => {
  const matchK = !keyword.value || c.domain.includes(keyword.value)
  const matchS = !filterStatus.value || c.status === filterStatus.value
  return matchK && matchS
}))

const kpiNormal = computed(() => certs.value.filter(c => c.status === '正常').length)
const kpiExpiring = computed(() => certs.value.filter(c => c.status === '即将过期').length)
const kpiExpired = computed(() => certs.value.filter(c => c.status === '已过期').length)

const form = reactive({ domain: '', type: 'DoT' as 'DoT' | 'DoH' | 'mTLS', certContent: '', keyContent: '' })

const openDialog = () => {
  Object.assign(form, { domain: '', type: 'DoT', certContent: '', keyContent: '' })
  dialogVisible.value = true
}

const submitCert = async () => {
  if (!form.domain || !form.certContent) { ElMessage.warning(t('security.fillCertInfo')); return }
  submitting.value = true
  try {
    const res = await uploadCertApi({ domain: form.domain, type: form.type, certContent: form.certContent, keyContent: form.keyContent })
    certs.value.unshift(res.data)
    dialogVisible.value = false
    ElMessage.success(t('security.certUploadSuccess'))
  } catch {
    ElMessage.error(t('security.certUploadFailed'))
  } finally {
    submitting.value = false
  }
}

const renewCert = async (row: CertRecord) => {
  loading.value = true
  try {
    const res = await renewCertApi(row.id)
    Object.assign(row, res.data)
    ElMessage.success(t('security.certRenewed', { domain: row.domain }))
  } catch {
    ElMessage.error(t('security.certRenewFailed'))
  } finally {
    loading.value = false
  }
}

const removeCert = async (row: CertRecord) => {
  await ElMessageBox.confirm(t('security.deleteCertConfirm', { domain: row.domain }), t('common.confirm'), { type: 'warning' })
  try {
    await deleteCertApi(row.id)
    certs.value = certs.value.filter(c => c.id !== row.id)
    ElMessage.success(t('common.deleted'))
  } catch {
    ElMessage.error(t('security.certDeleteFailed'))
  }
}

const openDetail = (row: CertRecord) => { detailRow.value = row; detailVisible.value = true }

// Translate the i18n status label back to a StatusPill intent. We
// compare against translated strings instead of an enum because the
// backend already returns a localised value here — a quirk we keep
// to avoid changing the API contract during this UI cleanup.
const statusIntent = (s: string): 'success' | 'warning' | 'danger' => {
  if (s === t('security.normal')) return 'success'
  if (s === t('security.expiring')) return 'warning'
  return 'danger'
}
const typeColor = { DoT: '#3b82f6', DoH: '#10b981', mTLS: '#8b5cf6' }
const daysColor = (d: number) => d < 0 ? 'var(--app-danger)' : d < 30 ? 'var(--app-warning)' : 'var(--app-success)'
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('security.certManage') }}</h1>
        <p class="page-subtitle">{{ $t('security.certSubtitle') }}</p>
      </div>
      <div class="page-actions">
        <el-button type="primary" @click="openDialog">{{ $t('security.uploadCert') }}</el-button>
      </div>
    </div>

    <!-- KPI -->
    <div class="mn-stats-strip">
      <div class="mn-stat-tile mn-stat-tile--success"><span class="mn-stat-value">{{ kpiNormal }}</span><span class="mn-stat-label">{{ $t('security.normal') }}</span></div>
      <div class="mn-stat-tile mn-stat-tile--warning"><span class="mn-stat-value">{{ kpiExpiring }}</span><span class="mn-stat-label">{{ $t('security.expiring') }}</span></div>
      <div class="mn-stat-tile mn-stat-tile--danger"><span class="mn-stat-value">{{ kpiExpired }}</span><span class="mn-stat-label">{{ $t('security.expired') }}</span></div>
      <div class="mn-stat-tile"><span class="mn-stat-value">{{ certs.length }}</span><span class="mn-stat-label">{{ $t('security.totalCerts') }}</span></div>
    </div>

    <el-card class="mn-card">
      <div class="mn-toolbar">
        <div class="mn-toolbar-filters">
          <el-input v-model="keyword" clearable :placeholder="$t('common.searchDomain')" style="width:220px">
            <template #prefix><svg class="mn-input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></template>
          </el-input>
          <el-select v-model="filterStatus" clearable :placeholder="$t('security.certStatus')" style="width:130px">
            <el-option v-for="s in [t('security.normal'),t('security.expiring'),t('security.expired')]" :key="s" :label="s" :value="s" />
          </el-select>
        </div>
      </div>
      <el-table :data="filteredCerts" v-loading="loading" stripe class="mn-table" table-layout="auto">
        <el-table-column prop="domain" :label="$t('cache.domain')" min-width="200" show-overflow-tooltip>
          <template #default="{ row }"><span class="mn-mono">{{ row.domain }}</span></template>
        </el-table-column>
        <el-table-column prop="type" :label="$t('common.type')" width="90" align="center">
          <template #default="{ row }">
            <span class="type-badge" :style="{ background: typeColor[row.type] }">{{ row.type }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="issuer" :label="$t('security.issuer')" min-width="150" />
        <el-table-column prop="expireAt" :label="$t('security.expireAt')" min-width="130">
          <template #default="{ row }"><span class="mn-mono">{{ row.expireAt }}</span></template>
        </el-table-column>
        <el-table-column prop="daysLeft" :label="$t('security.daysLeft')" width="110" align="right" sortable>
          <template #default="{ row }">
            <span class="mn-mono" :style="{ color: daysColor(row.daysLeft), fontWeight: 700 }">
              {{ row.daysLeft > 0 ? row.daysLeft + ' ' + $t('security.days') : $t('security.expired') }}
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="status" :label="$t('common.status')" width="110" align="center">
          <template #default="{ row }">
            <StatusPill :intent="statusIntent(row.status)">{{ row.status }}</StatusPill>
          </template>
        </el-table-column>
        <el-table-column prop="uploadedAt" :label="$t('security.uploadedAt')" min-width="120">
          <template #default="{ row }"><span class="mn-time">{{ row.uploadedAt }}</span></template>
        </el-table-column>
        <el-table-column :label="$t('common.operation')" width="160" fixed="right">
          <template #default="{ row }">
            <div class="mn-row-ops">
              <el-button link type="primary" @click="openDetail(row)">{{ $t('common.detail') }}</el-button>
              <el-button link type="warning" @click="renewCert(row)">{{ $t('security.renew') }}</el-button>
              <el-button link type="danger" @click="removeCert(row)">{{ $t('common.delete') }}</el-button>
            </div>
          </template>
        </el-table-column>
        <template #empty><el-empty :description="$t('security.noCerts')" /></template>
      </el-table>
    </el-card>

    <!-- Upload dialog -->
    <el-dialog v-model="dialogVisible" :title="$t('security.uploadCert')" width="560px" append-to-body>
      <el-form :model="form" label-position="top">
        <el-form-item :label="$t('cache.domain')"><el-input v-model="form.domain" placeholder="dns.example.com" /></el-form-item>
        <el-form-item :label="$t('security.certType')">
          <el-radio-group v-model="form.type">
            <el-radio-button label="DoT" value="DoT" />
            <el-radio-button label="DoH" value="DoH" />
            <el-radio-button label="mTLS" value="mTLS" />
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="$t('security.certContent')">
          <el-input v-model="form.certContent" type="textarea" :rows="5" :placeholder="$t('security.certContentPlaceholder')" style="font-family:monospace;font-size:12px" />
        </el-form-item>
        <el-form-item :label="$t('security.keyContent')">
          <el-input v-model="form.keyContent" type="textarea" :rows="5" :placeholder="$t('security.keyContentPlaceholder')" style="font-family:monospace;font-size:12px" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="submitting" @click="submitCert">{{ $t('common.upload') }}</el-button>
      </template>
    </el-dialog>

    <!-- Detail dialog -->
    <el-dialog v-model="detailVisible" :title="$t('security.certDetail')" width="480px" append-to-body>
      <div v-if="detailRow" class="cert-detail-grid">
        <div class="cert-detail-row"><span class="cert-detail-label">{{ $t('cache.domain') }}</span><span class="mn-mono">{{ detailRow.domain }}</span></div>
        <div class="cert-detail-row"><span class="cert-detail-label">{{ $t('common.type') }}</span><span>{{ detailRow.type }}</span></div>
        <div class="cert-detail-row"><span class="cert-detail-label">{{ $t('security.issuer') }}</span><span>{{ detailRow.issuer }}</span></div>
        <div class="cert-detail-row"><span class="cert-detail-label">{{ $t('security.expireAt') }}</span><span class="mn-mono">{{ detailRow.expireAt }}</span></div>
        <div class="cert-detail-row"><span class="cert-detail-label">{{ $t('security.fingerprint') }}</span><span class="mn-mono" style="font-size:11px">{{ detailRow.fingerprint }}</span></div>
        <div class="cert-detail-row"><span class="cert-detail-label">{{ $t('security.uploadedAt') }}</span><span class="mn-mono">{{ detailRow.uploadedAt }}</span></div>
        <div class="cert-detail-row"><span class="cert-detail-label">{{ $t('common.status') }}</span><StatusPill :intent="statusIntent(detailRow.status)">{{ detailRow.status }}</StatusPill></div>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.mn-stats-strip { display:flex; gap:14px; flex-wrap:wrap; }
.mn-stat-tile { display:flex; flex-direction:row; align-items:center; gap:14px; padding:14px 24px; border-radius:10px; border:1px solid var(--app-border); background:var(--app-bg); min-width:100px; flex:1; box-shadow:var(--app-shadow-soft); }
.mn-stat-value { font-size:28px; font-weight:700; font-variant-numeric:tabular-nums; letter-spacing:-0.03em; line-height:1; }
.mn-stat-label { font-size:12px; font-weight:500; color:var(--app-text-regular); line-height:1.4; }
.mn-stat-tile--success .mn-stat-value { color:var(--app-success); }
.mn-stat-tile--danger .mn-stat-value  { color:var(--app-danger); }
.mn-stat-tile--warning .mn-stat-value { color:var(--app-warning); }
.mn-stat-tile--accent .mn-stat-value  { color:var(--app-accent); }
.mn-stat-tile--neutral .mn-stat-value { color:var(--app-text-secondary); }
.type-badge { display:inline-flex; align-items:center; justify-content:center; padding:2px 8px; border-radius:4px; font-size:11px; font-weight:700; color:#fff; }
.cert-detail-grid { display:flex; flex-direction:column; gap:0; }
.cert-detail-row { display:grid; grid-template-columns:90px 1fr; gap:12px; padding:10px 0; border-bottom:1px solid var(--app-border); font-size:13px; }
.cert-detail-row:last-child { border-bottom:none; }
.cert-detail-label { color:var(--app-text-secondary); font-weight:600; font-size:12px; }
</style>
