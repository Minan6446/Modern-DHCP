import { defineStore } from 'pinia';
import {
  getRealtimeSnapshot,
  listAlertEvents,
  listAlertRules,
  listReportTasks
} from '@/api/monitoring';
import type { AlertEvent, AlertRule, PerfSnapshot, ReportTask } from '@/types/monitoring';
import { normalizeSnapshot } from '@/utils/monitoring';

interface MonitorState {
  snapshot: PerfSnapshot;
  alerts: AlertEvent[];
  rules: AlertRule[];
  reportTasks: ReportTask[];
  config: { autoRefresh: boolean; intervalMs: number };
  loading: boolean;
}

const STORAGE_KEY = 'mdhcp-monitor';

const loadPersisted = (): Partial<MonitorState> => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch (e) {
    return {};
  }
};

const persist = (state: MonitorState) => {
  localStorage.setItem(STORAGE_KEY, JSON.stringify({ config: state.config }));
};

export const useMonitorStore = defineStore('monitor', {
  state: (): MonitorState => ({
    snapshot: normalizeSnapshot({}),
    alerts: [],
    rules: [],
    reportTasks: [],
    config: { autoRefresh: true, intervalMs: 5000 },
    loading: false,
    ...loadPersisted()
  }),
  actions: {
    toList<T>(payload: any): T[] {
      return payload && 'items' in payload ? (payload.items as T[]) : (payload as T[]);
    },
    async refreshSnapshot() {
      const { data } = await getRealtimeSnapshot();
      this.snapshot = normalizeSnapshot(data.data);
    },
    async loadAlerts() {
      const { data } = await listAlertEvents();
      this.alerts = this.toList<AlertEvent>(data.data);
    },
    async loadRules() {
      const { data } = await listAlertRules();
      this.rules = this.toList<AlertRule>(data.data);
    },
    async loadReports() {
      const { data } = await listReportTasks();
      this.reportTasks = this.toList<ReportTask>(data.data);
    },
    setConfig(config: MonitorState['config']) {
      this.config = config;
      persist(this);
    },
    reset() {
      this.$reset();
      persist(this);
    }
  }
});
