import { defineConfig, loadEnv, type PluginOption } from 'vite';
import vue from '@vitejs/plugin-vue';
import vueJsx from '@vitejs/plugin-vue-jsx';
import path from 'node:path';
import AutoImport from 'unplugin-auto-import/vite';
import Components from 'unplugin-vue-components/vite';
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers';
import ElementPlus from 'unplugin-element-plus/vite';
import { visualizer } from 'rollup-plugin-visualizer';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const isProd = mode === 'production';
  const enableSourceMap = env.VITE_SOURCEMAP === 'true';
  const analyze = env.ANALYZE === 'true';
  const apiProxyTarget = env.VITE_API_PROXY_TARGET || 'http://localhost:8080';

  return {
    base: env.VITE_BASE || '/',
    envPrefix: 'VITE_',
    plugins: [
      vue(),
      vueJsx(),
      AutoImport({
        imports: ['vue', 'vue-router', 'pinia'],
        dts: 'src/types/auto-imports.d.ts',
        resolvers: [ElementPlusResolver({ importStyle: 'css' })],
        eslintrc: {
          enabled: true,
          filepath: './.eslintrc-auto-import.json',
          globalsPropValue: true
        }
      }),
      Components({
        dirs: ['src/components'],
        dts: 'src/types/components.d.ts',
        extensions: ['vue'],
        deep: true,
        resolvers: [ElementPlusResolver({ importStyle: 'css' })]
      }),
      ElementPlus({
        useSource: false
      }),
      analyze
        ? (visualizer({
            filename: 'dist/stats.html',
            gzipSize: true,
            brotliSize: true,
            template: 'treemap'
          }) as PluginOption)
        : null
    ].filter(Boolean) as PluginOption[],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, 'src'),
        'dayjs/plugin/localeData.js': path.resolve(__dirname, 'node_modules/dayjs/esm/plugin/localeData/index.js'),
        'dayjs/plugin/customParseFormat.js': path.resolve(__dirname, 'node_modules/dayjs/esm/plugin/customParseFormat/index.js'),
        'dayjs/plugin/weekOfYear.js': path.resolve(__dirname, 'node_modules/dayjs/esm/plugin/weekOfYear/index.js'),
        'dayjs/plugin/weekYear.js': path.resolve(__dirname, 'node_modules/dayjs/esm/plugin/weekYear/index.js'),
        'dayjs/plugin/weekday.js': path.resolve(__dirname, 'node_modules/dayjs/esm/plugin/weekday/index.js'),
        'dayjs/plugin/advancedFormat.js': path.resolve(__dirname, 'node_modules/dayjs/esm/plugin/advancedFormat/index.js'),
        'dayjs/plugin/dayOfYear.js': path.resolve(__dirname, 'node_modules/dayjs/esm/plugin/dayOfYear/index.js'),
        'dayjs/plugin/isSameOrAfter.js': path.resolve(__dirname, 'node_modules/dayjs/esm/plugin/isSameOrAfter/index.js'),
        'dayjs/plugin/isSameOrBefore.js': path.resolve(__dirname, 'node_modules/dayjs/esm/plugin/isSameOrBefore/index.js'),
        'dayjs/plugin/quarterOfYear.js': path.resolve(__dirname, 'node_modules/dayjs/esm/plugin/quarterOfYear/index.js'),
        'dayjs/plugin/localizedFormat.js': path.resolve(__dirname, 'node_modules/dayjs/esm/plugin/localizedFormat/index.js'),
        'dayjs/plugin/relativeTime.js': path.resolve(__dirname, 'node_modules/dayjs/esm/plugin/relativeTime/index.js'),
        'dayjs/plugin/isLeapYear.js': path.resolve(__dirname, 'node_modules/dayjs/esm/plugin/isLeapYear/index.js')
      }
    },
    server: {
      host: '0.0.0.0',
      port: 5173,
      open: true,
      proxy: {
        '/api': {
          target: apiProxyTarget,
          changeOrigin: true,
          secure: false,
          // preserve /api prefix so backend routes stay consistent
          rewrite: (path) => path
        }
      }
    },
    optimizeDeps: {
      include: [
        'vue',
        'vue-router',
        'pinia',
        'axios',
        '@vueuse/core',
        'dayjs',
        'dayjs/plugin/localeData',
        'dayjs/plugin/weekOfYear',
        'dayjs/plugin/weekYear',
        'dayjs/plugin/weekday',
        'dayjs/plugin/customParseFormat',
        'dayjs/plugin/advancedFormat',
        'dayjs/plugin/dayOfYear',
        'dayjs/plugin/isSameOrAfter',
        'dayjs/plugin/isSameOrBefore',
        'dayjs/plugin/quarterOfYear',
        'dayjs/plugin/localizedFormat',
        'dayjs/plugin/relativeTime',
        'dayjs/plugin/isLeapYear'
      ],
      exclude: ['element-plus']
    },
    build: {
      sourcemap: enableSourceMap,
      reportCompressedSize: false, // gzip/br handled by deploy infra
      cssCodeSplit: true,
      chunkSizeWarningLimit: 1200,
      rollupOptions: {
        output: {
          manualChunks(id) {
            if (id.includes('node_modules')) {
              if (id.includes('element-plus')) return 'element-plus';
              if (id.includes('echarts')) return 'echarts';
              if (id.includes('@vueuse')) return 'vueuse';
              return 'vendor';
            }
          }
        }
      },
      minify: isProd ? 'esbuild' : false
    },
    esbuild: {
      drop: env.VITE_DROP_CONSOLE === 'true' ? ['console', 'debugger'] : []
    }
  };
});
