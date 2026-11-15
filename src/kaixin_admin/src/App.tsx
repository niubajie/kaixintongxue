import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./api";
import type { Profile, Skill } from "./types";
import { UsersPanel } from "./components/UsersPanel";
import { SkillsPanel } from "./components/SkillsPanel";
import { VersionsPanel } from "./components/VersionsPanel";
import { NotesPanel } from "./components/NotesPanel";

type Tab = "users" | "skills" | "versions" | "notes";

const TABS: Array<{ key: Tab; label: string }> = [
  { key: "users", label: "用户管理" },
  { key: "skills", label: "技能管理" },
  { key: "versions", label: "版本管理" },
  { key: "notes", label: "升级说明" }
];

export function App() {
  const [profile, setProfile] = useState<Profile | null>(null);
  const [loading, setLoading] = useState(true);
  const [tab, setTab] = useState<Tab>("users");
  const [skills, setSkills] = useState<Skill[]>([]);
  const [toast, setToast] = useState("");
  const timer = useRef<number | undefined>(undefined);

  const notify = useCallback((message: string) => {
    setToast(message);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setToast(""), 2600);
  }, []);

  const loadSkills = useCallback(async () => {
    try {
      setSkills(await api.skills());
    } catch (error) {
      notify(error instanceof Error ? error.message : "读取技能失败。");
    }
  }, [notify]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const user = await api.me();
      if (cancelled) {
        return;
      }
      if (!user) {
        window.location.href = "/login.html?next=" + encodeURIComponent("/admin.html");
        return;
      }
      setProfile(user);
      setLoading(false);
      if (user.role === "Owner" || user.role === "Admin") {
        await loadSkills();
      }
    })();
    return () => {
      cancelled = true;
      window.clearTimeout(timer.current);
    };
  }, [loadSkills]);

  if (loading) {
    return <div className="center-hint">正在校验登录状态…</div>;
  }
  if (!profile) {
    return null;
  }
  if (profile.role !== "Owner" && profile.role !== "Admin") {
    return (
      <div className="center-hint">
        需要管理员账号才能访问该页面。<a href="/login.html">切换账号</a>
      </div>
    );
  }

  return (
    <>
      <header className="topbar">
        <div className="container topbar-inner">
          <a className="brand" href="/">
            <span className="brand-mark">开</span>开心同学 AI 助手
          </a>
          <nav className="nav">
            <a href="/">官网首页</a>
            <a href="/download.html">下载中心</a>
          </nav>
          <div className="topbar-actions">
            <span className="who">
              {profile.displayName} · {profile.role}
            </span>
            <button
              type="button"
              className="btn small"
              onClick={async () => {
                await fetch("/api/auth/logout", { method: "POST", credentials: "same-origin" });
                window.location.href = "/";
              }}
            >
              退出
            </button>
          </div>
        </div>
      </header>

      <main className="container">
        <div className="page-head">
          <h1>管理后台</h1>
          <p>注册用户管理、技能管理与版本管理三块合一。技能与版本改动会同步到客户端技能清单与下载中心。</p>
        </div>

        <div className="tabs">
          {TABS.map((item) => (
            <button
              key={item.key}
              type="button"
              className={tab === item.key ? "active" : ""}
              onClick={() => setTab(item.key)}
            >
              {item.label}
            </button>
          ))}
        </div>

        <div className="tab-body">
          {tab === "users" && <UsersPanel notify={notify} selfId={profile.id} />}
          {tab === "skills" && <SkillsPanel skills={skills} reload={loadSkills} notify={notify} />}
          {tab === "versions" && <VersionsPanel skills={skills} reload={loadSkills} notify={notify} />}
          {tab === "notes" && <NotesPanel notify={notify} />}
        </div>
      </main>

      <footer className="footer">
        <div className="container">© 2026 开心同学 AI 助手 · 仅管理员可访问</div>
      </footer>

      {toast && <div className="toast">{toast}</div>}
    </>
  );
}
