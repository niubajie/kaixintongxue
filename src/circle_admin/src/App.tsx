import { Navigate, Outlet, Route, Routes } from "react-router-dom";

import Layout from "./components/Layout";
import { useAuthStore } from "./store/auth";
import LoginPage from "./pages/LoginPage";
import DashboardPage from "./pages/DashboardPage";
import ResourcesPage from "./pages/ResourcesPage";
import CategoriesPage from "./pages/CategoriesPage";
import UsersPage from "./pages/UsersPage";
import RiskPage from "./pages/RiskPage";
import SettingsPage from "./pages/SettingsPage";
import AuditPage from "./pages/AuditPage";
import PlaceholderPage from "./pages/PlaceholderPage";
import { PLACEHOLDERS } from "./pages/placeholders";

function ProtectedRoute() {
  const token = useAuthStore((s) => s.token);
  if (!token) {
    return <Navigate to="/login" replace />;
  }
  return <Outlet />;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route element={<ProtectedRoute />}>
        <Route element={<Layout />}>
          <Route index element={<Navigate to="/dashboard" replace />} />
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/resources" element={<ResourcesPage />} />
          <Route path="/categories" element={<CategoriesPage />} />
          <Route path="/users" element={<UsersPage />} />
          <Route path="/risk" element={<RiskPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/audit" element={<AuditPage />} />
          {PLACEHOLDERS.map((p) => (
            <Route
              key={p.path}
              path={p.path}
              element={<PlaceholderPage title={p.title} desc={p.desc} planned={p.planned} />}
            />
          ))}
          <Route path="*" element={<Navigate to="/dashboard" replace />} />
        </Route>
      </Route>
    </Routes>
  );
}
