# Automatic Order reconciliation worker

## Purpose

The reconciliation worker repairs explicitly classified Order Saga inconsistencies that have already entered `reconciliation_required`.

It is not a generic workflow engine and it never infers repair intent from `failure_reason`. The intended action is stored as structured task data when the Order status changes.

## Transactional task creation

Ordering migration `00003_reconciliation_tasks.sql` adds:

```text
order_reconciliation_tasks
trg_orders_v2_create_reconciliation_task
```

The trigger runs after an Order status update. When an Order first enters `reconciliation_required`, the trigger maps the previous state to an explicit action:

| Previous Order state | Reconciliation action |
| --- | --- |
| `reserving` | `release_inventory_and_fail` |
| `cancelling` | `finalize_cancel` |
| `paying` | `finalize_payment` |
| any other state | `unsupported_from_<state>` with task status `unresolved` |

Because the trigger executes in the same MySQL transaction as the Order update, these two facts cannot diverge:

```text
Order is reconciliation_required
Task describing the required action exists
```

Rolling back the Order transition also rolls back task creation.

The trigger uses only the previous structured Order state. It does not read, parse or match free-form failure text.

## Task model

Each task stores:

```text
order_id
action
status
attempts
next_attempt_at
lease_owner
lease_until
last_error
created_at
updated_at
```

Task statuses:

| Status | Meaning |
| --- | --- |
| `pending` | ready for first claim |
| `failed` | a supported repair failed and remains retryable |
| `completed` | remote and local repair completed |
| `unresolved` | unsupported action or incompatible Order state; no automatic retry |

A unique `(order_id, action)` key prevents duplicate task creation for the same classified inconsistency.

## Worker process

A separate binary and image run the repair loop:

```text
cmd/order-reconciliation-worker
order-reconciliation-worker
```

The process owns no HTTP port. It connects to:

- `go_order_ordering` for task and Order state;
- Inventory Service through its internal authenticated API.

It reuses `InventoryClient`, so remote repair calls inherit:

- request deadlines;
- bounded retries;
- exponential backoff;
- operation-scoped circuit breakers;
- stable reservation identifiers.

## Claim and lease model

Workers claim tasks using:

```sql
SELECT ...
FROM order_reconciliation_tasks
WHERE status IN ('pending', 'failed')
  AND next_attempt_at <= NOW
  AND (lease_until IS NULL OR lease_until < NOW)
ORDER BY id
LIMIT ?
FOR UPDATE SKIP LOCKED;
```

The claiming transaction writes:

```text
attempts = attempts + 1
lease_owner = worker identifier
lease_until = current time + lease duration
```

Multiple replicas can poll the same table. An active task is owned by one Worker lease; another Worker can reclaim it after the lease expires.

This is an at-least-once repair model. A process may crash after the remote Inventory action succeeds but before the local transaction completes. The next Worker repeats the same idempotent remote action and then completes the local transition.

## Supported repair actions

### `release_inventory_and_fail`

Original inconsistency:

```text
Inventory reservation succeeded
Order pending/outbox local transaction failed
Release compensation failed
```

Repair:

1. call Inventory `release` with the original reservation ID;
2. update Order from `reconciliation_required` to `failed`;
3. mark the task `completed`.

The existing Order failure history is preserved.

### `finalize_cancel`

Original inconsistency:

```text
Inventory release succeeded
Local cancellation/outbox completion transaction failed
```

Repair:

1. call Inventory `release` again; the endpoint is idempotent;
2. update Order to `cancelled`;
3. complete any pending/published/failed timeout Outbox record;
4. mark the task `completed` in the same local transaction.

### `finalize_payment`

Original inconsistency:

```text
Inventory confirmation succeeded
Local paid/outbox completion transaction failed
```

Repair:

1. call Inventory `confirm` again; the endpoint is idempotent;
2. update Order to `paid`;
3. complete any active timeout Outbox record;
4. mark the task `completed` in the same local transaction.

## Retry and unresolved behavior

Supported remote failures move the task to `failed`, clear the lease and schedule the next attempt with bounded exponential delay.

Default settings:

```text
RECONCILIATION_POLL_INTERVAL=2s
RECONCILIATION_RETRY_DELAY=5s
RECONCILIATION_MAX_RETRY_DELAY=5m
RECONCILIATION_LEASE_DURATION=30s
RECONCILIATION_CALL_TIMEOUT=10s
RECONCILIATION_BATCH_SIZE=10
```

An unsupported action, missing Order or incompatible Order state moves the task to `unresolved`. It remains stored with `last_error` and emits a structured error log. The Worker does not guess an alternative repair.

## Persistence after remote deadlines

Remote calls use a bounded task Context. Task failure/unresolved updates and successful local finalization use a separate short persistence Context. This ensures a remote timeout does not also prevent the Worker from clearing its lease and recording the retry state.

## Structured logs

Worker logs include:

```text
worker_id
task_id
order_id
action
attempt
outcome/error
```

Completed, failed and unresolved tasks remain distinguishable.

## Compose and scaling

Local runtime:

```bash
docker compose up -d --wait \
  --scale order-timeout-worker=2 \
  --scale order-reconciliation-worker=2
```

The reconciliation service intentionally has no fixed `container_name`, allowing multiple replicas.

## Boundaries

This implementation does not provide:

- a human approval UI;
- arbitrary workflow definitions;
- exactly-once remote effects;
- cross-service distributed transactions;
- automatic repair for unclassified states;
- deletion of failure history.

The trigger is a deliberate transactional bridge for the current Ordering service. A future workflow/outbox design may replace it, but removing it requires another mechanism that guarantees task creation and status transition cannot diverge.
