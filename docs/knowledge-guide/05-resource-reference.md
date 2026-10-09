# 5. Resource reference: every field

The schema lives in [`api/v1alpha1/types.go`](../../api/v1alpha1/types.go). The CRDs in
[`deploy/crds/`](../../deploy/crds) are generated from it. This chapter walks through every field,
with the validation the API server applies and what the operator does with it.

All three resources are **cluster-scoped** (no namespace), in API group **`zoomlens.dev`**, version
**`v1alpha1`**.

| Kind | Plural | Short name | Objects in `knowledge/` |
|---|---|---|---|
| `MetricTreePack` | `metrictreepacks` | `mtp` | 37 (+1 demo) |
| `MetricRelations` | `metricrelations` | `mrel` | 1 (+1 demo) |
| `EntityGraph` | `entitygraphs` | `eg` | 1 |

---

## 5.1 MetricTreePack

One object per **(source, entity)**. The name must be `<source>-<entity in kebab case>`, for
example `kube-state-metrics-horizontal-pod-autoscaler`. The validator enforces this.

```yaml
apiVersion: zoomlens.dev/v1alpha1
kind: MetricTreePack
metadata:
  name: node-exporter-node                           # <source>-<entity>
  labels:
    app.kubernetes.io/part-of: zoomlens
    app.kubernetes.io/version: "1.0.0"               # content version
    zoomlens.dev/source: node-exporter               # = spec.source.name
spec:
  source:
    name: node-exporter                              # MetricSource.Name
    component: prometheus/node_exporter              # upstream project the names come from
    job: node-exporter                               # Prometheus job name
    endpoint: ":9100/metrics"
    scope: per-node                                  # per-node | cluster | control-plane
    # nameKind: metric                               # default; scraper | ebpf-program
  entity: Node                                       # an EntityGraph entity class
  identityLabels: [node]
  roots:
    - name: node_memory_MemAvailable_bytes
      domain: memory
      companions: [node_memory_MemTotal_bytes]
  children:
    node_memory_MemAvailable_bytes: [node_pressure_memory_waiting_seconds_total, node_vmstat_pgmajfault]
    node_pressure_memory_waiting_seconds_total: [node_pressure_memory_stalled_seconds_total]
  description: "node_systemd_* needs --collector.systemd; …"
```

### `spec.source` (type `MetricSource`)

| Field | Required | Validation (CRD) | Meaning / used by |
|---|---|---|---|
| `name` | yes | pattern `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$` | Source id. The operator's scrape templates are keyed by it. Today `cadvisor`, `node-exporter`, `app`, `hostmetrics` and `ebpf` can be actuated; other sources are indexed but skipped. Also the `zoomlens_source` rule label |
| `component` | yes | — | Upstream project and version, for humans and provenance |
| `job` | no | — | Prometheus job name for the source |
| `endpoint` | no | — | Port and path |
| `scope` | yes | enum `per-node`, `cluster`, `control-plane` | Which collector can scrape it. Per-node collectors can only scrape `per-node` sources on their node |
| `nameKind` | no | enum `metric` (default), `scraper`, `ebpf-program` | How to interpret names: exposition metric names, hostmetrics scraper names, or ebpf_exporter config names |

### Other `spec` fields (type `MetricTreePackSpec`)

| Field | Type | Meaning |
|---|---|---|
| `entity` | string | Entity class, which must be defined in `EntityGraph.spec.entities`. Its operator kind is used to match alerts (`zoomlens_entity`) |
| `identityLabels` | []string | Labels that identify the entity on this pack's series. Alert rules keep them in `by (…)` |
| `optional` | bool | The add-on exists only in some clusters (CNI, CSI, containerd). Shown in `kubectl get mtp` |
| `roots` | []`RootMetric` | Always collected. Each has `name`, `domain` (8-value enum) and optional `companions` |
| `children` | map[string][]string | `parent → children`. A parent is a root or another child. Depth is the distance from a root. Only names reachable from a root count |
| `ebpfPrograms` | []`EBPFProgram` | Only on the `ebpf` pack. Each program has `name`, `domain`, `metrics` (what it exposes), `cardinality` and `kernel` requirements |
| `description` | string | Caveats: flags, feature gates, deprecations, availability |

### `RootMetric`

| Field | Meaning |
|---|---|
| `name` | Root metric name |
| `domain` | `cpu` · `memory` · `storage` · `network` · `dns` · `lifecycle` · `control-plane` · `scheduling` |
| `companions` | Always collected with the root; needed by its alert; never expanded |

