import { useEffect, useState } from "react";
import { ComputerStatusBar } from "./components/ComputerStatusBar";
import { HomePage } from "./pages/HomePage";
import { ApplicationPage } from "./pages/ApplicationPage";
import { LibraryPage } from "./pages/LibraryPage";
import { SystemPage } from "./pages/SystemPage";
import { UpdatePage } from "./pages/UpdatePage";
import { VoicePage } from "./pages/VoicePage";
import { bridge } from "./platform";
import type { PageKey } from "./data";

const NAV: { key: PageKey; label: string; icon: string }[] = [
  { key: "home", label: "新建任务", icon: "\uE710" },
  { key: "application", label: "应用助手", icon: "\uE8A9" },
  { key: "library", label: "本地资料", icon: "\uE8F1" },
  { key: "system", label: "系统设置", icon: "\uE713" },
  { key: "update", label: "版本更新", icon: "\uE777" },
  { key: "voice", label: "实时语音", icon: "\uE720" }
];

// 演示入口只在开发构建出现：正式包不打包该入口（决策 Q-05）
const DEMO_MODE = import.meta.env.DEV;

export default function App() {
  const [page, setPage] = useState<PageKey>("home");
  const [notice, setNotice] = useState(
    DEMO_MODE ? "页面演示模式：数据为演示数据。" : "点击「应用助手」可管理你的 AI 应用。"
  );
  const [version, setVersion] = useState("0.5.0");

  // 版本号不再写死在界面里，统一从壳层读取
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
      case "library":
        return <LibraryPage onNotice={notify} />;
      case "system":
        return <SystemPage onNotice={notify} />;
      case "update":
        return <UpdatePage onNotice={notify} />;
      case "voice":
        return <VoicePage />;
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
            onClick={() => void bridge.window.close()}
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
              <button
                key={item.key}
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
            ))}
          </nav>
        </aside>

        <main className="content">{renderPage()}</main>
      </div>

      <footer className="notice">{notice}</footer>
    </div>
  );
}
