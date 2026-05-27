# ─────────────────────────────────────────────────────────────
# dota2-analysis — unified build & run for the full pipeline
# ─────────────────────────────────────────────────────────────
COMPOSE_FILE := deploy/docker-compose.yml
PROJECT_NAME := dota2-analysis
TAG         ?= latest

COMPOSE := docker compose -p $(PROJECT_NAME) --project-directory . -f $(COMPOSE_FILE)

.DEFAULT_GOAL := help
.PHONY: help build build-ingestion build-analysis \
        vendor \
        fetch-constants fetch-proxies bootstrap clean-assets \
        up upd down downv logs \
        infra migrate \
        ingestion analytics \
        train backtest \
        shell-db shell-redis \
        publish-core \
        test vet new-migration migrate-local \
        db-backup db-restore \
        prune armageddon \
        shell-db shell-redis

# ───── Help ─────
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | sort | \
	  awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ───── Vendor (no-op — vendoring not used) ─────
vendor: ## No-op — see README for module resolution
	@echo "This monorepo does not use vendor/ directories."
	@echo "Module resolution: go.work + replace directives + GOPROXY."

# ───── Build ─────
build-ingestion: ## Build all ingestion Docker images
	TAG=$(TAG) docker buildx bake -f deploy/docker-bake.hcl --load ingestion

build-analysis: ## Build all analysis Docker images
	TAG=$(TAG) docker buildx bake -f deploy/docker-bake.hcl --load analysis

build: ## Build all Docker images
	TAG=$(TAG) docker buildx bake -f deploy/docker-bake.hcl --load

# ───── Run ─────
infra: ## Start infrastructure (Postgres + Redis + Jaeger)
	$(COMPOSE) --profile infra up

migrate: ## Run database migrations (via Docker compose)
	$(COMPOSE) --profile migrate run --rm migrator

migrate-local: ## Run migrations against local Postgres (no Docker)
	go run -mod=mod ./go-ingestion/cmd/migrator

ingestion: ## Start the ingestion pipeline
	$(COMPOSE) --profile ingestion up

analytics: ## Start the analytics pipeline
	$(COMPOSE) --profile analytics up --force-recreate

up: infra migrate ingestion analytics ## Start everything

upd: ## Start everything (detached)
	$(COMPOSE) --profile all up --force-recreate

down: ## Stop everything
	$(COMPOSE) --profile all down --remove-orphans

downv: ## Stop everything and remove volumes
	$(COMPOSE) --profile all down -v --remove-orphans

logs: ## Follow all logs
	$(COMPOSE) --profile all logs -f

# ───── ML ─────
train: ## Train a new LightGBM model (one-shot)
	$(COMPOSE) -f deploy/compose/compose.analysis.yml --profile training up --build trainer

backtest: ## Run backtester against current model
	$(COMPOSE) --profile analytics run --rm backtester

# ───── Shell ─────
shell-db: ## Open psql shell
	$(COMPOSE) exec postgres psql -U $${POSTGRES_USER:-dota2} -d $${POSTGRES_DB:-dota2}

shell-redis: ## Open redis-cli
	$(COMPOSE) exec redis env REDISCLI_AUTH=$${REDIS_PASSWORD:-dota2} redis-cli

# ───── Bootstrap external assets ─────
fetch-constants: ## Download dotaconstants JSON from OpenDota
	@./tools/fetch-dotaconstants/fetch.sh

fetch-proxies: ## Download free proxy list from ProxyScrape
	@./tools/fetch-dotaconstants/fetch-proxies.sh

bootstrap: fetch-constants fetch-proxies ## One-shot dev setup
	@echo "Bootstrap complete."

clean-assets: ## Remove fetched dotaconstants JSON files
	@find go-ingestion/assets/dotaconstants -maxdepth 1 -name '*.json' -delete
	@echo "Cleaned dotaconstants JSON files."

