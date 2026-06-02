import { getCurrentScope, onScopeDispose, ref, shallowRef } from 'vue';
import { useDebounceFn, useThrottleFn } from '@vueuse/core';

export type UseDebouncedFetchMode = 'debounce' | 'throttle';

export interface UseDebouncedFetchOptions<T> {
  delay?: number;
  mode?: UseDebouncedFetchMode;
  immediate?: boolean;
  onSuccess?: (data: T) => void;
  onError?: (error: unknown) => void;
}

interface ExecuteContext<TArgs extends unknown[]> {
  args: TArgs;
}

export const useDebouncedFetch = <TArgs extends unknown[] = [], TResult = unknown>(
  fetcher: (args: TArgs, signal?: AbortSignal) => Promise<TResult>,
  options: UseDebouncedFetchOptions<TResult> = {}
) => {
  const mode = options.mode ?? 'debounce';
  const delay = options.delay ?? 250;
  const loading = ref(false);
  const error = shallowRef<unknown>(null);
  const data = shallowRef<TResult>();
  let controller: AbortController | null = null;
  let disposed = false;

  const abortActive = () => {
    if (controller) {
      controller.abort();
      controller = null;
    }
  };

  const execute = async ({ args }: ExecuteContext<TArgs>) => {
    if (disposed) return;
    abortActive();
    controller = new AbortController();
    loading.value = true;
    error.value = null;
    try {
      const result = await fetcher(args, controller.signal);
      if (disposed) return;
      data.value = result;
      options.onSuccess?.(result);
      return result;
    } catch (err) {
      if ((err as DOMException)?.name === 'AbortError') return;
      error.value = err;
      options.onError?.(err);
      throw err;
    } finally {
      loading.value = false;
    }
  };

  const runner =
    mode === 'throttle'
      ? useThrottleFn((args: TArgs) => execute({ args }), delay, false, true)
      : useDebounceFn((args: TArgs) => execute({ args }), delay, { maxWait: delay * 4 });

  const run = (...args: TArgs) => {
    if (disposed) return;
    loading.value = true;
    error.value = null;
    return runner(args);
  };
  const flush = () =>
    typeof (runner as any).flush === 'function' ? (runner as any).flush() : undefined;
  const cancel = () => {
    abortActive();
    if (typeof (runner as any).cancel === 'function') (runner as any).cancel();
  };

  const runNow = (...args: TArgs) => execute({ args });

  if (getCurrentScope()) {
    onScopeDispose(() => {
      disposed = true;
      cancel();
    });
  }

  if (options.immediate) run(...([] as unknown as TArgs));

  return { loading, error, data, run, runNow, flush, cancel };
};
