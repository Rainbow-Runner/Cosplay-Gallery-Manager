import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig(() => {
  const proxy = { target: "http://127.0.0.1:9999", changeOrigin: false };
  return {
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
      strictPort: true,
      proxy: {
        "/graphql": proxy,
        "/browse/similar-images": proxy,
        "/session": proxy,
        "/setup/status": proxy,
        "/setup/complete": proxy,
        "/about.json": proxy,
        "/resource": proxy,
        "/manage/coser-assets": proxy,
        "/manage/video-hardware": proxy,
        "/maintenance/status": proxy,
        "/maintenance/path-mappings": proxy,
        "/maintenance/resume": proxy,
      },
    },
  };
});
