#!/usr/bin/env python3

import argparse
import datetime as dt
import hashlib
import json
import pathlib
import re
import sys
from typing import Any

EXPECTED_DATABASES = (
    "go_order_identity",
    "go_order_catalog",
    "go_order_inventory",
    "go_order_ordering",
)
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
COMMIT_RE = re.compile(r"^[0-9a-f]{40}$")


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def sha256_file(path: pathlib.Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def read_json(path: pathlib.Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    require(isinstance(value, dict), f"{path} must contain a JSON object")
    return value


def write_json(path: pathlib.Path, value: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def dump_entry(directory: pathlib.Path, database: str) -> dict[str, Any]:
    filename = f"{database}.sql"
    path = directory / filename
    require(path.is_file(), f"missing database dump: {filename}")
    size_bytes = path.stat().st_size
    require(size_bytes > 0, f"database dump is empty: {filename}")
    return {
        "database": database,
        "file": filename,
        "size_bytes": size_bytes,
        "sha256": sha256_file(path),
    }


def validate_utc_timestamp(value: Any) -> str:
    require(isinstance(value, str) and value, "manifest created_at is required")
    try:
        parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as exc:
        raise ValueError("manifest created_at must be an ISO-8601 timestamp") from exc
    require(
        parsed.tzinfo is not None and parsed.utcoffset() == dt.timedelta(0),
        "manifest created_at must be UTC",
    )
    return value


def exact_dump_files(directory: pathlib.Path) -> tuple[str, ...]:
    require(directory.is_dir(), "dump directory is required")
    actual = tuple(sorted(path.name for path in directory.iterdir()))
    expected = tuple(sorted(f"{database}.sql" for database in EXPECTED_DATABASES))
    require(actual == expected, "dump directory must contain exactly the four manifest SQL files")
    return actual


def create_manifest(args: argparse.Namespace) -> None:
    require(isinstance(args.repository, str) and args.repository, "repository is required")
    require(COMMIT_RE.fullmatch(args.source_commit) is not None, "source commit must be a full lowercase SHA")
    require(isinstance(args.mysql_version, str) and args.mysql_version.strip(), "MySQL version is required")
    require(args.backup_duration_ms >= 0, "backup duration must be non-negative")
    require(args.restore_duration_ms >= 0, "restore duration must be non-negative")
    exact_dump_files(args.dumps)
    entries = [dump_entry(args.dumps, database) for database in EXPECTED_DATABASES]
    total_bytes = sum(int(entry["size_bytes"]) for entry in entries)
    created_at = dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()
    manifest = {
        "schema_version": 1,
        "repository": args.repository,
        "source_commit": args.source_commit,
        "created_at": created_at,
        "mysql_version": args.mysql_version.strip(),
        "backup_duration_ms": args.backup_duration_ms,
        "restore_duration_ms": args.restore_duration_ms,
        "total_bytes": total_bytes,
        "databases": entries,
    }
    validate_manifest(manifest, args.dumps, args.repository, args.source_commit)
    write_json(args.output, manifest)


def validate_manifest(
    manifest: dict[str, Any],
    dumps: pathlib.Path,
    expected_repository: str | None = None,
    expected_commit: str | None = None,
) -> list[str]:
    require(manifest.get("schema_version") == 1, "unsupported backup manifest schema")
    repository = manifest.get("repository")
    source_commit = manifest.get("source_commit")
    require(isinstance(repository, str) and repository, "manifest repository is required")
    require(isinstance(source_commit, str) and COMMIT_RE.fullmatch(source_commit) is not None, "invalid source commit")
    validate_utc_timestamp(manifest.get("created_at"))
    mysql_version = manifest.get("mysql_version")
    require(isinstance(mysql_version, str) and mysql_version.strip(), "manifest MySQL version is required")
    if expected_repository is not None:
        require(repository == expected_repository, "backup repository does not match expected repository")
    if expected_commit is not None:
        require(source_commit == expected_commit, "backup source commit does not match expected commit")
    for field in ("backup_duration_ms", "restore_duration_ms", "total_bytes"):
        value = manifest.get(field)
        require(isinstance(value, int) and value >= 0, f"invalid manifest field: {field}")

    exact_dump_files(dumps)
    raw_entries = manifest.get("databases")
    require(isinstance(raw_entries, list), "manifest databases must be a list")
    require(len(raw_entries) == len(EXPECTED_DATABASES), "manifest must contain exactly four databases")

    names: list[str] = []
    total_bytes = 0
    references: list[str] = []
    for index, raw_entry in enumerate(raw_entries):
        require(isinstance(raw_entry, dict), "database entry must be an object")
        database = raw_entry.get("database")
        filename = raw_entry.get("file")
        size_bytes = raw_entry.get("size_bytes")
        checksum = raw_entry.get("sha256")
        require(database == EXPECTED_DATABASES[index], "database order or membership drifted")
        require(filename == f"{database}.sql", f"unexpected dump filename for {database}")
        require(isinstance(size_bytes, int) and size_bytes > 0, f"invalid dump size for {database}")
        require(isinstance(checksum, str) and SHA256_RE.fullmatch(checksum) is not None, f"invalid checksum for {database}")
        path = dumps / filename
        require(path.is_file(), f"missing database dump: {filename}")
        require(path.stat().st_size == size_bytes, f"dump size mismatch for {database}")
        require(sha256_file(path) == checksum, f"dump checksum mismatch for {database}")
        names.append(database)
        total_bytes += size_bytes
        references.append(f"{database}\t{filename}\t{checksum}\t{size_bytes}")

    require(tuple(names) == EXPECTED_DATABASES, "backup database set is not exact")
    require(len(set(names)) == len(names), "backup manifest contains duplicate databases")
    require(manifest["total_bytes"] == total_bytes, "backup total byte count is incorrect")
    return references


def verify_manifest(args: argparse.Namespace) -> None:
    manifest = read_json(args.manifest)
    references = validate_manifest(
        manifest,
        args.dumps,
        args.repository,
        args.source_commit,
    )
    for reference in references:
        print(reference)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Create and verify exact four-database backup manifests")
    subparsers = parser.add_subparsers(dest="command", required=True)

    create = subparsers.add_parser("create")
    create.add_argument("--dumps", type=pathlib.Path, required=True)
    create.add_argument("--repository", required=True)
    create.add_argument("--source-commit", required=True)
    create.add_argument("--mysql-version", required=True)
    create.add_argument("--backup-duration-ms", type=int, required=True)
    create.add_argument("--restore-duration-ms", type=int, required=True)
    create.add_argument("--output", type=pathlib.Path, required=True)
    create.set_defaults(handler=create_manifest)

    verify = subparsers.add_parser("verify")
    verify.add_argument("--manifest", type=pathlib.Path, required=True)
    verify.add_argument("--dumps", type=pathlib.Path, required=True)
    verify.add_argument("--repository", required=True)
    verify.add_argument("--source-commit", required=True)
    verify.set_defaults(handler=verify_manifest)
    return parser


def main() -> int:
    args = build_parser().parse_args()
    args.handler(args)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"backup manifest operation failed: {exc}", file=sys.stderr)
        raise
