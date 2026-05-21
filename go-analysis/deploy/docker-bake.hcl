variable "TAG" {
  default = "latest"
}

group "default" {
  targets     = ["api", "featurizer", "backtester"]
  parallelism = 2
}

target "base" {
  context    = "."
  dockerfile = "deploy/dockerfiles/Dockerfile.base"
  tags       = ["go-dota2-analysis-base:${TAG}"]
  args = {
    GOMAXPROCS = "2"
  }
}

target "_common" {
  context    = "."
  depends_on = ["base"]
  contexts = {
    "go-dota2-analysis-base-local" = "target:base"
  }
  args = {
    GOMAXPROCS = "2"
  }
}

target "api" {
  inherits   = ["_common"]
  dockerfile = "deploy/dockerfiles/Dockerfile.api"
  tags       = ["go-dota2-analysis-api:${TAG}"]
}

target "featurizer" {
  inherits   = ["_common"]
  dockerfile = "deploy/dockerfiles/Dockerfile.featurizer"
  tags       = ["go-dota2-analysis-featurizer:${TAG}"]
}

target "backtester" {
  inherits   = ["_common"]
  dockerfile = "deploy/dockerfiles/Dockerfile.backtester"
  tags       = ["go-dota2-analysis-backtester:${TAG}"]
}
