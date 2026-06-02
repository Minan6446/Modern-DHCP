import { defineStore } from 'pinia';
import {
  login as apiLogin,
  logout as apiLogout,
  refreshToken as apiRefreshToken,
  getCapabilities,
  extractGrantedCapabilities
} from '@/modules/auth/api';
import type {
  LoginProfile,
  LoginRequest,
  LoginResponse,
  TokenPair,
  User
} from '@/modules/auth/types';
import { AuthErrorCode } from '@/modules/auth/types';
import { mapAuthError } from '@/modules/auth/hooks/errors';
import { tokenStorage } from '@/shared/auth/tokenStorage';
import { ensureCsrfCookie } from '@/shared/security/csrf';
import { usePermissionStore } from '@/store/permission';
import { useTenantStore } from '@/store/tenant';
import { isSuperAdminUser } from '@/shared/permission';

interface AuthState {
  accessToken: string;
  refreshToken: string;
  accessExpiresAt: number;
  user: User | null;
  lastUsername: string;
  remember: boolean;
  postLoginRedirect: string;
  isLoading: boolean;
  error: string;
}

const postLoginRedirectStorageKey = 'mdhcp:post-login-redirect';

const loadPostLoginRedirect = () => {
  try {
    return sessionStorage.getItem(postLoginRedirectStorageKey) || '';
  } catch {
    return '';
  }
};

const savePostLoginRedirect = (target: string) => {
  try {
    if (!target) {
      sessionStorage.removeItem(postLoginRedirectStorageKey);
      return;
    }
    sessionStorage.setItem(postLoginRedirectStorageKey, target);
  } catch {
    // ignore storage errors
  }
};

const parseExpiresIn = (expiresAt?: string | number | null) => {
  if (!expiresAt) return 0;
  const ts = typeof expiresAt === 'number' ? expiresAt : Date.parse(expiresAt);
  if (Number.isNaN(ts)) return 0;
  return Math.max(0, Math.round((ts - Date.now()) / 1000));
};

const normalizeTokens = (data: LoginResponse | TokenPair): TokenPair => {
  if ('token' in data || 'refreshToken' in data || 'expiresAt' in data) {
    return {
      accessToken: (data as LoginResponse).token || '',
      refreshToken: (data as LoginResponse).refreshToken || '',
      expiresIn: parseExpiresIn((data as LoginResponse).expiresAt)
    };
  }
  if ('access_token' in data) {
    return {
      accessToken: (data as LoginResponse).access_token || '',
      refreshToken: (data as LoginResponse).refresh_token || '',
      expiresIn: (data as LoginResponse).expires_in || 0
    };
  }
  return data as TokenPair;
};

const normalizeProfile = (profile: LoginProfile | undefined, fallbackUsername: string): User => {
  const username = fallbackUsername || profile?.displayName || 'user';
  return {
    id: profile?.id || profile?.tenantId || username,
    username,
    displayName: profile?.displayName || username,
    email: '',
    phone: '',
    status: 'active',
    roles: profile?.role ? [{ id: profile.role, name: profile.role }] : [],
    tenants: profile?.tenantId ? [{ id: profile.tenantId, name: profile.tenantId }] : [],
    lastLoginAt: undefined
  };
};

