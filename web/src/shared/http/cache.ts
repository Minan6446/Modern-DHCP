export interface CacheEntry<T> {
  value: T;
  expiresAt: number;
}

export interface RequestCache<T> {
  get: (key: string) => T | null;
  set: (key: string, value: T, ttl?: number) => void;
  delete: (key: string) => void;
  clear: () => void;
}

export const createRequestCache = <T>(defaultTtl = 0): RequestCache<T> => {
  const store = new Map<string, CacheEntry<T>>();

  const isExpired = (entry: CacheEntry<T>) => entry.expiresAt > 0 && Date.now() > entry.expiresAt;

  const get = (key: string) => {
    const entry = store.get(key);
    if (!entry) return null;
    if (isExpired(entry)) {
      store.delete(key);
      return null;
    }
    return entry.value;
  };

  const set = (key: string, value: T, ttl?: number) => {
    const expiresAt =
      ttl && ttl > 0 ? Date.now() + ttl : defaultTtl > 0 ? Date.now() + defaultTtl : 0;
    store.set(key, { value, expiresAt });
  };

  const clear = () => store.clear();

  return { get, set, delete: (key: string) => store.delete(key), clear };
};
