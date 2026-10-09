# 9. At runtime: how the operator uses the knowledge

This chapter follows one alert through the operator code, shows exactly what the knowledge
contributes at each step, and separates **what works today** from **what is designed but not
implemented yet**. File references are to the repository at commit `c1e696d`.

---

## 9.1 The moving parts

```
 Prometheus ──(rule fires)──▶ Alertmanager ──webhook POST /alerts──▶ ┌──────────────────── operator ────────────────────┐
                                                                    │ internal/alerts     parse → Signal              │
 Kubernetes API ◀──list every 1 min── crd.APILoader                 │ internal/incident   incident + dwell ladder     │
   MetricTreePack                        │                          │ internal/plan       Scope per node              │
   MetricRelations                       ▼                          │   ├ Tree.Expand      ◀── crd.Snapshot (DOWN)    │
   EntityGraph                     crd.Store (atomic)               │   └ Graph.RelatedForRoot ◀── (OUT gate)         │
                                                                    │ internal/render     OTel collector YAML          │
 deploy/operator/ladder.yaml ──── policy (ladder) ───────────────▶ │ internal/opamp      push per node                │
                                                                    │ internal/ebpf       ebpf_exporter pod per node   │
                                                                    └───────────────┬───────────────┬──────────────────┘
                                                                           OpAMP     │               │ create/delete Pod
                                                                                     ▼               ▼
                                                       Supervisor + otelcol on node X      zl-ebpf-… on node X
```

---

## 9.2 Step 0: loading the knowledge

Configuration (`internal/config`, set in `deploy/operator/operator.yaml`):

| Flag / env | Default in code | Set in the deployment | Meaning |
|---|---|---|---|
| `--knowledge-refresh` / `ZOOMLENS_KNOWLEDGE_REFRESH` | `1m` | — | How often the resources are re-listed |
| `--ladder-file` / `ZOOMLENS_LADDER_FILE` | `/etc/zoomlens/ladder.yaml` | same | The dwell ladder (policy) |

At start-up (`cmd/zoomlens/main.go`):

1. The operator reads the policy from the knowledge file.
2. With `crd`, `crd.APILoader.Load` lists `metrictreepacks`, `metricrelations` and `entitygraphs`
   with the dynamic client, converts them into `api/v1alpha1` types, and calls `crd.Build`. **If
   this fails, the operator does not start.** Apply `deploy/crds` and `knowledge/` first.
3. The resulting **`Snapshot`** goes into a `crd.Store`, an atomic pointer.
4. `crd.Run` re-lists every minute. **A failed reload keeps the previous snapshot**, and a change
   is logged as `knowledge reloaded`.

What `Build` indexes (`internal/knowledge/crd/snapshot.go`):

| Index | Key → value | Used for |
|---|---|---|
| `packs` | (source, entity) → pack spec | Children lookups. Two packs with the same source and entity are an error |
| `kindOf` | entity class → operator kind | `Container` → `Pod`, `CoreDNS` → `Pod`, `Etcd` → `EtcdMember` |
| `rootsOf` | (root name, operator kind) → packs | Finding which pack(s) an alert's root belongs to |
| `domainOf` | root name → domain | Gating the OUT walk (the first pack that declares the root wins) |
| `alwaysOn` | source → roots + companions | Never returning something the baseline already collects |
| `siblings` | root ref → other members of its groups | Sibling expansion |
| `explains`, `ebpf` | root ref → targets / programs | Explains and eBPF expansion |
| `edges` | EntityGraph edges between operator kinds | OUT gating |
| `supported` | the renderer's sources | Only `cadvisor`, `node-exporter`, `app`, `hostmetrics` and `ebpf` are returned |

The start-up log shows the stats:

```
"msg":"knowledge loaded","backend":"crd","stats":{"packs":38,"roots":112,"supportedRoots":25,"relations":2,"entityEdges":38}
```

- **38 packs:** the 37 curated packs plus the demo pack.
- **112 roots:** 111 plus the demo root.
- **25 supported roots:** node-exporter 13, cAdvisor 7, hostmetrics 4 and the demo app 1.

---

## 9.3 Step 1: an alert arrives

`internal/alerts` parses the Alertmanager webhook (payload version 4). An alert becomes a
**Signal** only if it has `zoomlens_root`. The entity comes from `zoomlens_entity`, or is inferred
from the `pod` and `node` labels:

