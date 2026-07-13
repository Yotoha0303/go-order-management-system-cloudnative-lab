#!/usr/bin/env python3

import argparse
import json
import pathlib
import re
import sys
from typing import Any

SCRIPT_DIR = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(SCRIPT_DIR))

import manifest as release_manifest

LOCAL_IMAGE_PREFIX = "go-order-management-system/"
EXPECTED_RENDER_COUNTS = {
    "api-gateway": 1,
    "identity-service": 2,
    "catalog-service": 2,
    "inventory-service": 2,
    "order-service": 2,
    "order-timeout-worker": 1,
    "order-reconciliation-worker": 1,
}
MIGRATION_SERVICES = {
    "identity-migrate": "identity-service",
    "catalog-migrate": "catalog-service",
    "inventory-migrate": "inventory-service",
    "ordering-migrate": "order-service",
}
DIGEST_REFERENCE_RE = re.compile(
    r"^ghcr\.io/[a-z0-9_.-]+/[a-z0-9_.\-/]+@sha256:[0-9a-f]{64}$"
)


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


def load_release(
    path: pathlib.Path, expected_repository: str, expected_commit: str
) -> tuple[dict[str, Any], dict[str, str]]:
    document = read_json(path)
    release_manifest.validate_manifest(document, expected_repository, expected_commit)
    references: dict[str, str] = {}
    for raw_entry in document["images"]:
        require(isinstance(raw_entry, dict), "release image entries must be objects")
        entry = release_manifest.validate_fragment(raw_entry, expected_commit)
        reference = entry["reference"]
        require(
            bool(DIGEST_REFERENCE_RE.fullmatch(reference)),
            f"invalid deployment reference for {entry['service']}",
        )
        references[entry["service"]] = reference
    require(tuple(references) == release_manifest.EXPECTED_SERVICES, "release service order drifted")
    return document, references


def render_release(args: argparse.Namespace) -> None:
    _, references = load_release(args.manifest, args.repository, args.commit)
    rendered = args.input.read_text(encoding="utf-8")

    for service in release_manifest.EXPECTED_SERVICES:
        local_reference = f"{LOCAL_IMAGE_PREFIX}{service}:local"
        expected_count = EXPECTED_RENDER_COUNTS[service]
        actual_count = rendered.count(local_reference)
        require(
            actual_count == expected_count,
            f"expected {expected_count} rendered occurrences for {service}, found {actual_count}",
        )
        rendered = rendered.replace(local_reference, references[service])

    require(
        not re.search(r"image:\s+go-order-management-system/[a-z0-9-]+:local", rendered),
        "rendered release still contains a local application image",
    )
    for service, reference in references.items():
        require(
            rendered.count(reference) == EXPECTED_RENDER_COUNTS[service],
            f"rendered release does not contain the exact digest count for {service}",
        )

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(rendered, encoding="utf-8")


def release_reference(args: argparse.Namespace) -> None:
    _, references = load_release(args.manifest, args.repository, args.commit)
    require(args.service in references, f"unknown release service: {args.service}")
    print(references[args.service])


def item_map(document: dict[str, Any], kind: str) -> dict[str, dict[str, Any]]:
    require(document.get("kind") == "List", f"{kind} inventory must be a Kubernetes List")
    raw_items = document.get("items")
    require(isinstance(raw_items, list), f"{kind} inventory items must be a list")
    items: dict[str, dict[str, Any]] = {}
    for raw_item in raw_items:
        require(isinstance(raw_item, dict), f"{kind} inventory entries must be objects")
        metadata = raw_item.get("metadata")
        require(isinstance(metadata, dict), f"{kind} inventory entry is missing metadata")
        name = metadata.get("name")
        require(isinstance(name, str) and name, f"{kind} inventory entry is missing a name")
        require(name not in items, f"duplicate {kind} inventory entry: {name}")
        items[name] = raw_item
    return items


