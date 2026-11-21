import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The Go binary embeds cmd/photobooth/frontend_dist (see cmd/photobooth/main.go).
export default defineConfig({
  plugins: [react()],
  base: "./",
  build: { outDir: "../cmd/photobooth/frontend_dist", emptyOutDir: true, target: "es2022" },
  test: { environment: "jsdom", clearMocks: true },
});
