#!/usr/bin/env sh
set -eu

export SMOKE_RUNTIME=kubernetes
export KUBERNETES_NAMESPACE="${KUBERNETES_NAMESPACE:-go-order-system}"
export BASE_URL="${BASE_URL:-http://127.0.0.1:8082}"
export MYSQL_PASSWORD="${MYSQL_PASSWORD:-local-dev-password}"
export IDENTITY_DB_NAME="${IDENTITY_DB_NAME:-go_order_identity}"
export SMOKE_RUN_SUFFIX="${SMOKE_RUN_SUFFIX:-${GITHUB_RUN_NUMBER:-$(date +%s)}}"

exec sh scripts/smoke/microservices-saga.sh
