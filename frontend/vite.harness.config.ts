import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";

// Dev/screenshot build only: same app, but @wailsio/runtime is replaced by the HTTP shim for cmd/uxharness.
export default defineConfig({
  plugins: [react()],
  base: "./",
  resolve: { alias: { "@wailsio/runtime": fileURLToPath(new URL("./harness/wails-shim.ts", import.meta.url)) } },
  build: { outDir: "dist-harness", emptyOutDir: true, target: "es2022" },
});
