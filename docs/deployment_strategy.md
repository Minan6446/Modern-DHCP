# 17. 多样化部署计划

本章节将 Modern-DHCP 的多环境交付策略落地为可执行的清单，覆盖裸金属、虚拟化、容器、云原生与公有云托管五大模式，并定义弹性伸缩与混合云治理的统一方法。配套的示例配置可在 `configs/config.example.yaml` 的 `deployment.*` 段落中找到。

## 17.1 部署模式

### 17.1.1 裸金属（Bare Metal）
- **供应链**：以 iPXE/Metal³ 触发自动装机，Ansible 播种基础包、内核参数与 systemd 服务。
- **硬件基线**：双路 x86_64、≥128 GB RAM、NVMe RAID10、双 25/40/100GbE，使用 LACP Bond0 对接 ToR。
- **自动化**：Redfish/BMC API 负责上电、带外健康检查，装机后通过 `deployment.bareMetal.*` 参数注入 RAID/NIC 拓扑。
- **网络形态**：管理/租约/复制 网络分区，DHCP 中继通过 Anycast VIP 指向集群。

### 17.1.2 虚拟机（VMware / KVM / Hyper-V）
- **黄金镜像**：发布 OVA/QCOW2/VHDX，预装依赖并集成 cloud-init，允许在导入时注入节点 ID、集群角色。
- **平台支持**：vSphere（含 DRS/HA）、OpenStack/KubeVirt、Hyper-V SCVMM。`deployment.virtualMachine.hypervisors` 用于声明认证范围。
- **资源模板**：默认 8 vCPU/32 GB RAM/200 GB 系统盘，支持 SR-IOV vNIC；可通过 API 或 Terraform 模块批量克隆。
- **生命周期管理**：借助 vCenter/Libvirt hook 在热迁移和快照时通知 Modern-DHCP HA 协调器。

### 17.1.3 容器化（Docker 单机）
- **镜像**：发布至 GHCR/ECR，标签与 git tag 保持一致。入口脚本会根据 `SERVICE_MODE` 选择 DHCP/控制面组合。
- **编排**：提供 `deploy/docker/docker-compose.yaml`，拉起 DHCP 栈、MySQL、Redis 以及可选的 Kafka，方便 PoC/边缘场景。
- **数据持久化**：通过绑定卷挂载 `/var/lib/modern-dhcp`、`/var/log/modern-dhcp`，支持本地或 NFS 存储。
- **网络**：Host 模式暴露 67/68/547 UDP，管理面 8080/9090 走 bridge；可选 Macvlan 以满足与物理网络同段需求。

### 17.1.4 云原生（Kubernetes 集群）
- **Helm 发布**：`deployment.kubernetes.helmChart` + `valuesFile` 组合定义组件、副本数、探针与持久卷。
- **调度策略**：`nodeSelector`/`tolerations` 将 DHCP Pod 固定在专用 Node Pool，可选形态包含 EKS、AKS、GKE、ACK 以及 on-prem K8s。
- **Stateful 服务**：租约面可运行在 StatefulSet（带 local PersistentVolume），控制面和 API 采用 Deployment。
- **可观测性**：Prometheus Operator 自动抓取 `/metrics`，OpenTelemetry Collector 侧车发送 Trace。
- **Day-2**：通过 ArgoCD/Fleet 做 GitOps，同步 `deployment.scaling.*` 与 HPA/Cluster Autoscaler 策略。

### 17.1.5 公有云部署（AWS / Azure / 阿里云）
- **托管组件**：Aurora MySQL / Azure Database / PolarDB 作为权威存储，Redis Enterprise / Elasticache 充当缓存，MSK/Kafka on ACK 承载事件。
- **网络**：NLB/ALB/AGW 提供 Anycast/地域路由，Global Accelerator / Azure Front Door 负责跨区引流。
- **安全**：结合 IAM、STS、Key Vault/KMS/Secrets Manager 管理凭证；Security Group / NSG / 云防火墙匹配多端口需求。
- **自动化**：Terraform 模块生成 VPC/VNet、子网、Transit Gateway/VPN，Ansible 负责节点引导；CI/CD 通过 GitHub Actions + OIDC 接入云账号。

## 17.2 弹性伸缩

### 17.2.1 自动水平扩展
- 采集 `dhcp_lease_tps`, `requests_inflight`, `cpu_usage` 等指标，经 Prometheus Adapter 转换为 K8s HPA，或由裸机/VM 环境的扩缩控制器（Cluster API、vSphere DRS）驱动。
- `deployment.scaling.metric/scaleOutThreshold/scaleInThreshold` 定义扩缩触发点，`minReplicas/maxReplicas/cooldown` 则防止抖动。
- 在容器/虚拟化环境通过 placement API 为新节点注册到集群并广播到中继。

