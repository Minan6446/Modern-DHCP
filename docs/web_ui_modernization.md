# Section 19 – Modern Web Interface Blueprint

## 1. Objectives
- Deliver a single-page application that covers management, visualization, and collaboration workflows on desktop, tablet, and mobile form factors.
- Provide theming, localization, and accessibility as first-class capabilities so UI policies can be enforced per tenant.
- Unlock intuitive visual tooling (drag & drop pool builder, topology maps, lease geography, data center 3D) tightly integrated with Modern-DHCP APIs.
- Enable collaborative operations (co-editing, comments, approvals, task assignment) that produce auditable records consistent with the platform's governance model.

## 2. Reference Architecture
| Layer | Responsibilities | Technology Candidates |
| --- | --- | --- |
| Presentation | React 18 + TypeScript SPA, component library (MUI v6 / Chakra / custom design system) with CSS Grid/Flexbox, theme tokens, translation hooks | React, Vite build, Emotion/Styled Components |
| Data/State | RTK Query / React Query for REST data, Zustand for transient UI state, WebSocket channel for collaboration presence and optimistic lock heartbeats | Redux Toolkit, WebSocket, Server-Sent Events |
| Visualization | Recharts/ECharts for 2D charts, Mapbox GL for lease map, Cytoscape.js for topology, Three.js for 3D racks, Drag & Drop via `dnd-kit` | Mapbox, Cytoscape, Three.js, dnd-kit |
| Collaboration | Presence service, comment stream, approval workflows surfaced via side panels, ties into audit trail and notifications | WebSocket hub, Kafka topics, audit service |
| Backend Support | UI metadata endpoint (themes, locales, accessibility features), visualization APIs (topology graph, geo heatmap), collaboration APIs (lock tokens, comments, tasks, approvals) | Existing Go HTTP server with new routes + services |

## 3. Responsive & Theming Strategy (19.1)
- **Adaptive Layouts**: Establish design tokens for breakpoints (`xs <600px`, `sm <960px`, `md <1280px`, `lg <1920px`, `xl`), with CSS clamp utilities to keep forms and canvases usable on narrow screens.
- **Theme System**: Define base palettes (light/dark) with semantic tokens (surface, accent, success/warn/error). Support per-tenant overrides stored via API; browser `prefers-color-scheme` used as default.
- **Localization**: Manage translations through ICU message catalogs; shipped languages: zh-CN, en-US, ja-JP. Provide locale negotiation via `Accept-Language`, profile settings, or URL query.
- **Accessibility**: Meet WCAG 2.1 AA—keyboard focus order, high contrast toggle, ARIA labels on interactive controls, adjustable font scale (90–130%), motion-reduced animations, screen-reader friendly SVG descriptions.

## 4. Visualization Toolkit (19.2)
1. **Drag & Drop Pool Builder**
   - Canvas-based editor with hierarchical nodes (global → region → subnet → VLAN → reservation).
   - Snap-to-grid alignment, constraint validation powered by `/api/v2/pools/validate`.
   - Drafts persisted via optimistic locking with auto-save.
2. **Network Topology Graph**
   - Supports auto-discovered graph ingestion (`/api/v2/topology/graph`) plus manual edits.
   - Layers for physical, logical, and overlay networks; manual nodes include routers, switches, relay agents.
   - Live status overlays (health, latency, utilization) sourced from monitoring service.
3. **Lease Map View**
   - Mapbox/Leaflet heatmap representing lease density, security alerts, churn; filters by tenant, pool, time.
   - Integrates geocoding for relay locations/IP heuristics.
4. **3D Data Center Visualization**
   - Three.js scene graph showing rooms → aisles → racks → equipment; overlays lease/pool usage and HA pairings.
   - Supports camera presets, annotations, and screenshot exports for reports.

## 5. Collaboration Features (19.3)
- **Real-time Co-editing**: Optimistic lock tokens issued by `/api/v2/collab/locks`. Heartbeat via WebSocket; stale locks auto-released. Presence indicators show editing users.
- **Comments & Annotations**: Threaded comments attached to pools, policies, topology elements. Stored via `/api/v2/collab/comments`, referencing audit correlation IDs.
- **Approval Workflow**: Configurable multi-stage approvals (e.g., operator → net-admin → auditor). Workflows defined per change template, tracked through `/api/v2/collab/approvals` with SLA timers.
- **Task Assignment & Tracking**: Kanban-style board for operational tasks linking back to configuration items; notifications emitted through existing events bus/Kafka topics.

