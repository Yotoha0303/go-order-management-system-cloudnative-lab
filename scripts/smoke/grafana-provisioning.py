#!/usr/bin/env python3

import base64
import json
import os
import sys
import time
import urllib.error
import urllib.request

BASE_URL = os.environ.get("GRAFANA_URL", "http://127.0.0.1:3000").rstrip("/")
ADMIN_USER = os.environ.get("GRAFANA_ADMIN_USER", "admin")
ADMIN_PASSWORD = os.environ.get("GRAFANA_ADMIN_PASSWORD", "admin")
EXPECTED_PANEL_TITLES = {
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


def request_json(path: str, authenticated: bool = True) -> dict:
    request = urllib.request.Request(BASE_URL + path)
    if authenticated:
        token = base64.b64encode(f"{ADMIN_USER}:{ADMIN_PASSWORD}".encode()).decode()
        request.add_header("Authorization", f"Basic {token}")
    with urllib.request.urlopen(request, timeout=5) as response:
        if response.status != 200:
            raise RuntimeError(f"unexpected HTTP status {response.status} for {path}")
        return json.load(response)


def wait_ready(timeout_seconds: int = 180) -> None:
    deadline = time.monotonic() + timeout_seconds
    last_error = None
    while time.monotonic() < deadline:
        try:
            payload = request_json("/api/health", authenticated=False)
            if payload.get("database") == "ok":
                return
        except Exception as exc:  # noqa: BLE001 - diagnostic polling boundary
            last_error = exc
        time.sleep(2)
    raise RuntimeError(f"Grafana did not become ready: {last_error}")


def wait_provisioned(timeout_seconds: int = 120) -> None:
    deadline = time.monotonic() + timeout_seconds
    last_error = None
    while time.monotonic() < deadline:
        try:
            datasource = request_json("/api/datasources/uid/prometheus")
            dashboard_payload = request_json("/api/dashboards/uid/go-order-overview")
            dashboard = dashboard_payload.get("dashboard", {})
            meta = dashboard_payload.get("meta", {})
            titles = {panel.get("title") for panel in dashboard.get("panels", [])}
            if (
                datasource.get("type") == "prometheus"
                and datasource.get("url") == "http://prometheus:9090"
                and datasource.get("isDefault") is True
                and dashboard.get("title") == "Go Order Management Overview"
                and dashboard.get("uid") == "go-order-overview"
                and meta.get("provisioned") is True
                and EXPECTED_PANEL_TITLES.issubset(titles)
            ):
                return
        except (urllib.error.URLError, RuntimeError, KeyError, ValueError) as exc:
            last_error = exc
        time.sleep(2)
    raise RuntimeError(f"Grafana datasource or dashboard was not provisioned: {last_error}")


def main() -> int:
    wait_ready()
    wait_provisioned()
    print("Grafana Prometheus datasource and Go Order dashboard verified")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001 - command boundary
        print(f"Grafana verification failed: {exc}", file=sys.stderr)
        raise
