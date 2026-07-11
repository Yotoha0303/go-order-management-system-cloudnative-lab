# Reliability indicator verification

## Database integration coverage

The MySQL integration test creates isolated Ordering tables and inserts controlled Outbox and Order records.

The test verifies:

- all four Outbox status counts;
- active lease detection;
- claim eligibility based on status, retry time and lease expiry;
- overdue records;
- oldest actionable age;
- maximum attempts;
- failed-attempt total;
- every current Order status count;
- reconciliation-required count and oldest age;
- stale `reserving`, `paying` and `cancelling` detection;
- a configurable transient-state threshold.

The reporter clock is fixed in the test, so age values do not depend on wall-clock timing.

## Empty database behavior

An empty Ordering database must return:

- zero counts;
- zero ages;
- a valid collection timestamp;
- no scan or NULL conversion errors.

## Query count

A counting GORM logger wraps the test database. One snapshot must execute exactly two SQL statements:

```text
1 Outbox aggregate
1 Order aggregate
```

This protects the implementation against status-by-status query growth and N+1 behavior.

## Internal endpoint contract

Endpoint:

```http
GET /internal/v1/operations/reliability
```

Tests verify:

- missing internal token returns HTTP 401;
- an unauthorized request never reaches the snapshotter;
- a valid token returns HTTP 200 and the stable snapshot JSON;
- collection failures return a generic HTTP 500 response;
- database error details are not exposed.

## Worker logging behavior

The timeout Worker starts the reporting loop independently from the RabbitMQ session. Operational expectations:

- indicator-query failure logs a warning;
- query failure does not terminate the Worker;
- RabbitMQ reconnect behavior remains unchanged;
- cancellation of the process context stops the reporter loop.

## Full regression gate

The feature is not complete until the existing CI remains green:

```text
golangci-lint
go test ./...
go test -race ./...
go vet ./...
go build ./...
legacy migration validation
four service migration validations
six service binary builds
Docker Compose validation
all microservice image builds
four-database startup
two timeout Worker replicas
Gateway readiness
complete Order Saga smoke test
```

## Non-claims

Passing these tests does not mean Prometheus, alerting or automatic reconciliation has been implemented. The JSON snapshot and structured logs are the reusable operational data source for those later stages.
