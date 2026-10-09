# Knowledge layer: interfaces and contract

> This page is for **code**: implementing or replacing the knowledge interfaces. To **author or
> maintain** the knowledge content (packs, relations, rules), read the
> [knowledge guide](knowledge-guide/README.md).

The **knowledge layer** answers three questions for the operator: *which finer metrics sit below this
root* (the **metric tree**), *which entities are related to this one* (the **dependency graph**), and
*how long to wait before going deeper* (the **escalation policy**). The operator depends only on the
three interfaces in [`internal/knowledge/knowledge.go`](../internal/knowledge/knowledge.go).

## What provides each part

| Part | Provided by | Source of truth |
|---|---|---|
| Metric tree (DOWN) | [`internal/knowledge/crd`](../internal/knowledge/crd) | `MetricTreePack` and `MetricRelations` resources, authored in [`knowledge/`](../knowledge/README.md) |
| Dependency graph (OUT) | [`internal/knowledge/crd`](../internal/knowledge/crd) + [`internal/knowledge/topology`](../internal/knowledge/topology) | the `EntityGraph` resource says which edges exist and for which problem domain; `topology` resolves Pod → Node → co-located Pods from the Kubernetes API |
| Escalation policy | [`internal/knowledge/policy`](../internal/knowledge/policy) | the ladder file, [`deploy/operator/ladder.yaml`](../deploy/operator/ladder.yaml) |

The operator loads the knowledge resources at start-up and re-lists them every `--knowledge-refresh`
(default 1m), so editing a pack takes effect without a restart. It refuses to start if the CRDs are
missing. Authoring and validating the resources is described in
[knowledge/README.md](../knowledge/README.md) and the
[knowledge guide](knowledge-guide/README.md); `make knowledge` validates and regenerates them.

### The ladder file

```yaml
clearFor: 5m                 # resolved for this long -> revert to the baseline
levels:                      # sorted by `after`; the highest level whose `after` has elapsed applies
  - {after: 0s,  depth: 1, hops: 1, scrapeInterval: 10s}
  - {after: 5m,  depth: 2, hops: 1, scrapeInterval: 5s, enableEBPF: true}
  - {after: 10m, depth: 3, hops: 2, scrapeInterval: 5s, enableEBPF: true}
```

`depth` is how far down the metric tree, `hops` how far out the dependency graph, `enableEBPF`
allows the tree's eBPF programs to start. Unknown fields and intervals below 1s are rejected.

---

## 1. The three interfaces

```go
type MetricTree interface {
    // Every metric below root for this entity kind, down to depth (1 = direct
    // children). Root excluded. Unknown root: empty slice, nil error.
    Expand(root string, kind EntityKind, depth int) ([]MetricRef, error)
}

type DependencyGraph interface {
    // The entity itself plus every entity up to hops away.
    // MUST fill Entity.Node for node-bound entities.
    Related(ctx context.Context, e Entity, hops int) ([]Entity, error)
}

type EscalationPolicy interface {
    LevelFor(dwell time.Duration) Level // level for an alert firing for `dwell`
    ClearFor() time.Duration            // how long "resolved" before reverting
}
```

Types (same file): `MetricRef{Source, Name}`, `Entity{Kind, Name, Namespace, Node}`,
`Level{Index, Depth, Hops, ScrapeInterval, EnableEBPF}`.

---

## 2. How the operator uses them

On every reconcile (every 15 s and on each webhook) the operator turns each firing incident's
dwell time into a ladder level, expands the root down the tree to that depth, walks the graph out
to that many hops, and merges the resulting scopes per node. The full step-by-step walkthrough is
[knowledge guide, chapter 6](knowledge-guide/06-runtime-how-it-is-used.md).

---

## 3. Rules an implementation must follow

| Rule | Why |
|---|---|
| **`MetricRef.Name` is the metric name at scrape time** (the exposition name, e.g. `container_cpu_cfs_throttled_periods_total`) | the collector filters with `metric_relabel_configs` before any translation |
| `Source` must be one of `cadvisor`, `node-exporter`, `app`, `hostmetrics`, `ebpf` | each maps to a scrape template in [`internal/render`](../internal/render/render.go). New sources need a renderer change |
| For `hostmetrics`, `Name` is a **scraper** (`paging`, `processes`, `disk`, `network`, `filesystem`, …) | enables that scraper on the node |
| For `ebpf`, `Name` is an **ebpf_exporter example config** name (`biolatency`, `tcp-retransmit`, `cfs-throttling`, `udp-drops`, `kfree_skb`, `oomkill`, `cachestat`, …) | becomes `--config.names` of the per-node pod; only used when `Level.EnableEBPF` |
| **Do not return root metrics** from `Expand` | roots are always collected by the baseline ([`deploy/collector/base-config.yaml`](../deploy/collector/base-config.yaml)); returning them duplicates series under a `zl-*` job |
| Metric names must match `^[a-zA-Z_:][a-zA-Z0-9_:]*$`; pod/node names must be valid DNS names | the renderer rejects anything else (the node keeps its baseline, error in `/debug/state`) |
| `Related` must include the entity itself and fill `Node` | the operator actuates per node; entities without `Node` are ignored |
| `LevelFor` must be monotonic in `dwell`, return `Index 0` for "no escalation", and `ScrapeInterval ≥ 1s` | the ladder only moves up; interval is also used as scrape timeout |
| Implementations must be safe for concurrent use and fast (called on every reconcile) | cache API lookups (informers) instead of querying per call |

---

## 4. Contract with alert rules

The operator reads the `zoomlens_root`, `zoomlens_entity`, `pod`, `namespace` and `node` labels from
Alertmanager alerts ([`internal/alerts`](../internal/alerts/alerts.go)); `zoomlens_root` is required
and is the first argument of `Tree.Expand`. The full label table and what the validator checks are in
[knowledge guide, chapter 5.2](knowledge-guide/05-alert-rules-and-promql.md#52-the-label-contract-with-the-operator).

---

## 5. Plugging in your own knowledge source

The three interfaces are the extension point. To read the tree or graph from somewhere else (a
service, a different CRD group, a file), implement `knowledge.MetricTree` and/or
`knowledge.DependencyGraph` and assemble them in [`cmd/zoomlens/main.go`](../cmd/zoomlens/main.go):

```go
k := knowledge.Knowledge{
    Tree:   crd.Tree{Store: store},                                         // replace with yours
    Graph:  crd.Graph{Store: store, Inner: topology.NewGraph(client)},      // replace with yours
    Policy: pol,
}
```

Nothing else in the operator changes. If the new implementation needs more RBAC (informers on PVCs,
EndpointSlices, …), extend the `zoomlens-operator` ClusterRole in
[`deploy/operator/operator.yaml`](../deploy/operator/operator.yaml); it already allows get/list/watch
on pods, nodes, PVCs, PVs, services, endpointslices and the `zoomlens.dev` resources.

## 6. Testing

- **Unit:** [`internal/plan/plan_test.go`](../internal/plan/plan_test.go) has fake `MetricTree` and
  `DependencyGraph` implementations; [`internal/knowledge/crd/crd_test.go`](../internal/knowledge/crd/crd_test.go)
  exercises the real loader against the shipped `knowledge/` files.
- **Knowledge content:** `make knowledge` checks every pack, relation, entity edge and rule against the
  schema and the upstream metric catalogues in `knowledge/generated/catalog`.