## 6. Backend Enhancements Required
- **Configuration**: Extend config with `ui.*` block (themes, locales, accessibility toggles, collaboration policies).
- **Services**: New packages—`internal/ui/metadata`, `internal/ui/collab`, `internal/ui/visualization`—for caching metadata, managing locks, serving topology snapshots, and proxying telemetry.
- **API Endpoints**: `/api/v1/ui/metadata`, `/api/v2/ui/themes`, `/api/v2/visualization/*`, `/api/v2/collab/*` for locks/comments/approvals/tasks.
- **RBAC Metadata Envelope**: `/api/v1/ui/metadata` now emits `{ catalog, granted, temporary }` capability sets so the SPA can drive nav visibility and feature flags client-side while keeping temporary grants explicit.
- **Storage**: Additional tables (`ui_preferences`, `collab_sessions`, `collab_comments`, `approvals`, `tasks`) with version columns for optimistic locking.
- **Events**: Kafka topics `ui.collab.events` and `ui.approvals.events` to broadcast updates to connected clients.

## 7. Roadmap & Milestones
1. **Milestone A – Foundation**
   - Add config schema + metadata endpoint.
   - Scaffold SPA workspace (Vite + React + Storybook) and CI build.
   - Implement localization/theming primitives and auth bootstrap screens.
2. **Milestone B – Visualization Beta**
   - Deliver drag/drop pool builder MVP and topology view with read-only data.
   - Implement lease heatmap and connect to monitoring APIs.
   - Add HA overlay data to topology.
3. **Milestone C – Collaboration Alpha**
   - Ship optimistic locking, presence indicators, comment threads.
   - Introduce approval workflow service + API, integrate with audit logs.
   - Task board UI with Kafka-backed notifications.
4. **Milestone D – Production Hardening**
   - Accessibility audit, localization QA, load testing for WebSocket hub.
   - Offline caching, deep-linking, RBAC-driven feature flags.
   - End-to-end documentation, runbooks, and demo datasets.

## 8. Acceptance Criteria
- UI responds within 200 ms to navigation on desktop, 400 ms on mobile (P95) under 2k concurrent sessions.
- Accessibility score ≥ 95 (Lighthouse) with keyboard-only coverage for all critical paths.
- Localization coverage ≥ 95% strings for supported languages; fallback logic verified.
- Collaboration conflicts resolved within 3 seconds; audit trail captures every approval/comment/task event.
- Visualization components gracefully degrade (fallback table view) when WebGL unavailable.

## 9. RBAC Visibility & Audit Hooks
- **Capability-aware shell**: The SPA consumes `/api/v1/ui/metadata` and feeds the response into `usePermissions`, which drives navigation groups, action buttons, and high-risk flows (leases, pools, audit logs). Sections with unmet capability requirements stay hidden to avoid leaking affordances to unauthorized principals.
- **Session-bound credential injection**: The shared Axios client injects `Authorization`, `X-Tenant-ID`, `X-Principal-ID`, `X-Actor-Name`, and `X-Audit-Source: modern-dhcp-ui` headers for every outbound call, keeping backend audit middleware aware of the tenant, principal, and UI context without repeating boilerplate in each service method.
- **Automatic token revocation**: Interceptors watch for `401` responses and immediately clear the Pinia session store so new requests are forced through the login screen, preventing stale tokens from being reused after backend revocation.
- **Operator intent surfaced to audit trail**: Any mutation triggered from the console (policy changes, lease actions, pool approvals) carries the actor metadata above, enabling `internal/audit` to stitch UI events with server-side audit payloads and SIEM exports.

## 10. Request Validation & Parameter Guardrails
- **Dual-stage validation**: Filters and advanced search inputs (IP ranges, MAC 前缀,最近活动窗口) now run through `useLeaseFilterGuards`, enforcing IPv4 syntax, prefix structure, and numeric bounds before Axios ever fires. Any violation renders inline form errors plus a compact banner so operators correct issues locally.
- **Normalized payloads**: Successful validation produces a sanitized payload (trimmed query text, uppercase MAC prefixes, clamped minute windows) that is sent to `/v1/core/leases`. The backend still performs canonical Path/Query/Body validation and business rules, giving us the mandated“双重防护”.
- **Shared utilities**: `src/utils/validation.ts` encapsulates IP, MAC, and integer helpers so other forms (policy builders, tenant filters) can plug into the same guardrails without duplicating regexes or edge-case math.
