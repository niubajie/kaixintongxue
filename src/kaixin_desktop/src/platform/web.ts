// 浏览器里的降级实现：不报错，也不假装支持。
// 组件通过 capabilities 判断该不该渲染对应入口。

import { DEFAULT_LOCAL_PORT, type DesktopBridge } from "./types";

export const webBridge: DesktopBridge = {
  kind: "web",

  capabilities: {
    windowControls: false,
    appVersion: false,
    localService: false,
    pickFolder: false,
    openExternal: true,
    pickFiles: false,
    updater: false
  },

  updater: {
    supported: false,
    async check(): Promise<never> {
      throw new Error("浏览器环境不支持自动更新，请下载桌面安装包");
    },
    async downloadAndInstall(): Promise<never> {
      throw new Error("浏览器环境不支持自动更新");
    },
    async relaunch(): Promise<never> {
      throw new Error("浏览器环境不支持重启应用");
    }
  },

  async getAppVersion(): Promise<string> {
    return "web";
  },

  async openExternal(url: string): Promise<void> {
    const opened = window.open(url, "_blank", "noopener");
    if (!opened) {
      throw new Error("浏览器拦截了弹窗，请手动打开：" + url);
    }
  },

  async pickFolder(): Promise<string | null> {
    // 浏览器没有系统文件夹选择器
    return null;
  },

  async pickFiles(): Promise<string[]> {
    // 浏览器里用 <input type=file>，此处不实现
    return [];
  },

  async getAppExePath(): Promise<string> {
    return "";
  },

  async getLocalServicePort(): Promise<number> {
    // 浏览器里没有 sidecar，使用本机默认端口（需要手动启动 kaixin-local）
    return DEFAULT_LOCAL_PORT;
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
