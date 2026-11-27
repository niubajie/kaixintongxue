import { useEffect, useState } from "react";
import { bridge } from "../platform";
import type { UpdateCheckResult } from "../platform/types";

interface Props {
  onNotice: (message: string) => void;
}

type Phase = "idle" | "checking" | "downloading" | "installed";

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
  const [result, setResult] = useState<UpdateCheckResult | null>(null);
  const [error, setError] = useState("");
  const [checkedAt, setCheckedAt] = useState("");
  const [downloaded, setDownloaded] = useState(0);
  const [total, setTotal] = useState(0);

  const supported = bridge.updater.supported;

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
    if (!supported) {
      setError("当前环境不支持应用内更新，请下载桌面安装包后覆盖安装。");
      return;
    }
    setPhase("checking");
    setError("");
    setResult(null);
    setDownloaded(0);
    setTotal(0);
    try {
      const res = await bridge.updater.check();
      setResult(res);
      onNotice(res.available ? `发现新版本 ${res.version ?? ""}` : `已是最新版本 ${res.currentVersion}`);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "检查更新失败");
    } finally {
      markChecked();
      setPhase("idle");
    }
  };

  const install = async () => {
    setPhase("downloading");
    setError("");
    try {
      await bridge.updater.downloadAndInstall((bytes, size) => {
        setDownloaded(bytes);
        if (size) {
          setTotal(size);
        }
      });
      setPhase("installed");
      onNotice("更新已安装，重启应用后生效。");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "下载或安装失败");
      setPhase("idle");
    }
  };

  const restart = async () => {
    try {
      await bridge.updater.relaunch();
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "重启应用失败，请手动重新打开");
    }
  };

  // 三态互斥：检查失败 / 有新版本 / 已是最新（UPD-02）
  const badge = (() => {
    if (!supported) {
      return { text: "当前环境不支持", cls: "tag" };
    }
    if (phase === "checking") {
      return { text: "检查中…", cls: "tag" };
    }
    if (phase === "downloading") {
      return { text: "下载中…", cls: "tag accent" };
    }
    if (phase === "installed") {
      return { text: "待重启", cls: "tag accent" };
    }
    if (error) {
      return { text: "检查失败", cls: "tag" };
    }
    if (!result) {
      return { text: "未检查", cls: "tag" };
    }
    if (result.available) {
      return { text: `发现 ${result.version ?? "新版本"}`, cls: "tag accent" };
    }
    return { text: "已是最新", cls: "tag accent" };
  })();

  const percent = total > 0 ? Math.min(100, Math.round((downloaded / total) * 100)) : 0;

  return (
    <div className="page">
      <div className="page-narrow">
        <h1 className="page-title">版本更新</h1>
        <p className="page-subtitle">检查新版本，下载后自动校验签名并升级</p>

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
            <button
              type="button"
              className="primary-btn"
              onClick={() => void check()}
              disabled={phase === "checking" || phase === "downloading" || !supported}
            >
              {phase === "checking" ? "检查中…" : "检查更新"}
            </button>
            <span className="caption">
              {phase === "checking"
                ? "正在获取更新清单…"
                : checkedAt
                  ? `上次检查：${checkedAt}`
                  : "尚未检查"}
            </span>
          </div>

          {error && (
            <div className="update-notes" style={{ marginTop: 14 }}>
              {error}
              {"\n"}请检查网络连接，或确认更新服务器地址已发布最新清单。
            </div>
          )}

          {result && !error && (
            <div className="update-notes" style={{ marginTop: 14 }}>
              {result.available
                ? `发现新版本 ${result.version}（当前 ${result.currentVersion}）`
                : `当前已是最新版本 ${result.currentVersion}`}
              {result.available && result.notes ? `\n${result.notes}` : ""}
            </div>
          )}

          {result?.available && !error && (
            <div className="row" style={{ marginTop: 12, gap: 8 }}>
              {phase === "installed" ? (
                <button type="button" className="primary-btn" onClick={() => void restart()}>
                  立即重启
                </button>
              ) : (
                <button
                  type="button"
                  className="primary-btn"
                  onClick={() => void install()}
                  disabled={phase === "downloading"}
                >
                  {phase === "downloading" ? "下载并安装中…" : "下载并安装"}
                </button>
              )}
              <span className="caption">
                {phase === "downloading"
                  ? `${percent ? `${percent}% · ` : ""}已下载 ${formatSize(downloaded)}${total ? ` / ${formatSize(total)}` : ""}`
                  : phase === "installed"
                    ? "安装完成，重启后使用新版本"
                    : "下载完成后自动校验签名，不再需要手动运行安装包"}
              </span>
            </div>
          )}

          <p className="caption" style={{ marginTop: 12 }}>
            更新包使用 minisign 签名校验，篡改或来源不明的安装包会被拒绝。
            若发布方尚未配置 Windows 代码签名证书，首次安装仍可能被 SmartScreen 提示。
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
