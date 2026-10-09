# Zoom-Lens: Design One-Pager

| | |
|---|---|
| **Authors** | Agila Govindaraju, Prathibha Ramachandra |
| **Status** | Proof of concept. Operator, knowledge layer and demo are built and were run end to end on a single-node cluster. |
| **Scope** | Metrics only, across the whole Kubernetes environment: app pods, nodes/OS, kernel (eBPF) |

## 1. Problem

Monitoring collects everything at a fixed cadence, or collects less using rules fixed in advance.
The first is expensive and noisy. The second leaves you blind when an incident starts. Today's
actuators (OpAMP, Prometheus scrape config, eBPF tools) can change collection at runtime, but a
human still has to decide **where** and **what** to collect.

## 2. Goals / non-goals

| Goals | Non-goals |
|---|---|
| Keep steady-state series count and collection cost low | Traces and logs; tail or retroactive sampling |
| Automatically add detail around the affected entities when a root alert fires | Root-cause analysis. We provide the data; humans or tools diagnose |
| Deepen collection while the alert persists, and revert automatically afterwards | Replacing Prometheus, OTel or eBPF tooling |
| Use only existing open-source CNCF components as actuators | Recovering data from before the alert fired |

## 3. Design

**Two graphs + one alert.**
- **Metric tree** (per entity type): *root* metrics are always on. *Children* are finer metrics
  collected on escalation. The deepest children are eBPF programs.
- **Dependency graph**: built live from Kubernetes informers (Pod → Node → kernel, Pod → PVC →
  device, Pod → CoreDNS, co-located pods), plus annotations for app-to-app callers.

**Control loop** (operator, reconcile every 15s):

```
Alertmanager webhook ─▶ Analyzer ─▶ Planner ─▶ Renderer ─▶ Actuators
 (entity id)            DOWN tree   dwell ladder  per-node   A: OpAMP → collector on that node
                        OUT graph   budget check  base +     B: create/delete ebpf_exporter Pod
                                                  incident jobs
```

**Dwell ladder** (level derived from `now − alert.startsAt`):

| L0 steady | L1 @ 0 min | L2 @ 5 min | L3 @ 10 min | Resolved |
|---|---|---|---|---|
| roots, 30s | depth 1, 1 hop, 10s | depth 2, 5s, eBPF Pod on the node | 2 hops (co-located pods) | cool-down for `clearFor`, then revert to L0 |

## 4. Key decisions

1. **OTel Collectors do all the scraping.** Prometheus only stores data and evaluates rules. OpAMP
   can only change collection if the scraper is an OpAMP-managed agent.
2. **One collector per node (DaemonSet + OpAMP Supervisor).** Each node has its own OpAMP identity,
   so changes are per node and the rest of the cluster is untouched.
3. **Incident jobs contain children only.** Roots keep flowing from the base config, so there are
   no duplicate series. Overlapping incidents are merged per node.
4. **eBPF as a Pod pinned to one node.** `ebpf_exporter` can't load programs at runtime. Creating
   the Pod loads them and deleting it unloads them, so the saving happens at the source.
5. **Revert by rebuild.** Each node's config is recomputed from all active incidents. Removing an
   incident just drops its jobs, so there's no separate delete path.

## 5. Guardrails and failure modes

| Risk | Mitigation |
|---|---|
| Expansion storm during a large outage | Global series budget (fail closed); depth/hop limits; merged scopes |
| Flapping alerts | Cool-down keeps the current level; a re-fire resumes at that level |
| Operator crashes with eBPF running | `activeDeadlineSeconds` on eBPF Pods |
| Collector restart on new config | Supervisor rolls back to the last good config; a short gap of a few seconds is expected |
| No pre-alert history | Roots are chosen to trip alerts reliably. Detail starts at L1 |

## 6. Components

Kubernetes · Prometheus + Alertmanager (kube-prometheus-stack) · OpenTelemetry Collector
(`otelcol-contrib`) · OpAMP (`opamp-go`, Supervisor) · Cloudflare `ebpf_exporter` · `client-go` ·
Grafana. **New:** the Zoom-Lens operator (Go).

## 7. Success criteria

- Series count in steady state vs. incident vs. collect-everything, measured on the reference cluster.
- Child series appear within ~20s of the alert. eBPF runs only on the affected node, only from L2.
- Collection returns to L0 after the alert clears, with no leftover jobs or Pods.

## 8. Open items

Load the curated knowledge ([knowledge/](../knowledge/README.md): `MetricTreePack`, `MetricRelations`
and `EntityGraph` resources) in the operator, add sources beyond node-exporter/cAdvisor/eBPF, and
classify custom metrics · a full run on real
VMs · a one-command installer · alternative actuators (Prometheus Operator `ScrapeConfig`, Tetragon).
