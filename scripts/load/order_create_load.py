#!/usr/bin/env python3

from __future__ import annotations

import argparse
import concurrent.futures
import dataclasses
import datetime as dt
import json
import math
import os
import pathlib
import threading
import time
import urllib.error
import urllib.request
import uuid
from collections import Counter
from typing import Any

MAX_CONCURRENCY = 32
MAX_STAGE_SECONDS = 15.0
MAX_WARMUP_SECONDS = 10.0
MAX_REQUESTS = 3000


@dataclasses.dataclass(frozen=True)
class Sample:
    stage: str
    sequence: int
    started_at: str
    duration_ms: float
    status: int | None
    outcome: str
    error: str | None

    def as_dict(self) -> dict[str, Any]:
        return dataclasses.asdict(self)


class Allocator:
    def __init__(self, deadline: float, limit: int, sequence_offset: int = 0) -> None:
        self.deadline = deadline
        self.limit = limit
        self.sequence = sequence_offset
        self.issued = 0
        self.lock = threading.Lock()

    def next(self) -> int | None:
        with self.lock:
            if time.monotonic() >= self.deadline or self.issued >= self.limit:
                return None
            self.issued += 1
            self.sequence += 1
            return self.sequence


def percentile(values: list[float], quantile: float) -> float:
    if not values:
        return 0.0
    if not 0.0 <= quantile <= 1.0:
        raise ValueError("quantile must be between zero and one")
    ordered = sorted(values)
    if len(ordered) == 1:
        return ordered[0]
    position = quantile * (len(ordered) - 1)
    lower = math.floor(position)
    upper = math.ceil(position)
    if lower == upper:
        return ordered[lower]
    weight = position - lower
    return ordered[lower] * (1.0 - weight) + ordered[upper] * weight


def validate_levels(raw: str) -> tuple[int, ...]:
    try:
        levels = tuple(int(value.strip()) for value in raw.split(",") if value.strip())
    except ValueError as exc:
        raise ValueError("concurrency levels must be comma-separated integers") from exc
    if not levels:
        raise ValueError("at least one concurrency level is required")
    if any(level < 1 or level > MAX_CONCURRENCY for level in levels):
        raise ValueError(f"concurrency levels must be between 1 and {MAX_CONCURRENCY}")
    if tuple(sorted(set(levels))) != levels:
        raise ValueError("concurrency levels must be strictly increasing and unique")
    return levels


def api_json(
    *,
    base_url: str,
    path: str,
    method: str,
    payload: dict[str, Any],
    timeout_seconds: float,
    bearer_token: str | None = None,
) -> dict[str, Any]:
    headers = {"Content-Type": "application/json"}
    if bearer_token:
        headers["Authorization"] = f"Bearer {bearer_token}"
    request = urllib.request.Request(
        f"{base_url.rstrip('/')}{path}",
        data=json.dumps(payload).encode("utf-8"),
        method=method,
        headers=headers,
    )
    with urllib.request.urlopen(request, timeout=timeout_seconds) as response:
        body = json.loads(response.read())
    if not isinstance(body, dict) or body.get("code") != 0:
        raise RuntimeError(f"fixture API failed for {method} {path}")
    return body


