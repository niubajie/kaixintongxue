// 浏览器里的降级实现：不报错，也不假装支持。
// 组件通过 capabilities 判断该不该渲染对应入口。

import type { DesktopBridge } from "./types";

export const webBridge: DesktopBridge = {
  kind: "web",

  capabilities: {
    windowControls: false,
    appVersion: false
  },

  async getAppVersion(): Promise<string> {
    return "web";
  },

  window: {
    async minimize(): Promise<void> {
      // 浏览器里没有窗口控制
    },
    async toggleMaximize(): Promise<void> {
      // 同上
    },
    async close(): Promise<void> {
      // 同上
    }
  }
};
