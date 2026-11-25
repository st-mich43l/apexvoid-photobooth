#!/usr/bin/env python3
"""Release tooling for the master CI (Python 3 standard library only). See docs/RELEASING.md.

    release.py check-version                 version files agree (frontend/package.json is the authority)
    release.py eligibility --event E --ref R [--manifest-url U | --manifest-file F]
                                             decide whether this run publishes; writes GITHUB_OUTPUT
    release.py assemble --exe EXE --notices TXT --out DIR [--expected-sha256 H]
                                             build the release package (release.json + files/) for the publisher

Nothing here talks to the VPS. Publishing itself is ansible-library's publish-photobooth-release.yml.
"""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import re
import shutil
import sys
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
PRODUCT_NAME = "ApexVoid Photobooth"
SEMVER = re.compile(
    r"^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)"
    r"(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?$"
)
SHA256 = re.compile(r"^[0-9a-f]{64}$")
COMMIT = re.compile(r"^[0-9a-f]{40}$")
MANIFEST_URL = "https://photobooth.apexvoid.net/downloads/index.json"
VARIANTS = {"standard": False, "canon": True}  # variant -> includes the Canon SDK
REQUIRED_GATES = {"standard": ["applicationLicense", "ffmpegLgpl"], "canon": ["applicationLicense", "ffmpegLgpl", "canonRedistribution"]}
FFMPEG_BUILD = "ffmpeg-n8.1.3-39-g660b5aa7db-win64-lgpl-8.1"
FFMPEG_RELEASE = "https://github.com/BtbN/FFmpeg-Builds/releases/tag/autobuild-2026-11-21-12-37"


class ReleaseError(Exception):
    pass


def exe_filename(version: str) -> str:
    return f"ApexVoidPhotobooth-v{version}-windows-x64.exe"


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


# ------------------------------------------------------------------ version
def app_version(root: Path = ROOT) -> str:
    v = json.loads((root / "frontend/package.json").read_text(encoding="utf-8"))["version"]
    if not isinstance(v, str) or not SEMVER.match(v):
        raise ReleaseError(f"frontend/package.json version {v!r} is not a semantic version (no build metadata)")
    return v


def check_version(root: Path = ROOT) -> str:
    """frontend/package.json is the single version authority; the Windows resources must match it."""
    v = app_version(root)
    core = v.split("-", 1)[0]
    res = json.loads((root / "build/winres/winres.json").read_text(encoding="utf-8"))["RT_VERSION"]["#1"]["0000"]
    want = {
        "fixed.file_version": f"{core}.0",
        "fixed.product_version": f"{core}.0",
        "info.0409.FileVersion": core,
        "info.0409.ProductVersion": v,
    }
    got = {
        "fixed.file_version": res["fixed"].get("file_version"),
        "fixed.product_version": res["fixed"].get("product_version"),
        "info.0409.FileVersion": res["info"]["0409"].get("FileVersion"),
        "info.0409.ProductVersion": res["info"]["0409"].get("ProductVersion"),
    }
    bad = [f"{k}: {got[k]!r} (expected {want[k]!r})" for k in want if got[k] != want[k]]
    if bad:
        raise ReleaseError("build/winres/winres.json does not match frontend/package.json " + v + ": " + "; ".join(bad))
    return v


# ------------------------------------------------------------------ publication gates
def load_publication(root: Path = ROOT) -> dict:
    return json.loads((root / "release/publication.json").read_text(encoding="utf-8"))


def gate_blockers(pub: dict) -> list[str]:
    """The gates that stop publication. Raises on a configuration that must never be accepted."""
    variant = pub.get("variant")
    if variant not in VARIANTS:
        raise ReleaseError(f"release/publication.json: variant must be one of {sorted(VARIANTS)}")
    gates = pub.get("gates") or {}
    blockers = []
    for name in REQUIRED_GATES[variant]:
        g = gates.get(name) or {}
        if g.get("approved") is not True:
            blockers.append(f"{name}: {g.get('note') or 'not approved'}")
    signing = gates.get("codeSigning") or {}
    if signing.get("signed") is not False:
        # This pipeline does not sign. Claiming a signature would be false.
        raise ReleaseError("release/publication.json: codeSigning.signed must be false (this pipeline has no signing step)")
    return blockers


def release_notes(version: str, root: Path = ROOT) -> Path:
    return root / "docs/releases" / f"v{version}.md"


