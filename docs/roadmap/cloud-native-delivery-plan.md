# Cloud-Native Delivery Roadmap

## 1. Purpose

This roadmap defines the next four delivery stages for `go-order-management-system-cloudnative-lab` after the completion of:

- process-level microservice separation;
- service-owned databases;
- inventory reservation and Order Saga;
- Transactional Outbox;
- lease-based multi-worker polling;
- service-owned Goose migrations;
- end-to-end Docker Compose CI verification.

The goal is not to add cloud-native terminology for display. Every item must produce executable configuration, automated verification, failure evidence and a rollback path.

## 2. Delivery principles

1. One concern per PR whenever practical.
2. Every PR must preserve the existing end-to-end Order Saga smoke test.
3. Infrastructure changes must include a local reproducible verification path.
4. Reliability features must define idempotency and retry boundaries before implementation.
5. Kubernetes is introduced only after application-level timeout, retry and observability contracts are stable.
6. Production claims are prohibited until backup recovery, rollback and fault drills have evidence.
7. Each stage ends with synchronized documentation and an explicit completion matrix.

## 3. Stage 1 — Reliability closure

### 3.1 Scope

- RabbitMQ Publisher Confirms
- unified HTTP timeout budgets
- bounded retries with exponential backoff
- basic circuit breaking
- request rate limiting
- Outbox failure and backlog metrics
- automatic reconciliation job

### 3.2 Implementation order

#### PR 1.1 — RabbitMQ Publisher Confirms

Deliverables:

- enable confirm mode on the publisher channel;
- correlate broker acknowledgements with Outbox records;
- mark an Outbox event as `published` only after positive confirmation;
- classify nack, timeout and connection-loss outcomes;
- preserve at-least-once semantics;
- add integration tests for ack, nack/timeout and reconnect cases.

Acceptance criteria:

- a broker acknowledgement is required before database status changes to `published`;
- a missing acknowledgement leaves the event retryable;
- duplicate publication remains safe through idempotent consumers;
- two Worker replicas still pass the complete Saga smoke test.

Rollback boundary:

- retain the current lease-based publisher implementation behind a small interface so the confirm publisher can be reverted without changing Order Service APIs or the Outbox schema.

#### PR 1.2 — HTTP timeout budget and retry policy

Deliverables:

- define one request budget propagated across Gateway and services;
- configure connect, response-header and total request timeouts;
- add bounded exponential backoff with jitter;
- retry only explicitly safe operations;
- preserve idempotency keys and reservation identifiers across retries;
- propagate request ID and trace context headers.

Initial retry classification:

| Operation | Retry policy |
| --- | --- |
| Catalog product snapshot query | bounded retry allowed |
| Identity role query | bounded retry allowed |
| Inventory reservation create | retry only with stable reservation/idempotency identity |
| Inventory confirm/release | bounded retry because endpoint is idempotent |
| Order create | no blind client retry without idempotency key |
| Payment state transition | no blind retry unless operation identity is stable |

Acceptance criteria:

- retries stop before the caller budget expires;
- permanent 4xx errors are not retried;
- timeout and retry attempts are observable in logs and metrics;
- end-to-end Saga remains deterministic.

#### PR 1.3 — Circuit breaker and request limiting

Deliverables:

- per-upstream circuit breakers for synchronous internal clients;
- closed/open/half-open states;
- local token-bucket or leaky-bucket request limits at Gateway;
- separate external API limits from internal service traffic;
- standard overload response and `Retry-After` where applicable.

Acceptance criteria:

- repeated upstream failures open the breaker;
- half-open probes restore the circuit only after successful calls;
- one failing upstream does not exhaust all client goroutines/connections;
- rate limits are covered by deterministic tests.

#### PR 1.4 — Reliability metrics and reconciliation

Deliverables:

- metrics for Outbox pending, failed, leased and oldest-event age;
- metrics for Saga compensation and `reconciliation_required` orders;
- a standalone reconciliation command/job;
- bounded batch scanning and idempotent repair actions;
- dry-run mode and structured repair audit logs.

Acceptance criteria:

- the reconciliation job can identify uncertain orders without changing healthy orders;
- dry-run output is testable;
- repair operations are idempotent;
- CI includes a reconciliation smoke case.

### 3.3 Stage completion gate

Stage 1 is complete only when:

- Publisher Confirms are verified against RabbitMQ;
- all internal HTTP clients use explicit budgets;
- retry and breaker behavior is covered by tests;
- Gateway rate limiting is active;
- Outbox/Saga reliability metrics are exposed;
- reconciliation can run safely in dry-run and repair modes;
- current four-database, two-worker Saga CI remains green.

## 4. Stage 2 — Kubernetes foundation

### 4.1 Scope

- Deployment
- Service
- ConfigMap
- Secret
- Migration Job
- Ingress
- startupProbe
- livenessProbe
- readinessProbe
- resource requests and limits
- RollingUpdate
- rollout undo

### 4.2 Target structure

```text
deploy/kubernetes/
├── base/
│   ├── namespace.yaml
│   ├── configmap.yaml
│   ├── secret.example.yaml
│   ├── api-gateway/
│   ├── identity-service/
│   ├── catalog-service/
│   ├── inventory-service/
│   ├── order-service/
│   ├── order-timeout-worker/
│   ├── migrations/
│   └── ingress.yaml
└── overlays/
    ├── local/
    └── test/
```

Kustomize is the initial packaging mechanism. Helm is deferred until configuration duplication or release packaging justifies it.

