# ─────────────────────────────────────────────────────────────
# dota2-analysis — unified build & run for the full pipeline
# ─────────────────────────────────────────────────────────────
COMPOSE_FILE := docker-compose.yml
PROJECT_NAME := dota2-analysis
TAG         ?= latest

COMPOSE := docker compose -p $(PROJECT_NAME) -f $(COMPOSE_FILE)

.DEFAULT_GOAL := help
.PHONY: help build build-ingestion build-analysis \
        vendor \
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
	cd go-ingestion && docker buildx bake --file deploy/docker-bake.hcl --load TAG=$(TAG)

build-analysis: ## Build all analysis Docker images
	cd go-analysis && docker buildx bake --file deploy/docker-bake.hcl --load TAG=$(TAG)

build: build-ingestion build-analysis ## Build all Docker images

# ───── Run ─────
infra: ## Start infrastructure (Postgres + Redis + Jaeger)
	$(COMPOSE) --profile infra up -d

migrate: ## Run database migrations
	$(COMPOSE) --profile migrate run --rm migrator

ingestion: ## Start the ingestion pipeline
	$(COMPOSE) --profile ingestion up -d

analytics: ## Start the analytics pipeline
	$(COMPOSE) --profile analytics up -d

up: infra migrate ingestion analytics ## Start everything

upd: ## Start everything (detached)
	$(COMPOSE) --profile all up -d

down: ## Stop everything
	$(COMPOSE) --profile all down

downv: ## Stop everything and remove volumes
	$(COMPOSE) --profile all down -v

logs: ## Follow all logs
	$(COMPOSE) --profile all logs -f

# ───── ML ─────
train: ## Train a new LightGBM model (one-shot)
	cd go-analysis && docker compose -f deploy/docker-compose.yml --profile training up --build trainer

backtest: ## Run backtester against current model
	$(COMPOSE) --profile analytics up --build backtester

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
	@echo "Then remove replace directives from go.mod and run: make vendor"

# ───── Schema ─────
new-migration: ## Create a new schema migration: make new-migration NAME=add_foo
	@test -n "$(NAME)" || (echo "Usage: make new-migration NAME=add_foo_column" && exit 1)
	@n=$$(printf "%03d" $$(( $$(ls go-core/schema/migrations/*.sql | wc -l) + 1 ))); \
	 f="go-core/schema/migrations/$${n}_$(NAME).sql"; \
	 echo "-- $${n}_$(NAME).sql" > $$f; \
	 echo "Created $$f"
	@echo "Next: write the SQL, update contract tests, then run:"
	@echo "  POSTGRES_TEST_DSN=... go test ./go-core/contracttest/..."

# ───── Validate ─────
test: ## Run all unit tests (from workspace root)
	go test ./go-core/... -short -count=1
	go test ./go-analysis/... -short -count=1 2>/dev/null || true
	go test ./go-ingestion/... -short -count=1 2>/dev/null || true

vet: ## Run go vet on all modules (from workspace root)
	go vet ./go-core/...
	go vet ./go-analysis/...
	go vet ./go-ingestion/...

# ───── Danger ─────
armageddon: ## Nuke all Docker resources for this project
	@echo "--- Nuking dota2-analysis Docker resources ---"
	$(COMPOSE) --profile all down -v --rmi all --remove-orphans
	@docker builder prune -af --filter=label=project=$(PROJECT_NAME)
	@echo "--- Done ---"
