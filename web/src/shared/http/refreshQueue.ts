type RefreshExecutor = () => Promise<string | null | undefined>;

type Subscriber = (token: string | null) => void;

export interface RefreshQueue {
  run: (executor: RefreshExecutor) => Promise<string | null>;
  notify: (token: string | null) => void;
  reset: () => void;
  isRefreshing: () => boolean;
}

export const createRefreshQueue = (): RefreshQueue => {
  let refreshingPromise: Promise<string | null> | null = null;
  let subscribers: Subscriber[] = [];

  const notify = (token: string | null) => {
    subscribers.forEach((cb) => cb(token));
    subscribers = [];
  };

  const reset = () => {
    refreshingPromise = null;
    subscribers = [];
  };

  const run = (executor: RefreshExecutor) => {
    if (!refreshingPromise) {
      refreshingPromise = executor()
        .then((token) => {
          notify(token ?? null);
          return token ?? null;
        })
        .catch((err) => {
          notify(null);
          throw err;
        })
        .finally(() => {
          refreshingPromise = null;
        });
    }

    return new Promise<string | null>((resolve, reject) => {
      const subscriber: Subscriber = (token) => {
        if (token) resolve(token);
        else reject(new Error('refresh_failed'));
      };
      subscribers.push(subscriber);
      refreshingPromise?.catch(reject);
    });
  };

  const isRefreshing = () => !!refreshingPromise;

  return { run, notify, reset, isRefreshing };
};
