export interface TaskQueue<T> {
  enqueue: (task: () => Promise<T>) => Promise<T>;
  isRunning: () => boolean;
  clear: () => void;
}

export interface SingleFlight<T> {
  run: (task: () => Promise<T>) => Promise<T>;
  reset: () => void;
  isRunning: () => boolean;
}

export const createTaskQueue = <T>(): TaskQueue<T> => {
  const tasks: Array<() => Promise<unknown>> = [];
  let running = false;

  const runNext = async (): Promise<void> => {
    if (running) return;
    const next = tasks.shift();
    if (!next) return;
    running = true;
    try {
      await next();
    } finally {
      running = false;
      if (tasks.length) runNext();
    }
  };

  const enqueue = (task: () => Promise<T>) =>
    new Promise<T>((resolve, reject) => {
      tasks.push(async () => {
        try {
          const result = await task();
          resolve(result);
          return result as T;
        } catch (err) {
          reject(err);
        }
      });
      runNext();
    });

  return { enqueue, isRunning: () => running, clear: () => tasks.splice(0, tasks.length) };
};

export const createSingleFlight = <T>(): SingleFlight<T> => {
  let inflight: Promise<T> | null = null;

  const run = (task: () => Promise<T>) => {
    if (!inflight) {
      inflight = task().finally(() => {
        inflight = null;
      });
    }
    return inflight;
  };

  const reset = () => {
    inflight = null;
  };

  return { run, reset, isRunning: () => !!inflight };
};
