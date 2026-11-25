import { describe, expect, it } from "vitest";
import { formatBytes, latestStable, sortReleases, windowsAsset, type Release } from "../../src/lib/releases";
import data from "../../src/content/releases.json";

const rel = (tag: string, publishedAt: string, prerelease = false, assets: Release["assets"] = []): Release => ({
  tag,
  name: `ApexVoid Photobooth ${tag}`,
  publishedAt,
  prerelease,
  notesUrl: `https://example.test/${tag}`,
  assets,
});

describe("release data", () => {
  it("is empty until the first official release: the site must not invent one", () => {
    expect(data).toEqual([]);
    expect(latestStable(data as Release[])).toBeNull();
  });

  it("the latest stable release ignores pre-releases and sorts by date", () => {
    const list = [
      rel("v1.0.0", "2027-01-01T00:00:00Z"),
      rel("v1.1.0-rc.1", "2027-03-01T00:00:00Z", true),
      rel("v1.0.1", "2027-02-01T00:00:00Z"),
    ];
    expect(latestStable(list)?.tag).toBe("v1.0.1");
    expect(sortReleases(list).map((r) => r.tag)).toEqual(["v1.1.0-rc.1", "v1.0.1", "v1.0.0"]);
    expect(latestStable([rel("v2.0.0-beta", "2027-01-01T00:00:00Z", true)])).toBeNull();
  });

  it("picks the Windows x64 executable, then any .exe, then a Windows zip", () => {
    const a = (name: string) => ({ name, size: 1, url: `https://example.test/${name}` });
    expect(windowsAsset(rel("v1", "2027-01-01T00:00:00Z", false, [a("notes.txt"), a("ApexVoidPhotobooth-windows-x64.exe")]))?.name).toBe(
      "ApexVoidPhotobooth-windows-x64.exe",
    );
    expect(windowsAsset(rel("v1", "2027-01-01T00:00:00Z", false, [a("ApexVoidPhotobooth.exe")]))?.name).toBe("ApexVoidPhotobooth.exe");
    expect(windowsAsset(rel("v1", "2027-01-01T00:00:00Z", false, [a("ApexVoidPhotobooth-windows-x64.zip")]))?.name).toBe(
      "ApexVoidPhotobooth-windows-x64.zip",
    );
    expect(windowsAsset(rel("v1", "2027-01-01T00:00:00Z", false, [a("source.tar.gz")]))).toBeNull();
  });

  it("formats sizes", () => {
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(48.2 * 1024 * 1024)).toBe("48.2 MB");
    expect(formatBytes(-1)).toBe("—");
  });
});
