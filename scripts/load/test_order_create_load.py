#!/usr/bin/env python3

import argparse
import importlib.util
import json
import os
import pathlib
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

MODULE_PATH = pathlib.Path(__file__).with_name("order_create_load.py")
SPEC = importlib.util.spec_from_file_location("order_create_load", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
load = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = load
SPEC.loader.exec_module(load)


class OrderHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    observed: list[tuple[str, str, dict | None]] = []

    def read_payload(self) -> dict | None:
        length = int(self.headers.get("Content-Length", "0"))
        if length == 0:
            return None
        return json.loads(self.rfile.read(length))

    def write_json(self, status: int, value: dict) -> None:
        body = json.dumps(value).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self) -> None:  # noqa: N802
        payload = self.read_payload()
        self.observed.append(("POST", self.path, payload))
        if self.path == "/api/v1/auth/login":
            username = str((payload or {}).get("username", ""))
            self.write_json(200, {"code": 0, "data": {"access_token": f"token-{username}"}})
            return
        if self.path == "/api/v1/products":
            if payload != {
                "name": "Load Product fixture-unit",
                "description": "bounded synthetic load-test product",
                "price_fen": 1999,
            }:
                self.write_json(400, {"code": 1})
                return
            self.write_json(201, {"code": 0, "data": {"id": 17}})
            return
        if self.path == "/api/v1/inventory/init":
            if payload != {"product_id": 17, "quantity": 100000}:
                self.write_json(400, {"code": 1})
                return
            self.write_json(201, {"code": 0, "data": {"product_id": 17}})
            return
        if self.path == "/api/v1/orders" and (payload or {}).get("idempotency_key"):
            self.write_json(201, {"code": 0, "data": {"status": "pending"}})
            return
        self.write_json(400, {"code": 1})

    def do_PATCH(self) -> None:  # noqa: N802
        payload = self.read_payload()
        self.observed.append(("PATCH", self.path, payload))
        if self.path == "/api/v1/products/17/on-sale" and payload is None:
            self.write_json(200, {"code": 0, "data": {"id": 17, "status": "on_sale"}})
            return
        self.write_json(400, {"code": 1})

    def log_message(self, _format: str, *_args: object) -> None:
        return


class LoadDriverTest(unittest.TestCase):
    def setUp(self) -> None:
        self.previous_token = os.environ.get("LOAD_BEARER_TOKEN")
        os.environ["LOAD_BEARER_TOKEN"] = "unit-token"
        OrderHandler.observed = []

    def tearDown(self) -> None:
        if self.previous_token is None:
            os.environ.pop("LOAD_BEARER_TOKEN", None)
        else:
            os.environ["LOAD_BEARER_TOKEN"] = self.previous_token
        os.environ.pop("LOAD_ADMIN_PASSWORD", None)
        os.environ.pop("LOAD_BUYER_PASSWORD", None)

    def base_args(self) -> argparse.Namespace:
        return argparse.Namespace(
            concurrency_levels="1,4",
            prepare_fixture=False,
            admin_username=None,
            buyer_username=None,
            admin_password_env="LOAD_ADMIN_PASSWORD",
            buyer_password_env="LOAD_BUYER_PASSWORD",
            token_env="LOAD_BEARER_TOKEN",
            product_id=1,
            warmup_seconds=1.0,
            stage_seconds=2.0,
            request_timeout_seconds=3.0,
            max_requests=100,
        )

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
        valid = self.base_args()
        self.assertEqual(load.validate_args(valid), (1, 4))
        invalid = argparse.Namespace(**vars(valid))
        invalid.max_requests = load.MAX_REQUESTS + 1
        with self.assertRaisesRegex(ValueError, "maximum requests"):
            load.validate_args(invalid)

    def test_fixture_preparation_requires_named_users_and_password_env(self) -> None:
        args = self.base_args()
        args.prepare_fixture = True
        args.product_id = None
        with self.assertRaisesRegex(ValueError, "admin and buyer usernames"):
            load.validate_args(args)
        args.admin_username = "admin"
        args.buyer_username = "buyer"
        with self.assertRaisesRegex(ValueError, "password environment"):
            load.validate_args(args)
        os.environ["LOAD_ADMIN_PASSWORD"] = "admin-test"
        os.environ["LOAD_BUYER_PASSWORD"] = "buyer-test"
        self.assertEqual(load.validate_args(args), (1, 4))

    def test_prepare_fixture_uses_gateway_contract_and_does_not_persist_tokens(self) -> None:
        server = ThreadingHTTPServer(("127.0.0.1", 0), OrderHandler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            token, product_id, fixture = load.prepare_fixture(
                base_url=f"http://127.0.0.1:{server.server_port}",
                run_id="fixture-unit",
                admin_username="ci-admin-fixture-unit",
                admin_password="CiAdmin123!",
                buyer_username="ci-buyer-fixture-unit",
                buyer_password="CiBuyer123!",
                timeout_seconds=2.0,
            )
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=2)
        self.assertEqual(token, "token-ci-buyer-fixture-unit")
        self.assertEqual(product_id, 17)
        self.assertFalse(fixture["tokens_persisted"])
        self.assertNotIn("token", json.dumps(fixture).lower().replace("tokens_persisted", ""))
        self.assertIn(("PATCH", "/api/v1/products/17/on-sale", None), OrderHandler.observed)
        self.assertIn(("POST", "/api/v1/inventory/init", {"product_id": 17, "quantity": 100000}), OrderHandler.observed)

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
        self.assertNotIn("unit-token", rendered)


if __name__ == "__main__":
    unittest.main()