def prepare_fixture(
    *,
    base_url: str,
    run_id: str,
    admin_username: str,
    admin_password: str,
    buyer_username: str,
    buyer_password: str,
    timeout_seconds: float,
) -> tuple[str, int, dict[str, Any]]:
    admin_login = api_json(
        base_url=base_url,
        path="/api/v1/auth/login",
        method="POST",
        payload={"username": admin_username, "password": admin_password},
        timeout_seconds=timeout_seconds,
    )
    buyer_login = api_json(
        base_url=base_url,
        path="/api/v1/auth/login",
        method="POST",
        payload={"username": buyer_username, "password": buyer_password},
        timeout_seconds=timeout_seconds,
    )
    admin_token = str(admin_login["data"]["token"])
    buyer_token = str(buyer_login["data"]["token"])
    if not admin_token or not buyer_token:
        raise RuntimeError("fixture login returned an empty token")

    product = api_json(
        base_url=base_url,
        path="/api/v1/products",
        method="POST",
        payload={"name": f"Load Product {run_id}", "price": 1999},
        timeout_seconds=timeout_seconds,
        bearer_token=admin_token,
    )
    product_id = int(product["data"]["id"])
    api_json(
        base_url=base_url,
        path=f"/api/v1/products/{product_id}/status",
        method="PATCH",
        payload={"status": "on_sale"},
        timeout_seconds=timeout_seconds,
        bearer_token=admin_token,
    )
    api_json(
        base_url=base_url,
        path="/api/v1/inventory/init",
        method="POST",
        payload={"product_id": product_id, "quantity": 100000},
        timeout_seconds=timeout_seconds,
        bearer_token=admin_token,
    )
    fixture = {
        "schema_version": 1,
        "run_id": run_id,
        "admin_username": admin_username,
        "buyer_username": buyer_username,
        "product_id": product_id,
        "initial_inventory": 100000,
        "tokens_persisted": False,
    }
    return buyer_token, product_id, fixture


def request_order(
    *,
    base_url: str,
    token: str,
    product_id: int,
    run_id: str,
    stage: str,
    sequence: int,
    timeout_seconds: float,
) -> Sample:
    started_wall = dt.datetime.now(dt.timezone.utc).isoformat()
    started = time.perf_counter()
    payload = json.dumps(
        {
            "idempotency_key": f"load-{run_id}-{stage}-{sequence}-{uuid.uuid4().hex[:12]}",
            "items": [{"product_id": product_id, "quantity": 1}],
        }
    ).encode("utf-8")
    request = urllib.request.Request(
        f"{base_url.rstrip('/')}/api/v1/orders",
        data=payload,
        method="POST",
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
            "X-Request-ID": f"load-{run_id}-{stage}-{sequence}",
        },
    )
    status: int | None = None
    outcome = "error"
    error: str | None = None
    try:
        with urllib.request.urlopen(request, timeout=timeout_seconds) as response:
            status = response.status
            response.read()
        outcome = "success" if 200 <= status < 300 else "http_error"
    except urllib.error.HTTPError as exc:
        status = exc.code
        exc.read()
        outcome = "http_error"
        error = f"HTTPError:{exc.code}"
    except urllib.error.URLError as exc:
        outcome = "network_error"
        error = f"URLError:{type(exc.reason).__name__}"
    except TimeoutError:
        outcome = "timeout"
        error = "TimeoutError"
    except Exception as exc:
        outcome = "client_error"
        error = type(exc).__name__
    duration_ms = (time.perf_counter() - started) * 1000.0
    return Sample(
        stage=stage,
        sequence=sequence,
        started_at=started_wall,
        duration_ms=round(duration_ms, 3),
        status=status,
        outcome=outcome,
        error=error,
    )


def run_stage(
    *,
    base_url: str,
    token: str,
    product_id: int,
    run_id: str,
    stage_name: str,
    concurrency: int,
    duration_seconds: float,
    request_limit: int,
    timeout_seconds: float,
    sequence_offset: int,
) -> tuple[list[Sample], float, int]:
    started = time.monotonic()
    allocator = Allocator(started + duration_seconds, request_limit, sequence_offset)
    samples: list[Sample] = []
    samples_lock = threading.Lock()

    def worker() -> None:
        local: list[Sample] = []
        while True:
            sequence = allocator.next()
            if sequence is None:
                break
            local.append(
                request_order(
                    base_url=base_url,
                    token=token,
                    product_id=product_id,
                    run_id=run_id,
                    stage=stage_name,
                    sequence=sequence,
                    timeout_seconds=timeout_seconds,
                )
            )
        with samples_lock:
            samples.extend(local)

    with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as executor:
        futures = [executor.submit(worker) for _ in range(concurrency)]
        for future in futures:
            future.result()
    elapsed = max(time.monotonic() - started, 0.001)
    samples.sort(key=lambda sample: sample.sequence)
    return samples, elapsed, allocator.sequence


