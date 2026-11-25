# ApexVoid Photobooth — website

The public product website for **ApexVoid Photobooth**, the local-first photobooth application for Windows.
Target address: **https://photobooth.apexvoid.net** (not deployed yet).

It introduces the application, describes what it does today, and is the foundation for the future download and
documentation centre. It is a static site: no backend, database, accounts, CMS, analytics or tracking.

## Branches

This repository has three independent lifecycles:

| Branch      | Contains                                                                  |
| ----------- | ------------------------------------------------------------------------- |
| `master`    | The stable desktop application and its official releases (default branch) |
| `community` | Desktop application feature development                                   |
| `webpage`   | **This website only** — an orphan branch with no desktop history          |

`webpage` contains nothing from the desktop application: no Go, Wails, Canon SDK, FFmpeg, desktop frontend, drivers,
desktop tests or desktop CI. It builds on any Linux machine with Node.js. Never merge `webpage` into an application
branch or the other way round. Website feature branches are named `webpage/<topic>` and open pull requests into `webpage`.

## Stack

- [Astro 7](https://astro.build) with static output (every route pre-rendered to `dist/<route>/index.html`)
- TypeScript (strictest Astro preset, TypeScript 6 — `@astrojs/check` and `typescript-eslint` do not support 7 yet)
- Tailwind CSS 4 on top of CSS custom-property design tokens
- Lucide icons (`@lucide/astro`), self-hosted Inter and Fraunces variable fonts (`@fontsource-variable`)
- No UI framework at runtime: one small module script (theme, mobile menu, language memory) plus a tiny inline
  pre-paint theme script
- Vitest (unit), Playwright + axe-core (browser journeys and accessibility), Prettier, ESLint

Node.js **22.23.4 or newer** (Astro 7 requirement). CI uses Node 22.

## Commands

```bash
npm ci              # install exactly the locked dependencies
npm run dev         # development server with hot reload (http://localhost:4321)
npm run check       # astro check + tsc (types, Astro templates, content schema)
npm run lint        # prettier --check + eslint
npm run test        # unit tests (Vitest)
npm run build       # static build to dist/, then the internal link and route check
npm run test:e2e    # browser tests against dist/ (run `npm run build` first)
npm run preview     # serve dist/ with Astro's preview server
npm run format      # apply Prettier
```

`npm run test:e2e` needs Chromium once: `npx playwright install chromium`. The tests serve `dist/` with
`scripts/serve-dist.mjs`, which follows the same rules as the production Nginx configuration (directory index,
`/path` → `/path/` redirect, `404.html`). Astro 7's `preview` runs in the background when it has no terminal, so it is
not used by the test runner.

## Layout

```text
src/
  pages/            routes: one thin file per page and language (English at /, Vietnamese under /vi/)
  views/            the page bodies, shared by both languages (HomeView, FeaturesView, …)
  components/
    common/         Logo, Screenshot, SectionHeading, StatusPill, GithubMark
    layout/         Seo (meta, canonical, hreflang, Open Graph, JSON-LD), Footer, PageIntro
    navigation/     Header (desktop + mobile menu), LanguageSwitch, ThemeSwitch
    marketing/      home page sections (Hero, Experience, FeatureGroups, Journey, Hardware, LocalFirst, DownloadCta)
    product/        DownloadPanel, ReleaseList, PlatformFacts
  i18n/             en.ts (source), vi.ts, locale and route helpers
  lib/              releases (data model), seo, screenshots, client scripts (theme, menu)
  content/          releases.json — empty until the first official release
  config/site.ts    URLs and product facts
  styles/global.css design tokens, base styles, a few component classes
public/             favicon, touch icon, Open Graph image, robots.txt
scripts/            link checker, static server, brand image generator, site screenshots
test/unit, test/e2e
deploy/             Nginx config and Dockerfile for the future container (not used by CI)
```

## Content and product claims

Every product statement comes from the desktop application's own documentation on `master` (README, `docs/INSTALL`,
`CAMERAS`, `PRINTING`, `FRAMES`, `THIRD-PARTY`, `HARDWARE_TESTS`). Rules:

- Describe what the software does; never present a software integration as a verified device. The hardware section
  has three states: _software integration_, _physically verified_ (empty today) and _needs hardware testing_.
