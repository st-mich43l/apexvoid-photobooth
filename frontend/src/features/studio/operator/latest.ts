import { useRef } from "react";

/** "Latest request wins": a slow, older answer can never overwrite a newer one. */
export function useLatest() {
  const n = useRef(0);
  return { next: () => ++n.current, isCurrent: (id: number) => id === n.current };
}
