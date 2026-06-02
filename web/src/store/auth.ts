export { useAuthStore } from '@/modules/auth/store';

/* Legacy implementation kept for reference during migration
import { defineStore } from 'pinia';
import { login as apiLogin, logout as apiLogout, refreshToken as apiRefreshToken } from '@/api/system/auth';
import type { LoginRequest, LoginResponse, TokenPair, User } from '@/types/system';

interface AuthState {
  accessToken: string;
  refreshToken: string;
  accessExpiresAt: number;
  user: User | null;
  lastUsername: string;
  isLoading: boolean;
  error: string;
}

const ACCESS_TOKEN_KEY = 'mdhcp_access_token';
const ACCESS_EXPIRES_KEY = 'mdhcp_access_expires';
const REFRESH_TOKEN_KEY = 'mdhcp_refresh_token';
const USER_KEY = 'mdhcp_user_info';
const LAST_USERNAME_KEY = 'mdhcp_last_username';

const loadAccessToken = () => sessionStorage.getItem(ACCESS_TOKEN_KEY) || '';
const loadAccessExpires = () => Number(sessionStorage.getItem(ACCESS_EXPIRES_KEY) || 0);
const loadRefreshToken = () => localStorage.getItem(REFRESH_TOKEN_KEY) || sessionStorage.getItem(REFRESH_TOKEN_KEY) || '';
const loadUser = (): User | null => {
  const raw = localStorage.getItem(USER_KEY) || sessionStorage.getItem(USER_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as User;
  } catch (e) {
    console.warn('Failed to parse cached user', e);
    return null;
  }
};
const loadLastUsername = () => localStorage.getItem(LAST_USERNAME_KEY) || '';

const saveAccessToken = (token: string, expiresAt: number) => {
  if (token) {
    sessionStorage.setItem(ACCESS_TOKEN_KEY, token);
    sessionStorage.setItem(ACCESS_EXPIRES_KEY, String(expiresAt || 0));
  } else {
    sessionStorage.removeItem(ACCESS_TOKEN_KEY);
    sessionStorage.removeItem(ACCESS_EXPIRES_KEY);
  }
};

const saveRefreshToken = (token: string, remember: boolean) => {
  sessionStorage.removeItem(REFRESH_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
  if (token) {
    (remember ? localStorage : sessionStorage).setItem(REFRESH_TOKEN_KEY, token);
  }
};

const saveUser = (user: User | null, remember: boolean) => {
  sessionStorage.removeItem(USER_KEY);
  localStorage.removeItem(USER_KEY);
  if (user) {
    const payload = JSON.stringify(user);
    (remember ? localStorage : sessionStorage).setItem(USER_KEY, payload);
  }
};

const saveLastUsername = (username: string) => {
  if (username) {
    localStorage.setItem(LAST_USERNAME_KEY, username);
  }
};

const normalizeTokens = (data: LoginResponse | TokenPair): TokenPair => {
  if ('access_token' in data) {
    return {
      accessToken: data.access_token,
      refreshToken: data.refresh_token,
      expiresIn: data.expires_in || 0
    };
  }
  return data as TokenPair;
};

export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    accessToken: loadAccessToken(),
    refreshToken: loadRefreshToken(),
    accessExpiresAt: loadAccessExpires(),
    user: loadUser(),
    lastUsername: loadLastUsername(),
    isLoading: false,
    error: ''
  }),
  getters: {
    displayName: (state) => state.user?.displayName || state.user?.username || '',
    isAuthenticated: (state) => !!state.accessToken
  },
  actions: {
    async login(payload: LoginRequest) {
      this.isLoading = true;
      this.error = '';
      try {
        const { data } = await apiLogin(payload);
        const tokens = normalizeTokens(data);
        this.setTokens(tokens, !!payload.remember);
        this.user = data.user_info;
        this.lastUsername = payload.username;
        saveUser(this.user, !!payload.remember);
        saveLastUsername(payload.username);
      } catch (error: any) {
        this.handleError(error);
        throw error;
      } finally {
        this.isLoading = false;
      }
    },
    async checkAuth() {
      const now = Date.now();
      const access = loadAccessToken();
      const expires = loadAccessExpires();
      const refresh = loadRefreshToken();
      this.accessToken = access;
      this.refreshToken = refresh;
      this.accessExpiresAt = expires;
      this.lastUsername = loadLastUsername();
      this.user = loadUser();

      const willExpireSoon = expires > 0 && expires - now < 30 * 1000;
      if (willExpireSoon && this.refreshToken) {
        try {
          await this.refresh();
        } catch {
          await this.logout();
        }
      }
    },
    setTokens(tokens: TokenPair, remember: boolean) {
      this.accessToken = tokens.accessToken;
      this.refreshToken = tokens.refreshToken;
      this.accessExpiresAt = tokens.expiresIn ? Date.now() + tokens.expiresIn * 1000 : 0;
      saveAccessToken(tokens.accessToken, this.accessExpiresAt);
      saveRefreshToken(tokens.refreshToken, remember);
    },
    async refresh() {
      if (!this.refreshToken) return;
      try {
        const { data } = await apiRefreshToken(this.refreshToken);
        const tokens = normalizeTokens(data);
        this.setTokens(tokens, !!localStorage.getItem(REFRESH_TOKEN_KEY));
      } catch (error: any) {
        this.handleError(error);
        throw error;
      }
    },
    async logout() {
      try {
        await apiLogout();
      } finally {
        this.clear();
      }
    },
    clear() {
      this.accessToken = '';
      this.refreshToken = '';
      this.accessExpiresAt = 0;
      this.user = null;
      this.lastUsername = loadLastUsername();
      this.error = '';
      saveAccessToken('', 0);
      saveRefreshToken('', false);
      saveUser(null, false);
    },
    handleError(error: any) {
      if (error?.code === 'ERR_NETWORK') {
        this.error = '网络异常，请检查连接';
        return;
      }
      const status = error?.response?.status;
      const message = error?.response?.data?.message || error?.message;
      if (status === 401) {
        this.error = '用户名或密码错误';
        return;
      }
      if (status === 423) {
        this.error = '账户已被锁定，请联系管理员';
        return;
      }
      if (status === 498 || message?.includes('expired')) {
        this.error = '登录已过期，请重新登录';
        return;
      }
      this.error = message || '登录失败，请稍后重试';
    }
  }
});
*/