def pod_containers(item: dict[str, Any], resource_name: str) -> list[dict[str, Any]]:
    try:
        containers = item["spec"]["template"]["spec"]["containers"]
    except (KeyError, TypeError) as exc:
        raise ValueError(f"{resource_name} is missing pod containers") from exc
    require(isinstance(containers, list), f"{resource_name} containers must be a list")
    normalized: list[dict[str, Any]] = []
    for container in containers:
        require(isinstance(container, dict), f"{resource_name} contains an invalid container")
        normalized.append(container)
    return normalized


def named_container_image(item: dict[str, Any], resource_name: str, container_name: str) -> str:
    matches = [
        container
        for container in pod_containers(item, resource_name)
        if container.get("name") == container_name
    ]
    require(len(matches) == 1, f"{resource_name} must contain exactly one {container_name} container")
    image = matches[0].get("image")
    require(isinstance(image, str) and image, f"{resource_name}/{container_name} is missing an image")
    return image


def reject_unexpected_application_images(
    items: dict[str, dict[str, Any]], resource_kind: str
) -> None:
    for resource_name, item in items.items():
        for container in pod_containers(item, resource_name):
            image = container.get("image")
            if not isinstance(image, str):
                continue
            require(
                not image.startswith(LOCAL_IMAGE_PREFIX),
                f"{resource_kind} {resource_name} still uses local application image {image}",
            )
            if "go-order-management-system-cloudnative-lab-" in image:
                require(
                    bool(DIGEST_REFERENCE_RE.fullmatch(image)),
                    f"{resource_kind} {resource_name} uses a non-digest GHCR application image: {image}",
                )


def verify_inventory(args: argparse.Namespace) -> None:
    document, references = load_release(args.manifest, args.repository, args.commit)
    deployments = item_map(read_json(args.deployments), "Deployment")
    jobs = item_map(read_json(args.jobs), "Job")

    actual_deployments: dict[str, str] = {}
    for service in release_manifest.EXPECTED_SERVICES:
        require(service in deployments, f"missing Deployment: {service}")
        image = named_container_image(
            deployments[service], f"Deployment/{service}", service
        )
        require(
            image == references[service],
            f"Deployment/{service} image does not match the accepted digest",
        )
        actual_deployments[service] = image

    actual_migrations: dict[str, str] = {}
    for job_name, service in MIGRATION_SERVICES.items():
        require(job_name in jobs, f"missing migration Job: {job_name}")
        image = named_container_image(jobs[job_name], f"Job/{job_name}", "migrate")
        require(
            image == references[service],
            f"Job/{job_name} image does not match the accepted digest",
        )
        actual_migrations[job_name] = image

    reject_unexpected_application_images(deployments, "Deployment")
    reject_unexpected_application_images(jobs, "Job")

    write_json(
        args.output,
        {
            "schema_version": 1,
            "repository": document["repository"],
            "commit_sha": document["commit_sha"],
            "deployments": actual_deployments,
            "migrations": actual_migrations,
        },
    )


def add_release_arguments(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--manifest", type=pathlib.Path, required=True)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--commit", required=True)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Render and verify digest-pinned Kubernetes releases"
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    render = subparsers.add_parser(
        "render", help="replace all local application images with manifest digests"
    )
    add_release_arguments(render)
    render.add_argument("--input", type=pathlib.Path, required=True)
    render.add_argument("--output", type=pathlib.Path, required=True)
    render.set_defaults(handler=render_release)

    reference = subparsers.add_parser(
        "reference", help="print one digest-qualified service reference"
    )
    add_release_arguments(reference)
    reference.add_argument("--service", required=True)
    reference.set_defaults(handler=release_reference)

    inventory = subparsers.add_parser(
        "verify-inventory", help="verify deployed application and migration images"
    )
    add_release_arguments(inventory)
    inventory.add_argument("--deployments", type=pathlib.Path, required=True)
    inventory.add_argument("--jobs", type=pathlib.Path, required=True)
    inventory.add_argument("--output", type=pathlib.Path, required=True)
    inventory.set_defaults(handler=verify_inventory)
    return parser


def main() -> int:
    args = build_parser().parse_args()
    args.handler(args)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"release deployment operation failed: {exc}", file=sys.stderr)
        raise
