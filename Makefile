# Developer entry point for MeuRPG. Run `make help` to see all targets.
#
# Every target is written to work from the repo root, no matter which
# directory the underlying tool actually needs to run in.

# Default DATABASE_URL for host-run commands (`make run`, `make migrate`):
# it points at localhost:26257, where either the CockroachDB container of
# `make up` or the native one of `make db-native-start` listens. Override on the command line if needed, e.g.:
#   make run DATABASE_URL=postgresql://root@localhost:26257/otherdb?sslmode=disable
DATABASE_URL ?= postgresql://root@localhost:26257/meurpg?sslmode=disable

# Sub-command for `make migrate`, e.g.: make migrate MIGRATE_ARGS=status
MIGRATE_ARGS ?= up

COMPOSE_FILE := deploy/local/compose.yaml

# The ELK stack that ships the API's logs (deploy/elk, docs/operations/logs.md#locally-with-elk).
ELK_COMPOSE := docker compose -f deploy/elk/compose.yaml

# Where the local stack's database runs. `container` (the default, and what
# CI uses) runs CockroachDB in Docker with the rest of the stack.
# `LOCAL_DB=native` points the stack at a CockroachDB running on this
# machine instead (`make db-native-start`): on a Mac every container runs
# inside a Linux VM, and the database is by far the heaviest part of the
# stack. Set it once in your shell (`export LOCAL_DB=native`) or per command
# (`make up LOCAL_DB=native`). See CONTRIBUTING.md, "CockroachDB nativo".
LOCAL_DB ?= container
COMPOSE_FILES := -f $(COMPOSE_FILE)

# How the local stack runs. `docker` (the default, and what CI uses) runs
# the API and devidp in containers. `LOCAL_STACK=native` runs them as
# processes on this machine, against the native CockroachDB, with no Docker
# at all (deploy/local/native.sh): on a Mac that frees the Linux VM's memory
# for the in-memory test databases. It implies the native database. See
# CONTRIBUTING.md, "Tudo nativo".
LOCAL_STACK ?= docker
ifeq ($(LOCAL_DB),native)
COMPOSE_FILES += -f deploy/local/compose.native-db.yaml
endif

# The native CockroachDB: the same version as the container image in
# deploy/local/compose.yaml and the go-db job in .github/workflows/backend.yml
# (install it with `brew install cockroachdb/tap/cockroach@26.2`). Its data
# lives outside the repo, so every worktree shares one database.
COCKROACH_VERSION := v26.2.8
COCKROACH_STORE ?= $(HOME)/.meurpg/cockroach

# The test CockroachDB (`make db-test-start`): a second native node on port
# 26258 for the integration tests only, whose store lives in memory (2 GiB at
# most) and disappears when it stops. Schema changes, which the test
# databases are made of, take a quarter of the time they take on disk, and
# the development database on 26257 doesn't fill up with test databases.
# Dropped databases give their memory back after 10 minutes, not the
# default 4 hours (gc.ttlseconds).
#
# TEST_DB_PORT starts more than one, each with its own memory and console
# (`make db-test-start TEST_DB_PORT=26259`): two runs of the integration
# tests then never wait for each other. The console port follows the SQL
# port (26258 -> 8082, 26259 -> 8083...).
TEST_DB_PORT ?= 26258
TEST_DB_HTTP_PORT := $(shell echo $$(( $(TEST_DB_PORT) - 26258 + 8082 )))
ifeq ($(TEST_DB_PORT),26258)
TEST_DB_DIR ?= $(HOME)/.meurpg/cockroach-test
else
TEST_DB_DIR ?= $(HOME)/.meurpg/cockroach-test-$(TEST_DB_PORT)
endif
TEST_DATABASE_URL := postgresql://root@localhost:$(TEST_DB_PORT)/defaultdb?sslmode=disable

# sqlc is pinned: its version is written into every file it generates, so
# every machine and CI must run the same one. `go run pkg@version` builds
# exactly that version, checked against Go's checksum database, with no
# install step. The first run compiles it (about a minute); later runs use
# Go's build cache.
SQLC := go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1

