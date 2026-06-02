<template>
  <div class="threat-center page-block">
    <section class="command-bar">
      <div class="command-text">
        <h2>{{ t('security.threat.pageTitle') }}</h2>
        <p>{{ t('security.threat.pageSubtitle') }}</p>
      </div>
      <div class="command-actions">
        <el-button
          type="warning"
          :disabled="!selectedRogues.length"
          :loading="batchAction === 'quarantine'"
          @click="batchQuarantine"
          >{{ t('security.threat.batchQuarantine') }}</el-button
        >
        <el-button
          type="danger"
          :disabled="!selectedRogues.length"
          :loading="batchAction === 'block'"
          @click="batchBlockAcl"
          >{{ t('security.threat.batchBlockAcl') }}</el-button
        >
        <el-button type="primary" :loading="loading" @click="loadAll">{{ t('security.threat.refresh') }}</el-button>
      </div>
    </section>

    <section class="stats-grid">
      <el-card shadow="never" class="stats-card">
        <p class="stats-label">{{ t('security.threat.statSpoofingLabel') }}</p>
        <div class="stats-value">{{ spoofingCount }}</div>
      </el-card>
      <el-card shadow="never" class="stats-card">
        <p class="stats-label">{{ t('security.threat.statExhaustionLabel') }}</p>
        <div class="stats-value">{{ exhaustionCount }}</div>
      </el-card>
      <el-card shadow="never" class="stats-card">
        <p class="stats-label">{{ t('security.threat.statRogueLabel') }}</p>
        <div class="stats-value">{{ rogueCount }}</div>
      </el-card>
      <el-card shadow="never" class="stats-card pending-card">
        <p class="stats-label">{{ t('security.threat.statPendingLabel') }}</p>
        <div class="stats-value">{{ pendingCount }}</div>
      </el-card>
    </section>

    <section class="rules-section">
      <el-tabs v-model="activeRuleTab" class="rules-tabs">
        <el-tab-pane :label="t('security.threat.tabSpoofing')" name="spoofing">
          <p class="rule-desc">{{ t('security.threat.spoofRuleDesc') }}</p>
          <el-form :inline="true" :model="spoofRule" class="inline-form">
            <el-form-item :label="t('security.threat.enableProtection')">
              <el-switch v-model="spoofRule.enabled" />
            </el-form-item>
            <el-form-item :label="t('security.threat.strictMode')">
              <el-switch v-model="spoofRule.strictMode" />
            </el-form-item>
            <el-form-item :label="t('security.threat.blockThreshold')">
              <el-input-number v-model="spoofRule.blockThreshold" :min="1" :max="100" />
            </el-form-item>
            <el-form-item :label="t('security.threat.autoBlock')">
              <el-switch v-model="spoofRule.autoBlock" />
            </el-form-item>
          </el-form>
          <div class="rule-actions">
            <el-button type="primary" @click="saveSpoofRule">{{ t('security.threat.saveConfig') }}</el-button>
            <el-button @click="resetSpoofRule">{{ t('security.threat.resetDefault') }}</el-button>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('security.threat.tabExhaustion')" name="exhaustion">
          <p class="rule-desc">{{ t('security.threat.exhaustRuleDesc') }}</p>
          <el-form :inline="true" :model="exhaustRule" class="inline-form">
            <el-form-item :label="t('security.threat.enableProtection')">
              <el-switch v-model="exhaustRule.enabled" />
            </el-form-item>
            <el-form-item :label="t('security.threat.windowSeconds')">
              <el-input-number v-model="exhaustRule.windowSeconds" :min="10" />
            </el-form-item>
            <el-form-item :label="t('security.threat.attemptLimit')">
              <el-input-number v-model="exhaustRule.attemptLimit" :min="1" />
            </el-form-item>
            <el-form-item :label="t('security.threat.autoRateLimit')">
              <el-switch v-model="exhaustRule.autoRateLimit" />
            </el-form-item>
          </el-form>
          <div class="rule-actions">
            <el-button type="primary" :loading="rateSaving" @click="saveExhaustRule">{{ t('security.threat.saveConfig') }}</el-button>
            <el-button @click="resetExhaustRule">{{ t('security.threat.resetDefault') }}</el-button>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('security.threat.tabRogue')" name="rogue">
          <p class="rule-desc">{{ t('security.threat.rogueRuleDesc') }}</p>
          <el-form :inline="true" :model="rogueRule" class="inline-form">
            <el-form-item :label="t('security.threat.enableProtection')">
              <el-switch v-model="rogueRule.enabled" />
            </el-form-item>
            <el-form-item :label="t('security.threat.detectInterval')">
              <el-input-number v-model="rogueRule.detectInterval" :min="5" />
            </el-form-item>
            <el-form-item :label="t('security.threat.confidenceThreshold')">
              <el-input-number v-model="rogueRule.confidenceThreshold" :min="1" :max="100" />
            </el-form-item>
            <el-form-item :label="t('security.threat.autoQuarantine')">
              <el-switch v-model="rogueRule.autoQuarantine" />
            </el-form-item>
          </el-form>
          <div class="rule-actions">
            <el-button type="primary" @click="saveRogueRule">{{ t('security.threat.saveConfig') }}</el-button>
            <el-button @click="resetRogueRule">{{ t('security.threat.resetDefault') }}</el-button>
          </div>
        </el-tab-pane>
      </el-tabs>
    </section>

    <section class="dispose-center">
      <div class="dispose-title">{{ t('security.threat.disposeTitle') }}</div>

      <div class="dispose-row">
        <el-card shadow="never" class="dispose-card">
          <template #header>
            <div class="dispose-card-header">
              <span>{{ t('security.threat.eventFlowTitle') }}</span>
              <el-button class="refresh-btn" size="small" :loading="loading" @click="handleModuleRefresh"
                >{{ t('security.threat.refresh') }}</el-button
              >
            </div>
          </template>
          <el-table :data="events" border stripe size="small">
            <template #empty>
              <div class="empty-lite">
                <el-empty :image-size="0" :description="t('security.threat.emptyRefreshHint')" />
              </div>
            </template>
            <el-table-column prop="type" :label="t('security.threat.colEventType')" min-width="130" />
            <el-table-column prop="sourceIp" :label="t('security.threat.colSourceIp')" min-width="140" />
            <el-table-column prop="port" :label="t('security.threat.colPort')" width="100" />
            <el-table-column prop="vlan" label="VLAN" width="90" />
            <el-table-column prop="score" :label="t('security.threat.colRiskScore')" width="100">
              <template #default="{ row }">
                <el-tag :type="riskType(row.score)">{{ row.score }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="occurredAt" :label="t('security.threat.colOccurredAt')" min-width="180">
              <template #default="{ row }">{{ formatTs(row.occurredAt) }}</template>
            </el-table-column>
          </el-table>
        </el-card>

        <el-card shadow="never" class="dispose-card">
          <template #header>
            <div class="dispose-card-header">
              <span>{{ t('security.threat.rogueDetectTitle', { count: selectedRogues.length }) }}</span>
              <el-button class="refresh-btn" size="small" :loading="loading" @click="handleModuleRefresh"
                >{{ t('security.threat.refresh') }}</el-button
              >
            </div>
          </template>
          <el-table
            :data="rogueServers"
            border
            stripe
            size="small"
            @selection-change="onSelectionChange"
          >
            <template #empty>
              <div class="empty-lite">
                <el-empty :image-size="0" :description="t('security.threat.emptyRefreshHint')" />
              </div>
            </template>
            <el-table-column type="selection" width="50" />
            <el-table-column prop="ip" label="IP" min-width="130" />
            <el-table-column prop="mac" label="MAC" min-width="150" />
            <el-table-column prop="vlan" label="VLAN" width="90" />
            <el-table-column prop="severity" :label="t('security.threat.colSeverity')" width="100">
              <template #default="{ row }">
                <el-tag :type="severityType(row.severity)">{{ row.severity }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="detectedAt" :label="t('security.threat.colDetectedAt')" min-width="180">
              <template #default="{ row }">{{ formatTs(row.detectedAt) }}</template>
            </el-table-column>
          </el-table>
        </el-card>
      </div>

      <div class="dispose-row">
        <el-card shadow="never" class="dispose-card">
          <template #header>
            <div class="dispose-card-header">
              <span>{{ t('security.threat.topoTitle') }}</span>
              <el-button class="refresh-btn" size="small" :loading="loading" @click="handleModuleRefresh"
                >{{ t('security.threat.refresh') }}</el-button
              >
            </div>
          </template>
          <el-table :data="topologyNodes" border stripe size="small">
            <template #empty>
              <div class="empty-lite">
                <el-empty :image-size="0" :description="t('security.threat.emptyRefreshHint')" />
              </div>
            </template>
            <el-table-column prop="label" :label="t('security.threat.colNode')" min-width="140" />
            <el-table-column prop="type" :label="t('security.threat.colType')" width="120" />
            <el-table-column prop="status" :label="t('security.threat.colStatus')" width="120">
              <template #default="{ row }">
                <el-tag :type="topoStatusType(row.status)">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
          </el-table>
        </el-card>

        <el-card shadow="never" class="dispose-card timeline-card">
          <template #header>
            <div class="dispose-card-header">
              <span>{{ t('security.threat.timelineTitle') }}</span>
              <el-button class="refresh-btn" size="small" :loading="loading" @click="handleModuleRefresh"
                >{{ t('security.threat.refresh') }}</el-button
              >
            </div>
          </template>
          <el-timeline>
            <el-timeline-item
              v-for="item in timelineData"
              :key="item.id"
              :timestamp="formatTs(item.occurredAt)"
              :type="riskType(item.score)"
              placement="top"
            >
              <div class="timeline-title">{{ item.type }}</div>
              <div class="timeline-desc">{{ item.description || t('security.threat.noDescription') }}</div>
              <div class="timeline-meta">{{ item.sourceIp || '-' }} / {{ item.port || '-' }}</div>
            </el-timeline-item>
            <div v-if="!timelineData.length" class="empty-lite">
              <el-empty :image-size="0" :description="t('security.threat.emptyRefreshHint')" />
            </div>
          </el-timeline>
        </el-card>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { showHttpError } from '@/shared/errors/errorToast';
import { showSuccess } from '@/shared/errors/messageToast';
import { useTenantStore } from '@/store/tenant';
import {
  listThreatEvents,
  listRogueServers,
  quarantineRogue,
  blockRogue,
  getTopology,
  saveRateLimit,
  getThreatRules,
  updateThreatRules
} from '@/api/security';
import type {
  ThreatEvent,
  RogueServerRecord,
  TopologyNode,
  ThreatRuleConfig
} from '@/types/security';
import { formatTs } from '@/utils/time';

const { t } = useI18n();
const tenantStore = useTenantStore();

const loading = ref(false);
const eventLoading = ref(false);
const topologyLoading = ref(false);
const rogueLoading = ref(false);
const rateSaving = ref(false);
const batchAction = ref<'quarantine' | 'block' | ''>('');
const rulesSaving = ref(false);

const events = ref<ThreatEvent[]>([]);
const rogueServers = ref<RogueServerRecord[]>([]);
const topologyNodes = ref<TopologyNode[]>([]);
const selectedRogues = ref<RogueServerRecord[]>([]);

const activeRuleTab = ref('spoofing');

const spoofRule = reactive({
  enabled: true,
  strictMode: true,
  blockThreshold: 80,
  autoBlock: true
});

const exhaustRule = reactive({
  enabled: true,
  windowSeconds: 60,
  attemptLimit: 120,
  autoRateLimit: true
});

const rogueRule = reactive({
  enabled: true,
  detectInterval: 30,
  confidenceThreshold: 70,
  autoQuarantine: true
});

const currentRulePayload = (): ThreatRuleConfig => ({
  spoofing: { ...spoofRule },
  exhaustion: { ...exhaustRule },
  rogue: { ...rogueRule }
});

const applyRules = (rules?: Partial<ThreatRuleConfig>) => {
  if (!rules) return;
  if (rules.spoofing) Object.assign(spoofRule, rules.spoofing);
  if (rules.exhaustion) Object.assign(exhaustRule, rules.exhaustion);
  if (rules.rogue) Object.assign(rogueRule, rules.rogue);
};

const loadThreatRules = async () => {
  try {
    const res = await getThreatRules(tenantParam());
    applyRules(res.data.data || undefined);
  } catch (error) {
    showHttpError(error, t('security.threat.loadRulesFail'));
  }
};

const saveThreatRulesConfig = async (successText: string) => {
  rulesSaving.value = true;
  try {
    await updateThreatRules({ ...currentRulePayload(), ...tenantParam() });
    showSuccess(successText);
  } catch (error) {
    showHttpError(error, t('security.saveFail'));
    throw error;
  } finally {
    rulesSaving.value = false;
  }
};

const spoofingCount = computed(() => events.value.filter((item) => item.type === 'spoofing').length);
const exhaustionCount = computed(() => events.value.filter((item) => item.type === 'exhaustion').length);
const rogueCount = computed(() => events.value.filter((item) => item.type === 'rogue-server').length);
const pendingCount = computed(() => events.value.filter((item) => item.score >= 70).length);

const timelineData = computed(() =>
  [...events.value]
    .sort((a, b) => new Date(b.occurredAt).getTime() - new Date(a.occurredAt).getTime())
    .slice(0, 8)
);

const tenantParam = () => ({ tenantId: tenantStore.currentTenantId || undefined });

const riskType = (score: number) => {
  if (score >= 80) return 'danger';
  if (score >= 60) return 'warning';
  return 'info';
};

const severityType = (severity: string) => {
  if (severity === 'high') return 'danger';
  if (severity === 'medium') return 'warning';
  return 'info';
};

const topoStatusType = (status: string) => {
  if (status === 'critical') return 'danger';
  if (status === 'warning') return 'warning';
  return 'success';
};

const loadEvents = async () => {
  eventLoading.value = true;
  try {
    const res = await listThreatEvents(tenantParam());
    events.value = res.data.data || [];
  } catch (error) {
    showHttpError(error, t('monitoring.loadFail'));
  } finally {
    eventLoading.value = false;
  }
};

const loadRogues = async () => {
  rogueLoading.value = true;
  try {
    const res = await listRogueServers(tenantParam());
    rogueServers.value = res.data.data || [];
    selectedRogues.value = [];
  } catch (error) {
    showHttpError(error, t('security.loadFailRogue'));
  } finally {
    rogueLoading.value = false;
  }
};

const loadTopology = async () => {
  topologyLoading.value = true;
  try {
    const res = await getTopology(tenantParam());
    topologyNodes.value = res.data.data?.nodes || [];
  } catch (error) {
    showHttpError(error, t('monitoring.loadFail'));
  } finally {
    topologyLoading.value = false;
  }
};

const loadAll = async () => {
  loading.value = true;
  await Promise.all([loadEvents(), loadRogues(), loadTopology(), loadThreatRules()]);
  loading.value = false;
};

const handleModuleRefresh = () => {
  void loadAll();
};

const onSelectionChange = (rows: RogueServerRecord[]) => {
  selectedRogues.value = rows;
};

const batchQuarantine = async () => {
  if (!selectedRogues.value.length) return;
  batchAction.value = 'quarantine';
  try {
    await Promise.all(selectedRogues.value.map((item) => quarantineRogue(item.id, tenantParam())));
    showSuccess(t('security.threat.batchQuarantineDone'));
    await loadRogues();
  } catch (error) {
    showHttpError(error, t('security.saveFail'));
  } finally {
    batchAction.value = '';
  }
};

const batchBlockAcl = async () => {
  if (!selectedRogues.value.length) return;
  batchAction.value = 'block';
  try {
    await Promise.all(selectedRogues.value.map((item) => blockRogue(item.id, tenantParam())));
    showSuccess(t('security.threat.batchBlockDone'));
    await loadRogues();
  } catch (error) {
    showHttpError(error, t('security.saveFail'));
  } finally {
    batchAction.value = '';
  }
};

const saveSpoofRule = async () => {
  await saveThreatRulesConfig(t('security.threat.spoofRuleSaved'));
};

const resetSpoofRule = async () => {
  Object.assign(spoofRule, {
    enabled: true,
    strictMode: true,
    blockThreshold: 80,
    autoBlock: true
  });
  await saveSpoofRule();
};

const saveExhaustRule = async () => {
  rateSaving.value = true;
  try {
    await saveRateLimit({
      scope: 'port',
      target: 'dhcp-exhaustion-window',
      limitPps: exhaustRule.attemptLimit,
      burst: Math.max(0, Math.floor(exhaustRule.attemptLimit / 2)),
      tenantId: tenantStore.currentTenantId || undefined
    });
    await saveThreatRulesConfig(t('security.threat.exhaustRuleSaved'));
  } catch (error) {
    showHttpError(error, t('security.saveFail'));
  } finally {
    rateSaving.value = false;
  }
};

const resetExhaustRule = async () => {
  Object.assign(exhaustRule, {
    enabled: true,
    windowSeconds: 60,
    attemptLimit: 120,
    autoRateLimit: true
  });
  await saveExhaustRule();
};

const saveRogueRule = async () => {
  await saveThreatRulesConfig(t('security.threat.rogueRuleSaved'));
};

const resetRogueRule = async () => {
  Object.assign(rogueRule, {
    enabled: true,
    detectInterval: 30,
    confidenceThreshold: 70,
    autoQuarantine: true
  });
  await saveRogueRule();
};

watch(
  () => tenantStore.currentTenantId,
  () => {
    loadAll();
  }
);

loadAll();
</script>

<style scoped>
.threat-center {
  --threat-card-bg: var(--el-bg-color);
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.command-bar {
  background: var(--threat-card-bg);
  border: 1px solid var(--el-border-color-light);
  border-radius: 12px;
  padding: 16px 18px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.command-text h2 {
  margin: 0;
  font-size: 22px;
}

.command-text p {
  margin: 6px 0 0;
  color: var(--el-text-color-secondary);
}

.command-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.stats-card {
  border-radius: 10px;
}

.stats-label {
  margin: 0;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.stats-value {
  margin-top: 8px;
  font-size: 30px;
  font-weight: 700;
}

.pending-card {
  background: linear-gradient(135deg, #7c3aed, #ef4444);
  color: var(--el-color-white);
}

.pending-card .stats-label {
  color: rgba(255, 255, 255, 0.86);
}

.rules-section,
.dispose-center {
  background: var(--threat-card-bg);
  border: 1px solid var(--el-border-color-light);
  border-radius: 12px;
  padding: 16px;
}

.rule-desc {
  margin: 0 0 12px;
  color: var(--el-text-color-regular);
}

.inline-form {
  margin-bottom: 12px;
}

.rule-actions {
  display: flex;
  gap: 8px;
}

.dispose-title {
  font-size: 18px;
  font-weight: 700;
  color: var(--el-text-color-primary);
  margin-bottom: 12px;
}

.dispose-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.dispose-row + .dispose-row {
  margin-top: 12px;
}

.dispose-card {
  border-radius: 10px;
}

.dispose-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}

.refresh-btn {
  margin-left: auto;
}

.empty-lite {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 10px 0;
}

.timeline-card {
  min-height: 360px;
}

.timeline-title {
  font-weight: 600;
}

.timeline-desc {
  margin-top: 4px;
}

.timeline-meta {
  margin-top: 4px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

@media (max-width: 1200px) {
  .stats-grid,
  .dispose-row {
    grid-template-columns: 1fr;
  }

  .command-bar {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
