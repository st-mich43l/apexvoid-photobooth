import { useCallback, useEffect, useRef, useState } from "react";

/** Does a scrolling area have more content below the fold? Drives the fade + "more" cue so guests know to keep scrolling. */
export function useMoreBelow(deps: unknown[]) {
  const ref = useRef<HTMLDivElement>(null);
  const [more, setMore] = useState(false);
  const measure = useCallback(() => {
    const el = ref.current;
    if (el) setMore(el.scrollHeight - el.scrollTop - el.clientHeight > 12);
  }, []);
  useEffect(() => {
    measure();
    const el = ref.current;
    if (!el) return;
    const ro = typeof ResizeObserver !== "undefined" ? new ResizeObserver(measure) : null;
    ro?.observe(el);
    Array.from(el.children).forEach((c) => ro?.observe(c));
    window.addEventListener("resize", measure);
    return () => { ro?.disconnect(); window.removeEventListener("resize", measure); };
  }, [measure, ...deps]); // eslint-disable-line react-hooks/exhaustive-deps
  return { ref, more, onScroll: measure };
}
