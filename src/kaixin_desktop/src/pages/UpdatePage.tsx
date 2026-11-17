import { useEffect, useState } from "react";
import { bridge } from "../platform";
import { checkUpdates, type UpdateCheckResult } from "../deployApi";

interface Props {
  onNotice: (message: string) => void;
}

const HISTORY_NOTES: { version: string; date: string; note: string }[] = [
  { version: "0.5.0", date: "11-15", note: "新增历史任务入口，精简导航与对话框" },
  { version: "0.4.0", date: "11-13", note: "数据库、账号与本地加密存储接入" },
  { version: "0.3.0", date: "11-10", note: "技能部署、系统设置与版本更新页面" }
];

export function UpdatePage({ onNotice }: Props) {
  const [version, setVersion] = useState("");
  const [checking, setChecking] = useState(false);
  const [result, setResult] = useState<UpdateCheckResult | null>(null);
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

  const check = async () => {
    setChecking(true);
    try {
      const res = await checkUpdates();
      setResult(res);
      onNotice(res.message);
    } catch (err: unknown) {
      setResult({
        configured: false,
        ok: false,
        message: err instanceof Error ? err.message : "检查更新失败",
        notes: []
      });
    } finally {
      const now = new Date();
      const pad = (n: number) => String(n).padStart(2, "0");
      setCheckedAt(`${pad(now.getHours())}:${pad(now.getMinutes())}`);
      setChecking(false);
    }
  };

  const badge = checking ? "检查中…" : result ? (result.ok ? "已获取" : "无法检查") : "未检查";
  const badgeClass = !result || result.ok ? "tag accent" : "tag";

  return (
    <div className="page">
      <div className="page-narrow">
        <h1 className="page-title">版本更新</h1>
        <p className="page-subtitle">检查新版本并查看本次升级内容</p>

        <div className="card" style={{ marginTop: 18 }}>
          <div className="row" style={{ justifyContent: "space-between" }}>
            <div>
              <div style={{ fontSize: 15, fontWeight: 600 }}>开心同学 AI 助手</div>
              <div className="caption" style={{ marginTop: 6 }}>
                当前版本 {version || "读取中…"} · 桌面端
              </div>
            </div>
            <span className={badgeClass}>{badge}</span>
          </div>

          <div className="row" style={{ marginTop: 16 }}>
            <button type="button" className="primary-btn" onClick={() => void check()} disabled={checking}>
              {checking ? "检查中…" : "检查更新"}
            </button>
            <span className="caption">
              {checking ? "正在连接更新服务…" : checkedAt ? `上次检查：${checkedAt}` : "尚未检查"}
            </span>
          </div>

          {result && (
            <div className="update-notes" style={{ marginTop: 14 }}>
              {result.message}
              {!result.configured && (
                <>
                  {"\n"}
                  到「系统设置 → 服务器」填入服务端地址后再试。
                </>
              )}
              {"\n"}
              当前不会自动下载或安装任何内容；下载安装仍在开发中。
            </div>
          )}

          {result && result.notes.length > 0 && (
            <div style={{ marginTop: 12 }}>
              <div className="caption">服务端版本说明</div>
              <table className="table" style={{ marginTop: 6 }}>
                <thead>
                  <tr>
                    <th>版本</th>
                    <th>日期</th>
                    <th>说明</th>
                  </tr>
                </thead>
                <tbody>
                  {result.notes.map((item) => (
                    <tr key={item.version + item.date}>
                      <td>{item.version}</td>
                      <td>{item.date}</td>
                      <td>{item.note}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
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
