import { bridge } from "../platform";
import type { Envelope } from "./types";

const TOKEN_KEY = "circle_admin_token";

export class ApiError extends Error {
  code: number;
  status: number;
  constructor(message: string, code: number, status: number) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
  }
}

let unauthorizedHandler: (() => void) | null = null;
export function setUnauthorizedHandler(fn: () => void) {
  unauthorizedHandler = fn;
}

interface RequestOptions {
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  body?: unknown;
}

export async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const token = bridge.secureStorage.get(TOKEN_KEY);
  const headers: Record<string, string> = { Accept: "application/json" };
  if (opts.body !== undefined) headers["Content-Type"] = "application/json";
  if (token) headers.Authorization = `Bearer ${token}`;

  let res: Response;
  try {
    res = await fetch(path, {
      method: opts.method ?? "GET",
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body)
    });
  } catch {
    throw new ApiError("网络异常，请检查服务是否启动", -1, 0);
  }

  const env = (await res.json().catch(() => {
    throw new ApiError(`响应解析失败（HTTP ${res.status}）`, -1, res.status);
  })) as Envelope<T>;

  if (res.status === 401) {
    unauthorizedHandler?.();
  }
  if (!res.ok || env.code !== 0) {
    throw new ApiError(env.message || `请求失败（HTTP ${res.status}）`, env.code ?? -1, res.status);
  }
  return env.data;
}

export const tokenStore = {
  get: () => bridge.secureStorage.get(TOKEN_KEY),
  set: (token: string) => bridge.secureStorage.set(TOKEN_KEY, token),
  clear: () => bridge.secureStorage.remove(TOKEN_KEY)
};
