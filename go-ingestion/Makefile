# ─────────────────────────────────────────────────────────────
# go-dota2 — build & run
# ─────────────────────────────────────────────────────────────
BAKE_FILE    := ../deploy/docker-bake.hcl
COMPOSE_FILE := ../deploy/docker-compose.yml
PROJECT_NAME := go-dota2
TAG          ?= latest

# Suppress entitlement prompt: bake contexts use ".." to reach repo root.
export BUILDX_BAKE_ENTITLEMENTS_FS=0

COMPOSE := docker compose -p $(PROJECT_NAME) -f $(COMPOSE_FILE)
BAKE    := TAG=$(TAG) docker buildx bake --file $(BAKE_FILE)

.DEFAULT_GOAL := help
.PHONY: help build rebuild up upd up-db down downv logs \
        shell-db shell-redis migrate migrate-local vendor armageddon

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
	  awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

# ───── Dependencies ─────
vendor: ## Vendor all dependencies (run after any go.mod change)
	go mod vendor

# ───── Build ─────
build: vendor ## Build all service images (cached)
	$(BAKE) --load

rebuild: vendor ## Force-rebuild all images without cache
	$(BAKE) --load --no-cache --set *.args.GOMAXPROCS=2

# ───── Run ─────
up: ## Start the full pipeline (foreground)
	$(COMPOSE) --profile all up

upd: ## Start the full pipeline (detached)
	$(COMPOSE) --profile all up -d

up-db: ## Start only Redis + Postgres
	$(COMPOSE) --profile db up -d

down: ## Stop all services and remove containers
	$(COMPOSE) --profile all down

downv: ## Stop all services and remove volumes
	$(COMPOSE) --profile all down -v

logs: ## Follow logs for all services
	$(COMPOSE) --profile all logs -f

# ───── Migrate ─────
migrate: vendor ## Run DB migrations (builds image, runs once)
	$(BAKE) migrator --load
	$(COMPOSE) --profile db --profile migrate run --rm migrator

migrate-local: ## Run migrator against local postgres (uses embedded migrations from go-core)
	go run -mod=mod ./cmd/migrator

shell-db: ## Open psql shell
	-$(COMPOSE) exec postgres psql -U $${POSTGRES_USER:-dota2} -d $${POSTGRES_DB:-dota2} -c " SELECT COUNT(*) AS matches, COUNT(*) FILTER (WHERE is_parsed) AS parsed FROM matches;"

shell-redis: ## Open redis-cli shell
	-$(COMPOSE) exec redis redis-cli

# ───── Danger zone ─────
armageddon: ## Remove all Docker containers, networks, volumes, images for this project
	@echo "--- Nuking go-dota2 Docker resources ---"
	$(COMPOSE) --profile all down -v --rmi all --remove-orphans
	@docker builder prune -af --filter=label=project=$(PROJECT_NAME)
	@echo "--- Project nuked (other Docker resources preserved) ---"
