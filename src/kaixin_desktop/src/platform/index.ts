import { createTauriBridge } from "./tauri";
import { webBridge } from "./web";
import type { DesktopBridge } from "./types";

function detect(): DesktopBridge {
  const inTauri = typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;
  return inTauri ? createTauriBridge() : webBridge;
}

// 全局唯一的桌面能力入口。组件只从这里拿，不关心底层是 Tauri 还是浏览器。
export const bridge: DesktopBridge = detect();

export type { DesktopBridge } from "./types";
