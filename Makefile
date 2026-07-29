# Validator local-dev orchestrator.
#
# Brings up the full stack (Postgres + Restate in Docker, then the Go API and
# Worker binaries) from a single command. The UI dev server is started too if it
# is not already running.
#
# Quick start:
#   make up        # start everything (will prompt for your sudo password for Docker)
#   make status    # show containers, app pids, listening ports
#   make down      # stop containers + app processes (keeps data volumes)
#
# Docker is invoked through sudo because the dev user is not in the docker group.
# sudo caches the password for ~15 min, so you only type it once per `make` run.

SHELL := /bin/bash

# --- overridable knobs (e.g. `make up PG_PORT=5432`) ---
GO            ?= go
SUDO          ?= sudo
DOCKER        ?= $(SUDO) docker

PG_NAME       ?= validator-pg
PG_IMAGE      ?= postgres:16
# 5432 matches the original data container that holds existing ideas.
PG_PORT       ?= 5432
PG_USER       ?= validator
PG_PASS       ?= validator
PG_DB         ?= validator
PG_VOLUME     ?= validator-pg-data

RESTATE_NAME       ?= validator-restate
RESTATE_IMAGE      ?= docker.io/restatedev/restate:latest
# Restate ingress (API surface calls this) and meta/admin (CLI + /services).
RESTATE_INGRESS    ?= 8080
RESTATE_META       ?= 9070
RESTATE_VOLUME     ?= validator-restate-data

API_PORT      ?= 8000
WORKER_PORT   ?= 9080
# docker0 bridge gateway: how the restate container reaches the host worker.
WORKER_HOST   ?= 172.17.0.1
UI_PORT       ?= 5173

# --- derived paths ---
ROOT     := $(CURDIR)
BACKEND  := $(ROOT)/validator-backend
UI       := $(ROOT)/validator-ui
LOGDIR   := $(ROOT)/.logs

.DEFAULT_GOAL := help
.PHONY: help up down restart infra db restate env-sync build api worker ui \
        tunnel start-api start-worker start-ui register wait-port wait-pg services status \
        logs logs-api logs-worker logs-pg logs-restate logs-ui stop-apps clean

help: ## Show this help
	@awk 'BEGIN{FS=":.*##"; printf "Validator local dev\n\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	  /^[a-zA-Z_-]+:.*##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ---------------------------------------------------------------------------
# Full stack
# ---------------------------------------------------------------------------

up: infra start-api start-worker register ## Start Postgres + Restate (docker), API, Worker, register worker; start UI if down
	@if (: < /dev/tcp/127.0.0.1/$(UI_PORT)) 2>/dev/null; then \
	   echo "ui already running on :$(UI_PORT)"; \
	 else echo "starting ui..."; $(MAKE) --no-print-directory start-ui; fi
	@echo ""
	@echo "=========================================================="
	@echo " All services up"
	@echo "   UI:        http://localhost:$(UI_PORT)"
	@echo "   API:       http://localhost:$(API_PORT)"
	@echo "   Restate:   meta http://localhost:$(RESTATE_META)  ingress :$(RESTATE_INGRESS)"
	@echo "   Postgres:  localhost:$(PG_PORT)  (db=$(PG_DB) user=$(PG_USER))"
	@echo " logs:       make logs-api | logs-worker | logs-pg | logs-restate | logs-ui"
	@echo " status:     make status      stop: make down"
	@echo "=========================================================="

down: stop-apps ## Stop containers + app processes (keeps data volumes)
	-@$(DOCKER) stop $(PG_NAME) $(RESTATE_NAME) 2>/dev/null || true
	-@$(DOCKER) rm $(PG_NAME) $(RESTATE_NAME) 2>/dev/null || true
	@echo "containers + apps stopped (volumes kept). 'make clean' to wipe data."

restart: down ## Restart the whole stack
	@$(MAKE) --no-print-directory up

# ---------------------------------------------------------------------------
# Docker infrastructure
# ---------------------------------------------------------------------------

infra: env-sync db restate ## Ensure Postgres + Restate containers are running

env-sync: ## Rewrite DATABASE_URL in validator-backend/.env to point at the containerized Postgres
	@cd $(BACKEND) && sed -i -E 's|^DATABASE_URL=.*|DATABASE_URL=postgres://$(PG_USER):$(PG_PASS)@localhost:$(PG_PORT)/$(PG_DB)?sslmode=disable|' .env
	@echo "synced $(BACKEND)/.env  ->  DATABASE_URL=postgres://$(PG_USER):***@localhost:$(PG_PORT)/$(PG_DB)"

