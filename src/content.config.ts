import { defineCollection } from "astro:content";
import { file } from "astro/loaders";
import { z } from "astro/zod";

/**
 * Published releases. Empty until the first official GitHub Release exists; WEB-2 generates this file
 * from the GitHub Releases API. The schema mirrors `src/lib/releases.ts`.
 */
const releases = defineCollection({
  loader: file("src/content/releases.json", {
    parser: (text) => (JSON.parse(text) as { tag: string }[]).map((r) => ({ id: r.tag, ...r })),
  }),
  schema: z.object({
    tag: z.string().regex(/^v\d+\.\d+\.\d+/),
    name: z.string(),
    publishedAt: z.iso.datetime(),
    prerelease: z.boolean(),
    notesUrl: z.url(),
    assets: z.array(
      z.object({
        name: z.string(),
        size: z.number().int().nonnegative(),
        url: z.url(),
        sha256: z
          .string()
          .regex(/^[a-f0-9]{64}$/)
          .optional(),
      }),
    ),
  }),
});

export const collections = { releases };
