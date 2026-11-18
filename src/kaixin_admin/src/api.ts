import type {
  AdminUser,
  MaterialProject,
  ModelPrice,
  Optimization,
  Profile,
  RechargeRecord,
  Role,
  Skill,
  SkillInput,
  UserStatus,
  VersionInfo,
  VersionNote
} from "./types";

export class ApiError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const isForm = options.body instanceof FormData;
  const response = await fetch(path, {
    credentials: "same-origin",
    headers: isForm ? undefined : { "Content-Type": "application/json" },
    ...options
  });
  const text = await response.text();
  const data = text ? (JSON.parse(text) as unknown) : null;
  if (!response.ok) {
    const message =
      data && typeof data === "object" && "message" in data
        ? String((data as { message: unknown }).message)
        : `请求失败（${response.status}）`;
    throw new ApiError(message, response.status);
  }
  return data as T;
}

export const api = {
  login: (account: string, password: string) =>
    request<{ message: string; user: Profile }>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ account, password })
    }),

  logout: () => request<{ message: string }>("/api/auth/logout", { method: "POST" }),

  me: async (): Promise<Profile | null> => {
    try {
      const data = await request<{ user: Profile }>("/api/auth/me");
      return data.user;
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        return null;
      }
      return null;
    }
  },

  users: (params: { keyword: string; role: string; page: number; size: number }) => {
    const query = new URLSearchParams({
      keyword: params.keyword,
      role: params.role,
      page: String(params.page),
      size: String(params.size)
    });
    return request<{ total: number; page: number; size: number; items: AdminUser[] }>(
      `/api/admin/users?${query.toString()}`
    );
  },

  setRole: (id: number, role: Role) =>
    request<{ message: string }>(`/api/admin/users/${id}/role`, {
      method: "POST",
      body: JSON.stringify({ role })
    }),

  setStatus: (id: number, status: UserStatus) =>
    request<{ message: string }>(`/api/admin/users/${id}/status`, {
      method: "POST",
      body: JSON.stringify({ status })
    }),

  deleteUser: (id: number) => request<{ message: string }>(`/api/admin/users/${id}`, { method: "DELETE" }),

  skills: () => request<Skill[]>("/api/software"),

  createSkill: (body: SkillInput) =>
    request<{ message: string }>("/api/admin/software", { method: "POST", body: JSON.stringify(body) }),

  updateSkill: (id: number, body: SkillInput) =>
    request<{ message: string }>(`/api/admin/software/${id}`, { method: "PUT", body: JSON.stringify(body) }),

  deleteSkill: (id: number) =>
    request<{ message: string }>(`/api/admin/software/${id}`, { method: "DELETE" }),

  nextVersion: (id: number) => request<{ version: string }>(`/api/admin/software/${id}/next-version`),

  publish: (id: number, form: FormData) =>
    request<{ message: string }>(`/api/admin/software/${id}/publish`, { method: "POST", body: form }),

  confirm: (id: number, version: string) =>
    request<{ message: string }>(`/api/admin/software/${id}/versions/${encodeURIComponent(version)}/confirm`, {
      method: "POST"
    }),

  rollback: (id: number) =>
    request<{ message: string }>(`/api/admin/software/${id}/rollback`, { method: "POST" }),

  rescan: (id: number) => request<{ message: string }>(`/api/admin/software/${id}/scan`, { method: "POST" }),

  history: (id: number) => request<VersionInfo[]>(`/api/admin/software/${id}/history`),

  notes: () => request<VersionNote[]>("/api/version-notes"),

  addNote: (body: { version: string; title: string; detail: string }) =>
    request<{ message: string }>("/api/admin/version-notes", { method: "POST", body: JSON.stringify(body) }),

  // ---- 资料项目 ----
  materials: () => request<{ items: MaterialProject[] }>("/api/admin/materials"),

  saveMaterial: (body: Partial<MaterialProject> & { title: string }) =>
    request<{ message: string }>("/api/admin/materials", { method: "POST", body: JSON.stringify(body) }),

  deleteMaterial: (id: number) =>
    request<{ message: string }>(`/api/admin/materials/${id}`, { method: "DELETE" }),

  runMaterial: (id: number) =>
    request<{ message: string }>(`/api/admin/materials/${id}/run`, { method: "POST" }),

  // ---- 充值 ----
  recharges: () => request<{ items: RechargeRecord[] }>("/api/admin/recharges"),

  addRecharge: (body: { account: string; amountCents: number; channel: string; note: string }) =>
    request<{ message: string }>("/api/admin/recharges", { method: "POST", body: JSON.stringify(body) }),

  // ---- Token 价格 ----
  prices: () => request<{ items: ModelPrice[] }>("/api/admin/model-prices"),

  savePrice: (body: { model: string; inputPerMillionCents: number; outputPerMillionCents: number }) =>
    request<{ message: string }>("/api/admin/model-prices", { method: "POST", body: JSON.stringify(body) }),

  // ---- 自主优化 ----
  optimizations: () => request<{ items: Optimization[] }>("/api/admin/optimizations"),

  addOptimization: (body: { version: string; category: string; detail: string }) =>
    request<{ message: string }>("/api/admin/optimizations", { method: "POST", body: JSON.stringify(body) })
};
