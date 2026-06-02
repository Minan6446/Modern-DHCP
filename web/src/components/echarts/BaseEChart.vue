<template>
  <div ref="chartRef" class="echart" />
</template>

<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch, shallowRef } from 'vue';

type GenericRecord = Record<string, any>;

let echartsLoader: Promise<typeof import('echarts/core')> | null = null;

const loadEcharts = async () => {
  if (!echartsLoader) {
    echartsLoader = (async () => {
      const core = await import('echarts/core');
      const charts = await import('echarts/charts');
      const components = await import('echarts/components');
      const renderer = await import('echarts/renderers');

      core.use([
        charts.BarChart,
        charts.LineChart,
        charts.PieChart,
        charts.RadarChart,
        charts.GaugeChart,
        charts.GraphChart,
        components.GridComponent,
        components.TooltipComponent,
        components.LegendComponent,
        components.TitleComponent,
        components.DatasetComponent,
        components.VisualMapComponent,
        renderer.CanvasRenderer
      ]);

      return core;
    })();
  }
  return echartsLoader;
};

const props = defineProps<{ option: any; theme?: string; autoresize?: boolean; lazy?: boolean }>();

const chartRef = ref<HTMLDivElement>();
const chart = shallowRef<any>();
let visibilityObserver: IntersectionObserver | null = null;
let resizeFrame = 0;

const isObject = (value: unknown): value is GenericRecord =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

const cloneValue = <T>(value: T): T => {
  if (Array.isArray(value)) {
    return value.map((item) => cloneValue(item)) as T;
  }
  if (isObject(value)) {
    const result: GenericRecord = {};
    Object.entries(value).forEach(([key, item]) => {
      result[key] = cloneValue(item);
    });
    return result as T;
  }
  return value;
};

const mergeObject = (target: GenericRecord, source: GenericRecord) => {
  Object.entries(source).forEach(([key, sourceValue]) => {
    if (sourceValue === undefined) return;
    const targetValue = target[key];
    if (isObject(targetValue) && isObject(sourceValue)) {
      mergeObject(targetValue, sourceValue);
      return;
    }
    target[key] = cloneValue(sourceValue);
  });
};

const currentThemeTokens = () => {
  if (typeof window === 'undefined') {
    return {
      text: '#1F2937',
      textSecondary: '#6B7280',
      bg: 'rgba(255,255,255,0.97)',
      border: 'rgba(15, 23, 42, 0.18)',
      shadow: '0 8px 20px rgba(15, 23, 42, 0.12)'
    };
  }
  const style = getComputedStyle(document.documentElement);
  const text = style.getPropertyValue('--el-text-color-primary').trim() || '#1F2937';
  const textSecondary = style.getPropertyValue('--el-text-color-secondary').trim() || '#6B7280';
  const bg = style.getPropertyValue('--el-bg-color-overlay').trim() || 'rgba(255,255,255,0.97)';
  const borderToken = style.getPropertyValue('--el-border-color').trim() || 'rgba(31,41,55,0.2)';
  return {
    text,
    textSecondary,
    bg,
    border: borderToken || 'rgba(15, 23, 42, 0.18)',
    shadow: '0 8px 20px rgba(15, 23, 42, 0.12)'
  };
};

const resolveViewport = () => {
  const width = chartRef.value?.clientWidth || window.innerWidth || 1200;
  if (width <= 768) {
    return {
      width,
      grid: { left: 28, right: 20, top: 44, bottom: 52, containLabel: true },
      labelWidth: 58,
      labelRotate: 30,
      legendType: 'scroll' as const,
      legendTop: 8,
      legendRight: 8,
      legendLeft: 8,
      tooltipMaxWidth: 220,
      fixedTooltip: true
    };
  }
  if (width <= 1200) {
    return {
      width,
      grid: { left: 44, right: 24, top: 42, bottom: 44, containLabel: true },
      labelWidth: 78,
      labelRotate: 20,
      legendType: 'scroll' as const,
      legendTop: 8,
      legendRight: 12,
      legendLeft: 12,
      tooltipMaxWidth: 280,
      fixedTooltip: false
    };
  }
  if (width <= 1920) {
    return {
      width,
      grid: { left: 48, right: 26, top: 40, bottom: 34, containLabel: true },
      labelWidth: 96,
      labelRotate: 0,
      legendType: 'plain' as const,
      legendTop: 8,
      legendRight: 14,
      legendLeft: 14,
      tooltipMaxWidth: 320,
      fixedTooltip: false
    };
  }
  return {
    width,
    grid: { left: 52, right: 28, top: 40, bottom: 30, containLabel: true },
    labelWidth: 120,
    labelRotate: 0,
    legendType: 'plain' as const,
    legendTop: 8,
    legendRight: 18,
    legendLeft: 18,
    tooltipMaxWidth: 360,
    fixedTooltip: false
  };
};

