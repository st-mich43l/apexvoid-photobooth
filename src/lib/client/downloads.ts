import {
  downloadUrl,
  downloadView,
  archiveReleases,
  formatBytes,
  retainedPublicReleases,
  windowsAsset,
  type Manifest,
  type Release,
  type ReleaseAsset,
} from "../releases";
import { loadManifest, resetManifestCache, type ManifestResult } from "./release-service";

/**
 * Fills the download panel and the release list from the runtime manifest. Only validated values are
 * written, always as text (`textContent`) or as links built from validated relative paths — never as HTML.
 */
const lang = document.documentElement.lang.startsWith("vi") ? "vi-VN" : "en-GB";
const dateFmt = new Intl.DateTimeFormat(lang, { dateStyle: "long", timeZone: "UTC" });

const panels = [...document.querySelectorAll<HTMLElement>("[data-download-panel], [data-release-list]")];

function show(root: HTMLElement, state: string): void {
  root.dataset.state = state;
  root.removeAttribute("aria-busy");
  root.querySelectorAll<HTMLElement>("[data-when]").forEach((el) => {
    el.hidden = !(el.dataset.when ?? "").split(" ").includes(state);
  });
}

function fill(scope: ParentNode, r: Release, a: ReleaseAsset, flags: Record<string, boolean>): void {
  const text: Record<string, string> = {
    name: r.name,
    version: r.version,
    tag: r.tag,
    date: dateFmt.format(new Date(r.publishedAt)),
    filename: a.filename,
    size: formatBytes(a.size),
    sha256: a.sha256,
  };
  scope.querySelectorAll<HTMLElement>("[data-field]").forEach((el) => {
    const k = el.dataset.field ?? "";
    if (k in text) el.textContent = text[k] ?? "";
    if (k === "date") el.setAttribute("datetime", r.publishedAt);
  });
  const href: Record<string, string> = {
    download: downloadUrl(a.path),
    notes: downloadUrl(r.notesPath),
    checksums: downloadUrl(r.checksumsPath),
    notices: downloadUrl(r.noticesPath),
  };
  scope.querySelectorAll<HTMLAnchorElement>("a[data-href]").forEach((el) => {
    const h = href[el.dataset.href ?? ""];
    if (h) el.href = h;
  });
  scope.querySelectorAll<HTMLElement>("[data-if]").forEach((el) => {
    el.hidden = !flags[el.dataset.if ?? ""];
  });
}

function assetFlags(a: ReleaseAsset): Record<string, boolean> {
  return { signed: a.signed, unsigned: !a.signed, canon: a.includesCanonSdk, "no-canon": !a.includesCanonSdk };
}

function renderPanel(root: HTMLElement, m: Manifest): void {
  const v = downloadView(m);
  if (v.state === "empty") return show(root, "empty");
  fill(root, v.release, v.asset, { stable: true, preview: false, ...assetFlags(v.asset) });
  show(root, v.state);
}

function renderList(root: HTMLElement, m: Manifest): void {
  const list = root.querySelector<HTMLOListElement>("[data-items]");
  const tpl = root.querySelector<HTMLTemplateElement>("template[data-item]");
  if (!list || !tpl) return;
  const releases = root.dataset.mode === "archive" ? archiveReleases(m) : retainedPublicReleases(m);
  const items = releases.flatMap((r) => {
    const a = windowsAsset(r);
    if (!a) return [];
    const li = tpl.content.cloneNode(true) as DocumentFragment;
    fill(li, r, a, { current: r.version === m.latest.stable, preview: false, ...assetFlags(a) });
    li.querySelectorAll("[data-if][hidden]").forEach((el) => el.remove()); // fresh copies: drop what does not apply
    return [li];
  });
  list.replaceChildren(...items);
  show(root, items.length ? "list" : "empty");
}

function render(result: ManifestResult): void {
  for (const root of panels) {
    if (result.kind !== "ok") {
      show(root, result.kind);
      continue;
    }
    if (root.hasAttribute("data-download-panel")) renderPanel(root, result.manifest);
    else renderList(root, result.manifest);
  }
  document.documentElement.dataset.releases = result.kind;
}

function run(): void {
  for (const root of panels) {
    show(root, "loading");
    root.setAttribute("aria-busy", "true");
  }
  void loadManifest().then(render);
}

if (panels.length) {
  document.querySelectorAll<HTMLButtonElement>("[data-retry]").forEach((b) =>
    b.addEventListener("click", () => {
      resetManifestCache();
      run();
    }),
  );
  run();
}
