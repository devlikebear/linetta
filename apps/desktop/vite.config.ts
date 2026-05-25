import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Tauri dev server config: fixed port, no minification of dev assets.
// https://tauri.app/v2/guides/getting-started/setup/vite/
export default defineConfig({
  plugins: [react()],
  clearScreen: false,
  server: {
    port: 1420,
    strictPort: true,
  },
  envPrefix: ["VITE_", "TAURI_"],
  build: {
    target: "es2021",
    minify: !process.env.TAURI_DEBUG ? "esbuild" : false,
    sourcemap: !!process.env.TAURI_DEBUG,
  },
});
