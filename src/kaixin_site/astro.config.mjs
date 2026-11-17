import { defineConfig } from "astro/config";

// 官网是纯静态站：零 JS 优先，首屏 HTML 可被搜索引擎直接抓取。
// 需要登录态或实时数据的部分不放这里，交给产品端（桌面 / 网页 / 手机）。
export default defineConfig({
  site: "https://kaixintongxue.example.com",
  output: "static",
  build: {
    assets: "assets"
  },
  compressHTML: true
});
