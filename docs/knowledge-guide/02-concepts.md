# 2. Concepts, with examples

This chapter defines every term used by the knowledge graph. Each term links to where it appears
in the files. The glossary at the end repeats the definitions in one line each.

---

## 2.1 Metric vocabulary (Prometheus)

| Term | Meaning | Example |
|---|---|---|
| **Metric name** | The name a target exposes on its `/metrics` endpoint (the *exposition name*). All knowledge files use these names | `node_disk_io_time_seconds_total` |
| **Labels** | Key/value pairs that split one metric into many **series** | `{node="worker-1", device="sda"}` |
| **Series** | One metric name plus one unique label set. Storage cost is proportional to the number of series | `node_disk_io_time_seconds_total{node="worker-1",device="sda"}` |
| **Cardinality** | How many series a metric produces. It depends on its labels (per CPU, per device, per pod, per IP…) | `tcp-retransmit` in eBPF has local IP × peer IP × port: high cardinality |
| **Counter** | Only goes up (resets on restart). Use `rate()` / `increase()` | `*_total` |
| **Gauge** | A value that goes up and down | `node_memory_MemAvailable_bytes` |
| **Histogram** | Buckets of observations. It is exposed as three series families: `<name>_bucket`, `<name>_sum`, `<name>_count` | `etcd_disk_wal_fsync_duration_seconds` |

Histograms are listed in the knowledge by their **base name**. Anything that turns a name into a
scrape filter must add the suffixes. That is what the `keepRegex` field in
`knowledge/generated/catalog/*.json` does: `name(_bucket|_sum|_count)`.

---

## 2.2 Entity vocabulary (Kubernetes)

| Term | Meaning | Example |
|---|---|---|
| **Entity** | One real thing a metric describes | node `worker-1`, pod `shop/checkout-7d9f` |
| **Entity class** | The kind of entity, defined in the `EntityGraph` | `Node`, `Pod`, `Container`, `PersistentVolumeClaim`, `Etcd` |
| **Identity labels** | Labels that identify the entity on every series of a pack | `[node]`, `[namespace, pod, container]` |
| **Join labels** | Labels that join series of the same entity class across sources | `Pod` joins on `[namespace, pod]` |
| **Operator kind** | The value the operator understands in the `zoomlens_entity` alert label. It defaults to the class name; a class may override it | `Container` → `Pod` (a container alert acts on its pod). `CoreDNS` → `Pod`. `Etcd` → `EtcdMember` |

Today the operator acts on two operator kinds only: **`Pod`** and **`Node`**.

---

## 2.3 Source and pack

| Term | Meaning | Example |
|---|---|---|
| **Source** | One metric-producing component and endpoint. Also the id of a scrape template in the operator | `node-exporter`, `cadvisor`, `kubelet`, `kube-state-metrics`, `ebpf` |
| **Scope** | Which collector can scrape the source | `per-node` (every node has one), `cluster` (one endpoint for the whole cluster, e.g. kube-state-metrics), `control-plane` |
| **Name kind** | What the names in a pack are | `metric` (normal), `scraper` (hostmetrics scraper names like `paging`), `ebpf-program` (ebpf_exporter config names like `biolatency`) |
| **MetricTreePack** ("pack") | The metric tree of **one source for one entity class**. Named `<source>-<entity>` | `kubelet-persistent-volume-claim` |

A source can have several packs. `kubelet` has three: `kubelet-node`, `kubelet-pod` and
`kubelet-persistent-volume-claim`. A metric name appears **only once per source**, so two packs of
the same source never both collect the same series.

---

## 2.4 The tree: roots, companions, children, depth

```
L0  node_memory_MemAvailable_bytes            ◀ ROOT: always collected, has an alert rule
      + node_memory_MemTotal_bytes             ◀ COMPANION: always collected, needed by the alert
    ├─ d1 node_pressure_memory_waiting_seconds_total        ◀ CHILD depth 1: collected from L1
    │   └─ d2 node_pressure_memory_stalled_seconds_total    ◀ CHILD depth 2: collected from L2
    ├─ d1 node_vmstat_pgmajfault
    │   ├─ d2 node_vmstat_pgfault
    │   └─ d2 node_vmstat_pgpgin …
    └─ d1 node_memory_Slab_bytes
        └─ d2 node_memory_SUnreclaim_bytes …
```

