<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string
    subtitle?: string
    compact?: boolean
    hideCopy?: boolean
  }>(),
  {
    subtitle: '',
    compact: false,
    hideCopy: false,
  },
)
</script>

<template>
  <div class="brand-identity" :class="{ compact }">
    <div class="brand-badge" aria-hidden="true">
      <span class="brand-badge-ring"></span>
      <span class="brand-badge-core">DNS</span>
    </div>
    <div v-if="!hideCopy" class="brand-copy">
      <strong>{{ title }}</strong>
      <span v-if="subtitle">{{ subtitle }}</span>
    </div>
  </div>
</template>

<style scoped>
.brand-identity {
  display: inline-flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}

.brand-identity.compact {
  gap: 12px;
}

.brand-badge {
  position: relative;
  display: grid;
  flex: 0 0 auto;
  width: 48px;
  height: 48px;
  place-items: center;
  border-radius: 16px;
  background: linear-gradient(145deg, #0e42d2 0%, var(--app-accent) 58%, #59a8ff 100%);
  box-shadow: 0 14px 30px rgba(22, 93, 255, 0.28);
  overflow: hidden;
}

.brand-identity.compact .brand-badge {
  width: 36px;
  height: 36px;
  border-radius: 12px;
  box-shadow: 0 10px 18px rgba(22, 93, 255, 0.2);
}

.brand-badge-ring {
  position: absolute;
  inset: 6px;
  border: 1px solid rgba(255, 255, 255, 0.3);
  border-radius: 12px;
}

.brand-identity.compact .brand-badge-ring {
  inset: 4px;
  border-radius: 9px;
}

.brand-badge-core {
  position: relative;
  z-index: 1;
  color: var(--el-color-white);
  font-size: 16px;
  font-weight: 800;
  letter-spacing: 0.08em;
}

.brand-identity.compact .brand-badge-core {
  font-size: 12px;
}

.brand-copy {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.brand-copy strong {
  color: var(--app-title);
  font-size: 18px;
  font-weight: 700;
  line-height: 1.2;
}

.brand-copy span {
  margin-top: 3px;
  color: var(--app-text-regular);
  font-size: 12px;
  line-height: 1.3;
}

.brand-identity.compact .brand-copy strong {
  font-size: 15px;
}

.brand-identity.compact .brand-copy span {
  margin-top: 2px;
  font-size: 11px;
}
</style>