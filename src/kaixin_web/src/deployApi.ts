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

// 「AI 技能 + 硬件配置」模式按所选技能反推的推荐硬件。
export interface HardwareRecommendation {
  minMemoryGb: number;
  minVramGb: number;
  needDiskGb: number;
  items: string[];
}

export interface DeployPlan {
  steps: PlanStep[];
  authSteps: PlanSiteAuth[];
  totalSizeMb: number;
  estimatedMinutes: number;
  warnings: string[];
  recommendation?: HardwareRecommendation;
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

// refresh=true 时跳过服务端缓存强制重新检测（用于「重新检测」按钮）。
export async function fetchHardware(refresh = false): Promise<HardwareInfo> {
  const path = refresh ? "/api/system/hardware?refresh=1" : "/api/system/hardware";
  const payload = await request<{ hardware: HardwareInfo }>(path, undefined, 60000);
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
  siteIds: string[] = [],
  mode: "existing" | "upgrade" = "existing"
): Promise<DeployPlan> {
  return request<DeployPlan>("/api/deploy/plan", {
    method: "POST",
    body: JSON.stringify({ skills, extraSoftwareIds, siteIds, mode })
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


// ---------- 自定义技能（落本地库，见 docs/08 第 11 节） ----------

export interface CustomSkillRecord {
  id: string;
  name: string;
  description: string;
  softwareIds: string[];
  siteIds: string[];
  createdAt: string;
  updatedAt: string;
}

export async function listCustomSkills(): Promise<CustomSkillRecord[]> {
  const base = BASE;
  const response = await fetch(`${base}/api/skills/custom`);
  if (!response.ok) {
    throw new Error(`读取本地技能失败（HTTP ${response.status}）`);
  }
  const payload = (await response.json()) as { skills?: CustomSkillRecord[] };
  return payload.skills ?? [];
}

export async function saveCustomSkill(input: {
  id?: string;
  name: string;
  description: string;
  softwareIds: string[];
  siteIds: string[];
}): Promise<CustomSkillRecord> {
  const base = BASE;
  const response = await fetch(`${base}/api/skills/custom`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input)
  });
  const payload = (await response.json().catch(() => null)) as
    | { skill?: CustomSkillRecord; message?: string }
    | null;
  if (!response.ok || !payload?.skill) {
    throw new Error(payload?.message ?? `保存到本地库失败（HTTP ${response.status}）`);
  }
  return payload.skill;
}

export async function deleteCustomSkill(id: string): Promise<void> {
  const base = BASE;
  const response = await fetch(`${base}/api/skills/custom/${encodeURIComponent(id)}`, {
    method: "DELETE"
  });
  if (!response.ok && response.status !== 404) {
    throw new Error(`删除本地技能失败（HTTP ${response.status}）`);
  }
}


// ---------- 网站寻址（从需求推导到具体页面） ----------

export interface ResolveSiteBrief {
  id: string;
  name: string;
  url: string;
  category: string;
}

export interface ResolveStep {
  order: number;
  kind: "login" | "open-page" | "search" | "manual";
  title: string;
  url: string;
  why: string;
  successWhen?: string;
  requiresAuth: boolean;
}

export interface ResolveResult {
  need: string;
  intent: "auth" | "read" | "publish" | "configure";
  objects: string[];
  site?: ResolveSiteBrief;
  matchedKeywords?: string[];
  steps: ResolveStep[];
  alternatives?: ResolveSiteBrief[];
  advice?: string;
}

export async function resolveSiteNeed(need: string): Promise<ResolveResult> {
  const base = BASE;
  const response = await fetch(`${base}/api/sites/resolve`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ need })
  });
  if (!response.ok) {
    throw new Error(`网站寻址失败（HTTP ${response.status}）`);
  }
  return (await response.json()) as ResolveResult;
}

export async function openSitePage(input: {
  siteId?: string;
  page?: string;
  url?: string;
}): Promise<void> {
  const base = BASE;
  const response = await fetch(`${base}/api/sites/open`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input)
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { message?: string } | null;
    throw new Error(payload?.message ?? `打开页面失败（HTTP ${response.status}）`);
  }
}


// ---------- 部署历史 ----------

export interface DeployHistoryRun {
  id: string;
  state: string;
  startedAt: string;
  finishedAt?: string;
  totalSizeMb: number;
  logPath: string;
}

export async function fetchDeployHistory(): Promise<DeployHistoryRun[]> {
  const base = BASE;
  const response = await fetch(`${base}/api/deploy/history`);
  if (!response.ok) {
    throw new Error(`读取部署历史失败（HTTP ${response.status}）`);
  }
  const payload = (await response.json()) as { runs?: DeployHistoryRun[] };
  return payload.runs ?? [];
}


// ---------- 本地资料（真实文件） ----------

export interface LibraryItem {
  name: string;
  path: string;
  relPath: string;
  group: string;
  type: string;
  size: number;
  sizeText: string;
  modTime: string;
  date: string;
}

export interface LibraryScan {
  root: string;
  exists: boolean;
  items: LibraryItem[];
  total: number;
  groups: number;
  thisWeek: number;
  sizeBytes: number;
  sizeText: string;
  truncated: boolean;
  scannedAt: string;
}

export async function fetchLibraryScan(): Promise<LibraryScan> {
  const base = BASE;
  const response = await fetch(`${base}/api/library/scan`);
  if (!response.ok) {
    throw new Error(`扫描资料目录失败（HTTP ${response.status}）`);
  }
  return (await response.json()) as LibraryScan;
}

export async function openLibraryFile(path: string): Promise<void> {
  const base = BASE;
  const response = await fetch(`${base}/api/library/open`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { message?: string } | null;
    throw new Error(payload?.message ?? `打开文件失败（HTTP ${response.status}）`);
  }
}

export async function openLibraryFolder(): Promise<void> {
  const base = BASE;
  const response = await fetch(`${base}/api/library/open-folder`, { method: "POST" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { message?: string } | null;
    throw new Error(payload?.message ?? `打开文件夹失败（HTTP ${response.status}）`);
  }
}
