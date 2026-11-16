/**
 * 本地部署服务（Go，127.0.0.1:5085）接口。
 * 纯本地、无数据库；服务未启动时所有请求抛出带中文说明的错误。
 */

const BASE = "http://127.0.0.1:5085";

export interface DiskInfo {
  name: string;
  totalGb: number;
  freeGb: number;
}

export interface HardwareInfo {
  cpuName: string;
  cpuCores: number;
  cpuLogical: number;
  memoryTotalGb: number;
  memoryFreeGb: number;
  gpuName: string;
  gpuVramGb: number | null;
  osCaption: string;
  disks: DiskInfo[];
  detectedAt: string;
}

export interface MatchReason {
  text: string;
  softwareId: string;
  software: string;
  score: number;
  reasons: string[];
  siteId?: string;
  site?: string;
  siteUrl?: string;
}

export interface MatchResult {
  lines: MatchReason[];
  content: string;
}

export interface SkillSelection {
  id: string;
  name: string;
  softwareIds: string[];
}

export interface PlanStep {
  order: number;
  softwareId: string;
  name: string;
  action: "extract" | "pull-model" | "manual";
  sizeMb: number;
  url?: string;
  requires: string[];
  alreadyInstalled: boolean;
  note: string;
}

// 技能涉及的指定网站：部署时需要完成登录授权。
export interface PlanSiteAuth {
  siteId: string;
  name: string;
  url: string;
  loginUrl: string;
  note: string;
  authorized: boolean;
}

export interface DeployPlan {
  steps: PlanStep[];
  authSteps: PlanSiteAuth[];
  totalSizeMb: number;
  estimatedMinutes: number;
  warnings: string[];
}

export interface DeployStepStatus {
  order: number;
  softwareId: string;
  name: string;
  action: string;
  status: string;
  message: string;
  hashVerified?: boolean;
  sizeMb: number;
  totalBytes?: number;
  downloadedBytes?: number;
  bytesPerSecond?: number;
  downloadStartedAt?: string;
  siteId?: string;
  loginUrl?: string;
}

export interface DeployJob {
  id: string;
  state: "running" | "done" | "failed" | "cancelled";
  startedAt: string;
  finishedAt?: string;
  logPath: string;
  steps: DeployStepStatus[];
}

export interface SoftwareEntry {
  id: string;
  name: string;
  kind: string;
  sizeMb: number;
  note: string;
}

async function request<T>(path: string, init?: RequestInit, timeoutMs = 20000): Promise<T> {
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(BASE + path, {
      ...init,
      headers: { "Content-Type": "application/json" },
      signal: controller.signal
    });
    const payload = await response.json();
    if (!response.ok) {
      throw new Error(payload.message || `请求失败（${response.status}）`);
    }
    return payload as T;
  } catch (error) {
    if (error instanceof TypeError) {
      throw new Error("未检测到本地部署服务：请先运行 kaixin-local.exe（127.0.0.1:5085）。");
    }
    if (error instanceof DOMException && error.name === "AbortError") {
      throw new Error("本地服务响应超时，请稍后重试。");
    }
    throw error;
  } finally {
    window.clearTimeout(timer);
  }
}

export async function fetchHardware(): Promise<HardwareInfo> {
  const payload = await request<{ hardware: HardwareInfo }>("/api/system/hardware", undefined, 30000);
  return payload.hardware;
}

export function matchSoftware(content: string): Promise<MatchResult> {
  return request<MatchResult>("/api/skills/match-software", {
    method: "POST",
    body: JSON.stringify({ content })
  });
}

export function fetchCatalog(): Promise<SoftwareEntry[]> {
  return request<{ software: SoftwareEntry[] }>("/api/skills/catalog").then((payload) => payload.software);
}

export function buildPlan(
  skills: SkillSelection[],
  extraSoftwareIds: string[],
  siteIds: string[] = []
): Promise<DeployPlan> {
  return request<DeployPlan>("/api/deploy/plan", {
    method: "POST",
    body: JSON.stringify({ skills, extraSoftwareIds, siteIds })
  }, 40000);
}

export function executeDeploy(
  steps: PlanStep[],
  force: boolean,
  authSteps: PlanSiteAuth[] = []
): Promise<{ jobId: string }> {
  return request<{ jobId: string }>("/api/deploy/execute", {
    method: "POST",
    body: JSON.stringify({ steps, authSteps, force })
  });
}

// 登录框点击「我已完成授权」：把授权记录写入本机，后续部署自动跳过。
export function confirmSiteAuth(siteId: string): Promise<{ state: string }> {
  return request<{ state: string }>("/api/auth/confirm", {
    method: "POST",
    body: JSON.stringify({ siteId })
  });
}

// 用系统默认浏览器打开网站登录页。
export function openSiteLogin(siteId: string): Promise<{ state: string }> {
  return request<{ state: string }>("/api/auth/open", {
    method: "POST",
    body: JSON.stringify({ siteId })
  });
}

export function fetchDeployStatus(): Promise<DeployJob | { state: "idle" }> {
  return request<DeployJob | { state: "idle" }>("/api/deploy/status");
}

/** 取消当前部署任务：置位 sidecar 的任务上下文，各步骤会尽快退出。 */
export async function cancelDeploy(): Promise<{ state: string }> {
  const base = BASE;
  const response = await fetch(`${base}/api/deploy/cancel`, { method: "POST" });
  const payload = (await response.json().catch(() => null)) as { message?: string; state?: string } | null;
  if (!response.ok) {
    throw new Error(payload?.message ?? `取消失败（HTTP ${response.status}）`);
  }
  return { state: payload?.state ?? "cancelling" };
}
