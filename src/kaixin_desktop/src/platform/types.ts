// 与壳无关的桌面能力接口。
// 组件只能依赖这里的类型，禁止直接 import @tauri-apps/*。

export type PlatformKind = "tauri" | "web";

export interface DesktopBridge {
  readonly kind: PlatformKind;

  readonly capabilities: {
    windowControls: boolean;
    appVersion: boolean;
  };

  getAppVersion(): Promise<string>;

  window: {
    minimize(): Promise<void>;
    toggleMaximize(): Promise<void>;
    close(): Promise<void>;
  };
}
