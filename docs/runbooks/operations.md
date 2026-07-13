# Go Order Management System Operations Runbook

## 1. Purpose and operating boundary

This Runbook is the operational entry point for the repository's verified Compose, Kubernetes, observability, release, backup, rollback and fault-drill paths.

The project is a deeply verified cloud-native engineering laboratory, not a production service. The procedures below operate on local Docker Compose, disposable `kind` clusters and GitHub Actions. They do not authorize handling real customer data, production credentials, public traffic, managed cloud infrastructure or destructive testing outside disposable environments.

## 2. Architecture quick reference

| Layer | Components | Primary dependency | Primary evidence |
| --- | --- | --- | --- |
| Edge | API Gateway | Identity, Catalog, Inventory, Order | Gateway readiness, HTTP metrics, access logs |
| Identity | Identity Service | `go_order_identity` | auth/role responses, DB migration state |
| Catalog | Catalog Service | `go_order_catalog`, Identity | product API, client metrics, traces |
| Inventory | Inventory Service | `go_order_inventory` | available/reserved stock, stock logs |
| Ordering | Order Service | `go_order_ordering`, Catalog, Inventory | order state, Outbox, Saga logs |
| Async | Timeout Worker, Reconciliation Worker | RabbitMQ, Ordering DB, internal HTTP | session metrics, queue state, leases |
| Data | MySQL 8.4 | four service-owned databases | migrations, logical fingerprints, counts |
| Messaging | RabbitMQ 4.1 | Workers | queue state, publish/consume metrics |
| Observability | Prometheus, Grafana, Tempo, OTel Collector | application `/metrics`, OTLP | targets, rules, dashboards, traces |
| Release | GHCR + release manifest | GitHub Actions | OCI digest, release artifact, Issue evidence |
| Test delivery | disposable `kind` | verified release manifest | image inventory, two Sagas, rollback evidence |

### Service-owned databases

```text
go_order_identity
go_order_catalog
go_order_inventory
go_order_ordering
```

Do not introduce cross-service table writes during incident mitigation. A service owns its database and migration directory.

## 3. Severity and response posture

| Severity | Definition in this project | Required action |
| --- | --- | --- |
| SEV-1 | Data integrity loss, unrecoverable security exposure or complete service outage with no safe rollback | Stop changes, preserve evidence, isolate environment, use Last-Known-Good digest or restore procedure, escalate immediately |
| SEV-2 | Major business flow unavailable, repeated Saga failure, RabbitMQ unavailable without recovery, migration blocked | Halt promotion, collect metrics/logs/traces, execute bounded recovery, open incident record |
| SEV-3 | Elevated errors/latency, single Worker degraded, backlog growth with service still usable | Diagnose component, reduce load, restart only the affected disposable component if justified |
| SEV-4 | Warning, test flake or non-user-facing evidence gap | Record, reproduce and fix through a separate PR |

The repository has no production on-call organization. “Escalation” means notifying the repository owner, stopping automated promotion and preserving GitHub-native evidence before further changes.

## 4. Evidence-first rule

Before restart, rollback, deletion or database action, record:

- Git commit and release manifest commit;
- GitHub Actions run and artifact names;
- UTC timestamp;
- `docker compose ps -a` or `kubectl get all,jobs,pvc`;
- recent events;
- affected service logs with Request ID, Order ID or Event ID;
- Prometheus target/rule state and relevant metric query;
- Tempo Trace ID when a cross-service request exists;
- MySQL/RabbitMQ state when relevant.

Never place passwords, JWTs, GHCR tokens or real user data in artifacts or issue comments.

## 5. Startup, readiness and smoke verification

### Docker Compose

Required environment values:

```text
MYSQL_PASSWORD
JWT_SECRET
INTERNAL_SERVICE_TOKEN
```

Start the data-isolated topology:

```bash
docker compose up -d --build --wait \
  --scale order-timeout-worker=2 \
  --scale order-reconciliation-worker=2
```

Verify:

```bash
curl --fail http://127.0.0.1:8082/readyz
docker compose ps
SMOKE_RUN_SUFFIX="manual-$(date +%s)" sh scripts/smoke/microservices-saga.sh
```

Stop and delete disposable data:

```bash
docker compose down -v --remove-orphans
```

### Local Kubernetes

```bash
bash scripts/k8s/deploy-local.sh
curl --fail http://127.0.0.1:8082/readyz
SMOKE_RUNTIME=kubernetes \
KUBERNETES_NAMESPACE=go-order-system \
SMOKE_RUN_SUFFIX="manual-k8s-$(date +%s)" \
sh scripts/smoke/microservices-saga-kubernetes.sh
```

Collect state before cleanup:

```bash
kubectl -n go-order-system get pods,services,deployments,statefulsets,jobs -o wide
kubectl -n go-order-system get events --sort-by=.lastTimestamp
```

Delete the disposable cluster with the same `kind` cluster name used by the deployment script.

