// Tauri 实现。
// 关键约束：@tauri-apps/* 全部用动态 import，
// 否则将来打成 Web 版时会把 Tauri 代码混进产物。

import { DEFAULT_LOCAL_PORT, type DesktopBridge } from "./types";

async function currentWindow() {
  const { getCurrentWindow } = await import("@tauri-apps/api/window");
  return getCurrentWindow();
}

export function createTauriBridge(): DesktopBridge {
  return {
    kind: "tauri",

    capabilities: {
      windowControls: true,
      appVersion: true,
      localService: true,
      pickFolder: true,
      pickFiles: true
    },

    async getAppVersion(): Promise<string> {
      const { invoke } = await import("@tauri-apps/api/core");
      // 对应 src-tauri/src/lib.rs 里的 app_version 命令
      return invoke<string>("app_version");
    },

    async pickFolder(): Promise<string | null> {
      const { open } = await import("@tauri-apps/plugin-dialog");
      const picked = await open({ directory: true, multiple: false, title: "选择资料文件夹" });
      return typeof picked === "string" && picked ? picked : null;
    },

    async pickFiles(): Promise<string[]> {
      const { open } = await import("@tauri-apps/plugin-dialog");
      const picked = await open({ multiple: true, directory: false, title: "选择附件" });
      if (!picked) {
        return [];
      }
      return Array.isArray(picked) ? picked : [picked];
    },

    async getLocalServicePort(): Promise<number> {
      const { invoke } = await import("@tauri-apps/api/core");
      // 对应 src-tauri/src/lib.rs 里的 local_service_port 命令；0 表示 sidecar 启动失败
      const port = await invoke<number>("local_service_port");
      return port > 0 ? port : DEFAULT_LOCAL_PORT;
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
