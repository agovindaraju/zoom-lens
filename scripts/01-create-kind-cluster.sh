#!/usr/bin/env bash
# Optional step 1: create a local kind cluster (1 control-plane + 2 workers).
# Skip this if you already have a cluster.
. "$(dirname "$0")/lib.sh"

require_cmd kind docker kubectl

step "Create kind cluster '${ZL_KIND_CLUSTER}'"
if kind get clusters 2>/dev/null | grep -qx "${ZL_KIND_CLUSTER}"; then
  ok "cluster already exists"
else
  kind create cluster --name "${ZL_KIND_CLUSTER}" --config "${ZL_ROOT}/deploy/kind/cluster.yaml" --wait 180s
  ok "cluster created"
fi
kubectl config use-context "kind-${ZL_KIND_CLUSTER}" >/dev/null
kubectl get nodes -o wide | sed 's/^/    /'
