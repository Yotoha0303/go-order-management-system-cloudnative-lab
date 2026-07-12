#!/usr/bin/env python3

import argparse
import json
import pathlib
import sys
import tempfile
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))

import manifest  # noqa: E402


COMMIT = "0123456789abcdef0123456789abcdef01234567"
DIGESTS = {
    service: "sha256:" + format(index + 1, "064x")
    for index, service in enumerate(manifest.EXPECTED_SERVICES)
}
REPOSITORY = "Yotoha0303/go-order-management-system-cloudnative-lab"
CREATED_AT = "2026-07-12T11:36:41+00:00"


class ReleaseManifestTest(unittest.TestCase):
    def create_fragment(self, directory: pathlib.Path, service: str, *, digest: str | None = None) -> pathlib.Path:
        image = f"ghcr.io/yotoha0303/go-order-management-system-cloudnative-lab-{service}"
        output = directory / f"release-fragment-{service}.json"
        args = argparse.Namespace(
            service=service,
            image=image,
            tag=f"sha-{COMMIT}",
            digest=digest or DIGESTS[service],
            commit=COMMIT,
            output=output,
        )
        manifest.create_fragment(args)
        return output

    def create_all_fragments(self, directory: pathlib.Path) -> None:
        for service in manifest.EXPECTED_SERVICES:
            self.create_fragment(directory, service)

    def assemble_exact_release(self, root: pathlib.Path) -> dict:
        fragments = root / "fragments"
        fragments.mkdir()
        self.create_all_fragments(fragments)
        output = root / "release-manifest.json"
        manifest.assemble_manifest(
            argparse.Namespace(
                input_dir=fragments,
                output=output,
                repository=REPOSITORY,
                commit=COMMIT,
                created_at=CREATED_AT,
            )
        )
        return json.loads(output.read_text(encoding="utf-8"))

    def test_assemble_and_verify_exact_release(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            document = self.assemble_exact_release(pathlib.Path(temp))
            references = manifest.validate_manifest(document, REPOSITORY, COMMIT)
            self.assertEqual(document["schema_version"], 1)
            self.assertEqual(
                [entry["service"] for entry in document["images"]],
                list(manifest.EXPECTED_SERVICES),
            )
            self.assertEqual(len(references), 7)
            self.assertTrue(all("@sha256:" in reference for reference in references))

    def test_tagged_references_match_commit_tags(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            document = self.assemble_exact_release(pathlib.Path(temp))
            references = manifest.tagged_references(document, REPOSITORY, COMMIT)
            self.assertEqual(len(references), 7)
            self.assertEqual(
                references,
                [
                    f"ghcr.io/yotoha0303/go-order-management-system-cloudnative-lab-{service}:sha-{COMMIT}"
                    for service in manifest.EXPECTED_SERVICES
                ],
            )

    def test_missing_service_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            for service in manifest.EXPECTED_SERVICES[:-1]:
                self.create_fragment(root, service)

            with self.assertRaisesRegex(ValueError, "missing release fragments"):
                manifest.assemble_manifest(
                    argparse.Namespace(
                        input_dir=root,
                        output=root / "release-manifest.json",
                        repository=REPOSITORY,
                        commit=COMMIT,
                        created_at=CREATED_AT,
                    )
                )

    def test_duplicate_service_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            self.create_all_fragments(root)
            duplicate = json.loads((root / "release-fragment-api-gateway.json").read_text(encoding="utf-8"))
            (root / "release-fragment-api-gateway-copy.json").write_text(
                json.dumps(duplicate), encoding="utf-8"
            )

            with self.assertRaisesRegex(ValueError, "duplicate release fragment"):
                manifest.assemble_manifest(
                    argparse.Namespace(
                        input_dir=root,
                        output=root / "release-manifest.json",
                        repository=REPOSITORY,
                        commit=COMMIT,
                        created_at=CREATED_AT,
                    )
                )

    def test_invalid_digest_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            with self.assertRaisesRegex(ValueError, "invalid OCI digest"):
                self.create_fragment(pathlib.Path(temp), "api-gateway", digest="sha256:not-a-digest")

    def test_mutable_or_mismatched_tag_is_rejected(self) -> None:
        fragment = {
            "service": "api-gateway",
            "image": "ghcr.io/yotoha0303/go-order-management-system-cloudnative-lab-api-gateway",
            "tag": "latest",
            "digest": DIGESTS["api-gateway"],
            "reference": "ghcr.io/yotoha0303/go-order-management-system-cloudnative-lab-api-gateway@"
            + DIGESTS["api-gateway"],
            "commit_sha": COMMIT,
        }
        with self.assertRaisesRegex(ValueError, "immutable tag mismatch"):
            manifest.validate_fragment(fragment, COMMIT)

    def test_uppercase_ghcr_path_is_rejected(self) -> None:
        fragment = {
            "service": "api-gateway",
            "image": "ghcr.io/Yotoha0303/go-order-management-system-cloudnative-lab-api-gateway",
            "tag": f"sha-{COMMIT}",
            "digest": DIGESTS["api-gateway"],
            "reference": "ghcr.io/Yotoha0303/go-order-management-system-cloudnative-lab-api-gateway@"
            + DIGESTS["api-gateway"],
            "commit_sha": COMMIT,
        }
        with self.assertRaisesRegex(ValueError, "must be lowercase"):
            manifest.validate_fragment(fragment, COMMIT)


if __name__ == "__main__":
    unittest.main()
