#!/usr/bin/env sh
set -eu

CLUSTER_NAME="${CLUSTER_NAME:-go-order-local}"
NAMESPACE="${KUBERNETES_NAMESPACE:-go-order-system}"
OVERLAY="deploy/kubernetes/overlays/local"

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    printf 'required command not found: %s\n' "$1" >&2
    exit 1
  }
}

wait_for_http() {
  url="$1"
  attempts="${2:-30}"
  delay="${3:-2}"
  current=1
  while [ "$current" -le "$attempts" ]; do
    if curl --fail --silent --show-error "$url"; then
      return 0
    fi
    if [ "$current" -eq "$attempts" ]; then
      break
    fi
    current=$((current + 1))
    sleep "$delay"
  done
  printf 'HTTP endpoint did not become ready: %s\n' "$url" >&2
  return 1
}

require_command docker
require_command kind
require_command kubectl
require_command curl

if ! kind get clusters | grep -Fxq "$CLUSTER_NAME"; then
  kind create cluster \
    --name "$CLUSTER_NAME" \
    --config "$OVERLAY/kind-config.yaml"
fi

kubectl config use-context "kind-$CLUSTER_NAME" >/dev/null

services='
api-gateway
identity-service
catalog-service
inventory-service
order-service
order-timeout-worker
order-reconciliation-worker
'

printf '%s\n' 'Building application images...'
for service in $services; do
  docker build \
    --file deploy/docker/Dockerfile.service \
    --build-arg "SERVICE=$service" \
    --tag "go-order-management-system/$service:local" \
    .
done

printf '%s\n' 'Loading images into kind...'
for service in $services; do
  kind load docker-image \
    "go-order-management-system/$service:local" \
    --name "$CLUSTER_NAME"
done

kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# Job pod templates are immutable. Remove previous one-shot migrations before reapplying.
kubectl -n "$NAMESPACE" delete job \
  identity-migrate \
  catalog-migrate \
  inventory-migrate \
  ordering-migrate \
  --ignore-not-found

printf '%s\n' 'Applying local Kustomize overlay...'
kubectl apply -k "$OVERLAY"

printf '%s\n' 'Waiting for stateful infrastructure...'
kubectl -n "$NAMESPACE" rollout status statefulset/mysql --timeout=240s
kubectl -n "$NAMESPACE" rollout status statefulset/rabbitmq --timeout=240s

printf '%s\n' 'Waiting for service-owned migrations...'
for job in identity-migrate catalog-migrate inventory-migrate ordering-migrate; do
  kubectl -n "$NAMESPACE" wait --for=condition=complete "job/$job" --timeout=240s
done

printf '%s\n' 'Waiting for application rollouts...'
for deployment in \
  identity-service \
  catalog-service \
  inventory-service \
  order-service \
  order-timeout-worker \
  order-reconciliation-worker \
  api-gateway; do
  kubectl -n "$NAMESPACE" rollout status "deployment/$deployment" --timeout=300s
done

printf '%s\n' 'Checking Gateway readiness...'
wait_for_http http://127.0.0.1:8082/readyz 30 2
printf '\nKubernetes local topology is ready at http://127.0.0.1:8082\n'
kubectl -n "$NAMESPACE" get pods,services,jobs
