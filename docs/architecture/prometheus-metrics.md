# Prometheus metrics foundation

## Scope

Phase 7.1 adds bounded application metrics without changing business APIs, database schemas, Saga states or message contracts.

The implementation uses a small standard-library registry that emits Prometheus text format `0.0.4`. It supports counters, gauges, fixed-bucket histograms and named scrape-time collectors. Grafana, alert rules and OpenTelemetry remain separate phases.

## Scrape endpoints

| Runtime | Endpoint |
| --- | --- |
| API Gateway | `:8082/metrics` |
| Identity Service | `:8083/metrics` |
| Catalog Service | `:8084/metrics` |
| Inventory Service | `:8085/metrics` |
| Order Service | `:8086/metrics` |
| Timeout Worker | `:9091/metrics` |
| Reconciliation Worker | `:9092/metrics` |

`/metrics` bypasses business rate limiting and request-budget middleware. Scrapes are not recursively included in HTTP request metrics.

## HTTP server metrics

```text
go_order_http_server_requests_total
go_order_http_server_response_bytes_total
go_order_http_server_request_duration_seconds
```

Labels:

```text
service
method
route_group
status_class
```

`route_group` is selected from a fixed list such as `api_orders`, `internal_inventory`, `readyz` and `unmatched`. Raw URLs, IDs and query strings are never labels.

## HTTP client metrics

```text
go_order_http_client_attempts_total
go_order_http_client_attempt_duration_seconds
```

Labels:

```text
upstream
operation
outcome
status_class
retryable
```

The transport records each real network attempt. `operation` uses the same bounded route grouping. Retry attempts are visible independently, while the existing Executor logs retain the logical operation and attempt number.

## Order reliability gauges

The Order Service and both Order Workers reuse the existing `ReliabilityReporter`. A scrape performs the existing Outbox aggregate query and Order aggregate query, then updates:

```text
go_order_outbox_events{status}
go_order_outbox_leased
go_order_outbox_retry_ready
go_order_outbox_overdue
go_order_outbox_oldest_actionable_age_seconds
go_order_outbox_maximum_attempts
go_order_outbox_failed_attempts_total_snapshot

go_order_orders{status}
go_order_reconciliation_required
go_order_reconciliation_oldest_age_seconds
go_order_saga_stuck_transient
go_order_saga_stuck_threshold_seconds
go_order_reliability_snapshot_query_duration_seconds
go_order_reliability_snapshot_collected_timestamp_seconds
```

A failed scrape-time database collection increments:

```text
go_order_metrics_collection_errors_total{collector="order_reliability"}
```

The metrics endpoint still returns the last successfully collected values. A metrics collection failure does not make the business readiness endpoint fail.

## Worker and RabbitMQ metrics

Workers expose:

```text
go_order_worker_up{worker}
go_order_worker_metrics_listener_up{worker}
```

The confirmed RabbitMQ publisher exposes:

```text
go_order_rabbitmq_publish_total{outcome}
go_order_rabbitmq_publish_duration_seconds{outcome}
```

Bounded outcomes:

```text
ack
nack
timeout
channel_closed
publish_error
other_error
```

Consumer delivery counters are intentionally deferred until the delivery state machine is instrumented in a dedicated change; Outbox completion and overdue gauges already expose the business result of timeout consumption.

## Cardinality rules

Forbidden labels:

```text
request_id
trace_id
user_id
order_id
reservation_id
event_id
worker_id
raw URL
query string
error message
```

Allowed dynamic values must come from a reviewed finite set. Upstream host names are deployment configuration values rather than user input.

## Compose

The default business topology remains unchanged. Add Prometheus with:

```bash
docker compose -f compose.yml -f compose.observability.yml up -d --build --wait \
  --scale order-timeout-worker=2 \
  --scale order-reconciliation-worker=2
```

Prometheus is available on host port `9090` by default. The configuration scrapes all five HTTP services and both Worker types.

## Kubernetes

The base adds standard Pod annotations:

```text
prometheus.io/scrape: "true"
prometheus.io/path: /metrics
prometheus.io/port: <service port>
```

Worker containers also declare their metrics ports. This is a ServiceMonitor-independent scrape contract; Phase 7.1 does not install Prometheus Operator or a Kubernetes Prometheus server.

## Failure and rollback boundary

Metrics are additive. Reverting the metrics package, listeners, Compose overlay and Kubernetes annotations leaves business persistence, APIs, Saga behavior and existing deployment paths unchanged.
