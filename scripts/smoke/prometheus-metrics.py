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
        state = {}
        for target in active:
            labels = target.get("labels", {})
            job = labels.get("job")
            if job in EXPECTED_JOBS:
                state[job] = target.get("health")
        last_state = state
        if set(state) == EXPECTED_JOBS and all(value == "up" for value in state.values()):
            return
        time.sleep(2)
    raise RuntimeError(f"Prometheus targets did not become healthy: {last_state}")


def query(metric: str) -> list:
    encoded = urllib.parse.urlencode({"query": metric})
    payload = get_json("/api/v1/query?" + encoded)
    if payload.get("status") != "success":
        raise RuntimeError(f"query failed for {metric}: {payload}")
    return payload.get("data", {}).get("result", [])


def assert_metric(metric: str) -> None:
    result = query(metric)
    if not result:
        raise RuntimeError(f"metric query returned no series: {metric}")


def main() -> int:
    wait_ready()
    wait_targets()
    for metric in (
        "go_order_http_server_requests_total",
        "go_order_http_server_request_duration_seconds_count",
        "go_order_orders",
        "go_order_outbox_events",
        "go_order_worker_up",
    ):
        assert_metric(metric)
    print("Prometheus targets and bounded application metrics verified")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001 - command boundary
        print(f"Prometheus verification failed: {exc}", file=sys.stderr)
        raise
