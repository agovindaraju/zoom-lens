#!/usr/bin/env bash
# Step 6: verify the steady state (L0) end to end:
#   operator ready -> every collector connected over OpAMP -> baseline (root)
#   metrics in Prometheus -> alert rules loaded -> no overlay active.
. "$(dirname "$0")/lib.sh"

require_cmd kubectl curl
require_cluster
failed=0
check() { if "$@"; then ok "${CHECK_MSG}"; else warn "FAILED: ${CHECK_MSG}"; failed=1; fi; }

step "Pods"
kubectl -n "${ZL_NS}" get pods -o wide | sed 's/^/    /'
kubectl -n "${ZL_DEMO_NS}" get pods -o wide | sed 's/^/    /'

step "Operator and OpAMP agents"
port_forward "${ZL_NS}" zoomlens 8080
OP="${PF_PORT}"
nodes="$(kubectl get nodes --no-headers | wc -l | tr -d ' ')"
agents_connected() {
  local n
  n="$(curl -s "http://127.0.0.1:${OP}/debug/state" | grep -c '"instanceUid"' || true)"
  [ "${n}" -ge "${nodes}" ]
}
CHECK_MSG="all ${nodes} collectors connected to the OpAMP server"; check retry 24 5 agents_connected
agents_in_sync() { ! curl -s "http://127.0.0.1:${OP}/debug/state" | grep -q '"inSync": false'; }
CHECK_MSG="all collectors report the current remote config"; check retry 24 5 agents_in_sync
info "agents:"
curl -s "http://127.0.0.1:${OP}/debug/state" | grep -E '"node"|"configStatus"|"inSync"' | sed 's/^ */      /' | head -30

step "Baseline metrics in Prometheus"
port_forward "${ZL_MONITORING_NS}" prometheus-operated 9090
PROM="${PF_PORT}"
for q in 'container_cpu_usage_seconds_total{job="base-cadvisor"}' \
         'node_cpu_seconds_total{job="base-node-exporter"}' \
         'http_server_request_duration_seconds_bucket{job="base-app"}' \
         'count({job="hostmetrics"})' \
         'zoomlens_opamp_agents_connected'; do
  CHECK_MSG="has data: ${q}"; check retry 24 5 prom_has_data "${PROM}" "${q}"
done
no_overlay() { ! prom_has_data "${PROM}" 'count({job=~"zl-.*"})'; }
CHECK_MSG="no Zoom-Lens overlay series (steady state; fails if an incident is already active)"; check no_overlay

step "Alert rules"
rules_loaded() { curl -s "http://127.0.0.1:${PROM}/api/v1/rules" | grep -q 'ZoomLensAppLatencyHigh'; }
CHECK_MSG="Zoom-Lens alert rules loaded"; check retry 12 5 rules_loaded

if [ "${failed}" = 0 ]; then
  ok "steady state verified. Next: ./demo/scripts/scenario.sh cpu start   (see demo/docs/DEMO.md)"
else
  die "verification failed; see docs/DEPLOYMENT.md, Troubleshooting"
fi
