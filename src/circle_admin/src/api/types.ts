export interface Envelope<T> {
  code: number;
  message: string;
  data: T;
  traceId?: string;
}

export interface Category {
  code: string;
  name: string;
  enabled: boolean;
  sortOrder: number;
  contactPriceCents: number;
  accessPriceCents: number;
  sensitiveLevel: "low" | "medium" | "high" | "extreme";
  disclaimerText: string;
  createdAt: string;
}

export type ResourceStatus = "draft" | "pending" | "online" | "rejected" | "offline" | "expired";

export interface Resource {
  uuid: string;
  ownerUserId: number;
  categoryCode: string;
  title: string;
  summary: string;
  detailMd: string;
  regionCode: string;
  providerType: "enterprise" | "individual" | "broker";
  contactMode: "apply";
  status: ResourceStatus;
  qualityScore: number;
  rejectReason: string;
  expiresAt: string | null;
  publishedAt: string | null;
  createdAt: string;
}

export interface PageResult<T> {
  items: T[];
  total: number;
}

export interface LoginResult {
  accessToken: string;
  role: string;
  username: string;
}

export interface ResourceQuery {
  status?: string;
  category?: string;
  keyword?: string;
  page?: number;
  size?: number;
}

export interface CategoryPatch {
  enabled?: boolean;
  sortOrder?: number;
  contactPriceCents?: number;
  accessPriceCents?: number;
  sensitiveLevel?: Category["sensitiveLevel"];
  disclaimerText?: string;
}

export const STATUS_LABEL: Record<ResourceStatus, string> = {
  draft: "草稿",
  pending: "待审核",
  online: "已上线",
  rejected: "已拒绝",
  offline: "已下架",
  expired: "已过期"
};

// ---- 后台：用户 / 风控 / 配置 / 审计 ----

export type UserStatus = "active" | "blocked" | "pending_delete";
export type RiskLevel = "normal" | "low" | "medium" | "high";

export interface AdminUser {
  uuid: string;
  nickname: string;
  status: UserStatus;
  riskLevel: RiskLevel;
  phoneBound: boolean;
  resourceCount: number;
  lastLoginAt: string | null;
  createdAt: string;
}

export const USER_STATUS_LABEL: Record<UserStatus, string> = {
  active: "正常",
  blocked: "已封禁",
  pending_delete: "待注销"
};

export const RISK_LEVEL_LABEL: Record<RiskLevel, string> = {
  normal: "正常",
  low: "低",
  medium: "中",
  high: "高"
};

export type RiskSeverity = "low" | "medium" | "high" | "critical";

export interface RiskEvent {
  id: number;
  eventType: string;
  severity: RiskSeverity;
  userId: number | null;
  ip: string;
  deviceHash: string;
  detail: unknown;
  createdAt: string;
}

export const SEVERITY_LABEL: Record<RiskSeverity, string> = {
  low: "低",
  medium: "中",
  high: "高",
  critical: "严重"
};

export interface Setting {
  key: string;
  value: number;
  updatedBy: string;
  updatedAt: string;
}

export interface AuditLog {
  id: number;
  actorType: "user" | "admin" | "system";
  actorId: string;
  action: string;
  targetType: string;
  targetId: string;
  traceId: string;
  detail: unknown;
  createdAt: string;
}

export interface VerifyResult {
  ok: boolean;
  checked: number;
  brokenAt: number | null;
}

export interface UserQuery {
  keyword?: string;
  status?: string;
  riskLevel?: string;
  page?: number;
  size?: number;
}

export interface RiskQuery {
  severity?: string;
  eventType?: string;
  page?: number;
  size?: number;
}

export interface AuditQuery {
  actorType?: string;
  action?: string;
  keyword?: string;
  page?: number;
  size?: number;
}