# golangci-lint is pinned to the version CI runs (golangci-lint-action in
# .github/workflows/backend.yml), so `make lint` and CI agree. Change both
# together. The first run compiles it (a minute or two); Go caches it after.
GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0

.PHONY: help proto sqlc proto-lint lint lint-all test before-ready run migrate up down logs docker-build web-install web-test web-build e2e db-native-start db-native-stop db-test-start db-test-stop elk-up elk-down elk-logs

help: ## Show this help message
	@echo "MeuRPG - available targets:"
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

# buf's TypeScript plugin runs from web/node_modules/.bin (see
# proto/buf.gen.yaml), so `proto` depends on it as a real file target:
# installed once, then only reinstalled when package.json/package-lock.json
# change, instead of on every `make proto`.
web/node_modules/.bin/protoc-gen-es: web/package.json web/package-lock.json
	cd web && npm ci --ignore-scripts

proto: web/node_modules/.bin/protoc-gen-es ## Generate Go and TypeScript code from proto/ (writes into backend/gen and web/src/gen)
	cd proto && buf generate

sqlc: ## Generate Go code from the SQL queries (backend/sqlc.yaml) into backend/internal/*/*db
	cd backend && $(SQLC) generate

proto-lint: ## Lint proto files and check formatting
	cd proto && buf lint
	cd proto && buf format --diff --exit-code

# The Go linters run as a ratchet: `lint` reports only what your branch added
# since LINT_BASE (the merge base with it), exactly like CI on a pull request;
# `lint-all` reports the whole backlog. Make sure the base is up to date first
# (`git fetch origin main`).
LINT_BASE ?= origin/main

# Every finding of the backlog: by default golangci-lint shows at most 50 per
# linter, 3 with the same text and one per line, and gocyclo, gocognit and
# maintidx all report on the same `func` line.
LINT_ALL_FLAGS := --max-issues-per-linter=0 --max-same-issues=0 --uniq-by-line=false

lint: proto-lint ## Run all linters (proto + Go); Go only for what changed since origin/main, like CI
	cd backend && $(GOLANGCI_LINT) run --new-from-merge-base=$(LINT_BASE)

lint-all: proto-lint ## Run all linters on the whole code, including the backlog the ratchet lets through
	cd backend && $(GOLANGCI_LINT) run $(LINT_ALL_FLAGS)

test: ## Run backend Go tests with the race detector
	cd backend && go test -race ./...

# shell-quote puts its argument in single quotes for the shell. It is called with $(value VAR), the
# text of the variable as it was typed: make would otherwise read a `$` of a regex as a variable
# ('^a$|^b$' would become '^a^b$').
shell-quote = '$(subst ','\'',$(1))'

# The tests a branch's own change makes stale (goldens, hashes, frozen tables, stored fixtures,
# the lists of methods, error reasons and event kinds), run against the test database like
# `make test`. The list and the command that regenerates each file are in
# backend/internal/archtest/before_ready.txt. It runs the lines the change can affect; ALL=1 runs
# every line. ONLY='internal/play$' (a Go regex on the package directory) narrows the lines to the
# packages whose directory matches.
before-ready: ## Run the golden, frozen, fixture and inventory tests the change can affect (all of them with ALL=1) (needs MEURPG_TEST_DATABASE_URL); run it before writing `ready:`
	TEST_DATABASE_URL='$(TEST_DATABASE_URL)' BEFORE_READY_ONLY=$(call shell-quote,$(or $(value ONLY),$(value BEFORE_READY_ONLY))) BEFORE_READY_ALL=$(call shell-quote,$(or $(value ALL),$(value BEFORE_READY_ALL))) bash .github/scripts/before-ready.sh

run: ## Run the API server on the host (needs `make up` for the DB)
	cd backend && DATABASE_URL="$(DATABASE_URL)" go run ./cmd/api

migrate: ## Run DB migrations on the host, e.g. `make migrate MIGRATE_ARGS=status`
	cd backend && DATABASE_URL="$(DATABASE_URL)" go run ./cmd/migrate $(MIGRATE_ARGS)

