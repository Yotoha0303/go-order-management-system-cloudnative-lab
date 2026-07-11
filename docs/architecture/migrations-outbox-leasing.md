# Service migrations and outbox leasing

## Service-owned migrations

Runtime services no longer call GORM `AutoMigrate`. Schema changes are applied by dedicated, one-shot Goose jobs before the owning service starts.

| Service | Migration directory | Database |
| --- | --- | --- |
| Identity | `migrations/identity` | `go_order_identity` |
| Catalog | `migrations/catalog` | `go_order_catalog` |
| Inventory | `migrations/inventory` | `go_order_inventory` |
| Ordering | `migrations/ordering` | `go_order_ordering` |

Each database has its own Goose version table. A service process therefore needs data read/write permissions, while migration credentials can be separated later without changing application code.

The legacy root migration directory remains only for monolith regression validation. It is not used by the microservices Compose runtime.

## Outbox delivery semantics

The timeout outbox is an at-least-once delivery mechanism. Multiple worker replicas may poll the same table, but an event can be actively owned by only one publisher lease at a time.

A claim is selected using:

```sql
SELECT ...
FROM order_timeout_outbox_v2
WHERE status IN ('pending', 'failed')
  AND next_attempt_at <= NOW(3)
  AND (lease_until IS NULL OR lease_until < NOW(3))
ORDER BY id
LIMIT ?
FOR UPDATE SKIP LOCKED;
```

The claiming transaction writes:

- `lease_owner`: unique worker identifier;
- `lease_until`: crash-recovery deadline;
- `next_attempt_at`: retry scheduling boundary.

A successful publish clears the lease and sets `published`. A failed publish clears the lease, sets `failed`, increments attempts and delays the next attempt. If a process dies while holding a lease, another worker can reclaim the event after `lease_until`.

A crash after RabbitMQ accepted a message but before the database update can still cause a duplicate publish. This is intentional at-least-once behavior; timeout cancellation remains idempotent.

## Scaling rule

`order-timeout-worker` no longer has a fixed Compose `container_name`, so it can be scaled:

```bash
docker compose up -d --wait --scale order-timeout-worker=2
```

The CI pipeline starts two replicas and verifies both are running. Horizontal scaling beyond this point does not require a leader election mechanism for outbox polling.

## Remaining constraints

- RabbitMQ publisher confirms are not yet enabled; a future change should confirm broker acceptance before marking an event published.
- Migration jobs currently use the same MySQL root credential as runtime services. Production deployment should use separate least-privilege accounts.
- Service migration rollback is available through Goose, but automatic rollback during deployment is intentionally not performed.
