<template>
  <div class="binding-batch">
    <section class="panel">
      <div class="panel-header">
        <div>
          <h3>{{ t('binding.tabBatch') }}</h3>
          <p class="panel-hint">
            {{ t('binding.batchHint', '使用模板、API 或脚本批量管理静态绑定。') }}
          </p>
        </div>
      </div>

      <div class="batch-grid">
        <BatchImport />
        <el-card shadow="hover">
          <template #header>{{ t('binding.apiTitle', 'REST API 批量接口') }}</template>
          <el-descriptions :column="1" border>
            <el-descriptions-item :label="'POST /api/bindings/batch'">{{
              t('binding.apiBatchDesc', '批量导入或更新静态绑定。')
            }}</el-descriptions-item>
            <el-descriptions-item :label="'POST /api/bindings/batch-delete'">{{
              t('binding.apiDeleteDesc', '按条件批量清理静态绑定。')
            }}</el-descriptions-item>
            <el-descriptions-item :label="t('binding.apiScopes', '所需权限')"
              >binding.manage</el-descriptions-item
            >
          </el-descriptions>
          <pre class="code">
POST /api/bindings/batch
Authorization: Bearer &lt;token&gt;
Content-Type: application/json
{
  "items": [
    { "mac": "AA-BB-CC-00-11-22", "ip": "10.10.2.10", "hostname": "voice-01" }
  ]
}
          </pre>
        </el-card>
        <el-card shadow="hover">
          <template #header>{{ t('binding.cliTitle', '脚本与 CLI') }}</template>
          <p>{{ t('binding.cliDesc', '在自动化平台或本地终端执行批量同步。') }}</p>
          <pre class="code">
mdhcp bindings import --file bindings.csv --tenant TENANT_A
mdhcp bindings export --format csv --out export.csv
          </pre>
          <el-alert type="info" :closable="false">
            {{ t('binding.cliHint', '可通过计划任务/流水线触发，配合 API Token 管理。') }}
          </el-alert>
        </el-card>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import BatchImport from './components/BatchImport.vue';

const { t } = useI18n();
</script>

<style scoped>
.binding-batch {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.panel {
  background: var(--el-fill-color-blank);
  padding: 16px;
  border-radius: 12px;
  box-shadow: var(--el-box-shadow-light);
}

.panel-header {
  display: flex;
  justify-content: space-between;
}

.panel-hint {
  margin: 4px 0 0;
  color: var(--el-text-color-secondary);
}

.batch-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 16px;
}

.code {
  font-family: 'Fira Code', 'Courier New', monospace;
  font-size: 12px;
  background: var(--el-fill-color-light);
  padding: 12px;
  border-radius: 8px;
  white-space: pre-wrap;
}
</style>
