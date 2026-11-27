import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";

import { useAuthStore } from "../store/auth";
import { ToastHost } from "./Toast";

const NAV = [
  { to: "/dashboard", label: "数据看板" },
  { to: "/resources", label: "资源审核" },
  { to: "/categories", label: "分类管理" },
  { to: "/users", label: "用户管理" },
  { to: "/risk", label: "风控中心" },
  { to: "/orders", label: "订单退款" },
  { to: "/complaints", label: "投诉处理" },
  { to: "/settings", label: "系统配置" },
  { to: "/audit", label: "审计查询" }
];

const ROLE_LABEL: Record<string, string> = {
  super_admin: "超级管理员",
  admin_operator: "运营管理员",
  reviewer: "审核员",
  finance: "财务",
  auditor: "审计员"
};

export default function Layout() {
  const location = useLocation();
  const navigate = useNavigate();
  const username = useAuthStore((s) => s.username);
  const role = useAuthStore((s) => s.role);
  const logout = useAuthStore((s) => s.logout);

  const current = NAV.find((n) => location.pathname.startsWith(n.to));

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-title">1个亿资源圈</div>
          <div className="brand-sub">管理后台</div>
        </div>
        <nav className="nav">
          {NAV.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) => (isActive ? "nav-item active" : "nav-item")}
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
      </aside>

      <div className="main">
        <header className="topbar">
          <h1 className="page-title">{current?.label ?? "管理后台"}</h1>
          <div className="topbar-user">
            <span className="user-name">{username ?? "未知用户"}</span>
            <span className="role-tag">{ROLE_LABEL[role ?? ""] ?? role ?? "-"}</span>
            <button
              className="btn btn-ghost"
              onClick={() => {
                logout();
                navigate("/login", { replace: true });
              }}
            >
              退出
            </button>
          </div>
        </header>

        <main className="content">
          <Outlet />
        </main>
      </div>

      <ToastHost />
    </div>
  );
}
