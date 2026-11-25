import { LOCALE_KEY, THEME_KEY, parsePref, resolveTheme, type ThemePref } from "./theme";

/* -------- theme -------- */
const root = document.documentElement;
const mq = window.matchMedia("(prefers-color-scheme: dark)");

function store(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* storage blocked: the choice still applies to this page */
  }
}

function applyTheme(pref: ThemePref): void {
  const theme = resolveTheme(pref, mq.matches);
  root.dataset.theme = theme;
  root.dataset.themePref = pref;
  root.style.colorScheme = theme;
  document.querySelectorAll<HTMLButtonElement>("[data-theme-value]").forEach((b) => {
    b.setAttribute("aria-pressed", String(b.dataset.themeValue === pref));
  });
}

applyTheme(parsePref(root.dataset.themePref));
document.querySelectorAll<HTMLButtonElement>("[data-theme-value]").forEach((b) => {
  b.addEventListener("click", () => {
    const pref = parsePref(b.dataset.themeValue);
    store(THEME_KEY, pref);
    applyTheme(pref);
  });
});
mq.addEventListener("change", () => {
  if (parsePref(root.dataset.themePref) === "system") applyTheme("system");
});

/* -------- language: remember the reader's explicit choice (never redirect on our own) -------- */
document.querySelectorAll<HTMLAnchorElement>("[data-locale-link]").forEach((a) => {
  a.addEventListener("click", () => store(LOCALE_KEY, a.dataset.localeLink ?? "en"));
});

/* -------- mobile navigation -------- */
const toggle = document.querySelector<HTMLButtonElement>("[data-menu-toggle]");
const panel = document.querySelector<HTMLElement>("[data-mobile-nav]");

function setOpen(open: boolean, focusToggle = false): void {
  if (!toggle || !panel) return;
  panel.hidden = !open;
  toggle.setAttribute("aria-expanded", String(open));
  toggle.setAttribute("aria-label", (open ? toggle.dataset.labelClose : toggle.dataset.labelOpen) ?? "");
  toggle.querySelector(".icon-open")?.classList.toggle("hidden", open);
  toggle.querySelector(".icon-close")?.classList.toggle("hidden", !open);
  document.body.classList.toggle("menu-open", open);
  if (open) panel.querySelector<HTMLElement>("a, button")?.focus();
  else if (focusToggle) toggle.focus();
}

toggle?.addEventListener("click", () => setOpen(toggle.getAttribute("aria-expanded") !== "true"));
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && toggle?.getAttribute("aria-expanded") === "true") setOpen(false, true);
});
panel?.addEventListener("click", (e) => {
  if ((e.target as HTMLElement).closest("a")) setOpen(false);
});
window.matchMedia("(min-width: 64rem)").addEventListener("change", (e) => {
  if (e.matches) setOpen(false);
});