| `zoomlens_entity` | Needs | Result |
|---|---|---|
| `Pod` | `pod`, `namespace` (`node` optional) | Pod entity |
| `Node` | `node` | Node entity |
| anything else | — | **skipped** (counted in `zoomlens_alerts_skipped_total`) |

---

## 9.4 Step 2: the incident and the ladder

`internal/incident/manager.go`:

- **Opening.** One **incident per alert fingerprint**. `StartsAt` is the alert's `startsAt`, so
  dwell time survives Alertmanager re-sends and operator restarts. Because incidents are per
  fingerprint, 16 alerts with the same root on the same node become 16 incidents (finding F-01).
- **Escalating.** Every 15 s, and on every webhook, the target level is
  `policy.LevelFor(now − StartsAt)`. The ladder **only moves up**. Before each step the series
  budget is checked; if it fails, the step is held.
- **Planning happens at escalation time.** The incident's scope is computed when it reaches a new
  level, and is not recomputed until the next level. A knowledge refresh affects the next
  escalation, not the current scope.
- **Resolving and closing.** On `resolved`, the cool-down starts and the level is kept. If the
  alert fires again during the cool-down, the incident continues at its level. After `clearFor`,
  the incident is closed and its collection disappears from the merged overlay.
- **Startup grace** (90 s). After a restart, nothing is reverted until Alertmanager has had time to
  re-send the alerts that are still firing.

---

## 9.5 Step 3: DOWN with `Snapshot.Expand(root, kind, depth)`

This is the heart of the knowledge at runtime. In pseudocode (the real code is ~50 lines in
`snapshot.go`):

```
result = {}
for each pack P that has `root` as a root for operator kind `kind`:          # rootsOf
    # 1. the pack's own tree, level by level, up to `depth`
    frontier = [root]
    for d in 1..depth:
        next = []
        for m in frontier: for c in P.children[m]: add(P.source, c); next += c
        frontier = next
    # 2. siblings: depth-1 children of every other member of the root's groups
    for member in siblings[P.source, P.entity, root]:
        for c in pack(member).children[member.metric]: add(member.source, c)
    # 3. explains targets with depth <= depth
    for t in explains[...]: if t.depth <= depth: add(t.source, t.metric)
    # 4. eBPF programs with depth <= depth
    for p in ebpf[...]:     if p.depth <= depth: add("ebpf", p.name)

add(source, name):  keep only if source is supported by the renderer
                    and name is NOT a root or companion of that source (always-on)
```

What this means in practice:
- **Siblings add only depth 1**, whatever the level.
- **Explains and eBPF entries are cumulative**: at depth 3 you get the entries for depths 1, 2
  and 3.
- **Unsupported sources are silently dropped.** kubelet children of a kubelet root return nothing
  today, but that root's node-exporter siblings still return their children.
- **Roots and companions are never returned.** A test guards this:
  `TestExpandNeverReturnsAlwaysOnMetrics`.
- **`zoomlens_source` is not used.** If the same root name exists in several packs with the same
  operator kind, all of them are expanded.
- eBPF programs are only **used** if the level has `enableEBPF` (`plan.newOverlay`).

---

## 9.6 Step 4: OUT with `Graph.RelatedForRoot(root, entity, hops)`

`internal/knowledge/crd/store.go`:

1. Look up the root's domain `D`. If no pack declares the root (for example an application root
   without a pack), **no gating** is applied.
2. Walk the fixed path **Pod → Node → Pod** (or **Node → Pod** from a node). Count how many of the
   requested hops have an EntityGraph edge between those operator kinds that admits `D`.
3. Call the inner graph (the Kubernetes-API walk in `internal/knowledge/topology`) with that many hops:
   - hop 1 from a Pod is its Node;
   - hop 2 is the pods on that node;
   - hop 1 from a Node is the pods on it.

| Root (domain) | From | Requested hops | Allowed | Why |
|---|---|---|---|---|
| `container_cpu_usage_seconds_total` (cpu) | Pod | 2 | **2** | `runs-on` admits cpu, both ways |
| `node_disk_io_time_seconds_total` (storage) | Node | 2 | **1** | From a Node, the only step is Node → Pods |
| `node_timex_offset_seconds` (lifecycle) | Node | 1 | **1** | `runs-on` admits lifecycle |
| `kubelet_running_pods` (scheduling) | Node | 1 | **0** | No Pod ↔ Node edge admits scheduling, so only the node itself |

These cases are covered by `TestGraphDomainGating`, except the last one, which follows from the
same rule.

---

## 9.7 Step 5: from scope to collector config

