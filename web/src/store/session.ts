import { defineStore } from 'pinia';
import { forceLogout, listSessions } from '@/api/sessions';
import type { PageQuery, SessionInfo } from '@/types/system';

interface SessionState {
  sessions: SessionInfo[];
  total: number;
  loading: boolean;
}

export const useSessionStore = defineStore('session', {
  state: (): SessionState => ({ sessions: [], total: 0, loading: false }),
  actions: {
    async fetch(params: PageQuery = { page: 1, pageSize: 20 }) {
      this.loading = true;
      try {
        const { data } = await listSessions(params);
        this.sessions = data.data.items;
        this.total = data.data.total;
      } finally {
        this.loading = false;
      }
    },
    async kick(id: string) {
      await forceLogout(id);
      this.sessions = this.sessions.filter((s) => s.id !== id);
    }
  }
});
