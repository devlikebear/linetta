import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

/// <reference types="vitest" />

// Tauri dev server config: fixed port (1430, so it can run beside Linetta's
// 1420), no minification of dev assets.
export default defineConfig({
  plugins: [react()],
  clearScreen: false,
  server: {
    port: 1430,
    strictPort: true,
  },
  envPrefix: ["VITE_", "TAURI_"],
  build: {
    target: "es2021",
    minify: !process.env.TAURI_DEBUG ? "esbuild" : false,
    sourcemap: !!process.env.TAURI_DEBUG,
  },
  test: {
    environment: "jsdom",
    setupFiles: ["src/test/setup.ts"],
    globals: true,
    css: true,
  },
});
