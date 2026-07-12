# Grafana dashboards and Prometheus alert rules

## Scope

Phase 7.2 turns the bounded Phase 7.1 metrics into an operator-facing overview and a small, deterministic alert set. It does not add OpenTelemetry, Alertmanager delivery integrations, infrastructure exporters or production SLO commitments.

The observability stack remains optional. Grafana and Prometheus are added only through `compose.observability.yml`; application readiness never depends on either component.

## Local stack

```bash
docker compose -f compose.yml -f compose.observability.yml up -d --build --wait \
  --scale order-timeout-worker=2 \
  --scale order-reconciliation-worker=2
```

Default endpoints:

| Component | URL |
| --- | --- |
| Prometheus | `http://127.0.0.1:9090` |
| Grafana | `http://127.0.0.1:3000` |

Optional environment variables:

```text
PROMETHEUS_HOST_PORT
GRAFANA_HOST_PORT
GRAFANA_ADMIN_USER
GRAFANA_ADMIN_PASSWORD
```

The Compose defaults are local-development values only. Non-local deployments must inject credentials through their deployment secret mechanism.

## Grafana provisioning

The repository provisions:

```text
deploy/grafana/provisioning/datasources/prometheus.yml
deploy/grafana/provisioning/dashboards/dashboards.yml
deploy/grafana/dashboards/go-order-overview.json
```

The Prometheus data source has stable UID `prometheus` and points to `http://prometheus:9090` on the Compose network. The dashboard has stable UID `go-order-overview` and is file managed; UI edits are not treated as source of truth.

## Dashboard coverage

The overview includes:

- application scrape targets currently down;
- aggregate and per-service request rate;
- per-service HTTP 5xx ratio;
- per-service p95 request latency;
- internal HTTP attempt rate by upstream and outcome;
- Orders grouped by status;
- timeout Outbox events grouped by status;
- reconciliation-required count;
- stuck transient Saga count;
- overdue and oldest actionable Outbox indicators;
- RabbitMQ Publisher Confirm outcomes;
- Worker availability by bounded Worker type.

The service selector uses the finite `service` label. Dashboard queries do not use request IDs, user IDs, order IDs, reservation IDs, event IDs, Worker instance IDs or raw URLs.

## Recording rules

`deploy/prometheus/rules/recording-rules.yml` defines:

```text
service:http_requests:rate5m
service:http_server_errors:rate5m
service:http_server_error_ratio:rate5m
service:http_server_request_duration_seconds:p95
service:http_client_attempts:rate5m
worker:up:max
```

These rules provide stable query contracts for dashboards and alerts while keeping aggregation dimensions bounded.

## Alert rules

`deploy/prometheus/rules/alert-rules.yml` defines:

| Alert | Default threshold | `for` window | Intent |
| --- | --- | --- | --- |
| `GoOrderTargetDown` | application target `up == 0` | 2m | scrape/runtime availability |
| `GoOrderWorkerDown` | all scraped instances of a Worker type report `0` | 2m | background process availability |
| `GoOrderElevatedHTTP5xxRatio` | ratio > 5% and request rate > 0.1/s | 5m | sustained server errors |
| `GoOrderHighP95Latency` | p95 > 1s | 10m | sustained latency degradation |
| `GoOrderOutboxOverdue` | overdue events > 0 | 3m | timeout delivery backlog |
| `GoOrderOutboxActionableAgeHigh` | oldest actionable age > 300s | 5m | aging Outbox work |
| `GoOrderReconciliationBacklog` | reconciliation-required > 0 | 5m | unresolved Saga repair work |
| `GoOrderSagaStuck` | stuck transient Orders > 0 | 5m | Saga state exceeded threshold |
| `GoOrderMetricsCollectionFailing` | collection errors increased in 10m | 2m | stale reliability snapshots |

Thresholds are conservative lab defaults, not production SLOs. Alertmanager routing, paging policies and notification integrations remain deferred.

## Validation

The Observability Stack workflow performs four levels of validation:

1. `docker compose ... config --quiet` validates the merged topology.
2. `scripts/verify/observability-contracts.py` validates dashboard JSON, stable UIDs, panel coverage, provisioning paths, exact rule names, explicit `for` windows and forbidden labels.
3. `promtool check config` and `promtool test rules` validate Prometheus syntax plus firing and non-firing fixtures.
4. Runtime smoke starts the full application, Prometheus and Grafana, runs the complete Order Saga, verifies all seven scrape targets, checks rule health and recording series, and reads the provisioned data source and dashboard through the Grafana API.

## Known limitations

- no Alertmanager or delivery receiver;
- no production SLO/error-budget policy;
- no infrastructure exporters, kube-state-metrics or node-level dashboards;
- no Kubernetes Grafana/Prometheus deployment in the repository;
- no distributed tracing or trace-log correlation;
- file-provisioned dashboards intentionally reject persistent UI edits.

## Rollback boundary

Grafana files, recording rules, alert rules and Compose services are additive. Reverting Phase 7.2 leaves application metrics, business APIs, persistence, Saga behavior and Kubernetes delivery contracts unchanged.
