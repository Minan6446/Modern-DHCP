# Performance Benchmarks

This directory holds micro-benchmarks that gate the DHCP parser and serializer hot paths. Run them before and after each performance change so regressions show up immediately.

## How to run

```powershell
cd c:/Users/minan/Desktop/Modern-DHCP
go test ./perf/benchmarks -bench . -benchmem
```

## Baseline (2025-12-03, Ryzen 7 5800U)

| Benchmark              | ns/op | B/op | allocs/op |
|------------------------|-------|------|-----------|
| BenchmarkDHCPv4Parse   | 479.5 | 752  | 11        |
| BenchmarkDHCPv4Marshal | 421.4 | 760  | 4         |

Re-run these benchmarks after implementing the connection-pool overrides and UDP batching to verify we only improve latency/allocations. Keep this table updated with the most recent measurements from CI or dedicated perf hardware.
