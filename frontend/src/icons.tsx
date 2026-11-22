import type { ReactNode } from "react";

/** One consistent line-icon set (24px grid, 1.7 stroke) so the UI does not depend on font glyphs or emoji. */
const PATHS: Record<string, ReactNode> = {
  dashboard: <><rect x="3.5" y="3.5" width="7" height="8" rx="2" /><rect x="13.5" y="3.5" width="7" height="5" rx="2" /><rect x="13.5" y="11.5" width="7" height="9" rx="2" /><rect x="3.5" y="14.5" width="7" height="6" rx="2" /></>,
  camera: <><path d="M4 8.5A2.5 2.5 0 0 1 6.5 6h1.2l1.1-1.6A1.5 1.5 0 0 1 10 3.8h4a1.5 1.5 0 0 1 1.2.6L16.3 6h1.2A2.5 2.5 0 0 1 20 8.5v8a2.5 2.5 0 0 1-2.5 2.5h-11A2.5 2.5 0 0 1 4 16.5z" /><circle cx="12" cy="12.5" r="3.6" /></>,
  photo: <><rect x="3.5" y="4.5" width="17" height="15" rx="3" /><circle cx="9" cy="10" r="1.7" /><path d="m4 17 4.6-4.4a1.6 1.6 0 0 1 2.2 0L14 15.5l1.6-1.5a1.6 1.6 0 0 1 2.2 0L20.5 17" /></>,
  video: <><rect x="3.5" y="6" width="12.5" height="12" rx="3" /><path d="m16 10.5 4.2-2.4a.8.8 0 0 1 1.2.7v6.4a.8.8 0 0 1-1.2.7L16 13.5" /></>,
  gallery: <><rect x="3.5" y="3.5" width="7.2" height="7.2" rx="2" /><rect x="13.3" y="3.5" width="7.2" height="7.2" rx="2" /><rect x="3.5" y="13.3" width="7.2" height="7.2" rx="2" /><rect x="13.3" y="13.3" width="7.2" height="7.2" rx="2" /></>,
  frames: <><rect x="6" y="2.8" width="12" height="18.4" rx="2.4" /><rect x="8.6" y="5.4" width="6.8" height="3.4" rx="1" /><rect x="8.6" y="10.3" width="6.8" height="3.4" rx="1" /><rect x="8.6" y="15.2" width="6.8" height="3.4" rx="1" /></>,
  printer: <><path d="M7 9V4.5h10V9" /><rect x="3.5" y="9" width="17" height="8" rx="2.4" /><rect x="7" y="14" width="10" height="6" rx="1.4" /><circle cx="17.2" cy="11.8" r=".7" /></>,
  settings: <><path d="M4 7h9M17 7h3M4 17h3M11 17h9" /><circle cx="15" cy="7" r="2.2" /><circle cx="9" cy="17" r="2.2" /></>,
  diagnostics: <path d="M3.5 12h4l2.2-6 4.2 12 2.4-6h4.2" />,
  storage: <><path d="M5.5 14 7.6 5.9A2 2 0 0 1 9.5 4.5h5a2 2 0 0 1 1.9 1.4L18.5 14" /><rect x="3.5" y="14" width="17" height="6" rx="2.2" /><circle cx="17" cy="17" r=".8" /></>,
  sparkle: <path d="M12 3.5l1.9 5.1 5.1 1.9-5.1 1.9L12 17.5l-1.9-5.1L5 10.5l5.1-1.9zM18.5 15.5l.8 2.1 2.1.8-2.1.8-.8 2.1-.8-2.1-2.1-.8 2.1-.8z" />,
  upload: <><path d="M12 15.5V4.5M7.6 8.6 12 4.2l4.4 4.4" /><path d="M4.5 14.5v3A2.5 2.5 0 0 0 7 20h10a2.5 2.5 0 0 0 2.5-2.5v-3" /></>,
  play: <path d="M8 5.6v12.8a.9.9 0 0 0 1.4.8l10-6.4a.9.9 0 0 0 0-1.5l-10-6.4A.9.9 0 0 0 8 5.6z" />,
  check: <path d="m5 12.5 4.4 4.4L19 7.2" />,
  back: <path d="M14.5 5.5 8 12l6.5 6.5" />,
  retake: <><path d="M4.5 12a7.5 7.5 0 0 1 12.9-5.2L20 9.5" /><path d="M20 4.5v5h-5" /><path d="M19.5 12a7.5 7.5 0 0 1-12.9 5.2L4 14.5" /><path d="M4 19.5v-5h5" /></>,
  home: <><path d="M4 11.2 12 4.5l8 6.7" /><path d="M6 10v8.5A1.5 1.5 0 0 0 7.5 20h9a1.5 1.5 0 0 0 1.5-1.5V10" /></>,
  arrow: <path d="M5 12h14M13.5 6.5 19 12l-5.5 5.5" />,
  chevdown: <path d="m6.5 9.5 5.5 5.5 5.5-5.5" />,
  swap: <><path d="M5 8h13M14.5 4.5 18 8l-3.5 3.5" /><path d="M19 16H6M9.5 12.5 6 16l3.5 3.5" /></>,
  exit: <><path d="M10 4.5H6.5A2.5 2.5 0 0 0 4 7v10a2.5 2.5 0 0 0 2.5 2.5H10M15 8l4 4-4 4M19 12H9.5" /></>,
};

export function Icon({ name, size = 20, className }: { name: keyof typeof PATHS | string; size?: number; className?: string }) {
  return (
    <svg className={`icon ${className ?? ""}`} width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">{PATHS[name]}</svg>
  );
}
