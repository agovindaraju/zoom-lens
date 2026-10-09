#!/usr/bin/env bash
# Zoom-Lens operator events in local time.
#   ./scripts/oplog.sh [since] [regex]
#   ./scripts/oplog.sh 10m 'incident|pushed remote config|remote config status|ebpf'
. "$(dirname "$0")/lib.sh"
require_cmd kubectl python3
since="${1:-10m}"
filter="${2:-incident|pushed remote config|remote config status|ebpf|agent identified}"
kubectl -n "${ZL_NS}" logs deploy/zoomlens-operator --since="${since}" | grep -E "${filter}" | python3 -c '
import datetime, json, sys
KEYS = ("alert", "entity", "from", "to", "depth", "hops", "node", "status", "pod", "programs", "incident")
for line in sys.stdin:
    try:
        d = json.loads(line)
    except ValueError:
        continue
    t = datetime.datetime.strptime(d["time"][:19] + "+0000", "%Y-%m-%dT%H:%M:%S%z").astimezone().strftime("%H:%M:%S")
    extra = {k: (str(d[k]).replace("RemoteConfigStatuses_", "")) for k in KEYS if k in d}
    print(t, d["msg"], " ".join(f"{k}={v}" for k, v in extra.items()))'
