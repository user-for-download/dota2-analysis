# ─────────────────────────────────────────────────────────────
# go-dota2-analysis — build & run
# ─────────────────────────────────────────────────────────────
BAKE_FILE       := ../deploy/docker-bake.hcl
COMPOSE_FILE    := ../deploy/compose/compose.analysis.yml
SIBLING_COMPOSE := ../deploy/docker-compose.yml
PROJECT_NAME    := go-dota2
TAG             ?= latest

# Suppress entitlement prompt: bake contexts use ".." to reach repo root.
export BUILDX_BAKE_ENTITLEMENTS_FS=0

COMPOSE := docker compose -p $(PROJECT_NAME) -f $(COMPOSE_FILE)
BAKE    := TAG=$(TAG) docker buildx bake --file $(BAKE_FILE)

.DEFAULT_GOAL := help
.PHONY: help build rebuild up upd down downv logs \
        shell-db armageddon \
        vendor train backtest

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
	  awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

# ───── Dependencies ─────
vendor: ## Vendor all dependencies (run after any go.mod change)
	go mod vendor

# ───── Build ─────
build: vendor ## Build all service images (cached)
	mkdir -p assets/models/imitation/current assets/models/value/current
	$(BAKE) --load

rebuild: vendor ## Force-rebuild all images without cache
	$(BAKE) --load --no-cache --set *.args.GOMAXPROCS=2

# ───── Run ─────
up: ## Start the full pipeline (foreground)
	$(COMPOSE) --profile all up

upd: ## Start the full pipeline (detached)
	$(COMPOSE) --profile all up -d

down: ## Stop analysis services only
	$(COMPOSE) --profile all down

downv: ## Stop analysis services and remove their volumes
	$(COMPOSE) --profile all down -v

logs: ## Follow logs for analysis services
	$(COMPOSE) --profile all logs -f api featurizer backtester

# ───── Train & Evaluate ─────
train: ## Train a new LightGBM model (one-shot)
	$(COMPOSE) --profile training up --build trainer

backtest: ## Run backtester against current model
	$(COMPOSE) --profile all up --build backtester

# ───── Shell ─────
shell-db: ## Open psql shell
	-docker compose -p $(PROJECT_NAME) -f $(SIBLING_COMPOSE) exec postgres psql -U $${POSTGRES_USER:-dota2} -d $${POSTGRES_DB:-dota2}

# ───── Danger zone ─────
armageddon: ## Remove analysis containers and images (preserves sibling project)
	@echo "--- Removing go-dota2-analysis services ---"
	-$(COMPOSE) --profile all rm -fs api featurizer backtester trainer
	@docker image rm -f go-dota2-analysis-api:$(TAG) go-dota2-analysis-featurizer:$(TAG) go-dota2-analysis-backtester:$(TAG) 2>/dev/null || true
	@docker builder prune -f --filter=label=project=$(PROJECT_NAME) 2>/dev/null || true
	@echo "--- Analysis services removed (sibling project preserved) ---"
