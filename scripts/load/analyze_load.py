#!/usr/bin/env python3

from __future__ import annotations

import argparse
import json
import pathlib
from collections import defaultdict
from typing import Any


def parse_cpu(value: str) -> float:
    normalized = value.strip().removesuffix("%")
    return float(normalized) if normalized else 0.0


def load_resource_peaks(path: pathlib.Path) -> dict[str, dict[str, Any]]:
    peaks: dict[str, dict[str, Any]] = defaultdict(
        lambda: {"peak_cpu_percent": 0.0, "peak_memory_usage": "", "samples": 0}
    )
    with path.open("r", encoding="utf-8") as handle:
        for line in handle:
            if not line.strip():
                continue
            record = json.loads(line)
            container = record.get("container", {})
            name = str(container.get("Name") or container.get("Container") or "unknown")
            cpu = parse_cpu(str(container.get("CPUPerc", "0%")))
            current = peaks[name]
            current["samples"] += 1
            if cpu >= current["peak_cpu_percent"]:
                current["peak_cpu_percent"] = round(cpu, 3)
                current["peak_memory_usage"] = str(container.get("MemUsage", ""))
    return dict(sorted(peaks.items()))


def highest_cpu(resource_peaks: dict[str, dict[str, Any]]) -> tuple[str, dict[str, Any]]:
    return max(
        resource_peaks.items(),
        key=lambda item: float(item[1].get("peak_cpu_percent", 0.0)),
        default=("unknown", {"peak_cpu_percent": 0.0}),
    )


def successful_throughput(stage: dict[str, Any]) -> float:
    if "successful_throughput_rps" in stage:
        return float(stage["successful_throughput_rps"])
    elapsed = max(float(stage.get("elapsed_seconds", 0.0)), 0.001)
    return float(stage.get("successes", 0)) / elapsed


def healthy_stages_before_boundary(
    stages: list[dict[str, Any]],
    boundary: dict[str, Any],
) -> list[dict[str, Any]]:
    boundary_concurrency = boundary.get("first_observed_at_concurrency")
    healthy: list[dict[str, Any]] = []
    for stage in stages:
        concurrency = stage.get("concurrency")
        if boundary_concurrency is not None and concurrency == boundary_concurrency:
            break
        stop_reason = str(stage.get("stop_reason", "duration"))
        if not bool(stage.get("measurement_eligible", stop_reason == "duration")):
            break
        status_counts = stage.get("status_counts", {})
        if int(status_counts.get("429", 0)) > 0 or float(stage.get("error_rate", 0.0)) >= 0.02:
            break
        healthy.append(stage)
    return healthy


def healthy_error_count(
    stages: list[dict[str, Any]],
    boundary: dict[str, Any],
) -> int:
    return sum(int(stage.get("errors", 0)) for stage in healthy_stages_before_boundary(stages, boundary))


def best_healthy_successful_throughput(
    stages: list[dict[str, Any]],
    boundary: dict[str, Any],
) -> float:
    healthy = healthy_stages_before_boundary(stages, boundary)
    return round(max((successful_throughput(stage) for stage in healthy), default=0.0), 3)


def infer_capacity_boundary(
    stages: list[dict[str, Any]],
    resource_peaks: dict[str, dict[str, Any]],
) -> dict[str, Any]:
    if not stages:
        raise ValueError("load summary contains no measured stages")

    previous: dict[str, Any] | None = None
    for stage in stages:
        concurrency = stage["concurrency"]
        stop_reason = str(stage.get("stop_reason", "duration"))
        measurement_eligible = bool(stage.get("measurement_eligible", stop_reason == "duration"))
        if not measurement_eligible:
            return {
                "classification": "request_ceiling_before_stage_duration",
                "first_observed_at_concurrency": concurrency,
                "measured_evidence": (
                    f"The per-stage request ceiling stopped concurrency {concurrency} after "
                    f"{stage.get('issuance_elapsed_seconds', stage.get('elapsed_seconds', 0))}s, "
                    f"before the configured {stage.get('configured_duration_seconds', 'unknown')}s issuance window."
                ),
                "inference": (
                    "This is a measurement-safety boundary, not an application-capacity boundary. "
                    "The truncated stage is excluded from sustained throughput and tail-latency inference."
                ),
            }

        status_counts = stage.get("status_counts", {})
        error_rate = float(stage.get("error_rate", 0.0))
        if int(status_counts.get("429", 0)) > 0:
            return {
                "classification": "gateway_rate_limit",
                "first_observed_at_concurrency": concurrency,
                "measured_evidence": f"HTTP 429 appeared at concurrency {concurrency} with error rate {error_rate:.2%}",
                "inference": "The configured Gateway token bucket became the first hard request boundary.",
            }
        if error_rate >= 0.02:
            return {
                "classification": "request_error_boundary",
                "first_observed_at_concurrency": concurrency,
                "measured_evidence": f"Error rate reached {error_rate:.2%} at concurrency {concurrency}",
                "inference": (
                    "The first observed capacity boundary is request failure; inspect status and outcome counts "
                    "before attributing a component."
                ),
            }

        if previous is not None:
            previous_rps = successful_throughput(previous)
            current_rps = successful_throughput(stage)
            previous_p95 = float(previous["latency_ms"]["p95"])
            current_p95 = float(stage["latency_ms"]["p95"])
            throughput_gain = (current_rps - previous_rps) / max(previous_rps, 0.001)
            latency_growth = (current_p95 - previous_p95) / max(previous_p95, 0.001)
            if throughput_gain < 0.15 and latency_growth > 0.30:
                highest = highest_cpu(resource_peaks)
                return {
                    "classification": "throughput_plateau_with_tail_growth",
                    "first_observed_at_concurrency": concurrency,
                    "measured_evidence": (
                        f"Successful throughput gain was {throughput_gain:.2%} while P95 latency grew {latency_growth:.2%} "
                        f"from concurrency {previous['concurrency']} to {concurrency}"
                    ),
                    "inference": (
                        f"Saturation began within the synchronous order path. The highest sampled container CPU was "
                        f"{highest[0]} at {highest[1].get('peak_cpu_percent', 0.0):.3f}%; "
                        "this is a diagnostic lead, not proof of root cause."
                    ),
                }
        previous = stage

    highest = highest_cpu(resource_peaks)
    return {
        "classification": "not_reached_within_bounded_range",
        "first_observed_at_concurrency": None,
        "measured_evidence": (
            "No sustained-duration stage produced at least 2% errors or the defined successful-throughput plateau "
            "plus P95-growth signal."
        ),
        "inference": (
            f"The bounded test ceiling was reached before a defensible saturation point. Highest sampled container CPU was "
            f"{highest[0]} at {highest[1].get('peak_cpu_percent', 0.0):.3f}%; "
            "it must not be called a bottleneck without a wider test."
        ),
    }


