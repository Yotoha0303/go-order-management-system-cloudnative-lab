# Kubernetes foundation

## Scope

The project mirrors its current Compose runtime in Kubernetes without changing application APIs, database schemas, Saga states or messaging contracts.

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
    ├── fast-timeout-config.yaml
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

The base references one Secret named `app-secrets`. The local overlay generates development-only values for:

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
2. builds the seven application images directly from the shared service Dockerfile;
3. loads them into kind;
4. applies the local overlay;
5. waits for MySQL and RabbitMQ;
6. waits for all migration Jobs;
7. waits for all application rollouts;
8. applies a bounded Gateway readiness retry window.

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

## Automated runtime verification

CI now has two independent jobs.

### Quality and Compose regression

This job validates:

- Go lint, test, race, vet and build;
- legacy and service-owned migrations;
- Kustomize rendering;
- all Compose images;
- four databases, RabbitMQ and both Worker types;
- the complete Compose Order Saga.

### kind deployment, recovery and Saga

After the quality job succeeds, CI creates a clean disposable kind cluster and verifies:

1. the local overlay applies through the Kubernetes API;
2. MySQL and RabbitMQ StatefulSets become ready;
3. all four migration Jobs complete;
4. Gateway, four business services and both Worker Deployments become ready;
5. only the Gateway is a NodePort;
6. two Timeout and two Reconciliation Worker replicas are ready;
7. an unavailable Gateway image causes rollout failure;
8. `kubectl rollout undo` restores the previous image and readiness;
9. the complete register/catalog/inventory/order/pay/cancel/timeout Saga passes after recovery;
10. the cluster is always deleted.

Kubernetes administrator promotion uses an explicit TCP connection to MySQL inside the Pod. The common Saga script is shared with Compose so both environments execute the same business assertions.

On failure, CI uploads:

- Kubernetes resources and Services;
- Jobs and PVCs;
- sorted events;
- Pod descriptions;
- all container logs;
- exported kind node logs.

## Verification commands

Render:

```bash
kustomize build deploy/kubernetes/overlays/local >/tmp/go-order-kubernetes.yaml
```

Kubernetes Saga against an already-running local cluster:

```bash
sh scripts/smoke/microservices-saga-kubernetes.sh
```

## Remaining Kubernetes work

The following remain outside the verified foundation:

- Ingress controller integration;
- PodDisruptionBudget;
- HorizontalPodAutoscaler;
- NetworkPolicy;
- immutable Registry image tags and non-local overlays;
- multi-node failure behavior;
- managed-cloud storage, load balancer and identity integration.
