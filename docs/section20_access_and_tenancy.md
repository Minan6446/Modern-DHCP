# Section 20 – Access Control & Multi-Tenancy

## 20.1 Fine-Grained RBAC

### Current Posture
- Authentication (`internal/server/auth.go`) resolves **one** role per request (`reader` or `admin`).
- Enforcement (`internal/server/rbac.go`) is weight-based; every HTTP route simply requires `RoleReader` or `RoleAdmin`.
- There is no role catalog persisted in the database, no scoped assignments, no inheritance, and no workflow to approve elevated access.
- Audit coverage captures API activity but not the reason a role was granted or who approved it.

### Target Roles & Capabilities
We introduce a canonical role catalog with explicit capabilities. Roles inherit the capabilities of the roles listed above them:

| Role | Description | Inherits | Key Capabilities |
| --- | --- | --- | --- |
| `super_admin` | Platform owner with cross-tenant powers | – | Manage tenants/org tree, override quotas, approve privileged grants, read/write everything |
| `network_admin` | Tenant-level network architect | `auditor`, `read_only` | Create/update pools, policies, DHCP servers, manage quotas within tenant |
| `operator` | Day-2 operations team | `read_only` | Execute lifecycle actions (release leases, rotate keys), manage bindings, run reports |
| `auditor` | Compliance / SecOps | `read_only` | Read-only + audit log export, approve/deny temp grants |
| `read_only` | Observers | – | List-only APIs, dashboards, download reports |

Capabilities map one-to-many onto HTTP handlers (e.g., `pool.create`, `policy.update`, `lease.release`). Enforcement upgrades from boolean role checks to capability checks.

### Data Model Additions
```
rbac_roles (id, name, inherits_from, description)
rbac_org_units (id, parent_id, tenant_id, name)
rbac_assignments (id, principal_id, role, tenant_id NULLABLE, org_unit_id NULLABLE,
                  resource_type, resource_id, created_by, created_at)
rbac_temp_grants (id, assignment_id, requested_by, reason, expires_at,
                  status[pending|approved|rejected|revoked], approved_by, approved_at)
rbac_approval_rules (id, role, scope, min_approvers, approver_role)
```
Key behaviors:
- **Organizational inheritance**: grants tied to `rbac_org_units` automatically apply to descendant org units (modeled as adjacency list; queries leverage recursive CTEs).
- **Tenant vs. global scope**: `tenant_id = NULL` implies platform scope (used by `super_admin` and shared services).
- **Resource scoping**: optional `resource_type/resource_id` columns support fine-grained locks (e.g., per-pool operator).

### Runtime Enforcement Plan
1. **Role Resolver** (new package `internal/rbac,resolver.go`):
   - On request, combine static API key metadata, JWT claims, persistent assignments, and active temporary grants.
   - Cache resolved capability sets per `(principal, tenant)` for ~60s with audit-aware invalidation hooks.
2. **Authorization Layer**:
   - Replace `RequireRole` middleware with `RequireCapability("pool.update")` wrappers.
   - Extend `authContext` to carry `PrincipalID`, `TenantScopes`, `Capabilities`, `GrantMetadata`.
   - HTTP handlers that select tenant IDs must cross-check that the capability is granted for the requested tenant/org unit.
3. **Audit Hooks**:
   - Each grant/approval writes to existing audit stream via `audit.Service` with structured payload (grant ID, scope, ttl, approver).

### Temporary Grants & Approval Workflow
- Operators request elevation via `POST /api/v1/rbac/requests` with desired role, tenant scope, and duration.
- Requests are stored in `rbac_temp_grants` with status `pending`.
- Approval rules (e.g., "operator -> network_admin requires 1 auditor") are evaluated by a workflow engine:
  - Notification fan-out uses the existing collaboration subsystem (reuse `collab_events` to stream approval notifications across `/ws/collab`).
  - Approvers act via `POST /api/v1/rbac/requests/:id/approve`.
- Approved grants become live assignments tagged with `expires_at`; a background janitor job revokes expired grants and broadcasts the revocation over websockets and audit logs.

### API & UI Touchpoints
- `GET /api/v1/rbac/roles` – role catalog (super admin only).
- `GET /api/v1/tenants/:tenantId/rbac/assignments` – tenant scoped view.
- `POST/DELETE /api/v1/tenants/:tenantId/rbac/assignments` – permanent grants (requires `super_admin` or delegated `network_admin`).
- `POST /api/v1/rbac/requests` – temporary grant workflow.
- SPA receives capability map via `/api/v1/ui/metadata` to toggle UI affordances per user.