db: ## Start the Postgres container
	@if $(DOCKER) inspect $(PG_NAME) >/dev/null 2>&1; then \
	   echo "postgres: starting existing container $(PG_NAME)"; $(DOCKER) start $(PG_NAME); \
	 else echo "postgres: creating container $(PG_NAME) on :$(PG_PORT)"; \
	   $(DOCKER) run -d --name $(PG_NAME) \
	     -e POSTGRES_USER=$(PG_USER) -e POSTGRES_PASSWORD=$(PG_PASS) -e POSTGRES_DB=$(PG_DB) \
	     -p $(PG_PORT):5432 $(PG_IMAGE); \
	 fi
	@$(MAKE) --no-print-directory wait-pg

# Postgres real-readiness probe: TCP-open fires before first-boot init finishes,
# so we wait until pg_isready reports "accepting connections".
wait-pg:
	@echo "  waiting for postgres to accept connections..."
	@for i in $$(seq 1 60); do \
	  pg_isready -h 127.0.0.1 -p $(PG_PORT) -t 1 2>/dev/null | grep -q "accepting connections" \
	    && { echo "  [ready] postgres on :$(PG_PORT)"; exit 0; }; \
	  sleep 0.5; \
	 done; echo "  [timeout] postgres not ready on :$(PG_PORT)"; exit 1

restate: ## Start the Restate runtime container
	@if $(DOCKER) inspect $(RESTATE_NAME) >/dev/null 2>&1; then \
	   echo "restate: starting existing container $(RESTATE_NAME)"; $(DOCKER) start $(RESTATE_NAME); \
	 else echo "restate: creating container $(RESTATE_NAME) on :$(RESTATE_INGRESS)/:$(RESTATE_META)"; \
	   $(DOCKER) run -d --name $(RESTATE_NAME) \
	     -p $(RESTATE_INGRESS):8080 -p $(RESTATE_META):9070 \
	     -e RESTATE_LOG_FORMAT=compact -v $(RESTATE_VOLUME):/restate-data $(RESTATE_IMAGE); \
	 fi
	@$(MAKE) --no-print-directory wait-port PORT=$(RESTATE_META) WHAT=restate-meta

# ---------------------------------------------------------------------------
# Go binaries
# ---------------------------------------------------------------------------

build: ## Build cmd/api and cmd/worker into validator-backend/bin
	@mkdir -p $(BACKEND)/bin
	@cd $(BACKEND) && $(GO) build -o bin/api ./cmd/api && $(GO) build -o bin/worker ./cmd/worker
	@echo "built validator-backend/bin/{api,worker}"

api: build ## Run the API in the foreground (Ctrl-C to stop)
	@cd $(BACKEND) && ./bin/api

worker: build ## Run the Worker in the foreground (Ctrl-C to stop)
	@cd $(BACKEND) && ./bin/worker

ui: ## Run the Vite dev server in the foreground
	@cd $(UI) && npm run dev

# Delegates to dev.sh (single source of truth for the pinggy parsing logic).
tunnel: ## Start a free HTTPS tunnel (pinggy) and auto-sync WEBHOOK_PUBLIC_URL
	@./dev.sh tunnel

# Background runners used by `make up`. Each writes a pidfile + log under .logs/.
start-api: build
	@mkdir -p $(LOGDIR)
	@-pkill -x api 2>/dev/null; sleep 1
	@( cd $(BACKEND) && exec ./bin/api ) > $(LOGDIR)/api.log 2>&1 & echo $$! > $(LOGDIR)/api.pid
	@echo "api: started (pid $$(cat $(LOGDIR)/api.pid), log $(LOGDIR)/api.log)"
	@$(MAKE) --no-print-directory wait-port PORT=$(API_PORT) WHAT=api

start-worker: build
	@mkdir -p $(LOGDIR)
	@-pkill -x worker 2>/dev/null; sleep 1
	@( cd $(BACKEND) && exec ./bin/worker ) > $(LOGDIR)/worker.log 2>&1 & echo $$! > $(LOGDIR)/worker.pid
	@echo "worker: started (pid $$(cat $(LOGDIR)/worker.pid), log $(LOGDIR)/worker.log)"
	@$(MAKE) --no-print-directory wait-port PORT=$(WORKER_PORT) WHAT=worker

start-ui:
	@mkdir -p $(LOGDIR)
	@( cd $(UI) && exec npm run dev ) > $(LOGDIR)/ui.log 2>&1 & echo $$! > $(LOGDIR)/ui.pid
	@echo "ui: started (pid $$(cat $(LOGDIR)/ui.pid), log $(LOGDIR)/ui.log)"
	@$(MAKE) --no-print-directory wait-port PORT=$(UI_PORT) WHAT=ui

