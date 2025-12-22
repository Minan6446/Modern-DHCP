# Section 12 – Monitoring & Diagnostics Implementation Plan

This plan decomposes the "Full-Scope Monitoring" requirement into deliverable workstreams. The strategy is to ship continuously—each milestone adds API capabilities, metrics, or tooling that downstream UIs can consume.

## Milestone A – Metrics Foundation (current sprint)
1. **Collector Expansion**
   - Add counters/histograms/gauges for:
     - DHCP request lifecycle (DISCOVER/OFFER/REQUEST/ACK success + failure variants)
     - Request latency distributions (per message type)
     - Pool utilization gauges (allocated vs capacity)
     - System resource probes (CPU, memory, disk, NIC throughput)
     - Database query timings & open connections
   - Wire into existing handlers, pool service, lease service, DB instrumentation hooks.
2. **Monitoring API Surface**
   - Introduce `/api/v1/monitoring/*` namespace with read-only endpoints:
     - `/dashboard/overview` – aggregates metrics for UI cards.
     - `/dashboard/pools` – per-pool utilization snapshots & trends.
     - `/dashboard/requests` – success counts + latency histogram buckets.
     - `/health/system` – CPU/memory/disk/network snapshots plus HA state.
3. **Data Sources**
   - Implement `monitoring.Aggregator` that reads Prometheus collectors + runtime stats (Go `runtime`, OS counters, DB stats, security guard signals).
   - Persist short-term trend buffers in-memory (ring buffer) for 5–15m charts; rely on Prometheus for long-term trends.

## Milestone B – Diagnostics Tooling
1. **Client Simulation Service**
   - Extend existing `/simulate` to accept scenario templates & multi-step flows; log outcomes for later replay.
2. **Packet Capture Hooks**
   - Provide PCAP exporter that taps into DHCPv4/v6 UDP processing pipeline (toggle via config & API trigger).
3. **Lease History Explorer**
   - New endpoints to fetch per-client lease timelines + guard/security events.
4. **Conflict Detection Console**
   - Surface conflict metrics + latest offending clients, integrate with guard signals.

## Milestone C – Performance Monitoring & Advanced Dashboards
1. **Histogram APIs** – expose response time percentile data built from Prometheus histograms.
2. **Database Metrics Feed** – real-time query latency, connection usage, replication lag.
3. **Resource Trend Jobs** – background sampler persisting resource stats for 24h horizon (store in Redis/MySQL for UI retrieval).
4. **Network Traffic Monitor** – periodic netlink/Win32 counters that summarize ingress/egress DHCP packet rates per interface.

## Today’s Deliverables
- Complete Milestone A items 1 & 2 partially:
  - Expand `metrics.Collector` and wire request lifecycle counters.
  - Implement `internal/monitoring` package with an `Aggregator` capable of producing overview + pool + request + system payloads (fed from collectors/runtime stats).
  - Add `/api/v1/monitoring/overview`, `/api/v1/monitoring/pools`, `/api/v1/monitoring/requests`, `/api/v1/monitoring/health` endpoints returning aggregator results.

Subsequent PRs will iterate on Milestone B & C.
