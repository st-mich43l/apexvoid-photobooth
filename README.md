# ApexVoid Photobooth — website

The public product website for **ApexVoid Photobooth**, the local-first photobooth application for Windows.
Live at **https://photobooth.apexvoid.net**.

It introduces the application, describes what it does today, and offers official Windows releases for download. It is a
static site: no backend, database, accounts, CMS, analytics or tracking. The only runtime data is the release manifest
`/downloads/index.json`, read by the browser.

## Branches

This repository has three independent lifecycles:

| Branch      | Contains                                                                  |
| ----------- | ------------------------------------------------------------------------- |
| `master`    | The stable desktop application and its official releases (default branch) |
| `community` | Desktop application feature development                                   |
| `webpage`   | **This website only** — an orphan branch with no desktop history          |

`webpage` contains nothing from the desktop application: no Go, Wails, Canon SDK, FFmpeg, desktop frontend, drivers,
desktop tests or desktop CI. It builds on any Linux machine with Node.js. Never merge `webpage` into an application
branch or the other way round. Website feature branches are named `webpage-<topic>` (Git cannot hold `webpage/<topic>` next to a `webpage` branch) and open pull requests into `webpage`.

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
    common/         Logo, Screenshot, SectionHeading, GithubMark
    layout/         Seo (meta, canonical, hreflang, Open Graph, JSON-LD), Footer, PageIntro
    navigation/     Header (desktop + mobile menu), LanguageSwitch, ThemeSwitch
    marketing/      home page sections (Hero, Experience, FeatureGroups, Journey, Hardware, LocalFirst, DownloadCta)
    product/        DownloadPanel, ReleaseList, PlatformFacts
  i18n/             en.ts (source), vi.ts, locale and route helpers
  lib/              releases (manifest contract), seo, screenshots, client scripts (theme, menu, release service, downloads)
  config/site.ts    URLs and product facts
  styles/global.css design tokens, base styles, a few component classes