### `EBPFProgram` (ebpf pack only)

| Field | Example |
|---|---|
| `name` | `biolatency` (passed as `--config.names`) |
| `domain` | `storage` |
| `metrics` | `[ebpf_exporter_bio_latency_seconds]` |
| `cardinality` | `low (device, operation)` |
| `kernel` | `>= 5.16 (use cachestat-pre-kernel-5.16 below)` (for `cachestat`) |

### Printer columns

`kubectl get mtp` prints `NAME`, `SOURCE`, `ENTITY` and `OPTIONAL`.

---

## 5.2 MetricRelations

```yaml
apiVersion: zoomlens.dev/v1alpha1
kind: MetricRelations
metadata:
  name: kubernetes
spec:
  siblings:
    - name: node-memory                     # SiblingGroup
      domain: memory
      members:                              # ≥ 2 MetricRef, all roots, from ≥ 2 packs
        - {source: node-exporter, entity: Node, metric: node_memory_MemAvailable_bytes}
        - {source: kubelet, entity: Node, metric: kubelet_evictions}
  explains:
    - root: {source: cadvisor, entity: Container, metric: container_cpu_usage_seconds_total}
      targets:                              # DepthRef = MetricRef + depth 1..3; must be children
        - {source: node-exporter, entity: Node, metric: node_pressure_cpu_waiting_seconds_total, depth: 1}
  ebpf:
    - root: {source: cadvisor, entity: Container, metric: container_fs_writes_bytes_total}
      programs:                             # ProgramRef = name + depth 2..3
        - {name: biolatency, depth: 2}
```

| Type | Fields | Validation |
|---|---|---|
| `MetricRef` | `source`, `entity`, `metric` | Must point at an existing pack (source + entity) and a name in it |
| `SiblingGroup` | `name`, `domain`, `members` | `members` min 2 (CRD). The validator also requires roots from ≥ 2 packs |
| `DepthRef` | `MetricRef` inline + `depth` | `depth` 1..3 (CRD). The validator requires the target to be a **child** |
| `ExplainsLink` | `root`, `targets` | `root` must be a root |
| `ProgramRef` | `name`, `depth` | `depth` 2..3 (CRD). `name` must be an `ebpfPrograms` entry |
| `EBPFLink` | `root`, `programs` | `root` must be a root |

Several `MetricRelations` objects can exist. The operator merges them all; the demo app ships its
own (`demo-app`).

---

## 5.3 EntityGraph

```yaml
apiVersion: zoomlens.dev/v1alpha1
kind: EntityGraph
metadata:
  name: kubernetes
spec:
  entities:
    - {name: Node, joinLabels: [node], description: "Per-node scrapes add node=<K8S_NODE_NAME> …"}
    - {name: Container, operatorKind: Pod, joinLabels: [namespace, pod, container]}
  edges:
    - from: Pod
      relation: runs-on
      to: Node
      discovery: informer                  # informer | rule | annotation | metrics
      definedBy: Pod.spec.nodeName
      domains: [cpu, memory, storage, network, lifecycle]   # min 1
      # bidirectional: true                # default
```

| Type | Field | Meaning |
|---|---|---|
| `EntityClass` | `name` | Class name used by packs (`spec.entity`) and edges |
| | `operatorKind` | Value of the `zoomlens_entity` label for this class. Defaults to `name`. Set for `Container` → `Pod`, `CoreDNS` → `Pod`, `Etcd` → `EtcdMember` |
| | `joinLabels` | Labels that join this entity's series across sources |
| | `description` | How to join when labels alone are not enough |
| `EntityEdge` | `from`, `relation`, `to` | A typed dependency |
| | `discovery` | How the edge is built (enum) |
| | `definedBy` | The field, object or rule that defines it |
| | `domains` | Gate: follow the edge only for roots in these domains |
| | `bidirectional` | Walk both ways (default `true`) |
| | `description` | Context |

Exactly one `EntityGraph` is expected (the validator checks this).

---

## 5.4 Alert rule files (`knowledge/rules/<source>.rules.yml`)

These are standard Prometheus rule files. One file per source, with one group per file named
`zoomlens.<source>` and an interval of 30 s.

