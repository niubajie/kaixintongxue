import { webBridge } from "./web";
import type { DesktopBridge } from "./types";

// 网页端只使用浏览器实现，不加载任何桌面壳能力。
export const bridge: DesktopBridge = webBridge;

export type { DesktopBridge } from "./types";
