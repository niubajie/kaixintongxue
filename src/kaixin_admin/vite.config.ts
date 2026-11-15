import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// 后台页面由 FastAPI 在 /admin/ 下托管，构建产物输出到服务端 web/admin。
export default defineConfig({
  plugins: [react()],
  base: "/admin/",
  server: {
    port: 5182,
    proxy: {
      "/api": "http://127.0.0.1:5080"
    }
  },
  build: {
    outDir: "dist",
    emptyOutDir: true
  }
});
