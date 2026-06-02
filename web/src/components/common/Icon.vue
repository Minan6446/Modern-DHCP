<template>
  <span class="icon" :style="styleObject">
    <slot>
      <span class="icon-fallback">{{ name }}</span>
    </slot>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(defineProps<{ name?: string; size?: number }>(), {
  name: '',
  size: 16
});

const styleObject = computed(() => ({
  fontSize: `${props.size}px`,
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center'
}));
</script>

<style scoped>
.icon {
  line-height: 1;
}
.icon-fallback {
  opacity: 0.6;
  font-weight: 600;
}
</style>
