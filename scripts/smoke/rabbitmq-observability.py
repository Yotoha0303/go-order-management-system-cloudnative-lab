#!/usr/bin/env python3

import argparse
import base64
import json
import os
import sys
import time
import urllib.parse
import urllib.request

PROMETHEUS_URL = os.environ.get("PROMETHEUS_URL", "http://127.0.0.1:9090").rstrip("/")
MANAGEMENT_URL = os.environ.get("RABBITMQ_MANAGEMENT_URL", "http://127.0.0.1:15672").rstrip("/")
RABBITMQ_USER = os.environ.get("RABBITMQ_USER", "order_app")
RABBITMQ_PASSWORD = os.environ.get("RABBITMQ_PASSWORD", "order_dev_password")


def prometheus_query(expression: str) -> list:
    encoded = urllib.parse.urlencode({"query": expression})
    with urllib.request.urlopen(PROMETHEUS_URL + "/api/v1/query?" + encoded, timeout=5) as response:
        payload = json.load(response)
    if payload.get("status") != "success":
        raise RuntimeError(f"Prometheus query failed: {expression}: {payload}")
    return payload.get("data", {}).get("result", [])


def scalar_max(expression: str) -> float:
    results = prometheus_query(expression)
    values = [float(item.get("value", [0, "0"])[1]) for item in results]
    return max(values, default=0.0)


def wait_condition(expression: str, timeout_seconds: int = 120) -> None:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        if prometheus_query(expression):
            return
        time.sleep(2)
    raise RuntimeError(f"condition did not become true: {expression}")


def publish_invalid_timeout_message() -> None:
    endpoint = MANAGEMENT_URL + "/api/exchanges/%2F/order.timeout.v2/publish"
    payload = json.dumps(
        {
            "properties": {"content_type": "application/json", "delivery_mode": 2},
            "routing_key": "cancel",
            "payload": '{"order_id":0}',
            "payload_encoding": "string",
        }
    ).encode()
    request = urllib.request.Request(endpoint, data=payload, method="POST")
    request.add_header("Content-Type", "application/json")
    token = base64.b64encode(f"{RABBITMQ_USER}:{RABBITMQ_PASSWORD}".encode()).decode()
    request.add_header("Authorization", f"Basic {token}")
    with urllib.request.urlopen(request, timeout=5) as response:
        result = json.load(response)
    if result.get("routed") is not True:
        raise RuntimeError(f"invalid timeout fixture was not routed: {result}")


def verify_delivery_metrics() -> None:
    wait_condition("max(go_order_rabbitmq_session_up) == 1")
    wait_condition("max(go_order_rabbitmq_management_up) == 1")
    wait_condition('go_order_rabbitmq_queue_messages{queue_role="delay",state="total"}')
    wait_condition('go_order_rabbitmq_queue_consumers{queue_role="cancel"}')
    wait_condition('go_order_rabbitmq_delivery_total{outcome="acknowledged"} > 0')

    processing_before = scalar_max('go_order_rabbitmq_delivery_total{outcome="processing_failure"}')
    rejected_before = scalar_max('go_order_rabbitmq_delivery_total{outcome="rejected"}')
    publish_invalid_timeout_message()
    wait_condition(
        f'go_order_rabbitmq_delivery_total{{outcome="processing_failure"}} > {processing_before}',
        timeout_seconds=60,
    )
    wait_condition(
        f'go_order_rabbitmq_delivery_total{{outcome="rejected"}} > {rejected_before}',
        timeout_seconds=60,
    )
    print("RabbitMQ queue, session, acknowledged and controlled failure metrics verified")


def wait_session(expected: int) -> None:
    if expected not in (0, 1):
        raise ValueError("session expectation must be 0 or 1")
    wait_condition(f"max(go_order_rabbitmq_session_up) == {expected}", timeout_seconds=120)
    print(f"RabbitMQ application session gauge reached {expected}")


def main() -> int:
    parser = argparse.ArgumentParser()
    subcommands = parser.add_subparsers(dest="command", required=True)
    subcommands.add_parser("verify-deliveries")
    session = subcommands.add_parser("wait-session")
    session.add_argument("value", type=int, choices=(0, 1))
    args = parser.parse_args()

    if args.command == "verify-deliveries":
        verify_delivery_metrics()
    else:
        wait_session(args.value)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001 - command boundary
        print(f"RabbitMQ observability verification failed: {exc}", file=sys.stderr)
        raise
