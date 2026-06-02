import { createApp } from 'vue';
import App from './App.vue';
import router from './router';
import { createPinia } from 'pinia';
import { i18n } from './i18n';
import { bootstrapLocale } from './i18n/useLocale';
import ElementPlus from 'element-plus';
import 'element-plus/theme-chalk/base.css';
import './styles/global.css';
import './styles/reset.scss';
import { usePermissionStore } from '@/store/permission';
import { useAuthStore } from '@/modules/auth/store';
import { useWs } from '@/utils/ws';
import { showError } from '@/shared/errors/messageToast';

const app = createApp(App);
const pinia = createPinia();
app.use(pinia);

const permissionStore = usePermissionStore(pinia);
const authStore = useAuthStore(pinia);

const isIgnorableGlobalError = (message?: string) => {
  const text = (message || '').toLowerCase();
  return (
    text.includes('resizeobserver loop completed with undelivered notifications') ||
    text.includes('resizeobserver loop limit exceeded')
  );
};

const bootstrap = async () => {
  await authStore.checkAuth();
  if (authStore.isAuthenticated) {
    try {
      await permissionStore.loadPermissions();
    } catch (err) {
      console.warn('[Permissions] load failed, continue without permissions', err);
      permissionStore.setPermissions([] as any);
    }
  }

  // i18n: resolve persisted/browser locale and apply side-effects (html lang, dayjs).
  // ElementPlus locale is bound reactively via <el-config-provider> in App.vue.
  bootstrapLocale(i18n.global.locale as { value: 'zh' | 'en' });

  // Permission directive
  const can = (required?: string | string[]) => {
    if (!required) return true;
    const list = Array.isArray(required) ? required : [required];
    return permissionStore.hasAny(list);
  };

  app.directive('permission', {
    mounted(el, binding) {
      if (!can(binding.value)) {
        el.parentNode && el.parentNode.removeChild(el);
      }
    }
  });

  // Global WS instance (optional)
  const wsUrl = import.meta.env.VITE_WS_BASE;
  let wsClient: ReturnType<typeof useWs> | null = null;
  if (wsUrl) {
    wsClient = useWs({ url: wsUrl, heartbeatMs: 30000, reconnect: true, pauseOnHidden: true });
    window.addEventListener('beforeunload', () => wsClient?.close());
    app.config.globalProperties.$ws = wsClient;
  }

  // Global error handlers
  window.addEventListener('error', (e) => {
    const message = String(e?.message || e?.error?.message || '');
    if (isIgnorableGlobalError(message)) {
      return;
    }
    console.error('[GlobalError]', e.error || e.message);
    showError('Unexpected error occurred');
  });

  window.addEventListener('unhandledrejection', (e) => {
    const reasonMessage =
      typeof e.reason === 'string' ? e.reason : String((e.reason as { message?: string } | undefined)?.message || '');
    if (isIgnorableGlobalError(reasonMessage)) {
      return;
    }
    console.error('[UnhandledRejection]', e.reason);
    showError('Request failed, please retry');
  });

  app.use(router);
  app.use(i18n);
  app.use(ElementPlus);
  app.mount('#app');
};

bootstrap().catch((err) => {
  console.error('[BootstrapError]', err);
  showError('启动失败，请刷新重试');
});