`internal/plan` builds a **Scope** (one overlay per node):
- For every related entity with a node, it adds an overlay for that node with **the same list of
  metric refs** (from Expand), the level's interval, and the incident id.
- For every related **Pod**, it adds the pod to the overlay's pod list. Pod-scoped sources are
  restricted to those pods.
- Refs are split by source: `hostmetrics` refs become **host scrapers**, `ebpf` refs become
  **eBPF programs** (only if the level enables eBPF), and the rest become **metric names per
  source**.
- Empty overlays are dropped.

All incidents' scopes are then **merged** per node: union of everything, shortest interval wins.

`internal/render` turns one node's overlay into OpenTelemetry Collector YAML:

| Source | Job | How it is scoped | Filter |
|---|---|---|---|
| `cadvisor` | `zl-cadvisor` | `kubernetes_sd` role `node`, selector `metadata.name=<node>`, `https://…/metrics/cadvisor` | `__name__` keep regex; `namespace;pod` keep regex for the related pods |
| `node-exporter` | `zl-node-exporter` | pod SD on the node, node-exporter port | `__name__` keep regex |
| `app` | `zl-app` | pod SD on the node, annotation `zoomlens.dev/scrape: "true"` | `__name__` keep regex; related pods |
| `ebpf` | `zl-ebpf` | pod SD on the node in the `zoomlens` namespace, eBPF port | none (finding F-09: exporter self-metrics are kept too) |
| `hostmetrics` | (receiver) | `hostmetrics` receiver with the extra scrapers, `root_path` | — |

- The keep regex is built from the **exact names** returned by Expand. That is correct for today's
  sources, whose children include no histograms. When histogram children are added (kubelet, API
  server, …), the renderer must add `_bucket|_sum|_count`. `knowledge/generated/catalog/*.json`
  already carries the right `keepRegex` for every name.
- An empty merged overlay renders a **no-op body**, never an empty config.

Then:
- `internal/opamp` pushes each node's body to that node's Supervisor. The Supervisor merges it
  with the local baseline, restarts the collector, and reports `APPLYING` → `APPLIED`.
- `internal/ebpf` ensures one `ebpf_exporter` pod per node with exactly the merged program set,
  and deletes it when no program is needed.

---

## 9.8 What the curated rules can do today

Replaying the operator's logic on the 113 curated rules (supported sources, Pod/Node entities):

| Outcome | Rules | Why |
|---|---|---|
| **Collect extra metrics** | **33** | Pod/Node entity, and Expand returns at least one supported ref (own tree, siblings or explains) |
| Open an incident, but the overlay is **empty** | 25 | Pod/Node entity, but every child is on an unsupported source (kubelet, KSM, kube-proxy, CoreDNS, CNI, containerd) and no sibling or explains target is supported |
| **Skipped** | 55 | `zoomlens_entity` is not Pod or Node (API server, etcd, scheduler, controller-manager, workloads, PVC, …) |

The 33 rules that collect today:

| Source | Rules |
|---|---|
| node-exporter (13 of 14) | all except `ZLNodeFilesystemAlmostOutOfFiles` (its root has no children) |
| cadvisor (6 of 6) | all |
| kube-state-metrics (4) | `ZLKubeNodeNotReady`, `ZLKubeNodePressure`, `ZLKubeNodeReadinessFlapping` (explains → node PSI) and `ZLKubePodRestarting` (explains → cAdvisor `memory.max` events, plus the `container-memory` / `container-oom` siblings) |
| kubelet (4) | `ZLKubeNodeEviction`, `ZLKubeletNodeMemoryNearCapacity` (the `node-memory` sibling → node-exporter and hostmetrics children), `ZLKubeletStorageOperationErrors` (the `node-disk` sibling), `ZLPodProbeFailures` (explains → cAdvisor throttling and pressure) |
| coredns (3) | `ZLCoreDNSLatencyHigh`, `ZLCoreDNSErrorsHigh`, `ZLCoreDNSForwardHealthcheckBroken` (explains and eBPF → node UDP, conntrack, CPU throttling, `udp-drops`) |
| kube-proxy (1) | `ZLKubeProxySyncSlow` (the `node-service-routing` sibling → node conntrack children) |
| cilium, calico (2) | `ZLCiliumDrops`, `ZLCalicoDataplaneFailures` (the `node-network-errors` sibling → node network error children) |