const normalizeGrid = (option: GenericRecord, viewport: ReturnType<typeof resolveViewport>) => {
  if (!('grid' in option) && !('xAxis' in option) && !('yAxis' in option)) {
    return;
  }
  if (Array.isArray(option.grid)) {
    option.grid = option.grid.map((item: any) => {
      const next = isObject(item) ? cloneValue(item) : {};
      if (!isObject(next)) return viewport.grid;
      mergeObject(next, { containLabel: true });
      if (next.left === undefined) next.left = viewport.grid.left;
      if (next.right === undefined) next.right = viewport.grid.right;
      if (next.top === undefined) next.top = viewport.grid.top;
      if (next.bottom === undefined) next.bottom = viewport.grid.bottom;
      return next;
    });
    return;
  }
  if (!isObject(option.grid)) {
    option.grid = cloneValue(viewport.grid);
    return;
  }
  mergeObject(option.grid, { containLabel: true });
  if (option.grid.left === undefined) option.grid.left = viewport.grid.left;
  if (option.grid.right === undefined) option.grid.right = viewport.grid.right;
  if (option.grid.top === undefined) option.grid.top = viewport.grid.top;
  if (option.grid.bottom === undefined) option.grid.bottom = viewport.grid.bottom;
};

const normalizeXAxis = (option: GenericRecord, viewport: ReturnType<typeof resolveViewport>, tokens: ReturnType<typeof currentThemeTokens>) => {
  if (!option.xAxis) return;
  const apply = (axisInput: any) => {
    const axis = isObject(axisInput) ? axisInput : {};
    if (!isObject(axis.axisLabel)) axis.axisLabel = {};
    if (axis.axisLabel.hideOverlap === undefined) axis.axisLabel.hideOverlap = true;
    if (axis.axisLabel.showMinLabel === undefined) axis.axisLabel.showMinLabel = true;
    if (axis.axisLabel.showMaxLabel === undefined) axis.axisLabel.showMaxLabel = true;
    if (axis.axisLabel.overflow === undefined) axis.axisLabel.overflow = 'truncate';
    if (axis.axisLabel.width === undefined) axis.axisLabel.width = viewport.labelWidth;
    if (axis.axisLabel.margin === undefined) axis.axisLabel.margin = 10;
    if (axis.axisLabel.rotate === undefined && viewport.width <= 1200) axis.axisLabel.rotate = viewport.labelRotate;
    if (axis.axisLabel.color === undefined) axis.axisLabel.color = tokens.textSecondary;
    if (axis.axisTick === undefined) axis.axisTick = { alignWithLabel: true };
    if (axis.axisLine === undefined) axis.axisLine = { lineStyle: { color: 'rgba(148, 163, 184, 0.32)' } };
    return axis;
  };
  if (Array.isArray(option.xAxis)) {
    option.xAxis = option.xAxis.map((axis: any) => apply(axis));
    return;
  }
  option.xAxis = apply(option.xAxis);
};

const normalizeLegend = (option: GenericRecord, viewport: ReturnType<typeof resolveViewport>, tokens: ReturnType<typeof currentThemeTokens>) => {
  if (option.legend === false) return;
  const apply = (legendInput: any) => {
    const legend = isObject(legendInput) ? legendInput : {};
    if (legend.type === undefined) legend.type = viewport.legendType;
    if (legend.top === undefined && legend.bottom === undefined) legend.top = viewport.legendTop;
    if (legend.left === undefined && legend.right === undefined) legend.right = viewport.legendRight;
    if (!isObject(legend.textStyle)) legend.textStyle = {};
    if (legend.textStyle.color === undefined) legend.textStyle.color = tokens.textSecondary;
    if (legend.itemGap === undefined) legend.itemGap = viewport.width <= 1200 ? 8 : 12;
    if (legend.tooltip === undefined) legend.tooltip = { show: true };
    return legend;
  };
  if (Array.isArray(option.legend)) {
    option.legend = option.legend.map((legend) => apply(legend));
    return;
  }
  option.legend = apply(option.legend);
};

