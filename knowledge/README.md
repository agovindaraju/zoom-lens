# Zoom-Lens knowledge: Kubernetes metric trees and entity graph

This directory is the knowledge Zoom-Lens uses to decide **what to collect and where** when an
alert fires:

- **Metric trees (DOWN):** for each metric source and entity, the few *root* metrics that are
  always collected, and the finer *child* metrics collected level by level while an incident lasts.
- **Relations:** roots in different sources that report the same condition, targeted cross-layer
  children, and the eBPF programs to start on the affected node.
- **Entity graph (OUT):** typed Kubernetes dependencies (Pod → Node, Pod → PVC → PV, Pod →
  CoreDNS, apiserver → etcd, …), each gated by problem domain.
- **Alert rules:** standard Prometheus rules on the roots, labelled so the operator knows which
  tree to expand.

Everything is plain Kubernetes and Prometheus:

- The knowledge is expressed as Kubernetes custom resources (`zoomlens.dev/v1alpha1`). Their types
  are in [`api/v1alpha1`](../api/v1alpha1), and the CRDs are generated with controller-gen into
  [`deploy/crds`](../deploy/crds).
- The alert rules are standard Prometheus rule files.

| | |
|---|---|
| **Coverage** | node-exporter, OpenTelemetry hostmetrics, cAdvisor, kubelet, kube-state-metrics, kube-apiserver, etcd, kube-scheduler, kube-controller-manager, kube-proxy, CoreDNS, eBPF (Cloudflare ebpf_exporter); optional containerd, CSI sidecars, Cilium, Calico |
| **Size** | 37 `MetricTreePack`s · 1,385 metric names · 111 roots · 113 alert rules · 27 eBPF programs · 26 entity classes · 38 entity edges |
| **Upstream versions** | Kubernetes metrics reference v1.37 · kube-state-metrics v2.20 · node_exporter and cAdvisor `master` (2026-09-30) · etcd v3.5/v3.6 · CoreDNS 1.14 · ebpf_exporter `master` |
| **Content version** | `1.0.0` (label `app.kubernetes.io/version` on every object) |

Browse all trees at once: **[generated/TREE.md](generated/TREE.md)**.

---

## Layout

```
knowledge/
├── packs/<source>.yaml          MetricTreePack objects, one per (source, entity)
├── relations.yaml               MetricRelations: siblings, explains, ebpf
├── entity-graph.yaml            EntityGraph: entity classes and domain-gated edges
├── rules/<source>.rules.yml     Prometheus alerting rules on the roots (source of truth)
├── kustomization.yaml           kubectl apply -k knowledge/
├── sources/*.md                 upstream-derived metric catalogues (provenance for every name)
└── generated/                   do not edit; rebuilt by tools/knowledge-gen
    ├── TREE.md                  every tree, rendered
    ├── catalog/<source>.json    hydration data per name (keep regex, type, endpoint, labels, evidence)
    └── prometheusrule.yaml      the rules as one Prometheus Operator PrometheusRule
```

---

## Install

```bash
kubectl apply -f deploy/crds/                  # MetricTreePack, MetricRelations, EntityGraph
kubectl apply -k knowledge/                    # the knowledge objects (cluster-scoped)
kubectl get mtp                                # SOURCE / ENTITY / OPTIONAL columns
```

Alert rules, either way:

```yaml
# Plain Prometheus: prometheus.yml
rule_files:
  - /etc/prometheus/zoomlens/*.rules.yml     # copy knowledge/rules/ (skip optional add-ons you don't run)
```
```bash
# Prometheus Operator / kube-prometheus-stack
kubectl apply -f knowledge/generated/prometheusrule.yaml
```

---

## The resources

### MetricTreePack

One pack per metric **source** and **entity**, named `<source>-<entity>` (for example
`cadvisor-container`).

```yaml
apiVersion: zoomlens.dev/v1alpha1
kind: MetricTreePack
metadata:
  name: kubelet-persistent-volume-claim
  labels: {app.kubernetes.io/part-of: zoomlens, app.kubernetes.io/version: "1.0.0", zoomlens.dev/source: kubelet}
spec:
  source:
    name: kubelet                      # scrape template id; also the zoomlens_source alert label
    component: kubernetes/kubelet
    job: kubelet
    endpoint: ":10250 /metrics, /metrics/resource, /metrics/probes"
    scope: per-node                    # per-node | cluster | control-plane
  entity: PersistentVolumeClaim        # an EntityGraph entity class
  identityLabels: [namespace, persistentvolumeclaim]
  roots:
    - name: kubelet_volume_stats_available_bytes
      domain: storage                  # gates which entity-graph edges the alert may follow
      companions: [kubelet_volume_stats_capacity_bytes]   # collected with the root; needed by its alert
  children:
    kubelet_volume_stats_available_bytes: [kubelet_volume_stats_used_bytes]
```

