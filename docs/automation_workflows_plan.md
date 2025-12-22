# Phase 2 Automation Workflows & UI Tooling Plan

## Objectives
- Provide tenant-aware automation so inventory sync, policy audits, security scans, and alert fanout can be orchestrated without manual CLI usage.
- Expose clear operator and tenant-admin controls in the Modern DHCP UI to schedule, pause, and inspect automation jobs.
- Integrate automation state with notifications so alerting upgrades can reuse fanout workflows with minimal duplication.
- Ensure every workflow is observable (metrics + logs) and auditable, aligning with Phase 2 multi-tenant access controls.

## Current Baseline
- `internal/automation` offers an in-memory scheduler (`Scheduler`), recurring schedules (`Service`), predefined `JobType` constants, and a `NotificationFanoutHandler` that bridges to `internal/notifications`.
- No persistent queue or job history; runtime state is limited to `Snapshot()` diagnostics.
- Config lacks declarative schedule definitions; enabling/disabling workflows requires code changes.
- UI has no surfaces for automation (no list of schedules, no job inspector, no run-now controls).

## Workflow Catalog (Phase 2)
1. **Inventory Sync (`inventory.sync`)**
   - Pull CMDB/asset inventories to refresh pool metadata + device fingerprints per tenant.
   - Inputs: tenant ID, source endpoints, delta window.
2. **Policy Audit (`policy.audit`)**
   - Re-evaluate policy baselines (RBAC, quotas, guardrails) and produce compliance reports.
   - Inputs: tenant, policy set IDs, severity thresholds.
3. **Security Scan (`security.scan`)**
   - Run DHCP snooping/rogue detection tasks; feed results into guard + alerting pipelines.
4. **Analytics Snapshot (`analytics.snapshot`)**
   - Export monitoring aggregates + lease statistics for offline BI jobs.
5. **Notification Fanout (`notification.fanout`)**
   - Distribute alert payloads to email/webhook/PagerDuty channels based on automation outcomes.
6. **Workflow Execution (`workflow.execution`)**
   - Execute user-authored multi-step workflows (Phase 2.5) referencing a definition stored in DB.

Each workflow requires tenant scoping, labels for filtering, and payload schemas published to docs/api.

## Backend Delivery Plan
1. **Config-Driven Schedules**
   - Introduce `automation.schedules` section in config YAML with entries per `JobType` (enabled, interval, tenant, payload, channels).
   - Extend `cmd/dhcpd/main.go` wiring to parse config and feed `ServiceOptions.Schedules`.
2. **Handler Implementations**
   - Build dedicated handlers under `internal/automation/workflow/` for inventory sync, policy audit, security scan, analytics export, workflow execution. Each handler should:
     - Accept context + job payload, validate schema.
     - Publish audit events and metrics tags (`automation_job_duration_seconds`, `automation_job_failures_total`).
     - Support idempotent retries (respect `Job.Attempts`).
3. **Persistence & History**
   - Add lightweight persistence (PostgreSQL/MySQL) for job runs: `automation_jobs` table capturing id, type, tenant, payload hash, status, timestamps, output summary.
   - Provide API endpoints `/api/v1/automation/jobs` (list/filter) and `/api/v1/automation/jobs/{id}` (details/logs).
4. **Runbook APIs**
   - Add `/api/v1/automation/schedules` (GET) returning `ScheduleSummary` and PATCH endpoint to toggle intervals/tenants.
   - Provide `POST /api/v1/automation/jobs` to allow UI to trigger ad-hoc runs with payload override (subject to RBAC scopes).
5. **Notification Integration**
   - Ensure security scan + policy audit handlers enqueue `notification.fanout` jobs with severity derived from findings.
   - Add mapping between automation outcomes and alert rules (e.g., policy audit failure => alert feed entry).
6. **Observability**
   - Emit Prometheus metrics per job type: counts, durations, retry attempts, in-flight worker count.
   - Extend `Scheduler.Snapshot()` to include average wait time and expose via `/api/v1/monitoring/automation` endpoint.

## UI Tooling Plan
1. **Automation Service Module**
   - Create `web/ui/src/services/automation.ts` with methods: `fetchSchedules`, `updateSchedule`, `listJobs`, `getJob`, `runJobNow`.
   - Wire authentication headers + tenant context automatically.
2. **Automation Workspace Route**
   - Build new route `/automation` (guarded by RBAC `automation:manage`). Layout includes:
     - Schedule table (job type, interval, tenant, status, next run) with enable/disable toggle + edit drawer.
     - Job history panel (filter by tenant/job type/status, virtualization for large lists).
     - Job detail drawer showing payload, logs, completion metrics, linked notifications.
3. **Run-Now & Payload Builder**
   - Provide modal to trigger ad-hoc jobs with payload templates specific to each workflow (pre-filled fields for tenant, filters, channels).
   - Validate JSON before submit; show computed `channels` result (uses backend `MergeChannels` behavior as reference).
4. **Notification Hooks**
   - When a job generates alerts, surface them inline (chips linking to Alert Inbox) so operators can correlate automation + alerting.
5. **Tenant Awareness**
   - Reuse TenantContextDrawer state to scope list queries; tenant admins see only their jobs/schedules while super admins can view all.
6. **Testing**
   - Vitest unit tests for service/store logic with mocked API.
   - Playwright e2e covering schedule toggle, job run, and detail inspection flows.

## Rollout & Validation
- **Week 1**: Config schema + backend schedule ingestion, persistence schema migration, API endpoints for schedules/jobs.
- **Week 2**: Implement job handlers (inventory, policy, security, analytics) with metrics + notification fanout integration.
- **Week 3**: Build UI workspace, service layer, and wiring for tenant context + RBAC.
- **Week 4**: Add run-now UX, job detail view, alert correlations, and e2e tests; run shadow deployments to validate retry/backoff behavior.

This plan completes the "Design automation workflows & UI tooling" milestone and outlines concrete backend and frontend deliverables for Phase 2 automation.
