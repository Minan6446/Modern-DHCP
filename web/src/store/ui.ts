import { defineStore } from 'pinia';
import { loadState, saveState, removeState } from './persist';

interface TabItem {
  key: string;
  title: string;
  path: string;
}

interface UiState {
  sidebarCollapsed: boolean;
  menuOpenKeys: string[];
  tabs: TabItem[];
  activeTab: string;
  loading: boolean;
}

const STORAGE_KEY = 'mdhcp-ui';

const loadPersisted = (): Partial<UiState> => loadState<UiState>(STORAGE_KEY);

const persist = (state: UiState) => {
  if (state.tabs.length || state.sidebarCollapsed || state.menuOpenKeys.length || state.activeTab) {
    saveState(STORAGE_KEY, {
      sidebarCollapsed: state.sidebarCollapsed,
      menuOpenKeys: state.menuOpenKeys,
      tabs: state.tabs,
      activeTab: state.activeTab
    });
  } else {
    removeState(STORAGE_KEY);
  }
};

export const useUiStore = defineStore('ui', {
  state: (): UiState => ({
    sidebarCollapsed: false,
    menuOpenKeys: [],
    tabs: [],
    activeTab: '',
    loading: false,
    ...loadPersisted()
  }),
  actions: {
    toggleSidebar(val?: boolean) {
      this.sidebarCollapsed = val ?? !this.sidebarCollapsed;
      persist(this);
    },
    setMenuOpen(keys: string[]) {
      this.menuOpenKeys = keys;
      persist(this);
    },
    openTab(tab: TabItem) {
      if (!this.tabs.find((t) => t.key === tab.key)) this.tabs.push(tab);
      this.activeTab = tab.key;
      persist(this);
    },
    closeTab(key: string) {
      this.tabs = this.tabs.filter((t) => t.key !== key);
      if (this.activeTab === key && this.tabs.length)
        this.activeTab = this.tabs[this.tabs.length - 1].key;
      persist(this);
    },
    setLoading(val: boolean) {
      this.loading = val;
    },
    reset() {
      this.$reset();
      removeState(STORAGE_KEY);
    }
  }
});
