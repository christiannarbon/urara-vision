# Convenience targets. Nothing here is required -- every command is a plain
# docker/go/npm invocation you can run yourself.

.DEFAULT_GOAL := help
COMPOSE := docker compose
DOCS ?= /path/to/data-modelling/star-schema

# Kubernetes
NS       := urara-vision
PF_PORT  ?= 18081
PF_PID   := .k8s-portforward.pid
PF_LOG   := .k8s-portforward.log

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: up
up: ## Build and start the whole stack
	$(COMPOSE) up -d --build

.PHONY: down
down: ## Stop the stack, keeping volumes
	$(COMPOSE) down

.PHONY: clean
clean: ## Stop the stack and delete its volumes
	$(COMPOSE) down -v

.PHONY: logs
logs: ## Follow backend logs
	$(COMPOSE) logs -f backend

GO_IMAGE    := golang:1.25-alpine
# docs/ is mounted as well as the module because the demo suites read the sample
# documentation sets, which live outside backend/. Relative to
# backend/tests/unit/demo/ those are ../../../../docs/demo, which resolves to
# /docs/demo once /src is the module root -- so the same path works inside the
# container and out, and the suites need no build tag or environment variable to
# find them. Read-only: the suites parse the samples, they never write them.
GO_RUN      := docker run --rm \
                 -v "$(PWD)/backend":/src \
                 -v "$(PWD)/docs":/docs:ro \
                 -w /src $(GO_IMAGE)
# Integration tests connect to the compose stack over its own network, so they
# address the services by container name rather than through published ports.
COMPOSE_NET := urara-vision_default
# A database of its own, so a test run cannot disturb whatever you have ingested
# locally. Neo4j Community allows only one database, so the graph suites share it
# and isolate by snapshot ID instead.
TEST_DB     := relviz_test

.PHONY: test
test: ## Run the fast suites: backend unit tests and frontend typecheck + unit tests
	$(GO_RUN) go test ./...
	cd frontend && npm run typecheck && npm run test:run

.PHONY: test-unit
test-unit: ## Backend unit tests only (no databases needed)
	$(GO_RUN) go test ./tests/unit/...

.PHONY: test-integration
test-integration: ## Backend integration tests against the compose stack (starts it if needed)
	@echo "==> databases"
	$(COMPOSE) up -d postgres neo4j
	@$(COMPOSE) exec -T postgres sh -c \
	  'until pg_isready -U relviz -d relviz >/dev/null 2>&1; do sleep 1; done'
	@$(COMPOSE) exec -T postgres psql -U relviz -d relviz \
	  -c "CREATE DATABASE $(TEST_DB)" >/dev/null 2>&1 || true
	@echo "==> waiting for neo4j to accept bolt connections"
	@$(COMPOSE) exec -T neo4j sh -c \
	  'until cypher-shell -u neo4j -p relviz-dev-password "RETURN 1" >/dev/null 2>&1; do sleep 2; done'
	@echo "==> tests"
	docker run --rm --network $(COMPOSE_NET) -v "$(PWD)/backend":/src -w /src \
	  -e TEST_POSTGRES_DSN="postgres://relviz:relviz@postgres:5432/$(TEST_DB)?sslmode=disable" \
	  -e TEST_NEO4J_URI="bolt://neo4j:7687" \
	  -e TEST_NEO4J_USER="neo4j" \
	  -e TEST_NEO4J_PASSWORD="relviz-dev-password" \
	  $(GO_IMAGE) go test -tags=integration -count=1 ./tests/integration/...

.PHONY: test-all
test-all: test test-integration ## Everything: unit, frontend and integration

.PHONY: test-cover
test-cover: ## Backend unit test coverage over the packages under test
	$(GO_RUN) go test -coverpkg=./internal/... -coverprofile=coverage.out ./tests/unit/...
	$(GO_RUN) go tool cover -func=coverage.out | tail -1
	@echo "full report: cd backend && go tool cover -html=coverage.out"

.PHONY: test-frontend
test-frontend: ## Frontend unit and component tests
	cd frontend && npm run test:run

.PHONY: lint-docs
lint-docs: ## Parse a documentation directory and report diagnostics (make lint-docs DOCS=...)
	docker run --rm -v "$(PWD)/backend":/src -v "$(DOCS)":/docs:ro -w /src \
	  golang:1.25-alpine go run ./cmd/relctl -dir /docs

