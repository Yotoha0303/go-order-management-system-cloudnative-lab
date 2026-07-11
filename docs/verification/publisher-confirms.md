# Publisher Confirm verification

## Scope

This verification applies to Issue #18 and the `phase/05-publisher-confirms` branch.

## Required checks

### Unit outcomes

`waitForPublisherConfirmation` must distinguish:

- positive broker acknowledgement;
- negative broker acknowledgement;
- closed confirmation channel;
- confirmation timeout with the original context deadline preserved.

### RabbitMQ integration

A real RabbitMQ channel must:

1. enter publisher confirm mode;
2. publish to a bound test queue;
3. receive a positive broker acknowledgement;
4. leave the routed message visible in the queue.

### Outbox state

Using a real MySQL table:

1. an event is claimed under a Worker lease;
2. a simulated unconfirmed publish returns an error;
3. the event becomes `failed`, increments `attempts` and clears its lease;
4. the event is never marked `published`;
5. a later Worker can reclaim it and mark it `published` only after a successful publisher result.

### Regression gate

The repository CI must continue to pass:

- lint;
- all Go tests;
- race detector;
- vet;
- all service builds;
- all migration validation;
- all image builds;
- four-database Compose startup;
- two timeout Worker replicas;
- Gateway readiness;
- the complete Order Saga smoke test.

## Delivery semantics

The implementation remains at least once. Publisher confirms prove broker acknowledgement before the database is marked `published`, but a process crash between ACK receipt and the database update can still cause a duplicate publish after lease recovery. The timeout consumer remains idempotent for this reason.
