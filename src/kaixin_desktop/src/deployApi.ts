/**
 * 本地部署服务（Go sidecar）接口。
 * 桌面端由 Tauri 启动 sidecar 并分配随机端口，前端通过 IPC 读取；
 * 网页端/手动运行时回退到 127.0.0.1:5085。
 */

import { bridge } from "./platform";

let baseUrl: string | null = null;

// 端口一律通过平台抽象层获取，禁止在本文件直接 import @tauri-apps/*，
// 否则网页端产物会被 Tauri 代码污染（AGENTS.md 第 6 节）。
async function getBaseUrl(): Promise<string> {
  if (baseUrl) return baseUrl;
  const port = await bridge.getLocalServicePort();
  baseUrl = `http://127.0.0.1:${port}`;
  return baseUrl;
}

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
  action: "extract" | "pull-model" | "cloud" | "manual";
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
  expiresAt?: string;
}

// 部署方案：软件步骤 + 网站授权步骤 + 体积与耗时预估。
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
  /** 服务端 catalog.json 里的匹配关键词，用于把技能/依赖名映射到软件 */
  keywords?: string[];
}

async function request<T>(path: string, init?: RequestInit, timeoutMs = 20000): Promise<T> {
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), timeoutMs);
  try {
    const base = await getBaseUrl();
    const response = await fetch(base + path, {
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
      throw new Error("未检测到本地部署服务：桌面端会自动启动，若仍失败请重启应用；网页端请先运行 kaixin-local.exe。");
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
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/deploy/cancel`, { method: "POST" });
  const payload = (await response.json().catch(() => null)) as { message?: string; state?: string } | null;
  if (!response.ok) {
    throw new Error(payload?.message ?? `取消失败（HTTP ${response.status}）`);
  }
  return { state: payload?.state ?? "cancelling" };
}


// ---------- 自定义技能（落本地库，见 docs/08 第 11 节） ----------

export interface CustomSkillRecord {
  category?: string;
  id: string;
  name: string;
  description: string;
  softwareIds: string[];
  siteIds: string[];
  createdAt: string;
  updatedAt: string;
}

export async function listCustomSkills(): Promise<CustomSkillRecord[]> {
  const base = await getBaseUrl();
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
  category?: string;
  softwareIds: string[];
  siteIds: string[];
}): Promise<CustomSkillRecord> {
  const base = await getBaseUrl();
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
  const base = await getBaseUrl();
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
  const base = await getBaseUrl();
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
  const base = await getBaseUrl();
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
  attempt: number;
  error: string;
  resumable: boolean;
  failedOrder: number;
}

export async function fetchDeployHistory(): Promise<DeployHistoryRun[]> {
  const base = await getBaseUrl();
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
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/library/scan`);
  if (!response.ok) {
    throw new Error(`扫描资料目录失败（HTTP ${response.status}）`);
  }
  return (await response.json()) as LibraryScan;
}

export async function openLibraryFile(path: string): Promise<void> {
  const base = await getBaseUrl();
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
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/library/open-folder`, { method: "POST" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { message?: string } | null;
    throw new Error(payload?.message ?? `打开文件夹失败（HTTP ${response.status}）`);
  }
}


export async function setLibraryDir(path: string): Promise<LibraryScan> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/library/set-dir`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path })
  });
  const payload = (await response.json().catch(() => null)) as (LibraryScan & { message?: string }) | null;
  if (!response.ok || !payload) {
    throw new Error(payload?.message ?? `切换资料目录失败（HTTP ${response.status}）`);
  }
  return payload;
}


// ---------- 本地知识库 ----------

export interface KBDocument {
  path: string;
  name: string;
  chunks: number;
  status: string; // indexed / failed / unsupported / absent
  revision?: number;
  error: string;
  indexedAt: string;
}

export interface KBBatchResult {
  ok: number;
  failed: number;
  skipped: number;
  results: KBDocument[];
}

export interface KBSearchHit {
  path: string;
  name: string;
  idx: number;
  snippet: string;
}

