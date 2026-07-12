#!/usr/bin/env python3

import json
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
DASHBOARD_PATH = ROOT / "deploy/grafana/dashboards/go-order-overview.json"
DATASOURCE_PATH = ROOT / "deploy/grafana/provisioning/datasources/prometheus.yml"
PROVIDER_PATH = ROOT / "deploy/grafana/provisioning/dashboards/dashboards.yml"
RECORDING_RULES_PATH = ROOT / "deploy/prometheus/rules/recording-rules.yml"
ALERT_RULES_PATH = ROOT / "deploy/prometheus/rules/alert-rules.yml"

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
}

REQUIRED_DASHBOARD_TERMS = {
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

FORBIDDEN_LABEL_PATTERN = re.compile(
    r"\b(request_id|trace_id|user_id|order_id|reservation_id|event_id|worker_id)\s*=",
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


def validate_dashboard() -> None:
    dashboard = json.loads(read_text(DASHBOARD_PATH))
    require(dashboard.get("uid") == "go-order-overview", "unexpected dashboard uid")
    require(dashboard.get("title") == "Go Order Management Overview", "unexpected dashboard title")

    panels = dashboard.get("panels", [])
    require(len(panels) >= 12, f"expected at least 12 dashboard panels, got {len(panels)}")
    panel_ids = [panel.get("id") for panel in panels]
    require(len(panel_ids) == len(set(panel_ids)), "dashboard panel ids must be unique")

    expressions = collect_expressions(dashboard)
    joined = "\n".join(expressions)
    missing_terms = sorted(term for term in REQUIRED_DASHBOARD_TERMS if term not in joined)
    require(not missing_terms, f"dashboard is missing metric coverage: {missing_terms}")
    require(not FORBIDDEN_LABEL_PATTERN.search(joined), "dashboard query contains a forbidden high-cardinality label")

    templating = dashboard.get("templating", {}).get("list", [])
    require(any(item.get("name") == "service" for item in templating), "service dashboard variable is missing")


def validate_provisioning() -> None:
    datasource = read_text(DATASOURCE_PATH)
    require("uid: prometheus" in datasource, "Grafana datasource uid must be prometheus")
    require("url: http://prometheus:9090" in datasource, "Grafana datasource URL is incorrect")
    require("isDefault: true" in datasource, "Prometheus datasource must be default")

    provider = read_text(PROVIDER_PATH)
    require("path: /var/lib/grafana/dashboards" in provider, "Grafana dashboard provider path is incorrect")
    require("allowUiUpdates: false" in provider, "provisioned dashboard must remain file-managed")


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


def main() -> int:
    validate_dashboard()
    validate_provisioning()
    validate_rules()
    print("Grafana dashboard, provisioning and Prometheus rule contracts verified")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001 - command boundary
        print(f"Observability contract verification failed: {exc}", file=sys.stderr)
        raise
