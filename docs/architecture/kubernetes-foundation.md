# Kubernetes foundation

## Scope

This phase mirrors the current Compose runtime in Kubernetes without changing application APIs, database schemas, Saga states or messaging contracts.

The manifests live under:

```text
deploy/kubernetes/
├── base/
│   ├── namespace.yaml
│   ├── configmap.yaml
│   ├── infrastructure.yaml
│   ├── migrations.yaml
│   ├── applications.yaml
│   └── kustomization.yaml
└── overlays/local/
    ├── gateway-nodeport.yaml
    ├── kind-config.yaml
    └── kustomization.yaml
```

## Runtime topology

The local overlay deploys:

- one API Gateway;
- Identity, Catalog, Inventory and Order services;
- two Timeout Worker replicas;
- two Reconciliation Worker replicas;
- one MySQL StatefulSet;
- one RabbitMQ StatefulSet;
- four service-owned Goose migration Jobs.

Redis is intentionally absent because it is not part of the current `main` runtime.

Only the Gateway Service becomes a NodePort in the local overlay. kind maps host port `8082` to NodePort `30082`. MySQL, RabbitMQ and business Services remain cluster-internal.

## Secret contract

The base references one Secret named `app-secrets`. The local overlay generates development-only placeholder values for:

```text
MYSQL_PASSWORD
JWT_SECRET
INTERNAL_SERVICE_TOKEN
RABBITMQ_USER
RABBITMQ_PASSWORD
RABBITMQ_URL
```

These values are not production credentials. A test or production overlay must replace the local generator with a secret-management workflow and must not commit real values.

## Migration ordering

Kubernetes has no Docker Compose-style `depends_on`. The deployment uses two explicit controls:

1. each migration Job waits for MySQL, creates only its owned logical database and runs its Goose directory;
2. each database-backed application Pod has an initContainer that waits for one required table before starting the service binary.

This prevents a workload from becoming ready merely because MySQL accepts connections while its schema is still absent.

## Probes and rollout behavior

HTTP workloads define:

- `startupProbe`;
- `readinessProbe`;
- `livenessProbe`;
- RollingUpdate with `maxUnavailable: 0` and `maxSurge: 1`.

Workers do not expose HTTP ports. Their probes verify that PID 1 remains alive; operational correctness continues to rely on leases, structured logs and reliability indicators.

All long-running workloads and migration Jobs define CPU and memory requests/limits.

## Local deployment

Requirements:

```text
Docker
Docker Compose v2
kind
kubectl
curl
```

Deploy:

```bash
sh scripts/k8s/deploy-local.sh
```

The script:

1. creates `kind-go-order-local` when absent;
2. builds the seven application images;
3. loads them into kind;
4. applies the local overlay;
5. waits for MySQL and RabbitMQ;
6. waits for all migration Jobs;
7. waits for all application rollouts;
8. checks `http://127.0.0.1:8082/readyz`.

Inspect:

```bash
kubectl -n go-order-system get pods,services,jobs
kubectl -n go-order-system logs deployment/order-service
kubectl -n go-order-system logs deployment/order-timeout-worker
```

Delete the cluster:

```bash
kind delete cluster --name go-order-local
```

## Render validation

```bash
kustomize build deploy/kubernetes/overlays/local >/tmp/go-order-kubernetes.yaml
```

CI performs this render check in addition to the existing Compose topology and Order Saga regression.

## Deferred work

This first Kubernetes PR intentionally defers:

- Ingress controller integration;
- PodDisruptionBudget;
- HorizontalPodAutoscaler;
- NetworkPolicy;
- test overlay and immutable Registry image tags;
- live kind deployment in CI;
- rollout failure and `rollout undo` exercise.

Those are separate Phase 6 deliverables so this base remains reviewable and reversible.