def summarize_stage(name: str, concurrency: int, elapsed: float, samples: list[Sample]) -> dict[str, Any]:
    durations = [sample.duration_ms for sample in samples]
    successes = sum(sample.outcome == "success" for sample in samples)
    errors = len(samples) - successes
    statuses = Counter("none" if sample.status is None else str(sample.status) for sample in samples)
    outcomes = Counter(sample.outcome for sample in samples)
    return {
        "name": name,
        "concurrency": concurrency,
        "elapsed_seconds": round(elapsed, 3),
        "requests": len(samples),
        "successes": successes,
        "errors": errors,
        "error_rate": round(errors / len(samples), 6) if samples else 0.0,
        "throughput_rps": round(len(samples) / elapsed, 3),
        "latency_ms": {
            "p50": round(percentile(durations, 0.50), 3),
            "p95": round(percentile(durations, 0.95), 3),
            "p99": round(percentile(durations, 0.99), 3),
            "max": round(max(durations), 3) if durations else 0.0,
        },
        "status_counts": dict(sorted(statuses.items())),
        "outcome_counts": dict(sorted(outcomes.items())),
    }


def render_markdown(document: dict[str, Any]) -> str:
    lines = [
        "# Bounded Order Creation Load Result",
        "",
        f"- Run ID: `{document['run_id']}`",
        f"- Product ID: `{document['product_id']}`",
        f"- Warm-up: {document['warmup']['requests']} requests / {document['warmup']['elapsed_seconds']}s",
        f"- Request timeout: {document['request_timeout_seconds']}s",
        f"- Maximum measured requests: {document['max_requests']}",
        "",
        "| Stage | Concurrency | Requests | Successes | Errors | Error rate | RPS | P50 ms | P95 ms | P99 ms | Max ms |",
        "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |",
    ]
    for stage in document["stages"]:
        latency = stage["latency_ms"]
        lines.append(
            "| {name} | {concurrency} | {requests} | {successes} | {errors} | {error_rate:.2%} | "
            "{throughput_rps:.3f} | {p50:.3f} | {p95:.3f} | {p99:.3f} | {max_value:.3f} |".format(
                name=stage["name"],
                concurrency=stage["concurrency"],
                requests=stage["requests"],
                successes=stage["successes"],
                errors=stage["errors"],
                error_rate=stage["error_rate"],
                throughput_rps=stage["throughput_rps"],
                p50=latency["p50"],
                p95=latency["p95"],
                p99=latency["p99"],
                max_value=latency["max"],
            )
        )
    lines.extend(
        [
            "",
            "> This file contains synthetic bounded measurements from one GitHub-hosted runner. It is not a production SLO or capacity guarantee.",
            "",
        ]
    )
    return "\n".join(lines)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run a bounded staged order-creation load measurement")
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--prepare-fixture", action="store_true")
    parser.add_argument("--admin-username")
    parser.add_argument("--buyer-username")
    parser.add_argument("--admin-password-env", default="LOAD_ADMIN_PASSWORD")
    parser.add_argument("--buyer-password-env", default="LOAD_BUYER_PASSWORD")
    parser.add_argument("--token-env", default="LOAD_BEARER_TOKEN")
    parser.add_argument("--product-id", type=int)
    parser.add_argument("--concurrency-levels", default="1,4,8,16,32")
    parser.add_argument("--warmup-seconds", type=float, default=5.0)
    parser.add_argument("--stage-seconds", type=float, default=8.0)
    parser.add_argument("--request-timeout-seconds", type=float, default=10.0)
    parser.add_argument("--max-requests", type=int, default=3000)
    parser.add_argument("--output-dir", type=pathlib.Path, required=True)
    return parser.parse_args()


