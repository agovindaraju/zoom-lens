# Development notes

Implementation status, tests, code layout and pinned versions. For the overview see the
[README](../README.md); for the design see [ARCHITECTURE.md](ARCHITECTURE.md).

## What is implemented

| Part | Where |
|---|---|
| Alertmanager webhook → incidents | `internal/alerts`, `internal/httpserver` |
| Dwell-ladder state machine (escalate, cool-down, re-fire, revert, start-up grace) | `internal/incident` |
| Per-node planning and merging of overlapping incidents | `internal/plan` |
| Collector overlay rendering (cAdvisor, node-exporter, app, hostmetrics, eBPF jobs) | `internal/render` |
| OpAMP server: node identification, per-node remote config, status tracking | `internal/opamp` |
| eBPF actuator: `ebpf_exporter` pod per node, deadline safety net | `internal/ebpf` |
| Series-budget guardrail (fails closed) | `internal/budget` |
| Knowledge: metric tree and entity graph from the `zoomlens.dev` resources | `internal/knowledge/crd`, `api/v1alpha1`, `knowledge/` |
| Knowledge: live Pod → Node → co-located Pods edges | `internal/knowledge/topology` |
| Knowledge: dwell ladder | `internal/knowledge/policy`, `deploy/operator/ladder.yaml` |
| Self-metrics, `/debug/state`, health probes | `internal/metrics`, `internal/httpserver` |
| Demo app, load generator, CPU and disk noisy neighbours (one image) | `demo/app` |
| Manifests, Helm values, alert rules, Grafana dashboard | `deploy/` |
| One-shot and step-by-step automation | `scripts/`, `Makefile` |

## How it was tested

| Test | Result | Command |
|---|---|---|
| Unit tests for every package, `go vet`, `gofmt` | pass | `make check` |
| Knowledge validation: packs, relations, entity graph, rules | 0 errors | `make knowledge` |
| Collector configs (baseline, baseline + rendered overlay) against otelcol-contrib 0.161.0 | pass | `otelcol-contrib validate` |
| Kustomize render of all manifests | pass | `make check` |
| End to end on a real cluster (Rancher Desktop, single-node k3s v1.32): steady state → alert → L1 → L2 with eBPF pod → L3 → resolve → revert | pass | `./scripts/deploy-all.sh`, then `./demo/scripts/scenario.sh cpu start` |
| Multi-node and large clusters, other eBPF kernels | **not yet run** | |

Things to check on a new cluster are in [DEPLOYMENT.md](DEPLOYMENT.md#8-troubleshooting)
(metric-name translation, kubelet TLS and RBAC, eBPF kernel support).

## Code layout

```
cmd/
  zoomlens/            operator entry point
internal/
  alerts/              Alertmanager payload -> signals
  budget/              Prometheus head-series guardrail
  config/              flags and ZOOMLENS_* environment
  ebpf/                ebpf_exporter pod actuator
  httpserver/          /alerts /healthz /readyz /metrics /debug/state
  incident/            dwell-ladder state machine and reconcile loop
  knowledge/           the three interfaces (the contract)
    crd/               metric tree + entity graph from the zoomlens.dev resources
    topology/          Pod -> Node -> co-located Pods from the Kubernetes API
    policy/            the dwell ladder from the ladder file
  metrics/             zoomlens_* self-metrics
  opamp/               OpAMP server (opamp-go)
  plan/                per-node scope planning and merge
  render/              collector overlay YAML
api/v1alpha1/          MetricTreePack, MetricRelations, EntityGraph types
knowledge/             curated packs, relations, entity graph, rules (make knowledge)
tools/knowledge-gen/              validator and generator for knowledge/ (separate Go module)
build/                 Dockerfiles (Go binaries; collector = Supervisor + contrib)
deploy/                kind, monitoring, collector, operator, crds
demo/                  everything demo-only: app/, deploy/, grafana/, monitoring/, scripts/, docs/
config/zoomlens.env    deployment settings, overridable through the environment
scripts/               deploy steps, scenarios, status, teardown
docs/                  documentation and poster
```

## Working on it

```bash
make test        # unit tests (CGO_ENABLED=0 by default)
make check       # vet + tests + render manifests + script syntax
make knowledge   # validate knowledge/ and regenerate knowledge/generated
make crds        # regenerate deepcopy code and deploy/crds from api/v1alpha1
make build       # bin/zoomlens, bin/demo-app
```

Pinned versions: Go 1.25, opamp-go v0.25.0, client-go v0.34.1, OpenTelemetry Collector and
Supervisor 0.161.0, ebpf_exporter v2.5.1, kube-prometheus-stack 91.8.2.

The Go module path is `github.com/zoomlens/zoomlens`. If you fork the project, rename it with
`go mod edit -module <path>` and update the import paths.
