variable "TAG" { default = "latest" }

group "default" {
  targets = ["fetcher", "discoverer", "enricher", "proxyloader", "migrator", "parser"]
  parallelism = 2
}

target "base" {
  context    = "."
  dockerfile = "deploy/dockerfiles/Dockerfile.base"
  tags       = ["go-dota2-base:${TAG}"]
  args = {
    GOMAXPROCS = "2"
  }
}

target "_common" {
  context    = "."
  depends_on = ["base"]
  contexts = {
    "go-dota2-base-local" = "target:base"
  }
  args = {
    GOMAXPROCS = "2"
  }
}

target "fetcher" {
  inherits   = ["_common"]
  dockerfile = "deploy/dockerfiles/Dockerfile.fetcher"
  tags       = ["go-dota2-fetcher:${TAG}"]
  args = {
    GOMAXPROCS = "2"
  }
}

target "parser" {
  inherits   = ["_common"]
  dockerfile = "deploy/dockerfiles/Dockerfile.parser"
  tags       = ["go-dota2-parser:${TAG}"]
  args = {
    GOMAXPROCS = "2"
  }
}

target "enricher" {
  inherits   = ["_common"]
  dockerfile = "deploy/dockerfiles/Dockerfile.enricher"
  tags       = ["go-dota2-enricher:${TAG}"]
  args = {
    GOMAXPROCS = "2"
  }
}

target "proxyloader" {
  inherits   = ["_common"]
  dockerfile = "deploy/dockerfiles/Dockerfile.proxyloader"
  tags       = ["go-dota2-proxyloader:${TAG}"]
  args = {
    GOMAXPROCS = "2"
  }
}

target "migrator" {
  inherits   = ["_common"]
  dockerfile = "deploy/dockerfiles/Dockerfile.migrator"
  tags       = ["go-dota2-migrator:${TAG}"]
  args = {
    GOMAXPROCS = "2"
  }
}

target "discoverer" {
  inherits   = ["_common"]
  dockerfile = "deploy/dockerfiles/Dockerfile.discoverer"
  tags       = ["go-dota2-discoverer:${TAG}"]
  args = {
    GOMAXPROCS = "2"
  }
}