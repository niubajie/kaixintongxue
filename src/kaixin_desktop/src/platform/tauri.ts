// Tauri 实现。
// 关键约束：@tauri-apps/* 全部用动态 import，
// 否则将来打成 Web 版时会把 Tauri 代码混进产物。

import type { DesktopBridge } from "./types";

async function currentWindow() {
  const { getCurrentWindow } = await import("@tauri-apps/api/window");
  return getCurrentWindow();
}

export function createTauriBridge(): DesktopBridge {
  return {
    kind: "tauri",

    capabilities: {
      windowControls: true,
      appVersion: true
    },

    async getAppVersion(): Promise<string> {
      const { invoke } = await import("@tauri-apps/api/core");
      // 对应 src-tauri/src/lib.rs 里的 app_version 命令
      return invoke<string>("app_version");
    },

    window: {
      async minimize(): Promise<void> {
        await (await currentWindow()).minimize();
      },
      async toggleMaximize(): Promise<void> {
        await (await currentWindow()).toggleMaximize();
      },
      async close(): Promise<void> {
        await (await currentWindow()).close();
      }
    }
  };
}
