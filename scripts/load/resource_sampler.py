#!/usr/bin/env python3

from __future__ import annotations

import argparse
import datetime as dt
import json
import pathlib
import subprocess
import time

MAX_DURATION_SECONDS = 240.0
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


def main() -> int:
    parser = argparse.ArgumentParser(description="Sample Docker resources until the load driver completes")
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--stop-file", type=pathlib.Path, required=True)
    parser.add_argument("--max-duration-seconds", type=float, default=180.0)
    parser.add_argument("--interval-seconds", type=float, default=2.0)
    args = parser.parse_args()
    if not 1.0 <= args.max_duration_seconds <= MAX_DURATION_SECONDS:
        raise ValueError(f"maximum duration must be between 1 and {MAX_DURATION_SECONDS} seconds")
    if not MIN_INTERVAL_SECONDS <= args.interval_seconds <= 10.0:
        raise ValueError("interval must be between 1 and 10 seconds")

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.stop_file.parent.mkdir(parents=True, exist_ok=True)
    args.stop_file.unlink(missing_ok=True)
    deadline = time.monotonic() + args.max_duration_seconds
    samples = 0
    stop_reason = "completion_file"
    with args.output.open("w", encoding="utf-8") as handle:
        while True:
            for record in snapshot():
                handle.write(json.dumps(record, sort_keys=True) + "\n")
                samples += 1
            handle.flush()
            if args.stop_file.is_file():
                break
            if time.monotonic() >= deadline:
                stop_reason = "maximum_duration"
                break
            time.sleep(args.interval_seconds)
    if samples == 0:
        raise RuntimeError("resource sampler produced no container records")
    print(f"resource_samples={samples}")
    print(f"stop_reason={stop_reason}")
    if stop_reason != "completion_file":
        raise RuntimeError("resource sampler reached its maximum duration before the load driver completed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
