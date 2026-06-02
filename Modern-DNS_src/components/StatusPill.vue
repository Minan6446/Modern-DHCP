<script setup lang="ts">
/**
 * StatusPill — single canonical status badge for the whole admin UI.
 *
 * Replaces the four ad-hoc patterns previously scattered across pages:
 *   1) <el-tag type="success">              (Element default, very tall)
 *   2) <span class="mn-badge mn-badge--*">  (soft tinted, custom CSS)
 *   3) <span class="type-badge" :style>     (solid color via inline style)
 *   4) <span class="status-dot"> + label    (colored dot + free text)
 *
 * Props
 * ─────
 *   intent  — semantic colour bucket. Defaults to "neutral" so unknown
 *             status strings render gracefully instead of throwing.
 *   variant — "soft" (default, low chroma background + colored text),
 *             "solid" (filled background, white text — for emphasis),
 *             "outline" (transparent background + colored text+border).
 *   dot     — show a leading 6px coloured dot. Useful inside dense
 *             tables where the whole pill would steal too much weight.
 *   size    — "sm" (default, 12px) or "xs" (11px) for ultra-compact rows.
 *
 * The displayed text comes from the default slot so callers retain
 * control over i18n (this component must NOT call useI18n itself).
 */

defineOptions({ name: 'StatusPill' })

withDefaults(
  defineProps<{
    intent?: 'success' | 'warning' | 'danger' | 'accent' | 'neutral'
    variant?: 'soft' | 'solid' | 'outline'
    dot?: boolean
    size?: 'sm' | 'xs'
  }>(),
  { intent: 'neutral', variant: 'soft', dot: false, size: 'sm' },
)
</script>

<template>
  <span
    class="status-pill"
    :class="[
      `status-pill--${intent}`,
      `status-pill--${variant}`,
      `status-pill--${size}`,
    ]"
  >
    <span v-if="dot" class="status-pill__dot" aria-hidden="true"></span>
    <slot />
  </span>
</template>

<style scoped>
.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border: 1px solid transparent;
  border-radius: 4px;
  font-weight: 500;
  line-height: 1.4;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

.status-pill--sm { font-size: 12px; }
.status-pill--xs { font-size: 11px; padding: 1px 6px; }

.status-pill__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  flex-shrink: 0;
}

/* ── Soft (default) — 10% tint background, coloured text ── */
.status-pill--soft.status-pill--success { background: rgba(22, 163, 74, 0.10); color: var(--app-success); }
.status-pill--soft.status-pill--warning { background: rgba(217, 119, 6, 0.12); color: var(--app-warning); }
.status-pill--soft.status-pill--danger  { background: rgba(220, 38, 38, 0.10); color: var(--app-danger); }
.status-pill--soft.status-pill--accent  { background: var(--app-accent-soft); color: var(--app-accent); }
.status-pill--soft.status-pill--neutral { background: rgba(134, 144, 156, 0.10); color: var(--app-text-regular); }

/* ── Solid — filled, white text. Reserve for high-emphasis status. ── */
.status-pill--solid { color: #fff; }
.status-pill--solid.status-pill--success { background: var(--app-success); }
.status-pill--solid.status-pill--warning { background: var(--app-warning); }
.status-pill--solid.status-pill--danger  { background: var(--app-danger); }
.status-pill--solid.status-pill--accent  { background: var(--app-accent); }
.status-pill--solid.status-pill--neutral { background: var(--app-disabled); }

/* ── Outline — transparent, ring + text. Good for "selected / current" badges. ── */
.status-pill--outline { background: transparent; }
.status-pill--outline.status-pill--success { color: var(--app-success); border-color: var(--app-success); }
.status-pill--outline.status-pill--warning { color: var(--app-warning); border-color: var(--app-warning); }
.status-pill--outline.status-pill--danger  { color: var(--app-danger);  border-color: var(--app-danger); }
.status-pill--outline.status-pill--accent  { color: var(--app-accent);  border-color: var(--app-accent); }
.status-pill--outline.status-pill--neutral { color: var(--app-text-regular); border-color: var(--app-border); }
</style>
