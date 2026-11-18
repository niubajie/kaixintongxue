import { useEffect, useState } from "react";
import { ComputerStatusBar } from "./components/ComputerStatusBar";
import { ackDataLocation, fetchDataInfo, type DataLocation } from "./deployApi";
import { HomePage } from "./pages/HomePage";
import { ApplicationPage } from "./pages/ApplicationPage";
import { GoalPage } from "./pages/GoalPage";
import { LearningPage } from "./pages/LearningPage";
import { LibraryPage } from "./pages/LibraryPage";
import { SkillsPage } from "./pages/SkillsPage";
import { SystemPage } from "./pages/SystemPage";
import { UpdatePage } from "./pages/UpdatePage";
import { HistoryPage } from "./pages/HistoryPage";
import { bridge } from "./platform";
import { ConfirmDialog } from "./components/ConfirmDialog";
import { isDeploying } from "./runtimeState";
import { cancelDeploy } from "./deployApi";
import type { PageKey } from "./data";

const NAV: { key: PageKey; label: string; icon: string }[] = [
  { key: "home", label: "新建任务", icon: "\uE710" },
  { key: "application", label: "应用助手", icon: "\uE8A9" },
  { key: "goal", label: "目标助手", icon: "\uE71D" },
  { key: "learning", label: "学习助手", icon: "\uE82D" },
  { key: "library", label: "本地资料", icon: "\uE8F1" },
  { key: "skills", label: "技能部署", icon: "\uE90F" },
  { key: "system", label: "系统设置", icon: "\uE713" },
  { key: "update", label: "版本更新", icon: "\uE777" }
];

// 演示入口只在开发构建出现：正式包不打包该入口（决策 Q-05）
const DEMO_MODE = import.meta.env.DEV;