export async function fetchKBStatus(): Promise<KBDocument[]> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/kb/status`);
  if (!response.ok) {
    throw new Error(`读取知识库状态失败（HTTP ${response.status}）`);
  }
  const payload = (await response.json()) as { documents?: KBDocument[] };
  return payload.documents ?? [];
}

export async function indexKnowledgeBase(path: string): Promise<KBBatchResult> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/kb/index`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { message?: string } | null;
    throw new Error(payload?.message ?? `加入知识库失败（HTTP ${response.status}）`);
  }
  return (await response.json()) as KBBatchResult;
}

export async function removeKnowledgeBase(path: string): Promise<void> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/kb/remove`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { message?: string } | null;
    throw new Error(payload?.message ?? `移出知识库失败（HTTP ${response.status}）`);
  }
}



// ---------- 语音转写（本地 Whisper） ----------

export interface ASRStatus {
  available: boolean;
  whisperPath: string;
  modelPath: string;
  ffmpegPath: string;
  message: string;
}

export interface ASRResult {
  text: string;
  elapsedMs: number;
  language: string;
}

export async function fetchASRStatus(): Promise<ASRStatus> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/asr/status`);
  if (!response.ok) {
    throw new Error(`读取语音转写状态失败（HTTP ${response.status}）`);
  }
  return (await response.json()) as ASRStatus;
}

export async function transcribeAudio(audioBase64: string, language = "zh"): Promise<ASRResult> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/asr/transcribe`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ audioBase64, language })
  });
  const payload = (await response.json().catch(() => null)) as (ASRResult & { message?: string }) | null;
  if (!response.ok || !payload) {
    throw new Error(payload?.message ?? `转写失败（HTTP ${response.status}）`);
  }
  return payload;
}


// ---------- 开机启动 ----------

export interface AutostartState {
  enabled: boolean;
  command: string;
  keyPath: string;
  message: string;
}

export async function fetchAutostart(): Promise<AutostartState> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/system/autostart`);
  if (!response.ok) {
    throw new Error(`读取开机启动状态失败（HTTP ${response.status}）`);
  }
  return (await response.json()) as AutostartState;
}

export async function setAutostart(enabled: boolean, exePath: string): Promise<AutostartState> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/system/autostart`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled, exePath })
  });
  const payload = (await response.json().catch(() => null)) as (AutostartState & { message?: string }) | null;
  if (!response.ok || !payload) {
    throw new Error(payload?.message ?? `设置开机启动失败（HTTP ${response.status}）`);
  }
  return payload;
}


export async function indexKnowledgeBaseBatch(paths: string[]): Promise<KBBatchResult> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/kb/index`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ paths })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { message?: string } | null;
    throw new Error(payload?.message ?? `批量加入失败（HTTP ${response.status}）`);
  }
  return (await response.json()) as KBBatchResult;
}

export async function removeKnowledgeBaseBatch(paths: string[]): Promise<number> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/kb/remove`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ paths })
  });
  const payload = (await response.json().catch(() => null)) as { count?: number; message?: string } | null;
  if (!response.ok) {
    throw new Error(payload?.message ?? `批量移出失败（HTTP ${response.status}）`);
  }
  return payload?.count ?? 0;
}


// ---------- 服务端配置与更新检查 ----------

export interface ServerConfigState {
  url: string;
  saved: boolean;
  message: string;
}

export interface ServerTestResult {
  ok: boolean;
  status: number;
  message: string;
  body: string;
}

export interface UpdateNote {
  version: string;
  date: string;
  note: string;
}

export interface UpdateCheckResult {
  configured: boolean;
  ok: boolean;
  message: string;
  notes: UpdateNote[];
}

export async function fetchServerConfig(): Promise<ServerConfigState> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/server/config`);
  if (!response.ok) {
    throw new Error(`读取服务器配置失败（HTTP ${response.status}）`);
  }
  return (await response.json()) as ServerConfigState;
}

