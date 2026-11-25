import { getCollection } from "astro:content";
import type { Release } from "./releases";

/** All published releases (build time). Empty until the first official GitHub Release. */
export async function loadReleases(): Promise<Release[]> {
  const entries = await getCollection("releases");
  return entries.map((e) => {
    const r: Release = {
      tag: e.data.tag,
      name: e.data.name,
      publishedAt: e.data.publishedAt,
      prerelease: e.data.prerelease,
      notesUrl: e.data.notesUrl,
      assets: [],
    };
    r.assets = e.data.assets.map((a) =>
      a.sha256 ? { name: a.name, size: a.size, url: a.url, sha256: a.sha256 } : { name: a.name, size: a.size, url: a.url },
    );
    return r;
  });
}
