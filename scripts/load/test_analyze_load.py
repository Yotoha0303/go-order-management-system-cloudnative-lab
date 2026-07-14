#!/usr/bin/env python3

import importlib.util
import json
import pathlib
import sys
import tempfile
import unittest

MODULE_PATH = pathlib.Path(__file__).with_name("analyze_load.py")
SPEC = importlib.util.spec_from_file_location("analyze_load", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
analysis = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = analysis
SPEC.loader.exec_module(analysis)


def stage(
    concurrency: int,
    *,
    rps: float,
    p95: float,
    errors: int = 0,
    requests: int = 100,
    status_counts: dict[str, int] | None = None,
    measurement_eligible: bool = True,
    stop_reason: str = "duration",
) -> dict:
    successes = requests - errors
    elapsed = requests / max(rps, 0.001)
    return {
        "name": f"c{concurrency}",
        "concurrency": concurrency,
        "requests": requests,
        "successes": successes,
        "errors": errors,
        "error_rate": errors / requests,
        "throughput_rps": rps,
        "successful_throughput_rps": rps * successes / requests,
        "elapsed_seconds": elapsed,
        "latency_ms": {"p50": p95 / 2, "p95": p95, "p99": p95 * 1.2, "max": p95 * 1.5},
        "status_counts": status_counts or {"201": successes},
        "outcome_counts": {"success": successes, "http_error": errors},
        "configured_duration_seconds": 8.0,
        "issuance_elapsed_seconds": 8.0 if measurement_eligible else 1.5,
        "measurement_eligible": measurement_eligible,
        "stop_reason": stop_reason,
    }


class LoadAnalysisTest(unittest.TestCase):
    def test_parse_cpu(self) -> None:
        self.assertEqual(analysis.parse_cpu("12.50%"), 12.5)
        self.assertEqual(analysis.parse_cpu("0%"), 0.0)
        self.assertEqual(analysis.parse_cpu(""), 0.0)

    def test_resource_peak_reader_keeps_highest_cpu_sample(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            path = pathlib.Path(temp) / "resources.jsonl"
            records = [
                {"captured_at": "a", "container": {"Name": "order-service", "CPUPerc": "12.0%", "MemUsage": "20MiB / 1GiB"}},
                {"captured_at": "b", "container": {"Name": "order-service", "CPUPerc": "18.5%", "MemUsage": "25MiB / 1GiB"}},
                {"captured_at": "c", "container": {"Name": "mysql", "CPUPerc": "8.0%", "MemUsage": "100MiB / 1GiB"}},
            ]
            path.write_text("".join(json.dumps(record) + "\n" for record in records), encoding="utf-8")
            peaks = analysis.load_resource_peaks(path)
        self.assertEqual(peaks["order-service"]["peak_cpu_percent"], 18.5)
        self.assertEqual(peaks["order-service"]["peak_memory_usage"], "25MiB / 1GiB")
        self.assertEqual(peaks["order-service"]["samples"], 2)

    def test_429_is_classified_as_gateway_rate_limit(self) -> None:
        result = analysis.infer_capacity_boundary(
            [stage(1, rps=10, p95=20), stage(4, rps=20, p95=30, errors=3, status_counts={"201": 97, "429": 3})],
            {"api-gateway": {"peak_cpu_percent": 5.0}},
        )
        self.assertEqual(result["classification"], "gateway_rate_limit")
        self.assertEqual(result["first_observed_at_concurrency"], 4)

    def test_error_rate_boundary_precedes_same_stage_plateau_inference(self) -> None:
        result = analysis.infer_capacity_boundary(
            [stage(1, rps=10, p95=20), stage(8, rps=11, p95=80, errors=2)],
            {"order-service": {"peak_cpu_percent": 70.0}},
        )
        self.assertEqual(result["classification"], "request_error_boundary")
        self.assertEqual(result["first_observed_at_concurrency"], 8)

    def test_earlier_plateau_precedes_later_request_errors(self) -> None:
        result = analysis.infer_capacity_boundary(
            [
                stage(1, rps=10, p95=20),
                stage(4, rps=40, p95=20),
                stage(8, rps=44, p95=30),
                stage(32, rps=50, p95=80, errors=5),
            ],
            {"order-service": {"peak_cpu_percent": 75.0}},
        )
        self.assertEqual(result["classification"], "throughput_plateau_with_tail_growth")
        self.assertEqual(result["first_observed_at_concurrency"], 8)

    def test_request_ceiling_is_not_misreported_as_capacity(self) -> None:
        result = analysis.infer_capacity_boundary(
            [
                stage(1, rps=10, p95=20),
                stage(4, rps=300, p95=10, measurement_eligible=False, stop_reason="request_limit"),
            ],
            {"api-gateway": {"peak_cpu_percent": 5.0}},
        )
        self.assertEqual(result["classification"], "request_ceiling_before_stage_duration")
        self.assertEqual(result["first_observed_at_concurrency"], 4)
        self.assertIn("measurement-safety boundary", result["inference"])

    def test_plateau_and_tail_growth_are_reported_separately_from_cpu_inference(self) -> None:
        result = analysis.infer_capacity_boundary(
            [stage(4, rps=40, p95=20), stage(8, rps=44, p95=30)],
            {
                "order-service": {"peak_cpu_percent": 75.0},
                "mysql": {"peak_cpu_percent": 30.0},
            },
        )
        self.assertEqual(result["classification"], "throughput_plateau_with_tail_growth")
        self.assertIn("Successful throughput gain", result["measured_evidence"])
        self.assertIn("diagnostic lead, not proof", result["inference"])

    def test_no_saturation_is_reported_without_inventing_a_bottleneck(self) -> None:
        result = analysis.infer_capacity_boundary(
            [stage(1, rps=10, p95=20), stage(4, rps=35, p95=25), stage(8, rps=65, p95=30)],
            {"mysql": {"peak_cpu_percent": 20.0}},
        )
        self.assertEqual(result["classification"], "not_reached_within_bounded_range")
        self.assertIsNone(result["first_observed_at_concurrency"])
        self.assertIn("must not be called a bottleneck", result["inference"])

    def test_best_throughput_uses_successes_and_stops_before_error_boundary(self) -> None:
        stages = [
            stage(1, rps=10, p95=20),
            stage(4, rps=40, p95=25),
            stage(8, rps=500, p95=10, errors=50, status_counts={"201": 50, "500": 50}),
        ]
        boundary = analysis.infer_capacity_boundary(stages, {"order-service": {"peak_cpu_percent": 20.0}})
        self.assertEqual(boundary["first_observed_at_concurrency"], 8)
        healthy = analysis.healthy_stages_before_boundary(stages, boundary)
        self.assertEqual([item["concurrency"] for item in healthy], [1, 4])
        self.assertEqual(analysis.best_healthy_successful_throughput(stages, boundary), 40.0)

    def test_boundary_stage_is_excluded_from_best_healthy_throughput(self) -> None:
        stages = [stage(1, rps=10, p95=20), stage(4, rps=40, p95=20), stage(8, rps=44, p95=30)]
        boundary = analysis.infer_capacity_boundary(stages, {"order-service": {"peak_cpu_percent": 20.0}})
        self.assertEqual(boundary["classification"], "throughput_plateau_with_tail_growth")
        self.assertEqual(analysis.best_healthy_successful_throughput(stages, boundary), 40.0)

    def test_healthy_error_count_excludes_boundary_and_later_stages(self) -> None:
        stages = [
            stage(1, rps=10, p95=20, errors=1, requests=100),
            stage(4, rps=40, p95=25),
            stage(8, rps=50, p95=30, errors=5, requests=100),
            stage(16, rps=60, p95=40, errors=20, requests=100),
        ]
        boundary = analysis.infer_capacity_boundary(stages, {"order-service": {"peak_cpu_percent": 20.0}})
        self.assertEqual(boundary["classification"], "request_error_boundary")
        self.assertEqual(boundary["first_observed_at_concurrency"], 8)
        self.assertEqual(analysis.healthy_error_count(stages, boundary), 1)

    def test_markdown_separates_measurement_and_interpretation(self) -> None:
        report = {
            "load": {"measured_requests": 10, "measured_successes": 9, "measured_errors": 1},
            "healthy_measured_errors": 0,
            "healthy_stage_count": 2,
            "best_healthy_successful_throughput_rps": 12.5,
            "highest_healthy_p95_ms": 25.0,
            "first_observed_boundary": {
                "classification": "not_reached_within_bounded_range",
                "first_observed_at_concurrency": None,
                "measured_evidence": "No boundary measured.",
                "inference": "No bottleneck claimed.",
            },
            "resource_peaks": {
                "mysql": {"peak_cpu_percent": 10.0, "peak_memory_usage": "100MiB / 1GiB", "samples": 3}
            },
        }
        markdown = analysis.render_markdown(report)
        self.assertIn("## Measured result", markdown)
        self.assertIn("Best healthy sustained successful throughput", markdown)
        self.assertIn("Highest healthy sustained P95", markdown)
        self.assertIn("Errors in healthy sustained stages before boundary", markdown)
        self.assertIn("Raw all-stage requests and errors", markdown)
        self.assertIn("## Interpretation", markdown)
        self.assertIn("not a production capacity guarantee", markdown)


if __name__ == "__main__":
    unittest.main()
