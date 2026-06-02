export const loadState = <T>(key: string): Partial<T> => {
  try {
    const raw = localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as Partial<T>) : {};
  } catch (e) {
    return {} as Partial<T>;
  }
};

export const saveState = <T>(key: string, value: T | null | undefined) => {
  try {
    if (value) {
      localStorage.setItem(key, JSON.stringify(value));
    } else {
      localStorage.removeItem(key);
    }
  } catch (e) {
    /* ignore persist errors */
  }
};

export const removeState = (key: string) => {
  try {
    localStorage.removeItem(key);
  } catch (e) {
    /* ignore persist errors */
  }
};
