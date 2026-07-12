# OpenTelemetry tracing verification

## Static and unit verification

The telemetry package tests cover:

- valid local Trace/Span IDs without an OTLP exporter;
- new trace creation for requests without inbound W3C headers;
- continuation of a valid `traceparent`;
- bounded server span names based on route groups;
- outbound client context injection;
- parent/child Trace ID continuity;
- trace/span correlation fields in structured logs;
- rejection of raw resource IDs and query values from span attributes.

All tests run under the normal unit test and race detector gates.

## Runtime trace smoke

The Observability Stack workflow sets a deterministic valid W3C context:

```text
TRACE_ID=0123456789abcdef0123456789abcdef
TRACEPARENT=00-0123456789abcdef0123456789abcdef-0123456789abcdef-01
TRACESTATE=goorder=ci
```

It then executes:

```bash
sh scripts/smoke/microservices-saga.sh
python3 scripts/smoke/tempo-trace.py
```

The Tempo verification requires the trace to contain resource attributes for:

```text
api-gateway
identity-service
catalog-service
inventory-service
order-service
```

It also requires:

- at least ten spans;
- the `order.create_saga` business span;
- a bounded `POST api_orders` HTTP span;
- no numeric resource ID or UUID in any span name.

## Existing regression gates

Tracing changes must continue to pass:

- golangci-lint;
- unit/integration tests;
- race detector;
- vet and package build;
- migration validation;
- seven service/Worker binaries;
- Docker image builds;
- four-database Compose runtime;
- complete Compose Order Saga;
- Prometheus targets/rules/metrics;
- Grafana provisioning;
- Kubernetes Kustomize contracts;
- real kind deployment, failed rollout, `rollout undo` and Kubernetes Saga.

## Failure diagnostics

On Observability Stack failure, CI stores:

- merged Compose process state and logs;
- Prometheus targets, rules and recording-series response;
- Grafana health, Prometheus datasource, Tempo datasource and Dashboard response;
- Tempo readiness response;
- the queried trace payload for the deterministic Trace ID.

## Acceptance boundary

A successful trace smoke proves application-level W3C propagation and OTLP export through the local Collector into Tempo. It does not prove production retention, tail sampling, Kubernetes tracing deployment, message-header propagation or managed-backend availability.
