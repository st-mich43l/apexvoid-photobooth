import { useEffect, useState } from "react";

export type Orientation = "portrait" | "landscape";

/** Portrait or landscape of the window right now (kiosks are mounted either way). Falls back to the aspect ratio. */
export function useOrientation(): Orientation {
  const read = (): Orientation => {
    if (typeof window.matchMedia === "function") return window.matchMedia("(orientation: portrait)").matches ? "portrait" : "landscape";
    return window.innerHeight > window.innerWidth ? "portrait" : "landscape";
  };
  const [o, setO] = useState<Orientation>(read);
  useEffect(() => {
    const f = () => setO(read());
    window.addEventListener("resize", f);
    const mq = typeof window.matchMedia === "function" ? window.matchMedia("(orientation: portrait)") : null;
    mq?.addEventListener?.("change", f);
    return () => { window.removeEventListener("resize", f); mq?.removeEventListener?.("change", f); };
  }, []);
  return o;
}
