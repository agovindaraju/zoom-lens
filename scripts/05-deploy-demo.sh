#!/usr/bin/env bash
# Step 5: deploy the demo app (with PVC) and its load generator on one node.
. "$(dirname "$0")/lib.sh"

require_cmd kubectl
require_cluster

step "Pick the demo node"
node="${ZL_DEMO_NODE}"
if [ -z "${node}" ]; then
  node="$(kubectl get nodes -l zoomlens.dev/demo-node=true -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
fi
if [ -z "${node}" ]; then
  node="$(kubectl get nodes -l '!node-role.kubernetes.io/control-plane' -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
fi
if [ -z "${node}" ]; then
  node="$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')"
  warn "no worker node found, using ${node}"
fi
kubectl label node "${node}" zoomlens.dev/demo-node=true --overwrite >/dev/null
ok "demo node: ${node}"

step "Demo app and load generator"
kubectl apply -f "${ZL_ROOT}/deploy/namespaces.yaml" >/dev/null
render_kustomize deploy/demo | kubectl apply -f -
kubectl -n "${ZL_DEMO_NS}" rollout status deployment/demo-app --timeout="${ZL_WAIT_TIMEOUT}"
kubectl -n "${ZL_DEMO_NS}" rollout status deployment/demo-load --timeout="${ZL_WAIT_TIMEOUT}"
ok "demo app and load generator running"
kubectl -n "${ZL_DEMO_NS}" get pods -o wide | sed 's/^/    /'