def validate_args(args: argparse.Namespace) -> tuple[int, ...]:
    levels = validate_levels(args.concurrency_levels)
    if args.prepare_fixture:
        if not args.admin_username or not args.buyer_username:
            raise ValueError("fixture preparation requires admin and buyer usernames")
        if not os.environ.get(args.admin_password_env) or not os.environ.get(args.buyer_password_env):
            raise ValueError("fixture preparation requires password environment variables")
    else:
        if args.product_id is None or args.product_id < 1:
            raise ValueError("product ID must be positive when fixture preparation is disabled")
        if not os.environ.get(args.token_env):
            raise ValueError("bearer token environment variable is required")
    if not 0.1 <= args.warmup_seconds <= MAX_WARMUP_SECONDS:
        raise ValueError(f"warm-up must be between 0.1 and {MAX_WARMUP_SECONDS} seconds")
    if not 0.1 <= args.stage_seconds <= MAX_STAGE_SECONDS:
        raise ValueError(f"stage duration must be between 0.1 and {MAX_STAGE_SECONDS} seconds")
    if not 0.1 <= args.request_timeout_seconds <= 20.0:
        raise ValueError("request timeout must be between 0.1 and 20 seconds")
    if not 1 <= args.max_requests <= MAX_REQUESTS:
        raise ValueError(f"maximum requests must be between 1 and {MAX_REQUESTS}")
    return levels


def main() -> int:
    args = parse_args()
    levels = validate_args(args)
    args.output_dir.mkdir(parents=True, exist_ok=True)

    if args.prepare_fixture:
        token, product_id, fixture = prepare_fixture(
            base_url=args.base_url,
            run_id=args.run_id,
            admin_username=args.admin_username,
            admin_password=os.environ[args.admin_password_env],
            buyer_username=args.buyer_username,
            buyer_password=os.environ[args.buyer_password_env],
            timeout_seconds=args.request_timeout_seconds,
        )
        (args.output_dir / "fixture.json").write_text(
            json.dumps(fixture, indent=2, sort_keys=True) + "\n", encoding="utf-8"
        )
    else:
        token = os.environ[args.token_env]
        product_id = int(args.product_id)

    warmup_samples, warmup_elapsed, sequence = run_stage(
        base_url=args.base_url,
        token=token,
        product_id=product_id,
        run_id=args.run_id,
        stage_name="warmup",
        concurrency=min(4, levels[-1]),
        duration_seconds=args.warmup_seconds,
        request_limit=min(200, args.max_requests),
        timeout_seconds=args.request_timeout_seconds,
        sequence_offset=0,
    )

    all_samples: list[Sample] = []
    stage_documents: list[dict[str, Any]] = []
    remaining = args.max_requests
    for concurrency in levels:
        if remaining <= 0:
            break
        stage_name = f"c{concurrency}"
        samples, elapsed, sequence = run_stage(
            base_url=args.base_url,
            token=token,
            product_id=product_id,
            run_id=args.run_id,
            stage_name=stage_name,
            concurrency=concurrency,
            duration_seconds=args.stage_seconds,
            request_limit=remaining,
            timeout_seconds=args.request_timeout_seconds,
            sequence_offset=sequence,
        )
        all_samples.extend(samples)
        remaining -= len(samples)
        stage_documents.append(summarize_stage(stage_name, concurrency, elapsed, samples))
        if not samples:
            break

    if not all_samples:
        raise RuntimeError("measured load produced no requests")

    document = {
        "schema_version": 1,
        "run_id": args.run_id,
        "product_id": product_id,
        "created_at": dt.datetime.now(dt.timezone.utc).isoformat(),
        "profile": "POST /api/v1/orders with one item and unique idempotency key",
        "concurrency_levels": list(levels),
        "stage_seconds": args.stage_seconds,
        "request_timeout_seconds": args.request_timeout_seconds,
        "max_requests": args.max_requests,
        "warmup": summarize_stage("warmup", min(4, levels[-1]), warmup_elapsed, warmup_samples),
        "stages": stage_documents,
        "measured_requests": len(all_samples),
        "measured_successes": sum(sample.outcome == "success" for sample in all_samples),
        "measured_errors": sum(sample.outcome != "success" for sample in all_samples),
        "credentials_persisted": False,
    }
    with (args.output_dir / "samples.jsonl").open("w", encoding="utf-8") as handle:
        for sample in all_samples:
            handle.write(json.dumps(sample.as_dict(), sort_keys=True) + "\n")
    (args.output_dir / "load-summary.json").write_text(
        json.dumps(document, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    (args.output_dir / "load-summary.md").write_text(render_markdown(document), encoding="utf-8")
    print(render_markdown(document))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
