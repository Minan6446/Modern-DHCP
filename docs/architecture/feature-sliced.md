# Frontend Feature-Sliced Migration (Auth First)

## Goal
Adopt feature-sliced structure to improve cohesion (by domain) and reduce cross-cutting coupling. This document tracks the migration steps and rollback plan.

## Target Layout
```
src/
  modules/
    auth/
      api/
      components/
      hooks/
      store/
      types/
      views/
    ...other domains
  shared/
    ui/
    lib/
    api-client/
    composables/
    types/
  app/
    router/
    store/
    styles/
    i18n/
  main.ts
```

## Phase Plan
1. Create new structure and keep legacy paths as thin wrappers.
2. Migrate one domain (auth) end-to-end; verify build/run.
3. Gradually migrate other domains; keep wrappers only while consumers update.
4. Remove wrappers/legacy paths once all imports point to modules/.*

## Completed (Auth)
- New locations:
  - modules/auth/views/Login.vue
  - modules/auth/store/index.ts
  - modules/auth/api/index.ts
  - modules/auth/types/index.ts
- Legacy wrappers kept:
  - views/system/Login.vue → renders new Login
  - store/auth.ts → re-exports useAuthStore
  - api/system/auth.ts → re-exports auth APIs
- Router/main/layout/http client imports updated to new store/view.
- types/system.ts re-exports auth types to maintain compatibility.

## Next Domains (suggested order)
1) Monitoring (views/components/api/store/types). 
2) System management (users/roles/tenants/config). 
3) Pools/leases/binding/security.

## Rollback Plan
- Switch imports back to legacy paths (views/system/Login.vue, store/auth.ts, api/system/auth.ts).
- Remove or ignore new modules/auth files if necessary (they are additive and isolated).
- Keep types/system.ts exporting auth types; no breaking changes expected.

## Notes
- Continue to use '@/modules/<domain>/...' for new work.
- Add shared composables/ui under shared/ to avoid duplication.
- When a domain is fully migrated, delete the corresponding legacy wrappers to reduce maintenance cost.
