"""Tests for scripts/release/release.py:  python3 -m unittest discover -s scripts/release"""
from __future__ import annotations

import json
import shutil
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import release as rel  # noqa: E402

REPO = Path(__file__).resolve().parents[2]
NOTICES = "SIL OPEN FONT LICENSE Version 1.1\n...\nGNU LESSER GENERAL PUBLIC LICENSE Version 3\n..."
COMMIT = "0123456789abcdef0123456789abcdef01234567"


class Repo:
    """A minimal copy of the files the tooling reads, so every test can change them freely."""

    def __init__(self, version: str = "0.3.0") -> None:
        self.dir = Path(tempfile.mkdtemp())
        for f in ("frontend/package.json", "build/winres/winres.json", "release/publication.json", "docs/THIRD-PARTY.md"):
            (self.dir / f).parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(REPO / f, self.dir / f)
        self.set_version(version)

    def set_version(self, v: str, winres: str | None = None) -> None:
        pkg = json.loads((self.dir / "frontend/package.json").read_text())
        pkg["version"] = v
        (self.dir / "frontend/package.json").write_text(json.dumps(pkg))
        res = json.loads((self.dir / "build/winres/winres.json").read_text())
        b = res["RT_VERSION"]["#1"]["0000"]
        core = (winres or v).split("-")[0]
        b["fixed"]["file_version"] = b["fixed"]["product_version"] = core + ".0"
        b["info"]["0409"]["FileVersion"] = core
        b["info"]["0409"]["ProductVersion"] = winres or v
        (self.dir / "build/winres/winres.json").write_text(json.dumps(res))

    def publication(self, enabled=True, approve=("applicationLicense", "ffmpegLgpl"), variant="standard", signed=False) -> None:
        pub = json.loads((self.dir / "release/publication.json").read_text())
        pub["enabled"] = enabled
        pub["variant"] = variant
        for g in approve:
            pub["gates"][g]["approved"] = True
        pub["gates"]["codeSigning"]["signed"] = signed
        (self.dir / "release/publication.json").write_text(json.dumps(pub))

    def notes(self, v: str) -> None:
        p = self.dir / "docs/releases" / f"v{v}.md"
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(f"# ApexVoid Photobooth {v}\n\nApproved notes.\n")

    def close(self) -> None:
        shutil.rmtree(self.dir)


def manifest(*versions: str) -> dict:
    return {"schemaVersion": 1, "updatedAt": "2027-01-01T00:00:00Z", "latest": {"stable": None}, "releases": [{"version": v} for v in versions]}


class VersionTests(unittest.TestCase):
    def test_the_repository_is_consistent(self):
        self.assertTrue(rel.SEMVER.match(rel.check_version(REPO)))

    def test_mismatched_windows_resources_fail(self):
        r = Repo("0.3.0")
        try:
            r.set_version("0.3.0", winres="0.2.0")
            with self.assertRaisesRegex(rel.ReleaseError, "winres"):
                rel.check_version(r.dir)
        finally:
            r.close()

    def test_prerelease_keeps_the_numeric_file_version(self):
        r = Repo("1.0.0-rc.1")
        try:
            self.assertEqual(rel.check_version(r.dir), "1.0.0-rc.1")
        finally:
            r.close()

    def test_invalid_version_fails(self):
        r = Repo("1.0")
        try:
            with self.assertRaises(rel.ReleaseError):
                rel.check_version(r.dir)
        finally:
            r.close()


