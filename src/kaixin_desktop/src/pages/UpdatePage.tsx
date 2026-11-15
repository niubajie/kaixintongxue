import { useEffect, useState } from "react";

interface Props {
  onNotice: (message: string) => void;
}

const NOTES = `1. 新增「应用助手 / 目标助手 / 学习助手」三个工作入口
2. 首页支持切换 6 个实时语音与视觉模型
3. 本地资料支持大类与小类浏览、搜索与详情预览
4. 技能部署支持多选技能并生成演示方案`;

export function UpdatePage({ onNotice }: Props) {
  const [checking, setChecking] = useState(false);
  const [available, setAvailable] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [progress, setProgress] = useState(0);

  useEffect(() => {
    if (!installing) {
      return;
    }
    const timer = window.setInterval(() => {
      setProgress((current) => {
        if (current >= 100) {
          window.clearInterval(timer);
          setInstalling(false);
          onNotice("演示更新已完成，真实更新将在接入安装包后执行。");
          return 100;
        }
        return current + 10;
      });
    }, 240);
    return () => window.clearInterval(timer);
  }, [installing, onNotice]);

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
                当前版本 0.5.0 · 桌面端
              </div>
            </div>
            <span className="tag accent">已是最新</span>
          </div>

          <div className="row" style={{ marginTop: 16 }}>
            <button
              type="button"
              className="primary-btn"
              onClick={() => {
                setChecking(true);
                setAvailable(false);
                window.setTimeout(() => {
                  setChecking(false);
                  setAvailable(true);
                  onNotice("检查完成：发现演示版本 0.6.0。");
                }, 900);
              }}
            >
              {checking ? "检查中…" : "检查更新"}
            </button>
            <span className="caption">{checking ? "正在连接更新服务…" : "上次检查：今天 09:20"}</span>
          </div>

          {available && (
            <div className="update-notes">
              新版本 0.6.0 更新说明
              {"\n"}
              {NOTES}
              <div style={{ marginTop: 12 }}>
                <button type="button" className="secondary-btn" onClick={() => setInstalling(true)}>
                  立即更新（演示）
                </button>
              </div>
            </div>
          )}

          {installing && (
            <div style={{ marginTop: 14 }}>
              <div className="progress">
                <span style={{ width: `${progress}%` }} />
              </div>
              <div className="caption" style={{ marginTop: 7 }}>
                正在下载并安装：{progress}%
              </div>
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
              <tr>
                <td>0.5.0</td>
                <td>11-15</td>
                <td>新增历史任务入口，精简导航与对话框</td>
              </tr>
              <tr>
                <td>0.4.0</td>
                <td>11-13</td>
                <td>数据库、账号与本地加密存储接入</td>
              </tr>
              <tr>
                <td>0.3.0</td>
                <td>11-10</td>
                <td>技能部署、系统设置与版本更新页面</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