up: ## Start the local stack in the background; LOCAL_DB=native uses the native database, LOCAL_STACK=native runs everything without Docker
ifeq ($(LOCAL_STACK),native)
	bash deploy/local/native.sh up
else
	docker compose $(COMPOSE_FILES) up --build -d
endif

down: ## Stop the local stack (and remove its containers)
ifeq ($(LOCAL_STACK),native)
	bash deploy/local/native.sh down
else
	docker compose $(COMPOSE_FILES) down
endif

logs: ## Follow logs from the local stack
ifeq ($(LOCAL_STACK),native)
	bash deploy/local/native.sh logs
else
	docker compose $(COMPOSE_FILES) logs -f
endif

elk-up: ## Start Elasticsearch, Kibana and Filebeat for the API logs (creates deploy/elk/.env with random passwords the first time)
	@mkdir -p "$${MEURPG_HOME:-$$HOME/.meurpg}"
	bash deploy/elk/init-env.sh
	$(ELK_COMPOSE) up -d
	@echo "Kibana: http://localhost:5601 (user elastic, password in deploy/elk/.env). Query from the terminal: deploy/elk/logs.sh help"

elk-down: ## Stop the ELK stack (the logs stay in the esdata volume; `docker compose -f deploy/elk/compose.yaml down -v` deletes them)
	$(ELK_COMPOSE) down

elk-logs: ## Follow the ELK stack's own logs (not the API's: use deploy/elk/logs.sh tail)
	$(ELK_COMPOSE) logs -f

db-native-start: ## Start CockroachDB on this machine (brew cockroach@26.2) for LOCAL_DB=native and the integration tests
	@command -v cockroach >/dev/null || { echo "cockroach not found: brew install cockroachdb/tap/cockroach@26.2 (CONTRIBUTING.md, \"CockroachDB nativo\")"; exit 1; }
	@cockroach version | grep -q 'Build Tag: *$(COCKROACH_VERSION)$$' || echo "warning: cockroach is not $(COCKROACH_VERSION), the version CI and the container use"
	@mkdir -p "$(COCKROACH_STORE)"
	@if cockroach sql --insecure --host=localhost:26257 -e 'SELECT 1' >/dev/null 2>&1; then \
		echo "CockroachDB is already running on localhost:26257"; \
	else \
		cockroach start-single-node --insecure --listen-addr=localhost:26257 --http-addr=localhost:8081 \
			--store="$(COCKROACH_STORE)" --pid-file="$(COCKROACH_STORE)/cockroach.pid" --background \
			>"$(COCKROACH_STORE)/start.log" 2>&1 || { cat "$(COCKROACH_STORE)/start.log"; exit 1; }; \
	fi
	cockroach sql --insecure --host=localhost:26257 -e 'CREATE DATABASE IF NOT EXISTS meurpg'
	@echo "CockroachDB on localhost:26257 (console: http://localhost:8081). Stop it with: make db-native-stop"

db-native-stop: ## Stop the native CockroachDB started by db-native-start
	@if [ -f "$(COCKROACH_STORE)/cockroach.pid" ] && kill -0 $$(cat "$(COCKROACH_STORE)/cockroach.pid") 2>/dev/null; then \
		kill $$(cat "$(COCKROACH_STORE)/cockroach.pid"); \
		while kill -0 $$(cat "$(COCKROACH_STORE)/cockroach.pid") 2>/dev/null; do sleep 0.5; done; \
		echo "CockroachDB stopped"; \
	else \
		echo "CockroachDB is not running (no live $(COCKROACH_STORE)/cockroach.pid)"; \
	fi