class EligibilityTests(unittest.TestCase):
    def setUp(self):
        self.r = Repo("0.3.0")

    def tearDown(self):
        self.r.close()

    def decide(self, event="push", ref="refs/heads/master", m=None):
        return rel.eligibility(event, ref, self.r.dir, manifest=m if m is not None else manifest())

    def test_the_committed_configuration_never_publishes(self):
        res = rel.eligibility("push", "refs/heads/master", REPO, manifest=manifest())
        self.assertFalse(res["publish"])
        self.assertIn("disabled", res["reason"])
        self.assertTrue(res["blockers"], "the committed gates must list their blockers")

    def test_only_master_pushes_and_manual_runs(self):
        self.r.publication()
        self.r.notes("0.3.0")
        for event, ref in [("pull_request", "refs/heads/master"), ("push", "refs/heads/community"), ("push", "refs/heads/feature"), ("schedule", "refs/heads/master")]:
            self.assertFalse(self.decide(event, ref)["publish"], f"{event} {ref}")

    def test_disabled_skips_and_reports_blockers(self):
        res = self.decide()
        self.assertFalse(res["publish"])
        self.assertEqual(len(res["blockers"]), 2)

    def test_enabled_with_unapproved_gates_fails(self):
        self.r.publication(approve=("applicationLicense",))
        with self.assertRaisesRegex(rel.ReleaseError, "ffmpegLgpl"):
            self.decide()

    def test_canon_variant_needs_canon_authorization(self):
        self.r.publication(variant="canon")
        with self.assertRaisesRegex(rel.ReleaseError, "canonRedistribution"):
            self.decide()

    def test_a_signing_claim_is_refused(self):
        self.r.publication(signed=True)
        with self.assertRaisesRegex(rel.ReleaseError, "signed"):
            self.decide()

    def test_no_release_intent_skips(self):
        self.r.publication()
        res = self.decide()
        self.assertFalse(res["publish"])
        self.assertIn("docs/releases/v0.3.0.md", res["reason"])

    def test_approved_new_version_publishes(self):
        self.r.publication()
        self.r.notes("0.3.0")
        res = self.decide(m=manifest("0.2.0"))
        self.assertTrue(res["publish"])
        self.assertEqual((res["version"], res["channel"], res["variant"]), ("0.3.0", "stable", "standard"))

    def test_prerelease_is_a_preview(self):
        self.r.set_version("0.4.0-rc.1")
        self.r.publication()
        self.r.notes("0.4.0-rc.1")
        self.assertEqual(self.decide()["channel"], "preview")

    def test_published_version_skips_on_push_and_retries_explicitly(self):
        self.r.publication()
        self.r.notes("0.3.0")
        self.assertFalse(self.decide("push", m=manifest("0.3.0"))["publish"])
        res = self.decide("workflow_dispatch", m=manifest("0.3.0"))
        self.assertTrue(res["publish"])
        self.assertIn("conflict", res["reason"])

    def test_manifest_is_only_fetched_when_needed(self):
        calls = []
        rel.eligibility("push", "refs/heads/master", self.r.dir, manifest_loader=lambda: calls.append(1) or manifest())
        self.assertEqual(calls, [], "disabled publishing must not depend on the public site")

    def test_a_broken_manifest_fails(self):
        self.r.publication()
        self.r.notes("0.3.0")
        with self.assertRaises(rel.ReleaseError):
            self.decide(m={"schemaVersion": 2})


class AssembleTests(unittest.TestCase):
    def setUp(self):
        self.r = Repo("0.3.0")
        self.r.notes("0.3.0")
        self.tmp = Path(tempfile.mkdtemp())
        self.exe = self.tmp / "ApexVoidPhotobooth.exe"
        self.exe.write_bytes(b"MZ" + bytes(range(256)) * 40)
        self.notices = self.tmp / "notices.txt"
        self.notices.write_text(NOTICES)

    def tearDown(self):
        self.r.close()
        shutil.rmtree(self.tmp)

    def build(self, **kw):
        args = dict(exe=self.exe, notices=self.notices, out=self.tmp / "pkg", version="0.3.0", commit=COMMIT, variant="standard",
                    expected_sha256=rel.sha256_file(self.exe), root=self.r.dir)
        args.update(kw)
        return rel.assemble(**args)

    def test_package_layout_and_record(self):
        rec = self.build()
        files = sorted(p.name for p in (self.tmp / "pkg/files").iterdir())
        self.assertEqual(files, ["ApexVoidPhotobooth-v0.3.0-windows-x64.exe", "RELEASE-NOTES.md", "SHA256SUMS", "SOURCE.md", "THIRD-PARTY-NOTICES.txt"])
        a = rec["assets"][0]
        self.assertEqual(a["sha256"], rel.sha256_file(self.exe))
        self.assertEqual(a["size"], self.exe.stat().st_size)
        self.assertIs(a["signed"], False)
        self.assertIs(a["includesCanonSdk"], False)
        sums = (self.tmp / "pkg/files/SHA256SUMS").read_text()
        self.assertEqual(sums, f"{a['sha256']}  ApexVoidPhotobooth-v0.3.0-windows-x64.exe\n")
        notices = (self.tmp / "pkg/files/THIRD-PARTY-NOTICES.txt").read_text()
        self.assertIn("GNU LESSER GENERAL PUBLIC LICENSE", notices)
        self.assertIn("# Third-party software", notices)
        self.assertEqual(json.loads((self.tmp / "pkg/release.json").read_text()), rec)
        self.assertNotIn("publishedAt", rec, "the publisher sets the publication time")

    def test_canon_variant_is_declared(self):
        self.assertIs(self.build(variant="canon")["assets"][0]["includesCanonSdk"], True)

    def test_checksum_mismatch_fails(self):
        with self.assertRaisesRegex(rel.ReleaseError, "does not match"):
            self.build(expected_sha256="0" * 64)

    def test_incomplete_notices_fail(self):
        self.notices.write_text("SIL OPEN FONT LICENSE only")
        with self.assertRaisesRegex(rel.ReleaseError, "notices"):
            self.build()

    def test_missing_release_notes_fail(self):
        with self.assertRaisesRegex(rel.ReleaseError, "missing"):
            self.build(version="9.9.9")

    def test_candidate_manifest_shape(self):
        m = rel.candidate_manifest(self.build())
        self.assertEqual(m["latest"]["stable"], "0.3.0")
        self.assertRegex(m["releases"][0]["publishedAt"], r"Z$")


if __name__ == "__main__":
    unittest.main()