def render_markdown(report: dict[str, Any]) -> str:
    load = report["load"]
    boundary = report["first_observed_boundary"]
    lines = [
        "# Bounded Load-Test Analysis",
        "",
        "## Measured result",
        "",
        f"- All bounded measured requests retained in the artifact: {load['measured_requests']}",
        f"- All bounded successes retained in the artifact: {load['measured_successes']}",
        f"- All bounded errors retained in the artifact: {load['measured_errors']}",
        f"- Errors in healthy sustained stages before boundary: {report['healthy_measured_errors']}",
        f"- Healthy sustained stages before boundary: {report['healthy_stage_count']}",
        f"- Best healthy sustained successful throughput: {report['best_healthy_successful_throughput_rps']:.3f} requests/second",
        f"- Highest healthy sustained P95: {report['highest_healthy_p95_ms']:.3f} ms",
        f"- Boundary classification: `{boundary['classification']}`",
        f"- First observed concurrency: `{boundary['first_observed_at_concurrency']}`",
        f"- Measured evidence: {boundary['measured_evidence']}",
        "",
        "## Interpretation",
        "",
        boundary["inference"],
        "",
        "## Resource peaks during measured stages",
        "",
        "| Container | Peak CPU | Memory at peak sample | Samples |",
        "| --- | ---: | --- | ---: |",
    ]
    for name, evidence in report["resource_peaks"].items():
        lines.append(
            f"| {name} | {evidence['peak_cpu_percent']:.3f}% | "
            f"{evidence['peak_memory_usage']} | {evidence['samples']} |"
        )
    lines.extend(
        [
            "",
            "> Summary throughput, P95 and healthy error count exclude the boundary stage and any later stage; throughput uses successful rather than attempted requests.",
            "",
            "> Raw all-stage requests and errors remain in the artifact for diagnosis and are not presented as healthy capacity evidence.",
            "",
            "> Measured evidence and inference are intentionally separated. "
            "This single-runner synthetic test is not a production capacity guarantee.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description="Analyze a bounded staged load run")
    parser.add_argument("--load-summary", type=pathlib.Path, required=True)
    parser.add_argument("--resource-samples", type=pathlib.Path, required=True)
    parser.add_argument("--output-json", type=pathlib.Path, required=True)
    parser.add_argument("--output-markdown", type=pathlib.Path, required=True)
    args = parser.parse_args()

    load_summary = json.loads(args.load_summary.read_text(encoding="utf-8"))
    if load_summary.get("schema_version") != 1:
        raise ValueError("unsupported load summary schema")
    peaks = load_resource_peaks(args.resource_samples)
    if not peaks:
        raise ValueError("resource sample set is empty")
    stages = load_summary["stages"]
    boundary = infer_capacity_boundary(stages, peaks)
    healthy = healthy_stages_before_boundary(stages, boundary)
    report = {
        "schema_version": 1,
        "load": load_summary,
        "resource_peaks": peaks,
        "first_observed_boundary": boundary,
        "healthy_stage_count": len(healthy),
        "healthy_measured_errors": healthy_error_count(stages, boundary),
        "best_healthy_successful_throughput_rps": round(
            max((successful_throughput(stage) for stage in healthy), default=0.0), 3
        ),
        "highest_healthy_p95_ms": round(
            max((float(stage["latency_ms"]["p95"]) for stage in healthy), default=0.0), 3
        ),
        "measurement_scope": {
            "environment": "single GitHub-hosted runner",
            "traffic": "synthetic order creation",
            "resource_interval": "measured stages only",
            "production_slo": False,
            "production_capacity_guarantee": False,
        },
    }
    args.output_json.parent.mkdir(parents=True, exist_ok=True)
    args.output_json.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    args.output_markdown.write_text(render_markdown(report), encoding="utf-8")
    print(render_markdown(report))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