export async function saveServerConfig(url: string): Promise<ServerConfigState> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/server/config`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url })
  });
  const payload = (await response.json().catch(() => null)) as (ServerConfigState & { message?: string }) | null;
  if (!response.ok || !payload) {
    throw new Error(payload?.message ?? `保存失败（HTTP ${response.status}）`);
  }
  return payload;
}

export async function testServer(): Promise<ServerTestResult> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/server/test`, { method: "POST" });
  const payload = (await response.json().catch(() => null)) as (ServerTestResult & { message?: string }) | null;
  if (!response.ok || !payload) {
    throw new Error(payload?.message ?? `测试失败（HTTP ${response.status}）`);
  }
  return payload;
}



// ---------- 续装与单步重试（FR-SKL-18） ----------

export async function resumeDeploy(runId: string): Promise<void> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/deploy/resume`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ runId })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { message?: string } | null;
    throw new Error(payload?.message ?? `续装失败（HTTP ${response.status}）`);
  }
}

export async function retryDeployStep(runId: string, order: number): Promise<void> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/deploy/retry`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ runId, order })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { message?: string } | null;
    throw new Error(payload?.message ?? `重试失败（HTTP ${response.status}）`);
  }
}


// ---------- 知识库版本历史 ----------

export interface KBRevision {
  path: string;
  revision: number;
  chars: number;
  chunks: number;
  indexedAt: string;
  source: string;
}

export async function fetchKBRevisions(path: string): Promise<KBRevision[]> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/kb/revisions?path=${encodeURIComponent(path)}`);
  if (!response.ok) {
    throw new Error(`读取版本历史失败（HTTP ${response.status}）`);
  }
  const payload = (await response.json()) as { revisions?: KBRevision[] };
  return payload.revisions ?? [];
}

export async function rollbackKnowledgeBase(path: string, revision: number): Promise<KBDocument> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/kb/rollback`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path, revision })
  });
  const payload = (await response.json().catch(() => null)) as (KBDocument & { message?: string }) | null;
  if (!response.ok || !payload) {
    throw new Error(payload?.message ?? `回滚失败（HTTP ${response.status}）`);
  }
  return payload;
}


// ---------- 账号（经 sidecar 代理，页面不持有 Token） ----------

export interface AccountState {
  loggedIn: boolean;
  id: number;
  account: string;
  displayName: string;
  role: string;
  message: string;
}

export async function fetchAccount(): Promise<AccountState> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/account/me`);
  if (!response.ok) {
    throw new Error(`读取账号失败（HTTP ${response.status}）`);
  }
  return (await response.json()) as AccountState;
}

export async function signIn(
  account: string,
  password: string,
  register = false,
  displayName = ""
): Promise<AccountState> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/account/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ account, password, register, displayName })
  });
  const payload = (await response.json().catch(() => null)) as (AccountState & { message?: string }) | null;
  if (!response.ok || !payload) {
    throw new Error(payload?.message ?? `${register ? "注册" : "登录"}失败（HTTP ${response.status}）`);
  }
  return payload;
}

export async function signOut(): Promise<void> {
  const base = await getBaseUrl();
  await fetch(`${base}/api/account/logout`, { method: "POST" });
}


// ---------- 检查更新与下载安装（UPD-02 / UPD-03） ----------

export interface UpdateAvailability {
  configured: boolean;
  available: boolean;
  current: string;
  latest: string;
  fileName: string;
  sizeBytes: number;
  sha256: string;
  notes: string;
  message: string;
}

export interface UpdateDownloadResult {
  ok: boolean;
  path: string;
  fileName: string;
  sizeBytes: number;
  verified: boolean;
  message: string;
}

export async function fetchUpdateAvailable(current: string): Promise<UpdateAvailability> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/update/available?current=${encodeURIComponent(current)}`);
  if (!response.ok) {
    throw new Error(`检查更新失败（HTTP ${response.status}）`);
  }
  return (await response.json()) as UpdateAvailability;
}

