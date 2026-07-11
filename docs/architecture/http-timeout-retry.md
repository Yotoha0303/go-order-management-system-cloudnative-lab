# HTTP timeout budget and bounded retry contract

## Goal

The microservices use one propagated request budget instead of independent, unconstrained service timeouts. Retries are operation-specific, bounded and stopped before the caller deadline is exhausted.

## Deadline propagation

The Gateway and all four HTTP services apply `resiliencehttp.BudgetHandler`.

Headers:

- `X-Request-ID`: generated when absent and preserved across every hop;
- `X-Request-Deadline`: an absolute UTC RFC3339Nano deadline.

Rules:

1. the Gateway creates a ten-second default budget when the client provides no deadline;
2. an incoming deadline is capped at thirty seconds;
3. every service keeps the earlier of the propagated deadline and its local server timeout;
4. expired requests receive HTTP 504 before entering the downstream handler;
5. every service-to-service attempt receives the same request ID and effective deadline;
6. retries stop when the remaining budget cannot cover the backoff and a minimum next-attempt gap.

The Gateway does not retry proxied business requests. It only propagates the budget and maps deadline failures to HTTP 504.

## Transport timeouts

Service clients use a shared HTTP transport with explicit boundaries:

| Boundary | Default |
| --- | ---: |
| connect timeout | 500 ms |
| TLS handshake timeout | 1 s |
| response header timeout | 1.5–2 s by client |
| total single-attempt timeout | 3–5 s by client |
| idle connection timeout | 90 s |

The caller context deadline always remains the outer limit.

## Retry policy

The default service policy uses:

- maximum three attempts;
- exponential backoff starting at 50 ms;
- maximum backoff of 300 ms;
- bounded jitter;
- minimum 100 ms remaining budget before another attempt.

Retryable outcomes:

- selected network/transport errors while the caller context remains valid;
- HTTP 502;
- HTTP 503;
- HTTP 504.

Permanent outcomes are returned immediately:

- HTTP 4xx;
- domain validation and conflict errors;
- successful non-2xx statuses outside 502/503/504;
- caller cancellation or caller deadline exhaustion.

## Operation matrix

| Operation | Policy | Safety condition |
| --- | --- | --- |
| Catalog product snapshot | bounded retry | read-only |
| Identity role check | bounded retry | read-only |
| Inventory reserve | bounded retry | every attempt reuses the same reservation ID and payload |
| Inventory confirm | bounded retry | endpoint is idempotent |
| Inventory release | bounded retry | endpoint is idempotent |
| Order timeout cancel | bounded retry | endpoint is idempotent |
| Gateway proxy | no retry | Gateway must not replay external writes |
| register/login | no client retry | external caller decides |

## Request reconstruction

JSON payloads are encoded once and a new request/body reader is created for every attempt. Internal service authentication, request ID, deadline and stable reservation identity are applied to every reconstructed request.

## Structured attempt logs

Each service-client attempt records:

- request ID;
- upstream service;
- operation;
- attempt number;
- outcome;
- HTTP status;
- retryable decision;
- attempt duration;
- remaining request budget;
- transport error when present.

## Remaining limits

- this phase does not include circuit breakers or rate limiting;
- retry state is in-process and not shared between replicas;
- no adaptive timeout calculation is performed from latency percentiles;
- OpenTelemetry trace context is not yet implemented, but the request budget package is ready to propagate additional headers later.
