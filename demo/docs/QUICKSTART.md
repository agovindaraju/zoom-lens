# Local Runbook — Rancher Desktop (verified)

Every command below was **actually run** on 2026-09-30 against Rancher Desktop and worked.
Timings are the measured ones. Run from the repository root.

| Environment | Value |
|---|---|
| Cluster | Rancher Desktop, k3s `v1.32.5+k3s1`, 1 node `lima-rancher-desktop` |
| Runtime | `docker://27.3.1` (moby) → built images are used directly, no registry |
| Node | 8 vCPU, ~6 GB, kernel `6.6.93`, BTF present (`/sys/kernel/btf/vmlinux`) |
| StorageClass | `local-path (default)` |

---

## 1. Deploy (about 15 min on the first run)

| # | Command | Took | Result |
|---|---|---|---|
| 0 | `kubectl config current-context` | — | `rancher-desktop` |
| 1 | `./scripts/00-preflight.sh` | 5 s | `✔ cluster uses this Docker daemon as its runtime … built images are used directly` |
| 2 | `./scripts/02-build-images.sh` | ~2 min | 3 images built, eBPF image pre-pulled |
| 3 | `./scripts/03-install-monitoring.sh` | ~10 min (image pulls) | 6 pods Running in `monitoring`, rule + dashboard applied |
| 4 | `./scripts/04-deploy-zoomlens.sh` | ~40 s | operator + 1 collector Running |
| 5 | `./demo/scripts/deploy-demo.sh` | ~20 s | `demo-app`, `demo-load` Running (single node → demo node = `lima-rancher-desktop`) |
| 6 | `./scripts/06-verify.sh` | ~1 min | `✔ steady state verified` |

Or all at once: `./scripts/deploy-all.sh`.

**Check after deploy:**

```bash
kubectl -n monitoring get pods
kubectl -n zoomlens get pods -o wide
kubectl -n zoomlens-demo get pods -o wide
kubectl -n zoomlens logs deploy/zoomlens-operator | grep -E 'agent identified|remote config status'
```

Expected operator log lines:

```
"msg":"agent identified","component":"opamp",...,"node":"lima-rancher-desktop","host":"zoomlens-collector-k59gj"
"msg":"agent remote config status",...,"status":"RemoteConfigStatuses_APPLIED"
```

---

## 2. Baseline (L0) measurements

```bash
./scripts/promq.sh \
  'histogram_quantile(0.99, sum by (le, route) (rate(http_server_request_duration_seconds_bucket{namespace="zoomlens-demo"}[1m])))' \
  'sum by (node) (rate(node_disk_io_time_seconds_total[1m]))' \
  'sum(prometheus_tsdb_head_series)' \
  'count by (job) ({job=~"base-.*|hostmetrics|zl-.*"})'
```

Measured at 18:00:17:

| Metric | Value |
|---|---|
| App p99 `/work/cpu` / `/work/disk` | 4.9 ms / 6.2 ms |
| Node disk busy | 0.5 % |
| Prometheus head series | 1294 |
| Baseline series | `base-node-exporter` 97, `base-cadvisor` 36, `base-app` 33, `hostmetrics` 9 |
| Overlay series (`zl-*`) | **none** |

---

## 3. Scenario A (CPU noisy neighbour): proof of L1

```bash
./demo/scripts/scenario.sh cpu start
```

### 3.1 Timeline (measured)

