import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import {
  compareVersions,
  downloadUrl,
  downloadView,
  archiveReleases,
  formatBytes,
  retainedPublicReleases,
  sortReleases,
  validateManifest,
  type Manifest,
} from "../../src/lib/releases";

/** Fixtures shared with the publisher (ansible-library roles/publish_photobooth_release/files/fixtures). */
const DIR = join(import.meta.dirname, "../fixtures/manifest");
const load = (name: string): unknown => JSON.parse(readFileSync(join(DIR, name), "utf8"));
const invalid = readdirSync(DIR).filter((f) => f.startsWith("manifest-invalid-"));

const ok = (m: unknown): Manifest => {
  const r = validateManifest(m);
  if (!r.ok) throw new Error(r.errors.join("; "));
  return r.manifest;
};

describe("manifest contract (shared fixtures)", () => {
  it("accepts the valid and the empty manifest", () => {
    expect(validateManifest(load("manifest-valid.json")).ok).toBe(true);
    expect(validateManifest(load("manifest-empty.json")).ok).toBe(true);
  });

  it("has every invalid fixture", () => expect(invalid.length).toBeGreaterThanOrEqual(20));

  it.each(invalid)("rejects %s", (name) => {
    const r = validateManifest(load(name));
    expect(r.ok).toBe(false);
  });

  it("rejects non-objects, wrong schema versions and missing lists", () => {
    for (const m of [
      null,
      [],
      "x",
      1,
      { schemaVersion: 2 },
      { schemaVersion: 1, updatedAt: "2027-01-01T00:00:00Z", latest: { stable: null } },
    ]) {
      expect(validateManifest(m).ok).toBe(false);
    }
  });

  it("rejects any link that could leave the release directory", () => {
    const base = load("manifest-valid.json") as Manifest;
    const bad = [
      "https://evil.test/x.exe",
      "//evil.test/x",
      "/releases/v0.2.1/RELEASE-NOTES.md",
      "releases/v0.2.1/../../x",
      "releases/v0.2.0/RELEASE-NOTES.md",
      "javascript:alert(1)",
    ];
    for (const p of bad) {
      const m = structuredClone(base);
      m.releases[1]!.notesPath = p;
      expect(validateManifest(m).ok, p).toBe(false);
    }
  });
});

describe("versions", () => {
  it("orders by SemVer precedence, not by text or date", () => {
    const v = ["0.10.0", "0.2.0", "0.2.0-rc.1", "0.2.0-rc.10", "0.2.0-rc.2", "0.2.0-alpha", "1.0.0", "0.9.9"];
    expect([...v].sort(compareVersions)).toEqual([
      "0.2.0-alpha",
      "0.2.0-rc.1",
      "0.2.0-rc.2",
      "0.2.0-rc.10",
      "0.2.0",
      "0.9.9",
      "0.10.0",
      "1.0.0",
    ]);
  });
  it("sorts releases newest first", () => {
    const m = ok(load("manifest-valid.json"));
    expect(sortReleases([...m.releases].reverse()).map((r) => r.version)).toEqual(["0.3.0-rc.1", "0.2.1", "0.2.0"]);
  });
});

describe("what the download box offers", () => {
  it("the manifest's current stable release, never a newer preview", () => {
    const v = downloadView(ok(load("manifest-valid.json")));
    expect(v.state).toBe("available");
    if (v.state !== "empty") {
      expect(v.release.version).toBe("0.2.1");
      expect(downloadUrl(v.asset.path)).toBe("/downloads/releases/v0.2.1/ApexVoidPhotobooth-v0.2.1-windows-x64.exe");
    }
  });
  it("follows latest.stable (a rollback), not the highest version", () => {
    const m = ok(load("manifest-valid.json"));
    m.latest.stable = "0.2.0";
    const v = downloadView(ok(m));
    expect(v.state === "available" && v.release.version).toBe("0.2.0");
  });
  it("does not offer previews when no stable release exists", () => {
    const m = ok(load("manifest-valid.json"));
    m.releases = m.releases.filter((r) => r.channel === "preview");
    m.latest.stable = null;
    const v = downloadView(ok(m));
    expect(v.state).toBe("empty");
  });
  it("nothing when the manifest is empty", () => {
    expect(downloadView(ok(load("manifest-empty.json"))).state).toBe("empty");
  });
});

describe("retained public release selectors", () => {
  it("uses the manifest pointer for current and returns no more than four previous stable releases", () => {
    const m = ok(load("manifest-valid.json"));
    const base = m.releases.find((r) => r.channel === "stable")!;
    const older = ["0.2.2", "0.2.3", "0.2.4", "0.2.5", "0.2.6", "0.2.7"].map((version) => {
      const rel = structuredClone(base);
      rel.version = version;
      rel.tag = `v${version}`;
      rel.publishedAt = "2026-11-25T00:00:00Z";
      rel.notesPath = `releases/v${version}/RELEASE-NOTES.md`;
      rel.checksumsPath = `releases/v${version}/SHA256SUMS`;
      rel.noticesPath = `releases/v${version}/THIRD-PARTY-NOTICES.txt`;
      rel.assets[0]!.filename = `ApexVoidPhotobooth-v${version}-windows-x64.exe`;
      rel.assets[0]!.path = `releases/v${version}/${rel.assets[0]!.filename}`;
      return rel;
    });
    m.releases = [...m.releases, ...older];
    m.latest.stable = "0.2.0"; // deliberate rollback; 0.2.1 stays an archive entry
    const validated = ok(m);
    expect(retainedPublicReleases(validated).map((r) => r.version)).toEqual(["0.2.0", "0.2.7", "0.2.6", "0.2.5", "0.2.4"]);
    expect(archiveReleases(validated).map((r) => r.version)).toEqual(["0.2.7", "0.2.6", "0.2.5", "0.2.4"]);
    expect(archiveReleases(validated).some((r) => r.version === validated.latest.stable)).toBe(false);
  });
});

describe("formatBytes", () => {
  it("uses binary units", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(1536)).toBe("1.5 KB");
    expect(formatBytes(48.2 * 1024 * 1024)).toBe("48.2 MB");
    expect(formatBytes(-1)).toBe("—");
  });
});
