# Grafana and alert validation

## Static contracts

```bash
python3 scripts/verify/observability-contracts.py
```

This checks:

- dashboard JSON and stable UID;
- required panel coverage;
- unique panel IDs;
- Prometheus datasource and dashboard provider paths;
- exact recording-rule and alert-rule names;
- explicit `for` windows on every alert;
- absence of forbidden high-cardinality labels.

## Prometheus syntax and fixtures

```bash
docker run --rm \
  -v "${PWD}/deploy/prometheus:/etc/prometheus:ro" \
  --entrypoint /bin/promtool \
  prom/prometheus:v3.5.0 \
  check config /etc/prometheus/prometheus.yml

docker run --rm \
  -v "${PWD}/deploy/prometheus:/etc/prometheus:ro" \
  -w /etc/prometheus/tests \
  --entrypoint /bin/promtool \
  prom/prometheus:v3.5.0 \
  test rules /etc/prometheus/tests/rules.test.yml
```

The fixtures cover:

- `GoOrderTargetDown` firing after its `for` window;
- the same alert remaining inactive for a healthy target;
- `GoOrderOutboxOverdue` firing after its `for` window.

## Runtime smoke

```bash
docker compose -f compose.yml -f compose.observability.yml up -d --build --wait \
  --scale order-timeout-worker=2 \
  --scale order-reconciliation-worker=2

sh scripts/smoke/microservices-saga.sh
python3 scripts/smoke/prometheus-metrics.py
python3 scripts/smoke/grafana-provisioning.py
```

The runtime checks confirm:

- all seven application scrape targets are `up`;
- all recording and alert rule groups load with healthy status;
- recording-rule series are queryable;
- Grafana reports a healthy database;
- the provisioned datasource uses UID `prometheus` and the Compose Prometheus URL;
- Dashboard UID `go-order-overview` exists and is file provisioned;
- required panels are present without manual import.

## Failure diagnostics

The Observability Stack workflow records:

- merged Compose status;
- complete application, Prometheus and Grafana logs;
- Prometheus targets;
- Prometheus rules;
- recording-rule query output;
- Grafana health;
- Grafana datasource metadata;
- Grafana dashboard payload.

## Validation boundary

Passing this validation proves local/test Compose provisioning, rule loading and application-level dashboards. It does not prove Alertmanager delivery, production threshold quality, Kubernetes monitoring deployment or distributed tracing.
