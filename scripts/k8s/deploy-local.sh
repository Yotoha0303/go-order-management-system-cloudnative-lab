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

require_command docker
require_command kind
require_command kubectl

if ! kind get clusters | grep -Fxq "$CLUSTER_NAME"; then
  kind create cluster \
    --name "$CLUSTER_NAME" \
    --config "$OVERLAY/kind-config.yaml"
fi

kubectl config use-context "kind-$CLUSTER_NAME" >/dev/null

printf '%s\n' 'Building application images...'
docker compose build \
  api-gateway \
  identity-service \
  catalog-service \
  inventory-service \
  order-service \
  order-timeout-worker \
  order-reconciliation-worker

images='
go-order-management-system/api-gateway:local
go-order-management-system/identity-service:local
go-order-management-system/catalog-service:local
go-order-management-system/inventory-service:local
go-order-management-system/order-service:local
go-order-management-system/order-timeout-worker:local
go-order-management-system/order-reconciliation-worker:local
'

printf '%s\n' 'Loading images into kind...'
for image in $images; do
  kind load docker-image "$image" --name "$CLUSTER_NAME"
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
curl --fail --silent --show-error http://127.0.0.1:8082/readyz
printf '\nKubernetes local topology is ready at http://127.0.0.1:8082\n'
kubectl -n "$NAMESPACE" get pods,services,jobs