| Term | Meaning |
|---|---|
| **Root** | A metric that is **always collected** (at L0) and is **alertable**. It is cheap, well understood, and usually used by an upstream mixin alert. An alert on a root starts an expansion of that root's tree |
| **Companion** | A metric that is always collected **together with its root** because the root's alert expression needs it. Typical companions are denominators (`MemTotal` for `MemAvailable`, a CPU limit for CPU usage) and guards (`node_filesystem_readonly`). Companions are never expanded |
| **Child** | A finer metric that is **dormant** until a related root fires. Children sit under a *parent* (a root or another child) |
| **Depth** | Distance from the root. Depth 1 = direct child. The trees stop at depth 3 |
| **`children` map** | `parent → [children]`. The tree is stored as this flat map, not as nested YAML |
| **Always-on** | Roots + companions of a source. The collector baseline must scrape all of them |

How depth was assigned:
- **Depth 1:** the cheapest, most diagnostic breakdown of the root. This usually follows the USE
  method: utilisation → saturation (for example PSI pressure) → errors.
- **Depth 2:** finer detail, often per-CPU, per-device or per-operation.
- **Depth 3:** rare context, configuration, or expensive detail.

---

## 2.5 The dwell ladder: levels, depth, hops

The **ladder** decides how far to expand, based on how long the alert has been firing
(`now − alert.startsAt`). It is defined in `deploy/operator/ladder.yaml` (demo timings):

| Level | After | Depth | Hops | Scrape interval | eBPF | Meaning |
|---|---|---|---|---|---|---|
| **L0** | — | — | — | 30 s (baseline) | no | Steady state: roots and companions only |
| **L1** | 0 s | 1 | 1 | 10 s | no | Direct children; the entity's immediate neighbours |
| **L2** | 3 min | 2 | 1 | 5 s | **yes** | Grandchildren; kernel eBPF programs on the affected node |
| **L3** | 6 min | 3 | 2 | 5 s | yes | Deepest detail; **two hops** (Pod → Node → co-located Pods) |

- **Depth** is how far DOWN the metric tree to go.
- **Hops** is how far OUT the entity graph to go.
- **`clearFor`** (3 min) is how long an alert must stay resolved before collection reverts. During
  this **cool-down** the incident keeps its level, and if it fires again it resumes at that level.
- **Startup grace** (90 s): after an operator restart, nothing is reverted until Alertmanager has
  had time to re-send the alerts that are still firing.
- The design defaults are L2 at 5 min and L3 at 10 min. The shorter timings are for live demos.

---

## 2.6 Domains and domain gating

Every root has a **domain**, one of eight:

`cpu` · `memory` · `storage` · `network` · `dns` · `lifecycle` · `control-plane` · `scheduling`

Every entity-graph edge lists the domains it is relevant for. An edge is followed **only if the
firing root's domain is in the edge's list**. This keeps expansions small and on-topic:

| Edge | Domains | So… |
|---|---|---|
| `Pod -runs-on-> Node` | cpu, memory, storage, network, lifecycle | A CPU alert on a pod looks at its node |
| `Pod -mounts-> PersistentVolumeClaim` | storage, lifecycle | A CPU alert never pulls in volume metrics; a disk alert does |
| `APIServer -stores-in-> Etcd` | control-plane | API latency alerts look at etcd; nothing else does |
| `Pod -resolves-via-> CoreDNS` | dns, network | Only DNS and network problems follow a pod to CoreDNS |

---

## 2.7 Cross-tree links: siblings, explains, eBPF

These are in `MetricRelations`. They let one root reach metrics outside its own pack.

| Link | Direction | Meaning | Example |
|---|---|---|---|
| **Sibling group** | symmetric | Roots in **different packs** that report the **same condition**. When one fires, the **depth-1 children of the others** are collected too | `node-memory`: node-exporter `node_memory_MemAvailable_bytes` ↔ hostmetrics `memory` ↔ kubelet `node_memory_working_set_bytes` ↔ kubelet `kubelet_evictions` |
| **Explains** | directed | Specific **child** metrics of **another entity** that are the usual cause of a root, each with the depth at which it is collected. This is a targeted, cheaper alternative to collecting everything about the related entity | cAdvisor `container_cpu_usage_seconds_total` → node-exporter `node_pressure_cpu_waiting_seconds_total` (d1), `node_schedstat_waiting_seconds_total` (d2) |
| **eBPF link** | directed | ebpf_exporter programs to start **on the affected node**, with the depth (2 or 3) from which they run. Only on ladder levels that enable eBPF | cAdvisor `container_fs_writes_bytes_total` → `biolatency` (d2), `ext4dist` / `xfsdist` (d3) |

