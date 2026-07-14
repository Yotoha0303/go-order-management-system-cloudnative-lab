#!/usr/bin/env python3

from __future__ import annotations

import argparse
import datetime as dt
import json
import pathlib
import subprocess
import time
from collections.abc import Callable

MAX_DURATION_SECONDS = 240.0
MAX_START_TIMEOUT_SECONDS = 120.0
MIN_INTERVAL_SECONDS = 1.0


def snapshot() -> list[dict[str, object]]:
    result = subprocess.run(
        ["docker", "stats", "--no-stream", "--format", "{{json .}}"],
        check=True,
        capture_output=True,
        text=True,
        timeout=30,
    )
    captured_at = dt.datetime.now(dt.timezone.utc).isoformat()
    records: list[dict[str, object]] = []
    for line in result.stdout.splitlines():
        if not line.strip():
            continue
        raw = json.loads(line)
        records.append({"captured_at": captured_at, "container": raw})
    return records


def wait_for_measurement_start(
    start_file: pathlib.Path,
    stop_file: pathlib.Path,
    timeout_seconds: float,
    poll_seconds: float = 0.1,
) -> None:
    deadline = time.monotonic() + timeout_seconds
    while True:
        if stop_file.is_file():
            raise RuntimeError("load completed before the measurement-start marker appeared")
        if start_file.is_file():
            return
        if time.monotonic() >= deadline:
            raise RuntimeError("measurement-start marker did not appear before the startup timeout")
        time.sleep(poll_seconds)


def sample_until_complete(
    output: pathlib.Path,
    stop_file: pathlib.Path,
    max_duration_seconds: float,
    interval_seconds: float,
    snapshot_func: Callable[[], list[dict[str, object]]] = snapshot,
) -> tuple[int, str]:
    deadline = time.monotonic() + max_duration_seconds
    samples = 0
    stop_reason = "completion_file"
    with output.open("w", encoding="utf-8") as handle:
        while True:
            if stop_file.is_file():
                break
            records = snapshot_func()
            if stop_file.is_file():
                break
            for record in records:
                handle.write(json.dumps(record, sort_keys=True) + "\n")
                samples += 1
            handle.flush()
            if time.monotonic() >= deadline:
                stop_reason = "maximum_duration"
                break
            time.sleep(interval_seconds)
    if samples == 0:
        raise RuntimeError("resource sampler produced no measured-stage container records")
    if stop_reason != "completion_file":
        raise RuntimeError("resource sampler reached its maximum duration before the load driver completed")
    return samples, stop_reason


def main() -> int:
    parser = argparse.ArgumentParser(description="Sample Docker resources only during measured load stages")
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--start-file", type=pathlib.Path, required=True)
    parser.add_argument("--stop-file", type=pathlib.Path, required=True)
    parser.add_argument("--start-timeout-seconds", type=float, default=60.0)
    parser.add_argument("--max-duration-seconds", type=float, default=180.0)
    parser.add_argument("--interval-seconds", type=float, default=2.0)
    args = parser.parse_args()
    if not 1.0 <= args.start_timeout_seconds <= MAX_START_TIMEOUT_SECONDS:
        raise ValueError(f"start timeout must be between 1 and {MAX_START_TIMEOUT_SECONDS} seconds")
    if not 1.0 <= args.max_duration_seconds <= MAX_DURATION_SECONDS:
        raise ValueError(f"maximum duration must be between 1 and {MAX_DURATION_SECONDS} seconds")
    if not MIN_INTERVAL_SECONDS <= args.interval_seconds <= 10.0:
        raise ValueError("interval must be between 1 and 10 seconds")

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.start_file.parent.mkdir(parents=True, exist_ok=True)
    args.stop_file.parent.mkdir(parents=True, exist_ok=True)
    wait_for_measurement_start(
        args.start_file,
        args.stop_file,
        args.start_timeout_seconds,
    )
    samples, stop_reason = sample_until_complete(
        args.output,
        args.stop_file,
        args.max_duration_seconds,
        args.interval_seconds,
    )
    print("start_reason=measurement_start_file")
    print(f"resource_samples={samples}")
    print(f"stop_reason={stop_reason}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