## 6. Immutable release and automatic test delivery

### Release source of truth

`.github/workflows/publish-images.yml` publishes exactly seven GHCR images tagged by the full source commit and records digest-qualified references in `release-manifest.json`.

A valid application reference has this shape:

```text
ghcr.io/<owner>/go-order-management-system-cloudnative-lab-<service>@sha256:<digest>
```

Do not deploy `latest`, branch tags or locally rebuilt application images as a verified release.

### Automatic CD acceptance

`.github/workflows/deploy-test-release.yml` consumes the exact release manifest into a disposable `kind` cluster. Acceptance requires:

1. seven Deployments and four migration Jobs use accepted digest references;
2. Gateway readiness passes;
3. complete Order Saga passes;
4. a deliberately nonexistent Gateway digest fails rollout;
5. the complete Last-Known-Good seven-image set is restored;
6. image inventory and a second complete Saga pass;
7. the cluster is deleted.

### Rollback procedure

1. Identify the accepted `release-manifest.json` and source run.
2. Verify repository and full commit binding.
3. Extract the seven digest-qualified references.
4. Apply all seven references, not only the visibly failed service.
5. Wait for each Deployment rollout.
6. Verify the deployed image inventory against the manifest.
7. Run readiness and a complete Saga.
8. Record the run, artifact and rollback result.

Application rollback does not roll back database Schema. A release requiring incompatible down migration is outside this project's automated rollback boundary.

## 7. MySQL backup and isolated restore

Use `.github/workflows/mysql-backup-restore.yml` or the underlying script only with synthetic data.

Procedure:

1. Create representative business data.
2. Stop application writers and RabbitMQ; keep source MySQL running.
3. Create four independent restorable dumps with transactional snapshot and foreign-key restore guards.
4. Create the exact four-database manifest with sizes and SHA-256.
5. Restore into a separate MySQL 8.4 container.
6. Verify migration versions and representative records.
7. Compare Source-Before and Restored logical Schema plus ordered-data fingerprints.
8. Compare Source-Before and Source-After fingerprints.
9. Prove a corrupted dump is rejected.
10. Delete the restore container and anonymous volume.

Do not restore into the source database. Production physical backup, Binlog/PITR, encryption, external retention and formal RPO/RTO remain unimplemented.

## 8. Incident: RabbitMQ outage

### Detection

- `go_order_rabbitmq_session_up` falls from 1 to 0;
- RabbitMQ publish/consume errors increase;
- queue API or `rabbitmq-diagnostics` becomes unavailable;
- Worker logs show session loss/reconnect attempts.

### Diagnosis

```bash
docker compose ps rabbitmq order-timeout-worker order-reconciliation-worker
docker compose logs --since=10m rabbitmq order-timeout-worker order-reconciliation-worker
curl --fail http://127.0.0.1:19090/api/v1/query?query=go_order_rabbitmq_session_up
```

### Mitigation

- stop promotion and additional fault injection;
- keep MySQL and application state intact;
- restart only RabbitMQ in the disposable environment;
- do not restart Workers initially, because automatic reconnect is part of the verified behavior.

### Recovery verification

- session metric returns to 1;
- Workers remain running;
- publish/consume metrics resume;
- timeout-driven complete Saga passes.

If session does not recover within the bounded drill interval, collect logs and restart the affected Worker only after preserving evidence.

## 9. Incident: HTTP timeout or circuit open

### Detection

- HTTP client attempt outcomes show timeout/transport/502/503/504;
- structured logs contain `outcome=circuit_open`;
- requests fail fast with `ErrCircuitOpen` and a retry-after duration;
- P95/P99 latency or server errors rise.

### Diagnosis

1. Find the circuit key `<upstream>/<operation>`.
2. Inspect upstream readiness and server metrics.
3. Correlate Request ID and Trace ID across Gateway, caller and upstream.
4. Distinguish caller deadline exhaustion from breaker-counted infrastructure failure.

### Mitigation

- reduce or stop synthetic load;
- repair the upstream dependency;
- do not disable timeouts or increase retry count blindly;
- preserve the request budget and avoid retry storms.

### Recovery verification

- after Open Interval, one bounded Half-Open probe succeeds;
- breaker returns to Closed;
- normal request succeeds;
- upstream call count proves Open requests did not perform network I/O.

## 10. Incident: Worker crash while holding a lease

### Detection

- Outbox/Reconciliation row has `lease_owner` and future `lease_until`;
- owning process or container is absent;
- pending/in-progress backlog stops decreasing.

### Diagnosis

```sql
SELECT id, status, attempts, lease_owner, lease_until, next_attempt_at
FROM order_timeout_outbox_v2
WHERE lease_owner <> '' OR status <> 'published'
ORDER BY id;
```

### Mitigation

- do not manually clear a live lease before expiry;
- preserve Event ID, Order ID, Owner and Lease Until;
- allow another Worker replica to reclaim after expiry;
- manually modify a lease only when the owner is proven dead and the normal recovery path is unavailable.

