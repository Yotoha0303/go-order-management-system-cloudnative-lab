# OpenTelemetry distributed tracing

## Scope

Phase 7.3 adds standards-based tracing without changing business APIs, database schemas, Saga states, message contracts, timeout budgets, retry behavior, circuit breakers or rate limits.

Tracing is optional. Services always create valid local trace contexts, while OTLP export is enabled only when an OTLP endpoint is configured. Collector, Tempo and Grafana remain outside application readiness dependencies.

## Runtime topology

```text
Client
  -> API Gateway
      -> Identity / Catalog / Inventory / Order
          -> internal HTTP dependencies

service spans
  -> OTLP/HTTP
  -> OpenTelemetry Collector
  -> OTLP/gRPC
  -> Tempo
  -> Grafana Tempo datasource
```

Compose adds:

```text
otel-collector:4318
Tempo API:      :3200
Grafana Tempo datasource UID: tempo
```

The default business Compose topology does not require these components.

## Propagation

The global propagator is W3C Trace Context:

```text
traceparent
tracestate
```

Inbound HTTP middleware extracts valid parent context before creating a server span. Outbound HTTP transports create a client span and inject the resulting context into cloned request headers.

`X-Request-ID` and Trace ID remain separate concepts:

- Request ID is the existing application correlation and request-budget identifier.
- Trace ID is the distributed tracing identifier generated or continued by OpenTelemetry.

HTTP responses expose diagnostic headers when a valid span exists:

```text
X-Trace-ID
X-Span-ID
```

These headers are observability aids and are not authentication or idempotency tokens.

## Span model

HTTP server and client span names use bounded route groups rather than raw paths:

```text
POST api_orders
GET api_products
POST internal_inventory
```

The Order create path adds the bounded business span:

```text
order.create_saga
```

Workers add bounded operational spans such as:

```text
timeout_worker.publish_batch
timeout_worker.consume
reconciliation_worker.process_batch
reconciliation_worker.process_task
```

Retries create separate client-attempt spans because each attempt is a real network operation. Circuit-open rejection occurs before network transport and therefore does not create a transport span.

## Allowed attributes

Current span attributes use finite or deployment-controlled values:

```text
http.request.method
http.response.status_code
server.address
go_order.route_group
go_order.outcome
go_order.saga.operation
go_order.batch_size
```

Forbidden span attributes and span-name content:

```text
request body
Authorization or internal tokens
passwords
raw query string
user_id
order_id
reservation_id
event_id
raw error text
resource IDs embedded in span names
```

Trace IDs and Span IDs may appear in logs but must never become Prometheus labels.

## Log correlation

The shared `slog.Handler` appends:

```text
trace_id
span_id
```

only when the log call uses a Context containing a valid span. HTTP client attempt logs and instrumented Worker/Saga paths use `InfoContext`, `WarnContext` or `ErrorContext` so log records can be correlated with Tempo traces.

## Sampling and export

Configuration:

```text
OTEL_EXPORTER_OTLP_ENDPOINT
OTEL_EXPORTER_OTLP_TRACES_ENDPOINT
OTEL_EXPORTER_OTLP_PROTOCOL
OTEL_TRACES_SAMPLER_ARG
OTEL_DEPLOYMENT_ENVIRONMENT
```

The sampler is parent-based with a configurable root ratio. The local Compose overlay defaults to ratio `1` for deterministic verification.

When no exporter is configured, the process still installs an SDK provider and generates valid local contexts, but spans are not sent to a backend. If exporter initialization fails, local tracing remains available and the service logs a warning.

Production sampling, tail sampling and managed backends are intentionally deferred.

## Compose verification

The Observability Stack workflow:

1. validates the Compose overlay;
2. starts the application, Prometheus, Grafana, Collector and Tempo;
3. executes the complete Order Saga with a fixed valid W3C trace context;
4. verifies Prometheus and Grafana contracts;
5. queries Tempo by Trace ID;
6. requires spans from Gateway, Identity, Catalog, Inventory and Order;
7. requires `order.create_saga` and bounded HTTP span names;
8. rejects span names containing numeric resource IDs or UUIDs.

## Known boundaries

- RabbitMQ messages do not yet carry W3C trace headers; Publisher and consumer work is represented by local Worker spans.
- No baggage is propagated.
- No tail sampling or production sampling policy is defined.
- No Kubernetes Collector/Tempo deployment is included in this phase.
- No database spans or SQL statement capture are enabled.
- Tempo local storage is for reproducible development and CI, not production retention.

## Rollback

Tracing is additive. Reverting the telemetry package, OTLP configuration, Collector/Tempo services and trace smoke test removes distributed tracing without changing persistence, business APIs, metrics, Saga behavior or deployment readiness contracts.