.PHONY: demo-docs
demo-docs: ## Parse every shipped demo documentation set (make demo-docs SET=jaffle-shop-ddd for one)
	@for set in $(or $(SET),$(notdir $(patsubst %/,%,$(dir $(wildcard docs/demo/*/.))))); do \
	  echo "== $$set =="; \
	  (cd backend && go run ./cmd/relctl -dir ../docs/demo/$$set) || exit $$?; \
	done

.PHONY: images
images: ## Build both images tagged :dev for a local cluster
	docker build -t urara-vision/backend:dev ./backend
	docker build -t urara-vision/frontend:dev ./frontend

.PHONY: k8s-load
k8s-load: images ## Build images and push them into minikube's daemon
	minikube image load urara-vision/backend:dev urara-vision/frontend:dev

.PHONY: k8s-up
k8s-up: ## Bring the whole Kubernetes stack up and open a tunnel to it
	@echo "==> cluster"
	@minikube status >/dev/null 2>&1 || minikube start
	@minikube addons list 2>/dev/null | grep -qE 'ingress .*enabled' || minikube addons enable ingress
	@echo "==> images"
	@$(MAKE) --no-print-directory k8s-load
	@echo "==> deploy"
	kubectl apply -k k8s/overlays/dev
	@echo "==> waiting for pods (first boot takes a minute or two)"
	kubectl -n $(NS) wait --for=condition=Ready pod/postgres-0 --timeout=300s
	kubectl -n $(NS) wait --for=condition=Ready pod/neo4j-0 --timeout=300s
	kubectl -n $(NS) rollout status deploy/backend --timeout=300s
	kubectl -n $(NS) rollout status deploy/frontend --timeout=180s
	@$(MAKE) --no-print-directory k8s-tunnel
	@echo
	@echo "  Urara Vision is up:  http://localhost:$(PF_PORT)"
	@echo "  logs:                  make k8s-logs"
	@echo "  shut down:             make k8s-down"

.PHONY: k8s-tunnel
k8s-tunnel: ## (Re)start the background port-forward
	@$(MAKE) --no-print-directory k8s-untunnel
	@nohup kubectl -n $(NS) port-forward svc/frontend $(PF_PORT):80 >$(PF_LOG) 2>&1 & echo $$! >$(PF_PID)
	@echo "==> tunnel on :$(PF_PORT) (pid $$(cat $(PF_PID)))"
	@for i in $$(seq 1 30); do \
		curl -sf http://localhost:$(PF_PORT)/healthz >/dev/null 2>&1 && exit 0; \
		sleep 1; \
	done; \
	echo "tunnel did not come up; see $(PF_LOG)" >&2; exit 1

.PHONY: k8s-untunnel
k8s-untunnel: ## Stop the background port-forward
	@if [ -f $(PF_PID) ]; then kill $$(cat $(PF_PID)) 2>/dev/null || true; rm -f $(PF_PID); fi
	@pkill -f "port-forward svc/frontend $(PF_PORT):80" 2>/dev/null || true

.PHONY: k8s-open
k8s-open: ## Port-forward in the foreground (ctrl-c to stop)
	@echo "http://localhost:$(PF_PORT)  (ctrl-c to stop)"
	kubectl -n $(NS) port-forward svc/frontend $(PF_PORT):80

.PHONY: k8s-logs
k8s-logs: ## Follow backend logs in the cluster
	kubectl -n $(NS) logs -f deploy/backend

.PHONY: k8s-status
k8s-status: ## Show what is running in the cluster
	@if kubectl get ns $(NS) >/dev/null 2>&1; then \
		kubectl -n $(NS) get pods,pvc,ingress; \
		if [ -f $(PF_PID) ] && kill -0 $$(cat $(PF_PID)) 2>/dev/null; then \
			echo; echo "tunnel: http://localhost:$(PF_PORT) (pid $$(cat $(PF_PID)))"; \
		else \
			echo; echo "tunnel: not running -- start it with 'make k8s-tunnel'"; \
		fi; \
	else \
		echo "not deployed -- bring it up with 'make k8s-up'"; \
	fi

.PHONY: k8s-down
k8s-down: ## Tear the Kubernetes stack down (deletes its PVCs and data)
	@$(MAKE) --no-print-directory k8s-untunnel
	kubectl delete -k k8s/overlays/dev --ignore-not-found --wait=false
	@echo "==> waiting for the namespace to finish deleting"
	@kubectl wait --for=delete ns/$(NS) --timeout=180s 2>/dev/null || true
	@rm -f $(PF_LOG)
	@echo "Urara Vision removed. The minikube cluster itself is still running."

.PHONY: k8s-validate
k8s-validate: ## Render and schema-check both overlays
	kubectl kustomize k8s/overlays/dev > /tmp/relviz-dev.yaml
	kubectl kustomize k8s/overlays/prod > /tmp/relviz-prod.yaml
	docker run --rm -v /tmp:/work ghcr.io/yannh/kubeconform:latest \
	  -strict -summary -kubernetes-version 1.33.0 \
	  /work/relviz-dev.yaml /work/relviz-prod.yaml

# --- CI ---------------------------------------------------------------------
# The workflows in .github/workflows are meant to be run here first. Every
# target below drives the real workflow file, so what passes locally is what
# GitHub will run -- and a failure costs no Actions minutes.
ACT           := act
ACTIONLINT    := rhysd/actionlint:1.7.12
GHALINT       := github.com/suzuki-shunsuke/ghalint/cmd/ghalint@v1.5.6
# act runs natively on Apple silicon, which is fast but not what GitHub uses.
# Set ACT_ARCH=linux/amd64 to reproduce a runner exactly (slow: it emulates).
ACT_ARCH      ?=
ACT_FLAGS     := $(if $(ACT_ARCH),--container-architecture $(ACT_ARCH),)

.PHONY: ci-lint
ci-lint: ## Lint the workflows themselves (no containers, ~1s)
	@command -v ghalint >/dev/null && ghalint run || go run $(GHALINT) run
	docker run --rm -v "$(PWD)":/repo -w /repo $(ACTIONLINT) -color

.PHONY: ci-list
ci-list: ## List every job act can run
	$(ACT) --list

.PHONY: ci-backend
ci-backend: ## Run the backend build and unit-test job locally
	$(ACT) $(ACT_FLAGS) -W .github/workflows/backend.yml -j check

.PHONY: ci-backend-integration
ci-backend-integration: ## Run the backend integration job locally (starts service containers)
	@# The job's service containers publish the same ports the compose stack
	@# uses, so the two cannot both be up. A bare "port is already allocated"
	@# from the daemon is not obvious, so say what to do about it.
	@for port in 5432 7474 7687; do \
	  if lsof -nP -iTCP:$$port -sTCP:LISTEN >/dev/null 2>&1; then \
	    echo "port $$port is in use -- the integration services cannot bind it."; \
	    echo "stop the dev stack first:  make down"; \
	    echo "or clear a service container a failed run left behind:  make ci-clean"; \
	    exit 1; \
	  fi; \
	done
	$(ACT) $(ACT_FLAGS) -W .github/workflows/backend.yml -j integration

.PHONY: ci-frontend
ci-frontend: ## Run the frontend job locally
	$(ACT) $(ACT_FLAGS) -W .github/workflows/frontend.yml -j check

.PHONY: ci-manifests
ci-manifests: ## Run the kustomize/kubeconform job locally
	$(ACT) $(ACT_FLAGS) -W .github/workflows/manifests.yml -j validate

.PHONY: ci-security
ci-security: ## Run the dependency scans locally
	$(ACT) $(ACT_FLAGS) -W .github/workflows/security.yml

.PHONY: ci-images
ci-images: ## Run the image build job locally
	$(ACT) $(ACT_FLAGS) -W .github/workflows/images.yml

.PHONY: ci-workflows
ci-workflows: ## Run the workflow-lint job locally, the way GitHub will
	$(ACT) $(ACT_FLAGS) -W .github/workflows/workflows.yml -j lint

.PHONY: ci
ci: ci-lint ci-backend ci-frontend ci-manifests ## What a pull request runs, minus the service containers

.PHONY: ci-clean
ci-clean: ## Remove the containers act keeps around between runs
	@docker ps -aq --filter "name=act-" | xargs -r docker rm -f
	@echo "act containers removed"
