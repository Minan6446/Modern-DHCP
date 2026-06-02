import { defineStore } from 'pinia';
import { getGlobalConfig, getNotificationConfig } from '@/api/system/config';
import type { GlobalConfig, NotificationConfig } from '@/types/system';

interface SystemState {
  globalConfig: GlobalConfig | null;
  notificationConfig: NotificationConfig | null;
  locale: string;
  params: Record<string, string>;
  loading: boolean;
}

const STORAGE_KEY = 'mdhcp-system';

const loadPersisted = (): Partial<SystemState> => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch (e) {
    return {};
  }
};

const persist = (state: SystemState) => {
  localStorage.setItem(
    STORAGE_KEY,
    JSON.stringify({ locale: state.locale, params: state.params })
  );
};

export const useSystemStore = defineStore('system', {
  state: (): SystemState => ({
    globalConfig: null,
    notificationConfig: null,
    locale: 'zh-CN',
    params: {},
    loading: false,
    ...loadPersisted()
  }),
  actions: {
    async loadConfigs() {
      this.loading = true;
      try {
        const [g, n] = await Promise.all([getGlobalConfig(), getNotificationConfig()]);
        this.globalConfig = g.data.data;
        this.notificationConfig = n.data.data;
        persist(this);
      } finally {
        this.loading = false;
      }
    },
    setLocale(locale: string) {
      this.locale = locale;
      persist(this);
    },
    setParam(key: string, value: string) {
      this.params[key] = value;
      persist(this);
    },
    reset() {
      this.$reset();
      persist(this);
    }
  }
});