| Time | Event | Command that shows it |
|---|---|---|
| 18:00:27 | Noisy neighbour started (16 workers, sized to 2 × 8 CPUs) | `kubectl -n zoomlens-demo logs job/noisy-cpu` → `"cpu stress started","workers":16` |
| 18:02:06 | Node CPU saturated (8.0 of 8 cores busy); app p99 5 → **33 ms** | queries in 3.2 |
| 18:02:44 | Alert threshold calibrated to 20 ms (`demo/monitoring/alert-rules.yaml`) | `kubectl apply -f demo/monitoring/alert-rules.yaml` |
| 18:03:13 | Alert **pending** (rule reloaded in ~30 s) | `ALERTS{…}` query |
| 18:04:19 | Alert **firing** → webhook → **incident opened** → **L1** → **OpAMP push** (same second) | operator log |
| 18:04:29 | Collector reports **APPLIED** (10 s = `config_apply_timeout`) | operator log |
| 18:04:59 | `zl-cadvisor`, `zl-node-exporter` series in Prometheus, 10 s cadence | queries in 3.4 |
| 18:04:49 | Alert resolved early (rate-window gap during the collector restart) → cool-down | operator log `incident resolved, cool-down started` |
| 18:06:48 | Scenario stopped (still at L1) | `kubectl -n zoomlens-demo delete job noisy-cpu` |
| 18:08:02 | Incident closed → no-op config pushed (revert) | operator log `incident closed, reverting its collection` |
| 18:08:12 | Collector APPLIED the baseline; `zl-*` last samples 18:07:54 / 18:08:01 | query in 3.5 |

### 3.2 Is the fault real? (app latency and CPU)

```bash
./scripts/promq.sh \
  'histogram_quantile(0.99, sum by (le, namespace, pod, node) (rate(http_server_request_duration_seconds_bucket{namespace="zoomlens-demo"}[1m])))' \
  'sum(rate(node_cpu_seconds_total{mode!="idle"}[1m]))' \
  'sum by (pod) (rate(container_cpu_usage_seconds_total{namespace="zoomlens-demo"}[1m]))'
```

Measured output:

```
0.0333   {'namespace': 'zoomlens-demo', 'node': 'lima-rancher-desktop', 'pod': 'demo-app-8584bfb4fd-x25tw'}
8.0053   {}
0.0261   {'pod': 'demo-app-8584bfb4fd-x25tw'}
4.0507   {'pod': 'noisy-cpu-xtc4f'}
```

### 3.3 Is the alert firing? Did the operator get it and push over OpAMP?

**Alert state** (repeat every ~15 s: none → pending → firing):

```bash
./scripts/promq.sh 'ALERTS{alertname="ZoomLensAppLatencyHigh"}'
```

**Operator (webhook → incident → L1 → OpAMP):**

```bash
kubectl -n zoomlens logs deploy/zoomlens-operator --since=10m \
  | grep -E 'incident|pushed|remote config status|ebpf'
```

Measured output:

```
18:04:19.759 "msg":"incident opened","incident":"3172f3bd","alert":"ZoomLensAppLatencyHigh","root":"http_server_request_duration_seconds","entity":"Pod/zoomlens-demo/demo-app-8584bfb4fd-x25tw"
18:04:19.767 "msg":"incident escalated","incident":"3172f3bd","from":0,"to":1,"depth":1,"hops":1,"interval":10000000000,"ebpf":false,"nodes":["lima-rancher-desktop"]
18:04:19.768 "msg":"pushed remote config","component":"opamp","node":"lima-rancher-desktop","hash":"0ece55edf782"
18:04:19.794 "msg":"agent remote config status","status":"RemoteConfigStatuses_APPLYING"
18:04:29.799 "msg":"agent remote config status","status":"RemoteConfigStatuses_APPLIED"
```

(Log timestamps are UTC in the raw output; shown here in local time, IST.)

**Operator state** (incidents, per-node overlay, agents):

```bash
kubectl -n zoomlens port-forward svc/zoomlens 8080 &
curl -s localhost:8080/debug/state
./scripts/status.sh          # same, formatted
```

### 3.4 Did the collector really start collecting the child metrics?

```bash
./scripts/promq.sh \
  'count by (job) ({job=~"zl-.*"})' \
  'count by (__name__) ({job="zl-cadvisor"})' \
  'count by (__name__) ({job="zl-node-exporter"})' \
  'count_over_time(up{job=~"zl-.*"}[1m])'
```

