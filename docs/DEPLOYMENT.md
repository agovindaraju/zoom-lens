# Zoom-Lens — Deployment Guide

This guide deploys Zoom-Lens on **any Kubernetes cluster** (a local
`kind` cluster or an existing one), step by step or in one go.

- One-shot automation: [`scripts/deploy-all.sh`](../scripts/deploy-all.sh) (`make deploy`)
- Each step is also a separate script (`scripts/00-…` to `scripts/06-…`) and a `make` target
- Design background: [`ARCHITECTURE.md`](ARCHITECTURE.md)
- Demo workload and fault scenarios: [`demo/docs/DEMO.md`](../demo/docs/DEMO.md)

---

## Contents

1. [What gets deployed](#1-what-gets-deployed)
2. [Prerequisites](#2-prerequisites)
3. [Configuration](#3-configuration)
4. [Quick start (one go)](#4-quick-start-one-go)
5. [Step by step](#5-step-by-step)
6. [Running the demo scenarios](#6-running-the-demo-scenarios) (moved to the demo guide)
7. [Observing Zoom-Lens](#7-observing-zoom-lens)
8. [Troubleshooting](#8-troubleshooting)
9. [Teardown](#9-teardown)
10. [Sources](#sources)

---

## 1. What gets deployed

| Component | Namespace | Image | Role |
|---|---|---|---|
| kube-prometheus-stack (Helm) | `monitoring` | upstream chart `91.8.2` | Prometheus (storage + alert rules, remote-write receiver on), Alertmanager (webhook to Zoom-Lens), Grafana, node-exporter, kube-state-metrics. **Chart scraping of workloads is disabled**: collectors are the only scrapers |
| Root alert rules | `monitoring` | — | `ZoomLensAppLatencyHigh`, `ZoomLensNodeDiskBusy` (on **root** metrics only) |
| Grafana dashboard | `monitoring` | — | "Zoom-Lens: expand-on-signal metrics" |
| **Zoom-Lens operator** | `zoomlens` | built here: `zoomlens-operator` | Alertmanager webhook, dwell ladder, OpAMP server, eBPF pod manager |
| **Collector DaemonSet** | `zoomlens` | built here: `zoomlens-collector` (OpAMP Supervisor + otelcol-contrib 0.161.0) | One per node. Baseline (roots only) from a local file; incident overlays over OpAMP |
| eBPF exporter pods | `zoomlens` | `ghcr.io/cloudflare/ebpf_exporter:v2.5.1` | **Created only during an incident**, only on the affected node |

Only three images are built from this repository (the third, the demo app, is optional: see the demo guide). All third-party images are pinned
(`config/zoomlens.env`) and were checked to exist for both `amd64` and `arm64`.

---

## 2. Prerequisites

### 2.1 Tools on the machine that runs the scripts

| Tool | Tested with | Needed for | Install |
|---|---|---|---|
| Docker (with BuildKit) | Docker 24+ / Rancher Desktop / Docker Desktop | building images | https://docs.docker.com/get-docker/ |
| kubectl | 1.35 (any ≥ 1.28) | everything | https://kubernetes.io/docs/tasks/tools/ |
| helm | 3.18 (any ≥ 3.13) | kube-prometheus-stack | https://helm.sh/docs/intro/install/ |
| curl, sed, bash | macOS / Linux defaults | scripts (bash 3.2+ is fine) | preinstalled |
| kind *(optional)* | ≥ 0.20 | only for a local kind cluster | https://kind.sigs.k8s.io/docs/user/quick-start/#installation |
| python3 *(optional)* | 3.9+ | nicer output of `scripts/status.sh` | preinstalled on most systems |
| Go *(optional)* | 1.25 | only for `make test` (images build Go inside Docker) | https://go.dev/dl/ |

macOS with Homebrew: `brew install kubectl helm kind`.

### 2.2 Cluster requirements

| Requirement | Why | Notes |
|---|---|---|
| Kubernetes ≥ 1.28 (any maintained version) | standard APIs only (Deployments, DaemonSets, Jobs, RBAC, Pod Security labels) | kind, kubeadm, k3s, EKS/GKE/AKS all work for the core demo |
| **2+ nodes** recommended | "expand OUT to other nodes" is only visible with several nodes | single node works, with a warning |
| A **default StorageClass** | demo PVC (demo only) | kind and managed clusters ship one |
| **Privileged pods + hostPath allowed** in namespace `zoomlens` | collector mounts `/` read-only for host metrics; eBPF pods are privileged | the namespace is labelled `pod-security.kubernetes.io/enforce: privileged` |
| Linux nodes, **kernel ≥ 5.15 with BTF** | only for the eBPF level (L2+) | `ls /sys/kernel/btf/vmlinux` on a node; without it the eBPF pod fails but everything else works |
| Image pulls from Docker Hub, ghcr.io, gcr.io, quay.io, registry.k8s.io | chart and pinned images | mirror them for air-gapped clusters |

### 2.3 Where images go

| Cluster | What the scripts do | What you set |
|---|---|---|
| **kind** | `kind load docker-image` into every node, no registry needed | nothing |
| **any other cluster** | `docker push` to your registry | `ZL_REGISTRY=registry.example.com/zoomlens` (and `docker login` first) |
| other CPU architecture than your laptop | cross-build | `ZL_PLATFORM=linux/amd64` (for example) |
| corporate network needing a Go proxy | used inside the image build | `ZL_GOPROXY=https://your-proxy,direct` |

> **kind caveat:** all kind "nodes" are containers on one host (or one Docker VM), so
> they share a kernel. Host-level and eBPF metrics look the same on every node.
> Use kind for development; record the final demo on real VMs.

---

## 3. Configuration

All settings live in [`config/zoomlens.env`](../config/zoomlens.env). Every value can be
overridden from the environment, for example
`ZL_REGISTRY=ghcr.io/me ZL_IMAGE_TAG=v1 ./scripts/deploy-all.sh`.

| Variable | Default | Meaning |
|---|---|---|
| `ZL_IMAGE_TAG` | `dev` | Tag of the three images built here |
| `ZL_REGISTRY` | *(empty)* | Empty = kind load; otherwise push to this prefix |
| `ZL_KIND_CLUSTER` | `zoomlens` | kind cluster name |
| `ZL_PLATFORM` | *(empty)* | `docker build --platform`, e.g. `linux/amd64` |
| `ZL_GOPROXY` | `https://proxy.golang.org,direct` | Go module proxy used inside the build |
| `ZL_OTEL_VERSION` | `0.161.0` | Collector + Supervisor version |
| `ZL_EBPF_IMAGE` | `ghcr.io/cloudflare/ebpf_exporter:v2.5.1` | eBPF exporter image (must contain `/examples`) |
| `ZL_KPS_CHART_VERSION` | `91.8.2` | kube-prometheus-stack chart |
| `ZL_KPS_RELEASE` / `ZL_MONITORING_NS` | `kps` / `monitoring` | Helm release / namespace |
| `ZL_WAIT_TIMEOUT` | `600s` | Rollout / Helm wait timeout |

Tuning:

| What | File |
|---|---|
| Dwell ladder (L1/L2/L3 timing, depth, hops, interval, eBPF) | [`deploy/operator/ladder.yaml`](../deploy/operator/ladder.yaml) |
| Metric trees, relations, entity graph | [`knowledge/`](../knowledge/README.md) |
| Root metrics collected at L0 | [`deploy/collector/base-config.yaml`](../deploy/collector/base-config.yaml) |
| Alert thresholds | [`demo/monitoring/alert-rules.yaml`](../demo/monitoring/alert-rules.yaml) |
| Operator flags (budget, grace, intervals…) | env vars in [`deploy/operator/operator.yaml`](../deploy/operator/operator.yaml), see `zoomlens -h` |

---

## 4. Quick start (one go)

**A. New local kind cluster** (Docker + kind installed):

```bash
./scripts/deploy-all.sh --create-kind      # or: make deploy-kind
```

**B. Existing cluster** (current kubectl context):

```bash
kubectl config current-context             # make sure this is the right cluster
# kind cluster: nothing else needed. Any other cluster:
export ZL_REGISTRY=registry.example.com/zoomlens   # a registry your nodes can pull from
docker login registry.example.com
./scripts/deploy-all.sh                    # or: make deploy
```

It runs steps 0–6 below and ends with:

```
==> Done in NNNs
    Grafana:   make grafana      (admin / zoomlens)  -> dashboard 'Zoom-Lens: expand-on-signal metrics'
    Status:    ./scripts/status.sh --watch
    Scenario:  ./demo/scripts/scenario.sh cpu start    (stop: ./demo/scripts/scenario.sh cpu stop)
```

Typical duration: 8–15 minutes on the first run (image builds + chart), 2–4 minutes after.
Re-running is safe: every step is idempotent (`helm upgrade --install`, `kubectl apply`).
Use `--skip-build` to reuse images already built.

---

## 5. Step by step

Run from the repository root. Each step prints `✔` lines on success and stops on the
first error.

### Step 0 — Preflight

```bash
./scripts/00-preflight.sh          # make preflight
./scripts/00-preflight.sh --no-cluster   # before a kind cluster exists
```

Checks the tools, the Docker daemon, cluster access, whether the cluster is kind (→ image
loading) or needs `ZL_REGISTRY`, the default StorageClass, and the node count.

**Expected:** `✔ preflight passed`.

### Step 1 — (Optional) create a kind cluster

```bash
./scripts/01-create-kind-cluster.sh    # make kind
```

Creates `zoomlens` (1 control-plane + 2 workers) from
[`deploy/kind/cluster.yaml`](../deploy/kind/cluster.yaml) and switches kubectl to
`kind-zoomlens`. Skip this with an existing cluster.

**Check:** `kubectl get nodes` shows 3 `Ready` nodes.

### Step 2 — Build and publish images

```bash
./scripts/02-build-images.sh          # make images
./scripts/02-build-images.sh --no-publish   # build only
```

| Image | Dockerfile | Contents |
|---|---|---|
| `zoomlens-operator:<tag>` | `build/go.Dockerfile` (`PKG=./cmd/zoomlens`) | distroless, non-root |
| `zoomlens-collector:<tag>` | `build/collector.Dockerfile` | official Supervisor image + `/otelcol-contrib` from the official contrib image |

Then either `kind load` (kind) or `docker push` (`ZL_REGISTRY`). On kind it also pre-loads
the eBPF exporter image so the first L2 escalation does not wait for a pull.

**Manual equivalent:**

```bash
docker build -f build/go.Dockerfile --build-arg PKG=./cmd/zoomlens -t zoomlens-operator:dev .
docker build -f build/collector.Dockerfile --build-arg OTEL_VERSION=0.161.0 -t zoomlens-collector:dev .
kind load docker-image zoomlens-operator:dev zoomlens-collector:dev --name zoomlens
```

### Step 3 — Monitoring stack

```bash
./scripts/03-install-monitoring.sh    # make monitoring
```

Installs kube-prometheus-stack with [`deploy/monitoring/values.yaml`](../deploy/monitoring/values.yaml),
waits for Prometheus and Alertmanager, applies the root alert rules and the dashboard.

**Check:**

```bash
kubectl -n monitoring get pods              # prometheus-*, alertmanager-*, *-grafana-*, node-exporter, kube-state-metrics
kubectl -n monitoring get prometheusrule zoomlens-demo
```

### Step 4 — Zoom-Lens operator and collectors

```bash
./scripts/04-deploy-zoomlens.sh       # make zoomlens
```

1. Namespaces `zoomlens` and `zoomlens-demo`.
2. Operator: Deployment + Service `zoomlens` (ports `8080` webhook/metrics/debug, `4320` OpAMP),
   RBAC (read pods/nodes; create/delete pods in `zoomlens` only), ladder ConfigMap.
3. Collectors: DaemonSet `zoomlens-collector` on every node (tolerates all taints), Supervisor
   config + baseline config ConfigMap, RBAC for Kubernetes SD and kubelet metrics.

**Check:**

```bash
kubectl -n zoomlens get pods -o wide          # 1 operator + 1 collector per node, all Running
kubectl -n zoomlens logs deploy/zoomlens-operator | grep -E "agent identified|pushed remote config"
```

You should see one `agent identified … node=<node>` per node, then after the startup grace
(90 s) one `pushed remote config` per node: the no-op overlay (baseline only).

### Step 5 — Demo workload (optional)

```bash
./demo/scripts/deploy-demo.sh           # make demo
```

Deploys the demo app, load generator and noisy-neighbour tooling. It is not part of
Zoom-Lens itself: see [`demo/docs/DEMO.md`](../demo/docs/DEMO.md). `deploy-all.sh` runs it
as its last step.

### Step 6 — Verify the steady state

```bash
./scripts/06-verify.sh                # make verify
```

Checks, with retries:

- every collector is connected over OpAMP and reports the current config as applied
- baseline metrics exist in Prometheus: `base-cadvisor`, `base-node-exporter`, `base-app`,
  `hostmetrics`, and the operator's own `zoomlens_*` metrics
- **no** overlay (`zl-*`) series exist yet
- the Zoom-Lens alert rules are loaded

**Expected:** `✔ steady state verified`.

---

## 6. Running the demo scenarios

Moved to [`demo/docs/DEMO.md`](../demo/docs/DEMO.md) (CPU and disk noisy-neighbour
scenarios, expected timings, demo troubleshooting).

---

## 7. Observing Zoom-Lens

| What | How |
|---|---|
| Live summary | `./scripts/status.sh` / `--watch` / `--raw` |
| Full operator state (incidents, per-node overlays, eBPF pods, OpAMP agents) | `kubectl -n zoomlens port-forward svc/zoomlens 8080` then `curl localhost:8080/debug/state` |
| Operator metrics | `curl localhost:8080/metrics \| grep ^zoomlens_` |
| Operator logs | `kubectl -n zoomlens logs deploy/zoomlens-operator -f` |
| A collector's Supervisor logs | `kubectl -n zoomlens get pods -o wide` to find the pod on a node, then `kubectl -n zoomlens logs <collector-pod> -f` |
| Effective collector config on a node | `kubectl -n zoomlens exec <collector-pod> -- cat /var/lib/otelcol/supervisor/effective.yaml` (the collector image is Alpine-based) |
| Prometheus | `make prometheus` → http://localhost:9090 |

Useful PromQL:

```promql
count by (job) ({job=~"base-.*|zl-.*|hostmetrics"})     # series per collection job
count_over_time(up{job=~"zl-.*"}[1m])                   # scrapes/min per overlay target (6 = 10s, 12 = 5s)
max by (incident, alertname) (zoomlens_incident_level)  # ladder level
zoomlens_remote_config_status                          # APPLIED / APPLYING / FAILED per node
sum(prometheus_tsdb_head_series)                        # used by the series budget
```

---

## 8. Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `00-preflight`: *not a kind cluster: set ZL_REGISTRY* | images need a registry | set `ZL_REGISTRY` and `docker login` |
| Image build fails downloading Go modules | network needs a proxy | `ZL_GOPROXY=https://your-proxy,direct` |
| Pods `ErrImagePull` / `ImagePullBackOff` for `zoomlens-*` | images not loaded/pushed, or wrong arch | re-run step 2; set `ZL_PLATFORM=linux/amd64` for amd64 nodes built on arm64 |
| Collector logs *could not get bootstrap info from the Collector* | collector binary slow to start | raise `agent.bootstrap_timeout` in `deploy/collector/supervisor.yaml`, re-run step 4 |
| Verify: *collectors connected* fails | operator Service unreachable or not ready | `kubectl -n zoomlens logs deploy/zoomlens-operator`; check `svc/zoomlens` endpoints |
| Verify: `base-cadvisor` has no data | kubelet auth/TLS or RBAC | collector logs; check the `zoomlens-collector` ClusterRole (`nodes/metrics`, `nodes/proxy`) |
| Verify: `hostmetrics` has no data | hostPath `/` blocked by policy | allow hostPath / privileged in namespace `zoomlens` |
| Remote config status `FAILED` | an overlay made the collector fail; the Supervisor rolled back | `configError` in `/debug/state` and collector logs; baseline keeps running |
| Incident stuck with *held by series budget* | Prometheus head series above `ZOOMLENS_MAX_HEAD_SERIES` or Prometheus unreachable | raise the limit in `deploy/operator/operator.yaml` or fix Prometheus |
| eBPF pod `Error` / `CrashLoopBackOff` | kernel < 5.15, no BTF, or privileged pods blocked | `kubectl -n zoomlens logs <zl-ebpf-…>`; the rest of the demo still works |
| Metric names differ in Prometheus | remote-write name translation | query `{job="base-cadvisor"}` to see stored names; adjust rules/trees |

---

## 9. Teardown

```bash
./scripts/teardown.sh                    # demo + Zoom-Lens + monitoring   (make teardown)
./scripts/teardown.sh --keep-monitoring  # keep kube-prometheus-stack
./scripts/teardown.sh --delete-kind      # delete the whole kind cluster   (make teardown-kind)
```

---

## Sources

**Checked while building this implementation:**
- OpAMP Supervisor (config keys, merge order of `config_files` and remote config, rollback, apply timeout, empty-config behaviour): https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/cmd/opampsupervisor
- Official Supervisor image Dockerfile (binary only, no collector): https://github.com/open-telemetry/opentelemetry-collector-releases/blob/main/cmd/opampsupervisor/Dockerfile
- opamp-go server API (v0.25.0): https://github.com/open-telemetry/opamp-go
- prometheusremotewrite exporter options: https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/prometheusremotewriteexporter
- hostmetrics receiver: https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/hostmetricsreceiver
- kube-prometheus-stack chart values (91.8.2): https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack
- Cloudflare ebpf_exporter (flags, kernel requirements, examples in the image): https://github.com/cloudflare/ebpf_exporter

**Project references:**
- kind: https://kind.sigs.k8s.io/
- Prometheus configuration (Kubernetes SD, relabelling): https://prometheus.io/docs/prometheus/latest/configuration/configuration/
- Alertmanager webhook receiver: https://prometheus.io/docs/alerting/latest/configuration/#webhook_config
- Pod Security Standards: https://kubernetes.io/docs/concepts/security/pod-security-standards/
