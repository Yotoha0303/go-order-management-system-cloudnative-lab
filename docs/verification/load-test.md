# Bounded Order-Creation Load Test

## Goal

The Phase 8.5 load test measures the synchronous order-creation path under bounded synthetic concurrency:

```text
API Gateway
  -> Order Service
    -> Catalog snapshot
    -> Inventory reservation
    -> Ordering transaction and Timeout Outbox
```

It reports throughput, errors, P50/P95/P99 latency, container CPU/memory, MySQL state, RabbitMQ state and application metrics. The test also identifies the first observed capacity boundary when the evidence supports one.

This is a single GitHub-hosted Runner, short-duration, synthetic measurement. It is not a production SLO, benchmark, capacity guarantee or hardware comparison.

## Workload profile

Each measured request is:

```http
POST /api/v1/orders
Authorization: Bearer <in-memory synthetic buyer token>
Content-Type: application/json
```

The body contains:

- one on-sale synthetic product;
- quantity `1`;
- a globally unique Idempotency Key;
- a unique Request ID.

The workflow creates:

- one synthetic administrator;
- one synthetic buyer;
- one dedicated on-sale product;
- inventory quantity `100000`.

Tokens remain in the Python process memory. They are not written to command-line arguments, environment artifacts, JSONL samples, Markdown reports or GitHub Issues.

## Bounded test profile

Default profile:

| Setting | Value |
| --- | ---: |
| Warm-up | 5 seconds, maximum 200 requests |
| Concurrency levels | 1, 4, 8, 16, 32 |
| Measured duration per level | 8 seconds |
| Measured request ceiling | 3000 |
| Per-request timeout | 10 seconds |
| Resource sampling | every 2 seconds, maximum 70 seconds |
| Workflow timeout | 30 minutes |

Hard code limits prevent concurrency above 32, a measured stage above 15 seconds, warm-up above 10 seconds or more than 3000 measured requests.

Gateway rate-limit values are raised only inside this disposable workflow so the first measured boundary is more likely to be the synchronous business path rather than the default protective token bucket. HTTP 429 remains recorded and is classified explicitly when it occurs.

Order timeout delay is set to 10 minutes, longer than the bounded measurement, so automatic cancellation does not rewrite the test orders during latency measurement. RabbitMQ publication, Outbox and queue state remain observable.

## Measurement output

`scripts/load/order_create_load.py` writes:

- `samples.jsonl`: one record per measured request;
- `load-summary.json`: machine-readable stage totals;
- `load-summary.md`: stage table;
- `fixture.json`: non-secret synthetic fixture metadata.

Each stage reports:

- total requests;
- successes and errors;
- error rate;
- status and outcome counts;
- throughput in requests/second;
- P50, P95, P99 and maximum latency.

Warm-up requests are reported separately and excluded from measured request totals.

## Operational evidence

Before and after the measurement, the workflow captures:

- `docker stats --no-stream`;
- MySQL global status, service table counts, order state and Outbox state;
- RabbitMQ queue names, messages, ready/unacknowledged counts and consumers;
- Prometheus HTTP request, latency, client attempt and RabbitMQ queries;
- Gateway readiness.

During the measured interval, `resource_sampler.py` writes timestamped container resource samples as JSON Lines.

Failure diagnostics include Compose state, complete container logs, current resource state and Prometheus target status. Cleanup always removes containers, networks and volumes.

## Capacity-boundary rules

`analyze_load.py` separates measured evidence from inference and evaluates stages in this order:

1. **Gateway rate limit**: any HTTP 429 is a hard measured boundary.
2. **Request error boundary**: error rate reaches at least 2%.
3. **Throughput plateau with tail growth**: from one level to the next, throughput grows less than 15% while P95 grows more than 30%.
4. **Not reached within bounded range**: none of the above occurs.

For a plateau, the highest sampled container CPU is recorded only as a diagnostic lead. It is not declared the root cause without corroborating MySQL, RabbitMQ, metrics, logs or trace evidence.

When no boundary is reached, the report explicitly states that the test ceiling was reached before defensible saturation. It does not invent a bottleneck.

## Repeatability and comparison

Compare two runs only when all of the following match:

- source commit and code path;
- GitHub Runner class;
- concurrency levels, stage duration and request ceiling;
- Gateway rate-limit overrides;
- timeout delay;
- worker replica counts;
- workload body and inventory size.

A single run may identify a regression or diagnostic lead, but stable capacity claims require repeated runs and variance analysis that are outside the project closure scope.

## Limitations

The test does not model:

- real user think time or traffic distribution;
- multi-region or multi-node Kubernetes;
- cloud load balancers, ingress or TLS;
- production database size, indexes, storage latency or managed-service behavior;
- long-duration memory leaks, connection churn or queue accumulation;
- mixed reads/writes, payment traffic or cancellation ratios;
- network shaping, packet loss or geographical latency;
- production security controls and rate-limit identity behind trusted proxies.

## Rollback boundary

Removing the load workflow and scripts removes only the synthetic GitHub Actions measurement. It does not change public API behavior, database Schema, GHCR images, automatic test CD, backup/restore, fault drills or normal CI.
