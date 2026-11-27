// 与壳无关的桌面能力接口。
// 组件只能依赖这里的类型，禁止直接 import @tauri-apps/*。

export type PlatformKind = "tauri" | "web";

/** 本地 Go sidecar 的默认端口（浏览器环境或 sidecar 启动失败时使用）。 */
export const DEFAULT_LOCAL_PORT = 5085;

/** 一次更新检查的结果。 */
export interface UpdateCheckResult {
  available: boolean;
  currentVersion: string;
  /** 有新版本时才有值 */
  version?: string;
  /** 发布说明（来自 latest.json 的 notes） */
  notes?: string;
}

export interface UpdaterBridge {
  readonly supported: boolean;
  /** 拉取更新清单；无新版本时 available=false，不抛错。 */
  check(): Promise<UpdateCheckResult>;
  /** 下载并安装已检查到的版本；onProgress 收到累计字节与总字节（总数可能未知）。 */
  downloadAndInstall(onProgress?: (downloadedBytes: number, totalBytes?: number) => void): Promise<void>;
  /** 安装完成后重启应用。 */
  relaunch(): Promise<void>;
}

export interface DesktopBridge {
  readonly kind: PlatformKind;

  /** 自动更新：检查清单、下载安装（安装包经 minisign 验签）、重启生效。 */
  readonly updater: UpdaterBridge;

  readonly capabilities: {
    windowControls: boolean;
    appVersion: boolean;
    /** 能否拿到本地部署服务的端口（仅桌面端为真） */
    localService: boolean;
    /** 能否弹出系统文件夹选择器（仅桌面端为真） */
    pickFolder: boolean;
    /** 能否多选文件（仅桌面端为真） */
    pickFiles: boolean;
    /** 能否用系统默认浏览器打开外部链接（微信扫码登录用） */
    openExternal: boolean;
    /** 是否支持应用内自动更新（仅桌面端为真） */
    updater: boolean;
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

  /** 主程序可执行文件路径（桌面端用于写开机启动项）。 */
  getAppExePath(): Promise<string>;

  /**
   * 弹出系统文件夹选择器，返回所选目录；用户取消或环境不支持时返回 null。
   */
  pickFolder(): Promise<string | null>;

  /** 用系统默认浏览器打开链接；不支持时抛错，由调用方提示用户手动打开。 */
  openExternal(url: string): Promise<void>;

  window: {
    minimize(): Promise<void>;
    toggleMaximize(): Promise<void>;
    close(): Promise<void>;
  };
}
