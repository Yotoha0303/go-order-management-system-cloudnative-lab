#!/usr/bin/env python3

import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
PUBLISH_WORKFLOW = ROOT / ".github/workflows/publish-images.yml"
CONTRACT_WORKFLOW = ROOT / ".github/workflows/release-contracts.yml"
DOCKERFILE = ROOT / "deploy/docker/Dockerfile.service"
MANIFEST_TOOL = ROOT / "scripts/release/manifest.py"
MANIFEST_TEST = ROOT / "scripts/release/test_manifest.py"
RELEASE_DOC = ROOT / "docs/verification/ghcr-release-images.md"

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
    require(path.is_file(), f"required release file is missing: {path.relative_to(ROOT)}")
    return path.read_text(encoding="utf-8")


def validate_publish_workflow() -> None:
    workflow = read_text(PUBLISH_WORKFLOW)
    require("workflow_dispatch:" in workflow, "image publishing must support an explicit manual run")
    require('tags:\n      - "release-*"' in workflow, "image publishing tag trigger must remain release-*")
    require("pull_request:" not in workflow, "package publishing must never run in pull-request context")
    require("permissions:\n  contents: read" in workflow, "top-level workflow permissions must remain read-only")
    require(workflow.count("packages: write") == 1, "packages: write must exist only on the publish job")
    require("packages: read" in workflow, "preflight and manifest verification need explicit package read access")
    require("github.ref == 'refs/heads/main'" in workflow, "manual publishing must be restricted to main")
    require("cancel-in-progress: false" in workflow, "release publishing must not cancel an active run")

    required_actions = (
        "actions/checkout@v4",
        "docker/setup-buildx-action@v3",
        "docker/login-action@v3",
        "docker/build-push-action@v6",
        "actions/upload-artifact@v4",
        "actions/download-artifact@v4",
    )
    for action in required_actions:
        require(action in workflow, f"publishing workflow is missing pinned action major: {action}")

    required_contracts = (
        "sha-${GITHUB_SHA}",
        "docker buildx imagetools inspect",
        "immutable image tag already exists; refusing overwrite",
        "push: true",
        "provenance: mode=max",
        "sbom: true",
        "steps.build.outputs.digest",
        "release-fragment-*",
        "merge-multiple: true",
        "release-manifest.json",
        "release-references.txt",
        "scripts/release/manifest.py assemble",
        "scripts/release/manifest.py verify",
    )
    for contract in required_contracts:
        require(contract in workflow, f"publishing workflow is missing contract: {contract}")

    require(":latest" not in workflow, "mutable latest tag is forbidden")
    require("secrets.GITHUB_TOKEN" in workflow, "GHCR login must use the scoped GitHub token")
    require("password:" in workflow and "username:" in workflow, "GHCR login configuration is incomplete")

    for service in EXPECTED_SERVICES:
        occurrences = len(re.findall(rf"^\s*-?\s*{re.escape(service)}\s*$", workflow, re.MULTILINE))
        require(occurrences >= 2, f"service must appear in preflight and publish matrix: {service}")


def validate_contract_workflow() -> None:
    workflow = read_text(CONTRACT_WORKFLOW)
    require("pull_request:" in workflow, "release contracts must run for pull requests")
    require("branches: [\"main\"]" in workflow, "release contracts must target main")
    require("permissions:\n  contents: read" in workflow, "release contract workflow must remain read-only")
    require("packages: write" not in workflow, "release contract workflow must never receive package write access")
    require("python3 -m unittest" in workflow, "release manifest unit tests are not wired into CI")
    require("scripts/verify/release-contracts.py" in workflow, "static release contract check is not wired into CI")


def validate_dockerfile() -> None:
    dockerfile = read_text(DOCKERFILE)
    require("ARG SERVICE" in dockerfile, "shared service Dockerfile must retain the SERVICE build argument")
    require('"./cmd/${SERVICE}"' in dockerfile, "shared service Dockerfile must build the selected cmd target")
    require("CGO_ENABLED=0 GOOS=linux" in dockerfile, "release binary must remain a static Linux build")
    require("USER app" in dockerfile, "release image must run as the non-root app user")
    require("CMD [\"./service\"]" in dockerfile, "release image entrypoint contract drifted")


def validate_release_files() -> None:
    manifest_tool = read_text(MANIFEST_TOOL)
    manifest_test = read_text(MANIFEST_TEST)
    documentation = read_text(RELEASE_DOC)

    require("EXPECTED_SERVICES" in manifest_tool, "manifest tool must own the exact workload set")
    require("sha256:" in manifest_tool, "manifest tool must validate OCI sha256 digests")
    require("sha-{commit_sha}" in manifest_tool, "manifest tool must bind tags to the source commit")
    require("manifest image order or service set drifted" in manifest_tool, "manifest order must be deterministic")
    require("test_assemble_and_verify_exact_release" in manifest_test, "happy-path manifest test is missing")
    require("test_missing_service_is_rejected" in manifest_test, "missing-image manifest test is missing")
    require("test_duplicate_service_is_rejected" in manifest_test, "duplicate-image manifest test is missing")
    require("test_invalid_digest_is_rejected" in manifest_test, "invalid-digest manifest test is missing")
    require("同一 commit-SHA 标签" in documentation, "release documentation must explain immutable tag behavior")
    require("部分发布" in documentation, "release documentation must disclose partial-publication recovery")
    require("不执行 Kubernetes 部署" in documentation, "release documentation must preserve the phase boundary")


def main() -> int:
    validate_publish_workflow()
    validate_contract_workflow()
    validate_dockerfile()
    validate_release_files()
    print("Immutable GHCR publishing and release manifest contracts verified")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001 - command boundary
        print(f"Release contract verification failed: {exc}", file=sys.stderr)
        raise
