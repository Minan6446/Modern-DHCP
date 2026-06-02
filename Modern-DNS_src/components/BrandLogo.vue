<script setup lang="ts">
/**
 * Modern DNS brand mark — "Particle stream" iteration.
 *
 * Design language (per product brief):
 *   - Symmetric, minimalist, tech-forward DNS service mark
 *   - Glowing diamond at the very centre = the resolver core
 *   - 16 small particles (8 axes × 2 radii) radiate uniformly
 *     outward; large/bright near the core, small/dim at the
 *     perimeter — the same gradient reads simultaneously as
 *     "queries converging inward" and "responses fanning
 *     outward" depending on which direction the eye tracks
 *   - Monochrome blue→cyan gradient on a deep navy backdrop;
 *     no orange, no text — pure icon
 *   - Vector / flat / fully scalable; holds up from 16px to 256px
 *
 * Geometry on a 64×64 design grid centred at (32, 32):
 *   - 8 axes at 45° intervals
 *   - inner ring (r=15): 8 dots, larger + brighter
 *   - outer ring (r=22): 8 dots, smaller + dimmer
 *   - centre diamond: 9-unit half-diagonal rhombus with
 *     linear cyan→azure gradient and a soft radial halo
 *
 * Props:
 *   - `size`  : any CSS length-compatible number (default 40)
 *   - `solid` : whether to paint the dark rounded backdrop. Turn
 *               off when embedding in already-coloured surfaces.
 */
defineProps<{ size?: number | string; solid?: boolean }>()
</script>

<template>
  <svg
    :width="size ?? 40"
    :height="size ?? 40"
    viewBox="0 0 64 64"
    xmlns="http://www.w3.org/2000/svg"
    role="img"
    aria-label="Modern DNS"
    class="brand-logo"
  >
    <!-- ─── Reusable paint definitions ───────────────────────
         Gradient IDs are local to the component; if a page
         renders the mark multiple times the browser dedupes
         on the `<defs>` so the cost stays constant.
    -->
    <defs>
      <!-- Backdrop: deep navy with a soft radial lift around
           the resolver core so the cyan diamond never sits on
           a flat black field. -->
      <radialGradient id="bl-bg" cx="50%" cy="50%" r="70%">
        <stop offset="0%" stop-color="#0E2547" />
        <stop offset="100%" stop-color="#04122A" />
      </radialGradient>
      <!-- Diamond core: bright cyan top-left → azure bottom-right.
           Direction matches the dominant light source so the
           rhombus reads as a faceted crystal, not a flat shape. -->
      <linearGradient id="bl-core" x1="0%" y1="0%" x2="100%" y2="100%">
        <stop offset="0%" stop-color="#7CF3FF" />
        <stop offset="100%" stop-color="#1AB6FF" />
      </linearGradient>
      <!-- Halo behind the diamond. Pure radial cyan with full
           transparency at the edge so it blends into whatever
           backdrop it's drawn over. -->
      <radialGradient id="bl-halo" cx="50%" cy="50%" r="50%">
        <stop offset="0%" stop-color="#7CF3FF" stop-opacity="0.55" />
        <stop offset="60%" stop-color="#1AB6FF" stop-opacity="0.18" />
        <stop offset="100%" stop-color="#1AB6FF" stop-opacity="0" />
      </radialGradient>
    </defs>

    <rect
      v-if="solid !== false"
      width="64"
      height="64"
      rx="14"
      fill="url(#bl-bg)"
    />

    <!-- ─── Particle field ───────────────────────────────────
         Two concentric rings of cyan dots on 8 evenly-spaced
         axes. Inner ring (r=15) is brighter and larger; outer
         ring (r=22) is dimmer and smaller. The size + alpha
         falloff is what makes the eye read both an *inward*
         flow (small far → big near = "queries arriving") and
         an *outward* flow (big near → small far = "responses
         dispersing") simultaneously.
    -->
    <g fill="#7CF3FF">
      <!-- Inner ring (r=15) — brighter / larger -->
      <circle cx="47"     cy="32"    r="2"   fill-opacity="0.95" />
      <circle cx="42.61"  cy="42.61" r="2"   fill-opacity="0.95" />
      <circle cx="32"     cy="47"    r="2"   fill-opacity="0.95" />
      <circle cx="21.39"  cy="42.61" r="2"   fill-opacity="0.95" />
      <circle cx="17"     cy="32"    r="2"   fill-opacity="0.95" />
      <circle cx="21.39"  cy="21.39" r="2"   fill-opacity="0.95" />
      <circle cx="32"     cy="17"    r="2"   fill-opacity="0.95" />
      <circle cx="42.61"  cy="21.39" r="2"   fill-opacity="0.95" />
      <!-- Outer ring (r=22) — dimmer / smaller -->
      <circle cx="54"     cy="32"    r="1.1" fill-opacity="0.5" />
      <circle cx="47.56"  cy="47.56" r="1.1" fill-opacity="0.5" />
      <circle cx="32"     cy="54"    r="1.1" fill-opacity="0.5" />
      <circle cx="16.44"  cy="47.56" r="1.1" fill-opacity="0.5" />
      <circle cx="10"     cy="32"    r="1.1" fill-opacity="0.5" />
      <circle cx="16.44"  cy="16.44" r="1.1" fill-opacity="0.5" />
      <circle cx="32"     cy="10"    r="1.1" fill-opacity="0.5" />
      <circle cx="47.56"  cy="16.44" r="1.1" fill-opacity="0.5" />
    </g>

    <!-- ─── Resolver core ────────────────────────────────────
         Soft halo first (so the diamond reads as luminous
         rather than pasted), then the cyan-gradient rhombus,
         then a single white pixel at dead-centre to reinforce
         the bright-spot read at very small sizes (16–24px
         where the gradient itself can't be resolved).
    -->
    <circle cx="32" cy="32" r="11" fill="url(#bl-halo)" />
    <path
      d="M32 23 L41 32 L32 41 L23 32 Z"
      fill="url(#bl-core)"
    />
    <circle cx="32" cy="32" r="1.4" fill="#FFFFFF" />
  </svg>
</template>

<style scoped>
.brand-logo {
  display: block;
  flex-shrink: 0;
}
</style>
