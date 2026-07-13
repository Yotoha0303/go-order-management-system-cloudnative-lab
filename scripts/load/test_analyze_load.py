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
) -> dict:
    return {
        "name": f"c{concurrency}",
        "concurrency": concurrency,
        "requests": requests,
        "successes": requests - errors,
        "errors": errors,
        "error_rate": errors / requests,
        "throughput_rps": rps,
        "latency_ms": {"p50": p95 / 2, "p95": p95, "p99": p95 * 1.2, "max": p95 * 1.5},
        "status_counts": status_counts or {"201": requests - errors},
        "outcome_counts": {"success": requests - errors, "http_error": errors},
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

    def test_error_rate_boundary_precedes_plateau_inference(self) -> None:
        result = analysis.infer_capacity_boundary(
            [stage(1, rps=10, p95=20), stage(8, rps=11, p95=80, errors=2)],
            {"order-service": {"peak_cpu_percent": 70.0}},
        )
        self.assertEqual(result["classification"], "request_error_boundary")
        self.assertEqual(result["first_observed_at_concurrency"], 8)

    def test_plateau_and_tail_growth_are_reported_separately_from_cpu_inference(self) -> None:
        result = analysis.infer_capacity_boundary(
            [stage(4, rps=40, p95=20), stage(8, rps=44, p95=30)],
            {
                "order-service": {"peak_cpu_percent": 75.0},
                "mysql": {"peak_cpu_percent": 30.0},
            },
        )
        self.assertEqual(result["classification"], "throughput_plateau_with_tail_growth")
        self.assertIn("Throughput gain", result["measured_evidence"])
        self.assertIn("diagnostic lead, not proof", result["inference"])

    def test_no_saturation_is_reported_without_inventing_a_bottleneck(self) -> None:
        result = analysis.infer_capacity_boundary(
            [stage(1, rps=10, p95=20), stage(4, rps=35, p95=25), stage(8, rps=65, p95=30)],
            {"mysql": {"peak_cpu_percent": 20.0}},
        )
        self.assertEqual(result["classification"], "not_reached_within_bounded_range")
        self.assertIsNone(result["first_observed_at_concurrency"])
        self.assertIn("must not be called a bottleneck", result["inference"])

    def test_markdown_separates_measurement_and_interpretation(self) -> None:
        report = {
            "load": {"measured_requests": 10, "measured_successes": 10, "measured_errors": 0},
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
        self.assertIn("## Interpretation", markdown)
        self.assertIn("not a production capacity guarantee", markdown)


if __name__ == "__main__":
    unittest.main()
