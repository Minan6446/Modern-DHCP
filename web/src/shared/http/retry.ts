export interface RetryOptions {
  retries?: number;
  baseDelay?: number;
  factor?: number;
  retryOn?: (error: unknown) => boolean;
  onRetry?: (attempt: number, error: unknown) => void;
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

export const retryAsync = async <T>(
  fn: () => Promise<T>,
  options: RetryOptions = {}
): Promise<T> => {
  const retries = options.retries ?? 2;
  const baseDelay = options.baseDelay ?? 250;
  const factor = options.factor ?? 2;

  let attempt = 0;
  let lastError: unknown;

  while (attempt <= retries) {
    try {
      return await fn();
    } catch (error) {
      lastError = error;
      if (attempt >= retries) break;
      if (options.retryOn && !options.retryOn(error)) break;
      options.onRetry?.(attempt + 1, error);
      const delay = baseDelay * Math.pow(factor, attempt);
      await sleep(delay);
      attempt += 1;
    }
  }

  throw lastError;
};
