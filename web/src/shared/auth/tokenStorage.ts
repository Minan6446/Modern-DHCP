export interface StorageLike {
  getItem: (key: string) => string | null;
  setItem: (key: string, value: string) => void;
  removeItem: (key: string) => void;
}

export interface TokenSnapshot {
  accessToken: string;
  refreshToken: string;
  accessExpiresAt: number;
  remember: boolean;
  lastUsername: string;
}

export interface TokenStorageOptions {
  accessKey?: string;
  accessExpiresKey?: string;
  refreshKey?: string;
  userKey?: string;
  rememberKey?: string;
  usernameKey?: string;
  session?: StorageLike;
  persistent?: StorageLike;
}

export interface TokenPayload {
  accessToken: string;
  refreshToken: string;
  expiresIn?: number;
  remember?: boolean;
}

const createMemoryStorage = (): StorageLike => {
  const store = new Map<string, string>();
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => store.set(key, value),
    removeItem: (key: string) => store.delete(key)
  };
};

const resolveStorage = (preferred?: StorageLike, fallback?: StorageLike) => {
  if (preferred) return preferred;
  if (typeof window === 'undefined') return fallback ?? createMemoryStorage();
  return fallback ?? window.sessionStorage;
};

export const createTokenStorage = (
  options: TokenStorageOptions = {}
): ReturnType<typeof buildTokenStorage> => buildTokenStorage(options);

const buildTokenStorage = (options: TokenStorageOptions) => {
  const accessKey = options.accessKey ?? 'mdhcp_access_token';
  const accessExpiresKey = options.accessExpiresKey ?? 'mdhcp_access_expires';
  const refreshKey = options.refreshKey ?? 'mdhcp_refresh_token';
  const userKey = options.userKey ?? 'mdhcp_user_info';
  const rememberKey = options.rememberKey ?? 'mdhcp_remember_me';
  const usernameKey = options.usernameKey ?? 'mdhcp_last_username';

  const session = resolveStorage(
    options.session,
    typeof window !== 'undefined' ? window.sessionStorage : undefined
  );
  const persistent = resolveStorage(
    options.persistent,
    typeof window !== 'undefined' ? window.localStorage : undefined
  );

  const saveTokens = (payload: TokenPayload) => {
    const remember = !!payload.remember;
    const accessStore = session;
    // Store refresh token only in session storage to reduce XSS persistence risk.
    const refreshStore = session;

    if (payload.accessToken) {
      accessStore.setItem(accessKey, payload.accessToken);
      accessStore.setItem(
        accessExpiresKey,
        String(payload.expiresIn ? Date.now() + payload.expiresIn * 1000 : 0)
      );
    } else {
      accessStore.removeItem(accessKey);
      accessStore.removeItem(accessExpiresKey);
    }

    if (payload.refreshToken) {
      refreshStore.setItem(refreshKey, payload.refreshToken);
    } else {
      session.removeItem(refreshKey);
      persistent.removeItem(refreshKey);
    }

    saveRemember(remember);
  };

  const loadTokens = (): TokenSnapshot => {
    const accessToken = session.getItem(accessKey) || '';
    const refreshToken = session.getItem(refreshKey) || '';
    const accessExpiresAt = Number(session.getItem(accessExpiresKey) || 0);
    const remember = loadRemember();
    const lastUsername = loadLastUsername();
    return { accessToken, refreshToken, accessExpiresAt, remember, lastUsername };
  };

  const clearTokens = () => {
    session.removeItem(accessKey);
    session.removeItem(accessExpiresKey);
    session.removeItem(refreshKey);
    persistent.removeItem(refreshKey);
  };

  const saveUser = (user: unknown | null, remember?: boolean) => {
    session.removeItem(userKey);
    persistent.removeItem(userKey);
    if (user) {
      const payload = JSON.stringify(user);
      const store = remember ? persistent : session;
      store.setItem(userKey, payload);
    }
  };

  const loadUser = <T>(): T | null => {
    const raw = persistent.getItem(userKey) || session.getItem(userKey);
    if (!raw) return null;
    try {
      return JSON.parse(raw) as T;
    } catch (e) {
      console.warn('Failed to parse cached user', e);
      return null;
    }
  };

  const saveRemember = (remember: boolean) => {
    session.removeItem(rememberKey);
    persistent.removeItem(rememberKey);
    if (remember) {
      persistent.setItem(rememberKey, '1');
    } else {
      session.setItem(rememberKey, '0');
    }
  };

  const loadRemember = () =>
    persistent.getItem(rememberKey) === '1' || session.getItem(rememberKey) === '1';

  const saveLastUsername = (username: string) => {
    if (!username) return;
    persistent.setItem(usernameKey, username);
  };

  const loadLastUsername = () => persistent.getItem(usernameKey) || '';

  const clearAll = () => {
    clearTokens();
    saveUser(null, false);
    saveRemember(false);
  };

  return {
    saveTokens,
    loadTokens,
    clearTokens,
    saveUser,
    loadUser,
    saveRemember,
    loadRemember,
    saveLastUsername,
    loadLastUsername,
    clearAll
  };
};

export const tokenStorage = createTokenStorage();