### Recovery verification

- the same Event ID is reclaimed once;
- terminal status is correct;
- attempts increment once for the recovery operation;
- lease fields are cleared;
- no duplicate row or duplicate business transition exists.

## 11. Incident: migration failure

### Detection

- migration Job or Goose exits nonzero;
- application promotion does not continue;
- migration logs identify the exact file and SQL failure.

### Diagnosis

- verify the migration directory and source commit;
- confirm the target database is isolated before reproducing;
- inspect `goose_db_version` and whether a partial DDL effect occurred;
- never test deliberate invalid SQL against a shared or production database.

### Mitigation

1. Stop promotion.
2. Preserve migration logs and database state.
3. Correct the forward migration in a new PR.
4. Validate all normal migration directories.
5. Re-run against a fresh disposable database.

### Recovery verification

- invalid migration remains rejected;
- no prohibited table/object was created;
- normal migration directories validate;
- a fresh migration run completes before application rollout.

## 12. Incident: high latency, errors or saturation

### Detection

- increasing HTTP error rate;
- throughput stops increasing while P95/P99 grows;
- container CPU/memory peaks;
- MySQL threads/row-lock waits increase;
- RabbitMQ or Outbox backlog grows;
- Gateway returns HTTP 429.

### Diagnosis order

1. Confirm the workload profile and concurrency stage.
2. Separate measured facts from inference.
3. Inspect Gateway status counts and rate-limit response.
4. Inspect service HTTP histograms and client attempts.
5. Inspect MySQL global status and service-owned row/backlog counts.
6. Inspect RabbitMQ queue state.
7. Compare container resource samples.
8. Use traces to find the longest synchronous span.

### Mitigation

- stop or reduce load first;
- retain artifacts;
- avoid changing multiple timeouts, pools or retries simultaneously;
- change one bounded configuration through a reviewed PR and repeat the same profile.

### Recovery verification

- readiness remains healthy;
- error rate returns to baseline;
- backlog drains or stabilizes;
- P95/P99 recover;
- a complete Saga still passes.

## 13. Observability investigation paths

### Prometheus

Default local address: `http://127.0.0.1:9090` or the workflow-specific mapped port.

Investigate:

- target health;
- recording and alert rules;
- `go_order_http_server_requests_total`;
- `go_order_http_server_request_duration_seconds_bucket`;
- `go_order_http_client_attempts_total`;
- RabbitMQ session/publish/consume metrics;
- Outbox, reconciliation and migration metrics.

### Grafana

Default local address: `http://127.0.0.1:3000` or the mapped workflow port. Confirm provisioning before interpreting an empty dashboard.

### Tempo

Search by Trace ID captured from response/log evidence. Follow Gateway → service → internal client spans. RabbitMQ message tracing is not claimed as complete end-to-end trace propagation.

### Logs

Prefer structured fields:

```text
request_id
trace_id
service
method
route
status
upstream
operation
order_id
event_id
lease_owner
outcome
```

## 14. Evidence collection commands

```bash
mkdir -p incident-evidence
docker compose ps -a > incident-evidence/compose-ps.txt
docker compose logs --no-color --timestamps > incident-evidence/compose.log
docker stats --no-stream > incident-evidence/docker-stats.txt
```

For Kubernetes:

```bash
kubectl -n go-order-system get all,jobs,pvc -o wide > incident-evidence/k8s-resources.txt
kubectl -n go-order-system get events --sort-by=.lastTimestamp > incident-evidence/k8s-events.txt
kubectl -n go-order-system describe pods > incident-evidence/k8s-pods.txt
```

Store evidence in GitHub Actions artifacts or the relevant GitHub Issue. Do not use an external artifact host for project evidence.

## 15. Post-incident review template

```markdown
# Incident title

- Severity:
- Start/end UTC:
- Detection source:
- Affected commit/release digest:
- User-visible or test-visible impact:

## Confirmed timeline

## Measured evidence

## Root cause

## Contributing conditions

## Mitigation and recovery

## Verification after recovery

## What worked

## What failed or was missing

## Corrective actions

| Action | Owner | Priority | Evidence/PR |
| --- | --- | --- | --- |

## Explicit non-actions

## Remaining production boundary
```

## 16. Production enhancements intentionally left optional

- managed Kubernetes, database and RabbitMQ;
- TLS, domain, ingress and external load balancer;
- production Secret Manager and credential rotation;
- remote object storage, encrypted backup and PITR;
- multi-zone or multi-region recovery;
- externally routed alert notifications and formal on-call escalation;
- distributed rate limiting and circuit state;
- message-level trace propagation;
- production SLO, capacity plan and real traffic validation;
- security review, threat model, compliance and data governance.

These are not hidden completion requirements for the current project. They are optional next-stage production work and must not be claimed as already implemented.
