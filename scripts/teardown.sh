#!/usr/bin/env bash
# Remove everything the demo installed.
#   teardown.sh               remove demo, Zoom-Lens and the monitoring stack
#   teardown.sh --keep-monitoring   keep kube-prometheus-stack
#   teardown.sh --delete-kind       also delete the kind cluster
. "$(dirname "$0")/lib.sh"

keep_monitoring=0
delete_kind=0
for a in "$@"; do
  case "$a" in
    --keep-monitoring) keep_monitoring=1 ;;
    --delete-kind) delete_kind=1 ;;
    *) die "unknown option $a" ;;
  esac
done
require_cmd kubectl

if [ "${delete_kind}" = 1 ]; then
  require_cmd kind
  step "Delete kind cluster '${ZL_KIND_CLUSTER}'"
  kind delete cluster --name "${ZL_KIND_CLUSTER}"
  ok "deleted"
  exit 0
fi

require_cluster
step "Delete demo and Zoom-Lens"
kubectl delete namespace "${ZL_DEMO_NS}" --ignore-not-found --wait=true
kubectl delete namespace "${ZL_NS}" --ignore-not-found --wait=true
kubectl delete clusterrole,clusterrolebinding zoomlens-operator zoomlens-collector --ignore-not-found
kubectl delete -f "${ZL_ROOT}/demo/deploy/knowledge-pack.yaml" --ignore-not-found >/dev/null 2>&1 || true
kubectl delete -k "${ZL_ROOT}/knowledge" --ignore-not-found >/dev/null 2>&1 || true
kubectl label nodes --all zoomlens.dev/demo-node- >/dev/null 2>&1 || true
ok "demo and Zoom-Lens removed"

if [ "${keep_monitoring}" = 0 ]; then
  require_cmd helm
  step "Delete kube-prometheus-stack"
  kubectl delete -f "${ZL_ROOT}/demo/monitoring/alert-rules.yaml" --ignore-not-found
  kubectl delete -f "${ZL_ROOT}/knowledge/generated/prometheusrule.yaml" --ignore-not-found
  kubectl delete -k "${ZL_ROOT}/demo/grafana" --ignore-not-found
  helm uninstall "${ZL_KPS_RELEASE}" -n "${ZL_MONITORING_NS}" --ignore-not-found --wait || true
  kubectl delete namespace "${ZL_MONITORING_NS}" --ignore-not-found --wait=true
  info "kube-prometheus-stack CRDs are left in place (helm does not delete CRDs)."
  ok "monitoring removed"
fi
