#!/usr/bin/env bash
# Shared helpers for the Zoom-Lens scripts. Source it; do not execute it.
# Compatible with bash 3.2 (macOS default).

set -euo pipefail

ZL_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=../config/zoomlens.env
. "${ZL_ROOT}/config/zoomlens.env"

ZL_NS="zoomlens"
ZL_DEMO_NS="zoomlens-demo"

_prefix() { if [ -n "${ZL_REGISTRY}" ]; then printf '%s/' "${ZL_REGISTRY%/}"; fi; }
IMG_OPERATOR="$(_prefix)zoomlens-operator:${ZL_IMAGE_TAG}"
IMG_DEMO="$(_prefix)zoomlens-demo-app:${ZL_IMAGE_TAG}"
IMG_COLLECTOR="$(_prefix)zoomlens-collector:${ZL_IMAGE_TAG}"

if [ -t 1 ]; then
  _c_blue=$'\033[34m'; _c_green=$'\033[32m'; _c_yellow=$'\033[33m'; _c_red=$'\033[31m'; _c_off=$'\033[0m'
else
  _c_blue=""; _c_green=""; _c_yellow=""; _c_red=""; _c_off=""
fi

step() { printf '\n%s==> %s%s\n' "${_c_blue}" "$*" "${_c_off}"; }
info() { printf '    %s\n' "$*"; }
ok()   { printf '    %s✔ %s%s\n' "${_c_green}" "$*" "${_c_off}"; }
warn() { printf '    %s! %s%s\n' "${_c_yellow}" "$*" "${_c_off}" >&2; }
die()  { printf '%s✘ %s%s\n' "${_c_red}" "$*" "${_c_off}" >&2; exit 1; }

require_cmd() {
  local c
  for c in "$@"; do
    command -v "$c" >/dev/null 2>&1 || die "'$c' is required but not installed (see docs/DEPLOYMENT.md, Prerequisites)"
  done
}

require_cluster() {
  kubectl version --request-timeout=10s >/dev/null 2>&1 \
    || die "cannot reach a Kubernetes cluster with the current kubeconfig context ($(kubectl config current-context 2>/dev/null || echo none))"
}

# True when the current cluster is a kind cluster.
is_kind_cluster() {
  local pid
  pid="$(kubectl get nodes -o jsonpath='{.items[0].spec.providerID}' 2>/dev/null || true)"
  case "${pid}" in kind://*) return 0 ;; esac
  return 1
}

# True when the cluster runs its containers in the same Docker daemon we
# build with (Rancher Desktop with the dockerd/moby runtime, Docker Desktop
# Kubernetes): built images are visible to the kubelet without load/push.
uses_local_docker() {
  local rt dn
  rt="$(kubectl get nodes -o jsonpath='{.items[0].status.nodeInfo.containerRuntimeVersion}' 2>/dev/null || true)"
  case "${rt}" in docker://*) ;; *) return 1 ;; esac
  dn="$(docker info --format '{{.Name}}' 2>/dev/null || true)"
  [ -n "${dn}" ] && kubectl get node "${dn}" >/dev/null 2>&1
}

# kind cluster name for the current context (kind-<name>), else the config value.
kind_cluster_name() {
  local ctx
  ctx="$(kubectl config current-context 2>/dev/null || true)"
  case "${ctx}" in kind-*) printf '%s' "${ctx#kind-}" ;; *) printf '%s' "${ZL_KIND_CLUSTER}" ;; esac
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then cat "$@" | sha256sum | cut -c1-16
  else cat "$@" | shasum -a 256 | cut -c1-16; fi
}

# render_kustomize <dir>: kustomize output with image and placeholder substitution.
render_kustomize() {
  local dir="$1" hash="${2:-none}"
  kubectl kustomize "${ZL_ROOT}/${dir}" | sed \
    -e "s#image: zoomlens-operator:dev#image: ${IMG_OPERATOR}#g" \
    -e "s#image: zoomlens-demo-app:dev#image: ${IMG_DEMO}#g" \
    -e "s#image: zoomlens-collector:dev#image: ${IMG_COLLECTOR}#g" \
    -e "s#EBPF_IMAGE_PLACEHOLDER#${ZL_EBPF_IMAGE}#g" \
    -e "s#CONFIG_HASH_PLACEHOLDER#${hash}#g"
}

# render_file <path>: same substitutions for a plain manifest.
render_file() {
  sed -e "s#image: zoomlens-demo-app:dev#image: ${IMG_DEMO}#g" "${ZL_ROOT}/$1"
}

# port_forward <ns> <svc> <remotePort>: starts a port-forward in the
# background and sets PF_PORT to the local port. Cleaned up on exit.
# Call it directly (not in $(...)): a subshell would kill the forward.
_PF_PIDS=""
PF_PORT=""
port_forward() {
  local ns="$1" svc="$2" rport="$3" lport i
  lport=$(( 20000 + (RANDOM % 20000) ))
  kubectl -n "${ns}" port-forward "svc/${svc}" "${lport}:${rport}" >/dev/null 2>&1 &
  _PF_PIDS="${_PF_PIDS} $!"
  trap '_pf_cleanup' EXIT
  for i in $(seq 1 30); do
    if curl -s -o /dev/null "http://127.0.0.1:${lport}/" 2>/dev/null; then PF_PORT="${lport}"; return 0; fi
    sleep 1
  done
  die "port-forward to ${ns}/${svc}:${rport} did not become ready"
}
_pf_cleanup() {
  local p
  for p in ${_PF_PIDS}; do kill "${p}" >/dev/null 2>&1 || true; done
  # Optional hook: a script may define zl_on_exit (e.g. to stop a screen recording).
  if declare -F zl_on_exit >/dev/null 2>&1; then zl_on_exit; fi
}

# prom_query <localPort> <promql>: prints the raw JSON response.
prom_query() {
  curl -s --get --data-urlencode "query=$2" "http://127.0.0.1:$1/api/v1/query"
}

# prom_has_data <localPort> <promql>: true if the query returns >= 1 series.
prom_has_data() {
  local out
  out="$(prom_query "$1" "$2")"
  case "${out}" in *'"result":[]'*|'') return 1 ;; *'"status":"success"'*) return 0 ;; esac
  return 1
}

# retry <attempts> <sleepSeconds> <cmd...>
retry() {
  local n="$1" s="$2" i
  shift 2
  for i in $(seq 1 "${n}"); do
    if "$@"; then return 0; fi
    sleep "${s}"
  done
  return 1
}