const buildTooltipPosition = (viewport: ReturnType<typeof resolveViewport>) => {
  return (
    point: number[],
    _params: any,
    _dom: HTMLDivElement,
    _rect: any,
    size: { contentSize: number[]; viewSize: number[] }
  ) => {
    const [mouseX, mouseY] = point;
    const [contentWidth, contentHeight] = size.contentSize;
    const [viewWidth, viewHeight] = size.viewSize;
    const padding = viewport.width <= 1200 ? 10 : 14;

    if (viewport.fixedTooltip) {
      const top = 12;
      const left = Math.max(8, Math.floor((viewWidth - Math.min(contentWidth, viewport.tooltipMaxWidth)) / 2));
      return [left, top];
    }

    let x = mouseX + 16;
    let y = mouseY + 16;

    if (x + contentWidth + padding > viewWidth) {
      x = mouseX - contentWidth - 16;
    }
    if (x < padding) {
      x = padding;
    }
    if (y + contentHeight + padding > viewHeight) {
      y = mouseY - contentHeight - 16;
    }
    if (y < padding) {
      y = padding;
    }
    return [x, y];
  };
};

const normalizeTooltip = (option: GenericRecord, viewport: ReturnType<typeof resolveViewport>, tokens: ReturnType<typeof currentThemeTokens>) => {
  if (option.tooltip === false) return;
  const tooltip = isObject(option.tooltip) ? option.tooltip : {};
  if (tooltip.triggerOn === undefined) tooltip.triggerOn = 'mousemove|click';
  tooltip.confine = true;
  tooltip.appendToBody = true;
  if (tooltip.enterable === undefined) tooltip.enterable = false;
  if (tooltip.transitionDuration === undefined) tooltip.transitionDuration = 0.12;
  if (tooltip.className === undefined) tooltip.className = 'modern-dhcp-chart-tooltip';
  if (!isObject(tooltip.textStyle)) tooltip.textStyle = {};
  if (tooltip.textStyle.color === undefined) tooltip.textStyle.color = tokens.text;
  if (tooltip.textStyle.fontSize === undefined) tooltip.textStyle.fontSize = 12;
  tooltip.backgroundColor = tokens.bg;
  tooltip.borderColor = tokens.border;
  tooltip.borderWidth = 1;
  tooltip.padding = tooltip.padding ?? [8, 10];
  tooltip.extraCssText = `max-width:${viewport.tooltipMaxWidth}px; white-space:normal; word-break:break-word; border-radius:8px; box-shadow:${tokens.shadow};`;
  tooltip.position = buildTooltipPosition(viewport);

  if (tooltip.axisPointer === undefined && tooltip.trigger !== 'item') {
    tooltip.axisPointer = { type: 'line', snap: true };
  }
  option.tooltip = tooltip;
};

const normalizeOption = (raw: any) => {
  const option = isObject(raw) ? cloneValue(raw) : {};
  const viewport = resolveViewport();
  const tokens = currentThemeTokens();

  normalizeGrid(option, viewport);
  normalizeXAxis(option, viewport, tokens);
  normalizeLegend(option, viewport, tokens);
  normalizeTooltip(option, viewport, tokens);

  return option;
};

const applyOption = () => {
  if (!chart.value) return;
  chart.value.setOption(normalizeOption(props.option) as any, true);
};

const resizeObserver = new ResizeObserver(() => {
  if (!chart.value) return;
  if (resizeFrame) {
    cancelAnimationFrame(resizeFrame);
  }
  resizeFrame = requestAnimationFrame(() => {
    chart.value?.resize();
    applyOption();
  });
});

onMounted(() => {
  if (!chartRef.value) return;
  const mountChart = () => {
    if (!chartRef.value || chart.value) return;
    loadEcharts().then((core) => {
      if (!chartRef.value || chart.value) return;
      chart.value = core.init(chartRef.value, props.theme) as any;
      applyOption();
      if (props.autoresize !== false) resizeObserver.observe(chartRef.value);
    });
  };

  if (props.lazy === false) {
    mountChart();
    return;
  }

  visibilityObserver = new IntersectionObserver(
    (entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        mountChart();
        visibilityObserver?.disconnect();
        visibilityObserver = null;
      }
    },
    { rootMargin: '120px' }
  );
  visibilityObserver.observe(chartRef.value);
});

watch(
  () => props.option,
  () => {
    applyOption();
  },
  { deep: true }
);

onBeforeUnmount(() => {
  if (resizeFrame) {
    cancelAnimationFrame(resizeFrame);
    resizeFrame = 0;
  }
  visibilityObserver?.disconnect();
  visibilityObserver = null;
  if (chartRef.value && props.autoresize !== false) resizeObserver.unobserve(chartRef.value);
  chart.value?.dispose();
});
</script>

<style scoped>
.echart {
  width: 100%;
  height: 100%;
}

:global(.modern-dhcp-chart-tooltip) {
  transition: opacity 0.12s ease, transform 0.12s ease;
  border-radius: 8px;
  backdrop-filter: blur(2px);
}

:global(.modern-dhcp-chart-tooltip:hover) {
  transform: translateY(-1px);
}
</style>
