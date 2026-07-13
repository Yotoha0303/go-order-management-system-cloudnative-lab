#!/usr/bin/env python3

import argparse
import json
import pathlib
import sys
import tempfile
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))

import deployment
import manifest

COMMIT = "0123456789abcdef0123456789abcdef01234567"
REPOSITORY = "Yotoha0303/go-order-management-system-cloudnative-lab"
CREATED_AT = "2026-07-13T00:00:00+00:00"


def release_document() -> dict:
    images = []
    for index, service in enumerate(manifest.EXPECTED_SERVICES, start=1):
        image = f"ghcr.io/yotoha0303/go-order-management-system-cloudnative-lab-{service}"
        digest = "sha256:" + format(index, "064x")
        images.append(
            {
                "service": service,
                "image": image,
                "tag": f"sha-{COMMIT}",
                "digest": digest,
                "reference": f"{image}@{digest}",
                "commit_sha": COMMIT,
            }
        )
    return {
        "schema_version": 1,
        "repository": REPOSITORY,
        "commit_sha": COMMIT,
        "created_at": CREATED_AT,
        "images": images,
    }


def workload(name: str, container_name: str, image: str) -> dict:
    return {
        "metadata": {"name": name},
        "spec": {
            "template": {
                "spec": {"containers": [{"name": container_name, "image": image}]}
            }
        },
    }


def write_inventory_files(
    root: pathlib.Path,
    deployments: dict,
    jobs: dict,
) -> tuple[pathlib.Path, pathlib.Path]:
    deployments_path = root / "deployments.json"
    jobs_path = root / "jobs.json"
    deployments_path.write_text(json.dumps(deployments), encoding="utf-8")
    jobs_path.write_text(json.dumps(jobs), encoding="utf-8")
    return deployments_path, jobs_path


class DeploymentReleaseTest(unittest.TestCase):
    def write_manifest(self, root: pathlib.Path) -> pathlib.Path:
        path = root / "release-manifest.json"
        path.write_text(json.dumps(release_document()), encoding="utf-8")
        return path

    def exact_inventory(self) -> tuple[dict, dict]:
        document = release_document()
        references = {entry["service"]: entry["reference"] for entry in document["images"]}
        deployments = {
            "kind": "List",
            "items": [
                workload(service, service, references[service])
                for service in manifest.EXPECTED_SERVICES
            ],
        }
        jobs = {
            "kind": "List",
            "items": [
                workload(job, "migrate", references[service])
                for job, service in deployment.MIGRATION_SERVICES.items()
            ],
        }
        return deployments, jobs

    def test_render_replaces_exact_application_occurrences(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            manifest_path = self.write_manifest(root)
            source = root / "rendered-local.yaml"
            output = root / "rendered-release.yaml"
            lines = []
            for service in manifest.EXPECTED_SERVICES:
                for _ in range(deployment.EXPECTED_RENDER_COUNTS[service]):
                    lines.append(f"image: go-order-management-system/{service}:local")
            source.write_text("\n".join(lines) + "\n", encoding="utf-8")

            deployment.render_release(
                argparse.Namespace(
                    manifest=manifest_path,
                    repository=REPOSITORY,
                    commit=COMMIT,
                    input=source,
                    output=output,
                )
            )
            rendered = output.read_text(encoding="utf-8")
            self.assertNotIn("go-order-management-system/", rendered)
            for entry in release_document()["images"]:
                self.assertEqual(
                    rendered.count(entry["reference"]),
                    deployment.EXPECTED_RENDER_COUNTS[entry["service"]],
                )

    def test_render_rejects_missing_workload_reference(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            manifest_path = self.write_manifest(root)
            source = root / "rendered-local.yaml"
            source.write_text("image: go-order-management-system/api-gateway:local\n", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "expected 2 rendered occurrences for identity-service"):
                deployment.render_release(
                    argparse.Namespace(
                        manifest=manifest_path,
                        repository=REPOSITORY,
                        commit=COMMIT,
                        input=source,
                        output=root / "out.yaml",
                    )
                )

    def test_verify_inventory_accepts_exact_deployments_and_migrations(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            manifest_path = self.write_manifest(root)
            deployments, jobs = self.exact_inventory()
            deployments_path, jobs_path = write_inventory_files(root, deployments, jobs)
            output = root / "inventory.json"

            deployment.verify_inventory(
                argparse.Namespace(
                    manifest=manifest_path,
                    repository=REPOSITORY,
                    commit=COMMIT,
                    deployments=deployments_path,
                    jobs=jobs_path,
                    output=output,
                )
            )
            inventory = json.loads(output.read_text(encoding="utf-8"))
            self.assertEqual(inventory["commit_sha"], COMMIT)
            self.assertEqual(len(inventory["deployments"]), 7)
            self.assertEqual(len(inventory["migrations"]), 4)

    def test_verify_inventory_rejects_mutable_application_image(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            manifest_path = self.write_manifest(root)
            deployments, jobs = self.exact_inventory()
            deployments["items"][0]["spec"]["template"]["spec"]["containers"][0]["image"] = (
                "ghcr.io/yotoha0303/go-order-management-system-cloudnative-lab-api-gateway:latest"
            )
            deployments_path, jobs_path = write_inventory_files(root, deployments, jobs)

            with self.assertRaisesRegex(ValueError, "does not match the accepted digest"):
                deployment.verify_inventory(
                    argparse.Namespace(
                        manifest=manifest_path,
                        repository=REPOSITORY,
                        commit=COMMIT,
                        deployments=deployments_path,
                        jobs=jobs_path,
                        output=root / "inventory.json",
                    )
                )

    def test_verify_inventory_rejects_unaccepted_application_sidecar_digest(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            manifest_path = self.write_manifest(root)
            deployments, jobs = self.exact_inventory()
            unexpected_image = (
                "ghcr.io/yotoha0303/"
                "go-order-management-system-cloudnative-lab-api-gateway@sha256:"
                + "f" * 64
            )
            deployments["items"][0]["spec"]["template"]["spec"]["containers"].append(
                {"name": "unaccepted-sidecar", "image": unexpected_image}
            )
            deployments_path, jobs_path = write_inventory_files(root, deployments, jobs)

            with self.assertRaisesRegex(ValueError, "not present in accepted release manifest"):
                deployment.verify_inventory(
                    argparse.Namespace(
                        manifest=manifest_path,
                        repository=REPOSITORY,
                        commit=COMMIT,
                        deployments=deployments_path,
                        jobs=jobs_path,
                        output=root / "inventory.json",
                    )
                )


if __name__ == "__main__":
    unittest.main()
