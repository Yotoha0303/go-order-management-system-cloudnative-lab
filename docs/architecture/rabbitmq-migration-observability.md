# RabbitMQ and migration observability

## Scope

Phase 7.4 closes the two observability items intentionally left open after Prometheus, Grafana and OpenTelemetry delivery:

- application-owned RabbitMQ consumer/session and bounded queue signals;
- a reproducible Kubernetes migration Job failure signal and alert contract.

The change is additive. It does not change business APIs, database schemas, Saga states, RabbitMQ exchanges/queues, message payloads, acknowledgement policy or Kubernetes migration execution.

## Timeout Worker delivery metrics

The Timeout Worker records:

```text
go_order_rabbitmq_delivery_total{outcome}
```

Bounded outcomes:

```text
received
acknowledged
requeued
rejected
processing_failure
settlement_error
```

Meaning:

- `received`: a delivery was read from the cancel queue;
- `acknowledged`: business processing and the final Outbox update succeeded, then `Ack` succeeded;
- `requeued`: processing or persistence failed and `Nack(requeue=true)` succeeded;
- `rejected`: an invalid payload was rejected without requeue;
- `processing_failure`: decoding, Order cancellation or local persistence failed;
- `settlement_error`: Ack/Nack/Reject itself returned an error.

One message may increment `received`, one processing result and one settlement result. This is intentional; the metric describes stages rather than mutually exclusive final states.

No delivery tag, message ID, Order ID, event ID, Worker ID or raw error text is used as a label.

## RabbitMQ session signal

```text
go_order_rabbitmq_session_up{role="timeout_worker"}
```

The gauge is set to `1` only after both publisher/consumer channels, topology declaration, QoS and the consumer registration succeed. It returns to `0` whenever the session exits because of context cancellation, connection loss or Channel closure.

Each Worker metrics target exports its own sample; PromQL uses `max by (role)` when determining whether any replica still has a live session.

## Management API Collector

The Timeout Worker metrics listener includes a scrape-time Collector using:

```text
RABBITMQ_MANAGEMENT_URL
RABBITMQ_URL
RABBITMQ_MANAGEMENT_METRICS_TIMEOUT
```

`RABBITMQ_URL` is the single source of truth for the AMQP username, password and virtual host. The Collector parses those values from the same URL used by the Worker, so changing only `RABBITMQ_URL` cannot leave message processing healthy while management scrapes use stale default credentials or query the `/` virtual host by mistake. The virtual-host path is percent-encoded before calling the RabbitMQ Management API.

The Collector uses an independent two-second default HTTP timeout and queries only the two fixed timeout queues. It emits bounded roles instead of real queue names:

```text
go_order_rabbitmq_management_up
go_order_rabbitmq_queue_messages{queue_role="delay|cancel",state="total|ready|unacknowledged"}
go_order_rabbitmq_queue_consumers{queue_role="delay|cancel"}
```

A collection failure sets `management_up=0` and increments the existing `go_order_metrics_collection_errors_total{collector="rabbitmq_management"}` counter through the registry. It does not fail Worker processing or application readiness.

The RabbitMQ account used by the Timeout Worker must also have Management API monitoring permission for its configured virtual host. The current contract deliberately avoids a second credential source; a separately configurable read-only monitoring identity remains a production-hardening option rather than an implemented claim.

RabbitMQ management port `15672` remains internal in Kubernetes. Compose retains the existing optional host mapping for local administration and CI fixtures; this phase does not add a new public exposure.

## Kubernetes migration Job signal

The optional directory:

```text
deploy/kubernetes/observability
```

contains an explicitly versioned `kube-state-metrics:v2.14.0` contract restricted to:

```text
resource: Jobs
namespace: go-order-system
```

Its Service is ClusterIP and its Pod carries standard Prometheus scrape annotations. The minimal ClusterRole grants only `list` and `watch` on batch Jobs.

The migration alert consumes the terminal Job condition signal:

```text
kube_job_failed{condition="true",namespace="go-order-system",job_name=~".*-migrate"}
```

It intentionally does not alert from `kube_job_status_failed`, because that metric can count transient failed Pods even when the Job retries and later completes successfully.

This optional contract is not included in the default local/test application overlays and does not claim cluster-wide monitoring parity. A target cluster must deploy a Prometheus instance or compatible collector capable of scraping the Service.

## Dashboard and alerts

Grafana automatically provisions:

```text
UID:   go-order-infrastructure
Title: Go Order Infrastructure Signals
```

Panels cover RabbitMQ application session, management collection, ready messages, consumer counts, delivery outcomes and terminally failed migration Jobs.

New alerts:

```text
GoOrderRabbitMQSessionUnavailable
GoOrderRabbitMQManagementCollectorDown
GoOrderRabbitMQQueueBacklogHigh
GoOrderMigrationJobFailed
```

The RabbitMQ backlog alert applies only to the actionable cancellation queue. Messages in the delay queue represent scheduled unpaid-order timeouts and are not treated as processing backlog.

All alerts define explicit `for` windows. The thresholds are local/test defaults, not production SLOs.

## Verification

Static and deterministic checks verify:

- only bounded labels are used;
- the infrastructure Dashboard contains all required signals;
- all 13 alerts have explicit `for` windows;
- RabbitMQ session loss fires and a healthy session does not;
- only the cancellation queue can trigger the RabbitMQ backlog alert;
- a terminal migration Job failure fires after its window;
- a transient failed Pod followed by successful Job completion does not fire the migration alert;
- both Timeout Worker and Reconciliation Worker replicas are discovered and healthy before metric assertions run;
- the kube-state-metrics image, resource filter, namespace and RBAC are explicit;
- the optional Kubernetes observability Kustomization renders.

The Observability Stack workflow also:

1. executes the complete Order Saga and observes an acknowledged timeout delivery;
2. publishes a controlled invalid payload through the RabbitMQ management API;
3. verifies `processing_failure` and `rejected` increase;
4. verifies queue depth and consumer-count metrics;
5. stops RabbitMQ and waits for the application session gauge to become `0`;
6. restarts RabbitMQ and waits for the gauge to return to `1` without restarting the Worker.

## Failure and rollback boundary

Reverting the consumer counters, session gauge, management Collector, optional kube-state-metrics contract, Dashboard and alert rules removes only observability. It does not alter RabbitMQ delivery semantics, queue topology, Order state transitions, Outbox persistence or Kubernetes migration Jobs.
