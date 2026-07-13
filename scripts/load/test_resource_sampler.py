#!/usr/bin/env python3

import pathlib
import tempfile
import unittest

import resource_sampler


class ResourceSamplerTest(unittest.TestCase):
    def test_wait_for_measurement_start_accepts_start_marker(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            start = root / "measurement-start"
            stop = root / "load-complete"
            start.touch()
            resource_sampler.wait_for_measurement_start(start, stop, 0.1, poll_seconds=0.001)

    def test_wait_for_measurement_start_rejects_early_completion(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            start = root / "measurement-start"
            stop = root / "load-complete"
            stop.touch()
            with self.assertRaisesRegex(RuntimeError, "completed before"):
                resource_sampler.wait_for_measurement_start(start, stop, 0.1, poll_seconds=0.001)

    def test_sampler_does_not_take_snapshot_after_completion_marker(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            output = root / "samples.jsonl"
            stop = root / "load-complete"
            calls = 0

            def snapshot() -> list[dict[str, object]]:
                nonlocal calls
                calls += 1
                stop.touch()
                return [{"captured_at": "now", "container": {"Name": "order-service", "CPUPerc": "1%"}}]

            samples, reason = resource_sampler.sample_until_complete(
                output,
                stop,
                max_duration_seconds=1.0,
                interval_seconds=0.001,
                snapshot_func=snapshot,
            )
            self.assertEqual(calls, 1)
            self.assertEqual(samples, 1)
            self.assertEqual(reason, "completion_file")
            self.assertEqual(len(output.read_text(encoding="utf-8").splitlines()), 1)


if __name__ == "__main__":
    unittest.main()
