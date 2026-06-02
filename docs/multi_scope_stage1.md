# Multi-Scope Refactor – Stage 1

> 当前系统已切换为单租户模式，本文件为历史方案备忘录，仅用于溯源与参考。

## AccessScope contract
- **Fields**: `ScopeID` (logical domain), optional `TenantID`, repeated `GroupIDs`, free-form `Labels`, and `Principal`.
- **Defaults**: `ScopeID` defaults to `global`; `Principal` must be non-empty; `TenantID` is optional and only used for backward compatibility; missing `GroupIDs`/`Labels` imply global access.
- **Helpers**: `WithTenantID`, `WithGroupIDs`, `WithLabel(s)` normalize and dedupe values. `EffectiveTenant()` and `LegacyScope()` provide bridge logic for tenants still required downstream.
- **Usage plan**:
  1. HTTP middleware builds an `AccessScope` from token metadata (principal, default groups) and optional query overrides.
  2. Service interfaces accept `AccessScope` instead of `(scopeID, tenantID)` tuples.
  3. Repository layer reads `AccessScope` to parameterize queries (tenant fallback, group filtering, label predicates).
  4. Auditing and background jobs record `PrincipalContext` (user, session, access scope) for traceability.

## RBAC schema plan
- **Assignments table**: add `group_ids JSON NULL` and `labels JSON NULL` columns (default empty arrays/objects). Backfill existing data by copying `tenant_id` into `labels` as `{"tenant":"<id>"}` when desired.
- **Generated columns / indexes**:
  - New stored columns `group_scope` and `label_scope` (hash of sorted labels) for efficient filtering.
  - Extend `uniq_assignment_scope` to include the new scopes so duplicates remain prevented.
  - Add index `idx_assignment_group (JSON_EXTRACT(group_ids, '$[*]'))` via virtual column if MySQL version allows; otherwise materialize to a helper table.
- **Migration steps**:
  1. Add nullable JSON columns and helper generated columns.
  2. Backfill new columns for existing rows inside a controlled transaction/batch.
  3. Update repository code to read/write the JSON columns (keeping tenant fields).
  4. After code deploy, drop obsolete generated columns (`tenant_scope` etc.) once no callers rely on them.
- **Rollback**: remove new columns/indexes and revert repository code path; since backfill is additive, restoring from snapshot or running `UPDATE ... SET group_ids=NULL, labels=NULL` suffices.

## Tenant field audit
| Entity / Table | Location | Notes |
| --- | --- | --- |
| Tenant, TenantQuota | `pkg/models/models.go` | Remain authoritative; represent org metadata and quotas even if user access becomes global. |
| AddressPool | `pkg/models/models.go` | `TenantID` currently mandatory; future state: optional + derived from AccessScope for legacy pools. |
| Lease / PrefixLease | `pkg/models/models.go` | Persist tenant for reporting; service entrypoints will switch to AccessScope but DB column may stay for analytics. |
| LeaseProfile, PolicyRule, StaticBinding | `pkg/models/models.go` | Same pattern as pools—candidate to make tenant optional + tag with group labels. |
| AuditEvent | `pkg/models/models.go` | `TenantID` column becomes optional; should log `Principal`, `GroupIDs`, `Labels` for richer traceability. |
| RBAC Org Units / Assignments | `migrations/0013_rbac.sql` | Schema still tenant-scoped; Stage 2 migration will add group/label storage as described above. |
| HTTP handlers | `internal/server/http_server.go` | Hundreds of call sites still demand `(scopeID, tenantID)`; Stage 2+ will replace helper with AccessScope extraction. |

## Stage 2 progress

- **Migration**: [`migrations/0022_rbac_assignment_scope.sql`](../migrations/0022_rbac_assignment_scope.sql) adds `group_ids`, `labels`, and hash-backed `group_scope` / `label_scope` columns plus supporting indexes. Down migration drops them cleanly.
- **Repository wiring**: [`internal/rbac/models.go`](../internal/rbac/models.go) now persists `Assignment.Scope` by serializing group/label JSON and computing deterministic scope hashes for uniqueness/indexing. `Assignment.HydrateScope()` rebuilds the struct when reading from DB, and `PrepareScopeColumns()` is invoked before inserts.
- **Query updates**: [`internal/rbac/repository.go`](../internal/rbac/repository.go) selects the new columns, hydrates scopes after reads, and writes the serialized scope blobs during `CreateAssignment`.
- **Next**: introduce `accessScopeContext` in HTTP middleware so new scope metadata can flow from tokens to repositories, then begin migrating pool/lease services to the richer context.

## Stage 3 progress

- **Access scope propagation**: The HTTP server now caches a `resource.AccessScope` per request, deriving it from headers (`X-Resource-Scope`, `X-Access-Groups`, `X-Access-Labels`) and query hints. `requestTenantID`/audit helpers read from that scope instead of raw tenant IDs.
- **Principal context exposure**: Each request also builds and stores an `rbac.PrincipalContext` (user ID, session/credential, access scope). Capability resolution uses this context, so downstream services can become tenant-optional.
- **Resolver alignment**: [`internal/rbac/resolver.go`](../internal/rbac/resolver.go) enforces assignment scopes via `AssignmentScope.MatchesAccessScope`, falling back to org-unit checks. Legacy callers still work because the resolver injects the previous `TenantID` into the access scope when needed.
- **Pool/lease handlers**: HTTP and core-ops routes now derive `pool.ResourceScope` / `lease.ResourceScope` directly from the cached `AccessScope`, so downstream services receive the full group/label context instead of reconstructed tenant IDs.
- **Next**: push the richer `AccessScope` deeper into pool/lease service layers (quota checks, repositories, analytics) and update the documentation/UI so multi-group access controls are visible end-to-end.
