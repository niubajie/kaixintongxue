// Tauri 实现。
// 关键约束：@tauri-apps/* 全部用动态 import，
// 否则将来打成 Web 版时会把 Tauri 代码混进产物。

import type { Update } from "@tauri-apps/plugin-updater";
import { DEFAULT_LOCAL_PORT, type DesktopBridge, type UpdateCheckResult } from "./types";

// check() 返回的更新对象要在 downloadAndInstall 时复用；
// type-only import 只参与编译，不会把插件代码打进 Web 产物。
let pendingUpdate: Update | null = null;

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
    openExternal: true,
      pickFiles: true,
      updater: true
    },

    updater: {
      supported: true,

      async check(): Promise<UpdateCheckResult> {
        const { check } = await import("@tauri-apps/plugin-updater");
        pendingUpdate = await check();
        if (pendingUpdate) {
          return {
            available: true,
            currentVersion: pendingUpdate.currentVersion,
            version: pendingUpdate.version,
            notes: pendingUpdate.body ?? undefined
          };
        }
        const { invoke } = await import("@tauri-apps/api/core");
        return { available: false, currentVersion: await invoke<string>("app_version") };
      },

      async downloadAndInstall(
        onProgress?: (downloadedBytes: number, totalBytes?: number) => void
      ): Promise<void> {
        if (!pendingUpdate) {
          throw new Error("请先检查更新");
        }
        let total: number | undefined;
        let downloaded = 0;
        // 安装包由 updater 用内置公钥做 minisign 验签，验签失败会在这里抛错。
        await pendingUpdate.downloadAndInstall((event) => {
          if (event.event === "Started") {
            total = event.data.contentLength;
          } else if (event.event === "Progress") {
            downloaded += event.data.chunkLength;
            onProgress?.(downloaded, total);
          }
        });
      },

      async relaunch(): Promise<void> {
        const { relaunch } = await import("@tauri-apps/plugin-process");
        await relaunch();
      }
    },

    async getAppVersion(): Promise<string> {
      const { invoke } = await import("@tauri-apps/api/core");
      // 对应 src-tauri/src/lib.rs 里的 app_version 命令
      return invoke<string>("app_version");
    },

    async openExternal(url: string): Promise<void> {
    const { invoke } = await import("@tauri-apps/api/core");
    await invoke("open_external", { url });
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

    async getAppExePath(): Promise<string> {
      const { invoke } = await import("@tauri-apps/api/core");
      return invoke<string>("app_exe_path");
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
