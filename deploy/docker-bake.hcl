# ─────────────────────────────────────────────────────────────
# docker-bake.hcl — Build ALL images across the monorepo
# ─────────────────────────────────────────────────────────────
# Usage:
#   docker buildx bake -f deploy/docker-bake.hcl --load
#
# Groups:
#   default    — everything (ingestion + analysis)
#   ingestion  — ingestion-base + all ingestion services
#   analysis   — analysis-base + all analysis services
#
# With cache:
#   docker buildx bake -f deploy/docker-bake.hcl --load \
#     --set *.cache-from=type=gha \
#     --set *.cache-to=type=gha,mode=max
# ─────────────────────────────────────────────────────────────

variable "TAG" {
  default = "latest"
}

variable "REGISTRY" {
  default = ""
}

# ───── Common settings ─────

target "_common" {
  cache-from = ["type=gha"]
  cache-to   = ["type=gha,mode=max"]
}

# ───── Ingestion targets ─────

target "ingestion-base" {
  inherits   = ["_common"]
  context    = "."
  dockerfile = "go-ingestion/build/dockerfiles/Dockerfile.base"
  tags       = ["${REGISTRY}go-dota2-ingestion-base:${TAG}"]
}

target "discoverer" {
  inherits   = ["_common"]
  context    = "."
  dockerfile = "go-ingestion/build/dockerfiles/Dockerfile.discoverer"
  tags       = ["${REGISTRY}go-dota2-discoverer:${TAG}"]
  contexts = {
    "go-dota2-base-local" = "target:ingestion-base"
  }
}

target "fetcher" {
  inherits   = ["_common"]
  context    = "."
  dockerfile = "go-ingestion/build/dockerfiles/Dockerfile.fetcher"
  tags       = ["${REGISTRY}go-dota2-fetcher:${TAG}"]
  contexts = {
    "go-dota2-base-local" = "target:ingestion-base"
  }
}

target "parser" {
  inherits   = ["_common"]
  context    = "."
  dockerfile = "go-ingestion/build/dockerfiles/Dockerfile.parser"
  tags       = ["${REGISTRY}go-dota2-parser:${TAG}"]
  contexts = {
    "go-dota2-base-local" = "target:ingestion-base"
  }
}

target "enricher" {
  inherits   = ["_common"]
  context    = "."
  dockerfile = "go-ingestion/build/dockerfiles/Dockerfile.enricher"
  tags       = ["${REGISTRY}go-dota2-enricher:${TAG}"]
  contexts = {
    "go-dota2-base-local" = "target:ingestion-base"
  }
}

target "dlq" {
  inherits   = ["_common"]
  context    = "."
  dockerfile = "go-ingestion/build/dockerfiles/Dockerfile.dlq"
  tags       = ["${REGISTRY}go-dota2-dlq:${TAG}"]
  contexts = {
    "go-dota2-base-local" = "target:ingestion-base"
  }
}

target "proxyloader" {
  inherits   = ["_common"]
  context    = "."
  dockerfile = "go-ingestion/build/dockerfiles/Dockerfile.proxyloader"
  tags       = ["${REGISTRY}go-dota2-proxyloader:${TAG}"]
  contexts = {
    "go-dota2-base-local" = "target:ingestion-base"
  }
}

target "ingestion-migrator" {
  inherits   = ["_common"]
  context    = "."
  dockerfile = "go-ingestion/build/dockerfiles/Dockerfile.migrator"
  tags       = ["${REGISTRY}go-dota2-ingestion-migrator:${TAG}"]
  contexts = {
    "go-dota2-base-local" = "target:ingestion-base"
  }
}

# ───── Migrator (canonical — shared by ingestion/analysis) ─────

target "migrator" {
  inherits   = ["_common"]
  context    = "."
  dockerfile = "go-ingestion/build/dockerfiles/Dockerfile.migrator"
  tags       = ["${REGISTRY}go-dota2-migrator:${TAG}"]
  contexts = {
    "go-dota2-base-local" = "target:ingestion-base"
  }
}

# ───── Analysis targets ─────

target "analysis-base" {
  inherits   = ["_common"]
  context    = "."
  dockerfile = "go-analysis/build/dockerfiles/Dockerfile.base"
  tags       = ["${REGISTRY}go-dota2-analysis-base:${TAG}"]
}

target "api" {
  inherits   = ["_common"]
  context    = "."
  dockerfile = "go-analysis/build/dockerfiles/Dockerfile.api"
  tags       = ["${REGISTRY}go-dota2-analysis-api:${TAG}"]
  contexts = {
    "go-dota2-analysis-base-local" = "target:analysis-base"
  }
}

target "featurizer" {
  inherits   = ["_common"]
  context    = "."
  dockerfile = "go-analysis/build/dockerfiles/Dockerfile.featurizer"
  tags       = ["${REGISTRY}go-dota2-analysis-featurizer:${TAG}"]
  contexts = {
    "go-dota2-analysis-base-local" = "target:analysis-base"
  }
}

target "backtester" {
  inherits   = ["_common"]
  context    = "."
  dockerfile = "go-analysis/build/dockerfiles/Dockerfile.backtester"
  tags       = ["${REGISTRY}go-dota2-analysis-backtester:${TAG}"]
  contexts = {
    "go-dota2-analysis-base-local" = "target:analysis-base"
  }
}

# ───── Groups ─────

group "ingestion" {
  targets = [
    "ingestion-base",
    "discoverer", "fetcher", "parser", "enricher",
    "dlq", "proxyloader", "ingestion-migrator",
  ]
}

group "analysis" {
  targets = ["analysis-base", "api", "featurizer", "backtester"]
}

group "all-images" {
  targets = ["ingestion", "analysis", "migrator"]
}

group "default" {
  targets = ["all-images"]
}