export async function downloadUpdate(
  version: string,
  fileName: string,
  sha256: string
): Promise<UpdateDownloadResult> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/update/download`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ version, fileName, sha256 })
  });
  const payload = (await response.json().catch(() => null)) as (UpdateDownloadResult & { message?: string }) | null;
  if (!response.ok || !payload) {
    throw new Error(payload?.message ?? `下载失败（HTTP ${response.status}）`);
  }
  return payload;
}

export async function installUpdate(path: string): Promise<void> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/update/install`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { message?: string } | null;
    throw new Error(payload?.message ?? `启动安装包失败（HTTP ${response.status}）`);
  }
}


// ---------- 电脑检测（FR-GLB-07） ----------

export interface ComputerStatus {
  diskFreeGb: number;
  diskTotalGb: number;
  diskName: string;
  memFreeGb: number;
  memTotalGb: number;
  online: boolean;
  checkedAt: string;
}

export async function fetchComputerStatus(): Promise<ComputerStatus> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/system/status`);
  if (!response.ok) {
    throw new Error(`读取电脑状态失败（HTTP ${response.status}）`);
  }
  return (await response.json()) as ComputerStatus;
}


// ---------- 数据位置（数据库在哪块硬盘） ----------

export interface DataLocation {
  path: string;
  dbPath: string;
  drive: string;
  freeGb: number;
  totalGb: number;
  dbSizeBytes: number;
  firstRun: boolean;
  message: string;
}

export interface DataInfo {
  store: { path: string; sizeKb: number; customSkills: number; deployRuns: number; schemaVersion: number };
  location: DataLocation;
}

export async function fetchDataInfo(): Promise<DataInfo> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/data/info`);
  if (!response.ok) {
    throw new Error(`读取数据位置失败（HTTP ${response.status}）`);
  }
  return (await response.json()) as DataInfo;
}


export async function moveDataLocation(dir: string): Promise<string> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/data/move`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ dir })
  });
  const payload = (await response.json().catch(() => null)) as { message?: string } | null;
  if (!response.ok) {
    throw new Error(payload?.message ?? `迁移失败（HTTP ${response.status}）`);
  }
  return payload?.message ?? "已迁移";
}


// ---------- 模型调用（经本地网关） ----------

export interface ModelConfigState {
  baseUrl: string;
  model: string;
  hasApiKey: boolean;
  inputPriceCents: number;
  outputPriceCents: number;
  localModels: string[];
  available: boolean;
  message: string;
}

export interface ChatResult {
  text: string;
  model: string;
  promptTokens: number;
  completionTokens: number;
  costCents: number;
  elapsedMs: number;
  note: string;
}

export interface UsageItem {
  model: string;
  promptTokens: number;
  completionTokens: number;
  costCents: number;
  elapsedMs: number;
  createdAt: string;
}

export async function fetchModelConfig(): Promise<ModelConfigState> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/model/config`);
  if (!response.ok) {
    throw new Error(`读取模型配置失败（HTTP ${response.status}）`);
  }
  return (await response.json()) as ModelConfigState;
}

export async function saveModelConfig(input: {
  baseUrl: string;
  model: string;
  apiKey: string;
  inputPriceCents: number;
  outputPriceCents: number;
}): Promise<ModelConfigState> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/model/config`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input)
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { message?: string } | null;
    throw new Error(payload?.message ?? `保存失败（HTTP ${response.status}）`);
  }
  return (await response.json()) as ModelConfigState;
}

export async function testModel(): Promise<{ ok: boolean; message: string; reply?: string; elapsedMs?: number }> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/model/test`, { method: "POST" });
  if (!response.ok) {
    throw new Error(`测试失败（HTTP ${response.status}）`);
  }
  return (await response.json()) as { ok: boolean; message: string; reply?: string; elapsedMs?: number };
}




export interface StreamMeta {
  model: string;
  promptTokens: number;
  completionTokens: number;
  costCents: number;
  elapsedMs: number;
  estimated?: boolean;
}

