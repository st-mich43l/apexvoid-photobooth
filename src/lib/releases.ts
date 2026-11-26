/**
 * Release manifest (schema version 1) — the contract shared with the publisher in ansible-library
 * (`roles/publish_photobooth_release/files/publish_release.py`). The manifest is read at runtime from
 * `/downloads/index.json`, so a new release appears without rebuilding the site. Nothing here invents a
 * release fact: every value shown on a page comes from a manifest that passed `validateManifest`.
 *
 * Keep the rules identical to the publisher's `validate_manifest`; both run against the same fixtures
 * (`test/fixtures/manifest/`, copied from ansible-library).
 */
export const SCHEMA_VERSION = 1;
export const PRODUCT_NAME = "ApexVoid Photobooth";
/** Where the website serves release storage (Nginx alias of the read-only bind mount). */
export const DOWNLOADS_BASE = "/downloads/";
export const MANIFEST_URL = `${DOWNLOADS_BASE}index.json`;

export type Channel = "stable" | "preview";

export interface ReleaseAsset {
  platform: "windows";
  architecture: "x64";
  format: "exe";
  filename: string;
  /** Relative to /downloads/, always `releases/v<version>/<filename>`. */
  path: string;
  size: number;
  sha256: string;
  /** false: the executable has no Authenticode signature. */
  signed: boolean;
  includesCanonSdk: boolean;
}

export interface Release {
  version: string;
  tag: string;
  name: string;
  channel: Channel;
  publishedAt: string;
  commit: string;
  notesPath: string;
  checksumsPath: string;
  noticesPath: string;
  assets: ReleaseAsset[];
}

export interface Manifest {
  schemaVersion: 1;
  updatedAt: string;
  latest: { stable: string | null };
  releases: Release[];
}

const SEMVER =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?$/;
const SHA256 = /^[0-9a-f]{64}$/;
const COMMIT = /^[0-9a-f]{40}$/;
const TIMESTAMP = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z$/;
const SAFE_NAME = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/;
const PLATFORMS: Record<string, readonly string[]> = { windows: ["x64"] };

export const exeFilename = (version: string): string => `ApexVoidPhotobooth-v${version}-windows-x64.exe`;

const isObject = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);
const isTimestamp = (v: unknown): v is string => typeof v === "string" && TIMESTAMP.test(v) && !Number.isNaN(Date.parse(v));

/* ------------------------------------------------------------------ versions */

type Parsed = { core: [number, number, number]; pre: string[] };

function parse(v: string): Parsed | null {
  const m = SEMVER.exec(v);
  if (!m) return null;
  return { core: [Number(m[1]), Number(m[2]), Number(m[3])], pre: m[4] ? m[4].split(".") : [] };
}

export const isSemver = (v: unknown): v is string => typeof v === "string" && parse(v) !== null;
export const isPrerelease = (v: string): boolean => (parse(v)?.pre.length ?? 0) > 0;

/** SemVer 2.0 precedence: negative when a < b. Throws on invalid input (validate first). */
export function compareVersions(a: string, b: string): number {
  const pa = parse(a);
  const pb = parse(b);
  if (!pa || !pb) throw new Error(`not a version: ${!pa ? a : b}`);
  for (let i = 0; i < 3; i++) if (pa.core[i] !== pb.core[i]) return pa.core[i]! - pb.core[i]!;
  if (!pa.pre.length || !pb.pre.length) return pb.pre.length - pa.pre.length; // a release outranks its pre-releases
  for (let i = 0; i < Math.min(pa.pre.length, pb.pre.length); i++) {
    const x = pa.pre[i]!;
    const y = pb.pre[i]!;
    if (x === y) continue;
    const nx = /^\d+$/.test(x);
    const ny = /^\d+$/.test(y);
    if (nx && ny) return Number(x) - Number(y);
    if (nx !== ny) return nx ? -1 : 1;
    return x < y ? -1 : 1;
  }
  return pa.pre.length - pb.pre.length;
}

/* ------------------------------------------------------------------ validation */

/** A path inside the release's own directory: `releases/v<version>/<plain file name>`. */
function relPathOk(path: unknown, version: string): path is string {
  if (typeof path !== "string") return false;
  const prefix = `releases/v${version}/`;
  if (!path.startsWith(prefix)) return false;
  const name = path.slice(prefix.length);
  return SAFE_NAME.test(name) && !name.includes("..");
}

