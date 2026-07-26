import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  base: "/",
  build: {
    outDir: "build",
    // Page components are split at React Router boundaries. Keep framework
    // packages under Rollup's normal chunking to avoid circular vendor chunks.
    sourcemap: false,
  },
  plugins: [react()],
  server: {
    cors: false,
    port: 3100,
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
    setupFiles: "./src/test/setup.ts",
  },
});
