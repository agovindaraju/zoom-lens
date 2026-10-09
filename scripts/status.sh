#!/usr/bin/env bash
# Show what Zoom-Lens is doing right now.
#   status.sh           one snapshot
#   status.sh --watch   refresh every 15s (Ctrl-C to stop)
#   status.sh --raw     print the operator's /debug/state JSON
. "$(dirname "$0")/lib.sh"

require_cmd kubectl curl
require_cluster
mode="${1:-}"

port_forward "${ZL_NS}" zoomlens 8080
OP="${PF_PORT}"
port_forward "${ZL_MONITORING_NS}" prometheus-operated 9090
PROM="${PF_PORT}"

value_of() { # first sample value of a PromQL query, or "-"
  prom_query "${PROM}" "$1" | sed -n 's/.*"value":\[[^,]*,"\([^"]*\)".*/\1/p' | head -1 | grep . || echo "-"
}

snapshot() {
  local state
  state="$(curl -s "http://127.0.0.1:${OP}/debug/state")"
  if [ "${mode}" = "--raw" ]; then printf '%s\n' "${state}"; return; fi
  printf '%s\n' "==== Zoom-Lens status  $(date '+%H:%M:%S') ===="
  if command -v python3 >/dev/null 2>&1; then
    printf '%s' "${state}" | python3 -c '
import json, sys
s = json.load(sys.stdin)
inc = s.get("incidents") or []
print("Incidents: %d" % len(inc))
for i in inc:
    e = i["entity"]
    print("  %-8s %-24s L%d  %-8s entity=%s/%s  since=%s%s" % (
        i["id"], i["alertName"], i["level"]["index"], "cooldown" if i.get("resolvedAt") else "firing",
        e["kind"], e["name"], __import__("datetime").datetime.strptime(i["startsAt"][:19] + "+0000", "%Y-%m-%dT%H:%M:%S%z").astimezone().strftime("%H:%M:%S"), ("  ERROR: " + i["lastError"]) if i.get("lastError") else ""))
ov = s.get("overlays") or {}
print("Overlays: %d node(s)" % len(ov))
for n, o in sorted(ov.items()):
    m = o.get("metrics") or {}
    print("  %-28s every %-5s metrics=%d pods=%d host=%s ebpf=%s" % (
        n, str(o.get("interval", 0) // 10**9) + "s", sum(len(v) for v in m.values()), len(o.get("pods") or []),
        ",".join(o.get("hostScrapers") or []) or "-", ",".join(o.get("ebpfPrograms") or []) or "-"))
pods = s.get("ebpfPods") or []
print("eBPF pods: %d" % len(pods))
for p in pods:
    print("  %-22s node=%-24s programs=%s phase=%s" % (p["name"], p["node"], ",".join(p["programs"]), p["phase"]))
ag = s.get("agents") or []
print("OpAMP agents: %d" % len(ag))
for a in ag:
    print("  %-28s config=%-10s inSync=%s %s" % (a["node"] or "?", a["configStatus"].replace("RemoteConfigStatuses_", ""), a["inSync"], a.get("configError") or ""))
for k, v in (s.get("lastErrors") or {}).items():
    print("Last %s error: %s" % (k, v))
'
  else
    printf '%s\n' "${state}"
  fi
  printf 'Prometheus: head series=%s  overlay series written in last 30s (zl-*)=%s  baseline series (base-*)=%s\n' \
    "$(value_of 'sum(prometheus_tsdb_head_series)')" \
    "$(value_of 'count(last_over_time({job=~"zl-.*"}[30s])) or vector(0)')" \
    "$(value_of 'count({job=~"base-.*"})')"
  printf 'Demo app p99: %ss\n' "$(value_of 'max(histogram_quantile(0.99, sum by (le, pod) (rate(http_server_request_duration_seconds_bucket{namespace="zoomlens-demo"}[2m]))))')"
}

if [ "${mode}" = "--watch" ]; then
  while true; do clear 2>/dev/null || true; snapshot; sleep 15; done
else
  snapshot
fi
