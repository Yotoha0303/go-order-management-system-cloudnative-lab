#!/usr/bin/env python3

import json
import os
import sys
import time
import urllib.parse
import urllib.request

BASE_URL = os.environ.get("PROMETHEUS_URL", "http://127.0.0.1:9090").rstrip("/")
EXPECTED_JOBS = {
    "api-gateway",
    "identity-service",
    "catalog-service",
    "inventory-service",
    "order-service",
    "order-timeout-worker",
    "order-reconciliation-worker",
}
EXPECTED_MIN_TARGETS = {
    "api-gateway": 1,
    "identity-service": 1,
    "catalog-service": 1,
    "inventory-service": 1,
    "order-service": 1,
    "order-timeout-worker": 2,
    "order-reconciliation-worker": 2,
}
EXPECTED_RULE_GROUPS = {
    "go-order-service-recording",
    "go-order-availability",
    "go-order-http",
    "go-order-reliability",
    "go-order-infrastructure",
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


def get_json(path: str) -> dict:
    with urllib.request.urlopen(BASE_URL + path, timeout=5) as response:
        if response.status != 200:
            raise RuntimeError(f"unexpected HTTP status {response.status} for {path}")
        return json.load(response)


def wait_ready(timeout_seconds: int = 120) -> None:
    deadline = time.monotonic() + timeout_seconds
    last_error = None
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(BASE_URL + "/-/ready", timeout=3) as response:
                if response.status == 200:
                    return
        except Exception as exc:  # noqa: BLE001 - diagnostic polling boundary
            last_error = exc
        time.sleep(2)
    raise RuntimeError(f"Prometheus did not become ready: {last_error}")


def wait_targets(timeout_seconds: int = 120) -> None:
    deadline = time.monotonic() + timeout_seconds
    last_state = {}
    while time.monotonic() < deadline:
        payload = get_json("/api/v1/targets")
        active = payload.get("data", {}).get("activeTargets", [])
        state = {job: [] for job in EXPECTED_JOBS}
        for target in active:
            labels = target.get("labels", {})
            job = labels.get("job")
            if job in EXPECTED_JOBS:
                state[job].append(
                    {
                        "instance": labels.get("instance"),
                        "health": target.get("health"),
                    }
                )
        last_state = state
        if all(
            len(state[job]) >= EXPECTED_MIN_TARGETS[job]
            and all(target["health"] == "up" for target in state[job])
            for job in EXPECTED_JOBS
        ):
            return
        time.sleep(2)
    raise RuntimeError(f"Prometheus targets did not become healthy: {last_state}")


def wait_rules(timeout_seconds: int = 120) -> None:
    deadline = time.monotonic() + timeout_seconds
    last_state = {}
    while time.monotonic() < deadline:
        payload = get_json("/api/v1/rules")
        groups = payload.get("data", {}).get("groups", [])
        group_names = {group.get("name") for group in groups}
        rules = [rule for group in groups for rule in group.get("rules", [])]
        alerts = {rule.get("name") for rule in rules if rule.get("type") == "alerting"}
        unhealthy = [rule.get("name") for rule in rules if rule.get("health") not in (None, "ok")]
        last_state = {"groups": sorted(group_names), "alerts": sorted(alerts), "unhealthy": unhealthy}
        if EXPECTED_RULE_GROUPS.issubset(group_names) and EXPECTED_ALERTS.issubset(alerts) and not unhealthy:
            return
        time.sleep(2)
    raise RuntimeError(f"Prometheus rules did not become healthy: {last_state}")


def query(expression: str) -> list:
    encoded = urllib.parse.urlencode({"query": expression})
    payload = get_json("/api/v1/query?" + encoded)
    if payload.get("status") != "success":
        raise RuntimeError(f"query failed for {expression}: {payload}")
    return payload.get("data", {}).get("result", [])


def wait_metric(expression: str, timeout_seconds: int = 120) -> None:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        if query(expression):
            return
        time.sleep(2)
    raise RuntimeError(f"metric query returned no series: {expression}")


def main() -> int:
    wait_ready()
    wait_targets()
    wait_rules()
    for metric in (
        "go_order_http_server_requests_total",
        "go_order_http_server_request_duration_seconds_count",
        "go_order_orders",
        "go_order_outbox_events",
        "go_order_worker_up",
        "go_order_rabbitmq_session_up",
        "go_order_rabbitmq_management_up",
        "go_order_rabbitmq_queue_messages",
        "go_order_rabbitmq_queue_consumers",
        "go_order_rabbitmq_delivery_total",
        "service:http_requests:rate5m",
        "service:http_server_error_ratio:rate5m",
        "service:http_server_request_duration_seconds:p95",
        "service:http_client_attempts:rate5m",
        "worker:up:max",
    ):
        wait_metric(metric)
    print("Prometheus targets, replica discovery, rules, alerts and bounded application/infrastructure metrics verified")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001 - command boundary
        print(f"Prometheus verification failed: {exc}", file=sys.stderr)
        raise
