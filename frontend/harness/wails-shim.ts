// Screenshot/E2E harness only (vite alias, see vite.harness.config.ts): stands in for @wailsio/runtime and talks to
// cmd/uxharness (the real Go service) over HTTP + server-sent events.
export const Call = {
  async ByName(name: string, ...args: unknown[]) {
    const method = name.split(".").pop()!;
    const r = await fetch(`/rpc/${method}`, { method: "POST", body: JSON.stringify(args) });
    if (!r.ok) throw new Error((await r.text()).trim());
    const t = await r.text();
    return t ? JSON.parse(t) : null;
  },
};
const subs = new Map<string, Set<(e: { data: unknown }) => void>>();
let es: EventSource | null = null;
export const Events = {
  On(name: string, cb: (e: { data: unknown }) => void) {
    if (!es) { es = new EventSource("/events"); es.onmessage = (m) => { const e = JSON.parse(m.data); subs.get(e.name)?.forEach((f) => f({ data: e.data })); }; }
    if (!subs.has(name)) subs.set(name, new Set());
    subs.get(name)!.add(cb);
    return () => { subs.get(name)?.delete(cb); };
  },
};
let fs = false;
export const Window = { Fullscreen: async () => { fs = true; }, UnFullscreen: async () => { fs = false; }, IsFullscreen: async () => fs };
