#!/usr/bin/env python3

import json
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
APPLICATION_DASHBOARD_PATH = ROOT / "deploy/grafana/dashboards/go-order-overview.json"
INFRASTRUCTURE_DASHBOARD_PATH = ROOT / "deploy/grafana/dashboards/go-order-infrastructure.json"
DATASOURCE_PATH = ROOT / "deploy/grafana/provisioning/datasources/prometheus.yml"
PROVIDER_PATH = ROOT / "deploy/grafana/provisioning/dashboards/dashboards.yml"
PROMETHEUS_CONFIG_PATH = ROOT / "deploy/prometheus/prometheus.yml"
RECORDING_RULES_PATH = ROOT / "deploy/prometheus/rules/recording-rules.yml"
ALERT_RULES_PATH = ROOT / "deploy/prometheus/rules/alert-rules.yml"
KUBE_STATE_METRICS_PATH = ROOT / "deploy/kubernetes/observability/kube-state-metrics.yaml"

EXPECTED_RECORDING_RULES = {
    "service:http_requests:rate5m",
    "service:http_server_errors:rate5m",
    "service:http_server_error_ratio:rate5m",
    "service:http_server_request_duration_seconds:p95",
    "service:http_client_attempts:rate5m",
    "worker:up:max",
}

EXPECTED_ALERTS = {
    "GoOrderTargetDown",
    "GoOrderWorkerDown",
    "GoOrderElevatedHTTP5xxRatio",
    "GoOrderHighP95Latency",
    "GoOrderOutboxOverdue",
    "GoOrderOutboxActionableAgeHigh",
    "GoOrderReconciliationBacklog",
    "GoOrderSagaStuck",
    "GoOrderMetricsCollectionFailing",
    "GoOrderRabbitMQSessionUnavailable",
    "GoOrderRabbitMQManagementCollectorDown",
    "GoOrderRabbitMQQueueBacklogHigh",
    "GoOrderMigrationJobFailed",
}

REQUIRED_APPLICATION_PANELS = {
    "Application targets down",
    "HTTP request rate by service",
    "HTTP 5xx ratio by service",
    "HTTP p95 latency by service",
    "Orders by status",
    "Outbox events by status",
    "Reconciliation required",
    "Stuck transient Sagas",
    "RabbitMQ publisher-confirm outcomes",
    "Worker availability",
}

REQUIRED_INFRASTRUCTURE_PANELS = {
    "RabbitMQ Application Session",
    "RabbitMQ Management Collector",
    "Ready Queue Messages",
    "RabbitMQ Queue Consumers",
    "Timeout Delivery Outcomes",
    "Terminally Failed Migration Jobs",
}

REQUIRED_APPLICATION_TERMS = {
    "service:http_requests:rate5m",
    "service:http_server_error_ratio:rate5m",
    "service:http_server_request_duration_seconds:p95",
    "go_order_orders",
    "go_order_outbox_events",
    "go_order_reconciliation_required",
    "go_order_saga_stuck_transient",
    "go_order_rabbitmq_publish_total",
    "go_order_worker_up",
}

REQUIRED_INFRASTRUCTURE_TERMS = {
    "go_order_rabbitmq_session_up",
    "go_order_rabbitmq_management_up",
    "go_order_rabbitmq_queue_messages",
    "go_order_rabbitmq_queue_consumers",
    "go_order_rabbitmq_delivery_total",
    "kube_job_failed",
}

FORBIDDEN_LABEL_PATTERN = re.compile(
    r"\b(request_id|trace_id|user_id|order_id|reservation_id|event_id|worker_id|delivery_tag|message_id|queue_name)\s*=",
    re.IGNORECASE,
)


def read_text(path: pathlib.Path) -> str:
    return path.read_text(encoding="utf-8")


def require(condition: bool, message: str) -> None:
    if not condition:
        raise RuntimeError(message)


def collect_expressions(dashboard: dict) -> list[str]:
    expressions: list[str] = []
    for panel in dashboard.get("panels", []):
        for target in panel.get("targets", []):
            expression = target.get("expr")
            if isinstance(expression, str) and expression.strip():
                expressions.append(expression.strip())
    return expressions


def validate_dashboard(
    path: pathlib.Path,
    expected_uid: str,
    expected_title: str,
    required_panels: set[str],
    required_terms: set[str],
    minimum_panels: int,
) -> None:
    dashboard = json.loads(read_text(path))
    require(dashboard.get("uid") == expected_uid, f"unexpected dashboard uid in {path.name}")
    require(dashboard.get("title") == expected_title, f"unexpected dashboard title in {path.name}")

    panels = dashboard.get("panels", [])
    require(len(panels) >= minimum_panels, f"expected at least {minimum_panels} panels in {path.name}, got {len(panels)}")
    panel_ids = [panel.get("id") for panel in panels]
    require(len(panel_ids) == len(set(panel_ids)), f"dashboard panel ids must be unique in {path.name}")
    panel_titles = {panel.get("title") for panel in panels}
    missing_panels = sorted(required_panels - panel_titles)
    require(not missing_panels, f"dashboard {path.name} is missing required panels: {missing_panels}")

    expressions = collect_expressions(dashboard)
    joined = "\n".join(expressions)
    missing_terms = sorted(term for term in required_terms if term not in joined)
    require(not missing_terms, f"dashboard {path.name} is missing metric coverage: {missing_terms}")
    require(not FORBIDDEN_LABEL_PATTERN.search(joined), f"dashboard {path.name} contains a forbidden high-cardinality label")