This shows why the relations matter: **14 of the 33** rules are on sources the operator can't
scrape yet: kube-state-metrics 4, kubelet 4, CoreDNS 3, kube-proxy 1, Cilium 1 and Calico 1. They
work only through siblings, explains and eBPF links.

---

## 9.9 Evidence from the live run

From a run on Rancher Desktop (single node, CPU noisy neighbour):

| Knowledge element | What was observed |
|---|---|
| Root rule `ZLNodeCPUHighUsage` | Fired when the node's CPU was ~100 % busy; incident `7da4045d` on `Node//lima-rancher-desktop` |
| Pack `node-exporter-node`, children of `node_cpu_seconds_total` | `zl-node-exporter` collected `node_schedstat_*`, `node_procs_running`, `node_context_switches_total`, `node_intr_total`, `node_forks_total`, `node_cpu_guest_seconds_total` and `node_cooling_device_*` across L1–L3 |
| Sibling group `node-cpu` | The hostmetrics scrapers `load` and `system` were enabled |
| eBPF link of `node_cpu_seconds_total` | L2 added `softirq-latency`; L3 added `timers` and `llcstat`. The pod was recreated at each step |
| Demo pack + demo relations | `demo_inflight_requests`, `demo_disk_bytes_written_total`, `container_memory_rss`, `container_cpu_cfs_throttled_seconds_total`, `paging`, `processes` and `biolatency` |
| Children of `node_timex_offset_seconds` and `node_scrape_collector_success` | Collected during the environment false positives: proof that the CRD trees drove collection |
| Revert | Both incidents closed after `clearFor`; the overlay shrank to what the other incident still needed, then to the no-op body; the eBPF pod was deleted |

Series at L3: 1,277 `zl-*` series (1,145 of them eBPF) over a baseline of about 567 `base-*`
series.

---

## 9.10 Design vs implementation

| Design (knowledge/README, this guide) | Implemented today | Consequence |
|---|---|---|
| OUT: on each reached entity, collect its roots' depth-1 children (or the explains targets for it) | The **same** metric set from Expand goes to every related node. Pods only restrict pod-scoped sources | Node-level children of a pod root are collected on its node; co-located pods get the pod-scoped children at L3 |
| OUT over all 38 edges (PVC, PV, Service, CoreDNS, control plane, workloads) | Only the Pod ↔ Node path, gated by the edges | Storage, DNS, control-plane and workload hops are not traversed yet |
| All 16 sources actuated | 5 sources (`cadvisor`, `node-exporter`, `app`, `hostmetrics`, `ebpf`) | Other sources' children are dropped; KSM needs a cluster-level collector |
| All entity kinds | `Pod` and `Node` | 55 curated rules are skipped |
| `zoomlens_source` disambiguates roots | Not read by the operator | Same-name roots with the same operator kind all expand |
| eBPF budget by `cardinality` | Budget uses total head series only | eBPF can dominate the overlay (F-04) |
| Keep only program metrics from eBPF | `zl-ebpf` keeps everything (F-09) | ~215 extra self-metric series at L3 |
| Histogram children scraped as `_bucket\|_sum\|_count` | Exact-name keep regex | Needed once histogram children are on supported sources |
| Ladder in the resources | Ladder in `deploy/operator/ladder.yaml` | The policy is not a CRD yet |

---

## 9.11 How to watch it happen

```bash
# Knowledge loaded? (stats line)
kubectl -n zoomlens logs deploy/zoomlens-operator | grep -E 'knowledge (loaded|reloaded)'

# Knowledge objects in the cluster
kubectl get mtp; kubectl get mrel; kubectl get eg
kubectl get mtp -l zoomlens.dev/source=node-exporter -o yaml | less

# Alert → incident → level → eBPF
kubectl -n zoomlens logs deploy/zoomlens-operator --since=15m | grep -E 'incident|escalated|ebpf'

# Everything the operator believes right now (incidents, scopes, overlays, agents)
kubectl -n zoomlens port-forward svc/zoomlens 8080:8080 &
curl -s localhost:8080/debug/state | head -c 4000

# Short summary (incidents, per-node overlay, eBPF pods, series counts)
./scripts/status.sh

# What the overlay actually collects
./scripts/promq.sh 'count by (job) ({job=~"zl-.*"})' 'count by (__name__) ({job="zl-node-exporter"})'

# Effective collector config on the node
kubectl -n zoomlens exec ds/zoomlens-collector -- cat /var/lib/otelcol/supervisor/effective.yaml | grep -E 'job_name|scrape_interval'
```