### Migration Strategy
1. Backfill `rbac_roles` with the five canonical roles.
2. Migrate existing API key roles:
   - `admin` -> `network_admin` scoped to all tenants (or `super_admin` for platform keys flagged in config).
   - `reader` -> `read_only`.
3. Provide a CLI (`cmd/dhcpd admin migrate-rbac`) to seed assignments per tenant.
4. Enable enforcement in staged mode (log-only) before flipping the `rbac.enforceCapabilities` flag.

---

## 20.2 Multi-Tenant Isolation & Branding

### Current Posture
- All tenants share the same database schema; isolation relies solely on `tenant_id` filters.
- Resource limits are implicit (none enforced per tenant). API quotas exist but are per-key/per-role, not per tenant.
- Policy evaluations and monitoring cache are multi-tenant but not namespace-isolated beyond tenant ID parameters.
- Branding is globally configured via `config.UI.*`; `models.Tenant.BrandTheme` is unused.

### Isolation Strategy
1. **Database-Level Isolation**
   - Introduce a tenant topology registry (`tenants` table already exists) with extended fields: `db_driver`, `db_dsn`, `schema`.
   - Refactor persistence to go through a `storage.TenantRouter` that hands out `*sqlx.DB` handles per tenant. Default mode keeps current shared DB; when `tenant.db_mode = dedicated`, router opens a dedicated connection (separate schema or dedicated database).
   - Migration runner receives a `--tenant` flag to migrate dedicated schemas individually.
2. **Data Access Enforcement**
   - Service layer (`lease.Service`, `pool.Service`, `policy.Service`, etc.) already take `tenantID`; augment repositories with `TenantScoped` interfaces that assert the router-provided handle belongs to the tenant before executing queries.
   - Add integration tests that attempt cross-tenant access to ensure router blocks them.

### Resource Quotas
- New table `tenant_quotas (tenant_id, pool_limit, lease_limit, client_limit, updated_at)`.
- Extend `metrics.Collector` to expose per-tenant usage counters (pools created, active leases) fetched from monitoring aggregator.
- Enforce quotas in `pool.Service.Create`, `lease.Service.AllocateOrReuseWithMetadata`, and binding APIs. Violations return `409` with structured payload.
- Provide `GET/PUT /api/v1/tenants/:tenantId/quotas` for super admins + delegated `network_admin` (if allowed by policy).

### Policy & Strategy Isolation
- `policy.Engine` gains a namespace parameter -> compile one policy bundle per tenant.
- `monitoring.Aggregator` caches results per tenant/DB handle.
- Cross-tenant operations (e.g., failover or global reports) require `super_admin` capability `tenant.read_all`.

### Tenant Branding
- Extend tenant metadata with `logo_url`, `primary_color`, `accent_color`, `login_message`.
- API: `GET /api/v1/tenants/:tenantId/branding` (read-only) and `PUT` for authorized roles.
- `ui/metadata` handler adds a `branding` block populated from tenant context (prefer request header `X-Tenant-ID` or JWT claim). SPA uses it to theme login shell and console.

### Implementation Phases
1. **Foundations**
   - Land `storage.TenantRouter` + config toggles.
   - Create new migrations for RBAC tables + tenant quota metadata.
2. **RBAC Integration**
   - Ship resolver + middleware, flip enforcement in shadow mode.
   - Deliver temp grant APIs + approval workflow UI endpoints.
3. **Tenant Isolation Enhancements**
   - Introduce tenant-specific DB handles, quotas, and policy caches.
   - Wire branding endpoints + SPA consumption.
4. **Operational Hardening**
   - Add Prometheus metrics for grant counts, quota usage, tenant DB health.
   - Document runbooks (new `docs/tenant_isolation_runbook.md`).

### Testing
- Unit: resolver, router, quota enforcement, approval workflow.
- Integration: multi-tenant DB harness to ensure no cross-tenant reads/writes.
- E2E: SPA scenario for temporary privilege request + approval + expiry.

### Rollout Considerations
- Config flag `rbac.enforceCapabilities` controls switch from legacy roles to capability checks.
- Config flag `tenancy.mode` (`shared`, `schema-per-tenant`, `database-per-tenant`).
- Provide migration scripts + dashboards to monitor tenant DB health and quota breaches during rollout.
