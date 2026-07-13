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

It reports attempted throughput, successful throughput, errors, P50/P95/P99 latency, container CPU/memory, MySQL state, RabbitMQ state and application metrics. The test also identifies the first observed capacity boundary when the evidence supports one.

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

## Readiness boundary

Before baseline capture, the workflow queries Prometheus `/api/v1/targets` and requires at least one healthy target for all seven Prometheus scrape jobs:

```text
api-gateway
identity-service
catalog-service
inventory-service
order-service
order-timeout-worker
order-reconciliation-worker
```

A single API Gateway request series is not enough. If any downstream service or Worker job has not been scraped successfully, the measurement does not start and the target snapshot remains in the evidence artifact.

## Bounded test profile

Default profile:

| Setting | Value |
| --- | ---: |
| Warm-up | 5 seconds, maximum 200 requests |
| Concurrency levels | 1, 4, 8, 16, 32 |
| Measured duration per level | up to 8 seconds |
| Measured request ceiling per stage | 3000 |
| Maximum total measured requests | 15000 |
| Per-request timeout | 10 seconds |
| Resource sampling | every 2 seconds during measured stages only |
| Sampler start wait | 60 seconds |
| Sampler measured-interval ceiling | 180 seconds |
| Workflow timeout | 30 minutes |

Hard code limits prevent concurrency above 32, a measured stage above 15 seconds, warm-up above 10 seconds, more than 3000 requests in one measured stage or more than 15000 requests across the five-stage profile.

Each measured stage receives its own 3000-request safety ceiling. The ceiling is not divided across all stages. This allows lower-concurrency stages to run for the intended eight seconds while retaining a hard bound for higher-concurrency stages. Each stage records its configured duration, issuance duration and stop reason. A stage that reaches its own ceiling before eight seconds remains in the artifact but is marked `measurement_eligible=false`; its burst RPS and latency are excluded from sustained-duration capacity evidence.

Gateway rate-limit values are raised only inside this disposable workflow so the first measured boundary is more likely to be the synchronous business path rather than the default protective token bucket. HTTP 429 remains recorded and is classified explicitly when it occurs.

Order timeout delay is set to 10 minutes, longer than the bounded measurement, so automatic cancellation does not rewrite the test orders during latency measurement. RabbitMQ publication, Outbox and queue state remain observable.

## Measurement interval and resource sampling

Fixture creation and warm-up execute before the measured interval. After warm-up, the load driver writes `measurement-start`; the resource sampler waits for that marker before taking its first `docker stats` snapshot. After all measured stages finish, the workflow writes `load-complete`.

The sampler checks `load-complete` before every new snapshot and again after each snapshot. Therefore:

- fixture creation is excluded from resource peaks;
- warm-up is excluded from resource peaks;
- no new snapshot starts after completion;
- reaching the 180-second safety ceiling before completion fails the workflow instead of accepting incomplete resource evidence.

The load summary also records `measurement_started_at` and `measurement_finished_at`.

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
- attempted throughput in requests/second;
- successful throughput in requests/second;
- P50, P95, P99 and maximum latency;
- configured duration and request-issuance duration;
- stop reason;
- sustained-duration eligibility.

Warm-up requests are reported separately and excluded from measured request totals.

## Operational evidence

Before and after the measurement, the workflow captures:

- `docker stats --no-stream`;
- MySQL global status, service table counts, order state and Outbox state;
- RabbitMQ queue names, messages, ready/unacknowledged counts and consumers;
- Prometheus queries using emitted metric labels;
- Gateway readiness.

The emitted metric labels are preserved as follows:

- HTTP server requests: `service`, `method`, `route_group`, `status_class`;
- HTTP client attempts: `upstream`, `operation`, `outcome`, `status_class`, `retryable`;
- RabbitMQ publish and delivery: application `outcome` plus Prometheus target `job`;
- RabbitMQ session and queue gauges: `job`, queue role and state where applicable.

During the measured stages only, `resource_sampler.py` writes timestamped container resource samples as JSON Lines.

Failure diagnostics include Compose state, complete container logs, current resource state and Prometheus target status. Cleanup always removes containers, networks and volumes.

## Capacity-boundary rules

`analyze_load.py` separates measured evidence from inference and evaluates every stage chronologically. At each concurrency level it applies:

1. **Measured request ceiling before stage duration**: records a measurement-safety boundary and excludes that truncated stage from sustained capacity inference.
2. **Gateway rate limit**: any HTTP 429 is a hard measured boundary.
3. **Request error boundary**: error rate reaches at least 2%.
4. **Throughput plateau with tail growth**: from the previous sustained-duration level to the current level, successful throughput grows less than 15% while P95 grows more than 30%.
5. **Not reached within bounded range**: none of the above occurs.

Because candidates are checked in chronological order, a plateau first observed at concurrency 8 cannot be hidden by request errors that appear later at concurrency 32.

The reported **best healthy sustained successful throughput** uses only stages before the first observed boundary that:

- completed their configured duration;
- have error rate below 2%;
- contain no HTTP 429.

The boundary stage and all later stages are excluded. Fast failed responses therefore cannot inflate the reported useful throughput. The reported highest healthy P95 and healthy stage count use the same pre-boundary set.

For a plateau, the highest sampled container CPU is recorded only as a diagnostic lead. It is not declared the root cause without corroborating MySQL, RabbitMQ, metrics, logs or trace evidence.

When no boundary is reached, the report explicitly states that the test ceiling was reached before defensible saturation. It does not invent a bottleneck.

## Repeatability and comparison

Compare two runs only when all of the following match:

- source commit and code path;
- GitHub Runner class;
- concurrency levels, stage duration, per-stage request ceiling and total request ceiling;
- the same set of healthy sustained pre-boundary stages;
- Gateway rate-limit overrides;
- timeout delay;
- Worker replica counts;
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
