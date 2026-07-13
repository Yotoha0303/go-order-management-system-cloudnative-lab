#!/usr/bin/env python3

import argparse
import json
import pathlib
import sys
import tempfile
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))

import manifest

COMMIT = "0123456789abcdef0123456789abcdef01234567"
REPOSITORY = "Yotoha0303/go-order-management-system-cloudnative-lab"


class BackupManifestTest(unittest.TestCase):
    def create_dumps(self, root: pathlib.Path) -> pathlib.Path:
        dumps = root / "dumps"
        dumps.mkdir()
        for database in manifest.EXPECTED_DATABASES:
            (dumps / f"{database}.sql").write_text(
                f"CREATE DATABASE `{database}`;\nSELECT '{database}';\n",
                encoding="utf-8",
            )
        return dumps

    def create_manifest(self, root: pathlib.Path, dumps: pathlib.Path) -> pathlib.Path:
        output = root / "backup-manifest.json"
        manifest.create_manifest(
            argparse.Namespace(
                dumps=dumps,
                repository=REPOSITORY,
                source_commit=COMMIT,
                mysql_version="8.4.0",
                backup_duration_ms=1200,
                restore_duration_ms=2300,
                output=output,
            )
        )
        return output

    def test_create_and_verify_exact_four_database_manifest(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            dumps = self.create_dumps(root)
            output = self.create_manifest(root, dumps)
            document = json.loads(output.read_text(encoding="utf-8"))
            references = manifest.validate_manifest(document, dumps, REPOSITORY, COMMIT)
            self.assertEqual(len(references), 4)
            self.assertEqual(
                tuple(entry["database"] for entry in document["databases"]),
                manifest.EXPECTED_DATABASES,
            )
            self.assertEqual(
                document["total_bytes"],
                sum((dumps / f"{database}.sql").stat().st_size for database in manifest.EXPECTED_DATABASES),
            )

    def test_missing_dump_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            dumps = self.create_dumps(root)
            (dumps / "go_order_inventory.sql").unlink()
            with self.assertRaisesRegex(ValueError, "missing database dump"):
                self.create_manifest(root, dumps)

    def test_corrupt_dump_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            dumps = self.create_dumps(root)
            output = self.create_manifest(root, dumps)
            document = json.loads(output.read_text(encoding="utf-8"))
            with (dumps / "go_order_ordering.sql").open("a", encoding="utf-8") as handle:
                handle.write("-- corruption\n")
            with self.assertRaisesRegex(ValueError, "dump size mismatch|dump checksum mismatch"):
                manifest.validate_manifest(document, dumps, REPOSITORY, COMMIT)

    def test_unexpected_or_reordered_database_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            dumps = self.create_dumps(root)
            output = self.create_manifest(root, dumps)
            document = json.loads(output.read_text(encoding="utf-8"))
            document["databases"][0]["database"] = "unexpected_database"
            with self.assertRaisesRegex(ValueError, "database order or membership drifted"):
                manifest.validate_manifest(document, dumps, REPOSITORY, COMMIT)

    def test_repository_and_commit_are_bound(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            dumps = self.create_dumps(root)
            output = self.create_manifest(root, dumps)
            document = json.loads(output.read_text(encoding="utf-8"))
            with self.assertRaisesRegex(ValueError, "repository does not match"):
                manifest.validate_manifest(document, dumps, "other/repository", COMMIT)
            with self.assertRaisesRegex(ValueError, "commit does not match"):
                manifest.validate_manifest(document, dumps, REPOSITORY, "f" * 40)


if __name__ == "__main__":
    unittest.main()
