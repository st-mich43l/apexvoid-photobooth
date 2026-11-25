import { MANIFEST_URL, validateManifest, type Manifest } from "../releases";

/**
 * Runtime access to the release manifest. Pure apart from `fetch`, so it is unit-tested with a fake.
 *
 * - `cache: "no-cache"`: always revalidate, so a new release shows up immediately.
 * - one request per page: concurrent callers share the same promise.
 * - a timeout per attempt, and one retry for network errors and 5xx (not for 404 or bad data).
 * - the result is a state, never an exception: the pages render every outcome.
 */
export type ManifestResult =
  | { kind: "ok"; manifest: Manifest }
  /** The manifest could not be fetched (network, timeout, 404, 5xx). */
  | { kind: "unavailable"; reason: string }
  /** The manifest was fetched but is not valid JSON or fails the schema: nothing from it is shown. */
  | { kind: "invalid"; errors: string[] };

export interface ServiceOptions {
  fetchImpl?: typeof fetch;
  url?: string;
  timeoutMs?: number;
  retries?: number;
  retryDelayMs?: number;
}

class Retryable extends Error {}

async function attempt(fetchImpl: typeof fetch, url: string, timeoutMs: number): Promise<ManifestResult> {
  const ctl = new AbortController();
  const timer = setTimeout(() => ctl.abort(), timeoutMs);
  let res: Response;
  try {
    res = await fetchImpl(url, {
      cache: "no-cache",
      credentials: "omit",
      headers: { Accept: "application/json" },
      signal: ctl.signal,
    });
  } catch (e) {
    throw new Retryable(ctl.signal.aborted ? "timeout" : `network: ${(e as Error).message}`);
  } finally {
    clearTimeout(timer);
  }
  if (res.status >= 500) throw new Retryable(`HTTP ${res.status}`);
  if (!res.ok) return { kind: "unavailable", reason: `HTTP ${res.status}` };
  let data: unknown;
  try {
    data = JSON.parse(await res.text());
  } catch {
    return { kind: "invalid", errors: ["not JSON"] };
  }
  const v = validateManifest(data);
  return v.ok ? { kind: "ok", manifest: v.manifest } : { kind: "invalid", errors: v.errors };
}

export async function fetchManifest(opts: ServiceOptions = {}): Promise<ManifestResult> {
  const fetchImpl = opts.fetchImpl ?? globalThis.fetch.bind(globalThis);
  const url = opts.url ?? MANIFEST_URL;
  const timeoutMs = opts.timeoutMs ?? 8000;
  const retries = opts.retries ?? 1;
  const delay = opts.retryDelayMs ?? 750;
  let last = "unknown";
  for (let i = 0; i <= retries; i++) {
    if (i > 0) await new Promise((r) => setTimeout(r, delay));
    try {
      return await attempt(fetchImpl, url, timeoutMs);
    } catch (e) {
      last = (e as Error).message;
    }
  }
  return { kind: "unavailable", reason: last };
}

let shared: Promise<ManifestResult> | null = null;

/** The page-wide manifest: fetched once, shared by the download panel and the release list. */
export function loadManifest(opts?: ServiceOptions): Promise<ManifestResult> {
  shared ??= fetchManifest(opts);
  return shared;
}

/** Tests only. */
export function resetManifestCache(): void {
  shared = null;
}
