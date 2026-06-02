import { defineStore } from 'pinia';
import { loadState, saveState, removeState } from './persist';

interface ListState {
  keyword: string;
  page: number;
  pageSize: number;
}

interface SessionState extends ListState {
  status: string;
  ip: string;
  tenant: string;
  startAt: string;
  endAt: string;
}

interface UserListState extends ListState {
  status: string;
  roleId: string;
}

interface LoginAuditState extends ListState {
  username: string;
  ip: string;
  result: string;
  operationType: string;
  idKeyword: string;
  timePreset: string;
  requestId: string;
  sessionId: string;
  startAt: string;
  endAt: string;
}

interface SystemViewState {
  tenant: ListState;
  role: ListState;
  user: UserListState;
  apikey: ListState;
  session: SessionState;
  loginAudit: LoginAuditState;
}

const STORAGE_KEY = 'mdhcp-system-view';

const loadPersisted = (): Partial<SystemViewState> => loadState<SystemViewState>(STORAGE_KEY);

const persist = (state: SystemViewState) => {
  saveState(STORAGE_KEY, state);
};

export const useSystemViewStore = defineStore('systemView', {
  state: (): SystemViewState => ({
    tenant: { keyword: '', page: 1, pageSize: 10 },
    role: { keyword: '', page: 1, pageSize: 10 },
    user: { keyword: '', status: '', roleId: '', page: 1, pageSize: 10 },
    apikey: { keyword: '', page: 1, pageSize: 10 },
    session: { keyword: '', status: '', ip: '', tenant: '', startAt: '', endAt: '', page: 1, pageSize: 10 },
    loginAudit: {
      keyword: '',
      username: '',
      ip: '',
      result: '',
      operationType: '',
      idKeyword: '',
      timePreset: '24h',
      requestId: '',
      sessionId: '',
      startAt: '',
      endAt: '',
      page: 1,
      pageSize: 10
    },
    ...loadPersisted()
  }),
  actions: {
    setTenant(state: Partial<ListState>) {
      this.tenant = { ...this.tenant, ...state };
      persist(this.$state);
    },
    setRole(state: Partial<ListState>) {
      this.role = { ...this.role, ...state };
      persist(this.$state);
    },
    setUser(state: Partial<UserListState>) {
      this.user = { ...this.user, ...state };
      persist(this.$state);
    },
    setApiKey(state: Partial<ListState>) {
      this.apikey = { ...this.apikey, ...state };
      persist(this.$state);
    },
    setSession(state: Partial<SessionState>) {
      this.session = { ...this.session, ...state };
      persist(this.$state);
    },
    setLoginAudit(state: Partial<LoginAuditState>) {
      this.loginAudit = { ...this.loginAudit, ...state };
      persist(this.$state);
    },
    reset() {
      this.$reset();
      removeState(STORAGE_KEY);
    }
  }
});
