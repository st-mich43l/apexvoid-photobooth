import { readFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchManifest, loadManifest, resetManifestCache } from "../../src/lib/client/release-service";

const valid = readFileSync(join(import.meta.dirname, "../fixtures/manifest/manifest-valid.json"), "utf8");
const reply =
  (body: string, status = 200) =>
  () =>
    Promise.resolve(new Response(body, { status }));
const opts = { retryDelayMs: 1, timeoutMs: 50 };

afterEach(() => resetManifestCache());

describe("release service", () => {
  it("returns a validated manifest and asks the server to revalidate", async () => {
    const f = vi.fn(reply(valid));
    const r = await fetchManifest({ ...opts, fetchImpl: f as unknown as typeof fetch });
    expect(r.kind).toBe("ok");
    expect(f).toHaveBeenCalledWith("/downloads/index.json", expect.objectContaining({ cache: "no-cache", credentials: "omit" }));
  });

  it("404 is unavailable without a retry", async () => {
    const f = vi.fn(reply("", 404));
    expect((await fetchManifest({ ...opts, fetchImpl: f as unknown as typeof fetch })).kind).toBe("unavailable");
    expect(f).toHaveBeenCalledTimes(1);
  });

  it("retries once after a server error, then succeeds", async () => {
    const f = vi.fn().mockImplementationOnce(reply("", 502)).mockImplementationOnce(reply(valid));
    expect((await fetchManifest({ ...opts, fetchImpl: f as unknown as typeof fetch })).kind).toBe("ok");
    expect(f).toHaveBeenCalledTimes(2);
  });

  it("network errors end as unavailable after the retry", async () => {
    const f = vi.fn(() => Promise.reject(new TypeError("offline")));
    const r = await fetchManifest({ ...opts, fetchImpl: f as unknown as typeof fetch });
    expect(r).toEqual({ kind: "unavailable", reason: "network: offline" });
    expect(f).toHaveBeenCalledTimes(2);
  });

  it("times out a hanging request", async () => {
    const hang = (_: unknown, init?: RequestInit) =>
      new Promise<Response>((_r, reject) =>
        init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError"))),
      );
    const r = await fetchManifest({ ...opts, retries: 0, fetchImpl: hang as unknown as typeof fetch });
    expect(r).toEqual({ kind: "unavailable", reason: "timeout" });
  });

  it("bad JSON and schema failures are invalid, never partially shown", async () => {
    expect((await fetchManifest({ ...opts, fetchImpl: reply("<html>") as unknown as typeof fetch })).kind).toBe("invalid");
    const bad = readFileSync(join(import.meta.dirname, "../fixtures/manifest/manifest-invalid-absolute-url.json"), "utf8");
    expect((await fetchManifest({ ...opts, fetchImpl: reply(bad) as unknown as typeof fetch })).kind).toBe("invalid");
  });

  it("concurrent callers share one request", async () => {
    const f = vi.fn(reply(valid));
    const o = { ...opts, fetchImpl: f as unknown as typeof fetch };
    const [a, b] = await Promise.all([loadManifest(o), loadManifest(o)]);
    expect(a).toBe(b);
    expect(f).toHaveBeenCalledTimes(1);
  });
});
