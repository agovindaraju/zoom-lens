# Zoom-Lens Demo Guide

The demo is an optional workload that shows Zoom-Lens working: a small app with CPU-bound
and disk-bound endpoints, a load generator, and two "noisy neighbour" faults that trigger
the dwell ladder. Everything demo-only lives under [`demo/`](..).

Deploy Zoom-Lens first (steps 0–4 and 6 of the [Deployment Guide](../../docs/DEPLOYMENT.md);
`scripts/deploy-all.sh` also runs the demo step for you). A recorded run with measured
timings is in [`QUICKSTART.md`](QUICKSTART.md).

## Layout

| Path | Contents |
|---|---|
| `demo/app/` | Demo workload binary: `server`, `load`, `stress-cpu`, `stress-disk` modes |
| `demo/deploy/` | App, load generator, noisy-neighbour Jobs, the demo knowledge pack |
| `demo/monitoring/alert-rules.yaml` | Root alert rules calibrated for the demo app |
| `demo/grafana/` | Dashboard "Zoom-Lens: expand-on-signal metrics" |
| `demo/scripts/` | `deploy-demo.sh`, `scenario.sh` |
| `demo/docs/` | This guide and the quickstart |

## What gets deployed

| Component | Namespace | Image | Role |
|---|---|---|---|
| Demo app + PVC | `zoomlens-demo` | built here: `zoomlens-demo-app` | CPU-bound and disk-bound endpoints, Prometheus `/metrics` |
| Load generator | `zoomlens-demo` | same image, `-mode=load` | Constant traffic |
| Noisy neighbour Jobs | `zoomlens-demo` | same image, `-mode=stress-cpu` / `stress-disk` | Started by `demo/scripts/scenario.sh` |

Image: `zoomlens-demo-app:<tag>` is built by `scripts/02-build-images.sh`
(`build/go.Dockerfile --build-arg PKG=./demo/app`). Manual equivalent:

```bash
docker build -f build/go.Dockerfile --build-arg PKG=./demo/app -t zoomlens-demo-app:dev .
kind load docker-image zoomlens-demo-app:dev --name zoomlens
```

Setting: `ZL_DEMO_NODE` (empty = first worker node) selects the node for the demo app.

## Deploy the demo workload

```bash
./demo/scripts/deploy-demo.sh           # make demo
```

Labels the demo node `zoomlens.dev/demo-node=true` (first worker, or `ZL_DEMO_NODE`), then
deploys the app (with PVC) and the load generator.

**Check:** `kubectl -n zoomlens-demo get pods -o wide`: `demo-app` and `demo-load` are
Running, and `demo-app` is on the demo node.

---

## Running the demo scenarios

Open two terminals: `make grafana` (then http://localhost:3000, `admin` / `zoomlens`,
dashboard **Zoom-Lens: expand-on-signal metrics**) and `./scripts/status.sh --watch`.

### Scenario A — CPU noisy neighbour

```bash
./demo/scripts/scenario.sh cpu start       # make cpu-start
```

Starts `noisy-cpu` (4 busy workers, **no CPU limit**) on the demo app's node.

| Time after start | What happens | Where to see it |
|---|---|---|
| 0–2 min | App p99 rises; after `for: 1m` the alert fires | dashboard "Demo app p99 latency"; Prometheus alerts |
| alert + ~10 s | **L1**: overlay pushed to the demo node only; CFS throttling, RSS, node CPU pressure, disk write time at **10 s** | `status.sh`: incident L1, overlay on one node; dashboard "Active series by collection job" shows `zl-*` jobs |
| alert + 3 min | **L2**: depth 2 at **5 s**; host scrapers `paging`, `processes`; `ebpf_exporter` pod with `biolatency` on that node | "Scrapes per minute" doubles; "eBPF exporter pods" = 1 |
| alert + 6 min | **L3**: 2 hops → co-located pods included → `noisy-cpu` visible | "Container CPU throttling" / container metrics for other pods |

```bash
./demo/scripts/scenario.sh cpu stop        # make cpu-stop
```

About 1–2 minutes later the alert resolves. After `clearFor` (3 min in the demo) the
operator pushes the no-op overlay and deletes the eBPF pod. `zl-*` series stop.

### Scenario B — disk noisy neighbour (eBPF at L2)

```bash
./demo/scripts/scenario.sh disk start      # make disk-start
./demo/scripts/scenario.sh disk stop       # make disk-stop
```

`noisy-disk` writes and fsyncs 1 MiB blocks on the same node. The app's `/work/disk`
latency rises, and `ZoomLensNodeDiskBusy` may fire too; both incidents are merged per node.
At L2 the `biolatency` eBPF program runs on that node only ("eBPF series by metric" panel).

> The demo ladder is shortened (L2 at 3 min, L3 at 6 min, `clearFor` 3 min). Change it in
> [`deploy/operator/ladder.yaml`](../../deploy/operator/ladder.yaml) and re-run step 4.

---

## Troubleshooting (demo)

| Symptom | Likely cause | Fix |
|---|---|---|
| `demo-app` Pending | no default StorageClass, or no node labelled `zoomlens.dev/demo-node=true` | create a default StorageClass; re-run step 5 (or set `ZL_DEMO_NODE`) |
| Alert never fires | threshold too high for your hardware | lower thresholds in `demo/monitoring/alert-rules.yaml`, `kubectl apply -f` it |

General troubleshooting: [Deployment Guide](../../docs/DEPLOYMENT.md#8-troubleshooting).
