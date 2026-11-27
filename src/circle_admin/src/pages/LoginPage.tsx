import { useState } from "react";
import type { FormEvent } from "react";
import { Navigate, useNavigate } from "react-router-dom";

import { adminLogin } from "../api/admin";
import { ApiError } from "../api/client";
import { useAuthStore } from "../store/auth";

export default function LoginPage() {
  const token = useAuthStore((s) => s.token);
  const login = useAuthStore((s) => s.login);
  const navigate = useNavigate();

  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  if (token) {
    return <Navigate to="/dashboard" replace />;
  }

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const res = await adminLogin(username.trim(), password);
      login(res.accessToken, res.role, res.username);
      navigate("/dashboard", { replace: true });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "登录失败，请稍后重试");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-page">
      <form className="login-card" onSubmit={submit}>
        <div className="login-brand">
          <div className="brand-title">1个亿资源圈</div>
          <div className="brand-sub">管理后台</div>
        </div>

        <label className="field">
          <span>用户名</span>
          <input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
            required
            placeholder="请输入管理员账号"
          />
        </label>

        <label className="field">
          <span>密码</span>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            required
            placeholder="请输入密码"
          />
        </label>

        {error && <div className="form-error">{error}</div>}

        <button className="btn btn-primary btn-block" type="submit" disabled={loading}>
          {loading ? "登录中…" : "登录"}
        </button>

        <p className="login-hint">
          首次启动由服务端环境变量 ADMIN_BOOTSTRAP_USERNAME / ADMIN_BOOTSTRAP_PASSWORD 创建超级管理员。
        </p>
      </form>
    </div>
  );
}
