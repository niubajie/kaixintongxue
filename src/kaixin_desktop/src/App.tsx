import { useState } from "react";
import { HomePage } from "./pages/HomePage";
import { ApplicationPage } from "./pages/ApplicationPage";
import { GoalPage } from "./pages/GoalPage";
import { LearningPage } from "./pages/LearningPage";
import { LibraryPage } from "./pages/LibraryPage";
import { SkillsPage } from "./pages/SkillsPage";
import { SystemPage } from "./pages/SystemPage";
import { UpdatePage } from "./pages/UpdatePage";
import { HistoryPage } from "./pages/HistoryPage";
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

async function controlWindow(action: "minimize" | "toggleMaximize" | "close") {
  if (!("__TAURI_INTERNALS__" in window)) {
    return;
  }
  const { getCurrentWindow } = await import("@tauri-apps/api/window");
  const current = getCurrentWindow();
  if (action === "minimize") {
    await current.minimize();
  } else if (action === "toggleMaximize") {
    await current.toggleMaximize();
  } else {
    await current.close();
  }
}

export default function App() {
  const [page, setPage] = useState<PageKey>("home");
  const [notice, setNotice] = useState("方案为演示数据；点击部署会执行真实环境检测与软件安装。");

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
            onNavigateSkills={() => {
              setPage("skills");
              notify("已打开技能部署，可选择技能生成方案。");
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
          <button type="button" className="ghost-btn" onClick={() => notify("开心同学 AI 助手 0.5.0（Tauri 桌面端）")}>
            关于
          </button>
          <span className="demo-badge">页面演示模式</span>
        </div>
        <div className="titlebar-right">
          <button type="button" className="window-btn" onClick={() => void controlWindow("minimize")} title="最小化">
            ─
          </button>
          <button
            type="button"
            className="window-btn"
            onClick={() => void controlWindow("toggleMaximize")}
            title="最大化"
          >
            □
          </button>
          <button
            type="button"
            className="window-btn close"
            onClick={() => void controlWindow("close")}
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
                    setNotice(`${item.label} · 页面演示模式`);
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

      <footer className="notice">{notice}</footer>
    </div>
  );
}
