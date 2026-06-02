<template>
  <el-card shadow="hover">
    <div class="panel-header">{{ t('binding.batchTitle') }}</div>
    <div class="actions">
      <el-button @click="downloadTemplate('csv')">{{ t('binding.downloadCsv') }}</el-button>
      <el-button @click="downloadTemplate('xlsx')">{{ t('binding.downloadXlsx') }}</el-button>
      <el-upload :auto-upload="false" :show-file-list="false" :on-change="handleFile" accept=".csv">
        <el-button type="primary" :disabled="!permissionStore.can('binding.manage')">{{
          t('binding.upload')
        }}</el-button>
      </el-upload>
    </div>
    <el-progress v-if="progress > 0 && progress < 100" :percentage="progress" />
    <el-table :data="preview" height="240" border :empty-text="t('binding.previewValidate')">
      <el-table-column prop="mac" :label="t('binding.formMac')" />
      <el-table-column prop="ip" :label="t('binding.formIp')" />
      <el-table-column prop="hostname" :label="t('binding.formHostname')" />
      <el-table-column :label="t('binding.formDesc')">
        <template #default="{ row }">{{ row.description }}</template>
      </el-table-column>
      <el-table-column :label="t('binding.previewValidate')">
        <template #default="{ row }">
          <el-tag :type="row.valid ? 'success' : 'danger'">{{
            row.valid ? t('binding.valid') : row.error
          }}</el-tag>
        </template>
      </el-table-column>
    </el-table>
    <div class="actions">
      <el-button type="primary" :disabled="!canImport" @click="submit">{{
        t('binding.import')
      }}</el-button>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { UploadFile } from 'element-plus';
import { showError, showInfo, showSuccess } from '@/shared/errors/messageToast';
import { parseCsv, downloadCsv } from '@/utils/csv';
import { isMac, formatMac } from '@/utils/mac';
import { isIPv4, isIPv6 } from '@/utils/ip';
import { batchImportBindings } from '@/api/bindings';
import type { BatchImportItem } from '@/types/binding';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { useI18n } from 'vue-i18n';

const preview = ref<BatchImportItem[]>([]);
const progress = ref(0);
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const { t } = useI18n();

const can = (key: string) => (permissionStore.can ? permissionStore.can(key) : true);

const canImport = computed(
  () => preview.value.length > 0 && preview.value.every((i) => i.valid) && can('binding.manage')
);

const downloadTemplate = (type: 'csv' | 'xlsx') => {
  const headers = ['mac', 'ip', 'hostname', 'description'];
  if (type === 'csv') downloadCsv('bindings_template.csv', headers.join(',') + '\n');
  else showInfo(t('binding.downloadXlsx'));
};

const handleFile = (file: UploadFile) => {
  if (!can('binding.manage')) {
    showError(t('binding.noPermission'));
    return;
  }
  progress.value = 0;
  preview.value = [];
  const fileName = file.name || (file.raw as any)?.name || 'upload.csv';
  const fileSize = file.size ?? file.raw?.size ?? 0;
  if (fileSize > 2 * 1024 * 1024) {
    showError(t('binding.fileTooLarge'));
    return;
  }
  if (!fileName.toLowerCase().endsWith('.csv')) {
    showError(t('binding.fileTypeInvalid'));
    return;
  }
  if (!file.raw) {
    showError(t('binding.importFail'));
    return;
  }
  const reader = new FileReader();
  reader.onload = () => {
    try {
      const text = reader.result?.toString() || '';
      const rows = parseCsv(text);
      if (!rows.length) {
        showError(t('binding.invalidRows'));
        return;
      }
      const header = rows[0].map((h) => h.toLowerCase()).join(',');
      if (!header.startsWith('mac,ip')) {
        showError(t('binding.headerInvalid'));
        return;
      }
      const seenMac = new Set<string>();
      preview.value = rows.slice(1).map((r) => {
        const item: BatchImportItem = {
          mac: formatMac(r[0] || ''),
          ip: (r[1] || '').trim(),
          hostname: r[2],
          description: r[3],
          valid: true
        };
        if (!isMac(item.mac)) {
          item.valid = false;
          item.error = t('binding.macInvalid');
        } else if (seenMac.has(item.mac)) {
          item.valid = false;
          item.error = t('binding.duplicateMac');
        }
        seenMac.add(item.mac);

        if (!item.ip) {
          item.valid = false;
          item.error = t('binding.ipRequired');
        } else if (!isIPv4(item.ip) && !isIPv6(item.ip)) {
          item.valid = false;
          item.error = t('binding.ipInvalid');
        }

        return item;
      });
      if (preview.value.some((p) => !p.valid)) {
        showError(t('binding.invalidRows'));
      }
      progress.value = 100;
    } catch (e) {
      showError(t('binding.importFail'));
      progress.value = 0;
    }
  };
  reader.onerror = () => {
    showError(t('binding.importFail'));
    progress.value = 0;
  };
  reader.readAsText(file.raw);
};

const submit = async () => {
  if (!can('binding.manage')) {
    showError(t('binding.noPermission'));
    return;
  }
  const validItems = preview.value.filter((i) => i.valid).map(({ error, valid, ...rest }) => rest);
  if (!validItems.length) {
    showError(t('binding.invalidRows'));
    return;
  }
  try {
    const { data } = await batchImportBindings(validItems, {
      tenantId: tenantStore.currentTenantId
    });
    showSuccess(
      t('binding.importSuccess', { success: data.data.success, failed: data.data.failed })
    );
  } catch (e) {
    showError(t('binding.importFail'));
  }
};

watch(
  () => tenantStore.currentTenantId,
  () => {
    preview.value = [];
    progress.value = 0;
  }
);
</script>

<style scoped>
.actions {
  display: flex;
  gap: 8px;
  margin: 8px 0;
}
</style>