- No invented release versions, sizes, dates, checksums, download counts, prices or ratings — on pages or in
  structured data.
- macOS and Linux are not supported, and the application has **no published licence**: do not call it open source.

When the application changes, update `src/i18n/en.ts` first, then `vi.ts`.

## Translations

- `src/i18n/en.ts` is the source dictionary. `vi.ts` must have the same shape: the `Dict` type makes a missing or extra
  key a compile error, and `test/unit/i18n.test.ts` also compares list lengths and rejects empty/placeholder strings.
- Components never hard-code text; they call `useDict(locale)`.
- To add a page: add the route to `routes` in `src/i18n/locales.ts`, a view in `src/views/`, and one file each in
  `src/pages/` and `src/pages/vi/`; add its route to `scripts/check-links.mjs` and the e2e route list.
- The language switcher links to the same page in the other language and remembers the choice in `localStorage`
  (`apx-locale`). The site never redirects on the browser language.
- The desktop documentation is English-only; the Vietnamese docs page says so.

## Themes

Light, dark and system. Tokens are CSS custom properties in `src/styles/global.css` (`--bg`, `--surface`, `--fg`,
`--fg-muted`, `--brand`, `--accent-text`, `--border`, `--focus`, radii, shadows, motion…), exposed to Tailwind as
`bg-surface`, `text-muted`, `border-line` and so on. Components never use raw colours.

An inline script in `<head>` (`THEME_BOOTSTRAP` in `src/lib/client/theme.ts`) applies the stored choice (`apx-theme`) or
the OS preference before first paint, so there is no flash. Without JavaScript the OS preference applies via
`prefers-color-scheme`. Motion is minimal and disabled with `prefers-reduced-motion`.

## Screenshots

`src/assets/screenshots/<name>.<locale>.png` are real renders of the desktop application, made with the application's
own browser harness (`cmd/uxharness` and `frontend/harness/*.mjs` on `master`) with the **simulated camera and fake
printer** — the app's "SIMULATED CAMERA" ribbon is visible on purpose. They are not evidence of hardware support, and
every page that shows them says so. Source: `master` at `e1a7aa1` (2026-11-25), see `src/config/site.ts`.

To refresh them, check out `master` in a separate worktree, build the harness as described at the top of
`frontend/harness/shots.mjs`, capture, and copy the PNGs here with the same names. A Vietnamese page uses the `.vi`
capture when it exists and the English one otherwise. Astro converts them to responsive AVIF/WebP with explicit
dimensions at build time. `node scripts/generate-brand.mjs` regenerates the favicon, touch icon and Open Graph image.

`scripts/site-screenshots.mjs` captures the website itself (desktop and mobile, light and dark) for review.

## Releases (future GitHub Releases integration)

`src/lib/releases.ts` defines the release model (`Release`, `ReleaseAsset`) and the logic the UI uses
(`latestStable`, `windowsAsset`, `formatBytes`). The data source is the `releases` content collection
(`src/content/releases.json`, validated by the schema in `src/content.config.ts`). It is an empty array today, so the
download and releases pages show the honest empty state. WEB-2 will generate this file from the GitHub Releases API at
build time; the components need no change.

## Deployment contract (not deployed yet)

| Property      | Value                                |
| ------------- | ------------------------------------ |
| Domain        | `photobooth.apexvoid.net`            |
| Container     | `apexvoid-photobooth-webpage`        |
| Internal port | `80`                                 |
| Network       | `routing` (shared Docker network)    |
| Public ports  | managed by the central routing proxy |

`deploy/nginx.conf` and `deploy/Dockerfile` describe that container: Nginx serves `dist/` with
`try_files $uri $uri/ =404`, `404.html`, immutable caching for `/_astro/` and basic security headers. The configuration
was checked locally with `nginx:1.27-alpine`. DNS, TLS and the routing, ansible-library and action-library repositories
are untouched; that integration is WEB-2.

## Known limitations

- No official release exists, so the site cannot offer a download; it points to GitHub and to unsigned CI test builds.
- Documentation is linked to GitHub (English) rather than hosted here.
- Screenshots come from the browser harness with simulated devices; no photo of real hardware is shown.
- No Content-Security-Policy header yet (the inline theme script needs a hash; planned with the deployment).
- Lighthouse results were measured on a local Nginx container with simulated throttling, not on the production host.
