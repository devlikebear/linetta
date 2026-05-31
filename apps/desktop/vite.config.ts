import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

/// <reference types="vitest" />

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
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes("node_modules/@tiptap") || id.includes("node_modules/prosemirror")) {
            return "editor";
          }
          if (id.includes("node_modules/lucide-react")) {
            return "icons";
          }
          if (id.includes("node_modules/react") || id.includes("node_modules/react-router-dom")) {
            return "vendor";
          }
        },
      },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["src/test/setup.ts"],
    globals: true,
    css: true,
  },
});