function validateRelease(r: unknown, where: string): string[] {
  if (!isObject(r)) return [`${where}: not an object`];
  const e: string[] = [];
  const v = r.version;
  if (!isSemver(v)) return [`${where}: invalid version`];
  if (r.tag !== `v${v}`) e.push(`${where}: tag must be v${v}`);
  if (r.name !== PRODUCT_NAME) e.push(`${where}: invalid name`);
  if (r.channel !== "stable" && r.channel !== "preview") e.push(`${where}: invalid channel`);
  else if (r.channel === "stable" && isPrerelease(v)) e.push(`${where}: a stable release cannot be a pre-release`);
  else if (r.channel === "preview" && !isPrerelease(v)) e.push(`${where}: a preview must be a pre-release version`);
  if (!isTimestamp(r.publishedAt)) e.push(`${where}: invalid publishedAt`);
  if (typeof r.commit !== "string" || !COMMIT.test(r.commit)) e.push(`${where}: invalid commit`);
  for (const k of ["notesPath", "checksumsPath", "noticesPath"] as const) {
    if (!relPathOk(r[k], v)) e.push(`${where}: invalid ${k}`);
  }
  const assets = r.assets;
  if (!Array.isArray(assets) || assets.length === 0) return [...e, `${where}: assets must be a non-empty list`];
  for (const a of assets as unknown[]) {
    if (!isObject(a)) {
      e.push(`${where}: asset is not an object`);
      continue;
    }
    const platform = typeof a.platform === "string" ? a.platform : "";
    if (!PLATFORMS[platform]?.includes(String(a.architecture))) e.push(`${where}: unsupported platform`);
    if (a.format !== "exe") e.push(`${where}: unsupported format`);
    if (a.filename !== exeFilename(v)) e.push(`${where}: invalid filename`);
    if (a.path !== `releases/v${v}/${exeFilename(v)}`) e.push(`${where}: invalid asset path`);
    if (typeof a.size !== "number" || !Number.isSafeInteger(a.size) || a.size <= 0) e.push(`${where}: invalid size`);
    if (typeof a.sha256 !== "string" || !SHA256.test(a.sha256)) e.push(`${where}: invalid sha256`);
    if (typeof a.signed !== "boolean") e.push(`${where}: invalid signed`);
    if (typeof a.includesCanonSdk !== "boolean") e.push(`${where}: invalid includesCanonSdk`);
  }
  return e;
}

export type ValidationResult = { ok: true; manifest: Manifest } | { ok: false; errors: string[] };

/** Validate an untrusted manifest. Any error makes the whole manifest unusable: nothing partial is shown. */
export function validateManifest(m: unknown): ValidationResult {
  if (!isObject(m)) return { ok: false, errors: ["manifest is not an object"] };
  if (m.schemaVersion !== SCHEMA_VERSION) return { ok: false, errors: ["unsupported schemaVersion"] };
  const errors: string[] = [];
  if (!isTimestamp(m.updatedAt)) errors.push("invalid updatedAt");
  if (!Array.isArray(m.releases)) return { ok: false, errors: [...errors, "releases must be a list"] };
  const seen = new Set<string>();
  const stable = new Set<string>();
  m.releases.forEach((r: unknown, i: number) => {
    const re = validateRelease(r, `releases[${i}]`);
    errors.push(...re);
    if (isObject(r) && typeof r.version === "string") {
      if (seen.has(r.version)) errors.push(`duplicate version ${r.version}`);
      seen.add(r.version);
      if (!re.length && r.channel === "stable") stable.add(r.version);
    }
  });
  if (!isObject(m.latest)) errors.push("latest must be an object");
  else {
    const ls = m.latest.stable;
    if (ls === null) {
      if (stable.size) errors.push("latest.stable is null although a stable release exists");
    } else if (typeof ls !== "string" || !stable.has(ls)) errors.push("latest.stable is not a published stable release");
  }
  return errors.length ? { ok: false, errors } : { ok: true, manifest: m as unknown as Manifest };
}

/* ------------------------------------------------------------------ what the pages show */

/** Newest first by version precedence. */
export const sortReleases = (list: readonly Release[]): Release[] => [...list].sort((a, b) => compareVersions(b.version, a.version));

export type DownloadView = { state: "available"; release: Release; asset: ReleaseAsset } | { state: "empty" };

/** Resolve current by the authoritative pointer; a rollback may intentionally point below the newest version. */
export function currentRelease(m: Manifest): Release | undefined {
  return m.latest.stable ? m.releases.find((r) => r.version === m.latest.stable && r.channel === "stable") : undefined;
}

/** The four highest SemVer stable versions other than the current pointer. */
export function archiveReleases(m: Manifest, limit = 4): Release[] {
  const current = currentRelease(m)?.version;
  return sortReleases(m.releases.filter((r) => r.channel === "stable" && r.version !== current)).slice(0, Math.max(0, limit));
}

/** Current first, followed by the bounded public archive. */
export function retainedPublicReleases(m: Manifest): Release[] {
  const current = currentRelease(m);
  return [...(current ? [current] : []), ...archiveReleases(m)];
}

/** The download box only offers the manifest's current stable release. */
export function downloadView(m: Manifest): DownloadView {
  const stable = currentRelease(m);
  const stableAsset = stable && windowsAsset(stable);
  if (stable && stableAsset) return { state: "available", release: stable, asset: stableAsset };
  return { state: "empty" };
}

/** The Windows x64 executable of a release (the only platform schema v1 allows). */
export const windowsAsset = (r: Release): ReleaseAsset | undefined =>
  r.assets.find((a) => a.platform === "windows" && a.architecture === "x64" && a.format === "exe");

/** Public URL of a validated relative path. */
export const downloadUrl = (path: string): string => DOWNLOADS_BASE + path;

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