### 4.3 Delivery batches

#### PR 2.1 — Base workloads and networking

- Namespace
- Deployments
- ClusterIP Services
- ConfigMap
- Secret example
- resource requests/limits
- service accounts

#### PR 2.2 — Migration Jobs and probes

- one Job per service-owned Goose directory;
- startup, liveness and readiness probes;
- application startup dependency rules that do not rely on Compose `depends_on`;
- migration failure blocks release completion.

#### PR 2.3 — Ingress and rollout operations

- Ingress for API Gateway only;
- RollingUpdate strategy with `maxUnavailable` and `maxSurge`;
- rollout status verification;
- documented `kubectl rollout undo` procedure;
- PodDisruptionBudget where multiple replicas are meaningful.

### 4.4 Stage completion gate

- a local `kind` or `k3d` cluster can deploy the complete application topology;
- only API Gateway is externally reachable;
- probes reflect process, dependency and readiness semantics correctly;
- migration Jobs complete before workloads are accepted;
- an intentionally bad image can be rolled back using documented commands;
- resource requests/limits are present for every workload.

## 5. Stage 3 — Observability

### 5.1 Scope

- Prometheus
- Grafana
- OpenTelemetry
- cross-service `trace_id` propagation
- Saga metrics
- Outbox metrics
- RabbitMQ metrics
- basic alerts

### 5.2 Delivery batches

#### PR 3.1 — Application metrics

Expose per-service `/metrics` with:

- HTTP request count, status and duration;
- internal client request count, retries, timeout and breaker state;
- database pool metrics;
- Saga started/completed/failed/compensated/reconciliation-required;
- Outbox pending/failed/published/oldest-age/lease-recovery;
- Worker publish and consume outcomes.

#### PR 3.2 — Distributed tracing

- OpenTelemetry SDK initialization;
- W3C Trace Context propagation through Gateway and internal HTTP clients;
- spans for Catalog lookup, Inventory reservation, Order local transactions and Outbox processing;
- trace/span IDs included in structured logs.

#### PR 3.3 — Prometheus, Grafana and alerts

- Prometheus scrape configuration;
- Grafana dashboards for service health, Saga and Outbox;
- RabbitMQ exporter/plugin metrics;
- initial alerts:
  - high 5xx rate;
  - high P95 latency;
  - Outbox oldest age above threshold;
  - reconciliation-required orders above zero;
  - RabbitMQ unavailable;
  - repeated migration Job failure.

### 5.3 Stage completion gate

- one Order Saga can be followed across all participating services by trace ID;
- dashboards show HTTP, Saga, Outbox and RabbitMQ health;
- alerts can be triggered in a controlled local/test scenario;
- metric label cardinality is reviewed and bounded.

## 6. Stage 4 — Runtime assurance

### 6.1 Scope

- image publishing to GHCR
- automatic test-environment deployment
- Smoke Test
- rollback of bad releases
- MySQL backup and restore
- failure drills
- Runbook
- load-test report

### 6.2 Delivery batches

#### PR 4.1 — Image release and test deployment

- build immutable service images;
- publish to GHCR using commit SHA tags;
- optionally add semantic release tags later;
- deploy the exact SHA set to the test environment;
- record image digests in deployment evidence.

#### PR 4.2 — Deployment verification and rollback

- post-deployment readiness verification;
- complete Saga Smoke Test;
- fail deployment when Smoke Test fails;
- automatic or operator-approved rollback to the last known good release;
- retain failure diagnostics.

#### PR 4.3 — Backup, restore and failure drills

- MySQL logical backup procedure for four databases;
- restore into an isolated environment;
- verify row counts and a representative order/inventory relationship;
- RabbitMQ outage drill;
- service timeout drill;
- Worker crash and lease-recovery drill;
- migration failure drill.

#### PR 4.4 — Runbook and load test

- operational Runbook with detection, diagnosis, mitigation and recovery steps;
- load-test scenarios for read traffic and order creation;
- P50/P95/P99 latency, throughput, error rate and resource utilization;
- identify the first bottleneck and document the evidence;
- define a safe operating envelope rather than claiming arbitrary QPS.

### 6.3 Stage completion gate

- GHCR contains immutable images for every deployable service;
- the test environment deploys automatically from an approved branch/tag;
- a bad release is demonstrably rolled back;
- backup restoration is performed and evidenced;
- at least three failure drills are documented with results;
- the repository contains an actionable Runbook and reproducible load-test report.

## 7. Cross-stage PR and branch strategy

Planning PR:

```text
roadmap/cloud-native-delivery
```

Implementation branches:

```text
phase/05-publisher-confirms
phase/05-http-resilience
phase/05-circuit-rate-limit
phase/05-reconciliation-metrics
phase/06-kubernetes-base
phase/06-kubernetes-operations
phase/07-observability
phase/08-delivery-runtime-assurance
```

Rules:

- implementation PRs target `main`;
- each PR references its tracking Issue;
- no stage is marked complete based only on configuration files;
- CI evidence and a reproducible command are required;
- documentation is synchronized in the same PR when behavior changes.

## 8. Immediate next action

The first implementation PR will be:

```text
phase/05-publisher-confirms
```

Before coding, it must inspect the current RabbitMQ publisher lifecycle, channel reconnect behavior, Outbox state transitions and existing integration tests. The implementation must not combine Publisher Confirms with HTTP retry or Kubernetes changes.
