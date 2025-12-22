# Deployment Artifacts

This directory hosts runnable references for Section 17 of the Modern-DHCP plan.

| Path | Purpose |
| --- | --- |
| `docker/docker-compose.yaml` | Spins up Modern-DHCP alongside MySQL and Redis for PoC/edge tests. |
| `kubernetes/values.yaml` | Opinionated Helm values capturing production defaults and HPA targets. |

Copy `configs/config.example.yaml` as the base configuration and adjust the `deployment.*` section to match your environment before applying these manifests.
