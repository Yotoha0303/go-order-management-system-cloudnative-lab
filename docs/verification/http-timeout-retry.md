# HTTP timeout and retry verification

## Scope

This verification applies to Issue #20 and branch `phase/05-http-timeout-retry`.

## Budget middleware tests

The test suite must prove:

- a request receives an effective context deadline;
- an existing `X-Request-ID` is preserved;
- `X-Request-Deadline` is propagated in RFC3339Nano form;
- outbound requests receive the same request ID and deadline;
- an already-expired deadline returns HTTP 504 without calling the application handler.

## Retry executor tests

The retry executor is tested with deterministic sleep and jitter functions so tests do not depend on wall-clock backoff.

Required cases:

- HTTP 503/502/504 can be retried up to the configured attempt limit;
- HTTP 400 is returned after one attempt;
- the same request ID, deadline and stable JSON identity are present on every attempt;
- insufficient remaining budget prevents sleeping or starting another attempt;
- a slow upstream is stopped by the caller context deadline;
- caller cancellation/deadline exhaustion is not treated as a retryable upstream failure.

## Integration behavior

The existing Order Saga smoke test exercises the migrated clients:

- Order to Catalog product snapshot;
- Order to Inventory reserve;
- Order to Inventory confirm after payment;
- Order to Inventory release after cancellation;
- Worker to Order timeout cancellation;
- Catalog/Inventory to Identity role checks.

The complete flow must still verify idempotent order creation, payment confirmation, active cancellation and RabbitMQ timeout compensation.

## Regression gate

Before merge, GitHub Actions must pass:

- golangci-lint;
- all Go tests;
- race detector;
- vet;
- all package and service builds;
- legacy and service-owned migration validation;
- Docker Compose validation;
- all image builds;
- four-database topology startup;
- two timeout Worker replicas;
- Gateway readiness;
- complete Order Saga smoke test.

## Non-claims

A passing result does not mean the system has a circuit breaker, rate limiter or adaptive production timeout model. Those are separate delivery items in Issue #14.
