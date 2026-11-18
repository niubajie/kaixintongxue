import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { api } from "./api";
import type { Profile, Skill } from "./types";
import { UsersPanel } from "./components/UsersPanel";
import { AdminUsersPanel } from "./components/AdminUsersPanel";
import { SkillsPanel } from "./components/SkillsPanel";
import { VersionsPanel } from "./components/VersionsPanel";
import { NotesPanel } from "./components/NotesPanel";
import { MaterialsPanel } from "./components/MaterialsPanel";
import { BillingPanel } from "./components/BillingPanel";
import { OptimizationsPanel } from "./components/OptimizationsPanel";

type Tab = "admins" | "users" | "skills" | "versions" | "notes" | "materials" | "billing" | "optimizations";

const TABS: Array<{ key: Tab; label: string }> = [
  { key: "users", label: "用户管理" },
  { key: "admins", label: "管理员管理" },
  { key: "skills", label: "技能管理" },
  { key: "versions", label: "版本管理" },
  { key: "notes", label: "升级说明" },
  { key: "materials", label: "资料管理" },
  { key: "billing", label: "充值与价格" },
  { key: "optimizations", label: "自主优化" }
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
      setLoading(false);
      if (!user) {
        return;
      }
      setProfile(user);
      if (user.role === "Owner" || user.role === "Admin") {
        await loadSkills();
      }
    })();
    return () => {
      cancelled = true;
      window.clearTimeout(timer.current);
    };
  }, [loadSkills]);

  const handleLogin = async (account: string, password: string) => {
    const data = await api.login(account, password);
    setProfile(data.user);
    if (data.user.role === "Owner" || data.user.role === "Admin") {
      await loadSkills();
    }
    notify(data.message);
  };

  const handleLogout = async () => {
    await api.logout();
    setProfile(null);
    setSkills([]);
  };

  if (loading) {
    return <div className="center-hint">正在校验登录状态…</div>;
  }
  if (!profile) {
    return <LoginPage onLogin={handleLogin} />;
  }
  if (profile.role !== "Owner" && profile.role !== "Admin") {
    return (
      <div className="center-hint">
        需要管理员账号才能访问该页面。
        <button type="button" className="btn ghost small" onClick={handleLogout}>
          切换账号
        </button>
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
            <button type="button" className="btn small" onClick={handleLogout}>
              退出
            </button>
          </div>
        </div>
      </header>

      <main className="container">
        <div className="page-head">
          <h1>管理后台</h1>
          <p>用户、技能、版本、资料、计费与自主优化统一管理；技能与版本改动会同步到客户端清单与下载中心。</p>
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
          {tab === "admins" && <AdminUsersPanel notify={notify} selfId={profile.id} />}
          {tab === "skills" && <SkillsPanel skills={skills} reload={loadSkills} notify={notify} />}
          {tab === "versions" && <VersionsPanel skills={skills} reload={loadSkills} notify={notify} />}
          {tab === "notes" && <NotesPanel notify={notify} />}
          {tab === "materials" && <MaterialsPanel notify={notify} />}
          {tab === "billing" && <BillingPanel notify={notify} />}
          {tab === "optimizations" && <OptimizationsPanel notify={notify} />}
        </div>
      </main>

      <footer className="footer">
        <div className="container">© 2026 开心同学 AI 助手 · 仅管理员可访问</div>
      </footer>

      {toast && <div className="toast">{toast}</div>}
    </>
  );
}

interface LoginProps {
  onLogin: (account: string, password: string) => Promise<void>;
}

function LoginPage({ onLogin }: LoginProps) {
  const [account, setAccount] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setError("");
    setBusy(true);
    try {
      await onLogin(account.trim(), password);
    } catch (err) {
      setError(err instanceof Error ? err.message : "登录失败，请稍后再试。");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="login-page">
      <form className="login-card" onSubmit={submit}>
        <div className="login-brand">
          <span className="brand-mark">开</span>开心同学 AI 助手
        </div>
        <h1>管理后台登录</h1>
        <p className="hint">仅 Owner 与 Admin 账号可进入。</p>

        <div className="field">
          <label>账号</label>
          <input
            type="text"
            value={account}
            autoComplete="username"
            placeholder="管理员账号"
            onChange={(event) => setAccount(event.target.value)}
          />
        </div>
        <div className="field">
          <label>密码</label>
          <input
            type="password"
            value={password}
            autoComplete="current-password"
            placeholder="登录密码"
            onChange={(event) => setPassword(event.target.value)}
          />
        </div>

        {error && <div className="form-error">{error}</div>}

        <button type="submit" className="btn login-submit" disabled={busy}>
          {busy ? "登录中…" : "登录"}
        </button>
      </form>
    </div>
  );
}
