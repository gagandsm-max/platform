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

.PHONY: help doctor dev-build dev-up dev-down dev-restart dev-status dev-logs test test-unit test-integration clean dev-reset certs post-reboot-verify

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
	@for p in $(PLATFORM_CORE_HOST_PORT) $(PLATFORM_UI_HOST_PORT) $(PLATFORM_POSTGRES_HOST_PORT) $(PLATFORM_CLICKHOUSE_HTTP_HOST_PORT) $(PLATFORM_CLICKHOUSE_NATIVE_HOST_PORT) $(PLATFORM_VALKEY_HOST_PORT); do \
		if ss -tuln | grep -q ":$$p "; then \
			if [ -z "$$(docker compose ps -q 2>/dev/null)" ]; then \
				echo "PORT CONFLICT: $$p is already bound on host by foreign process"; exit 1; \
			fi; \
		fi; \
	done; echo "OK (No conflicts detected)"
	@echo -n "Checking Host Disk Space... "
	@echo "$$(df -h / | awk 'NR==2 {print $$4}') available"
	@echo -n "Checking Host RAM... "
	@echo "$$(free -h | awk '/^Mem:/ {print $$7}') available"
	@echo "==> Platform Doctor Check Passed."

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

test-integration: ## Run full 20-test integration verification suite
	@echo "==> Running Platform 20-test integration suite..."
	@bash tests/run-all-tests.sh

test: test-unit test-integration ## Run both unit and integration tests

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