public/             favicon, touch icon, Open Graph image, robots.txt
scripts/            link checker, static server, brand image generator, site screenshots, /downloads/ Nginx test
test/unit, test/e2e, test/fixtures/manifest (release manifests shared with the publisher)
deploy/             Nginx config and Dockerfile of the website container
deployment-template/ docker-compose.yml.j2, rendered on the VPS by ansible-library
```

## Content and product claims

Product copy follows the desktop application's documentation on `master` (README, `docs/INSTALL`, `CAMERAS`,
`PRINTING`, `FRAMES`, `THIRD-PARTY`, `HARDWARE_TESTS`) and confirmed test results. Rules:

- Describe implemented camera workflows confidently, while tying compatibility to supported device families and
  documented requirements. Do not claim a specific camera model, printer or touchscreen was tested unless that exact
  result is confirmed.
- No invented release versions, sizes, dates, checksums, download counts, prices or ratings — on pages or in
  structured data.
- macOS and Linux are not supported. The application is MIT-licensed open source; bundled FFmpeg (LGPL), Canon EDSDK
  (proprietary) and Noto Sans (OFL) keep their own licences — never describe those as MIT.

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
printer** — the app's "SIMULATED CAMERA" ribbon is visible on purpose. The captures illustrate the application
interface and are not photographs of equipment. Source: `master` at `e1a7aa1` (2026-11-25), see `src/config/site.ts`.

To refresh them, check out `master` in a separate worktree, build the harness as described at the top of
`frontend/harness/shots.mjs`, capture, and copy the PNGs here with the same names. A Vietnamese page uses the `.vi`
capture when it exists and the English one otherwise. Astro converts them to responsive AVIF/WebP with explicit
dimensions at build time. `node scripts/generate-brand.mjs` regenerates the favicon, touch icon and Open Graph image.

`scripts/site-screenshots.mjs` captures the website itself (desktop and mobile, light and dark) for review.

## Releases and downloads

Official Windows builds are published by the desktop application's master CI to persistent storage on the VPS
(ansible-library `photobooth_release_storage` / `publish_photobooth_release`, documented in its
`docs/photobooth-release-distribution.md`). The site never contains release data at build time: a release appears
the moment the publisher replaces the manifest, without rebuilding or redeploying the site.

- **Storage → container.** The public storage directory is bind-mounted **read-only** at `/srv/photobooth-downloads`
  (`deployment-template/docker-compose.yml.j2`) and served by `deploy/nginx.conf` under `/downloads/`:
  `index.json` (`no-cache`) and, per release, `releases/v<version>/` with the executable (attachment, immutable,
  byte ranges), `SHA256SUMS`, `RELEASE-NOTES.md`, `THIRD-PARTY-NOTICES.txt` (and optional `LICENSE.txt`,
  `SOURCE.md`). Everything else under `/downloads/` is a 404: no listing, no fallback to the site, GET/HEAD only,
  symlinks refused.
- **Contract.** `src/lib/releases.ts` validates the manifest (schema version 1) with the same rules as the publisher;
  `test/fixtures/manifest/` is a copy of the publisher's fixtures, and the unit tests run all of them. When the
  publisher's schema changes, copy the fixtures again and update both validators together.
- **Runtime.** `src/lib/client/release-service.ts` fetches the manifest once per page (`cache: "no-cache"`, 8 s
  timeout, one retry for network errors and 5xx). `src/lib/client/downloads.ts` fills `DownloadPanel` and
  `ReleaseList`, which render every state on the server: _loading_, _available_ (current stable), _preview-only_
  (no stable yet: the newest preview, labelled), _empty_, _unavailable_ (with retry) and _invalid_ (nothing from the
  manifest is shown). Values are written as text, links only from validated relative paths.
- **What a release shows.** Version, date, file, size, SHA-256, notes, checksums and third-party notices; a
  "not code-signed" notice when `signed` is false (no advice to weaken Windows protections); whether the Canon EDSDK
  is included; how to check the hash in PowerShell.

Tests: `npm test` (contract and service), `npm run test:e2e` (every state through mocked manifests, both languages,
axe), `scripts/test-downloads-nginx.sh` (real Nginx with a read-only test storage: headers, ranges, integrity,
allowlist, traversal, symlinks, methods). CI runs all three.

## Deployment

| Property      | Value                                         |
| ------------- | --------------------------------------------- |
| Domain        | `photobooth.apexvoid.net` (HTTPS via routing) |
| Container     | `apexvoid-photobooth-webpage`                 |
| Image         | `mich43l/apexvoid-photobooth-webpage:<sha7>`  |
| Internal port | `80`                                          |
| Network       | `routing` (shared Docker network)             |
| Public ports  | managed by the central routing proxy          |

A push to `webpage` runs `webpage-ci.yml`. When every check passes, its `deploy` job calls
`st-mich43l/action-library`'s `build-push-deploy.yml`, which

1. builds `deploy/Dockerfile` (Node builds the site, `nginx:1.27-alpine` serves `dist/` with `deploy/nginx.conf`),
2. pushes `mich43l/apexvoid-photobooth-webpage:<short sha>` and `:latest` to Docker Hub (`DOCKERHUB_TOKEN`),
3. runs `ansible-library`'s deploy playbook for the project `apexvoid-photobooth-webpage` (`ANSIBLE_VAULT_PASSWORD`),
   which renders `deployment-template/docker-compose.yml.j2` on the VPS, pulls the image and starts it.

The deploy job pins `ansible-library-ref` to a commit; bump it on purpose when ansible-library changes this project.
The deploy also provisions the release storage (never overwriting a manifest) before the container starts.

Pull requests and `webpage-<topic>` branches are checked but never deployed. To redeploy without a change, run the
workflow manually on `webpage` (Actions → webpage-ci → Run workflow).

The container runs with a read-only root filesystem and a health check. Nginx serves every route as
`<route>/index.html` (`try_files $uri $uri/ =404`), `404.html` for unknown pages, immutable caching for `/_astro/`.
The routing proxy terminates TLS and sets the security headers (the site's own duplicates are dropped there).

## Known limitations

- Downloads need JavaScript to show the current release; without it the page links to the manifest.
- Executables are not code-signed yet.
- Documentation is linked to GitHub (English) rather than hosted here.
- Screenshots come from the browser harness with simulated devices; they show the application interface.
- No Content-Security-Policy header yet (the inline theme script needs a hash).
- Lighthouse results were measured on a local Nginx container with simulated throttling, not on the production host.
