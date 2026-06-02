<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'

const props = defineProps({
  option: {
    type: Object,
    default: () => ({}),
  },
  height: {
    type: String,
    default: '320px',
  },
  loading: {
    type: Boolean,
    default: false,
  },
  empty: {
    type: Boolean,
    default: false,
  },
})

const appStore = useAppStore()
const { t } = useI18n()
const chartRef = ref(null)
const echartsModule = ref(null)
let chartInstance

const getCssColor = (name: string, fallback: string): string => {
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return value || fallback
}

const chartColors = computed(() => [
  getCssColor('--app-chart-1', '#165DFF'),
  getCssColor('--app-chart-2', '#00B42A'),
  getCssColor('--app-chart-3', '#FF7D00'),
  getCssColor('--app-chart-4', '#F53F3F'),
  getCssColor('--app-chart-5', '#722ED1'),
  getCssColor('--app-chart-6', '#14C9C9'),
  getCssColor('--app-chart-7', '#FF9A2E'),
  getCssColor('--app-chart-8', '#B41EFF'),
])

const buildOption = () => ({
  grid: { left: 24, right: 24, top: 42, bottom: 24, containLabel: true },
  ...props.option,
  color: chartColors.value,
  tooltip: {
    trigger: 'axis',
    ...(props.option as any).tooltip,
    backgroundColor: getCssColor('--app-chart-tooltip-bg', appStore.theme === 'dark' ? '#1F1F1F' : '#FFFFFF'),
    borderColor: getCssColor('--app-chart-tooltip-border', appStore.theme === 'dark' ? '#3A3A3A' : '#E5E6EB'),
    textStyle: {
      color: getCssColor('--app-chart-tooltip-text', appStore.theme === 'dark' ? '#E5E6EB' : '#1D2129'),
    },
  },
})

const resizeChart = () => {
  chartInstance?.resize()
}

const ensureEcharts = async () => {
  if (!echartsModule.value) {
    const module = await import('../utils/echarts')
    echartsModule.value = module.echarts
  }
  return echartsModule.value
}

const renderChart = async () => {
  if (!chartRef.value || props.empty) {
    return
  }
  await nextTick()
  const echarts = await ensureEcharts()
  if (!chartInstance) {
    chartInstance = echarts.init(chartRef.value)
  }
  if (props.loading) {
    chartInstance.showLoading('default', { text: t('component.baseChart.loading') })
  } else {
    chartInstance.hideLoading()
  }
  chartInstance.setOption(buildOption(), true)
}

watch(() => props.option, renderChart, { deep: true })
watch(() => props.loading, renderChart)
watch(() => props.empty, renderChart)
watch(() => appStore.theme, () => {
  chartInstance?.dispose()
  chartInstance = null
  renderChart()
})

onMounted(() => {
  renderChart()
  window.addEventListener('resize', resizeChart)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', resizeChart)
  chartInstance?.dispose()
})
</script>

<template>
  <div :style="{ height, width: '100%' }">
    <el-empty v-if="empty" class="empty-block" :description="$t('component.baseChart.empty')" />
    <div v-else ref="chartRef" :style="{ height: '100%', width: '100%' }" />
  </div>
</template>
