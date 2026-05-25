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
        ingestion analysis \
        train backtest \
        shell-db shell-redis \
        publish-core \
        armageddon

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

migrate: ## Run database migrations
	$(COMPOSE) --profile all run --rm migrator

ingestion: ## Start the ingestion pipeline
	$(COMPOSE) --profile ingestion up

analytics: ## Start the analytics pipeline
	$(COMPOSE) --profile analytics up

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
	cd go-analysis && docker compose -f deploy/docker-compose.yml --profile training up --build trainer

backtest: ## Run backtester against current model
	$(COMPOSE) --profile analytics run --rm backtester

# ───── Shell ─────
shell-db: ## Open psql shell
	$(COMPOSE) exec postgres psql -U $${POSTGRES_USER:-dota2} -d $${POSTGRES_DB:-dota2}

shell-redis: ## Open redis-cli
	$(COMPOSE) exec redis redis-cli

# ───── Publish ─────
publish-core: ## Tag and push go-core module (run after version bump)
	@echo "=== Publishing go-dota2-core ==="
	@read -p "Version (e.g. v1.0.0): " VERSION; \
	cd go-core && git tag $$VERSION && git push origin $$VERSION
	@echo "Tag pushed. Update downstream go.mod files:"
	@echo "  cd go-ingestion && go get github.com/user-for-download/go-dota2-core@$$VERSION"
	@echo "  cd go-analysis && go get github.com/user-for-download/go-dota2-core@$$VERSION"
	@echo "Then remove replace directives from go.mod and run: go mod tidy"

# ───── Schema ─────
new-migration: ## Create a new schema migration: make new-migration NAME=add_foo_column
	@test -n "$(NAME)" || (echo "Usage: make new-migration NAME=add_foo_column" && exit 1)
	@echo "$(NAME)" | grep -qE '^[a-z][a-z0-9_]*$$' || \
	  (echo "NAME must be snake_case lowercase: ^[a-z][a-z0-9_]*$$" && exit 1)
	@n=$$(printf "%03d" $$(( $$(ls go-core/schema/migrations/*.sql 2>/dev/null | wc -l) + 1 ))); \
	 f="go-core/schema/migrations/$${n}_$(NAME).sql"; \
	 echo "-- $${n}_$(NAME).sql" > $$f; \
	 echo "Created $$f"
	@echo "Next: write the SQL, update contract tests, then run:"
	@echo "  POSTGRES_TEST_DSN=... go test ./go-core/contracttest/..."

# ───── Validate ─────
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
dump-db:
	docker exec dota2-postgres pg_dump -U dota2 -Fc dota2 > /home/ubuntu/dota2_dump.dump

restore-db:
    docker exec -i dota2-postgres pg_restore -U dota2 -d dota2 --clean --if-exists < /home/ubuntu/dota2_dump.dump
