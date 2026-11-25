/**
 * Release data model. The site never invents release facts: the list comes from
 * `src/content/releases.json`, which is empty until the first GitHub Release exists and will later be
 * generated from the GitHub Releases API (WEB-2). Everything the UI shows is derived from these fields.
 */
export interface ReleaseAsset {
  name: string;
  /** Size in bytes, as reported by GitHub. */
  size: number;
  /** Direct download URL (GitHub release asset). */
  url: string;
  /** Hex SHA-256 when the release publishes one; omitted otherwise (never computed or guessed by the site). */
  sha256?: string;
}

export interface Release {
  /** Git tag, e.g. "v1.0.0". */
  tag: string;
  name: string;
  /** ISO-8601 publication time from GitHub. */
  publishedAt: string;
  prerelease: boolean;
  /** Link to the release page with the notes. */
  notesUrl: string;
  assets: ReleaseAsset[];
}

/** Newest first; drafts never reach this list. */
export function sortReleases(list: readonly Release[]): Release[] {
  return [...list].sort((a, b) => Date.parse(b.publishedAt) - Date.parse(a.publishedAt));
}

/** The newest release that is not a pre-release, or null when there is none (the honest empty state). */
export function latestStable(list: readonly Release[]): Release | null {
  return sortReleases(list).find((r) => !r.prerelease) ?? null;
}

/** The Windows x64 download of a release: the portable executable, or a zip that contains it. */
export function windowsAsset(r: Release): ReleaseAsset | null {
  const byPriority = [/windows.*x64.*\.exe$/i, /\.exe$/i, /windows.*x64.*\.zip$/i];
  for (const re of byPriority) {
    const hit = r.assets.find((a) => re.test(a.name));
    if (hit) return hit;
  }
  return null;
}

/** Human-readable size (binary units), e.g. "48.2 MB". */
export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "—";
  const units = ["B", "KB", "MB", "GB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${i === 0 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`;
}
