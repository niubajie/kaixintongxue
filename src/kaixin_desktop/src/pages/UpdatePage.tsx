import { useEffect, useState } from "react";
import { bridge } from "../platform";
import {
  downloadUpdate,
  fetchUpdateAvailable,
  installUpdate,
  type UpdateAvailability
} from "../deployApi";

interface Props {
  onNotice: (message: string) => void;
}

type Phase = "idle" | "checking" | "downloading" | "downloaded";

const HISTORY_NOTES: { version: string; date: string; note: string }[] = [
  { version: "0.5.0", date: "11-15", note: "新增历史任务入口，精简导航与对话框" },
  { version: "0.4.0", date: "11-13", note: "数据库、账号与本地加密存储接入" },
  { version: "0.3.0", date: "11-10", note: "技能部署、系统设置与版本更新页面" }
];

function formatSize(bytes: number): string {
  if (!bytes) {
    return "未知";
  }
  if (bytes >= 1024 * 1024 * 1024) {
    return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`;
  }
  if (bytes >= 1024 * 1024) {
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  }
  return `${Math.round(bytes / 1024)} KB`;
}

export function UpdatePage({ onNotice }: Props) {
  const [version, setVersion] = useState("");
  const [phase, setPhase] = useState<Phase>("idle");
  const [result, setResult] = useState<UpdateAvailability | null>(null);
  const [error, setError] = useState("");
  const [downloadedPath, setDownloadedPath] = useState("");
  const [checkedAt, setCheckedAt] = useState("");

  useEffect(() => {
    let alive = true;
    void bridge.getAppVersion().then((value) => {
      if (alive) {
        setVersion(value);
      }
    });
    return () => {
      alive = false;
    };
  }, []);

  const markChecked = () => {
    const now = new Date();
    const pad = (n: number) => String(n).padStart(2, "0");
    setCheckedAt(`${pad(now.getHours())}:${pad(now.getMinutes())}`);
  };

  const check = async () => {
    setPhase("checking");
    setError("");
    setResult(null);
    setDownloadedPath("");
    try {
      const res = await fetchUpdateAvailable(version || "0.0.0");
      setResult(res);
      onNotice(res.message);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "检查更新失败");
    } finally {
      markChecked();
      setPhase("idle");
    }
  };

  const download = async () => {
    if (!result) {
      return;
    }
    setPhase("downloading");
    setError("");
    try {
      const res = await downloadUpdate(result.latest, result.fileName, result.sha256);
      setDownloadedPath(res.path);
      setPhase("downloaded");
      onNotice(`安装包已下载并校验通过：${formatSize(res.sizeBytes)}`);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "下载失败");
      setPhase("idle");
    }
  };

  const install = async () => {
    if (!downloadedPath) {
      return;
    }
    try {
      await installUpdate(downloadedPath);
      onNotice("已启动安装包，请按提示完成升级，之后重新打开应用生效。");
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "启动安装包失败");
    }
  };

  // 三态互斥：检查失败 / 有新版本 / 已是最新（UPD-02）
  const badge = (() => {
    if (phase === "checking") {
      return { text: "检查中…", cls: "tag" };
    }
    if (error) {
      return { text: "检查失败", cls: "tag" };
    }
    if (!result) {
      return { text: "未检查", cls: "tag" };
    }
    if (result.available) {
      return { text: `发现 ${result.latest}`, cls: "tag accent" };
    }
    if (!result.configured) {
      return { text: "未配置服务器", cls: "tag" };
    }
    return { text: "已是最新", cls: "tag accent" };
  })();

  return (
    <div className="page">
      <div className="page-narrow">
        <h1 className="page-title">版本更新</h1>
        <p className="page-subtitle">检查新版本、下载安装包并升级</p>

        <div className="card" style={{ marginTop: 18 }}>
          <div className="row" style={{ justifyContent: "space-between" }}>
            <div>
              <div style={{ fontSize: 15, fontWeight: 600 }}>开心同学 AI 助手</div>
              <div className="caption" style={{ marginTop: 6 }}>
                当前版本 {version || "读取中…"} · 桌面端
              </div>
            </div>
            <span className={badge.cls}>{badge.text}</span>
          </div>

          <div className="row" style={{ marginTop: 16 }}>
            <button type="button" className="primary-btn" onClick={() => void check()} disabled={phase === "checking"}>
              {phase === "checking" ? "检查中…" : "检查更新"}
            </button>
            <span className="caption">
              {phase === "checking" ? "正在连接更新服务…" : checkedAt ? `上次检查：${checkedAt}` : "尚未检查"}
            </span>
          </div>

          {error && (
            <div className="update-notes" style={{ marginTop: 14 }}>
              {error}
              {"\n"}请确认服务器地址已配置且可访问。
            </div>
          )}

          {result && !error && (
            <div className="update-notes" style={{ marginTop: 14 }}>
              {result.message}
              {!result.configured && <>{"\n"}到「系统设置 → 服务器」填入地址后再试。</>}
              {result.available && (
                <>
                  {"\n"}
                  新版本 {result.latest} · 安装包 {formatSize(result.sizeBytes)}
                  {result.notes ? ` \n ${result.notes}` : ""}
                </>
              )}
            </div>
          )}

          {result?.available && (
            <div className="row" style={{ marginTop: 12, gap: 8 }}>
              {phase !== "downloaded" ? (
                <button type="button" className="primary-btn" onClick={() => void download()} disabled={phase === "downloading"}>
                  {phase === "downloading" ? "下载中…" : "下载安装包"}
                </button>
              ) : (
                <button type="button" className="primary-btn" onClick={() => void install()}>
                  安装并重启
                </button>
              )}
              <span className="caption">
                {phase === "downloaded" ? "安装包已通过 SHA-256 校验" : "下载后会先校验完整性再允许安装"}
              </span>
            </div>
          )}

          <p className="caption" style={{ marginTop: 12 }}>
            当前版本没有代码签名，安装时 Windows 可能提示 SmartScreen；
            签名与静默自动升级仍在开发中。
          </p>
        </div>

        <div className="card">
          <h2 className="section-title">历史版本说明</h2>
          <table className="table">
            <thead>
              <tr>
                <th>版本</th>
                <th>日期</th>
                <th>升级内容</th>
              </tr>
            </thead>
            <tbody>
              {HISTORY_NOTES.map((item) => (
                <tr key={item.version}>
                  <td>{item.version}</td>
                  <td>{item.date}</td>
                  <td>{item.note}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
