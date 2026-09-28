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

.PHONY: watch
watch: frontend/node_modules ## Rebuild frontend bundles on change (restart `make run` to pick them up)
	cd frontend && npm run watch

.PHONY: test
test: frontend ## Type-check and test the frontend, then vet and test the backend
	cd frontend && npm run typecheck && npm test
	cd backend && go vet ./... && go test ./...

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

# Containers can't resolve each other by name unless an admin has run
# `sudo container system dns create <domain>`, so the app is given Mongo's IP.
.PHONY: up
up: image mongo ## Build and run the app + MongoDB in containers
	@container rm --force $(APP_CONTAINER) >/dev/null 2>&1 || true
	@# Deletion is asynchronous; wait so the name is free before reusing it.
	@for i in $$(seq 1 20); do container inspect $(APP_CONTAINER) >/dev/null 2>&1 || break; sleep 0.5; done
	@$(LOAD_ENV) MONGO_IP=$$(container inspect $(MONGO_CONTAINER) | jq -r '.[0].status.networks[0].ipv4Address | split("/")[0]'); \
	container run --detach --name $(APP_CONTAINER) --network $(NETWORK) --publish $(PORT):8080 \
		--env MONGODB_URI=mongodb://$$MONGO_IP:27017 --env MONGODB_DB=$(MONGODB_DB) --env LANGUAGES=$(LANGUAGES) \
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