```yaml
groups:
  - name: zoomlens.kubelet
    interval: 30s
    rules:
      - alert: ZLKubePersistentVolumeFillingUp
        expr: |-
          (min by (namespace, persistentvolumeclaim) (kubelet_volume_stats_available_bytes / kubelet_volume_stats_capacity_bytes)) < 0.03
        for: 1m
        labels:
          severity: critical
          zoomlens_source: kubelet
          zoomlens_root: kubelet_volume_stats_available_bytes
          zoomlens_entity: PersistentVolumeClaim
        annotations:
          summary: "PVC {{ $labels.namespace }}/{{ $labels.persistentvolumeclaim }} has {{ $value | humanizePercentage }} space left"
          description: "Source: kubernetes-mixin KubePersistentVolumeFillingUp (critical)."
```

| Element | Rule |
|---|---|
| `alert` | Unique across all files; prefix `ZL` |
| `expr` | `(<value expression>) <operator> <threshold>`. It keeps the identity labels in `by (…)`. It uses only L0 metrics |
| `for` | Upstream duration. Step 03 may override it for demos (`ZL_KNOWLEDGE_RULE_FOR`) |
| `labels.zoomlens_*` | Required, see chapter 8 |
| `annotations.description` | Where the rule comes from (mixin name, or USE/RED/operational) |

---

## 5.5 Generated files (`knowledge/generated/`, never edit)

### `TREE.md`

Every pack rendered as an indented tree:
- `L0` marks roots, and `+` marks companions;
- `d1`/`d2`/`d3` mark children by depth;
- `<domain>` is shown after each root, and `[alert: …]` lists the rules on it;
- lines starting with `->` show the siblings, explains targets and eBPF programs from
  `MetricRelations`.

### `catalog/<source>.json`

Hydration data per name. It is meant for whatever turns names into scrape configuration.

```json
"node_disk_write_time_seconds_total": {
  "pack": "node-exporter-node",
  "role": "child",
  "parent": "node_disk_io_time_seconds_total",
  "depth": 1,
  "domain": "storage",
  "type": "counter",
  "keepRegex": "node_disk_write_time_seconds_total",
  "labels": "device",
  "help": "Time spent on writes.",
  "evidence": "table",
  "catalogue": "sources/node-exporter-cadvisor.md"
}
```

| Field | Meaning |
|---|---|
| `role` | `root`, `companion`, `child` or `ebpf-metric` |
| `type` | From the catalogue: `counter`, `gauge`, `histogram`, `summary`, `custom` or `untyped`. Empty if the catalogue does not say |
| `keepRegex` | `name(_bucket\|_sum\|_count)` for histograms, `name(_bucket\|_sum\|_count)?` if the type is unknown, the plain name otherwise |
| `evidence` | `table` (a row in a metric table), `prose` (a list in the text), `mentioned` (named in the text) or `unverified` |

### `prometheusrule.yaml`

All rule files wrapped into one `monitoring.coreos.com/v1` `PrometheusRule` named
`zoomlens-roots` in namespace `monitoring`, for clusters that run the Prometheus Operator.

---

## 5.6 The escalation policy (not a CRD)

The dwell ladder is read from [`deploy/operator/ladder.yaml`](../../deploy/operator/ladder.yaml).

```yaml
policy:
  clearFor: 3m
  levels:
    - {after: 0s, depth: 1, hops: 1, scrapeInterval: 10s, enableEBPF: false}   # L1
    - {after: 3m, depth: 2, hops: 1, scrapeInterval: 5s,  enableEBPF: true}    # L2
    - {after: 6m, depth: 3, hops: 2, scrapeInterval: 5s,  enableEBPF: true}    # L3
```

---

## 5.7 Go types (for code readers)

| YAML | Go type (`api/v1alpha1`) |
|---|---|
| `MetricTreePack.spec` | `MetricTreePackSpec` |
| `spec.source` | `MetricSource` (with `Scope`, `NameKind`) |
| `spec.roots[]` | `RootMetric` (with `Domain`) |
| `spec.ebpfPrograms[]` | `EBPFProgram` |
| `MetricRelations.spec` | `MetricRelationsSpec` → `SiblingGroup`, `ExplainsLink`, `EBPFLink`, `MetricRef`, `DepthRef`, `ProgramRef` |
| `EntityGraph.spec` | `EntityGraphSpec` → `EntityClass` (`OperatorKindOf()`), `EntityEdge` (`Discovery`) |

`zz_generated.deepcopy.go` is generated by controller-gen. Don't edit it; run `make crds`.
