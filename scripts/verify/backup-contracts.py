#!/usr/bin/env python3

import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
RUNTIME_WORKFLOW = ROOT / ".github/workflows/mysql-backup-restore.yml"
CONTRACT_WORKFLOW = ROOT / ".github/workflows/backup-contracts.yml"
MANIFEST_TOOL = ROOT / "scripts/backup/manifest.py"
MANIFEST_TEST = ROOT / "scripts/backup/test_manifest.py"
RESTORE_SCRIPT = ROOT / "scripts/backup/run-backup-restore.sh"
DOCUMENTATION = ROOT / "docs/verification/mysql-backup-restore.md"

EXPECTED_DATABASES = (
    "go_order_identity",
    "go_order_catalog",
    "go_order_inventory",
    "go_order_ordering",
)


def require(condition: bool, message: str) -> None:
    if not condition:
        raise RuntimeError(message)


def read_text(path: pathlib.Path) -> str:
    require(path.is_file(), f"required backup file is missing: {path.relative_to(ROOT)}")
    return path.read_text(encoding="utf-8")


def verify_runtime_workflow() -> None:
    workflow = read_text(RUNTIME_WORKFLOW)
    require("workflow_dispatch:" in workflow, "manual backup verification entry is missing")
    require("pull_request:" not in workflow, "pull requests must not execute backup runtime")
    require("permissions:\n  contents: read" in workflow, "top-level backup permissions must remain read-only")
    require(workflow.count("issues: write") == 1, "issue write permission must be isolated to evidence")
    require("packages: write" not in workflow, "backup verification must not write packages")
    require("scripts/smoke/microservices-saga.sh" in workflow, "synthetic representative data creation is missing")
    require("Quiesce application writers" in workflow, "source writers must be stopped before fingerprinting")
    require("scripts/backup/run-backup-restore.sh" in workflow, "isolated backup/restore execution is missing")
    require("scripts/backup/manifest.py create" in workflow, "backup manifest creation is missing")
    require(workflow.count("scripts/backup/manifest.py verify") == 2, "normal and corrupt manifest verification are required")
    require("corrupt database dump unexpectedly passed verification" in workflow, "corrupt dump negative proof is missing")
    require("actions/upload-artifact@v4" in workflow, "backup evidence artifact is missing")
    require("retention-days: 30" in workflow, "synthetic backup evidence retention must be explicit")
    require("if: always()" in workflow and "docker compose down -v" in workflow, "disposable resources must always be removed")
    require('ACCEPTANCE_ISSUE_NUMBER: "50"' in workflow, "Phase 8.3 evidence issue is missing")
    require("Only synthetic CI data was stored" in workflow, "synthetic-data boundary is missing")
    for database in EXPECTED_DATABASES:
        require(database in workflow or database in read_text(RESTORE_SCRIPT), f"backup flow is missing database: {database}")


def verify_contract_workflow() -> None:
    workflow = read_text(CONTRACT_WORKFLOW)
    require("pull_request:" in workflow, "backup contracts must run on pull requests")
    require("permissions:\n  contents: read" in workflow, "backup contracts must be read-only")
    require("issues: write" not in workflow, "backup contracts must not write issues")
    require("packages: read" not in workflow and "packages: write" not in workflow, "backup contracts must not access packages")
    require("python3 -m unittest" in workflow, "backup unit tests are not wired")
    require("bash -n scripts/backup/run-backup-restore.sh" in workflow, "backup shell syntax validation is missing")
    require("scripts/verify/backup-contracts.py" in workflow, "backup static contract verification is missing")


def verify_tooling_and_documentation() -> None:
    tool = read_text(MANIFEST_TOOL)
    tests = read_text(MANIFEST_TEST)
    restore = read_text(RESTORE_SCRIPT)
    docs = read_text(DOCUMENTATION)
    require("EXPECTED_DATABASES" in tool, "exact database set is not encoded")
    require("sha256_file" in tool and "total_bytes" in tool, "backup integrity metadata is incomplete")
    require("test_missing_dump_is_rejected" in tests, "missing-dump rejection test is absent")
    require("test_corrupt_dump_is_rejected" in tests, "corrupt-dump rejection test is absent")
    require("test_unexpected_or_reordered_database_is_rejected" in tests, "unexpected database rejection test is absent")
    require("--single-transaction" in restore, "logical backup must use a transactional snapshot")
    require("--skip-dump-date" in restore, "logical dumps must avoid volatile timestamps")
    require("MYSQL_PWD" in restore, "password must not be passed as a command-line argument")
    require(restore.count("cmp ") >= 2, "restored and source database equality checks are missing")
    require("trap cleanup EXIT" in restore, "isolated restore cleanup is missing")
    for phrase in (
        "四个服务数据库",
        "隔离恢复",
        "SHA-256",
        "源数据库保持不变",
        "仅允许合成测试数据",
        "RPO",
        "RTO",
        "不等同于生产级灾难恢复",
    ):
        require(phrase in docs, f"backup documentation is missing boundary: {phrase}")


def main() -> int:
    verify_runtime_workflow()
    verify_contract_workflow()
    verify_tooling_and_documentation()
    print("Four-database backup and isolated restore contracts verified")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"Backup contract verification failed: {exc}", file=sys.stderr)
        raise
