/** Facts about the product and this website. Every product statement on the site is derived from
 *  the desktop application's own documentation on `master`; keep them in step with it. */
export const SITE_URL = "https://photobooth.apexvoid.net";
export const REPO_URL = "https://github.com/st-mich43l/apexvoid-photobooth";
export const ISSUES_URL = `${REPO_URL}/issues`;
export const CI_URL = `${REPO_URL}/actions/workflows/ci.yml?query=branch%3Amaster`;
/** Desktop documentation lives on the application branch until the website hosts it. */
export const DOCS_BASE = `${REPO_URL}/blob/master/docs`;
export const PRODUCT_NAME = "ApexVoid Photobooth";

/** Where the screenshots on this site come from. They are real renders of the desktop UI, made in the
 *  application's own browser test harness with its simulated camera and fake printer — not photos of hardware. */
export const SCREENSHOT_SOURCE = {
  commit: "e1a7aa1",
  date: "2026-11-25",
  harness: "cmd/uxharness + frontend/harness (master)",
} as const;

export const PLATFORM = {
  os: "Windows 10 / 11",
  arch: "x64",
  form: "portable",
} as const;
