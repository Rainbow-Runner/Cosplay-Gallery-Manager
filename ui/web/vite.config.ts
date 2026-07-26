import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  base: "",
  build: {
    outDir: "build",
    // Splitting React, Apollo and react-intl by package name creates circular
    // chunks because those packages import each other. Route-level lazy chunks
    // can be introduced once the browse pages are complete.
    sourcemap: false,
  },
  plugins: [react()],
  server: {
    cors: false,
    port: 3100,
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
  },
});
