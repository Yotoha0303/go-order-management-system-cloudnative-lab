# Kubernetes foundation

## Scope

The project mirrors its Compose runtime in Kubernetes without changing public APIs, database schemas, Saga states or messaging contracts.

The Kubernetes package now has three layers:

```text
deploy/kubernetes/
├── base/
│   ├── namespace.yaml
│   ├── configmap.yaml
│   ├── infrastructure.yaml
│   ├── migrations.yaml
│   ├── applications.yaml
│   └── kustomization.yaml
└── overlays/
    ├── local/
    │   ├── gateway-nodeport.yaml
    │   ├── fast-timeout-config.yaml
    │   ├── kind-config.yaml
    │   └── kustomization.yaml
    └── test/
        ├── ingress.yaml
        ├── pod-disruption-budgets.yaml
        ├── README.md
        └── kustomization.yaml
```

## Shared runtime topology

The base defines:

- API Gateway;
- Identity, Catalog, Inventory and Order services;
- Timeout Worker and Reconciliation Worker;
- MySQL and RabbitMQ StatefulSets;
- four service-owned Goose migration Jobs;
- internal Services;
- startup, readiness and liveness probes;
- CPU and memory requests/limits;
- RollingUpdate strategies.

Redis is intentionally absent because it is not part of the current runtime.

## Data and migration ordering

Kubernetes has no Docker Compose-style `depends_on`. The deployment uses two explicit controls:

1. every migration Job waits for MySQL, creates only its owned logical database and runs its Goose directory;
2. every database-backed application Pod has an initContainer that waits for a required table before starting the service binary.

This prevents a workload from becoming ready merely because MySQL accepts connections while its schema is still absent.

## Secret contract

The base references one Secret named `app-secrets` with:

```text
MYSQL_PASSWORD
JWT_SECRET
INTERNAL_SERVICE_TOKEN
RABBITMQ_USER
RABBITMQ_PASSWORD
RABBITMQ_URL
```

The committed local and test values are development/test placeholders only. A real deployment workflow must replace them with credentials from its secret-management system.

## Local overlay

The local overlay is optimized for disposable kind verification:

- one Gateway exposed through NodePort;
- host port `8082` mapped to NodePort `30082`;
- local Secret values;
- shortened timeout configuration for deterministic Saga smoke tests;
- two Timeout Worker replicas;
- two Reconciliation Worker replicas.

MySQL, RabbitMQ and business Services remain cluster-internal.

### Deploy locally

Requirements:

```text
Docker
kind
kubectl
curl
```

```bash
sh scripts/k8s/deploy-local.sh
```

The script:

1. creates the kind cluster when absent;
2. builds seven application images;
3. loads them into kind;
4. applies the local overlay;
5. waits for MySQL and RabbitMQ;
6. waits for four Migration Jobs;
7. waits for seven application Deployments;
8. applies a bounded Gateway readiness retry window.

Run the Kubernetes Saga against the deployed cluster:

```bash
sh scripts/smoke/microservices-saga-kubernetes.sh
```

Delete the cluster:

```bash
kind delete cluster --name go-order-local
```

## Test overlay

The test overlay is a non-production delivery contract. It adds:

- two replicas for the Gateway, four business services and both Worker types;
- one PDB per multi-replica Deployment with `minAvailable: 1`;
- one Gateway Ingress;
- `ingressClassName: nginx`;
- host `go-order.test.local`;
- test-only placeholder Secret values.

It intentionally leaves MySQL and RabbitMQ as single instances and does not apply PDBs to them.

The test overlay keeps all Services as ClusterIP. Only the Gateway is reachable through the Ingress contract.

Render:

```bash
kustomize build deploy/kubernetes/overlays/test >/tmp/go-order-test.yaml
```

The target cluster must already provide an ingress controller for the `nginx` class. The overlay does not install the controller, issue TLS certificates or configure DNS. Map `go-order.test.local` to the ingress address before sending traffic.

## Probes and rollout behavior

HTTP workloads define:

- `startupProbe`;
- `readinessProbe`;
- `livenessProbe`;
- RollingUpdate with `maxUnavailable: 0` and `maxSurge: 1`.

Workers use process probes because they do not expose HTTP endpoints. Their operational correctness still depends on leases, structured logs and reliability indicators.

The test overlay PDBs protect voluntary disruptions for multi-replica application workloads. They do not provide node redundancy by themselves and do not protect against involuntary failures.

## Automated verification

### Main CI

The existing CI validates:

- Go lint, test, race, vet and build;
- legacy and service-owned migrations;
- all Compose images;
- four databases, RabbitMQ and both Worker types;
- complete Compose Order Saga;
- live disposable kind deployment;
- StatefulSets, Migration Jobs and application rollouts;
- NodePort/internal Service exposure boundary;
- dual Worker replicas;
- unavailable Gateway revision detection;
- `kubectl rollout undo` recovery;
- complete Kubernetes Order Saga after recovery.

### Kubernetes Contracts workflow

The independent contract workflow validates:

- local overlay rendering;
- test overlay rendering;
- exactly one Gateway Ingress;
- exactly seven PDB resources;
- seven application Deployments rendered with two replicas;
- no NodePort in the test overlay;
- no PDB applied to MySQL or RabbitMQ;
- rendered manifests uploaded as CI artifacts.

## Verified boundaries

Completed:

- Deployment;
- Service;
- ConfigMap and Secret contract;
- service-owned Migration Jobs;
- startup/liveness/readiness probes;
- requests and limits;
- RollingUpdate;
- live kind deployment;
- failed rollout detection and `rollout undo`;
- Kubernetes Saga;
- Gateway Ingress contract;
- test overlay;
- PDB contracts for multi-replica application workloads.

Still deferred:

- ingress-controller installation and real Ingress traffic in CI;
- TLS and certificate management;
- HPA;
- NetworkPolicy;
- multi-node disruption tests;
- immutable registry image tags;
- managed-cloud storage, load balancer and workload identity integration.