Why "explains" exists: a container being CPU-throttled is often caused by its **node** being
saturated. The useful node metrics are CPU pressure and scheduler wait time, not all 75 depth-1
node children. The explains link names exactly those.

---

## 2.8 Collection vocabulary (operator and collector)

| Term | Meaning |
|---|---|
| **Baseline** | The collector config every node starts with: roots and companions only, every 30 s, from this node's targets only. File: `deploy/collector/base-config.yaml`. Jobs are named `base-*` |
| **Overlay** | What the operator adds on top of the baseline for one node during incidents. It is pushed over OpAMP. Jobs are named `zl-*` (`zl-node-exporter`, `zl-cadvisor`, `zl-app`, `zl-ebpf`), plus extra hostmetrics scrapers |
| **Children-only rule** | Overlays never contain roots or companions, because the baseline already collects them. This avoids duplicate series |
| **Scope (plan)** | The per-node result of planning one incident at one level: which metric names per source, which pods, which host scrapers and eBPF programs |
| **Merge** | Several incidents on the same node are merged: the union of metrics, pods, scrapers and programs; the shortest interval wins |
| **Series budget** | Before each escalation the operator checks `sum(prometheus_tsdb_head_series)` against a limit (default 200,000). If it is over, or the check fails, the escalation is held (fail closed) |

---

## 2.9 Alert rule vocabulary

| Term | Meaning |
|---|---|
| **Root alert rule** | A standard Prometheus alerting rule whose expression is about one root. It carries the labels below |
| `zoomlens_root` | The root to expand, which must be a root in the pack of `zoomlens_source` |
| `zoomlens_source` | The pack source. It disambiguates roots that exist in several sources, such as `rest_client_requests_total` |
| `zoomlens_entity` | The operator kind (`Node`, `Pod`, …) |
| `severity` | `info`, `warning` or `critical` |
| **Identity labels in `by (…)`** | The expression keeps the entity's identity labels (for example `node`, or `namespace, pod`) so they reach the alert and the operator knows *which* entity |
| **`for`** | How long the condition must hold before the alert fires. The curated rules use the upstream values (5 m–1 h). Step 03 of the deploy scripts can shorten them for demos |

---

## 2.10 Glossary (one line each)

| Term | One-line definition |
|---|---|
| Always-on | Roots and companions: what the baseline must collect |
| Baseline | L0 collector config, `base-*` jobs, roots and companions, 30 s |
| Cardinality | Number of series a metric produces |
| Child | Dormant metric below a root, collected from level = its depth |
| Companion | Always-on metric that a root's alert needs; never expanded |
| Cool-down | Time after an alert resolves before collection reverts (`clearFor`) |
| Depth | Distance from the root in a metric tree (1..3) |
| DOWN | Expanding a root into finer metrics of the same tree |
| Domain | Problem area of a root; gates entity-graph edges |
| eBPF link | Kernel program to start on the affected node from depth 2 or 3 |
| Entity class | Kind of entity in the entity graph (Node, Pod, …) |
| EntityGraph | Resource listing entity classes and domain-gated edges |
| Explains | Targeted cross-entity child metrics that usually cause a root |
| Hop | One edge in the entity graph |
| Identity labels | Labels identifying the entity on a pack's series |
| Level (L0–L3) | Rung of the dwell ladder: depth, hops, interval, eBPF |
| MetricRelations | Resource holding siblings, explains and eBPF links |
| MetricTreePack | Resource: the metric tree of one source for one entity class |
| Name kind | What pack names are: metric, scraper, ebpf-program |
| Operator kind | `zoomlens_entity` value for an entity class (Container → Pod) |
| OUT | Expanding to related entities through the entity graph |
| Overlay | Operator-pushed extra collection per node, `zl-*` jobs |
| Root | Always-collected, alertable metric that starts an expansion |
| Scope | Which collector scrapes a source (per-node / cluster / control-plane) |
| Sibling group | Roots in different packs reporting the same condition |
| Source | A metric-producing component and endpoint; a scrape template id |
