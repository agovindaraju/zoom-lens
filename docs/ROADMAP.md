# Roadmap

## Done

- Operator: Alertmanager webhook, dwell ladder, per-node planning, OpAMP server, eBPF actuator, series budget.
- Knowledge: curated Kubernetes metric trees, relations and entity graph as `zoomlens.dev/v1alpha1`
  resources, 113 root alert rules, validated against upstream metric catalogues.
- Demo app, noisy-neighbour scenarios, Grafana dashboard, deployment scripts.
- End-to-end run on a single-node cluster, including eBPF at L2 and automatic revert.

## Next

- Run on a real multi-node cluster and publish measured numbers: steady state vs. incident vs. collect-everything.
- Entity kinds beyond Pod and Node in the alert path (PVC, Service, …); the knowledge already describes them.
- Scrape templates for sources the renderer does not yet support (kube-state-metrics, control plane, CoreDNS, …).
- Alternative actuators behind the same loop: Prometheus Operator `ScrapeConfig`, Tetragon `TracingPolicy`,
  Pixie tracepoints.
- Alternative collection agents: source-side exporter knobs, Prometheus-native, vmagent.
- One-command install (Helm chart).
