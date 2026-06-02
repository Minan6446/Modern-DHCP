import { reactive, ref, type UnwrapNestedRefs } from 'vue';

export interface UseFormOptions<T extends object> {
  initialValues: () => T;
  validate?: (values: UnwrapNestedRefs<T>) => boolean | Promise<boolean | void> | void;
  submit: (values: UnwrapNestedRefs<T>) => Promise<void> | void;
  onSuccess?: (values: UnwrapNestedRefs<T>) => void;
  onError?: (error: unknown) => void;
  resetOnSuccess?: boolean;
  formatError?: (error: unknown) => string;
}

export const useForm = <T extends object>(options: UseFormOptions<T>) => {
  const values = reactive(options.initialValues()) as UnwrapNestedRefs<T>;
  const loading = ref(false);
  const error = ref('');

  const reset = () => {
    Object.assign(values, options.initialValues());
    error.value = '';
  };

  const setError = (message: string) => {
    error.value = message;
  };

  const runValidation = async () => {
    if (!options.validate) return true;
    try {
      const result = await options.validate(values);
      if (typeof result === 'boolean') return result;
      return true;
    } catch (err) {
      return false;
    }
  };

  const submit = async () => {
    if (loading.value) return false;
    error.value = '';
    loading.value = true;
    const valid = await runValidation();
    if (!valid) {
      loading.value = false;
      return false;
    }
    try {
      await options.submit(values);
      options.onSuccess?.(values);
      if (options.resetOnSuccess) reset();
      return true;
    } catch (err: any) {
      const friendly = options.formatError?.(err) || '提交失败，请稍后重试';
      error.value = friendly;
      options.onError?.(err);
      throw err;
    } finally {
      loading.value = false;
    }
  };

  return { values, loading, error, submit, reset, setError };
};