# ---------------------------------------------------------------------------
# Restate registration + introspection
# ---------------------------------------------------------------------------

register: ## Register the worker deployment with the Restate runtime
	@$(MAKE) --no-print-directory wait-port PORT=$(RESTATE_META) WHAT=restate-meta
	@$(MAKE) --no-print-directory wait-port PORT=$(WORKER_PORT) WHAT=worker
	@echo "restate: registering http://$(WORKER_HOST):$(WORKER_PORT) ..."
	@restate deployments register http://$(WORKER_HOST):$(WORKER_PORT) --yes \
	   || { echo "restate: register failed. If components changed shape, re-run: restate deployments register http://$(WORKER_HOST):$(WORKER_PORT) --force --yes"; exit 1; }
	@echo "restate: registered. Services:"; curl -s http://localhost:$(RESTATE_META)/services; echo

services: ## List services registered in the Restate runtime
	@curl -s http://localhost:$(RESTATE_META)/services && echo || echo "restate not reachable on :$(RESTATE_META)"

# ---------------------------------------------------------------------------
# Observability / lifecycle
# ---------------------------------------------------------------------------

# Generic TCP readiness probe. Usage: make wait-port PORT=8000 WHAT=api
wait-port:
	@test -n "$(PORT)" || { echo "wait-port: PORT is empty"; exit 1; }
	@for i in $$(seq 1 60); do \
	  if (: < /dev/tcp/127.0.0.1/$(PORT)) 2>/dev/null; then \
	    echo "  [ready] $(WHAT) on :$(PORT) ($$(echo "$$i*0.5"|bc)s)"; exit 0; \
	  fi; sleep 0.5; \
	done; echo "  [timeout] $(WHAT) on :$(PORT) not listening after 30s"; exit 1

status: ## Show containers, app pids, and listening ports
	@echo "=== docker containers ==="
	@-$(DOCKER) ps --filter name=$(PG_NAME) --filter name=$(RESTATE_NAME) \
	    --format '  {{.Names}}\t{{.Status}}\t{{.Ports}}' 2>/dev/null || echo "  (docker unavailable)"
	@echo "=== app processes ==="
	@for f in api worker ui; do \
	   if [ -f $(LOGDIR)/$$f.pid ]; then \
	     pid=$$(cat $(LOGDIR)/$$f.pid); \
	     if kill -0 $$pid 2>/dev/null; then echo "  $$f: running (pid $$pid)"; \
	     else echo "  $$f: stale pidfile ($$pid)"; fi; \
	   else echo "  $$f: not started"; fi; \
	 done
	@echo "=== ports ==="
	@for p in $(PG_PORT) $(RESTATE_META) $(RESTATE_INGRESS) $(API_PORT) $(WORKER_PORT) $(UI_PORT); do \
	   if (: < /dev/tcp/127.0.0.1/$$p) 2>/dev/null; then echo "  :$$p LISTEN"; else echo "  :$$p free"; fi; \
	 done

logs: logs-api ## Default: tail API log
logs-api:
	@tail -n 50 -f $(LOGDIR)/api.log
logs-worker:
	@tail -n 50 -f $(LOGDIR)/worker.log
logs-ui:
	@tail -n 50 -f $(LOGDIR)/ui.log
logs-pg:
	@$(DOCKER) logs --tail 50 -f $(PG_NAME)
logs-restate:
	@$(DOCKER) logs --tail 50 -f $(RESTATE_NAME)

stop-apps: ## Stop backgrounded api/worker/ui (pidfiles + stray sweep)
	@for f in api worker ui; do \
	   if [ -f $(LOGDIR)/$$f.pid ]; then \
	     pid=$$(cat $(LOGDIR)/$$f.pid); \
	     if kill -0 $$pid 2>/dev/null; then kill $$pid && echo "stopped $$f (pid $$pid)"; fi; \
	     rm -f $(LOGDIR)/$$f.pid; \
	   fi; \
	 done
	@-pkill -x api 2>/dev/null || true
	@-pkill -x worker 2>/dev/null || true

clean: down ## Stop everything and wipe containers, data volumes, build artifacts, logs
	-@$(DOCKER) volume rm $(PG_VOLUME) $(RESTATE_VOLUME) 2>/dev/null || true
	@rm -rf $(LOGDIR) $(BACKEND)/bin
	@echo "cleaned: containers, volumes, logs, binaries"
