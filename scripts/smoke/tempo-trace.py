#!/usr/bin/env python3

import json
import os
import re
import sys
import time
import urllib.error
import urllib.request

BASE_URL = os.environ.get("TEMPO_URL", "http://127.0.0.1:3200").rstrip("/")
TRACE_ID = os.environ.get("TRACE_ID", "").strip().lower()
EXPECTED_SERVICES = {
    "api-gateway",
    "identity-service",
    "catalog-service",
    "inventory-service",
    "order-service",
}
FORBIDDEN_SPAN_NAME = re.compile(r"(?:/\d+|[0-9a-f]{8}-[0-9a-f-]{27,})", re.IGNORECASE)


def require(condition: bool, message: str) -> None:
    if not condition:
        raise RuntimeError(message)


def get_json(path: str) -> dict:
    request = urllib.request.Request(BASE_URL + path)
    request.add_header("Accept", "application/json")
    with urllib.request.urlopen(request, timeout=5) as response:
        require(response.status == 200, f"unexpected Tempo status {response.status}")
        return json.load(response)


def wait_ready(timeout_seconds: int = 180) -> None:
    deadline = time.monotonic() + timeout_seconds
    last_error = None
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(BASE_URL + "/ready", timeout=3) as response:
                if response.status == 200:
                    return
        except Exception as exc:  # noqa: BLE001 - polling boundary
            last_error = exc
        time.sleep(2)
    raise RuntimeError(f"Tempo did not become ready: {last_error}")


def find_trace(timeout_seconds: int = 180) -> dict:
    deadline = time.monotonic() + timeout_seconds
    last_error = None
    while time.monotonic() < deadline:
        try:
            return get_json(f"/api/traces/{TRACE_ID}")
        except urllib.error.HTTPError as exc:
            if exc.code != 404:
                last_error = exc
        except Exception as exc:  # noqa: BLE001 - polling boundary
            last_error = exc
        time.sleep(2)
    raise RuntimeError(f"trace {TRACE_ID} was not queryable from Tempo: {last_error}")


def walk(value):
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from walk(child)
    elif isinstance(value, list):
        for child in value:
            yield from walk(child)


def string_value(value):
    if isinstance(value, dict):
        for key in ("stringValue", "string_value"):
            if key in value and isinstance(value[key], str):
                return value[key]
    return None


def extract_services(payload: dict) -> set[str]:
    services = set()
    for item in walk(payload):
        if item.get("key") == "service.name":
            service = string_value(item.get("value"))
            if service:
                services.add(service)
    return services


def extract_span_names(payload: dict) -> list[str]:
    names = []
    for item in walk(payload):
        if ("spanId" in item or "span_id" in item) and isinstance(item.get("name"), str):
            names.append(item["name"])
    return names


def main() -> int:
    require(re.fullmatch(r"[0-9a-f]{32}", TRACE_ID) is not None, "TRACE_ID must be 32 lowercase hex characters")
    wait_ready()
    payload = find_trace()
    services = extract_services(payload)
    missing = sorted(EXPECTED_SERVICES - services)
    require(not missing, f"trace is missing expected services: {missing}; observed={sorted(services)}")

    span_names = extract_span_names(payload)
    require(len(span_names) >= 10, f"expected at least 10 spans, got {len(span_names)}")
    require(any(name == "order.create_saga" for name in span_names), "Order Saga span is missing")
    leaking = sorted({name for name in span_names if FORBIDDEN_SPAN_NAME.search(name)})
    require(not leaking, f"span names contain resource identifiers: {leaking}")
    require(any(name == "POST api_orders" for name in span_names), "bounded Order HTTP span is missing")

    print(
        "Tempo cross-service trace verified: "
        f"trace_id={TRACE_ID} services={','.join(sorted(services))} spans={len(span_names)}"
    )
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001 - command boundary
        print(f"Tempo trace verification failed: {exc}", file=sys.stderr)
        raise
