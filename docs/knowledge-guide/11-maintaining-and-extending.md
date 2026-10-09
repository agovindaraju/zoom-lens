# 11. Maintaining and extending: recipes

Every recipe ends with the same check:

```bash
make knowledge                                   # = cd hack/mkg && go run . all ../../knowledge
CGO_ENABLED=0 go test ./internal/knowledge/crd/  # runtime behaviour against the real files
```

`make knowledge` prints a table per pack, then `N error(s), M warning(s)`, and regenerates
`knowledge/generated/`. Commit the regenerated files together with your change.
`MKG_VERBOSE=1 make knowledge` also lists the names that are backed by prose rather than a metric
table.

---

## Recipe A: add a child metric to a tree

1. **Check that the metric exists upstream** and is in `knowledge/sources/*.md`. If it isn't,
   add it to the right catalogue with its type, labels and a one-line meaning. The validator
   rejects names that are in no catalogue.
2. Pick the parent and the depth (chapter 4.3):
   - depth 1 is "open first";
   - depth 2 is the finer breakdown;
   - depth 3 is rare detail or context.
3. Add it under the parent in `children:` of the pack in `knowledge/packs/<source>.yaml`.
4. Check that it isn't already somewhere else in **the same source**. The validator rejects
   duplicates, and keeping one occurrence per source avoids duplicate series.
5. Run the checks.

```yaml
children:
  node_disk_io_time_seconds_total: [node_pressure_io_waiting_seconds_total, …, node_disk_io_now]
  node_disk_io_now: [node_disk_new_detail_total]            # new, depth 2
```

---

## Recipe B: add a root

A root costs series on every entity, all the time. Add one only if it is alertable and cheap
(chapter 4.2).

1. Add it under `roots:` with a `domain`, and with `companions` if its alert needs them.
2. Give it children (Recipe A). A root with no children expands into nothing.
3. Add its alert rule (Recipe C).
4. **Update the collector baseline** if the source is actuated (node-exporter, cAdvisor, app):
   add the root and its companions to the keep regex of the `base-*` job in
   `deploy/collector/base-config.yaml`. `TestBaselineCollectsAlwaysOnMetrics` fails if you forget.
5. Consider a sibling group (Recipe D) if another source reports the same condition.

---

## Recipe C: add an alert rule

