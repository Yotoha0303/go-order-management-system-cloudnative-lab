# Service migrations, outbox leasing and publisher confirms

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

If a process dies while holding a lease, another Worker can reclaim the event after `lease_until`.

## RabbitMQ Publisher Confirms

The Worker uses separate RabbitMQ channels for consuming timeout messages and publishing Outbox messages. The publisher channel enters confirm mode before any message is sent.

For each claimed Outbox event, the Worker now performs:

1. serialize the timeout payload;
2. publish one persistent RabbitMQ message;
3. wait for the corresponding broker confirmation within `RABBITMQ_PUBLISH_CONFIRM_TIMEOUT`;
4. set the Outbox status to `published` only after a positive ACK;
5. record a nack, confirmation timeout, channel closure or publish error as `failed`;
6. clear the failed event lease and release the unprocessed leases from the same claimed batch;
7. end the RabbitMQ session and reconnect before publishing another event after an uncertain result.

Only one publish is outstanding on a publisher channel at a time. This makes the ordered confirmation stream unambiguous. If confirmation times out, the channel is discarded so a late ACK cannot be mistaken for the next event.

Structured logs include:

- `event_id`;
- `order_id`;
- `worker_id`;
- `attempt`;
- `confirmation_outcome`.

The default confirmation timeout is five seconds. Local execution can override it with:

```env
RABBITMQ_PUBLISH_CONFIRM_TIMEOUT=5s
```

## Failure semantics

| Outcome | Outbox result | Session behavior |
| --- | --- | --- |
| Broker ACK | `published` | continue |
| Broker NACK | `failed` | reconnect |
| Confirmation timeout | `failed` | reconnect |
| Publisher channel closed | `failed` | reconnect |
| Immediate publish error | `failed` | reconnect |
| Worker crash while leased | unchanged until lease expiry | another Worker reclaims |

Publisher confirms remove the case where the Worker marks an event `published` without broker acknowledgement.

They do not provide exactly-once delivery. A crash after RabbitMQ sends an ACK but before the database update can still cause the event to be published again after lease recovery. Timeout cancellation therefore remains idempotent.

## Scaling rule

`order-timeout-worker` has no fixed Compose `container_name`, so it can be scaled:

```bash
docker compose up -d --wait --scale order-timeout-worker=2
```

The CI pipeline starts two replicas and verifies both are running. Horizontal scaling does not require leader election for Outbox polling because database leases provide exclusive active ownership.

## Verification

The Publisher Confirm change is verified by:

- unit tests for ACK, NACK, closed confirmation channel and timeout outcomes;
- a real RabbitMQ integration test that receives a positive broker ACK;
- a MySQL Outbox test proving an unconfirmed event becomes `failed`, remains retryable and is not marked `published`;
- the existing two-Worker Compose startup check;
- the complete Order Saga smoke test.

## Remaining constraints

- Delivery remains at least once because RabbitMQ acknowledgement and the database `published` update are not one atomic transaction.
- Migration jobs currently use the same MySQL root credential as runtime services. Production deployment should use separate least-privilege accounts.
- Service migration rollback is available through Goose, but automatic rollback during deployment is intentionally not performed.
