#!/usr/bin/env python3

import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / ".github/workflows/deploy-test-release.yml"
CONTRACT_WORKFLOW = ROOT / ".github/workflows/delivery-contracts.yml"
DEPLOYMENT_TOOL = ROOT / "scripts/release/deployment.py"
DEPLOYMENT_TEST = ROOT / "scripts/release/test_deployment.py"
DOCUMENTATION = ROOT / "docs/verification/test-environment-delivery.md"
BASE_KUSTOMIZATION = ROOT / "deploy/kubernetes/base/kustomization.yaml"
MIGRATION_RETENTION = ROOT / "deploy/kubernetes/base/migration-evidence-retention.yaml"

EXPECTED_SERVICES = (
    "api-gateway",
    "identity-service",
    "catalog-service",
    "inventory-service",
    "order-service",
    "order-timeout-worker",
    "order-reconciliation-worker",
)


def require(condition: bool, message: str) -> None:
    if not condition:
        raise RuntimeError(message)


def read_text(path: pathlib.Path) -> str:
    require(path.is_file(), f"required delivery file is missing: {path.relative_to(ROOT)}")
    return path.read_text(encoding="utf-8")


def verify_deployment_workflow() -> None:
    workflow = read_text(WORKFLOW)
    require('workflows: ["Publish Immutable Images"]' in workflow, "trusted release workflow trigger is missing")
    require("workflow_dispatch:" in workflow, "manual trusted acceptance entry is missing")
    require("pull_request:" not in workflow, "untrusted pull requests must never deploy a test release")
    require("permissions:\n  contents: read" in workflow, "top-level permissions must remain read-only")
    require(workflow.count("packages: read") == 1, "package read permission must exist only on the deployment job")
    require(workflow.count("actions: read") == 1, "cross-workflow artifact read permission must be isolated")
    require(workflow.count("issues: write") == 1, "issue write permission must exist only on the evidence job")
    require("github.event.workflow_run.conclusion == 'success'" in workflow, "failed image publication must not deploy")
    require("github.event.workflow_run.head_branch == 'main'" in workflow, "only protected main releases may deploy")
    require("actions/download-artifact@v4" in workflow, "release manifest must be downloaded through the pinned artifact action")
    require("run-id: ${{ steps.source.outputs.source_run_id }}" in workflow, "source workflow run ID is not bound")
    require("release-manifest-${{ steps.source.outputs.release_commit }}" in workflow, "release artifact must be commit-bound")
    require("scripts/release/manifest.py verify" in workflow, "release manifest validation is missing")
    require("scripts/release/deployment.py render" in workflow, "digest render step is missing")
    require(workflow.count("scripts/release/deployment.py verify-inventory") == 2, "inventory must be verified before and after rollback")
    require("kind create cluster" in workflow, "disposable kind cluster creation is missing")
    require("if: always()\n        shell: bash\n        run: kind delete cluster" in workflow, "kind cluster deletion must run unconditionally")
    require("scripts/smoke/microservices-saga-kubernetes.sh" in workflow, "post-deployment Saga smoke is missing")
    require(workflow.count("scripts/smoke/microservices-saga-kubernetes.sh") == 2, "Saga smoke must run before and after rollback")
    require("deliberately bad release unexpectedly succeeded" in workflow, "bounded bad-release proof is missing")
    require("bad_rollout=failed_as_expected" in workflow, "bad rollout evidence is missing")
    require("rollback=accepted_digest_set_restored" in workflow, "rollback evidence is missing")
    require('ACCEPTANCE_ISSUE_NUMBER: "48"' in workflow, "Phase 8.2 evidence target is missing")
    require("no project content was deployed outside GitHub infrastructure" in workflow, "GitHub-only delivery boundary is missing")
    require(":latest" not in workflow, "mutable latest references are forbidden")
    for service in EXPECTED_SERVICES:
        require(service in workflow, f"deployment workflow is missing service: {service}")


def verify_contract_workflow() -> None:
    workflow = read_text(CONTRACT_WORKFLOW)
    require("pull_request:" in workflow, "delivery contracts must run on pull requests")
    require("permissions:\n  contents: read" in workflow, "delivery contracts must remain read-only")
    require("packages: read" not in workflow and "packages: write" not in workflow, "contract verification must not access packages")
    require("issues: write" not in workflow, "contract verification must not write issues")
    require("python3 -m unittest" in workflow, "deployment unit tests are not wired")
    require("scripts/verify/delivery-contracts.py" in workflow, "static delivery verification is not wired")
    for path in (
        "deploy/kubernetes/base/kustomization.yaml",
        "deploy/kubernetes/base/migration-evidence-retention.yaml",
    ):
        require(workflow.count(path) == 2, f"delivery contracts must track {path}")


def verify_tooling_and_docs() -> None:
    tool = read_text(DEPLOYMENT_TOOL)
    tests = read_text(DEPLOYMENT_TEST)
    docs = read_text(DOCUMENTATION)
    base = read_text(BASE_KUSTOMIZATION)
    retention = read_text(MIGRATION_RETENTION)
    require("EXPECTED_RENDER_COUNTS" in tool, "rendering must enforce exact application occurrence counts")
    require("verify_inventory" in tool, "deployed inventory verification is missing")
    require("DIGEST_REFERENCE_RE" in tool, "deployment references must be digest-qualified")
    require("accepted_references" in tool, "all application containers must be restricted to accepted references")
    require("test_render_replaces_exact_application_occurrences" in tests, "render happy-path test is missing")
    require("test_verify_inventory_accepts_exact_deployments_and_migrations" in tests, "inventory happy-path test is missing")
    require("test_verify_inventory_rejects_mutable_application_image" in tests, "mutable-image rejection test is missing")
    require(
        "test_verify_inventory_rejects_unaccepted_application_sidecar_digest" in tests,
        "unaccepted application sidecar rejection test is missing",
    )
    require(
        "migration-evidence-retention.yaml" in base,
        "base Kustomization must retain migration evidence through rollback verification",
    )
    require(
        retention.count("ttlSecondsAfterFinished: 3600") == 4,
        "all four migration Jobs must remain available for the bounded delivery run",
    )
    for phrase in (
        "GitHub Actions 内的一次性 kind 集群",
        "不会发布到 GitHub 之外",
        "精确 OCI digest",
        "错误版本",
        "完整 Order Saga",
        "数据库 schema 回滚",
    ):
        require(phrase in docs, f"delivery documentation is missing boundary: {phrase}")


def main() -> int:
    verify_deployment_workflow()
    verify_contract_workflow()
    verify_tooling_and_docs()
    print("Digest-pinned disposable test delivery contracts verified")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"Delivery contract verification failed: {exc}", file=sys.stderr)
        raise
