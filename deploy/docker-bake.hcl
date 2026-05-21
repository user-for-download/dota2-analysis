# ─────────────────────────────────────────────────────────────
# docker-bake.hcl — Build ALL images across the monorepo
# ─────────────────────────────────────────────────────────────
# Usage:
#   docker buildx bake -f deploy/docker-bake.hcl --load
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

target "ingestion-base" {
  context     = "go-ingestion"
  dockerfile  = "deploy/dockerfiles/Dockerfile.base"
  tags        = ["${REGISTRY}dota2-ingestion-base:${TAG}"]
  cache-from  = ["type=gha"]
  cache-to    = ["type=gha,mode=max"]
}

target "discoverer" {
  context     = "go-ingestion"
  dockerfile  = "deploy/dockerfiles/Dockerfile.discoverer"
  tags        = ["${REGISTRY}dota2-discoverer:${TAG}"]
  cache-from  = ["type=gha"]
  cache-to    = ["type=gha,mode=max"]
}

target "fetcher" {
  context     = "go-ingestion"
  dockerfile  = "deploy/dockerfiles/Dockerfile.fetcher"
  tags        = ["${REGISTRY}dota2-fetcher:${TAG}"]
  cache-from  = ["type=gha"]
  cache-to    = ["type=gha,mode=max"]
}

target "parser" {
  context     = "go-ingestion"
  dockerfile  = "deploy/dockerfiles/Dockerfile.parser"
  tags        = ["${REGISTRY}dota2-parser:${TAG}"]
  cache-from  = ["type=gha"]
  cache-to    = ["type=gha,mode=max"]
}

target "enricher" {
  context     = "go-ingestion"
  dockerfile  = "deploy/dockerfiles/Dockerfile.enricher"
  tags        = ["${REGISTRY}dota2-enricher:${TAG}"]
  cache-from  = ["type=gha"]
  cache-to    = ["type=gha,mode=max"]
}

target "dlq" {
  context     = "go-ingestion"
  dockerfile  = "deploy/dockerfiles/Dockerfile.dlq"
  tags        = ["${REGISTRY}dota2-dlq:${TAG}"]
  cache-from  = ["type=gha"]
  cache-to    = ["type=gha,mode=max"]
}

target "proxyloader" {
  context     = "go-ingestion"
  dockerfile  = "deploy/dockerfiles/Dockerfile.proxyloader"
  tags        = ["${REGISTRY}dota2-proxyloader:${TAG}"]
  cache-from  = ["type=gha"]
  cache-to    = ["type=gha,mode=max"]
}

target "ingestion-migrator" {
  context     = "go-ingestion"
  dockerfile  = "deploy/dockerfiles/Dockerfile.migrator"
  tags        = ["${REGISTRY}dota2-ingestion-migrator:${TAG}"]
  cache-from  = ["type=gha"]
  cache-to    = ["type=gha,mode=max"]
}

target "analysis-base" {
  context     = "go-analysis"
  dockerfile  = "deploy/dockerfiles/Dockerfile.base"
  tags        = ["${REGISTRY}dota2-analysis-base:${TAG}"]
  cache-from  = ["type=gha"]
  cache-to    = ["type=gha,mode=max"]
}

target "api" {
  context     = "go-analysis"
  dockerfile  = "deploy/dockerfiles/Dockerfile.api"
  tags        = ["${REGISTRY}dota2-api:${TAG}"]
  cache-from  = ["type=gha"]
  cache-to    = ["type=gha,mode=max"]
}

target "featurizer" {
  context     = "go-analysis"
  dockerfile  = "deploy/dockerfiles/Dockerfile.featurizer"
  tags        = ["${REGISTRY}dota2-featurizer:${TAG}"]
  cache-from  = ["type=gha"]
  cache-to    = ["type=gha,mode=max"]
}

target "backtester" {
  context     = "go-analysis"
  dockerfile  = "deploy/dockerfiles/Dockerfile.backtester"
  tags        = ["${REGISTRY}dota2-backtester:${TAG}"]
  cache-from  = ["type=gha"]
  cache-to    = ["type=gha,mode=max"]
}

# ───── Groups ─────

group "ingestion" {
  targets = ["ingestion-base", "discoverer", "fetcher", "parser", "enricher", "dlq", "proxyloader", "ingestion-migrator"]
}

group "analysis" {
  targets = ["analysis-base", "api", "featurizer", "backtester"]
}

group "default" {
  targets = ["ingestion", "analysis"]
}
