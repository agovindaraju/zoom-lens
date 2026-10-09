#!/usr/bin/env bash
# Step 3: install kube-prometheus-stack (Prometheus + Alertmanager + Grafana +
# node-exporter + kube-state-metrics), the root alert rules and the dashboard.
. "$(dirname "$0")/lib.sh"

require_cmd helm kubectl
require_cluster

step "Helm repository"
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null 2>&1 || true
helm repo update prometheus-community >/dev/null
ok "prometheus-community repo ready"

step "Install kube-prometheus-stack ${ZL_KPS_CHART_VERSION} as '${ZL_KPS_RELEASE}' in '${ZL_MONITORING_NS}'"
helm upgrade --install "${ZL_KPS_RELEASE}" prometheus-community/kube-prometheus-stack \
  --version "${ZL_KPS_CHART_VERSION}" \
  --namespace "${ZL_MONITORING_NS}" --create-namespace \
  -f "${ZL_ROOT}/deploy/monitoring/values.yaml" \
  --wait --timeout "${ZL_WAIT_TIMEOUT}"
ok "kube-prometheus-stack installed"

step "Wait for Prometheus and Alertmanager"
kubectl -n "${ZL_MONITORING_NS}" rollout status statefulset -l app.kubernetes.io/name=prometheus --timeout="${ZL_WAIT_TIMEOUT}" 2>/dev/null \
  || kubectl -n "${ZL_MONITORING_NS}" wait --for=condition=Ready pod -l app.kubernetes.io/name=prometheus --timeout="${ZL_WAIT_TIMEOUT}"
kubectl -n "${ZL_MONITORING_NS}" wait --for=condition=Ready pod -l app.kubernetes.io/name=alertmanager --timeout="${ZL_WAIT_TIMEOUT}"
ok "Prometheus and Alertmanager ready"

step "Apply root alert rules and Grafana dashboard"
kubectl apply -f "${ZL_ROOT}/deploy/monitoring/alert-rules.yaml"
rules="${ZL_ROOT}/knowledge/generated/prometheusrule.yaml"
if [ -n "${ZL_KNOWLEDGE_RULE_FOR}" ]; then
  info "curated rules: every 'for:' set to ${ZL_KNOWLEDGE_RULE_FOR} (ZL_KNOWLEDGE_RULE_FOR)"
  sed -E "s/^([[:space:]]+for:)[[:space:]].*/\1 ${ZL_KNOWLEDGE_RULE_FOR}/" "${rules}" | kubectl apply -f -
else
  kubectl apply -f "${rules}"
fi
kubectl apply -k "${ZL_ROOT}/deploy/grafana"
ok "rules and dashboard applied"
