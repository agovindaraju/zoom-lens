#!/usr/bin/env bash
# Step 0: check tools, Docker and cluster access before changing anything.
#   --no-cluster   skip the cluster checks (e.g. before creating a kind cluster)
. "$(dirname "$0")/lib.sh"

check_cluster=1
[ "${1:-}" = "--no-cluster" ] && check_cluster=0

step "Preflight: tools"
require_cmd docker kubectl helm curl sed
ok "docker, kubectl, helm, curl, sed found"
info "kubectl: $(kubectl version --client 2>/dev/null | head -1)"
info "helm:    $(helm version --short 2>/dev/null)"
if command -v kind >/dev/null 2>&1; then info "kind:    $(kind version 2>/dev/null)"; else info "kind:    not installed (only needed for a local kind cluster)"; fi

step "Preflight: Docker daemon"
docker info >/dev/null 2>&1 || die "Docker daemon is not reachable. Start Docker Desktop / Rancher Desktop / dockerd and retry."
ok "Docker daemon reachable ($(docker info --format '{{.OSType}}/{{.Architecture}}' 2>/dev/null))"

step "Preflight: settings"
info "images: ${IMG_OPERATOR}"
info "        ${IMG_DEMO}"
info "        ${IMG_COLLECTOR}"
info "OpenTelemetry ${ZL_OTEL_VERSION}, ebpf_exporter ${ZL_EBPF_IMAGE}, kube-prometheus-stack ${ZL_KPS_CHART_VERSION}"

if [ "${check_cluster}" = 1 ]; then
  step "Preflight: cluster"
  require_cluster
  ok "cluster reachable, context: $(kubectl config current-context)"
  nodes="$(kubectl get nodes --no-headers 2>/dev/null | wc -l | tr -d ' ')"
  info "nodes: ${nodes}"
  kubectl get nodes -o wide | sed 's/^/    /'
  if is_kind_cluster; then
    ok "kind cluster detected ($(kind_cluster_name)); images will be loaded with 'kind load'"
    command -v kind >/dev/null 2>&1 || die "this is a kind cluster but the kind CLI is not installed"
    warn "kind nodes share one kernel: node-level and eBPF metrics look the same on every node"
  elif uses_local_docker; then
    ok "cluster uses this Docker daemon as its runtime (e.g. Rancher Desktop / moby): built images are used directly"
  elif [ -z "${ZL_REGISTRY}" ]; then
    die "not a kind cluster: set ZL_REGISTRY to a registry the cluster can pull from (see docs/DEPLOYMENT.md)"
  else
    ok "images will be pushed to ${ZL_REGISTRY}"
  fi
  kubectl get storageclass 2>/dev/null | grep -q '(default)' \
    || warn "no default StorageClass: the demo PVC will stay Pending (kind ships one; managed clusters usually do)"
  if [ "${nodes}" -lt 2 ]; then
    warn "single-node cluster: the demo works, but 'expand OUT to other nodes' is not visible"
  fi
fi

ok "preflight passed"
