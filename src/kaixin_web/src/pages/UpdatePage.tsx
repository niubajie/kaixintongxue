import { useEffect, useState } from "react";
import { bridge } from "../platform";

interface Props {
  onNotice: (message: string) => void;
}

type CheckState = "idle" | "checking" | "unavailable";

const HISTORY_NOTES: { version: string; date: string; note: string }[] = [
  { version: "0.5.0", date: "11-15", note: "新增历史任务入口，精简导航与对话框" },
  { version: "0.4.0", date: "11-13", note: "数据库、账号与本地加密存储接入" },
  { version: "0.3.0", date: "11-10", note: "技能部署、系统设置与版本更新页面" }
];

export function UpdatePage({ onNotice }: Props) {
  const [version, setVersion] = useState("");
  const [state, setState] = useState<CheckState>("idle");
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

  // 当前没有配置更新服务：如实告知「无法检查」，不编造新版本号。
  const check = () => {
    setState("checking");
    window.setTimeout(() => {
      setState("unavailable");
      const now = new Date();
      const pad = (n: number) => String(n).padStart(2, "0");
      setCheckedAt(`${pad(now.getHours())}:${pad(now.getMinutes())}`);
      onNotice("未配置更新服务，暂时无法检查新版本。");
    }, 700);
  };

  const badge =
    state === "checking" ? "检查中…" : state === "unavailable" ? "无法检查" : "未检查";

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
            <span className={`tag${state === "unavailable" ? "" : " accent"}`}>{badge}</span>
          </div>

          <div className="row" style={{ marginTop: 16 }}>
            <button type="button" className="primary-btn" onClick={check} disabled={state === "checking"}>
              {state === "checking" ? "检查中…" : "检查更新"}
            </button>
            <span className="caption">
              {state === "checking"
                ? "正在连接更新服务…"
                : checkedAt
                  ? `上次检查：${checkedAt}`
                  : "尚未检查"}
            </span>
          </div>

          {state === "unavailable" && (
            <div className="update-notes" style={{ marginTop: 14 }}>
              尚未接入更新服务，因此无法判断是否有新版本。
              {"\n"}
              接入云端后，这里会显示可用版本、更新说明与「立即更新」。
              {"\n"}
              当前不会自动下载或安装任何内容。
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
