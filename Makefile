# GoBlog.dev: a multilingual blog (English + Indonesian).
#
# Local:      make mongo && make run     (Go runs on your Mac, MongoDB in a container)
# Container:  make up                    (app + MongoDB both in Apple containers)
#
# Settings such as MONGODB_URI are read from .env (see .env.example).
# The admin password lives in MongoDB: open /admin and follow the setup steps.
#
# Container targets use Apple's `container` CLI: https://github.com/apple/container

APP_NAME        ?= blog
IMAGE           ?= $(APP_NAME):latest
PORT            ?= 8080
LANGUAGES       ?= en,id

NETWORK         ?= $(APP_NAME)
APP_CONTAINER   ?= $(APP_NAME)-app
MONGO_CONTAINER ?= $(APP_NAME)-mongo
MONGO_VOLUME    ?= $(APP_NAME)-mongo-data
MONGO_IMAGE     ?= docker.io/library/mongo:8
# Host port for the Mongo container; 27018 avoids clashing with a local mongod.
MONGO_PORT      ?= 27018
MONGODB_DB      ?= blog

.DEFAULT_GOAL := help

# Loads .env into a recipe's shell. Sourcing it (rather than make's `include`)
# keeps values containing # or $, like passwords, intact.
LOAD_ENV = if [ -f .env ]; then set -a; . ./.env; set +a; fi;

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ---------------------------------------------------------------- local ----

frontend/node_modules: frontend/package-lock.json
	cd frontend && npm ci
	@touch $@

.PHONY: frontend
frontend: frontend/node_modules ## Build the React bundles into the Go embed directory
	cd frontend && npm run build

.PHONY: build
build: frontend ## Compile bin/blog with the frontend embedded
	cd backend && go build -trimpath -o ../bin/$(APP_NAME) ./cmd/server

