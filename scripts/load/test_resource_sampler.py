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

    def test_sampler_discards_snapshot_overlapping_completion(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            output = root / "samples.jsonl"
            stop = root / "load-complete"
            calls = 0

            def snapshot() -> list[dict[str, object]]:
                nonlocal calls
                calls += 1
                if calls == 2:
                    stop.touch()
                return [
                    {
                        "captured_at": f"sample-{calls}",
                        "container": {"Name": "order-service", "CPUPerc": "1%"},
                    }
                ]

            samples, reason = resource_sampler.sample_until_complete(
                output,
                stop,
                max_duration_seconds=1.0,
                interval_seconds=0.001,
                snapshot_func=snapshot,
            )
            self.assertEqual(calls, 2)
            self.assertEqual(samples, 1)
            self.assertEqual(reason, "completion_file")
            lines = output.read_text(encoding="utf-8").splitlines()
            self.assertEqual(len(lines), 1)
            self.assertIn("sample-1", lines[0])
            self.assertNotIn("sample-2", lines[0])


if __name__ == "__main__":
    unittest.main()