| Field | Meaning |
|---|---|
| `source.nameKind` | `metric` (default): Prometheus exposition names, with histograms listed by base name. `scraper`: OpenTelemetry hostmetrics scraper names. `ebpf-program`: ebpf_exporter config names (only on the `ebpf` pack, in `ebpfPrograms`) |
| `roots[]` | Always collected (L0). Each has a `domain` and optional `companions` (denominators and guards its alert needs) |
| `children` | `metric → finer metrics`. Depth 1 is collected from L1, depth 2 from L2, depth 3 from L3 |
| `optional` | Add-ons that only some clusters run |
| `description` | Caveats: required collector flags, feature gates, deprecations |

### MetricRelations

```yaml
spec:
  siblings:                            # same condition seen by different sources
    - name: node-memory
      domain: memory
      members:
        - {source: node-exporter, entity: Node, metric: node_memory_MemAvailable_bytes}
        - {source: kubelet,       entity: Node, metric: node_memory_working_set_bytes}
        - {source: kubelet,       entity: Node, metric: kubelet_evictions}
  explains:                            # targeted children of another entity
    - root: {source: cadvisor, entity: Container, metric: container_cpu_usage_seconds_total}
      targets:
        - {source: node-exporter, entity: Node, metric: node_pressure_cpu_waiting_seconds_total, depth: 1}
        - {source: node-exporter, entity: Node, metric: node_schedstat_waiting_seconds_total, depth: 2}
  ebpf:                                # programs started on the affected node
    - root: {source: cadvisor, entity: Container, metric: container_fs_writes_bytes_total}
      programs: [{name: biolatency, depth: 2}, {name: ext4dist, depth: 3}, {name: xfsdist, depth: 3}]
```

- `siblings` members must be roots.
- `explains` targets must be children, because roots are already collected.
- eBPF programs start at depth 2 or 3, and only on ladder levels that enable eBPF.

### EntityGraph

```yaml
spec:
  entities:
    - {name: Container, operatorKind: Pod, joinLabels: [namespace, pod, container]}
    - {name: Node, joinLabels: [node]}
  edges:
    - {from: Pod, relation: runs-on, to: Node, discovery: informer, definedBy: Pod.spec.nodeName,
       domains: [cpu, memory, storage, network, lifecycle]}
    - {from: Pod, relation: mounts, to: PersistentVolumeClaim, discovery: informer,
       definedBy: "Pod.spec.volumes[].persistentVolumeClaim", domains: [storage, lifecycle]}
```

- `operatorKind` is the `zoomlens_entity` label value. It defaults to the entity name; a container
  alert, for example, acts on its Pod.
- An edge is followed only when the firing root's domain is in `domains`, so a CPU alert never
  pulls in volume metrics.
- `discovery` is one of `informer`, `rule`, `annotation` or `metrics`.

### Alert rules

Standard Prometheus rule files. Each rule is on a root and carries these labels:

| Label | Meaning |
|---|---|
| `zoomlens_source` | The pack source (`kubelet`, `cadvisor`, …). Needed because some roots (`rest_client_requests_total`) exist in several sources |
| `zoomlens_root` | The root metric whose tree is expanded |
| `zoomlens_entity` | Operator entity kind (`Node`, `Pod`, …) |
| `severity` | `info`, `warning` or `critical` |

Keep the entity's `identityLabels` in the expression's `by (…)` clause so they reach the alert.
Annotations are `summary` and `description`; the description says which upstream mixin (node,
kubernetes, etcd, CoreDNS) or method the rule comes from.

---

## How an expansion works

```
alert(zoomlens_source S, zoomlens_root R, entity E)        level L from the dwell ladder
D = domain of R
DOWN  children of R up to depth(L)
      + depth-1 children of R's siblings
      + explains targets of R with depth ≤ depth(L)
      + eBPF programs of R with depth ≤ depth(L), on E's node, if L enables eBPF
OUT   entities within hops(L) of E over edges whose domains contain D:
      their roots' depth-1 children (or only R's explains targets for that entity)
ACT   per node: incident scrape jobs with children only (roots are already collected) via OpAMP;
      eBPF programs in an ebpf_exporter Pod pinned to the node
REVERT after the alert has been resolved for clearFor, drop the jobs and delete the eBPF Pod
```

**Example (disk noisy neighbour):**
1. `ZLContainerHeavyDiskWrites` fires for a pod.
2. **L1:** container I/O children, plus node disk write time and I/O pressure (`explains`), plus PVC
   usage over Pod → PVC.