.PHONY: run
run: frontend ## Run the app locally (needs MongoDB; see `make mongo`)
	@$(LOAD_ENV) cd backend && \
		PORT=$${PORT:-$(PORT)} LANGUAGES=$${LANGUAGES:-$(LANGUAGES)} MONGODB_DB=$${MONGODB_DB:-$(MONGODB_DB)} \
		MONGODB_URI=$${MONGODB_URI:-mongodb://localhost:$(MONGO_PORT)} go run ./cmd/server

.PHONY: db-setup
db-setup: frontend ## Create the collections, schema validators and indexes in MONGODB_URI (once, on a new database)
	@$(LOAD_ENV) cd backend && \
		LANGUAGES=$${LANGUAGES:-$(LANGUAGES)} MONGODB_DB=$${MONGODB_DB:-$(MONGODB_DB)} \
		MONGODB_URI=$${MONGODB_URI:-mongodb://localhost:$(MONGO_PORT)} go run ./cmd/server setup-db

.PHONY: watch
watch: frontend/node_modules ## Rebuild frontend bundles on change (restart `make run` to pick them up)
	cd frontend && npm run watch

# ---------------------------------------------------------- quality ----

GOLANGCI_LINT_VERSION ?= v2.14.0
# Built with the project's Go version on first use, then cached.
GOLANGCI_LINT = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
MONGODB_TEST_URI ?= mongodb://localhost:$(MONGO_PORT)

.PHONY: test
test: test-unit ## Run the unit tests (same as test-unit)

.PHONY: test-unit
test-unit: frontend ## Unit tests: frontend (node:test) and backend (go test, in-memory stores)
	cd frontend && npm test
	cd backend && go test ./...

.PHONY: test-integration
test-integration: frontend ## Integration tests against a real MongoDB (start one with `make mongo`)
	cd backend && MONGODB_TEST_URI=$(MONGODB_TEST_URI) go test -count=1 -run Mongo ./...

.PHONY: test-all
test-all: test-unit test-integration ## Unit and integration tests

.PHONY: cover
cover: frontend ## Backend test coverage per package (writes backend/coverage.out)
	cd backend && go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -1

.PHONY: lint
lint: lint-go lint-frontend ## Lint Go (golangci-lint) and the frontend (ESLint + TypeScript)

.PHONY: lint-go
lint-go: ## Go: golangci-lint's standard linters, gofmt and goimports
	cd backend && $(GOLANGCI_LINT) run ./...

.PHONY: lint-frontend
lint-frontend: frontend/node_modules ## Frontend: ESLint (TypeScript, React, React Hooks) and a type check
	cd frontend && npm run lint && npm run typecheck

.PHONY: fmt
fmt: frontend/node_modules ## Fix formatting and auto-fixable lint issues
	cd backend && $(GOLANGCI_LINT) fmt ./...
	cd frontend && npx eslint --fix .

.PHONY: check
check: lint test-unit ## Everything the pre-commit hook runs: lint and unit tests

.PHONY: hooks
hooks: ## Install the git pre-commit hook (runs `make check` before each commit)
	git config core.hooksPath .githooks
	@echo "Pre-commit hook installed. Skip it once with: git commit --no-verify"

.PHONY: clean
clean: ## Remove build output
	rm -rf bin backend/internal/web/dist/assets backend/internal/web/dist/server.js

# ------------------------------------------------------------ container ----

.PHONY: container-system
container-system:
	@container system status >/dev/null 2>&1 || container system start

.PHONY: image
image: container-system ## Build the app image with `container build`
	container build --tag $(IMAGE) --file Dockerfile .

.PHONY: network
network: container-system
	@container network inspect $(NETWORK) >/dev/null 2>&1 || container network create $(NETWORK)

.PHONY: mongo
mongo: network ## Start MongoDB in a container, published on localhost:27018
	@container volume inspect $(MONGO_VOLUME) >/dev/null 2>&1 || container volume create $(MONGO_VOLUME)
	@if container inspect $(MONGO_CONTAINER) >/dev/null 2>&1; then \
		container start $(MONGO_CONTAINER) >/dev/null 2>&1 || true; \
	else \
		container run --detach --name $(MONGO_CONTAINER) --network $(NETWORK) \
			--volume $(MONGO_VOLUME):/data/db --publish $(MONGO_PORT):27017 $(MONGO_IMAGE); \
	fi
	@printf "Waiting for MongoDB"; \
	for i in $$(seq 1 60); do \
		container exec $(MONGO_CONTAINER) mongosh --quiet --eval 'db.runCommand({ping: 1}).ok' >/dev/null 2>&1 \
			&& { echo " ready."; exit 0; }; \
		printf "."; sleep 1; \
	done; echo " timed out."; exit 1

# The app connects to MONGODB_URI from .env. Inside a container localhost is the
# container itself, so localhost/127.0.0.1 is rewritten to the Mac's address on
# the container network. The bundled MongoDB container is started only when
# MONGODB_URI is unset (the app is then given its IP, since containers can't
# resolve each other by name) or points at its published port, localhost:$(MONGO_PORT).
.PHONY: up
up: image network ## Build and run the app in a container, connected to MONGODB_URI from .env
	@$(LOAD_ENV) case "$${MONGODB_URI:-}" in ""|*localhost:$(MONGO_PORT)*|*127.0.0.1:$(MONGO_PORT)*) $(MAKE) --no-print-directory mongo ;; esac
	@container rm --force $(APP_CONTAINER) >/dev/null 2>&1 || true
	@# Deletion is asynchronous; wait so the name is free before reusing it.
	@for i in $$(seq 1 20); do container inspect $(APP_CONTAINER) >/dev/null 2>&1 || break; sleep 0.5; done
	@$(LOAD_ENV) if [ -z "$${MONGODB_URI:-}" ]; then \
		MONGO_IP=$$(container inspect $(MONGO_CONTAINER) | jq -r '.[0].status.networks[0].ipv4Address | split("/")[0]'); \
		MONGODB_URI=mongodb://$$MONGO_IP:27017; \
	else \
		HOST_IP=$$(container network inspect $(NETWORK) | jq -r '.[0].status.ipv4Gateway'); \
		MONGODB_URI=$$(printf '%s' "$$MONGODB_URI" | sed -E "s#(@|://)(localhost|127\.0\.0\.1)([:/,]|$$)#\1$$HOST_IP\3#g"); \
	fi; \
	echo "Using MongoDB: $${MONGODB_URI##*@}"; \
	container run --detach --name $(APP_CONTAINER) --network $(NETWORK) --publish $(PORT):8080 \
		--env MONGODB_URI="$$MONGODB_URI" --env MONGODB_DB="$${MONGODB_DB:-$(MONGODB_DB)}" --env LANGUAGES=$(LANGUAGES) \
		$(IMAGE) >/dev/null
	@APP_IP=$$(container inspect $(APP_CONTAINER) | jq -r '.[0].status.networks[0].ipv4Address | split("/")[0]'); \
	printf "Waiting for the app"; \
	for i in $$(seq 1 30); do \
		curl -sf -m 2 http://localhost:$(PORT)/healthz >/dev/null && { echo " ready: http://localhost:$(PORT)"; exit 0; }; \
		if curl -sf -m 2 http://$$APP_IP:8080/healthz >/dev/null; then \
			echo " ready: http://$$APP_IP:8080"; \
			echo "localhost:$(PORT) isn't forwarding yet. If it stays that way, allow \"container\" under"; \
			echo "System Settings > Privacy & Security > Local Network, then run 'make up' again."; \
			exit 0; \
		fi; \
		printf "."; sleep 1; \
	done; echo " failed to start. Logs:"; container logs $(APP_CONTAINER); exit 1

.PHONY: down
down: ## Stop and remove the app and MongoDB containers (keeps the data volume)
	-@container rm --force $(APP_CONTAINER) $(MONGO_CONTAINER) >/dev/null 2>&1
	@echo "Stopped. Data kept in volume $(MONGO_VOLUME)."

.PHONY: admin-code
admin-code: ## Show the one-time admin setup code from the app container's log
	@code=$$(container logs $(APP_CONTAINER) 2>&1 | grep "Admin setup code" | tail -1); \
	if [ -n "$$code" ]; then echo "$$code"; \
	else echo "No setup code in the log. If an admin password already exists, sign in at /admin/login."; fi

.PHONY: admin-reset
admin-reset: ## Forgot the admin password? Remove it so it can be set up again
	container exec $(APP_CONTAINER) /blog reset-admin
	@echo "Open /admin to start setup again, then run 'make admin-code' for the new setup code."

.PHONY: logs
logs: ## Follow the app container's logs
	container logs --follow $(APP_CONTAINER)

.PHONY: ps
ps: ## List this project's containers
	@container list --all | awk 'NR == 1 || /$(APP_NAME)-/'

.PHONY: destroy
destroy: down ## `down`, then delete the MongoDB volume (ALL DATA), network, and image
	-container volume delete $(MONGO_VOLUME)
	-container network delete $(NETWORK)
	-container image delete $(IMAGE)
