#!/usr/bin/env bash
# Step 2: build the three images and make them available to the cluster
# (kind load, or push to ZL_REGISTRY).
#   --no-publish   build only
. "$(dirname "$0")/lib.sh"

publish=1
[ "${1:-}" = "--no-publish" ] && publish=0
require_cmd docker

platform_args=""
[ -n "${ZL_PLATFORM}" ] && platform_args="--platform ${ZL_PLATFORM}"
version="$(git -C "${ZL_ROOT}" describe --tags --always --dirty 2>/dev/null || echo "${ZL_IMAGE_TAG}")"

build() {
  local image="$1"; shift
  step "Build ${image}"
  # shellcheck disable=SC2086
  DOCKER_BUILDKIT=1 docker build ${platform_args} -t "${image}" "$@" "${ZL_ROOT}"
  ok "built ${image}"
}

build "${IMG_OPERATOR}" -f "${ZL_ROOT}/build/go.Dockerfile" \
  --build-arg PKG=./cmd/zoomlens --build-arg VERSION="${version}" --build-arg GOPROXY="${ZL_GOPROXY}"
build "${IMG_DEMO}" -f "${ZL_ROOT}/build/go.Dockerfile" \
  --build-arg PKG=./demo/app --build-arg VERSION="${version}" --build-arg GOPROXY="${ZL_GOPROXY}"
build "${IMG_COLLECTOR}" -f "${ZL_ROOT}/build/collector.Dockerfile" \
  --build-arg OTEL_VERSION="${ZL_OTEL_VERSION}"

[ "${publish}" = 1 ] || { ok "build only (--no-publish)"; exit 0; }

require_cluster
if is_kind_cluster; then
  require_cmd kind
  name="$(kind_cluster_name)"
  step "Load images into kind cluster '${name}'"
  for img in "${IMG_OPERATOR}" "${IMG_DEMO}" "${IMG_COLLECTOR}"; do
    kind load docker-image "${img}" --name "${name}"
    ok "loaded ${img}"
  done
  step "Pre-load ${ZL_EBPF_IMAGE} (optional, speeds up the first eBPF escalation)"
  if docker pull ${platform_args} "${ZL_EBPF_IMAGE}" >/dev/null && kind load docker-image "${ZL_EBPF_IMAGE}" --name "${name}"; then
    ok "loaded ${ZL_EBPF_IMAGE}"
  else
    warn "could not pre-load ${ZL_EBPF_IMAGE}; nodes will pull it on demand"
  fi
elif uses_local_docker; then
  step "Cluster shares this Docker daemon: images are already available to the kubelet"
  if docker pull ${platform_args} "${ZL_EBPF_IMAGE}" >/dev/null; then ok "pre-pulled ${ZL_EBPF_IMAGE}"; else warn "could not pre-pull ${ZL_EBPF_IMAGE}"; fi
elif [ -n "${ZL_REGISTRY}" ]; then
  step "Push images to ${ZL_REGISTRY}"
  for img in "${IMG_OPERATOR}" "${IMG_DEMO}" "${IMG_COLLECTOR}"; do
    docker push "${img}"
    ok "pushed ${img}"
  done
else
  die "not kind, not a shared Docker runtime, and ZL_REGISTRY is empty: nowhere to publish images"
fi