export default function App() {
  const [page, setPage] = useState<PageKey>("home");
  const [notice, setNotice] = useState(
    DEMO_MODE
      ? "方案为演示数据；点击部署会执行真实环境检测与软件安装。"
      : "点击「技能部署」可选择技能并生成方案。"
  );
  const [version, setVersion] = useState("0.5.0");
  const [closeConfirm, setCloseConfirm] = useState(false);
  const [dataNotice, setDataNotice] = useState<DataLocation | null>(null);

  // 版本号不再写死在界面里，统一从壳层读取
  // 首次启动告诉用户数据存在哪块硬盘
  useEffect(() => {
    let alive = true;
    fetchDataInfo()
      .then((info) => {
        if (alive && info.location.firstRun) {
          setDataNotice(info.location);
        }
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, []);

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

  const notify = (message: string) => setNotice(message);

  const renderPage = () => {
    switch (page) {
      case "application":
        return <ApplicationPage onNotice={notify} />;
      case "goal":
        return <GoalPage onNotice={notify} />;
      case "learning":
        return <LearningPage onNotice={notify} />;
      case "library":
        return <LibraryPage onNotice={notify} />;
      case "skills":
        return <SkillsPage onNotice={notify} />;
      case "system":
        return <SystemPage onNotice={notify} />;
      case "update":
        return <UpdatePage onNotice={notify} />;
      case "history":
        return <HistoryPage onNotice={notify} />;
      default:
        return (
          <HomePage
            onNotice={notify}
            onNavigateApps={() => {
              setPage("application");
              notify("已打开应用助手，可确认或删除待确认资料。");
            }}
          />
        );
    }
  };

  return (
    <div className="app">
      <header className="titlebar" data-tauri-drag-region>
        <div className="titlebar-left">
          <button
            type="button"
            className="icon-btn"
            title="首页"
            onClick={() => {
              setPage("home");
              setNotice("已返回新建任务。");
            }}
          >
            <span style={{ fontSize: 13 }}>☰</span>
          </button>
          <button type="button" className="ghost-btn" onClick={() => notify("编辑菜单将在功能实现阶段接入。")}>
            编辑
          </button>
          <button type="button" className="ghost-btn" onClick={() => notify("窗口菜单将在功能实现阶段接入。")}>
            窗口
          </button>
          <button type="button" className="ghost-btn" onClick={() => notify("帮助文档将在功能实现阶段接入。")}>
            帮助
          </button>
          <button
            type="button"
            className="ghost-btn"
            onClick={() => notify(`开心同学 AI 助手 ${version}（Tauri 桌面端）`)}
          >
            关于
          </button>
          {DEMO_MODE && <span className="demo-badge">页面演示模式</span>}
        </div>
        <div className="titlebar-right">
          <ComputerStatusBar />
          <button
            type="button"
            className="window-btn"
            onClick={() => void bridge.window.minimize()}
            title="最小化"
          >
            ─
          </button>
          <button
            type="button"
            className="window-btn"
            onClick={() => void bridge.window.toggleMaximize()}
            title="最大化"
          >
            □
          </button>
          <button
            type="button"
            className="window-btn close"
            onClick={() => (isDeploying() ? setCloseConfirm(true) : void bridge.window.close())}
            title="关闭"
          >
            ✕
          </button>
        </div>
      </header>

      <div className="body">
        <aside className="sidebar">
          <button type="button" className="brand" onClick={() => setPage("home")}>
            开心同学
          </button>
          <nav className="nav">
            {NAV.map((item) => (
              <div key={item.key}>
                <button
                  type="button"
                  className={`nav-item${page === item.key ? " active" : ""}`}
                  onClick={() => {
                    setPage(item.key);
                    setNotice(DEMO_MODE ? `${item.label} · 页面演示模式` : item.label);
                  }}
                >
                  <span className="nav-icon" style={{ fontFamily: "Segoe MDL2 Assets" }}>
                    {item.icon}
                  </span>
                  <span>{item.label}</span>
                </button>
                {item.key === "update" && (
                  <div className="recent-section">
                    <div className="recent-section-title">最近</div>
                    <button
                      type="button"
                      className="recent-task"
                      title="制作智能体及实施流程"
                      onClick={() => {
                        setPage("history");
                        setNotice("已打开最近任务：制作智能体及实施流程");
                      }}
                    >
                      <span className="recent-task-name">制作智能体及实施流程</span>
                      <span className="recent-task-time">18小时前</span>
                    </button>
                  </div>
                )}
              </div>
            ))}
          </nav>
        </aside>

        <main className="content">{renderPage()}</main>
      </div>


      {dataNotice && (
        <div className="dialog-mask">
          <div className="dialog dialog-small">
            <h2 className="dialog-title">数据保存在哪里</h2>
            <p className="caption" style={{ marginTop: 8 }}>{dataNotice.message}</p>
            <p className="caption" style={{ marginTop: 6, wordBreak: "break-all" }}>
              数据库：{dataNotice.dbPath}
            </p>
            <p className="caption" style={{ marginTop: 6 }}>
              右上角「硬盘剩余」显示的就是这块盘；以后可在「系统设置 → 数据位置」更换。
            </p>
            <div className="dialog-actions">
              <button
                type="button"
                className="primary-btn"
                onClick={() => {
                  void ackDataLocation();
                  setDataNotice(null);
                  setNotice("数据位置已记录，可在系统设置里更换。");
                }}
              >
                知道了
              </button>
            </div>
          </div>
        </div>
      )}

      <footer className="notice">{notice}</footer>

      <ConfirmDialog
        open={closeConfirm}
        title="部署任务还在进行"
        detail="关闭窗口会停止当前部署，已完成的步骤会保留。确定要停止并关闭吗？"
        confirmText="停止并关闭"
        onCancel={() => setCloseConfirm(false)}
        onConfirm={() => {
          setCloseConfirm(false);
          void cancelDeploy()
            .catch(() => undefined)
            .finally(() => bridge.window.close());
        }}
      />
    </div>
  );
}