Measured output at 18:04:59:

```
== count by (job) ({job=~"zl-.*"})
  12  {'job': 'zl-node-exporter'}
  10  {'job': 'zl-cadvisor'}
== count by (__name__) ({job="zl-cadvisor"})          # one series each: scoped to the demo-app pod only
  1   container_cpu_cfs_throttled_periods_total
  1   container_cpu_cfs_periods_total
  1   container_memory_rss
  1   container_fs_writes_bytes_total
== count by (__name__) ({job="zl-node-exporter"})
  3   node_disk_write_time_seconds_total
  3   node_disk_writes_completed_total
== count_over_time(up{job=~"zl-.*"}[1m])               # climbs to 6/min = 10 s interval
  3   zl-node-exporter
  3   zl-cadvisor
```

**The effective config inside the collector** (base + Zoom-Lens overlay):

```bash
kubectl -n zoomlens exec ds/zoomlens-collector -- cat /var/lib/otelcol/supervisor/effective.yaml | grep -E 'zoomlens|job_name'
```

### 3.5 Stop and revert

```bash
./demo/scripts/scenario.sh cpu stop          # or: kubectl -n zoomlens-demo delete job noisy-cpu
./scripts/promq.sh 'ALERTS{alertname=~"ZoomLens.*"}'           # → (no data) within ~1-2 min
# ~3 min (clearFor) after the resolve:
kubectl -n zoomlens logs deploy/zoomlens-operator --since=10m | grep -E 'resolved|closed|pushed'
# Prove collection stopped (instant count() keeps showing zl-* for ~5 min: lookback delta)
./scripts/promq.sh 'max by (job) (timestamp(up{job=~"zl-.*|base-.*"}))'
kubectl -n zoomlens exec ds/zoomlens-collector -- cat /var/lib/otelcol/supervisor/effective.yaml | grep -c 'metrics/zoomlens'   # → 0
```

Expected operator lines: `incident resolved, cool-down started` → `incident closed, reverting its collection` → `pushed remote config` (the no-op overlay).

---

## 4. Findings on this environment (and fixes made)

| Finding | Fix |
|---|---|
| Scripts only knew kind or a registry | Added detection of a cluster that shares the build Docker daemon (Rancher Desktop moby, Docker Desktop): `scripts/lib.sh` `uses_local_docker` |
| `base-cadvisor` had 0 series: with the Docker runtime (cri-dockerd), cAdvisor container-level series have **empty `container`/`pod` labels**; only pod-level cgroups carry `namespace`/`pod` | `deploy/collector/base-config.yaml`: keep series with a `pod` label, drop only `container="POD"` |
| 4 CPU-stress workers cannot contend on 8 vCPUs | `demo/scripts/scenario.sh` sizes workers to 2 × node CPUs (`ZL_STRESS_CPU_WORKERS` overrides) |
| p99 under contention is ~33 ms, not > 250 ms: Kubernetes CPU weights (app requests 200m, stress 10m) protect the app | Alert threshold calibrated to **20 ms** (4 × baseline) |
| `node_pressure_cpu_waiting_seconds_total` absent in the L1 overlay | This VM's node-exporter exposes no PSI metrics; not a Zoom-Lens issue |

Re-calibrate the threshold on other hardware: measure p99 at baseline and under
`scenario.sh cpu start` with the query in 3.2, then set the rule between the two.

---

## 5. Useful one-liners

```bash
./scripts/status.sh --watch                                    # live incidents / overlays / agents
make grafana                                                   # http://localhost:3000  admin / zoomlens
kubectl -n zoomlens logs deploy/zoomlens-operator -f           # operator
kubectl -n zoomlens logs ds/zoomlens-collector -f              # Supervisor + collector
kubectl -n monitoring port-forward svc/prometheus-operated 9090   # Prometheus UI → Alerts
./scripts/teardown.sh                                          # remove everything
```