def fetch_manifest(url: str, timeout: float = 20.0) -> dict:
    req = urllib.request.Request(url, headers={"Accept": "application/json", "Cache-Control": "no-cache"})
    with urllib.request.urlopen(req, timeout=timeout) as r:  # noqa: S310 (fixed https URL)
        return json.loads(r.read().decode("utf-8"))


def eligibility(event: str, ref: str, root: Path = ROOT, manifest: dict | None = None, manifest_loader=None) -> dict:
    """Decide whether this run publishes. Returns {publish, reason, version, channel, variant, ...}.

    Skips (publish=false, job succeeds) for: not master, publishing disabled, no approved release notes,
    version already published on a push. Raises (job fails) for misconfiguration: enabled with unapproved gates,
    invalid version files, a signing claim.
    """
    out = {"publish": False, "version": "", "channel": "", "variant": "", "reason": ""}
    if ref != "refs/heads/master" or event not in {"push", "workflow_dispatch"}:
        out["reason"] = f"only pushes and manual runs on master publish (this is {event} on {ref})"
        return out
    version = check_version(root)
    pub = load_publication(root)
    blockers = gate_blockers(pub)
    out.update(version=version, channel="preview" if "-" in version else "stable", variant=pub["variant"])
    if pub.get("enabled") is not True:
        out["reason"] = "publishing is disabled in release/publication.json"
        out["blockers"] = blockers
        return out
    if blockers:
        raise ReleaseError("publishing is enabled but gates are not approved: " + " | ".join(blockers))
    notes = release_notes(version, root)
    if not notes.is_file() or not notes.read_text(encoding="utf-8").strip():
        out["reason"] = f"no release intent for v{version}: docs/releases/v{version}.md does not exist"
        return out
    if manifest is None:
        manifest = (manifest_loader or (lambda: fetch_manifest(MANIFEST_URL)))()
    if not isinstance(manifest, dict) or manifest.get("schemaVersion") != 1 or not isinstance(manifest.get("releases"), list):
        raise ReleaseError("the public manifest is not a schema-1 manifest")
    published = any(isinstance(r, dict) and r.get("version") == version for r in manifest["releases"])
    if published and event == "push":
        out["reason"] = f"v{version} is already published; bump the version to release again"
        return out
    out["publish"] = True
    out["reason"] = (
        f"explicit re-publication of v{version}: identical files are a no-op, different files fail with a conflict"
        if published
        else f"v{version} is approved and not yet published"
    )
    return out


# ------------------------------------------------------------------ package
def assemble(exe: Path, notices: Path, out: Path, version: str, commit: str, variant: str, expected_sha256: str | None,
             root: Path = ROOT) -> dict:
    if not COMMIT.match(commit):
        raise ReleaseError("commit must be a full 40-character lower-case SHA")
    if variant not in VARIANTS:
        raise ReleaseError(f"unknown variant {variant!r}")
    sha = sha256_file(exe)
    if expected_sha256 is not None and sha != expected_sha256:
        raise ReleaseError(f"executable checksum {sha} does not match the validated build {expected_sha256}")
    notes = release_notes(version, root)
    if not notes.is_file():
        raise ReleaseError(f"{notes.relative_to(root)} is missing")
    embedded = notices.read_text(encoding="utf-8")
    if "GNU LESSER GENERAL PUBLIC LICENSE" not in embedded or "SIL OPEN FONT LICENSE" not in embedded:
        raise ReleaseError("the executable's notices are incomplete (LGPL or OFL text missing)")
    if out.exists() and any(out.iterdir()):
        raise ReleaseError(f"{out} is not empty")
    files = out / "files"
    files.mkdir(parents=True, exist_ok=True)
    name = exe_filename(version)
    shutil.copyfile(exe, files / name)
    if sha256_file(files / name) != sha:
        raise ReleaseError("copy of the executable is corrupt")
    (files / "SHA256SUMS").write_text(f"{sha}  {name}\n", encoding="utf-8")
    shutil.copyfile(notes, files / "RELEASE-NOTES.md")
    (files / "THIRD-PARTY-NOTICES.txt").write_text(
        f"{PRODUCT_NAME} {version} — third-party notices\n"
        f"Built from commit {commit}.\n\n"
        + (root / "docs/THIRD-PARTY.md").read_text(encoding="utf-8").rstrip()
        + "\n\n"
        + "=" * 78
        + "\nLicence texts embedded in the executable (ApexVoidPhotobooth.exe --notices)\n"
        + "=" * 78
        + "\n\n"
        + embedded.rstrip()
        + "\n",
        encoding="utf-8",
    )
    (files / "SOURCE.md").write_text(
        f"# Source for {PRODUCT_NAME} {version}\n\n"
        f"- Application: commit `{commit}` of the ApexVoid Photobooth repository (master branch).\n"
        f"- FFmpeg (LGPL v3, unmodified, run as a separate process): build `{FFMPEG_BUILD}` from {FFMPEG_RELEASE};\n"
        "  the FFmpeg source is available from https://ffmpeg.org/ and the build configuration from\n"
        "  https://github.com/BtbN/FFmpeg-Builds. The executable unpacks it to\n"
        "  `%LOCALAPPDATA%\\ApexVoidPhotobooth\\bin\\` and any `ffmpeg.exe` you place next to the application replaces it.\n"
        + ("- Canon EDSDK: proprietary, redistributed under Canon's terms; no source is available.\n" if VARIANTS[variant] else ""),
        encoding="utf-8",
    )
    base = f"releases/v{version}/"
    record = {
        "version": version,
        "tag": f"v{version}",
        "name": PRODUCT_NAME,
        "channel": "preview" if "-" in version else "stable",
        "commit": commit,
        "notesPath": base + "RELEASE-NOTES.md",
        "checksumsPath": base + "SHA256SUMS",
        "noticesPath": base + "THIRD-PARTY-NOTICES.txt",
        "assets": [
            {
                "platform": "windows",
                "architecture": "x64",
                "format": "exe",
                "filename": name,
                "path": base + name,
                "size": (files / name).stat().st_size,
                "sha256": sha,
                "signed": False,
                "includesCanonSdk": VARIANTS[variant],
            }
        ],
    }
    (out / "release.json").write_text(json.dumps(record, indent=2) + "\n", encoding="utf-8")
    return record