db-test-start: ## Start an in-memory test CockroachDB on localhost:26258 (TEST_DB_PORT=26259 for a second one) for the integration tests
	@command -v cockroach >/dev/null || { echo "cockroach not found: brew install cockroachdb/tap/cockroach@26.2 (CONTRIBUTING.md, \"CockroachDB nativo\")"; exit 1; }
	@mkdir -p "$(TEST_DB_DIR)"
	@if cockroach sql --insecure --host=localhost:$(TEST_DB_PORT) -e 'SELECT 1' >/dev/null 2>&1; then \
		echo "The test CockroachDB is already running on localhost:$(TEST_DB_PORT)"; \
	else \
		cockroach start-single-node --insecure --listen-addr=localhost:$(TEST_DB_PORT) --http-addr=localhost:$(TEST_DB_HTTP_PORT) \
			--store=type=mem,size=2GiB --cache=256MiB --max-sql-memory=512MiB \
			--log-dir="$(TEST_DB_DIR)/logs" --pid-file="$(TEST_DB_DIR)/cockroach.pid" --background \
			>"$(TEST_DB_DIR)/start.log" 2>&1 || { cat "$(TEST_DB_DIR)/start.log"; exit 1; }; \
		cockroach sql --insecure --host=localhost:$(TEST_DB_PORT) \
			-e "SET CLUSTER SETTING sql.stats.automatic_collection.enabled = false" \
			-e "SET CLUSTER SETTING kv.range_merge.queue.enabled = false" \
			-e "SET CLUSTER SETTING jobs.retention_time = '15s'" \
			-e "SET CLUSTER SETTING diagnostics.reporting.enabled = false" \
			-e "ALTER RANGE default CONFIGURE ZONE USING gc.ttlseconds = 600" >/dev/null; \
	fi
	@echo "Test CockroachDB on localhost:$(TEST_DB_PORT) (console: http://localhost:$(TEST_DB_HTTP_PORT)). Run the integration tests with:"
	@echo "  MEURPG_TEST_DATABASE_URL='$(TEST_DATABASE_URL)' make test"
	@echo "Stop it (and throw its data away) with: make db-test-stop TEST_DB_PORT=$(TEST_DB_PORT)"

db-test-stop: ## Stop an in-memory test CockroachDB started by db-test-start (TEST_DB_PORT, 26258 by default)
	@if [ -f "$(TEST_DB_DIR)/cockroach.pid" ] && kill -0 $$(cat "$(TEST_DB_DIR)/cockroach.pid") 2>/dev/null; then \
		kill $$(cat "$(TEST_DB_DIR)/cockroach.pid"); \
		while kill -0 $$(cat "$(TEST_DB_DIR)/cockroach.pid") 2>/dev/null; do sleep 0.5; done; \
		echo "The test CockroachDB on localhost:$(TEST_DB_PORT) stopped"; \
	else \
		echo "The test CockroachDB on localhost:$(TEST_DB_PORT) is not running (no live $(TEST_DB_DIR)/cockroach.pid)"; \
	fi

docker-build: ## Build the backend+web Docker image standalone (no compose)
	docker build \
		-f backend/Dockerfile \
		--build-arg VERSION=dev \
		--build-arg COMMIT=$$(git rev-parse --short HEAD) \
		-t meurpg-backend:dev \
		.

web-install: ## Install web/ dependencies (no install scripts)
	cd web && npm ci --ignore-scripts

web-test: ## Run the Angular unit tests (run `make web-install` first if needed)
	cd web && npm test

web-build: ## Build the Angular app for production (run `make web-install` first if needed)
	cd web && npm run build

# e2e/ dependencies (Playwright), installed like web/'s: once, then again
# only when package.json/package-lock.json change. No install scripts.
e2e/node_modules/.bin/playwright: e2e/package.json e2e/package-lock.json
	cd e2e && npm ci --ignore-scripts

e2e: e2e/node_modules/.bin/playwright ## Start the local stack, run the Playwright tests against it, and report (LOCAL_STACK=native: without Docker)
ifeq ($(LOCAL_STACK),native)
	bash deploy/local/native.sh up
else
	docker compose $(COMPOSE_FILES) up --build -d
endif
	cd e2e && npx playwright test; status=$$?; \
		echo "Report: e2e/playwright-report/index.html (cd e2e && npx playwright show-report). The stack is still up: make down$(if $(filter native,$(LOCAL_STACK)), LOCAL_STACK=native,)"; \
		exit $$status
