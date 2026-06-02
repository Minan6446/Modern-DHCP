<template>
  <div class="access-page">
    <section class="top-banner">
      <div>
        <h2>{{ t('security.accessCtrl.title') }}</h2>
        <p>{{ t('security.accessCtrl.subtitle') }}</p>
      </div>
      <el-button
        type="primary"
        :icon="RefreshRight"
        :loading="refreshing"
        @click="handleGlobalRefresh"
        >{{ t('security.accessCtrl.globalRefresh') }}</el-button
      >
    </section>

    <section class="stats-grid">
      <el-card
        v-for="card in statCards"
        :key="card.key"
        shadow="never"
        class="stat-card"
        @click="activeTab = card.tab"
      >
        <div class="stat-label">{{ card.label }}</div>
        <div class="stat-value">{{ card.value }}</div>
      </el-card>
    </section>

    <section class="tabs-panel">
      <el-tabs v-model="activeTab">
        <el-tab-pane :label="t('security.accessCtrl.tabTrustPorts')" name="trustPorts">
          <div class="toolbar">
            <div class="toolbar-left">
              <el-input v-model="filters.trustPorts" :placeholder="t('security.accessCtrl.filterPort')" clearable />
            </div>
            <div class="toolbar-actions">
              <el-button class="tool-btn" type="primary" size="default" @click="onSearch('trustPorts')"
                >{{ t('security.accessCtrl.search') }}</el-button
              >
              <el-button class="tool-btn" type="primary" size="default" @click="onAdd('trustPorts')"
                >{{ t('security.accessCtrl.add') }}</el-button
              >
            </div>
          </div>
          <el-table :data="filteredRows.trustPorts" border stripe>
            <template #empty>
              <el-empty :description="t('security.accessCtrl.emptyHint')" :image-size="72" />
            </template>
            <el-table-column prop="name" :label="t('security.accessCtrl.colPort')" min-width="180" />
            <el-table-column prop="desc" :label="t('security.accessCtrl.colDevice')" min-width="220" />
            <el-table-column prop="updatedAt" :label="t('security.accessCtrl.colUpdateTime')" min-width="180" />
            <el-table-column :label="t('security.accessCtrl.colActions')" width="180" fixed="right">
              <template #default="{ row }">
                <el-button plain size="small" type="primary" @click="editTrustPort(row.id)">
                  {{ t('security.accessCtrl.edit') }}
                </el-button>
                <el-button plain size="small" type="danger" @click="removeTrustPort(row.id)">
                  {{ t('security.accessCtrl.delete') }}
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane :label="t('security.accessCtrl.tabViolations')" name="violations">
          <div class="toolbar">
            <div class="toolbar-left">
              <el-input v-model="filters.violations" :placeholder="t('security.accessCtrl.filterPolicy')" clearable />
            </div>
            <div class="toolbar-actions">
              <el-button class="tool-btn" type="primary" size="default" @click="onSearch('violations')"
                >{{ t('security.accessCtrl.search') }}</el-button
              >
              <el-button class="tool-btn" type="primary" size="default" @click="onAdd('violations')"
                >{{ t('security.accessCtrl.add') }}</el-button
              >
            </div>
          </div>
          <el-table :data="filteredRows.violations" border stripe>
            <template #empty>
              <el-empty :description="t('security.accessCtrl.emptyHint')" :image-size="72" />
            </template>
            <el-table-column prop="name" :label="t('security.accessCtrl.colPolicy')" min-width="180" />
            <el-table-column prop="desc" :label="t('security.accessCtrl.colAction')" min-width="220" />
            <el-table-column prop="updatedAt" :label="t('security.accessCtrl.colUpdateTime')" min-width="180" />
            <el-table-column :label="t('security.accessCtrl.colActions')" width="180" fixed="right">
              <template #default="{ row }">
                <el-button plain size="small" type="primary" @click="editViolation(row.id)">
                  {{ t('security.accessCtrl.edit') }}
                </el-button>
                <el-button plain size="small" type="danger" @click="removeViolation(row.id)">
                  {{ t('security.accessCtrl.delete') }}
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane :label="t('security.accessCtrl.tabRateLimit')" name="rateLimit">
          <div class="toolbar">
            <div class="toolbar-left">
              <el-input v-model="filters.rateLimit" :placeholder="t('security.accessCtrl.filterTarget')" clearable />
            </div>
            <div class="toolbar-actions">
              <el-button class="tool-btn" type="primary" size="default" @click="onSearch('rateLimit')"
                >{{ t('security.accessCtrl.search') }}</el-button
              >
              <el-button class="tool-btn" type="primary" size="default" @click="onAdd('rateLimit')"
                >{{ t('security.accessCtrl.add') }}</el-button
              >
            </div>
          </div>
          <el-table :data="filteredRows.rateLimit" border stripe>
            <template #empty>
              <el-empty :description="t('security.accessCtrl.emptyHint')" :image-size="72" />
            </template>
            <el-table-column prop="name" :label="t('security.accessCtrl.colTarget')" min-width="180" />
            <el-table-column prop="desc" :label="t('security.accessCtrl.colRule')" min-width="220" />
            <el-table-column prop="updatedAt" :label="t('security.accessCtrl.colUpdateTime')" min-width="180" />
            <el-table-column :label="t('security.accessCtrl.colActions')" width="180" fixed="right">
              <template #default="{ row }">
                <el-button plain size="small" type="primary" @click="editRateLimit(row.id)">
                  {{ t('security.accessCtrl.edit') }}
                </el-button>
                <el-button plain size="small" type="danger" @click="removeRateLimitRow(row.id)">
                  {{ t('security.accessCtrl.delete') }}
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane :label="t('security.accessCtrl.tabRogueDetect')" name="rogueDetect">
          <div class="toolbar">
            <div class="toolbar-left">
              <el-input v-model="filters.rogueDetect" :placeholder="t('security.accessCtrl.filterIpMac')" clearable />
            </div>
            <div class="toolbar-actions">
              <el-button class="tool-btn" type="primary" size="default" @click="onSearch('rogueDetect')"
                >{{ t('security.accessCtrl.search') }}</el-button
              >
              <el-button class="tool-btn" type="primary" size="default" @click="onAdd('rogueDetect')"
                >{{ t('security.accessCtrl.add') }}</el-button
              >
            </div>
          </div>
          <el-table :data="filteredRows.rogueDetect" border stripe>
            <template #empty>
              <el-empty :description="t('security.accessCtrl.emptyHint')" :image-size="72" />
            </template>
            <el-table-column prop="name" :label="t('security.accessCtrl.colServer')" min-width="180" />
            <el-table-column prop="desc" :label="t('security.accessCtrl.colRiskDesc')" min-width="220" />
            <el-table-column prop="updatedAt" :label="t('security.accessCtrl.colUpdateTime')" min-width="180" />
            <el-table-column :label="t('security.accessCtrl.colActions')" width="180" fixed="right">
              <template #default="{ row }">
                <el-button plain size="small" type="primary" @click="editRogue(row.id)">
                  {{ t('security.accessCtrl.edit') }}
                </el-button>
                <el-button plain size="small" type="danger" @click="removeRogue(row.id)">
                  {{ t('security.accessCtrl.delete') }}
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </section>

    <el-dialog
      v-model="addDialog.visible"
      :title="addDialog.mode === 'edit' ? `${t('security.accessCtrl.edit')} - ${tabLabel(addDialog.tab)}` : `${t('security.accessCtrl.add')} - ${tabLabel(addDialog.tab)}`"
      width="520px"
    >
      <el-form label-width="110px">
        <template v-if="addDialog.tab === 'trustPorts'">
          <el-form-item :label="t('security.accessCtrl.formDevice')">
            <el-input v-model="addForm.trustPorts.device" :placeholder="t('security.accessCtrl.phDevice')" />
          </el-form-item>
          <el-form-item :label="t('security.accessCtrl.formPort')">
            <el-input v-model="addForm.trustPorts.port" :placeholder="t('security.accessCtrl.phPort')" />
          </el-form-item>
        </template>

        <template v-else-if="addDialog.tab === 'violations'">
          <el-form-item :label="t('security.accessCtrl.formPolicyName')">
            <el-input v-model="addForm.violations.name" :placeholder="t('security.accessCtrl.phPolicy')" />
          </el-form-item>
          <el-form-item :label="t('security.accessCtrl.formAction')">
            <el-select v-model="addForm.violations.action" style="width: 100%">
              <el-option :label="t('security.accessCtrl.optDrop')" value="drop" />
              <el-option :label="t('security.accessCtrl.optAlert')" value="alert" />
              <el-option :label="t('security.accessCtrl.optRateLimit')" value="rate-limit" />
              <el-option :label="t('security.accessCtrl.optShutdown')" value="shutdown" />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('security.accessCtrl.formBlockDuration')">
            <el-input-number v-model="addForm.violations.blockDurationSeconds" :min="0" />
          </el-form-item>
        </template>

        <template v-else-if="addDialog.tab === 'rateLimit'">
          <el-form-item :label="t('security.accessCtrl.formTarget')">
            <el-input v-model="addForm.rateLimit.target" :placeholder="t('security.accessCtrl.phTarget')" />
          </el-form-item>
          <el-form-item :label="t('security.accessCtrl.formRateLimit')">
            <el-input-number v-model="addForm.rateLimit.limitPps" :min="1" />
          </el-form-item>
          <el-form-item :label="t('security.accessCtrl.formStatus')">
            <el-select v-model="addForm.rateLimit.status" style="width: 100%">
              <el-option :label="t('security.accessCtrl.optActive')" value="active" />
              <el-option :label="t('security.accessCtrl.optDisabled')" value="disabled" />
            </el-select>
          </el-form-item>
        </template>

        <template v-else>
          <el-form-item :label="t('security.accessCtrl.formServerIp')">
            <el-input v-model="addForm.rogueDetect.ip" :placeholder="t('security.accessCtrl.phServerIp')" />
          </el-form-item>
          <el-form-item :label="t('security.accessCtrl.formServerMac')">
            <el-input v-model="addForm.rogueDetect.mac" :placeholder="t('security.accessCtrl.phServerMac')" />
          </el-form-item>
          <el-form-item :label="t('security.accessCtrl.formSeverity')">
            <el-select v-model="addForm.rogueDetect.severity" style="width: 100%">
              <el-option :label="t('security.accessCtrl.optLow')" value="low" />
              <el-option :label="t('security.accessCtrl.optMedium')" value="medium" />
              <el-option :label="t('security.accessCtrl.optHigh')" value="high" />
            </el-select>
          </el-form-item>
        </template>
      </el-form>
      <template #footer>
        <el-button @click="addDialog.visible = false">{{ t('security.accessCtrl.cancel') }}</el-button>
        <el-button type="primary" :loading="addDialog.submitting" @click="submitAdd">{{
          addDialog.mode === 'edit' ? t('security.accessCtrl.save') : t('security.accessCtrl.submit')
        }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { ElMessage, ElMessageBox } from 'element-plus';
import { RefreshRight } from '@element-plus/icons-vue';
import {
  createRogueServer,
  deleteRateLimit,
  deleteRogueServer,
  deleteViolationPolicy,
  listViolationPolicies,
  saveRateLimit,
  listRateLimits,
  listRogueServers,
  listTrustPorts,
  saveTrustPorts,
  saveViolationPolicy,
  updateRogueServer
} from '@/api/security';
import { showHttpError } from '@/shared/errors/errorToast';
import type { RateLimitRule, TrustPort, ViolationPolicy } from '@/types/security';

const { t } = useI18n();

type TabKey = 'trustPorts' | 'violations' | 'rateLimit' | 'rogueDetect';

type TableRow = {
  id?: string;
  name: string;
  desc: string;
  updatedAt: string;
};

const activeTab = ref<TabKey>('trustPorts');
const refreshing = ref(false);
const trustPortSource = ref<TrustPort[]>([]);
const rateLimitSource = ref<RateLimitRule[]>([]);
const rogueSource = ref<Array<{ id: string; ip: string; mac: string; severity: string }>>([]);

const addDialog = reactive({
  visible: false,
  tab: 'trustPorts' as TabKey,
  mode: 'create' as 'create' | 'edit',
  editingId: '',
  submitting: false
});

const addForm = reactive({
  trustPorts: { device: '', port: '' },
  violations: {
    name: t('security.accessCtrl.defaultViolationPolicy'),
    action: 'alert' as ViolationPolicy['action'],
    blockDurationSeconds: 300
  },
  rateLimit: { target: '', limitPps: 100, status: 'active' as 'active' | 'disabled' },
  rogueDetect: { ip: '', mac: '', severity: 'low' as 'low' | 'medium' | 'high' }
});

const filters = reactive<Record<TabKey, string>>({
  trustPorts: '',
  violations: '',
  rateLimit: '',
  rogueDetect: ''
});

const rows = reactive<Record<TabKey, TableRow[]>>({
  trustPorts: [],
  violations: [],
  rateLimit: [],
  rogueDetect: []
});

const violationPolicies = ref<ViolationPolicy[]>([]);

const filteredRows = computed<Record<TabKey, TableRow[]>>(() => {
  const pick = (tab: TabKey) => {
    const keyword = filters[tab].trim().toLowerCase();
    if (!keyword) return rows[tab];
    return rows[tab].filter((row) => {
      return [row.name, row.desc, row.updatedAt]
        .join(' ')
        .toLowerCase()
        .includes(keyword);
    });
  };
  return {
    trustPorts: pick('trustPorts'),
    violations: pick('violations'),
    rateLimit: pick('rateLimit'),
    rogueDetect: pick('rogueDetect')
  };
});

const statCards = computed(() => [
  { key: 'trust', label: t('security.accessCtrl.statTrustPorts'), value: rows.trustPorts.length, tab: 'trustPorts' as TabKey },
  { key: 'rate', label: t('security.accessCtrl.statRateLimits'), value: rows.rateLimit.length, tab: 'rateLimit' as TabKey },
  { key: 'anomaly', label: t('security.accessCtrl.statAnomalies'), value: rows.rogueDetect.length, tab: 'rogueDetect' as TabKey }
]);

const handleGlobalRefresh = () => {
  void loadAll();
};

const onSearch = (tab: TabKey) => {
  ElMessage.info(t('security.accessCtrl.searchDone', { tab: tabLabel(tab) }));
};

const onAdd = (tab: TabKey) => {
  addDialog.mode = 'create';
  addDialog.editingId = '';
  resetAddForm();
  addDialog.tab = tab;
  addDialog.visible = true;
};

const resetAddForm = () => {
  addForm.trustPorts = { device: '', port: '' };
  addForm.violations = { name: t('security.accessCtrl.defaultViolationPolicy'), action: 'alert', blockDurationSeconds: 300 };
  addForm.rateLimit = { target: '', limitPps: 100, status: 'active' };
  addForm.rogueDetect = { ip: '', mac: '', severity: 'low' };
};

const submitAdd = async () => {
  addDialog.submitting = true;
  try {
    if (addDialog.tab === 'trustPorts') {
      const newPort: TrustPort = {
        id: addDialog.mode === 'edit' ? addDialog.editingId : `tp-${Date.now()}`,
        device: addForm.trustPorts.device.trim(),
        port: addForm.trustPorts.port.trim(),
        trusted: true,
        lastUpdated: new Date().toISOString()
      };
      const nextPorts =
        addDialog.mode === 'edit'
          ? trustPortSource.value.map((item) => (item.id === newPort.id ? newPort : item))
          : [...trustPortSource.value, newPort];
      await saveTrustPorts(nextPorts);
      await loadTrustPorts();
    } else if (addDialog.tab === 'violations') {
      await saveViolationPolicy({
        id: addDialog.mode === 'edit' ? addDialog.editingId : undefined,
        name: addForm.violations.name.trim() || t('security.accessCtrl.defaultViolationPolicy'),
        action: addForm.violations.action,
        blockDurationSeconds: addForm.violations.blockDurationSeconds,
        alertChannels: ['email']
      });
      await loadViolations();
      ElMessage.success(addDialog.mode === 'edit' ? t('security.accessCtrl.violationUpdated') : t('security.accessCtrl.violationCreated'));
    } else if (addDialog.tab === 'rateLimit') {
      await saveRateLimit({
        id: addDialog.mode === 'edit' ? addDialog.editingId : undefined,
        target: addForm.rateLimit.target.trim(),
        limitPps: addForm.rateLimit.limitPps,
        status: addForm.rateLimit.status,
        scope: 'port'
      });
      await loadRateLimits();
    } else {
      const payload = {
        ip: addForm.rogueDetect.ip.trim(),
        mac: addForm.rogueDetect.mac.trim(),
        severity: addForm.rogueDetect.severity
      };
      if (addDialog.mode === 'edit') {
        await updateRogueServer(addDialog.editingId, payload);
      } else {
        await createRogueServer(payload);
      }
      await loadRogueServers();
      ElMessage.success(addDialog.mode === 'edit' ? t('security.accessCtrl.rogueUpdated') : t('security.accessCtrl.rogueCreated'));
    }
    addDialog.visible = false;
    resetAddForm();
    if (addDialog.tab !== 'violations') {
      ElMessage.success(addDialog.mode === 'edit' ? t('security.accessCtrl.saveSuccess') : t('security.accessCtrl.addSuccess'));
    }
  } catch (error) {
    showHttpError(error, t('security.accessCtrl.addFail'));
  } finally {
    addDialog.submitting = false;
  }
};

const normalizeTs = (value?: string) => {
  const raw = (value || '').trim();
  if (!raw) return '-';
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(
    date.getDate()
  ).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(
    date.getMinutes()
  ).padStart(2, '0')}`;
};

const loadTrustPorts = async () => {
  const { data } = await listTrustPorts();
  const items = ((data as any)?.data ?? []) as Array<{
    id?: string;
    device?: string;
    port?: string;
    trusted?: boolean;
    lastUpdated?: string;
  }>;
  trustPortSource.value = items.map((item) => ({
    id: item.id || `tp-${Math.random().toString(36).slice(2, 8)}`,
    device: item.device || '',
    port: item.port || '',
    trusted: Boolean(item.trusted),
    lastUpdated: item.lastUpdated
  }));
  rows.trustPorts = items.map((item) => ({
    id: item.id,
    name: item.port || '-',
    desc: item.device || '-',
    updatedAt: normalizeTs(item.lastUpdated)
  }));
};

const loadRateLimits = async () => {
  const { data } = await listRateLimits({ page: 1, pageSize: 50 });
  const page = ((data as any)?.data ?? { items: [] }) as {
    items?: Array<{ id?: string; target?: string; limitPps?: number; status?: string }>;
  };
  rateLimitSource.value = (page.items || []).map((item) => ({
    id: item.id || `rate-${Math.random().toString(36).slice(2, 8)}`,
    scope: 'port',
    target: item.target || '',
    limitPps: item.limitPps ?? 0,
    status: (item.status as RateLimitRule['status']) || 'active'
  }));
  rows.rateLimit = rateLimitSource.value.map((item) => ({
    id: item.id,
    name: item.target || '-',
    desc: `${item.limitPps ?? 0} pps · ${item.status || 'active'}`,
    updatedAt: '-'
  }));
};

const editTrustPort = (id?: string) => {
  if (!id) return;
  const target = trustPortSource.value.find((item) => item.id === id);
  if (!target) return;
  addDialog.mode = 'edit';
  addDialog.editingId = id;
  addDialog.tab = 'trustPorts';
  addForm.trustPorts = { device: target.device || '', port: target.port || '' };
  addDialog.visible = true;
};

const removeTrustPort = async (id?: string) => {
  if (!id) return;
  try {
    await ElMessageBox.confirm(t('security.accessCtrl.confirmDeleteTrustPort'), t('security.accessCtrl.confirm'), { type: 'warning' });
    const next = trustPortSource.value.filter((item) => item.id !== id);
    await saveTrustPorts(next);
    await loadTrustPorts();
    ElMessage.success(t('security.accessCtrl.trustPortDeleted'));
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      showHttpError(error, t('security.accessCtrl.deleteTrustPortFail'));
    }
  }
};

const editRateLimit = (id?: string) => {
  if (!id) return;
  const target = rateLimitSource.value.find((item) => item.id === id);
  if (!target) return;
  addDialog.mode = 'edit';
  addDialog.editingId = id;
  addDialog.tab = 'rateLimit';
  addForm.rateLimit = {
    target: target.target || '',
    limitPps: target.limitPps ?? 100,
    status: target.status || 'active'
  };
  addDialog.visible = true;
};

const removeRateLimitRow = async (id?: string) => {
  if (!id) return;
  try {
    await ElMessageBox.confirm(t('security.accessCtrl.confirmDeleteRateLimit'), t('security.accessCtrl.confirm'), { type: 'warning' });
    await deleteRateLimit(id);
    await loadRateLimits();
    ElMessage.success(t('security.accessCtrl.rateLimitDeleted'));
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      showHttpError(error, t('security.accessCtrl.deleteRateLimitFail'));
    }
  }
};

const loadRogueServers = async () => {
  const { data } = await listRogueServers();
  const items = ((data as any)?.data ?? []) as Array<{
    id?: string;
    ip?: string;
    mac?: string;
    severity?: string;
    detectedAt?: string;
    actions?: string[];
  }>;
  rogueSource.value = items.map((item) => ({
    id: item.id || `rogue-${Math.random().toString(36).slice(2, 8)}`,
    ip: item.ip || '',
    mac: item.mac || '',
    severity: item.severity || 'low'
  }));
  rows.rogueDetect = items.map((item) => ({
    id: item.id,
    name: `${item.ip || '-'} / ${item.mac || '-'}`,
    desc: `${item.severity || 'low'}${(item.actions || []).length ? ` · ${(item.actions || []).join('/')}` : ''}`,
    updatedAt: normalizeTs(item.detectedAt)
  }));
};

const editRogue = (id?: string) => {
  if (!id) return;
  const target = rogueSource.value.find((item) => item.id === id);
  if (!target) return;
  addDialog.mode = 'edit';
  addDialog.editingId = id;
  addDialog.tab = 'rogueDetect';
  addForm.rogueDetect = {
    ip: target.ip,
    mac: target.mac,
    severity: (target.severity as 'low' | 'medium' | 'high') || 'low'
  };
  addDialog.visible = true;
};

const removeRogue = async (id?: string) => {
  if (!id) return;
  try {
    await ElMessageBox.confirm(t('security.accessCtrl.confirmDeleteRogue'), t('security.accessCtrl.confirm'), { type: 'warning' });
    await deleteRogueServer(id);
    await loadRogueServers();
    ElMessage.success(t('security.accessCtrl.rogueDeleted'));
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      showHttpError(error, t('security.accessCtrl.deleteRogueFail'));
    }
  }
};

const loadViolations = async () => {
  try {
    const { data } = await listViolationPolicies();
    violationPolicies.value = ((data as any)?.data ?? []) as ViolationPolicy[];
    rows.violations = violationPolicies.value.map((item) => ({
      id: item.id,
      name: item.name || t('security.accessCtrl.defaultViolationPolicy'),
      desc: `${item.action} · ${item.blockDurationSeconds ?? 0}s`,
      updatedAt: normalizeTs(item.updatedAt)
    }));
  } catch (error) {
    showHttpError(error, t('security.accessCtrl.loadViolationsFail'));
  }
};

const editViolation = (id?: string) => {
  if (!id) return;
  const target = violationPolicies.value.find((item) => item.id === id);
  if (!target) return;
  addDialog.mode = 'edit';
  addDialog.editingId = id;
  addDialog.tab = 'violations';
  addForm.violations = {
    name: target.name || t('security.accessCtrl.defaultViolationPolicy'),
    action: target.action,
    blockDurationSeconds: target.blockDurationSeconds ?? 300
  };
  addDialog.visible = true;
};

const removeViolation = async (id?: string) => {
  if (!id) return;
  try {
    await ElMessageBox.confirm(t('security.accessCtrl.confirmDeleteViolation'), t('security.accessCtrl.confirm'), { type: 'warning' });
    await deleteViolationPolicy(id);
    await loadViolations();
    ElMessage.success(t('security.accessCtrl.violationDeleted'));
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      showHttpError(error, t('security.accessCtrl.deleteViolationFail'));
    }
  }
};

const loadAll = async () => {
  refreshing.value = true;
  try {
    await Promise.all([
      loadTrustPorts(),
      loadRateLimits(),
      loadRogueServers(),
      loadViolations()
    ]);
    ElMessage.success(t('security.accessCtrl.globalRefreshDone'));
  } catch (error) {
    showHttpError(error, t('security.accessCtrl.loadAllFail'));
  } finally {
    refreshing.value = false;
  }
};

const tabLabel = (tab: TabKey) => {
  const map: Record<TabKey, string> = {
    trustPorts: t('security.accessCtrl.tabTrustPorts'),
    violations: t('security.accessCtrl.tabViolations'),
    rateLimit: t('security.accessCtrl.tabRateLimit'),
    rogueDetect: t('security.accessCtrl.tabRogueDetect')
  };
  return map[tab];
};

onMounted(() => {
  resetAddForm();
  void loadAll();
});
</script>

<style scoped>
.access-page {
  --access-page-bg: var(--el-fill-color-page);
  --access-card-bg: var(--el-bg-color);
  --access-text-primary: var(--el-text-color-primary);
  --access-text-secondary: var(--el-text-color-secondary);
  min-height: 100%;
  padding: 16px;
  background: var(--access-page-bg);
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.top-banner {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  padding: 20px 24px;
  border-radius: 12px;
  background: linear-gradient(90deg, #1d4ed8 0%, #0ea5e9 100%);
  color: var(--el-color-white);
}

.top-banner h2 {
  margin: 0;
  font-size: 24px;
  font-weight: 700;
}

.top-banner p {
  margin: 8px 0 0;
  opacity: 0.95;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.stat-card {
  cursor: pointer;
  border-radius: 10px;
  background: var(--access-card-bg);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.stat-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 8px 18px rgba(0, 0, 0, 0.12);
}

.stat-label {
  font-size: 14px;
  color: var(--access-text-secondary);
}

.stat-value {
  margin-top: 8px;
  font-size: 28px;
  font-weight: 700;
  color: var(--access-text-primary);
}

.tabs-panel {
  background: var(--access-card-bg);
  border-radius: 12px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
  padding: 16px;
}

.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.toolbar-left {
  flex: 1;
}

.toolbar-left .el-input {
  width: 320px;
}

.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tool-btn {
  min-width: 92px;
  border-radius: 4px;
  padding: 8px 16px;
  font-size: 14px;
}

@media (max-width: 768px) {
  .toolbar {
    flex-direction: column;
    align-items: stretch;
  }

  .toolbar-left .el-input {
    width: 100%;
  }

  .toolbar-actions {
    justify-content: flex-end;
  }
}

@media (max-width: 1200px) {
  .stats-grid {
    grid-template-columns: 1fr;
  }

  .top-banner {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
