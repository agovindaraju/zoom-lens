#!/usr/bin/env bash
# Deploy everything in one go: preflight, (optional kind cluster), images,
# monitoring stack, Zoom-Lens, demo app, verification.
#
#   ./scripts/deploy-all.sh                use the current kubectl context
#   ./scripts/deploy-all.sh --create-kind  create a local kind cluster first
#   ./scripts/deploy-all.sh --skip-build   reuse images already built/published
. "$(dirname "$0")/lib.sh"

create_kind=0
skip_build=0
for a in "$@"; do
  case "$a" in
    --create-kind) create_kind=1 ;;
    --skip-build)  skip_build=1 ;;
    -h|--help) sed -n '2,8p' "$0"; exit 0 ;;
    *) die "unknown option $a" ;;
  esac
done

S="${ZL_ROOT}/scripts"
started=$(date +%s)

if [ "${create_kind}" = 1 ]; then
  "${S}/00-preflight.sh" --no-cluster
  "${S}/01-create-kind-cluster.sh"
fi
"${S}/00-preflight.sh"
if [ "${skip_build}" = 0 ]; then "${S}/02-build-images.sh"; fi
"${S}/03-install-monitoring.sh"
"${S}/04-deploy-zoomlens.sh"
"${S}/05-deploy-demo.sh"
"${S}/06-verify.sh"

step "Done in $(( $(date +%s) - started ))s"
info "Grafana:   make grafana      (admin / zoomlens)  -> dashboard 'Zoom-Lens: expand-on-signal metrics'"
info "Status:    ./scripts/status.sh --watch"
info "Scenario:  ./scripts/scenario.sh cpu start    (stop: ./scripts/scenario.sh cpu stop)"
