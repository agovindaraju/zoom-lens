# 1. The big picture

## 1.1 The problem the knowledge graph solves

Zoom-Lens changes **what is collected** while an incident is happening:

- **Steady state:** only a few cheap metrics per entity (node, pod, volume, …) are collected.
- **Incident:** when an alert fires, collection is widened *around* the affected entity.
- **After the incident:** collection is narrowed again once the alert clears.

The operator that does this is generic. It knows how to:
- receive alerts;
- keep a dwell timer per incident;
- push collector configs over OpAMP;
- start eBPF programs on a node.

What it does **not** know by itself is *Kubernetes monitoring knowledge*:

- Which of the ~1,500 standard Kubernetes metrics are worth collecting all the time?
- When `node_disk_io_time_seconds_total` shows a busy disk, which finer metrics explain why?
- When a container is CPU-throttled, which node-level metrics are relevant, and does it make sense
  to look at the pods next to it?
- Which alert should start all this, and when?

That knowledge is the **knowledge graph**. It is kept separate from the operator code, as data,
so it can be reviewed, versioned, validated and extended without touching Go code.

## 1.2 The four parts

| Part | Kubernetes resource / file | Axis | Answers |
|---|---|---|---|
| **Metric trees** | `MetricTreePack` (37 of them) | **DOWN** | For one source and one entity kind: which metrics are roots, and what finer metrics sit below each root, down to depth 3 |
| **Relations** | `MetricRelations` (1) | across trees | Which roots in *different* sources describe the same condition (*siblings*), which specific metrics of *another* entity explain a root (*explains*), which eBPF programs to start (*ebpf*) |
| **Entity graph** | `EntityGraph` (1) | **OUT** | How Kubernetes entities depend on each other (Pod runs-on Node, Pod mounts PVC, …), and for which problem domains each dependency is worth following |
| **Alert rules** | Prometheus rule files (14 files, 113 rules) | trigger | When a root is "bad enough" to start an expansion. Each rule is labelled with the root to expand |

There is a fifth input that is **not** part of the knowledge resources: the **dwell ladder**
(escalation policy: L1 at 0 min, L2 at 3 min, L3 at 6 min, `clearFor` 3 min). It lives in
[`deploy/operator/ladder.yaml`](../../deploy/operator/ladder.yaml).

## 1.3 Numbers (knowledge v1.0.0)

| What | Count |
|---|---|
| Metric sources covered | 16 (plus the demo app pack) |
| `MetricTreePack` objects | 37 (plus 1 demo pack) |
| Metric names in the trees | 1,385 |
| Roots (always collected) | 111 (plus 1 demo root) |
| Alert rules | 113 (plus 2 demo rules) |
| eBPF programs described | 27 |
| Sibling groups / explains links / eBPF links | 17 / 30 / 22 |
| Entity classes / entity edges | 26 / 38 |
| Upstream metrics researched to choose from | about 1,500 names, from 4 catalogues |

## 1.4 From authoring to runtime

```
①  RESEARCH           upstream docs and source  ──▶  knowledge/sources/*.md
                      (Kubernetes metrics reference v1.37, kube-state-metrics v2.20,
                       node_exporter + cAdvisor master, etcd 3.5/3.6, CoreDNS 1.14, ebpf_exporter)

②  AUTHOR             knowledge/packs/*.yaml, relations.yaml, entity-graph.yaml, rules/*.rules.yml

③  VALIDATE           make knowledge  (= cd hack/mkg && go run . all ../../knowledge)
                      • strict decode into api/v1alpha1 types      • every name exists in sources/
                      • tree rules (unique, depth ≤ 3, no cycles)  • rules parsed by Prometheus rulefmt
                      • every metric an alert uses is collected at L0
                      ──▶ regenerates knowledge/generated/ (TREE.md, catalog, prometheusrule.yaml)

④  DEPLOY             kubectl apply -k deploy/crds      (3 CRDs)
                      kubectl apply -k knowledge        (37 packs, relations, entity graph)
                      kubectl apply -f knowledge/generated/prometheusrule.yaml   (Prometheus Operator)
                      (scripts/03 and 04 do this for you)

⑤  LOAD               operator started (it requires the CRDs and the knowledge resources)
                      lists the 3 resource types every minute → in-memory Snapshot

⑥  USE                alert on a root → incident → level L → Snapshot.Expand + RelatedForRoot
                      → per-node overlay (children only) → OpAMP → collector → Prometheus
```

## 1.5 A worked example (from the real local run)

This is a condensed version of a run on a single-node cluster. A CPU noisy neighbour was started on
the only node of a Rancher Desktop cluster.

1. **Steady state (L0).** The collector on the node scrapes only roots and companions every 30 s,
   for example `node_cpu_seconds_total` from node-exporter.
2. **The root alert fires.** `ZLNodeCPUHighUsage`, from `knowledge/rules/node-exporter.rules.yml`,
   uses `1 - avg(rate(node_cpu_seconds_total{mode=~"idle|iowait"}[5m])) > 0.9`. It carries the
   labels `zoomlens_root=node_cpu_seconds_total`, `zoomlens_entity=Node` and `node=…`.
3. **The operator opens an incident at L1** (depth 1, 1 hop, 10 s). It asks the knowledge:
   - **DOWN:** the children of `node_cpu_seconds_total` in the `node-exporter-node` pack at
     depth 1, such as `node_pressure_cpu_waiting_seconds_total`, `node_schedstat_waiting_seconds_total`
     and `node_procs_running`.
   - **Siblings:** the `node-cpu` sibling group adds the depth-1 children of the hostmetrics `cpu`
     root, which are the `load`, `processes` and `system` scrapers.
   - **OUT:** the root's domain is `cpu`, so the edge `Pod -runs-on-> Node` may be followed (its
     domains include `cpu`). From a Node, one hop reaches the pods on it.
4. **L2 after 3 minutes** (depth 2, 5 s, eBPF allowed). The `ebpf` link of `node_cpu_seconds_total`
   starts `softirq-latency` in an `ebpf_exporter` pod on that node only.
5. **L3 after 6 minutes** (depth 3). `timers` and `llcstat` are added to the eBPF pod.
6. **Revert.** The fault is stopped and the alert resolves. After `clearFor` (3 min), the overlay
   is removed and the eBPF pod is deleted. The node goes back to roots only.

Measured at L3 (two incidents open): 1,277 extra `zl-*` series on top of about 567 baseline
series. 1,145 of those extra series came from eBPF histograms, which is why eBPF is never a root.

## 1.6 What is new here compared with typical Kubernetes monitoring

Typical setups (kube-prometheus-stack with its mixins) collect **everything** those exporters
expose, all the time, and use alert rules only to page people. The knowledge graph reuses the same
exporters, metric names and many of the same alert expressions. The difference is that it
organises them so collection can be **turned up and down** around an incident:

- **The mixin alerts** become the triggers.
- **Roots** are a small, deliberately chosen subset.
- **Everything else** is dormant until a related root fires. Nothing is dropped forever.
- **Kubernetes topology** (Pod → Node → co-located Pods, Pod → PVC → PV, …) decides *where* to
  collect. The problem domain decides *which* topology to follow.