def validate_provisioning() -> None:
    datasource = read_text(DATASOURCE_PATH)
    require("uid: prometheus" in datasource, "Grafana datasource uid must be prometheus")
    require("url: http://prometheus:9090" in datasource, "Grafana datasource URL is incorrect")
    require("isDefault: true" in datasource, "Prometheus datasource must be default")

    provider = read_text(PROVIDER_PATH)
    require("path: /var/lib/grafana/dashboards" in provider, "Grafana dashboard provider path is incorrect")
    require("allowUiUpdates: false" in provider, "provisioned dashboards must remain file-managed")


def validate_prometheus_scrape_contract() -> None:
    config = read_text(PROMETHEUS_CONFIG_PATH)
    for worker, port in (
        ("order-timeout-worker", 9091),
        ("order-reconciliation-worker", 9092),
    ):
        require(f"job_name: {worker}" in config, f"missing Prometheus job for {worker}")
        require(f'names: ["{worker}"]' in config, f"{worker} must use DNS service discovery")
        require(f"port: {port}" in config, f"{worker} scrape port is incorrect")
        require(
            f'targets: ["{worker}:{port}"]' not in config,
            f"{worker} must not collapse scaled replicas into one static target",
        )


def validate_rules() -> None:
    recording = read_text(RECORDING_RULES_PATH)
    records = set(re.findall(r"^\s*- record:\s*(\S+)\s*$", recording, re.MULTILINE))
    require(records == EXPECTED_RECORDING_RULES, f"recording rule set drifted: {sorted(records)}")

    alerts_text = read_text(ALERT_RULES_PATH)
    alerts = set(re.findall(r"^\s*- alert:\s*(\S+)\s*$", alerts_text, re.MULTILINE))
    require(alerts == EXPECTED_ALERTS, f"alert rule set drifted: {sorted(alerts)}")
    for_windows = re.findall(r"^\s*for:\s*\S+\s*$", alerts_text, re.MULTILINE)
    require(len(for_windows) == len(EXPECTED_ALERTS), "every alert must define an explicit for window")
    require(not FORBIDDEN_LABEL_PATTERN.search(alerts_text), "alert rules contain a forbidden high-cardinality label")
    require(
        'go_order_rabbitmq_queue_messages{queue_role="cancel",state="ready"}' in alerts_text,
        "RabbitMQ backlog alert must only use the actionable cancellation queue",
    )
    require(
        'kube_job_failed{condition="true"' in alerts_text,
        "migration alert must use the terminal Kubernetes Job Failed condition",
    )
    require(
        "kube_job_status_failed" not in alerts_text,
        "migration alert must not fire from transient failed Pod counts",
    )


def validate_kube_state_metrics_contract() -> None:
    contract = read_text(KUBE_STATE_METRICS_PATH)
    require("kube-state-metrics:v2.14.0" in contract, "kube-state-metrics image must be explicitly versioned")
    require("--resources=jobs" in contract, "kube-state-metrics must be restricted to Jobs")
    require("--namespaces=go-order-system" in contract, "kube-state-metrics must be namespace-scoped")
    require('prometheus.io/scrape: "true"' in contract, "kube-state-metrics scrape annotation is missing")
    require("resources: [\"jobs\"]" in contract, "RBAC must only grant Job observation")


def main() -> int:
    validate_dashboard(
        APPLICATION_DASHBOARD_PATH,
        "go-order-overview",
        "Go Order Management Overview",
        REQUIRED_APPLICATION_PANELS,
        REQUIRED_APPLICATION_TERMS,
        12,
    )
    validate_dashboard(
        INFRASTRUCTURE_DASHBOARD_PATH,
        "go-order-infrastructure",
        "Go Order Infrastructure Signals",
        REQUIRED_INFRASTRUCTURE_PANELS,
        REQUIRED_INFRASTRUCTURE_TERMS,
        6,
    )
    validate_provisioning()
    validate_prometheus_scrape_contract()
    validate_rules()
    validate_kube_state_metrics_contract()
    print("Grafana dashboards, provisioning, Prometheus rules and infrastructure contracts verified")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001 - command boundary
        print(f"Observability contract verification failed: {exc}", file=sys.stderr)
        raise
