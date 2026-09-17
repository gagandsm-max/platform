# ==============================================================================
# Platform Control Plane - Engineering Automation Makefile
# ==============================================================================

SHELL := /bin/bash
.DEFAULT_GOAL := help

# Environment files
ENV_FILE ?= .env
ifeq ($(wildcard $(ENV_FILE)),)
    $(shell cp .env.example .env)
endif

include $(ENV_FILE)
export

.PHONY: help doctor dev-build dev-up dev-down dev-restart dev-status dev-logs test test-unit test-static-contracts test-integration test-contract test-security-smoke test-agent-runtime-e2e evidence clean dev-reset certs post-reboot-verify dev-disk-check dev-hygiene

help: ## Show available Makefile targets
	@echo "========================================================================"
	@echo "Platform Engineering Environment Control"
	@echo "========================================================================"
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

certs: ## Generate local development TLS certificates if missing
	@bash scripts/generate-dev-certs.sh

doctor: ## Inspect host environment, verify Docker, Compose, ports, certs, disk, RAM
	@echo "==> Running Platform Environment Doctor..."
	@echo -n "Checking Docker Engine... "
	@docker --version > /dev/null 2>&1 && echo "OK ($$(docker --version))" || (echo "FAILED: Docker not found"; exit 1)
	@echo -n "Checking Docker Compose... "
	@docker compose version > /dev/null 2>&1 && echo "OK ($$(docker compose version))" || (echo "FAILED: Docker compose not found"; exit 1)
	@echo -n "Checking Dev TLS Certificates... "
	@[ -f deploy/docker/certs/platform-dev.crt ] && [ -f deploy/docker/certs/platform-dev.key ] && echo "OK" || (echo "MISSING; run 'make certs'"; exit 1)
	@echo -n "Checking Port Conflicts in range 20000-29999... "
	@command -v ss >/dev/null 2>&1 || (echo "FAILED: ss not found (install iproute2)"; exit 1)
	@ss_out="$$(ss -tuln)"; \
		if [ -z "$$ss_out" ]; then echo "FAILED: ss produced no listen table"; exit 1; fi; \
		for p in $(PLATFORM_CORE_HOST_PORT) $(PLATFORM_UI_HOST_PORT) $(PLATFORM_POSTGRES_HOST_PORT) $(PLATFORM_CLICKHOUSE_HTTP_HOST_PORT) $(PLATFORM_CLICKHOUSE_NATIVE_HOST_PORT) $(PLATFORM_VALKEY_HOST_PORT); do \
			if printf '%s\n' "$$ss_out" | grep -q ":$$p "; then \
				if [ -z "$$(docker compose ps -q 2>/dev/null)" ]; then \
					echo "PORT CONFLICT: $$p is already bound on host by foreign process"; exit 1; \
				fi; \
			fi; \
		done; \
		echo "OK (No conflicts detected; $$(printf '%s\n' "$$ss_out" | wc -l) ss listen rows)"
	@echo -n "Checking Host Disk Space... "
	@echo "$$(df -h / | awk 'NR==2 {print $$4}') available"
	@echo -n "Checking Host RAM... "
	@command -v free >/dev/null 2>&1 || (echo "FAILED: free not found (install procps)"; exit 1)
	@free_avail="$$(free -h | awk '/^Mem:/ {print $$7}')"; \
		if [ -z "$$free_avail" ]; then echo "FAILED: free did not report Mem available"; exit 1; fi; \
		echo "$$free_avail available"
	@echo "==> Platform Doctor Check Passed."

dev-disk-check: ## Read-only disk/Docker pressure report (70/80/90 thresholds)
	@bash scripts/dev-disk-check.sh

dev-hygiene: ## Safe reclaim: aged/capped build cache + dangling images (+ labeled ephemeral vols)
	@bash scripts/dev-hygiene.sh

dev-build: certs ## Build all Platform container images
	@echo "==> Building Platform containers..."
	@docker compose build

dev-up: certs ## Start Platform development services in background
	@echo "==> Starting Platform services (profile: dev)..."
	@docker compose up -d
	@echo "==> Services initiated. Run 'make dev-status' to verify health."

dev-down: ## Stop Platform development services gracefully (preserves named volumes)
	@echo "==> Stopping Platform services..."
	@docker compose down --remove-orphans

dev-restart: ## Restart Platform services
	@echo "==> Restarting Platform services..."
	@docker compose restart

dev-status: ## Display status and health of Platform services
	@docker compose ps

dev-logs: ## Tail logs for all Platform services
	@docker compose logs -f --tail=100

test-unit: ## Run unit tests inside isolated build containers
	@echo "==> Running containerized unit tests..."
	@bash tests/run-unit-tests.sh

test-static-contracts: ## Run dependency-free Device Pack, provisioning, and presentation contract validators
	@echo "==> Running Platform static contract validation..."
	@bash tests/run-static-contract-validation.sh

test-integration: ## Run full 20-test integration verification suite
	@echo "==> Running Platform 20-test integration suite..."
	@bash tests/run-all-tests.sh

test: test-unit test-integration ## Run both unit and integration tests

test-contract: ## Run API, event, and data model contract verification suite
	@echo "==> Running Platform contract tests..."
	@bash tests/run-contract-tests.sh

test-security-smoke: ## Run mTLS, port isolation, and host runtime security smoke tests
	@echo "==> Running Platform security smoke tests..."
	@bash tests/run-security-smoke.sh

test-telemetry-pipeline: ## Run end-to-end Sensor->Spool->Core->Kafka->ClickHouse pipeline verification
	@echo "==> Running Platform telemetry pipeline verification..."
	@bash tests/run-telemetry-pipeline-test.sh

test-agent-runtime-e2e: ## Real Agent→Sensor Receiver→Agent spool→Core→Kafka path (test-only overlay)
	@echo "==> Running Agent runtime E2E (does not change SENSOR_AGENT_RECEIVER_ENABLED compose default)..."
	@echo "==> Does not regenerate development PKI; run 'make certs' only on a stopped stack if CA files are missing."
	@bash tests/agent-runtime-e2e/run.sh

evidence: test test-contract test-security-smoke ## Run full regression and generate engineering evidence
	@echo "==> Capturing complete GA engineering evidence..."
	@bash scripts/post-reboot-verify.sh

clean: ## Clean local temporary build caches (does NOT delete persistent data)
	@echo "==> Cleaning ephemeral build caches..."
	@rm -rf core/target ui/dist sensor/bin agent/bin .pytest_cache

dev-reset: ## Clean-slate reset of Platform services and volumes (CONFIRM=yes required)
	@if [ "$(CONFIRM)" != "yes" ] && [ "$(FORCE)" != "1" ]; then \
		echo "SAFETY GUARD: dev-reset deletes Platform volumes (platform_pg_data, platform_ch_data, etc.)!"; \
		echo "Re-run with: make dev-reset CONFIRM=yes"; \
		exit 1; \
	fi
	@echo "==> Resetting Platform containers and named volumes..."
	@docker compose down -v --remove-orphans
	@echo "==> Platform dev environment reset complete."

post-reboot-verify: ## Verify stack integrity and convergence after host machine reboot
	@bash scripts/post-reboot-verify.sh
