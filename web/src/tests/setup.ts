import { config } from '@vue/test-utils';
import { createTestingPinia } from '@pinia/testing';
import ElementPlus from 'element-plus';
import { i18n } from '@/i18n';

config.global.plugins = [
  createTestingPinia({
    stubActions: false,
    initialState: {
      permission: {
        permissions: new Set([
          'binding.view',
          'binding.manage',
          'lease.view',
          'lease.manage',
          'monitoring.view',
          'pool.ipv4.view',
          'pool.ipv4.manage',
          'pool.ipv6.view',
          'pool.analytics.view'
        ]),
        tree: [],
        loading: false
      },
      tenant: {
        currentTenantId: 'test-tenant',
        tenants: [],
        loading: false
      }
    }
  }),
  i18n,
  ElementPlus
];

config.global.stubs = {
  transition: false
};
