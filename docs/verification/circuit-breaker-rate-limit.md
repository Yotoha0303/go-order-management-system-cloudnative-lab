# Circuit breaker and rate-limit verification

## Deterministic circuit-breaker tests

The test clock advances explicitly and does not wait for wall-clock time.

Required cases:

- closed calls remain allowed below the threshold;
- consecutive selected failures open the circuit;
- an open circuit returns `ErrCircuitOpen` without network I/O;
- expiry of the open interval permits a half-open probe;
- a successful probe closes the circuit;
- a failed probe reopens the circuit;
- half-open probe concurrency is bounded;
- HTTP 4xx responses do not open the circuit.

## Deterministic token-bucket tests

The limiter is constructed with an injectable clock.

Required cases:

- the configured burst is accepted immediately;
- the next request is rejected with a calculated retry duration;
- advancing the clock refills tokens at the configured rate;
- different client keys use independent buckets;
- the global bucket limits aggregate traffic;
- inactive client buckets are removed;
- client state never exceeds the configured maximum;
- the oldest bucket is evicted when the maximum is reached.

## Gateway HTTP contract tests

The Gateway test verifies:

- the first request reaches routing;
- the next request from the same source IP receives HTTP 429;
- `Retry-After` is present and is at least one second;
- `X-Request-ID` is present;
- the JSON response contains `rate_limited`, request ID and retry duration;
- `/live` and `/readyz` remain available after the client and global buckets are exhausted;
- IPv6 source addresses are normalized without their port.

## Full regression gate

The implementation is not complete until the existing CI still passes:

```text
golangci-lint
go test ./...
go test -race ./...
go vet ./...
go build ./...
legacy migration validation
four service migration validations
six service binary builds
Docker Compose configuration validation
all microservice image builds
four-database runtime startup
two timeout Worker replicas
Gateway readiness
complete Order Saga smoke test
```

## Expected runtime evidence

During failure testing, structured logs should show:

```text
HTTP circuit state changed
upstream HTTP circuit rejected request
```

During rate-limit rejection, the Gateway must respond before any reverse-proxy network call begins.

## Remaining limits

Passing this verification does not prove:

- shared circuit state across replicas;
- cluster-wide rate limiting;
- adaptive concurrency control;
- automatic tuning of thresholds;
- Prometheus metrics or alerts.
