// 与壳无关的桌面能力接口。
// 组件只能依赖这里的类型，禁止直接 import @tauri-apps/*。

export type PlatformKind = "tauri" | "web";

export interface DesktopBridge {
  readonly kind: PlatformKind;

  readonly capabilities: {
    windowControls: boolean;
    appVersion: boolean;
    /** 能否弹出系统文件夹选择器（仅桌面端为真） */
    pickFolder: boolean;
  };

  getAppVersion(): Promise<string>;

  /** 弹出系统文件夹选择器；用户取消或环境不支持时返回 null。 */
  pickFolder(): Promise<string | null>;

  window: {
    minimize(): Promise<void>;
    toggleMaximize(): Promise<void>;
    close(): Promise<void>;
  };
}
