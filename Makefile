# Zoom-Lens demo. Run `make help` for targets.
SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

GO        ?= go
GOFLAGS   ?=
export CGO_ENABLED ?= 0
MONITORING_NS ?= monitoring
KPS_RELEASE   ?= kps

.PHONY: help
help: ## Show targets
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

##@ Development
.PHONY: fmt vet test build check
fmt: ## Format Go code
	gofmt -w cmd internal

vet: ## go vet + gofmt check
	@test -z "$$(gofmt -l cmd internal)" || { echo "gofmt needed:"; gofmt -l cmd internal; exit 1; }
	$(GO) vet ./...

test: ## Run unit tests
	$(GO) test $(GOFLAGS) ./...

build: ## Build binaries into bin/
	$(GO) build -trimpath -o bin/zoomlens ./cmd/zoomlens
	$(GO) build -trimpath -o bin/demo-app ./demo/app

CONTROLLER_GEN ?= $(GO) run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.19.0
.PHONY: knowledge crds
knowledge: ## Validate knowledge/ (packs, relations, entity graph, rules); regenerate knowledge/generated
	cd tools/knowledge-gen && $(GO) run . all ../../knowledge

crds: ## Regenerate deepcopy code and deploy/crds from api/v1alpha1
	$(CONTROLLER_GEN) object crd paths=./api/v1alpha1 output:crd:artifacts:config=deploy/crds

check: vet test ## vet + tests + render all manifests
	@for d in deploy/collector deploy/operator demo/deploy demo/grafana; do kubectl kustomize $$d >/dev/null && echo "$$d renders"; done
	@for f in scripts/*.sh demo/scripts/*.sh; do bash -n $$f; done && echo "scripts: syntax ok"

##@ Deployment (see docs/DEPLOYMENT.md)
.PHONY: preflight kind images monitoring zoomlens demo verify deploy deploy-kind
preflight: ## Step 0: check tools and cluster
	./scripts/00-preflight.sh
kind: ## Step 1 (optional): create a local kind cluster
	./scripts/01-create-kind-cluster.sh
images: ## Step 2: build images and load/push them
	./scripts/02-build-images.sh
monitoring: ## Step 3: install kube-prometheus-stack, alert rules, dashboard
	./scripts/03-install-monitoring.sh
zoomlens: ## Step 4: deploy operator + collectors
	./scripts/04-deploy-zoomlens.sh
demo: ## Step 5: deploy demo app + load generator
	./demo/scripts/deploy-demo.sh
verify: ## Step 6: verify the steady state
	./scripts/06-verify.sh
deploy: ## All steps on the current kubectl context
	./scripts/deploy-all.sh
deploy-kind: ## All steps, creating a kind cluster first
	./scripts/deploy-all.sh --create-kind

##@ Demo
.PHONY: status watch cpu-start cpu-stop disk-start disk-stop grafana prometheus
status: ## One snapshot of incidents, overlays, eBPF pods, agents
	./scripts/status.sh
watch: ## Live status, refreshed every 15s
	./scripts/status.sh --watch
cpu-start: ## Scenario A: start CPU noisy neighbour
	./demo/scripts/scenario.sh cpu start
cpu-stop: ## Scenario A: stop
	./demo/scripts/scenario.sh cpu stop
disk-start: ## Scenario B: start disk noisy neighbour (eBPF at L2)
	./demo/scripts/scenario.sh disk start
disk-stop: ## Scenario B: stop
	./demo/scripts/scenario.sh disk stop
grafana: ## Port-forward Grafana to http://localhost:3000 (admin/zoomlens)
	kubectl -n $(MONITORING_NS) port-forward svc/$(KPS_RELEASE)-grafana 3000:80
prometheus: ## Port-forward Prometheus to http://localhost:9090
	kubectl -n $(MONITORING_NS) port-forward svc/prometheus-operated 9090:9090

##@ Cleanup
.PHONY: teardown teardown-kind
teardown: ## Remove demo, Zoom-Lens and monitoring
	./scripts/teardown.sh
teardown-kind: ## Delete the kind cluster
	./scripts/teardown.sh --delete-kind