export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    ...(() => {
      const snapshot = tokenStorage.loadTokens();
      return {
        accessToken: snapshot.accessToken,
        refreshToken: snapshot.refreshToken,
        accessExpiresAt: snapshot.accessExpiresAt,
        lastUsername: snapshot.lastUsername,
        remember: snapshot.remember,
        postLoginRedirect: loadPostLoginRedirect()
      };
    })(),
    user: tokenStorage.loadUser<User>(),
    isLoading: false,
    error: ''
  }),
  getters: {
    displayName: (state) => state.user?.displayName || state.user?.username || '',
    isAuthenticated: (state) => !!state.accessToken
  },
  actions: {
    setPostLoginRedirect(target: string) {
      this.postLoginRedirect = typeof target === 'string' ? target : '';
      savePostLoginRedirect(this.postLoginRedirect);
    },
    consumePostLoginRedirect() {
      const target = this.postLoginRedirect || loadPostLoginRedirect();
      this.postLoginRedirect = '';
      savePostLoginRedirect('');
      return target;
    },
    async login(payload: LoginRequest) {
      this.isLoading = true;
      this.error = '';
      try {
        await ensureCsrfCookie();
        const { data } = await apiLogin(payload);
        const tokens = normalizeTokens(data);
        const rememberFlag = payload.remember ?? false;
        this.setTokens(tokens, rememberFlag);
        this.user = normalizeProfile(data.profile, payload.username);
        // Ensure tenant is set so backend headers carry a tenant id
        const tenantStore = useTenantStore();
        if (!tenantStore.currentTenantId) {
          tenantStore.switchTenant('global');
        }
        this.lastUsername = payload.username;
        this.remember = rememberFlag;
        tokenStorage.saveUser(this.user, rememberFlag);
        tokenStorage.saveLastUsername(payload.username);
        // Load capabilities after login to drive route guards
        if (isSuperAdminUser(this.user)) {
          usePermissionStore().setPermissions(['*']);
        } else {
          try {
            const caps = await getCapabilities();
            const granted = extractGrantedCapabilities(caps);
            usePermissionStore().setPermissions(granted);
          } catch (err) {
            // Fail closed for non-superadmin when capability discovery fails
            this.clear();
            throw err;
          }
        }
        return data;
      } catch (error: any) {
        this.handleError(error);
        throw error;
      } finally {
        this.isLoading = false;
      }
    },
    async checkAuth() {
      const now = Date.now();
      const snapshot = tokenStorage.loadTokens();
      this.accessToken = snapshot.accessToken;
      this.refreshToken = snapshot.refreshToken;
      this.accessExpiresAt = snapshot.accessExpiresAt;
      this.lastUsername = snapshot.lastUsername;
      this.remember = snapshot.remember;
      this.user = tokenStorage.loadUser<User>();

      const willExpireSoon =
        snapshot.accessExpiresAt > 0 && snapshot.accessExpiresAt - now < 30 * 1000;
      if (willExpireSoon && this.refreshToken) {
        try {
          await this.refresh();
        } catch {
          await this.logout();
          return;
        }
      }

      if (this.accessToken) {
        try {
          if (isSuperAdminUser(this.user)) {
            usePermissionStore().setPermissions(['*']);
          } else {
            const caps = await getCapabilities();
            const granted = extractGrantedCapabilities(caps);
            usePermissionStore().setPermissions(granted);
          }
          const tenantStore = useTenantStore();
          if (!tenantStore.currentTenantId) {
            tenantStore.switchTenant('default');
          }
        } catch (err: any) {
          this.handleError(err);
          // Keep session for super admin even if capabilities endpoint fails
          if (isSuperAdminUser(this.user)) {
            usePermissionStore().setPermissions(['*']);
          } else {
            await this.logout();
          }
        }
      }
    },
    setTokens(tokens: TokenPair, remember: boolean) {
      this.accessToken = tokens.accessToken;
      this.refreshToken = tokens.refreshToken;
      this.accessExpiresAt = tokens.expiresIn ? Date.now() + tokens.expiresIn * 1000 : 0;
      tokenStorage.saveTokens({
        accessToken: tokens.accessToken,
        refreshToken: tokens.refreshToken,
        expiresIn: tokens.expiresIn,
        remember
      });
    },
    async refresh() {
      if (!this.refreshToken) return;
      try {
        const { data } = await apiRefreshToken(this.refreshToken);
        const tokens = normalizeTokens(data);
        this.setTokens(tokens, tokenStorage.loadRemember());
        // Reload capabilities to honor role changes without re-login
        if (isSuperAdminUser(this.user)) {
          usePermissionStore().setPermissions(['*']);
        } else {
          const caps = await getCapabilities();
          const granted = extractGrantedCapabilities(caps);
          usePermissionStore().setPermissions(granted);
        }
      } catch (error: any) {
        this.handleError(error);
        if (!isSuperAdminUser(this.user)) {
          this.clear();
        }
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
      this.lastUsername = tokenStorage.loadLastUsername();
      this.remember = tokenStorage.loadRemember();
      this.postLoginRedirect = '';
      this.error = '';
      tokenStorage.clearAll();
      savePostLoginRedirect('');
    },
    handleError(error: any) {
      const status = error?.response?.status as number | undefined;
      const message = error?.response?.data?.message || error?.message;
      if (status === AuthErrorCode.TokenExpired) {
        this.clear();
      }
      this.error = mapAuthError(status, message);
    }
  }
});
