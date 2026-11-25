/** Theme preference: "light" | "dark" | "system". Pure helpers, unit-tested; the DOM wiring is in ui.ts. */
export type ThemePref = "light" | "dark" | "system";
export const THEME_KEY = "apx-theme";
export const LOCALE_KEY = "apx-locale";

export function parsePref(v: string | null | undefined): ThemePref {
  return v === "light" || v === "dark" || v === "system" ? v : "system";
}

export function resolveTheme(pref: ThemePref, systemDark: boolean): "light" | "dark" {
  return pref === "system" ? (systemDark ? "dark" : "light") : pref;
}

/** The inline <head> script: runs before first paint so there is no flash of the wrong theme.
 *  Kept tiny and dependency-free; storage access is guarded (private mode, blocked storage). */
export const THEME_BOOTSTRAP = `(function(){var d=document.documentElement,p="system";try{p=localStorage.getItem("${THEME_KEY}")||"system"}catch(e){}if(p!=="light"&&p!=="dark")p="system";var dark=p==="dark"||(p==="system"&&window.matchMedia&&matchMedia("(prefers-color-scheme: dark)").matches);d.dataset.theme=dark?"dark":"light";d.dataset.themePref=p;d.style.colorScheme=dark?"dark":"light"})()`;
