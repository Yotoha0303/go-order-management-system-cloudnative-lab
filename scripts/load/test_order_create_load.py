#!/usr/bin/env python3

import argparse
import importlib.util
import json
import pathlib
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

MODULE_PATH = pathlib.Path(__file__).with_name("order_create_load.py")
SPEC = importlib.util.spec_from_file_location("order_create_load", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
load = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(load)


class OrderHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def do_POST(self) -> None:  # noqa: N802
        length = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(length))
        if self.path != "/api/v1/orders" or not payload.get("idempotency_key"):
            self.send_response(400)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        body = b'{"code":0,"data":{"status":"pending"}}'
        self.send_response(201)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, _format: str, *_args: object) -> None:
        return


class LoadDriverTest(unittest.TestCase):
    def test_percentile_interpolates_and_handles_empty(self) -> None:
        self.assertEqual(load.percentile([], 0.95), 0.0)
        self.assertEqual(load.percentile([10.0], 0.95), 10.0)
        self.assertEqual(load.percentile([1.0, 2.0, 3.0, 4.0], 0.5), 2.5)
        self.assertAlmostEqual(load.percentile([1.0, 2.0, 3.0, 4.0], 0.95), 3.85)

    def test_concurrency_levels_must_be_strict_and_bounded(self) -> None:
        self.assertEqual(load.validate_levels("1,4,8,16,32"), (1, 4, 8, 16, 32))
        for value in ("", "0", "1,1", "4,2", "33", "x"):
            with self.subTest(value=value):
                with self.assertRaises(ValueError):
                    load.validate_levels(value)

    def test_validate_args_enforces_runtime_caps(self) -> None:
        valid = argparse.Namespace(
            concurrency_levels="1,4",
            product_id=1,
            warmup_seconds=1.0,
            stage_seconds=2.0,
            request_timeout_seconds=3.0,
            max_requests=100,
        )
        self.assertEqual(load.validate_args(valid), (1, 4))
        invalid = argparse.Namespace(**vars(valid))
        invalid.max_requests = load.MAX_REQUESTS + 1
        with self.assertRaisesRegex(ValueError, "maximum requests"):
            load.validate_args(invalid)

    def test_real_local_server_stage_produces_success_samples(self) -> None:
        server = ThreadingHTTPServer(("127.0.0.1", 0), OrderHandler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            samples, elapsed, sequence = load.run_stage(
                base_url=f"http://127.0.0.1:{server.server_port}",
                token="synthetic-token",
                product_id=7,
                run_id="unit",
                stage_name="c2",
                concurrency=2,
                duration_seconds=0.2,
                request_limit=12,
                timeout_seconds=2.0,
                sequence_offset=0,
            )
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=2)
        self.assertGreater(len(samples), 0)
        self.assertLessEqual(len(samples), 12)
        self.assertEqual(sequence, len(samples))
        self.assertTrue(all(sample.outcome == "success" for sample in samples))
        summary = load.summarize_stage("c2", 2, elapsed, samples)
        self.assertEqual(summary["errors"], 0)
        self.assertGreater(summary["throughput_rps"], 0)
        self.assertGreater(summary["latency_ms"]["p95"], 0)

    def test_render_markdown_does_not_contain_token(self) -> None:
        document = {
            "run_id": "unit",
            "product_id": 1,
            "request_timeout_seconds": 2.0,
            "max_requests": 10,
            "warmup": {"requests": 1, "elapsed_seconds": 0.1},
            "stages": [
                {
                    "name": "c1",
                    "concurrency": 1,
                    "requests": 2,
                    "successes": 2,
                    "errors": 0,
                    "error_rate": 0.0,
                    "throughput_rps": 10.0,
                    "latency_ms": {"p50": 10.0, "p95": 11.0, "p99": 12.0, "max": 13.0},
                }
            ],
        }
        rendered = load.render_markdown(document)
        self.assertIn("P95 ms", rendered)
        self.assertNotIn("synthetic-token", rendered)


if __name__ == "__main__":
    unittest.main()
