# Outbox and Order Saga operational indicators

## Purpose

This stage makes reliability state inspectable before Prometheus is introduced. The implementation provides a stable internal snapshot that can later be reused by a Prometheus collector without duplicating database queries.

It is a read-only operational surface. It does not repair orders, change Saga states, publish messages or expose customer data.

## Collection model

`ReliabilityReporter` owns the snapshot query contract. One collection performs exactly two aggregate SQL statements:

1. one conditional aggregate over `order_timeout_outbox_v2`;
2. one conditional aggregate over `orders_v2`.

The implementation does not execute one query per status and does not load individual Order or Outbox records.

## Outbox indicators

The snapshot contains:

| Field | Meaning |
| --- | --- |
| `by_status.pending` | Outbox records in `pending` |
| `by_status.failed` | Outbox records in `failed` |
| `by_status.published` | Broker-confirmed records in `published` |
| `by_status.completed` | Completed timeout workflows |
| `leased` | Records with an active, unexpired publisher lease |
| `retry_ready` | Pending/failed records eligible for claim now |
| `overdue` | Records whose `due_at` is past and status is not completed |
| `oldest_actionable_age_seconds` | Age of the oldest pending/failed record |
| `maximum_attempts` | Maximum attempts value across the Outbox table |
| `total_failed_attempts` | Sum of attempts currently represented by failed records |

`retry_ready` requires all of the following:

```text
status IN (pending, failed)
next_attempt_at <= collected_at
lease_until IS NULL OR lease_until < collected_at
```

## Order Saga indicators

The snapshot includes counts for every current Order state:

```text
reserving
pending
paying
paid
cancelling
cancelled
finished
failed
reconciliation_required
```

Additional indicators:

| Field | Meaning |
| --- | --- |
| `reconciliation_required` | Number of orders explicitly requiring repair or review |
| `oldest_reconciliation_age_seconds` | Age since the oldest such order was last updated |
| `stuck_transient` | Orders remaining too long in a transient state |
| `transient_stuck_threshold_seconds` | Threshold applied to the snapshot |

The initial transient states are:

```text
reserving
paying
cancelling
```

`pending` is not classified as stuck because waiting for payment is an expected business state and is governed by the timeout Outbox.

Default threshold:

```text
ORDER_TRANSIENT_STUCK_THRESHOLD=5m
```

## Internal endpoint

Order Service exposes:

```http
GET /internal/v1/operations/reliability
X-Internal-Token: <shared internal token>
```

The route is protected by the existing constant-time internal-token middleware and remains inside the existing request-budget handler. It therefore preserves:

```text
X-Request-ID
X-Request-Deadline
```

The response contains only aggregate counts, ages, collection time and query duration. It does not expose:

- user IDs;
- order IDs;
- reservation IDs;
- Outbox payloads;
- failure reasons;
- customer or product data.

Example shape:

```json
{
  "collected_at": "2026-07-11T12:00:00Z",
  "query_duration_ms": 4,
  "outbox": {
    "by_status": {
      "pending": 2,
      "failed": 1,
      "published": 4,
      "completed": 100
    },
    "leased": 1,
    "retry_ready": 2,
    "overdue": 1,
    "oldest_actionable_age_seconds": 95,
    "maximum_attempts": 3,
    "total_failed_attempts": 3
  },
  "orders": {
    "by_status": {
      "reserving": 0,
      "pending": 10,
      "paying": 0,
      "paid": 90,
      "cancelling": 0,
      "cancelled": 5,
      "finished": 40,
      "failed": 2,
      "reconciliation_required": 1
    },
    "reconciliation_required": 1,
    "oldest_reconciliation_age_seconds": 120,
    "stuck_transient": 0,
    "transient_stuck_threshold_seconds": 300
  }
}
```

## Periodic structured summary

Each timeout Worker starts a separate read-only reporter loop. Default interval:

```text
ORDER_RELIABILITY_LOG_INTERVAL=1m
```

The log record is named:

```text
order reliability indicators
```

It emits flattened fields suitable for current log inspection and future collection. A snapshot query failure is logged as a warning and does not terminate or reconnect the RabbitMQ Worker session.

When two timeout Worker replicas run, both emit a summary. This is intentional for the current experiment, but Prometheus integration should scrape Order Service rather than count duplicated log summaries.

## Clock and duration semantics

Age calculations use the snapshot collection timestamp. The reporter clock is injectable in tests, making age boundaries deterministic.

`query_duration_ms` measures the two aggregate database calls using the process monotonic clock. It is operational evidence, not a database server execution-plan metric.

## Configuration

Optional runtime settings:

```text
ORDER_TRANSIENT_STUCK_THRESHOLD=5m
ORDER_RELIABILITY_LOG_INTERVAL=1m
```

Both reject invalid or non-positive durations at process startup.

## Remaining boundary

This stage does not provide:

- Prometheus exposition;
- Grafana dashboards;
- alert routing;
- automatic reconciliation;
- cross-service trace correlation;
- historical time-series storage.

Automatic repair is tracked separately so read-only observability is not coupled to state-changing reconciliation logic.