1. Edit `knowledge/rules/<source>.rules.yml` and follow the pattern (chapter 8):
   - `expr: (<value>) <op> <threshold>`;
   - the identity labels in `by (…)`;
   - only roots and companions in the expression (anything else can't fire in steady state);
   - the labels `severity`, `zoomlens_source`, `zoomlens_root` and `zoomlens_entity`, where
     `zoomlens_entity` is the pack entity's operator kind;
   - a `summary`, and a `description` that names the origin (mixin, USE/RED, operational).
2. Keep the alert name unique and prefixed `ZL`.
3. Run `make knowledge`. It loads the file with Prometheus's `rulefmt`, parses the expression and
   regenerates `knowledge/generated/prometheusrule.yaml`.
4. Deploy with step 03, or with `kubectl apply -f knowledge/generated/prometheusrule.yaml`.
5. Optionally, write a `promtool test rules` unit test with sample series.

---

## Recipe D: add a relation

**Sibling group:** roots in different packs that report the same condition.
```yaml
siblings:
  - name: node-memory
    domain: memory
    members:
      - {source: node-exporter, entity: Node, metric: node_memory_MemAvailable_bytes}
      - {source: kubelet,       entity: Node, metric: node_memory_working_set_bytes}
```

**Explains link:** targeted children of another entity (targets must be **children**, not roots).
```yaml
explains:
  - root: {source: cadvisor, entity: Container, metric: container_fs_writes_bytes_total}
    targets:
      - {source: node-exporter, entity: Node, metric: node_disk_write_time_seconds_total, depth: 1}
```

**eBPF link:** programs from the `ebpf` pack, at depth 2 or 3. Put high-cardinality programs at 3.
```yaml
ebpf:
  - root: {source: etcd, entity: Etcd, metric: etcd_disk_wal_fsync_duration_seconds}
    programs: [{name: biolatency, depth: 2}]
```

Application-specific relations belong with the application, like
`deploy/demo/knowledge-pack.yaml` (its own `MetricRelations` object), not in `knowledge/`.

---

## Recipe E: add a new metric source

Example: a new exporter `foo`.

1. **Catalogue.** Add `knowledge/sources/foo.md` with a metric table (columns `Metric`, `Type`,
   `Labels`, `Meaning`), built from the upstream docs or source. Add `foo` to `sourceFiles` in
   `hack/mkg/sources.go` so lookups prefer it.
2. **Entity.** If `foo` describes a new kind of entity, add an entity class to
   `knowledge/entity-graph.yaml` (Recipe F).
3. **Pack.** Create `knowledge/packs/foo.yaml` with one `MetricTreePack` per entity, named
   `foo-<entity>` and labelled `zoomlens.dev/source: foo`. Set `scope` and `identityLabels`.
4. **Kustomization.** Add `packs/foo.yaml` to `knowledge/kustomization.yaml`.
5. **Rules.** Add `knowledge/rules/foo.rules.yml` (Recipe C).
6. **Relations.** Link it to existing roots with siblings and explains (Recipe D).
7. **Operator (Go), so it is actually collected:**
   - add a `knowledge.Source` constant and include it in `KnownSources`
     (`internal/knowledge/knowledge.go`);
   - add a scrape template in `internal/render` (`scrapeJob`): discovery, scheme, path, auth and
     keep filters. For histogram children, use the `keepRegex` from `generated/catalog/foo.json`;
   - add the roots and companions to the baseline (`deploy/collector/base-config.yaml`) and to the
     baseline test;
   - for `scope: cluster` sources (like kube-state-metrics), plan a cluster-level collector. The
     per-node collectors can't scrape them.
8. Run the checks, plus `make check` for the operator.

Until step 7 is done, the source is fully described and validated, but the operator skips its
children (chapter 9.5).

---

## Recipe F: add an entity class or an edge

```yaml
entities:
  - {name: Ingress, joinLabels: [namespace, ingress]}          # operatorKind defaults to the name
edges:
  - from: Ingress
    relation: routes-to
    to: Service
    discovery: informer                                        # informer | rule | annotation | metrics
    definedBy: Ingress.spec.rules[].http.paths[].backend.service
    domains: [network, lifecycle]                              # when it is worth following
```

- Keep the `domains` list **narrow**. It is the blast-radius control.
- Set `bidirectional: false` for "calls" and "depends-on" style edges that shouldn't be walked
  backwards.
- Operator support: the current code gates only the Pod ↔ Node walk (chapter 9.6). New classes
  are validated and loaded, but not traversed until the graph code is extended.

---

## Recipe G: add knowledge for your own application

Follow `deploy/demo/knowledge-pack.yaml`:
- a `MetricTreePack` with `source.name: app`, `entity: Pod` and your RED root (for example a
  request-duration histogram), with children for your custom metrics;
- a `MetricRelations` object whose `explains` point at the cAdvisor and node-exporter children
  that usually explain your latency, plus eBPF links if they are useful;
- an alert rule with `zoomlens_root`, `zoomlens_entity: Pod` and `by (namespace, pod, node)`;
- the annotation `zoomlens.dev/scrape: "true"` on the pods, so the `base-app` and `zl-app` jobs
  find them.

The curated validator covers `knowledge/` only. Application packs are checked by the operator
tests (`TestExpandDemoApp`) or by `kubectl apply --dry-run=server`.

---

## Recipe H: follow an upstream release

1. Re-generate the matching catalogue in `knowledge/sources/` from the new upstream docs, for
   example the Kubernetes metrics reference for the new version.
2. Run `make knowledge`. Renamed or removed metrics show up as "not in any sources/*.md catalogue".
3. For each one, use the successor name (the catalogues list renames), or drop it.
4. Update the pack `component` string (for example `kubernetes/kube-apiserver v1.38`) and the
   pack `description` notes.
5. Bump `app.kubernetes.io/version` on all objects (a minor version for new names, a major version
   if roots change).

---

## Recipe I: change the schema

1. Edit `api/v1alpha1/types.go` and its markers (`+kubebuilder:validation:…`).
2. Run `make crds` to regenerate `deploy/crds/*.yaml` and `zz_generated.deepcopy.go`.
3. Update `hack/mkg/validate.go` (offline checks) and `internal/knowledge/crd` (runtime use).
4. For a breaking change, add a new version (`v1alpha2`) rather than editing `v1alpha1` in place.
5. Run `make knowledge`, `go test ./...`, and `kubectl apply -k deploy/crds` on the cluster.

---

## Checklist before committing

- [ ] `make knowledge`: 0 errors
- [ ] `go test ./internal/knowledge/crd/` passes (including the baseline guard)
- [ ] `knowledge/generated/` regenerated and committed
- [ ] New roots and companions added to the baseline, if the source is actuated
- [ ] `description` notes updated for flags, feature gates and deprecations
- [ ] `app.kubernetes.io/version` bumped if trees changed
