/**
 * Tree-shakeable echarts setup.
 * Only registers the chart types and components actually used across the project.
 * Vite's esbuild tree-shaking will eliminate unused chart code from the bundle.
 */
import { use } from 'echarts/core';
import { BarChart, LineChart, PieChart, RadarChart, GaugeChart, GraphChart } from 'echarts/charts';
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  TitleComponent,
  DatasetComponent,
  VisualMapComponent
} from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';

use([
  BarChart,
  LineChart,
  PieChart,
  RadarChart,
  GaugeChart,
  GraphChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  TitleComponent,
  DatasetComponent,
  VisualMapComponent,
  CanvasRenderer
]);

export type { EChartsOption } from 'echarts';

export { init, getInstanceByDom, connect, disconnect, dispose, registerMap, registerTheme } from 'echarts/core';