test: ## Run all unit tests (from workspace root)
	go test -short -count=1 ./go-core/... ./go-analysis/... ./go-ingestion/...

vet: ## Run go vet on all modules (from workspace root)
	go vet ./go-core/...
	go vet ./go-analysis/...
	go vet ./go-ingestion/...

# ───── Danger ─────
prune: ## Stop and remove ALL Docker containers (use with extreme caution)
	@echo "--- Stopping all containers ---"
	docker stop $(shell docker ps -aq) 2>/dev/null || true
	@echo "--- Removing all containers ---"
	docker rm $(shell docker ps -aq) 2>/dev/null || true
	@echo "--- Done ---"

armageddon: prune ## Nuke all Docker resources (containers, images, volumes, networks)
	@echo "--- Pruning networks ---"
	docker network prune -f
	@echo "--- Removing dangling images ---"
	docker rmi -f $(shell docker images --filter dangling=true -qa) 2>/dev/null || true
	@echo "--- Removing dangling volumes ---"
	docker volume rm $(shell docker volume ls --filter dangling=true -q) 2>/dev/null || true
	@echo "--- Removing all images ---"
	docker rmi -f $(shell docker images -qa) 2>/dev/null || true
	@echo "--- Everything wiped ---"

#--------db----------
# Define variables
DB_CONTAINER = dota2-postgres
DB_USER = dota2
DB_NAME = dota2
BACKUP_DIR = ./backups
TS := $(shell date +%Y%m%d_%H%M%S)

.PHONY: db-backup db-restore

db-backup: ## Create a full base backup (Roles + Data)
	@echo "Starting backup process..."
	@mkdir -p $(BACKUP_DIR)
	@echo "1/2: Exporting global roles..."
	@docker exec $(DB_CONTAINER) pg_dumpall -U $(DB_USER) --roles-only > $(BACKUP_DIR)/roles_$(TS).sql
	@echo "2/2: Exporting database (Custom Format)..."
	@docker exec $(DB_CONTAINER) pg_dump -U $(DB_USER) -Fc $(DB_NAME) > $(BACKUP_DIR)/dota2_$(TS).dump
	@echo "Backup complete! Files saved to $(BACKUP_DIR)/"
	@ls -lh $(BACKUP_DIR)/*$(TS)*

db-restore: ## Restore a database backup. Usage: make db-restore DUMP=file.dump ROLES=roles.sql
	@if [ -z "$(DUMP)" ] || [ -z "$(ROLES)" ]; then \
		echo "ERROR: You must specify the backup files."; \
		echo "Usage: make db-restore DUMP=./backups/dota2_xxx.dump ROLES=./backups/roles_xxx.sql"; \
		exit 1; \
	fi
	@echo "WARNING: This will completely wipe the existing '$(DB_NAME)' database."
	@printf "Are you sure? (y/N) "; \
	read confirm; \
	if [ "$$confirm" != "y" ]; then echo "Restore aborted."; exit 1; fi

	@echo "1/5: Restoring global roles..."
	@cat $(ROLES) | docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d postgres > /dev/null 2>&1 || { echo "ERROR: roles restore failed" >&2; exit 1; }

	@echo "2/5: Dropping existing database..."
	@docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d postgres -c "DROP DATABASE IF EXISTS $(DB_NAME) WITH (FORCE);"

	@echo "3/5: Creating fresh database..."
	@docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d postgres -c "CREATE DATABASE $(DB_NAME);"

	@echo "4/5: Copying dump file to container..."
	@docker cp $(DUMP) $(DB_CONTAINER):/tmp/db_restore.dump

	@echo "5/5: Restoring data sequentially (prevents trigger conflicts)..."
	@docker exec -i $(DB_CONTAINER) pg_restore -U $(DB_USER) -d $(DB_NAME) /tmp/db_restore.dump

	@echo "Cleaning up temporary files..."
	@docker exec -i $(DB_CONTAINER) rm /tmp/db_restore.dump

	@echo "Restore complete!"
