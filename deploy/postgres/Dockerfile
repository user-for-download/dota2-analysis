# syntax=docker/dockerfile:1
# ─────────────────────────────────────────────────────────────
# PostgreSQL 16 with pg_cron extension
# ─────────────────────────────────────────────────────────────
# pg_cron is compiled from the official Citus source so the
# image is self-contained and does not depend on any external
# registry beyond Docker Hub for the base image.
# ─────────────────────────────────────────────────────────────

FROM postgres:16-alpine AS builder

RUN set -eux; \
    apk add --no-cache --virtual .pg-cron-build-deps \
        build-base \
        clang19 \
        git \
        llvm19 \
        postgresql16-dev; \
    git clone --branch v1.6.7 --depth 1 \
        https://github.com/citusdata/pg_cron.git /tmp/pg_cron; \
    make -C /tmp/pg_cron -j"$(nproc)"; \
    make -C /tmp/pg_cron install; \
    rm -rf /tmp/pg_cron; \
    apk del .pg-cron-build-deps

FROM postgres:16-alpine

COPY --from=builder \
    /usr/local/lib/postgresql/pg_cron* \
    /usr/local/lib/postgresql/

COPY --from=builder \
    /usr/local/share/postgresql/extension/pg_cron* \
    /usr/local/share/postgresql/extension/

HEALTHCHECK --interval=5s --timeout=3s --retries=5 \
    CMD pg_isready -U "${POSTGRES_USER:-dota2}"
