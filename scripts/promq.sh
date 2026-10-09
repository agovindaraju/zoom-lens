#!/usr/bin/env bash
# Run PromQL queries against the in-cluster Prometheus and print one line per
# series: "<value>  {labels}".
#   ./scripts/promq.sh 'ALERTS{alertname=~"ZoomLens.*"}' 'count by (job) ({job=~"zl-.*"})'
. "$(dirname "$0")/lib.sh"
require_cmd kubectl curl python3
[ $# -ge 1 ] || die "usage: $0 <promql> [<promql>...]"
port_forward "${ZL_MONITORING_NS}" prometheus-operated 9090
for q in "$@"; do
  echo "== $q"
  prom_query "${PF_PORT}" "$q" | python3 -c '
import json, sys
d = json.load(sys.stdin)
if d.get("status") != "success":
    print("  ERROR", d.get("error")); sys.exit()
r = d["data"]["result"]
if not r:
    print("  (no data)")
for x in r:
    m = {k: v for k, v in x["metric"].items() if k not in ("otel_scope_name", "otel_scope_version")}
    print("  %-14s %s" % (x["value"][1], m))'
done
