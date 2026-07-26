# gRPC contract for the inventory reservation API

## Scope

This is the first Protobuf contract in the repository and the opening step of
the gRPC migration. It defines `inventory.v1.InventoryReservationService`, the
three calls order-service makes into inventory-service during the order saga,
and wires up code generation.

inventory-service now serves this contract on port 9085 alongside its HTTP
listener. Nothing calls it yet: order-service still uses the internal HTTP
endpoints, which remain authoritative until the client is moved and verified.

## Layout

| Path | Contents |
| --- | --- |
| `api/proto/inventory/v1/inventory.proto` | The contract |
| `buf.yaml` | Module, lint rules, breaking-change rules |
| `buf.gen.yaml` | Plugin configuration |
| `internal/platform/grpcapi/inventory/v1/` | Generated Go code |

Generated code sits under `internal/platform/` next to `internalapi`,
`serviceclient` and `servicehost`, the packages that already carry
service-to-service concerns. That placement also keeps it inside the
`./internal/...` entry of the Makefile's `GO_PACKAGES`, so `make fmt`, `vet`,
`test` and `lint` pick it up with no change to that variable. Generated files
carry the standard `Code generated ... DO NOT EDIT.` header, which
`.golangci.yml` already excludes through `exclusions.generated: strict`.

```bash
make proto-lint      # lint the contract
make proto-gen       # regenerate into internal/platform/grpcapi/
make proto-breaking  # compare against origin/main
```

`BUF` defaults to `go run github.com/bufbuild/buf/cmd/buf@v1.58.0`, so the
targets work with no prior setup. Override it with `make proto-gen BUF=buf`
after installing buf locally to avoid rebuilding it each time.

## What the contract preserves

The Protobuf definition was derived from the current HTTP handlers and their
caller, not designed fresh, so the two can be compared during the migration.

| HTTP | RPC |
| --- | --- |
| `POST /internal/v1/reservations` | `Reserve` |
| `POST /internal/v1/reservations/:id/confirm` | `Confirm` |
| `POST /internal/v1/reservations/:id/release` | `Release` |

Validation limits are carried over as comments rather than being invented:
`order_id` and every `product_id` and `quantity` must be positive, `items` holds
between 1 and 100 entries, and lines repeating a `product_id` are summed and
then sorted by `product_id` before rows are locked, which is what stops
concurrent reservations from deadlocking.

`reservation_id` stays caller-supplied and optional. Order-service generates it
before calling so that a request which times out with an unknown outcome can
still be released, and the server generates one only when the field is unset.

Idempotency is the property the saga leans on hardest, and it differs per call.
`Reserve` is keyed on `order_id`, which is unique per reservation, so a repeat
call returns the existing reservation rather than reserving twice. `Confirm` and
`Release` return the reservation unchanged when it already holds the target
status. Order-service and order-reconciliation-worker retry compensation
indefinitely and cannot distinguish a lost request from a lost reply, so these
guarantees are what keep retries from double-counting stock.

## Two shapes the HTTP contract had blurred

**Request and response items are not the same message.** The HTTP request item
carries `product_id` and `quantity`; the stored item also carries `id` and
`reservation_id`. Order-service reuses one Go struct for both and relies on
`encoding/json` silently dropping the unknown fields. Protobuf has no such
escape hatch, so the contract declares `ReserveItem` and `ReservationItem`
separately. The same mismatch applies to `created_at` and `updated_at`, which
the server sends and the client's struct omits.

**Status is an enum, not a free string.** `pending`, `confirmed` and `released`
were string constants compared by value. They are now
`RESERVATION_STATUS_PENDING`, `_CONFIRMED` and `_RELEASED`, with
`RESERVATION_STATUS_UNSPECIFIED` at zero as proto3 requires.

## Compatibility

`buf.yaml` enables the `FILE` breaking-change rule set, which enforces the
compatibility rules this project commits to: field numbers are never reused
after removal, existing field types never change, and additions stay optional.
`make proto-breaking` compares the working tree against `origin/main`, so it
reports what a reviewer would see rather than what happens to be uncommitted.

The check needs `origin/main` to already contain the contract, so it only starts
returning useful results from the commit after this one.

## The server

`internal/inventorysvc/grpc.go` implements the three methods on top of the same
`Service` the HTTP handlers use, so the two surfaces cannot drift apart. It is
registered in `cmd/inventory-service/main.go` on `GRPC_PORT`, default 9085, and
served next to HTTP through `servicehost.StartGRPC`.

Authentication moved to `internalapi.UnaryServerInterceptor`, the gRPC
counterpart of the existing middleware. It reads the `x-internal-token`
metadata key - lowercase, because gRPC rejects uppercase keys - keeps the
constant-time comparison, and still refuses every call when the expected token
is unset. It is a server-wide interceptor rather than a per-method check so a
method added later cannot be left unauthenticated by omission.

Error mapping is the reason the move is worth making. The HTTP endpoints answer
409 for insufficient stock, an unknown product and a malformed request alike,
and return three mutually incompatible error shapes: `{"error": "..."}` for
business failures, `{"code": 40101, "msg": "..."}` from the auth middleware, and
`{"code": "request_deadline_exceeded", ...}` from the budget handler.
Order-service parses none of them - it stores the raw body in `RemoteError` - so
it cannot tell a definitive rejection from an unknown-outcome transport failure,
and every compensation branch treats them alike.

The split that matters is whether a retry could succeed:

| Domain error | Status | Retry could succeed |
| --- | --- | --- |
| `ErrInvalidInventoryAmount` | `INVALID_ARGUMENT` | no |
| `ErrInventoryNotFound`, `ErrReservationNotFound` | `NOT_FOUND` | no |
| `ErrInsufficientInventory`, `ErrReservationTransition` | `FAILED_PRECONDITION` | no |
| storage failures | `INTERNAL` | maybe |

`INTERNAL` deliberately drops the underlying message, which can carry SQL
fragments and constraint names; a test asserts that.

## Remaining work

1. The order-service client. Resilience parity is the delicate part: the HTTP
   client retries at most 3 attempts with 50ms then 100ms backoff and +/-20%
   jitter, retrying only transport errors and 502, 503 and 504, and its circuit
   breaker opens after 5 consecutive failures for 5 seconds per upstream and
   operation. One detail is easy to lose - only retryable outcomes count as
   circuit failures, so HTTP 409 and 500 are recorded as successes today.
   Counting every non-OK gRPC status as a failure would make the breaker trip
   far more readily than it does now.
2. Compensation branches in `order.go` that could act on the new distinction
   instead of treating every failure alike.
3. Health checking. The gRPC port has no probe: Kubernetes still probes
   `/readyz` over HTTP, which does not prove the gRPC listener is up. The gRPC
   health checking protocol would close that gap.
4. The other three internal endpoints - identity's role check, catalog's product
   snapshot and order's timeout cancel - have no contract yet.

Until the client moves, the HTTP endpoints remain the path in use.