### 17.2.2 按需资源分配
- `deployment.resource.cpu|memoryGB` 设定可调区间与步长，调度器依据实时指标（CPU、RSS、GC pause）及租约 backlog 调整 cgroup/VM flavor。
- policy engine 推送 `triggerPolicies`（如 `cpu>70`）到运维管控层，自动提交 vCPU/RAM 扩展，完成后触发滚动重启或 Hot Add。

### 17.2.3 地理分布式部署
- `deployment.geo.regions` 为每个区域声明角色（active/standby/edge）与云/机房映射，HA 层依据 `latencyBudget` 挑选就近仲裁点。
- Edge 站点运行裁剪版容器栈，主站点通过 Kafka MirrorMaker + MySQL 异步复制同步租约事件，必要时可按区域进行防护隔离。
- 控制平面通过 Global Anycast + 地域优先算法（GeoDNS/Global Accelerator）下发管理 API 请求。

### 17.2.4 混合云部署支持
- `deployment.hybrid.*` 声明 VPN Mesh/Transit Gateway/Cloud WAN 拓扑，自动生成 IPsec / Direct Connect / ExpressRoute 配置片段。
- On-Prem/Cloud 以单一租户/地址池目录共享元数据，配置同步由 GitOps 或 Config Sync Backend（etcd/GCS/S3）驱动。
- 策略：`policy: prefer-onprem` 代表常态流量在本地结算，公有云节点作为弹性扩展或灾备；亦可设置 `cloud-first` 以满足边缘/移动场景。

## 17.3 交付工件速览
| 工件 | 说明 |
| --- | --- |
| `configs/config.example.yaml` | 展示 `deployment.*`、`scaling.*`、`geo.*` 与 `hybrid.*` 的推荐默认值。 |
| `deploy/docker/docker-compose.yaml` | PoC/边缘环境一键启动脚本，包含 DHCP + MySQL/Redis 依赖。 |
| `deploy/kubernetes/values.yaml` | Helm chart 参考值（可由 GitOps 同步）。 |
| `docs/deployment_strategy.md` | 本文档，详述第 17 章的运行手册。 |

通过以上清单，Modern-DHCP 可以在裸金属、虚拟化、容器、Kubernetes、公有云及混合形态下保持一致的运维体验，并满足弹性扩展与地理冗余的合规要求。

## 17.4 DHCPv4 UDP 并发参数

为满足 10 万+ 客户端与 10k TPS 的 Phase 3 指标，DHCPv4 listener 的并发度需要按环境调整。运行时可通过 `service.dhcpv4.*` 配置或 CLI 覆盖以下参数：

| 参数 | 作用 | 默认值 |
| --- | --- | --- |
| `workerCount` | 处理 DHCP 报文的 goroutine 数量 | `max(2, runtime.NumCPU())` |
| `queueDepth` | worker 队列长度 | `workerCount * 4` |
| `listenerFanout` | `SO_REUSEPORT` 监听器数量 | `min(runtime.NumCPU(), 16)` |
| `rxBatchSize` | 每个 listener 连续 `ReadFromUDP` 的批次 | `4` |
| `reusePort` | 是否启用 `SO_REUSEPORT`（非 Linux/*BSD 会自动回退） | `true` |

### 17.4.1 Docker Compose 示例

`deploy/docker/docker-compose.yaml` 现已将上述旗标透传给容器入口，可用环境变量一键覆盖：

```yaml
	dhcp:
		command:
			- /app/dhcpd
			- --config
			- /etc/modern-dhcp/config.yaml
			- --listeners
			- "${DHCPD_LISTENERS:-8}"
			- --dhcpv4-workers
			- "${DHCPD_WORKERS:-0}"
			- --dhcpv4-queue
			- "${DHCPD_QUEUE:-0}"
			- --rx-batch
			- "${DHCPD_RX_BATCH:-8}"
			- --reuseport
			- "${DHCPD_REUSEPORT:-true}"
```

在 PoC/边缘场景可将 `DHCPD_LISTENERS` 设为 `2-4`，主力节点则建议按 Numa/CPU 数设置为 `8-12`。

### 17.4.2 Kubernetes/Helm 覆盖

`deploy/kubernetes/values.yaml` 新增 `dhcpd.runtime` 块，可在 GitOps 流程中声明不同环境的并发度：

```yaml
dhcpd:
	runtime:
		listeners: 12
		workers: 48
		queueDepth: 512
		rxBatchSize: 8
		reusePort: true
```

Helm chart 可将这些值渲染到 `args` 或 `env`，GitOps pipeline 则基于区域/集群模板进行差异化配置，保证 Phase 3 的性能指标可控。