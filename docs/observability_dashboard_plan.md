# Phase 2 Observability Dashboard & Alerting Plan

## Objectives
- Give operators and tenant admins a single place to inspect pool utilization, DHCP lifecycle health, and infrastructure saturation in under 30 seconds.
- Close the feedback loop between the new multi-tenant auth model and monitoring payloads so every card/table can be scoped by tenant.
- Replace ad-hoc e-mail/pager rules with deterministic, configuration-backed alert pipelines that the `internal/monitoring` package can evaluate.
- Deliver UI-ready JSON payloads via `/api/v1/monitoring/*` endpoints so the Modern DHCP Web UI can hydrate dashboards without stitching Prometheus queries in the browser.

## Current Baseline
- `internal/monitoring.Aggregator` already assembles pool usage, request-phase counters, client distribution, system health, and guard telemetry.
- Metrics emitted via Prometheus (`modern_dhcp_pool_selector_*`, DHCP lifecycle counters, latency histograms) are available for SRE-driven dashboards.
- Alert scaffolding (`alert_controller.go`, `alert_rules.go`) exists but lacks concrete rule definitions, tenant scoping metadata, and notification routing.
- UI does not yet consume `/api/v1/monitoring/...` responses; there is no tenant-aware dashboard surface in `web/ui`.

## Dashboard Experiences
1. **Control Plane Overview (Ops/NOC)**
   - **Data**: `Aggregator.Overview` payload (`PoolUsage`, `RequestPhases`, `SystemHealth`, `Security`).
   - **Widgets**: utilization leader board, request success vs failures chart, CPU/memory/disk gauges, recent guard/rate-limit findings.
   - **Actions**: deep-link to pool detail and security incidents.
2. **Pool Utilization Explorer (Tenant Admin)**
   - **Data**: `Aggregator.Pools` + client distribution slices.
   - **Widgets**: heatmap by VLAN/location, sortable table with utilization %, inline sparklines fed by Prometheus range queries.
   - **Filters**: tenant switcher (reuses TenantContextDrawer), scope, VLAN, location.
3. **Request Lifecycle Tracker (Ops)**
   - **Data**: `Aggregator.Requests` snapshots + Prometheus histograms for latency percentiles.
   - **Widgets**: stacked area for DISCOVER/OFFER/REQUEST/ACK counts, latency percentile cards, error top causes (tie into `request_tracker` reasons).
4. **Security & Guard Board (Security/Ops)**
   - **Data**: `Aggregator.Security`, snooping tracker feeds, policy audit stream.
   - **Widgets**: rate-limit breach timeline, rogue DHCP detections, guard action list with tenant context.
5. **Alert Inbox**
   - **Data**: new `/api/v1/monitoring/alerts/feed` endpoint backed by `alert_feed.go`.
   - **Widgets**: severity-sorted list, acknowledge/assign controls (post-MVP), links to correlated dashboards.

## Alerting Upgrades
- **Rule Catalog**: author YAML (or JSON) definitions describing metric/query, threshold, comparison window, severity, tenant scope, and notification targets.
- **Evaluator Enhancements**:
  - Extend `alert_rules.Rule` to support PromQL expressions + Aggregator-sourced snapshots.
  - Implement per-tenant evaluation windows respecting the session tenant context.
  - Cache last-fired timestamps to avoid alert storms; expose status via `alert_controller`.
- **Notification Routing**:
  - Integrate with existing `internal/alerting` notifiers (email/webhook/PagerDuty) and tag payloads with tenant + auth provider for correlation.
  - Allow tenants to opt into self-managed webhooks via future UI panel.
- **Tactical Rules (MVP)**:
  1. Pool utilization > 85% for >5 minutes.
  2. DHCP REQUEST failures spike (>3σ above hourly mean) or >2% of traffic over 10 minutes.
  3. Guard detects >N snooping anomalies inside `securityWindow`.
  4. System resource saturation (CPU > 80% OR memory > 85% OR disk > 90%).

## Backend Delivery Plan
1. **Finalize Monitoring API**
   - Add `/api/v1/monitoring/overview|pools|requests|health|security` handlers calling the Aggregator.
   - Implement `/api/v1/monitoring/alerts/feed` (paged) & `/api/v1/monitoring/alerts/rules` (list active rule configs).
2. **Instrumentation Gaps**
   - Ensure `request_tracker` records tenant + failure reason for alert correlation.
   - Expose Prometheus summaries/histograms for latency percentiles consumed by dashboards.
3. **Alert Engine**
   - Parse rule configs on boot, add hot-reload via `SIGHUP` or config watcher.
   - Schedule evaluation loop (per tenant) and feed results to `alert_feed` + notifiers.
   - Persist alert state in Redis/MySQL for UI fetch + audit.

## Frontend Delivery Plan
1. **API Client**: add `monitoringService` with helpers (overview, pools, requests, alerts) mirroring backend payloads.
2. **Dashboard Shell**: create `MonitoringWorkspace` route with tenant-aware layout (header cards, charts, alert feed panel).
3. **Charting**: leverage existing chart lib (ECharts) for utilization + lifecycle charts; reuse Element Plus tables for pools/alerts.
4. **Alert Feed UI**: reusable list component showing severity chips, timestamps, tenant, action links.
5. **Testing**: Vitest + Playwright coverage for store/service logic and dashboard rendering with mocked API responses.

## Rollout & Validation
- **Integration Tests**: extend `internal/monitoring/aggregator_test.go` and add new HTTP handler tests to cover tenant scoping & pagination.
- **Load Verification**: use perf benchmarks (`perf/benchmarks`) to simulate 100 tenants * 50 pools to validate Aggregator latency < 150ms.
- **Dashboard Smoke Tests**: scripted Grafana JSON models to ensure Prometheus queries align with labels described in `metadata_observability.md`.
- **Alert Dry Runs**: run evaluator in shadow-mode for 48 hours storing results only; confirm thresholds before enabling notifications.

## Timeline (Target)
1. Week 1: Backend endpoints + request tracker enhancements.
2. Week 2: Alert rule ingestion + evaluator + persistence.
3. Week 3: UI dashboard shell + overview/pool widgets + alert feed MVP.
4. Week 4: Chart polish, tenant guard board, dry-run alerts → enable notifications per tenant.

This plan fulfills the "Plan observability dashboard & alerting upgrades" milestone and provides actionable steps for both backend and frontend teams.
