#!/usr/bin/env python3

import pathlib

ROOT = pathlib.Path(__file__).resolve().parents[2]
RUNTIME = ROOT / ".github/workflows/fault-drills.yml"
CONTRACTS = ROOT / ".github/workflows/fault-drill-contracts.yml"
HTTP_TEST = ROOT / "internal/platform/resiliencehttp/fault_drill_test.go"
WORKER_TEST = ROOT / "internal/ordersvc/fault_drill_test.go"
MIGRATION = ROOT / "scripts/fault-drills/migration-failure.sh"
DOCS = ROOT / "docs/verification/runtime-fault-drills.md"


def require(condition: bool, message: str) -> None:
    if not condition:
        raise RuntimeError(message)


def read(path: pathlib.Path) -> str:
    require(path.is_file(), f"required fault-drill file is missing: {path.relative_to(ROOT)}")
    return path.read_text(encoding="utf-8")


def verify_runtime() -> None:
    workflow = read(RUNTIME)
    require("workflow_dispatch:" in workflow, "manual fault-drill entry is missing")
    require("pull_request:" not in workflow, "pull requests must not execute runtime faults")
    require("permissions:\n  contents: read" in workflow, "top-level permissions must remain read-only")
    require(workflow.count("issues: write") == 1, "issue writes must be isolated to the evidence job")
    require("timeout-minutes: 35" in workflow, "fault workflow must have a bounded job timeout")
    require("stop rabbitmq" in workflow, "RabbitMQ outage injection is missing")
    require("wait-session 0" in workflow and "wait-session 1" in workflow, "RabbitMQ loss/recovery assertions are incomplete")
    require("rabbit-recovery-${GITHUB_RUN_ID}" in workflow, "post-RabbitMQ recovery Saga is missing")
    require("TestFaultDrillHTTPTimeoutCircuitRecovery" in workflow, "HTTP timeout/circuit drill is not executed")
    require("FAULT_DRILL_HTTP_OUTPUT: fault-evidence/http/http-circuit-recovery.json" in workflow, "HTTP evidence target is missing")
    require("TestFaultDrillWorkerLeaseProcess" in workflow, "Worker process crash drill is not executed")
    require("migration-failure.sh" in workflow, "migration failure drill is not executed")
    require("post-fault-drills-${GITHUB_RUN_ID}" in workflow, "final complete Saga is missing")
    require("fault-drill-summary.json" in workflow, "machine-readable drill summary is missing")
    require("actions/upload-artifact@v4" in workflow, "fault evidence artifact is missing")
    require("retention-days: 30" in workflow, "fault evidence retention is not explicit")
    require("if: always()" in workflow and "down -v --remove-orphans" in workflow, "disposable Compose cleanup is missing")
    require("docker rm -fv" in workflow, "isolated migration volume cleanup is missing")
    require('ACCEPTANCE_ISSUE_NUMBER: "51"' in workflow, "Phase 8.4 evidence target is missing")
    require("All faults were bounded to disposable GitHub-hosted resources" in workflow, "GitHub-only fault boundary is missing")


def verify_contract_workflow() -> None:
    workflow = read(CONTRACTS)
    require("pull_request:" in workflow, "fault contracts must run on pull requests")
    require("permissions:\n  contents: read" in workflow, "fault contracts must be read-only")
    require("issues: write" not in workflow, "fault contracts must not write issues")
    require("packages: read" not in workflow and "packages: write" not in workflow, "fault contracts must not access packages")
    require("go test ./internal/platform/resiliencehttp" in workflow, "HTTP drill contract test is missing")
    require("FaultDrillHTTPTimeoutCircuitRecovery|WriteFaultDrillEvidenceAnchorsRelativePathToWorkspace" in workflow, "HTTP behavior and workspace-path regressions must both execute")
    require("go test ./internal/ordersvc" in workflow, "Worker drill contract test is missing")
    require("bash -n scripts/fault-drills/migration-failure.sh" in workflow, "migration drill shell validation is missing")
    require("scripts/verify/fault-drill-contracts.py" in workflow, "static fault contract verification is missing")


def verify_drill_implementations() -> None:
    http_test = read(HTTP_TEST)
    worker_test = read(WORKER_TEST)
    migration = read(MIGRATION)
    require("httptest.NewServer" in http_test, "HTTP drill must use a real local HTTP server")
    require("ResponseHeaderTimeout" in http_test and "ErrCircuitOpen" in http_test, "HTTP timeout/open assertions are incomplete")
    require("network I/O" in http_test and "half-open recovery" in http_test, "HTTP open/half-open evidence is incomplete")
    require('os.Getenv("GITHUB_WORKSPACE")' in http_test, "relative HTTP evidence must anchor to the Actions workspace")
    require("filepath.IsAbs" in http_test and "filepath.Join(workspace, outputPath)" in http_test, "workspace evidence-path resolution is incomplete")
    require("os.MkdirAll(filepath.Dir(outputPath)" in http_test, "HTTP evidence parent directory must be created")
    require("TestWriteFaultDrillEvidenceAnchorsRelativePathToWorkspace" in http_test, "workspace evidence-path regression test is missing")
    require("exec.Command(os.Args[0]" in worker_test, "Worker owner must run as a separate process")
    require("helper.Process.Kill()" in worker_test, "Worker process termination is missing")
    require("recovery.claimPending" in worker_test, "expired lease reclamation is missing")
    require("duplicate outbox rows" in worker_test, "duplicate-row assertion is missing")
    require("docker run -d" in migration and "--tmpfs /var/lib/mysql" in migration, "migration database is not isolated and disposable")
    require("DELIBERATELY INVALID SQL" in migration, "invalid migration fixture is missing")
    require("unexpectedly succeeded" in migration, "migration failure expectation is missing")
    require("promotion-approved" in migration, "promotion-block assertion is missing")
    require(migration.count("goose -dir") >= 2, "normal migration validation is missing")
    require("trap cleanup EXIT" in migration and "docker rm -fv" in migration, "migration cleanup is incomplete")


def verify_docs() -> None:
    docs = read(DOCS)
    for phrase in (
        "RabbitMQ 断连与恢复",
        "HTTP 超时与熔断",
        "Worker 进程崩溃与租约回收",
        "迁移失败隔离",
        "检测信号",
        "预期影响",
        "恢复动作",
        "恢复验证",
        "一次性 GitHub Actions",
        "不等同于生产级混沌工程",
    ):
        require(phrase in docs, f"fault-drill documentation is missing: {phrase}")


def main() -> int:
    verify_runtime()
    verify_contract_workflow()
    verify_drill_implementations()
    verify_docs()
    print("Bounded runtime fault-drill contracts verified")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"Fault-drill contract verification failed: {exc}")
        raise
