import { onBeforeUnmount, onScopeDispose } from 'vue';

export interface LeakDetector {
  track: (cleanup: () => void) => () => void;
  disposeAll: () => void;
  pending: () => number;
  warn: () => void;
}

export const useLeakDetector = (label: string): LeakDetector => {
  const disposers: Array<() => void> = [];
  let disposed = false;

  const disposeAll = () => {
    disposed = true;
    while (disposers.length) {
      const disposer = disposers.pop();
      try {
        disposer?.();
      } catch (error) {
        console.warn(`[LeakDetector:${label}] cleanup failed`, error);
      }
    }
  };

  const track = (cleanup: () => void) => {
    if (disposed) {
      cleanup();
      return () => undefined;
    }
    disposers.push(cleanup);
    return () => {
      const idx = disposers.indexOf(cleanup);
      if (idx >= 0) disposers.splice(idx, 1);
      cleanup();
    };
  };

  const warn = () => {
    if (!disposed && disposers.length) {
      console.warn(`[LeakDetector:${label}] auto-cleaning ${disposers.length} pending disposers`);
    }
  };

  onBeforeUnmount(() => {
    warn();
    disposeAll();
  });

  onScopeDispose(disposeAll);

  return { track, disposeAll, pending: () => disposers.length, warn };
};

export const useSafeEventListener = (
  target: Pick<EventTarget, 'addEventListener' | 'removeEventListener'> | null | undefined,
  event: string,
  handler: EventListenerOrEventListenerObject,
  options?: boolean | AddEventListenerOptions,
  detector?: LeakDetector
) => {
  if (!target?.addEventListener) return () => undefined;
  target.addEventListener(event, handler, options);
  const cleanup = () => target.removeEventListener(event, handler, options);
  detector?.track(cleanup);
  onScopeDispose(cleanup);
  return cleanup;
};

export const useSafeInterval = (fn: () => void, interval: number, detector?: LeakDetector) => {
  const timer = window.setInterval(fn, interval);
  const cleanup = () => window.clearInterval(timer);
  detector?.track(cleanup);
  onScopeDispose(cleanup);
  return cleanup;
};

export const useSafeTimeout = (fn: () => void, delay: number, detector?: LeakDetector) => {
  const timer = window.setTimeout(fn, delay);
  const cleanup = () => window.clearTimeout(timer);
  detector?.track(cleanup);
  onScopeDispose(cleanup);
  return cleanup;
};
