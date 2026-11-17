// 与壳无关的桌面能力接口。
// 组件只能依赖这里的类型，禁止直接 import @tauri-apps/*。

export type PlatformKind = "tauri" | "web";

/** 本地 Go sidecar 的默认端口（浏览器环境或 sidecar 启动失败时使用）。 */
export const DEFAULT_LOCAL_PORT = 5085;

export interface DesktopBridge {
  readonly kind: PlatformKind;

  readonly capabilities: {
    windowControls: boolean;
    appVersion: boolean;
    /** 能否拿到本地部署服务的端口（仅桌面端为真） */
    localService: boolean;
    /** 能否弹出系统文件夹选择器（仅桌面端为真） */
    pickFolder: boolean;
    /** 能否多选文件（仅桌面端为真） */
    pickFiles: boolean;
  };

  getAppVersion(): Promise<string>;

  /**
   * 本地 Go sidecar 的监听端口。
   * 桌面端由 Tauri 启动时分配随机端口；浏览器环境固定回退到 5085。
   * 组件与 deployApi 都不允许自己 import @tauri-apps/*。
   */
  getLocalServicePort(): Promise<number>;

  /** 弹出系统文件选择器（可多选），返回所选文件路径；取消或环境不支持时返回空数组。 */
  pickFiles(): Promise<string[]>;

  /**
   * 弹出系统文件夹选择器，返回所选目录；用户取消或环境不支持时返回 null。
   */
  pickFolder(): Promise<string | null>;

  window: {
    minimize(): Promise<void>;
    toggleMaximize(): Promise<void>;
    close(): Promise<void>;
  };
}
