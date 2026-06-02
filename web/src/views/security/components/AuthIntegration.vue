<template>
  <el-card shadow="never">
    <template #header>
      <div class="card-header">
        <span>{{ t('security.comp.authTitle') }}</span>
        <el-button size="small" type="primary" @click="save">{{ t('security.comp.authSave') }}</el-button>
      </div>
    </template>
    <el-row :gutter="12">
      <el-col :span="8">
        <div class="section-title">{{ t('security.comp.dot1xTitle') }}</div>
        <el-table :data="dot1xProfiles" size="small" border height="240">
          <el-table-column prop="name" :label="t('security.comp.dot1xColName')" />
          <el-table-column prop="mode" :label="t('security.comp.dot1xColMode')" width="120" />
          <el-table-column prop="reauthInterval" :label="t('security.comp.dot1xColReauth')" width="110" />
        </el-table>
      </el-col>
      <el-col :span="8">
        <div class="section-title">{{ t('security.comp.macAuthTitle') }}</div>
        <el-table :data="macAuthProfiles" size="small" border height="240">
          <el-table-column prop="name" :label="t('security.comp.macAuthColName')" />
          <el-table-column prop="authList" :label="t('security.comp.macAuthColAcl')" />
          <el-table-column prop="fallbackVlan" :label="t('security.comp.macAuthColFallback')" width="110" />
        </el-table>
      </el-col>
      <el-col :span="8">
        <div class="section-title">{{ t('security.comp.radiusTitle') }}</div>
        <el-table :data="mappings" size="small" border height="240">
          <el-table-column prop="attribute" :label="t('security.comp.radiusColAttr')" />
          <el-table-column prop="localField" :label="t('security.comp.radiusColLocal')" />
          <el-table-column prop="transform" :label="t('security.comp.radiusColTransform')" />
          <el-table-column width="90" :label="t('security.comp.radiusColAction')">
            <template #default="{ row }">
              <el-button link type="danger" @click="remove(row)">{{ t('security.comp.radiusDelete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-button class="mt-2" size="small" type="primary" @click="addMapping">{{ t('security.comp.radiusAdd') }}</el-button>
      </el-col>
    </el-row>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  listDot1xProfiles,
  listMacAuthProfiles,
  listRadiusMappings,
  saveDot1xProfile,
  saveMacAuthProfile,
  saveRadiusMapping,
  deleteRadiusMapping
} from '@/api/security';
import type { Dot1xProfile, MacAuthProfile, RadiusMapping } from '@/types/security';

const { t } = useI18n();

const dot1xProfiles = ref<Dot1xProfile[]>([]);
const macAuthProfiles = ref<MacAuthProfile[]>([]);
const mappings = ref<RadiusMapping[]>([]);

const load = async () => {
  try {
    const { data } = await listDot1xProfiles();
    dot1xProfiles.value = data.data;
  } catch (e) {
    dot1xProfiles.value = [
      { id: 'd1', name: 'Office', mode: 'single', reauthInterval: 3600, eapTypes: ['peap', 'tls'] },
      { id: 'd2', name: 'Guest', mode: 'multi-auth', reauthInterval: 1800, eapTypes: ['peap'] }
    ];
  }

  try {
    const { data } = await listMacAuthProfiles();
    macAuthProfiles.value = data.data;
  } catch (e) {
    macAuthProfiles.value = [
      { id: 'm1', name: 'IoT Whitelist', authList: 'mac-acl-iot', fallbackVlan: 200 },
      { id: 'm2', name: 'Printer', authList: 'mac-acl-printer', fallbackVlan: 30 }
    ];
  }

  try {
    const { data } = await listRadiusMappings();
    mappings.value = data.data;
  } catch (e) {
    mappings.value = [
      { id: 'r1', attribute: 'Filter-Id', localField: 'policyName', transform: 'lower' },
      { id: 'r2', attribute: 'Tunnel-Private-Group-ID', localField: 'vlan', transform: 'toInt' }
    ];
  }
};

const save = async () => {
  // Persist current selections (no detailed form provided, placeholder calls)
  if (dot1xProfiles.value.length) await saveDot1xProfile(dot1xProfiles.value[0]);
  if (macAuthProfiles.value.length) await saveMacAuthProfile(macAuthProfiles.value[0]);
  if (mappings.value.length) await saveRadiusMapping(mappings.value[0]);
};

const addMapping = () => {
  mappings.value.push({
    id: `tmp-${Date.now()}`,
    attribute: 'Reply-Message',
    localField: 'note',
    transform: 'none'
  });
};

const remove = async (row: RadiusMapping) => {
  if (row.id?.startsWith('tmp-')) {
    mappings.value = mappings.value.filter((m) => m.id !== row.id);
    return;
  }
  await deleteRadiusMapping(row.id);
  load();
};

onMounted(load);
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.section-title {
  font-weight: 600;
  margin-bottom: 6px;
}

.mt-2 {
  margin-top: 8px;
}
</style>
