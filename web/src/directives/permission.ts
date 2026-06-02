import type { Directive } from 'vue';
import { usePermissionStore } from '@/store/permission';

export const permission: Directive<HTMLElement, string | string[]> = {
  mounted(el, binding) {
    const store = usePermissionStore();
    const value = binding.value;
    const allowed = Array.isArray(value)
      ? value.some((k) => store.can(k))
      : typeof value === 'string'
        ? store.can(value)
        : true;
    if (!allowed) {
      el.parentNode?.removeChild(el);
    }
  }
};