3. **L2:** latency detail, plus `biolatency` on that node only.
4. **L3:** co-located pods over Pod → Node → Pod, where the noisy `fio` Job shows up.
5. **Revert:** everything is removed after the alert clears.

---

## Design rules (enforced by the validator)

1. **Roots are cheap and alertable.** They come from the upstream mixins' alerts and USE/RED, with
   about 10 per entity at most.
2. **A name appears once per source**, so incident jobs never duplicate always-on series.
3. **Depth ≤ 3**, matching ladder levels L1–L3.
4. **Every metric an alert uses is collected at L0** (a root or companion) in some pack, so every
   alert can be evaluated in steady state.
5. **eBPF is never a root.** It costs kernel CPU per event, so it runs only during incidents on the
   affected node.
6. **Every name is backed by `sources/`.** It must appear in an upstream-derived catalogue there.

---

## Validate and regenerate

```bash
make knowledge          # = cd tools/knowledge-gen && go run . all ../../knowledge
make crds               # regenerate deploy/crds from api/v1alpha1 (controller-gen)
```

`tools/knowledge-gen` is a separate Go module, so the Prometheus library it needs stays out of the operator's
dependencies. It:

1. **Decodes** every object strictly into the `api/v1alpha1` types. Unknown fields are errors, and
   the CRD enum and pattern constraints are checked too.
2. **Parses rule files** with Prometheus's own `rulefmt` loader (as `promtool check rules` does).
   Alert expressions are parsed with the official PromQL parser.
3. **Checks the tree rules above** (uniqueness, reachability, cycles, depth, domains, and that every
   reference resolves), and that every name exists in `sources/`.
4. **Regenerates** `generated/`.

`MKG_VERBOSE=1` lists the names that are backed by prose or bullet lists rather than a metric table
(113, mostly netstat/sockstat/softnet fields).

---

## Using it with the operator today

The operator loads these resources at start-up ([docs/KNOWLEDGE-INTERFACES.md](../docs/KNOWLEDGE-INTERFACES.md)). Current limits:

- **Sources the collector renderer already supports:** `node-exporter`, `cadvisor`, `hostmetrics`,
  `ebpf`. Every other source needs a scrape template in `internal/render`.
- **kube-state-metrics** serves every object from one endpoint. It needs a cluster-level collector
  that filters by entity labels, not by node.
- **Entity kinds** other than `Pod` and `Node` need support in `alerts.entityFrom`. Until then those
  alerts are counted in `zoomlens_alerts_skipped_total` and ignored.
- **Baseline config:** [deploy/collector/base-config.yaml](../deploy/collector/base-config.yaml)
  collects a subset of the roots. Add the remaining roots and their companions.
- **Control-plane sources** (etcd, scheduler, controller-manager) are only reachable on
  self-managed clusters.

---

## Known limits

- **containerd metric names are derived, not observed.** containerd publishes no metric list.
  Confirm them against a live `/v1/metrics`.
- **Some label values need a check on a live cluster:** `prober_probe_total{result="failed"}`,
  `storage_operation_duration_seconds{status!="success"}`, and `kubernetes_healthcheck == 0` meaning
  failure.
- **Some series exist only with extra flags or feature gates:**
  - cAdvisor `container_pressure_*` needs KubeletPSI (on by default from 1.34).
  - `container_cpu_schedstat_*` and `container_tasks_state` need a standalone cAdvisor.
  - etcd `grpc_server_handling_seconds` needs `--metrics=extensive`.
  - `node_systemd_*`, `node_processes_*` and `node_ethtool_*` need those node_exporter collectors.

  Missing series are simply not collected.
- **Thresholds** follow the upstream mixins, and a few are lowered so Zoom-Lens zooms in earlier;
  each rule's `description` says which. Rules marked "tune" depend on the cluster.
- **Deliberately left out:**
  - KSM object-inventory metrics (annotations, resource versions, configmap/secret/RBAC info).
  - Build/info metrics.
  - Deprecated names.
  - Duplicates of cAdvisor on kubelet `/metrics/resource`.
  - Hardware-dynamic families (`node_hwmon_*`, `node_zfs_*`).
  - Demo-only eBPF examples.
  - Application metrics, which belong in app-specific packs.

## Contributing

1. Edit `packs/`, `relations.yaml`, `entity-graph.yaml` or `rules/`.
2. Run `make knowledge` and commit the regenerated `generated/`.
3. When an upstream project releases new metrics, refresh the matching `sources/*.md` first.
4. Bump `app.kubernetes.io/version` when trees change.