/** 流式对话：每收到一段文字就回调一次，结束后回调用量信息。 */
export async function streamChat(
  messages: { role: string; content: string }[],
  onDelta: (text: string) => void,
  onDone?: (meta: StreamMeta) => void
): Promise<void> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/chat/stream`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
    body: JSON.stringify({ messages })
  });
  if (!response.ok || !response.body) {
    const payload = (await response.json().catch(() => null)) as { message?: string } | null;
    throw new Error(payload?.message ?? `调用失败（HTTP ${response.status}）`);
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder("utf-8");
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    buffer += decoder.decode(value, { stream: true });
    const parts = buffer.split("\n\n");
    buffer = parts.pop() ?? "";
    for (const part of parts) {
      const line = part.trim();
      if (!line.startsWith("data:")) {
        continue;
      }
      const payload = JSON.parse(line.slice(5).trim()) as {
        delta?: string;
        done?: boolean;
        error?: string;
      } & StreamMeta;
      if (payload.error) {
        throw new Error(payload.error);
      }
      if (payload.delta) {
        onDelta(payload.delta);
      }
      if (payload.done && onDone) {
        onDone(payload);
      }
    }
  }
}


// ---------- 模型生成提问与方案（失败时前端回退模板） ----------

export async function generateQuestionsByModel(
  topic: string,
  kind: "goal" | "learning"
): Promise<{ questions: string[]; model: string; elapsedMs: number }> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/generate/questions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ topic, kind })
  });
  const payload = (await response.json().catch(() => null)) as
    | { questions?: string[]; model?: string; elapsedMs?: number; message?: string }
    | null;
  if (!response.ok || !payload?.questions) {
    throw new Error(payload?.message ?? `生成失败（HTTP ${response.status}）`);
  }
  return {
    questions: payload.questions,
    model: payload.model ?? "",
    elapsedMs: payload.elapsedMs ?? 0
  };
}

export async function generatePlanByModel(
  topic: string,
  kind: "goal" | "learning"
): Promise<{ stages: { name: string; steps: string[] }[]; model: string; elapsedMs: number }> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/generate/plan`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ topic, kind })
  });
  const payload = (await response.json().catch(() => null)) as
    | { stages?: { name: string; steps: string[] }[]; model?: string; elapsedMs?: number; message?: string }
    | null;
  if (!response.ok || !payload?.stages) {
    throw new Error(payload?.message ?? `生成失败（HTTP ${response.status}）`);
  }
  return {
    stages: payload.stages,
    model: payload.model ?? "",
    elapsedMs: payload.elapsedMs ?? 0
  };
}


/** 把早期对话压成摘要，用于上下文压缩。 */
export async function summarizeChat(
  messages: { role: string; content: string }[]
): Promise<{ summary: string; model: string; elapsedMs: number }> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/chat/summarize`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ messages })
  });
  const payload = (await response.json().catch(() => null)) as
    | { summary?: string; model?: string; elapsedMs?: number; message?: string }
    | null;
  if (!response.ok || !payload?.summary) {
    throw new Error(payload?.message ?? `压缩失败（HTTP ${response.status}）`);
  }
  return {
    summary: payload.summary,
    model: payload.model ?? "",
    elapsedMs: payload.elapsedMs ?? 0
  };
}


// ---------- 微信扫码登录 ----------

export interface WechatStart {
  state: string;
  mode: string;
  qrUrl: string;
  expiresIn: number;
}

export async function wechatLoginStart(): Promise<WechatStart> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/account/wechat/start`, { method: "POST" });
  const payload = (await response.json().catch(() => null)) as (WechatStart & { message?: string }) | null;
  if (!response.ok || !payload) {
    throw new Error(payload?.message ?? `发起微信登录失败（HTTP ${response.status}）`);
  }
  return payload;
}

export async function wechatLoginPoll(
  state: string
): Promise<{ status: string; user?: { account: string; displayName: string; role: string }; message?: string }> {
  const base = await getBaseUrl();
  const response = await fetch(`${base}/api/account/wechat/poll?state=${encodeURIComponent(state)}`);
  const payload = (await response.json().catch(() => null)) as
    | { status?: string; user?: { account: string; displayName: string; role: string }; message?: string }
    | null;
  if (!response.ok) {
    throw new Error(payload?.message ?? `轮询失败（HTTP ${response.status}）`);
  }
  return { status: payload?.status ?? "unknown", user: payload?.user, message: payload?.message };
}
