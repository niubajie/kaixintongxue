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
    /** 能否多选文件（仅桌面端为真） */
    pickFiles: boolean;
  };

  getAppVersion(): Promise<string>;

  /** 弹出系统文件夹选择器；用户取消或环境不支持时返回 null。 */
  pickFolder(): Promise<string | null>;

  /** 弹出系统文件选择器（可多选）；取消或环境不支持时返回空数组。 */
  pickFiles(): Promise<string[]>;

  /** 主程序可执行文件路径（桌面端用于写开机启动项）。 */
  getAppExePath(): Promise<string>;

  /** 本机 Go 服务端口；没有本地服务时返回 0。 */
  getLocalServicePort(): Promise<number>;

  window: {
    minimize(): Promise<void>;
    toggleMaximize(): Promise<void>;
    close(): Promise<void>;
  };
}
