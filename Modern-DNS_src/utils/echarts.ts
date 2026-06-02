import { BarChart, HeatmapChart, LineChart, PieChart } from 'echarts/charts'
import {
  GridComponent,
  LegendComponent,
  TooltipComponent,
  VisualMapComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { use } from 'echarts/core'
import * as echarts from 'echarts/core'

use([
  TooltipComponent,
  GridComponent,
  LegendComponent,
  VisualMapComponent,
  LineChart,
  BarChart,
  PieChart,
  HeatmapChart,
  CanvasRenderer,
])

export { echarts }