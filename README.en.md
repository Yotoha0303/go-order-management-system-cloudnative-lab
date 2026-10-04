# Go Order Management Cloud-Native Lab

> A cloud-native engineering laboratory evolved from a Go layered monolith, focusing on service boundaries, order/inventory consistency, reliable messaging, application resilience, Kubernetes delivery, observability, backup/recovery, fault drills, and repeatable verification.

[中文 README](README.md)

## Project Positioning

The system currently contains seven runtime units: API Gateway, Identity Service, Catalog Service, Inventory Service, Order Service, Order Timeout Worker, and Order Reconciliation Worker.

Four services own separate databases: `go_order_identity`, `go_order_catalog`, `go_order_inventory`, and `go_order_ordering`.

The project has completed the defined Phase 5–8 engineering route, but remains an executable cloud-native verification laboratory rather than a production platform claim.

## Completed Capabilities

| Domain | Current Implementation |
| --- | --- |
| Business consistency | Inventory Reservation, Order Saga, compensation, reconciliation |
| Messaging | Transactional Outbox, RabbitMQ TTL/DLX, Publisher Confirms, manual ACK, at-least-once |
| Worker concurrency | Multiple replicas, leases, `FOR UPDATE SKIP LOCKED`, crash recovery |
| HTTP resilience | Deadlines, bounded retries, exponential backoff, circuit breaker |
| Gateway protection | Token Bucket, HTTP 429, `Retry-After` |
| Kubernetes | Kustomize, StatefulSet, Deployment, Service, probes, resources, Ingress, PDB |
| Observability | Prometheus, Grafana, OpenTelemetry, Collector, Tempo |
| Image release | Immutable GHCR images, Commit SHA tags, OCI digests |
| Automated CD | Digest deployment, smoke tests, failed-release detection, rollback |
| Backup & recovery | Four DB backups, SHA-256 manifest, isolated MySQL restore |
| Fault drills | RabbitMQ, circuit breaker, worker lease, migration failure drills |
| Operations | Runbook, diagnosis/mitigation/recovery procedures, postmortem templates |
| Load testing | Concurrency 1/4/8/16/32, P50/P95/P99, resource evidence |

## Verification Evidence

Accepted bounded load-test observations:

```text
Best sustained healthy throughput: 177.989 requests/second
Highest healthy P95: 31.812 ms
Healthy-stage errors: 0
First observed boundary: throughput plateau and tail-latency growth at concurrency 8
```

These are synthetic tests on a GitHub-hosted runner, not production capacity or SLO commitments.

## Local Compose Verification

```bash
cp .env.example .env
docker compose config --quiet
docker compose up -d --build --wait \
  --scale order-timeout-worker=2 \
  --scale order-reconciliation-worker=2
curl --fail http://127.0.0.1:8082/readyz
sh scripts/smoke/microservices-saga.sh
```

## Observability

```bash
docker compose -f compose.yml -f compose.observability.yml up -d --build --wait \
  --scale order-timeout-worker=2 \
  --scale order-reconciliation-worker=2
```

Default endpoints: Prometheus `:9090`, Grafana `:3000`, Tempo `:3200`, OTLP/HTTP `:14318`.

## CI/CD

The repository uses GitHub Actions for CI, Kubernetes contracts, observability, immutable image publishing, verified test deployment, backup/restore verification, fault drills, operations contracts, and bounded load tests.

## Production Boundary

The project does not currently claim production multi-node or multi-AZ infrastructure, managed databases, long-term trace/backup storage, mTLS, workload identity, runtime least-privilege database accounts, TLS/HPA/NetworkPolicy production hardening, formal RPO/RTO, or formal SLO/error-budget governance.

## Documentation

See the Chinese README and `docs/README.md` for the complete architecture, verification, runbook, evolution, and production-boundary documentation.
