# Reconciliation worker verification

## Migration validation

Ordering migration `00003_reconciliation_tasks.sql` must pass Goose validation and apply during the Compose `ordering-migrate` job.

The migration creates:

```text
order_reconciliation_tasks
trg_orders_v2_create_reconciliation_task
```

The down migration drops the trigger before dropping the task table.

## Transactional trigger tests

Real MySQL tests verify that transitioning an Order to `reconciliation_required` creates exactly one explicit task:

| Previous state | Expected action | Initial task status |
| --- | --- | --- |
| `reserving` | `release_inventory_and_fail` | `pending` |
| `cancelling` | `finalize_cancel` | `pending` |
| `paying` | `finalize_payment` | `pending` |
| unsupported state | `unsupported_from_<state>` | `unresolved` |

A transaction rollback test proves that rolling back the Order status transition also removes the task created by the trigger.

## Lease tests

Two Worker instances claim the same task table with different Worker IDs.

Required assertions:

- first Worker claims only its configured batch;
- second Worker receives only unleased tasks;
- no task ID appears in both claims;
- each claim increments `attempts`;
- expired leases can be reclaimed by a third Worker;
- reclaimed tasks record the new lease owner.

## Supported action tests

Each action is tested against a real Ordering database and a controlled Inventory client.

### Release and fail

Verify:

- Inventory release is called with the original reservation ID;
- Order moves from `reconciliation_required` to `failed`;
- failure history remains stored;
- task becomes `completed` and clears its lease.

### Finalize cancel

Verify:

- Inventory release is repeated safely;
- Order becomes `cancelled`;
- active timeout Outbox becomes `completed`;
- task completion occurs in the same local transaction.

### Finalize payment

Verify:

- Inventory confirm is repeated safely;
- Order becomes `paid`;
- active timeout Outbox becomes `completed`;
- task completion occurs in the same local transaction.

## Failure and unresolved tests

A temporary Inventory failure must:

- leave the Order in `reconciliation_required`;
- move the task to `failed`;
- increment attempts;
- clear the lease;
- preserve `last_error`;
- schedule `next_attempt_at` using the configured retry delay.

An unknown action must:

- make no Inventory call;
- move the task to `unresolved`;
- clear the lease;
- preserve an explanatory error;
- remain visible rather than being marked successful.

## Context-boundary test intent

Remote Inventory calls use the task call timeout, while successful local completion and failure persistence use an independent short Context. Code review and timeout-path tests must ensure that an expired remote Context cannot strand the task under its lease without a recorded retry state.

## Runtime replica gate

CI must build and start:

```text
order-timeout-worker x 2
order-reconciliation-worker x 2
```

Both reconciliation replicas share the Ordering database and must remain running while Gateway readiness and the complete Order Saga smoke test execute.

## Full regression gate

The PR is not complete until all existing checks pass:

```text
golangci-lint
go test ./...
go test -race ./...
go vet ./...
go build ./...
legacy migration validation
four service migration validations
seven service binary builds
all Compose images
four-database startup
two timeout Worker replicas
two reconciliation Worker replicas
Gateway readiness
complete Order Saga smoke test
```

## Non-claims

Passing these tests does not establish exactly-once repair. The design relies on idempotent Inventory operations and local task leases to provide recoverable at-least-once processing.
