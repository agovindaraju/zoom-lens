#!/usr/bin/env bash
# Step 4: deploy the Zoom-Lens operator (OpAMP server) and the collector
# DaemonSet (OpAMP Supervisor + otelcol-contrib on every node).
. "$(dirname "$0")/lib.sh"

require_cmd kubectl
require_cluster

step "Namespaces"
kubectl apply -f "${ZL_ROOT}/deploy/namespaces.yaml"

step "Knowledge resources (metric trees, relations, entity graph)"
kubectl apply -k "${ZL_ROOT}/deploy/crds"
kubectl wait --for=condition=Established --timeout=60s \
  crd/metrictreepacks.zoomlens.dev crd/metricrelations.zoomlens.dev crd/entitygraphs.zoomlens.dev >/dev/null
kubectl apply -k "${ZL_ROOT}/knowledge" >/dev/null
kubectl apply -f "${ZL_ROOT}/deploy/demo/knowledge-pack.yaml" >/dev/null
ok "$(kubectl get mtp --no-headers | wc -l | tr -d ' ') MetricTreePacks, $(kubectl get mrel --no-headers | wc -l | tr -d ' ') MetricRelations, $(kubectl get eg --no-headers | wc -l | tr -d ' ') EntityGraph"

step "Zoom-Lens operator"
op_hash="$(sha256_of "${ZL_ROOT}/deploy/operator/ladder.yaml" "${ZL_ROOT}/deploy/operator/operator.yaml")"
render_kustomize deploy/operator "${op_hash}" | kubectl apply -f -
kubectl -n "${ZL_NS}" rollout status deployment/zoomlens-operator --timeout="${ZL_WAIT_TIMEOUT}"
ok "operator running (${IMG_OPERATOR})"

step "Collectors (DaemonSet)"
col_hash="$(sha256_of "${ZL_ROOT}/deploy/collector/supervisor.yaml" "${ZL_ROOT}/deploy/collector/base-config.yaml")"
render_kustomize deploy/collector "${col_hash}" | kubectl apply -f -
kubectl -n "${ZL_NS}" rollout status daemonset/zoomlens-collector --timeout="${ZL_WAIT_TIMEOUT}"
ok "collectors running on $(kubectl -n "${ZL_NS}" get ds zoomlens-collector -o jsonpath='{.status.numberReady}') node(s) (${IMG_COLLECTOR})"