def candidate_manifest(record: dict) -> dict:
    """A one-release manifest for validating the record with the publisher's validator before upload."""
    now = dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    r = dict(record, publishedAt=now)
    return {"schemaVersion": 1, "updatedAt": now, "latest": {"stable": r["version"] if r["channel"] == "stable" else None}, "releases": [r]}


# ------------------------------------------------------------------ CLI
def write_outputs(values: dict) -> None:
    path = os.environ.get("GITHUB_OUTPUT")
    lines = [f"{k}={'true' if v is True else 'false' if v is False else v}" for k, v in values.items() if not isinstance(v, (list, dict))]
    if path:
        with open(path, "a", encoding="utf-8") as f:
            f.write("\n".join(lines) + "\n")


def write_summary(res: dict) -> None:
    path = os.environ.get("GITHUB_STEP_SUMMARY")
    if not path:
        return
    lines = ["## Release decision", "", f"**{'Publishing' if res['publish'] else 'Not publishing'}** — {res['reason']}", ""]
    if res.get("version"):
        lines += [f"- Version: `{res['version']}` ({res['channel']}, {res['variant']} variant)"]
    if res.get("blockers"):
        lines += ["", "### Open publication gates (release/publication.json)", ""] + [f"- {b}" for b in res["blockers"]]
    with open(path, "a", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("check-version")
    p = sub.add_parser("eligibility")
    p.add_argument("--event", required=True)
    p.add_argument("--ref", required=True)
    p.add_argument("--manifest-file")
    p = sub.add_parser("assemble")
    p.add_argument("--exe", required=True, type=Path)
    p.add_argument("--notices", required=True, type=Path)
    p.add_argument("--out", required=True, type=Path)
    p.add_argument("--commit", required=True)
    p.add_argument("--variant", required=True)
    p.add_argument("--expected-sha256")
    p.add_argument("--candidate-manifest", type=Path, help="also write a one-release manifest for validation")
    a = ap.parse_args(argv)
    try:
        if a.cmd == "check-version":
            print(f"version {check_version()} is consistent")
        elif a.cmd == "eligibility":
            manifest = json.loads(Path(a.manifest_file).read_text(encoding="utf-8")) if a.manifest_file else None
            res = eligibility(a.event, a.ref, manifest=manifest)
            write_outputs(res)
            write_summary(res)
            print(json.dumps(res, indent=2))
        else:
            record = assemble(a.exe, a.notices, a.out, app_version(), a.commit, a.variant, a.expected_sha256)
            if a.candidate_manifest:
                a.candidate_manifest.write_text(json.dumps(candidate_manifest(record), indent=2), encoding="utf-8")
            write_outputs({"sha256": record["assets"][0]["sha256"], "size": record["assets"][0]["size"], "version": record["version"]})
            print(json.dumps(record, indent=2))
    except ReleaseError as e:
        print(f"::error::{e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
