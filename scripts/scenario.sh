#!/usr/bin/env bash
# Start/stop a demo fault.
#   scenario.sh cpu  start|stop|status   Scenario A: CPU noisy neighbour
#   scenario.sh disk start|stop|status   Scenario B: disk noisy neighbour (+ eBPF at L2)
. "$(dirname "$0")/lib.sh"

usage() { echo "usage: $0 cpu|disk start|stop|status" >&2; exit 2; }
[ $# -eq 2 ] || usage
case "$1" in
  cpu)  job=noisy-cpu;  file=deploy/demo/scenarios/stress-cpu.yaml ;;
  disk) job=noisy-disk; file=deploy/demo/scenarios/stress-disk.yaml ;;
  *) usage ;;
esac
require_cmd kubectl
require_cluster

case "$2" in
  start)
    step "Start scenario '$1' (${job})"
    kubectl -n "${ZL_DEMO_NS}" delete job "${job}" --ignore-not-found --wait=true >/dev/null
    # Size the noisy neighbour to the demo node, so it really contends.
    node="$(kubectl get nodes -l zoomlens.dev/demo-node=true -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
    cpus="$(kubectl get node "${node}" -o jsonpath='{.status.capacity.cpu}' 2>/dev/null || echo 2)"
    case "${cpus}" in *m) cpus=$(( ${cpus%m} / 1000 )) ;; esac
    [ "${cpus}" -ge 1 ] 2>/dev/null || cpus=2
    if [ "$1" = cpu ]; then workers="${ZL_STRESS_CPU_WORKERS:-$(( cpus * 2 ))}"; else workers="${ZL_STRESS_DISK_WORKERS:-4}"; fi
    info "noisy neighbour: ${workers} workers on ${node:-?} (${cpus} CPUs)"
    render_file "${file}" | sed "s/-workers=4/-workers=${workers}/" | kubectl apply -f -
    pod_exists() { [ -n "$(kubectl -n "${ZL_DEMO_NS}" get pods -l "app.kubernetes.io/name=${job}" -o name 2>/dev/null)" ]; }
    retry 30 2 pod_exists || die "the ${job} pod was not created (kubectl -n ${ZL_DEMO_NS} describe job ${job})"
    kubectl -n "${ZL_DEMO_NS}" wait --for=condition=Ready pod -l "app.kubernetes.io/name=${job}" --timeout=180s
    kubectl -n "${ZL_DEMO_NS}" get pods -o wide | sed 's/^/    /'
    ok "fault running on the demo node"
    info "Expected (demo ladder in deploy/operator/ladder.yaml):"
    info "  ~1-2 min  alert fires  -> L1: children @10s on the demo node"
    info "  +3 min    still firing -> L2: depth 2 @5s, host scrapers, eBPF pod (disk scenario)"
    info "  +6 min    still firing -> L3: 2 hops, co-located pods (the noisy neighbour)"
    info "Watch: ./scripts/status.sh --watch    Grafana: make grafana"
    ;;
  stop)
    step "Stop scenario '$1'"
    kubectl -n "${ZL_DEMO_NS}" delete job "${job}" --ignore-not-found --wait=true
    ok "fault removed"
    info "Alert resolves within ~1-2 min, then collection reverts after clearFor (3 min in the demo)."
    ;;
  status)
    kubectl -n "${ZL_DEMO_NS}" get job "${job}" 2>/dev/null || info "scenario '$1' not running"
    ;;
  *) usage ;;
esac
