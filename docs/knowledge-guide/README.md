# Zoom-Lens knowledge graph: a guided explanation

This folder explains the **Zoom-Lens knowledge graph** from first principles:
- what it is, and how it was built;
- which open-source technologies it relies on, and what to read to understand them;
- how its parts relate to each other;
- how it is deployed, and how the operator uses it at runtime.

It is a learning guide. It does **not** replace or change any other document. The authoritative
files are still the ones in [`knowledge/`](../../knowledge), [`api/v1alpha1/`](../../api/v1alpha1),
[`deploy/crds/`](../../deploy/crds) and [`internal/knowledge/crd/`](../../internal/knowledge/crd).

> Snapshot: repository at commit `c1e696d` (2026-09-30), knowledge content version `1.0.0`.
> Numbers and tables in this guide describe that snapshot. The always-current view of the
> trees is [`knowledge/generated/TREE.md`](../../knowledge/generated/TREE.md).

---

> Implementing or replacing the knowledge interfaces in Go? See
> [KNOWLEDGE-INTERFACES.md](../KNOWLEDGE-INTERFACES.md).

## Reading order

| # | File | You will understand |
|---|---|---|
| 1 | [01-big-picture.md](01-big-picture.md) | What the knowledge graph is for, its four parts, and the path from authoring to runtime |
| 2 | [02-concepts.md](02-concepts.md) | Every term: root, companion, child, depth, level, hop, domain, sibling, explains, overlay, and more, with examples |
| 3 | [03-resource-reference.md](03-resource-reference.md) | Every field of `MetricTreePack`, `MetricRelations`, `EntityGraph`, the rule files and the generated files |
| 4 | [04-relations-and-entity-graph.md](04-relations-and-entity-graph.md) | The 17 sibling groups, 30 explains links, 22 eBPF links, 26 entity classes and 38 edges |
| 5 | [05-alert-rules-and-promql.md](05-alert-rules-and-promql.md) | How the 113 alert rules are written, the PromQL patterns they use, and a table of every rule |
| 6 | [06-runtime-how-it-is-used.md](06-runtime-how-it-is-used.md) | What the operator does with the knowledge, step by step, what works today and what does not |
| 7 | [07-maintaining-and-extending.md](07-maintaining-and-extending.md) | Recipes: add a metric, a root, a rule, a source, an entity or an edge, and check it |

If you only have an hour, read 01 → 02 → 06, then skim 04.

---

## The whole idea in one picture

```
                    ┌──────────────────── AUTHORED (git) ─────────────────────┐
 upstream docs  ──▶ │ knowledge/sources/*.md    (catalogue of real metric names)│
 (k8s, KSM, node-   │ knowledge/packs/*.yaml    MetricTreePack  = DOWN axis     │
  exporter, etcd,   │ knowledge/relations.yaml  MetricRelations = cross-links   │
  CoreDNS, eBPF…)   │ knowledge/entity-graph.yaml EntityGraph   = OUT axis      │
                    │ knowledge/rules/*.rules.yml Prometheus rules on the roots │
                    └───────────────┬─────────────────────────────┬──────────┘
                                    │ tools/knowledge-gen validate + generate │
                                    ▼                              ▼
             kubectl apply -k deploy/crds, -k knowledge/     Prometheus loads the rules
                                    │                              │
                     Kubernetes API server (CRs)              alert fires on a ROOT
                                    │                              │ Alertmanager webhook
                                    ▼                              ▼
                        Zoom-Lens operator:  "root R fired on entity E, level L"
                           Tree.Expand(R, depth)   → which extra metrics   (DOWN)
                           Graph.RelatedForRoot(…) → which pods and nodes  (OUT)
                                    │
                                    ▼  per-node overlay over OpAMP (+ eBPF pod)
                        OpenTelemetry Collector on each affected node
                        scrapes the children → remote write → Prometheus
```

The knowledge graph answers three questions for the operator:

1. **What is worth collecting all the time?** The **roots**, plus the **companions** their alerts
   need.
2. **When a root alert fires, what finer metrics explain it?** The **children**, level by level,
   plus **siblings**, **explains** targets and **eBPF** programs.
3. **Which other entities should we look at, and for which kinds of problem?** The **entity
   graph**, gated by the root's **domain**.

---

## Where things live (repository map for this topic)

| Path | Role |
|---|---|
| `api/v1alpha1/types.go` | Go types of the three resources (the schema), with kubebuilder markers |
| `deploy/crds/*.yaml` | CustomResourceDefinitions generated from those types by controller-gen |
| `knowledge/packs/*.yaml` | 37 `MetricTreePack` objects (16 files, one per source) |
| `knowledge/relations.yaml` | 1 `MetricRelations` object: siblings, explains, eBPF links |
| `knowledge/entity-graph.yaml` | 1 `EntityGraph` object: 26 entity classes, 38 edges |
| `knowledge/rules/*.rules.yml` | 14 standard Prometheus rule files, 113 alert rules |
| `knowledge/sources/*.md` | 4 upstream-derived metric catalogues (evidence for every name) |
| `knowledge/generated/` | Built by the tool: `TREE.md`, `catalog/*.json`, `prometheusrule.yaml` |
| `tools/knowledge-gen/` | Validator and generator (separate Go module) |
| `internal/knowledge/crd/` | Operator code that loads and uses the resources at runtime |
| `internal/knowledge/knowledge.go` | The contract between the operator and any knowledge source |
| `internal/knowledge/topology/`, `internal/knowledge/policy/` | The Kubernetes-API graph walk and the ladder |
| `deploy/operator/ladder.yaml` | The dwell ladder (escalation timings) |
| `demo/deploy/knowledge-pack.yaml` | A demo application pack, in the same format |
