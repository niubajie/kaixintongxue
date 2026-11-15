import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  // 关掉清屏，否则 Rust 的编译错误会被 Vite 刷掉
  clearScreen: false,
  // Tauri 用自定义协议加载页面，相对路径更安全
  base: "./",
  server: {
    // 必须是固定端口：tauri.conf.json 的 devUrl 写死了 1420
    port: 1420,
    strictPort: true,
    host: "127.0.0.1",
    watch: {
      // 不忽略的话 Rust 每次编译都会触发前端热重载，形成死循环
      ignored: [/src-tauri/, /[\\/]target[\\/]/]
    }
  },
  build: {
    // Tauri 用系统 WebView2，版本因机器而异，不能用 esnext
    target: "chrome105",
    outDir: "dist",
    emptyOutDir: true
  }
});
