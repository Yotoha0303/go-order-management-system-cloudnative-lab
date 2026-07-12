#!/usr/bin/env python3

import argparse
import datetime as dt
import json
import pathlib
import re
import sys
from typing import Any

EXPECTED_SERVICES = (
    "api-gateway",
    "identity-service",
    "catalog-service",
    "inventory-service",
    "order-service",
    "order-timeout-worker",
    "order-reconciliation-worker",
)

COMMIT_RE = re.compile(r"^[0-9a-f]{40}$")
DIGEST_RE = re.compile(r"^sha256:[0-9a-f]{64}$")
REPOSITORY_RE = re.compile(r"^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def read_json(path: pathlib.Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    require(isinstance(value, dict), f"{path} must contain a JSON object")
    return value


def write_json(path: pathlib.Path, value: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def validate_commit(commit_sha: str) -> None:
    require(bool(COMMIT_RE.fullmatch(commit_sha)), "commit SHA must be 40 lowercase hexadecimal characters")


def validate_created_at(created_at: str) -> None:
    require(bool(created_at), "created_at must not be empty")
    parsed = dt.datetime.fromisoformat(created_at.replace("Z", "+00:00"))
    require(parsed.tzinfo is not None, "created_at must include a timezone")


def validate_image(image: str) -> None:
    require(image == image.lower(), "GHCR image path must be lowercase")
    require(image.startswith("ghcr.io/"), "image must use ghcr.io")
    require("@" not in image, "image field must not include a digest")
    require(":" not in image.removeprefix("ghcr.io/"), "image field must not include a tag")
    require(image.count("/") >= 2, "image must include owner and package path")


def validate_fragment(fragment: dict[str, Any], expected_commit: str | None = None) -> dict[str, str]:
    required_keys = {"service", "image", "tag", "digest", "reference", "commit_sha"}
    require(set(fragment) == required_keys, f"fragment keys must be exactly {sorted(required_keys)}")

    normalized = {key: str(fragment[key]) for key in required_keys}
    service = normalized["service"]
    image = normalized["image"]
    tag = normalized["tag"]
    digest = normalized["digest"]
    reference = normalized["reference"]
    commit_sha = normalized["commit_sha"]

    require(service in EXPECTED_SERVICES, f"unexpected service: {service}")
    validate_commit(commit_sha)
    if expected_commit is not None:
        require(commit_sha == expected_commit, f"fragment commit mismatch for {service}")
    validate_image(image)
    require(tag == f"sha-{commit_sha}", f"immutable tag mismatch for {service}")
    require(bool(DIGEST_RE.fullmatch(digest)), f"invalid OCI digest for {service}")
    require(reference == f"{image}@{digest}", f"digest-qualified reference mismatch for {service}")
    require(":latest" not in reference, "latest tag is forbidden")
    return normalized


def create_fragment(args: argparse.Namespace) -> None:
    validate_commit(args.commit)
    fragment = {
        "service": args.service,
        "image": args.image,
        "tag": args.tag,
        "digest": args.digest,
        "reference": f"{args.image}@{args.digest}",
        "commit_sha": args.commit,
    }
    validate_fragment(fragment, args.commit)
    write_json(args.output, fragment)


def assemble_manifest(args: argparse.Namespace) -> None:
    validate_commit(args.commit)
    require(bool(REPOSITORY_RE.fullmatch(args.repository)), "repository must use owner/name form")
    validate_created_at(args.created_at)

    paths = sorted(args.input_dir.glob("release-fragment-*.json"))
    require(paths, f"no release fragments found in {args.input_dir}")

    fragments: dict[str, dict[str, str]] = {}
    for path in paths:
        fragment = validate_fragment(read_json(path), args.commit)
        service = fragment["service"]
        require(service not in fragments, f"duplicate release fragment for {service}")
        fragments[service] = fragment

    missing = sorted(set(EXPECTED_SERVICES) - set(fragments))
    unexpected = sorted(set(fragments) - set(EXPECTED_SERVICES))
    require(not missing, f"missing release fragments: {missing}")
    require(not unexpected, f"unexpected release fragments: {unexpected}")
    require(len(paths) == len(EXPECTED_SERVICES), "release fragment file count does not match service count")

    manifest = {
        "schema_version": 1,
        "repository": args.repository,
        "commit_sha": args.commit,
        "created_at": args.created_at,
        "images": [fragments[service] for service in EXPECTED_SERVICES],
    }
    validate_manifest(manifest, args.repository, args.commit)
    write_json(args.output, manifest)


def validate_manifest(
    manifest: dict[str, Any], expected_repository: str | None = None, expected_commit: str | None = None
) -> list[str]:
    required_keys = {"schema_version", "repository", "commit_sha", "created_at", "images"}
    require(set(manifest) == required_keys, f"manifest keys must be exactly {sorted(required_keys)}")
    require(manifest["schema_version"] == 1, "unsupported manifest schema version")

    repository = str(manifest["repository"])
    commit_sha = str(manifest["commit_sha"])
    created_at = str(manifest["created_at"])
    require(bool(REPOSITORY_RE.fullmatch(repository)), "manifest repository must use owner/name form")
    validate_commit(commit_sha)
    validate_created_at(created_at)
    if expected_repository is not None:
        require(repository == expected_repository, "manifest repository mismatch")
    if expected_commit is not None:
        require(commit_sha == expected_commit, "manifest commit mismatch")

    images = manifest["images"]
    require(isinstance(images, list), "manifest images must be a list")
    require(len(images) == len(EXPECTED_SERVICES), "manifest must contain exactly seven images")

    services: list[str] = []
    references: list[str] = []
    for raw_fragment in images:
        require(isinstance(raw_fragment, dict), "manifest image entries must be objects")
        fragment = validate_fragment(raw_fragment, commit_sha)
        services.append(fragment["service"])
        references.append(fragment["reference"])

    require(tuple(services) == EXPECTED_SERVICES, "manifest image order or service set drifted")
    require(len(references) == len(set(references)), "manifest contains duplicate digest-qualified references")
    return references


def verify_manifest(args: argparse.Namespace) -> None:
    manifest = read_json(args.manifest)
    references = validate_manifest(manifest, args.repository, args.commit)
    for reference in references:
        print(reference)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Create and validate immutable GHCR release manifests")
    subparsers = parser.add_subparsers(dest="command", required=True)

    fragment = subparsers.add_parser("fragment", help="create one service image fragment")
    fragment.add_argument("--service", required=True)
    fragment.add_argument("--image", required=True)
    fragment.add_argument("--tag", required=True)
    fragment.add_argument("--digest", required=True)
    fragment.add_argument("--commit", required=True)
    fragment.add_argument("--output", type=pathlib.Path, required=True)
    fragment.set_defaults(handler=create_fragment)

    assemble = subparsers.add_parser("assemble", help="assemble exactly seven fragments")
    assemble.add_argument("--input-dir", type=pathlib.Path, required=True)
    assemble.add_argument("--output", type=pathlib.Path, required=True)
    assemble.add_argument("--repository", required=True)
    assemble.add_argument("--commit", required=True)
    assemble.add_argument("--created-at", required=True)
    assemble.set_defaults(handler=assemble_manifest)

    verify = subparsers.add_parser("verify", help="validate a release manifest and print immutable references")
    verify.add_argument("--manifest", type=pathlib.Path, required=True)
    verify.add_argument("--repository", required=True)
    verify.add_argument("--commit", required=True)
    verify.set_defaults(handler=verify_manifest)
    return parser


def main() -> int:
    args = build_parser().parse_args()
    args.handler(args)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001 - command boundary
        print(f"release manifest operation failed: {exc}", file=sys.stderr)
        raise
