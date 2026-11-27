import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// 1个亿资源圈 · 管理后台
// 独立工程：直接对接 Go 服务（server/，默认 8080）。
export default defineConfig({
  plugins: [react()],
  base: "/",
  server: {
    port: 5183,
    proxy: {
      "/api": "http://127.0.0.1:8080"
    }
  },
  build: {
    outDir: "dist",
    emptyOutDir: true
  }
});
