import { request } from "./client";
import type {
  AdminUser,
  AuditLog,
  AuditQuery,
  Category,
  CategoryPatch,
  LoginResult,
  PageResult,
  Resource,
  ResourceQuery,
  RiskEvent,
  RiskQuery,
  Setting,
  UserQuery,
  VerifyResult
} from "./types";

export function adminLogin(username: string, password: string) {
  return request<LoginResult>("/api/v1/admin/auth/login", {
    method: "POST",
    body: { username, password }
  });
}

export function listCategories() {
  return request<{ items: Category[] }>("/api/v1/admin/categories");
}

export function updateCategory(code: string, patch: CategoryPatch) {
  return request<Category>(`/api/v1/admin/categories/${encodeURIComponent(code)}`, {
    method: "PATCH",
    body: patch
  });
}

export function listResources(q: ResourceQuery) {
  const params = new URLSearchParams();
  if (q.status) params.set("status", q.status);
  if (q.category) params.set("category", q.category);
  if (q.keyword) params.set("keyword", q.keyword);
  params.set("page", String(q.page ?? 1));
  params.set("size", String(q.size ?? 20));
  return request<PageResult<Resource>>(`/api/v1/admin/resources?${params.toString()}`);
}

export type ReviewAction = "approve" | "reject" | "offline";

export function reviewResource(uuid: string, action: ReviewAction, reason?: string) {
  return request<Resource>(`/api/v1/admin/resources/${encodeURIComponent(uuid)}/review`, {
    method: "POST",
    body: { action, reason: reason ?? "" }
  });
}

function pageParams(q: { page?: number; size?: number }): URLSearchParams {
  const params = new URLSearchParams();
  params.set("page", String(q.page ?? 1));
  params.set("size", String(q.size ?? 20));
  return params;
}

export function listUsers(q: UserQuery) {
  const params = pageParams(q);
  if (q.keyword) params.set("keyword", q.keyword);
  if (q.status) params.set("status", q.status);
  if (q.riskLevel) params.set("riskLevel", q.riskLevel);
  return request<PageResult<AdminUser>>(`/api/v1/admin/users?${params.toString()}`);
}

export function updateUserStatus(uuid: string, status: "active" | "blocked") {
  return request<AdminUser>(`/api/v1/admin/users/${encodeURIComponent(uuid)}/status`, {
    method: "PATCH",
    body: { status }
  });
}

export function listRiskEvents(q: RiskQuery) {
  const params = pageParams(q);
  if (q.severity) params.set("severity", q.severity);
  if (q.eventType) params.set("eventType", q.eventType);
  return request<PageResult<RiskEvent>>(`/api/v1/admin/risk/events?${params.toString()}`);
}

export function listSettings() {
  return request<{ items: Setting[] }>("/api/v1/admin/settings");
}

export function updateSetting(key: string, value: number) {
  return request<Setting>(`/api/v1/admin/settings/${encodeURIComponent(key)}`, {
    method: "PATCH",
    body: { value }
  });
}

export function listAuditLogs(q: AuditQuery) {
  const params = pageParams(q);
  if (q.actorType) params.set("actorType", q.actorType);
  if (q.action) params.set("action", q.action);
  if (q.keyword) params.set("keyword", q.keyword);
  return request<PageResult<AuditLog>>(`/api/v1/admin/audit-logs?${params.toString()}`);
}

export function verifyAuditChain() {
  return request<VerifyResult>("/api/v1/admin/audit-logs/verify");
}
