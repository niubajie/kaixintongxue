﻿import { useCallback, useEffect, useRef, useState } from "react";
import {
  approvePendingDoc,
  deleteSchedule,
  deleteSite,
  fetchKBStatus,
  fetchLibraryScan,
  fetchLibraryTaxonomy,
  fetchPendingDocs,
  fetchSchedules,
  fetchSiteAssistStatus,
  fetchSites,
  startSiteAssist,
  confirmSiteAssistOpen,
  chooseSiteAssistEntry,
  submitSiteAssistSearchURL,
  beginSiteAssistLogin,
  confirmSiteAssistLogin,
  retrySiteAssist,
  cancelSiteAssist,
  rejectPendingDoc,
  saveSchedule,
  saveSite,
  searchKnowledgeBase,
  openExternalUrl,
  setScheduleEnabled,
  verifySite,
  openLibraryFile,
  type KBDocument,
  type KBSearchHit,
  type LibraryItem,
  type LibraryScan,
  type LibraryTaxonomy,
  type PendingDoc,
  type ScheduleFreq,
  type SearchSchedule,
  type SearchSite,
  type SiteAssistStatus
} from "../deployApi";

interface Props {
  onNotice: (message: string) => void;
}

const ALL = "全部资料";

type SearchMode = "local" | "web";

const FREQ_LABEL: Record<ScheduleFreq, string> = {
  instant: "即时",
  daily: "每天",
  weekly: "每周",
  monthly: "每月"
};

const WEEKDAY_LABEL = ["", "周一", "周二", "周三", "周四", "周五", "周六", "周日"];

const ALL_WEEKDAYS = [1, 2, 3, 4, 5, 6, 7];
const ALL_DAYS = Array.from({ length: 28 }, (_, index) => index + 1);

// 资料选项：默认勾选前四项，「留言」和「全部」默认不选。
const TYPE_OPTIONS = ["电子书", "文档", "视频", "杂志", "留言", "全部"];
const DEFAULT_TYPES = ["电子书", "文档", "视频", "杂志"];

// 「全部」与逐项互斥：勾上「全部」就只保留它，取消后回到逐项选择。
function toggleType(current: string[], type: string): string[] {
  if (type === "全部") {
    return current.includes("全部") ? [] : ["全部"];
  }
  const rest = current.filter((item) => item !== "全部");
  return rest.includes(type) ? rest.filter((item) => item !== type) : [...rest, type];
}

// 发布时间：空字符串表示不限。
const PUBLISH_OPTIONS: { value: string; label: string }[] = [
  { value: "", label: "不限" },
  { value: "1d", label: "1天" },
  { value: "3d", label: "3天" },
  { value: "1w", label: "1周" },
  { value: "1m", label: "1月" },
  { value: "3m", label: "3月" }
];

// 下载大小：滑条的 7 个节点（字节），节点之间按区间数值线性插值。
const DOWNLOAD_NODES = [0, 1e9, 10e9, 50e9, 100e9, 500e9, 1e12];
const DOWNLOAD_NODE_LABELS = ["0G", "1G", "10G", "50G", "100G", "500G", "1T"];
// 滑条内部按 0–600 取整：每 100 为一个节点区间。
const DOWNLOAD_STEP = 100;
// 距离节点小于该比例时自动吸附到节点。
const DOWNLOAD_SNAP_RATIO = 0.15;

// 把字节数格式化成界面显示的大小。
function formatDownloadSize(bytes: number): string {
  if (bytes <= 0) {
    return "0G";
  }
  if (bytes >= 1e12) {
    return `${(bytes / 1e12).toFixed(2).replace(/\.?0+$/, "")}T`;
  }
  const gb = bytes / 1e9;
  if (gb < 1) {
    return `${Math.round(bytes / 1e6)}MB`;
  }
  return `${gb.toFixed(1).replace(/\.0$/, "")}G`;
}

// 滑块位置（0–6 的浮点数）→ 字节：在相邻两个节点之间按数值线性插值。
function sliderPosToBytes(pos: number): number {
  const clamped = Math.min(Math.max(pos, 0), DOWNLOAD_NODES.length - 1);
  const lower = Math.floor(clamped);
  const upper = Math.min(lower + 1, DOWNLOAD_NODES.length - 1);
  const ratio = clamped - lower;
  return Math.round(DOWNLOAD_NODES[lower] + (DOWNLOAD_NODES[upper] - DOWNLOAD_NODES[lower]) * ratio);
}

// 字节 → 滑块位置（0–6），编辑已有任务时用来回显。
function bytesToSliderPos(bytes: number): number {
  if (bytes <= 0) {
    return 0;
  }
  for (let index = 0; index < DOWNLOAD_NODES.length - 1; index += 1) {
    const low = DOWNLOAD_NODES[index];
    const high = DOWNLOAD_NODES[index + 1];
    if (bytes <= high) {
      return index + (bytes - low) / (high - low);
    }
  }
  return DOWNLOAD_NODES.length - 1;
}

// 资料分类的展开状态记在本机，切换页面后回来仍保持上次的展开/收起。
const RAIL_EXPANDED_KEY = "kaixin.library.railExpanded";

function scheduleMeta(item: SearchSchedule): string {
  if (item.freq === "instant") {
    return "即时 · 执行一次";
  }
  const time = `${String(item.hour).padStart(2, "0")}:00`;
  if (item.freq === "daily") {
    return `每天 ${time}`;
  }
  if (item.freq === "weekly") {
    const weekdays = item.weekdays?.length ? item.weekdays : ALL_WEEKDAYS;
    const label = weekdays.length === 7 ? "每天" : weekdays.map((day) => WEEKDAY_LABEL[day]).join("、");
    return `每周 ${label} ${time}`;
  }
  const days = item.days?.length ? item.days : ALL_DAYS;
  const label = days.length === 28 ? "每天" : `${days.slice(0, 5).join("、")}${days.length > 5 ? " 等" : ""} 日`;
  return `每月 ${label} ${time}`;
}

// 本地文件路径只展示文件名。
function fileNameOf(path: string): string {
  const parts = path.split(/[\\/]/);
  return parts[parts.length - 1] || path;
}

interface ScheduleDraft {
  id: string;
  keyword: string;
  category: string;
  subCategory: string;
  freq: ScheduleFreq;
  hour: number;
  weekday: number;
  day: number;
  weekdays: number[];
  days: number[];
  types: string[];
  siteIds: string[];
  publishWithin: string;
  downloadLimitBytes: number;
  enabled: boolean;
}

function emptyDraft(): ScheduleDraft {
  return {
    id: "",
    keyword: "",
    category: "",
    subCategory: "",
    freq: "instant",
    hour: 9,
    weekday: 1,
    day: 1,
    weekdays: [...ALL_WEEKDAYS],
    days: [...ALL_DAYS],
    types: [...DEFAULT_TYPES],
    siteIds: [],
    publishWithin: "",
    // 默认 10G，用户可按需调整（0G 表示不下载任何资料）。
    downloadLimitBytes: 10e9,
    enabled: true
  };
}

export function LibraryPage({ onNotice }: Props) {
  const [state, setState] = useState<"loading" | "ready" | "error">("loading");
  const [scan, setScan] = useState<LibraryScan | null>(null);
  const [taxonomy, setTaxonomy] = useState<LibraryTaxonomy | null>(null);
  const [pending, setPending] = useState<PendingDoc[]>([]);
  // 待审核资料的预览弹窗：点条目打开。
  const [preview, setPreview] = useState<PendingDoc | null>(null);
  // 点任务卡弹出的「待审核框」：只列该任务搜集到的资料。
  const [taskPending, setTaskPending] = useState<SearchSchedule | null>(null);
  const [taskDocs, setTaskDocs] = useState<PendingDoc[]>([]);
  const [taskLoading, setTaskLoading] = useState(false);
  const [schedules, setSchedules] = useState<SearchSchedule[]>([]);

  const [mode, setMode] = useState<SearchMode>("local");
  const [keyword, setKeyword] = useState("");
  const [searching, setSearching] = useState(false);
  const [hits, setHits] = useState<KBSearchHit[] | null>(null);

  const [category, setCategory] = useState(ALL);
  const [sub, setSub] = useState("");
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [railExpanded, setRailExpanded] = useState(() => window.localStorage.getItem(RAIL_EXPANDED_KEY) !== "0");

  const [kb, setKb] = useState<Record<string, KBDocument>>({});

  // 点「开始」后的短暂保持：先显示搜索动图，等真实状态回来再切换。
  const [runHolds, setRunHolds] = useState<Record<string, number>>({});
  const [draft, setDraft] = useState<ScheduleDraft | null>(null);
  const [dialogTab, setDialogTab] = useState<"schedule" | "sites">("schedule");
  // 任务卡右键菜单：编辑 / 删除只在右键时出现。
  const [menu, setMenu] = useState<{ id: string; x: number; y: number } | null>(null);

  const [sites, setSites] = useState<SearchSite[]>([]);
  const [siteDraft, setSiteDraft] = useState({ name: "", domain: "" });
  // 每个网站的验证结果：ok=true 时按钮标绿，false 时提示网址无效。
  const [siteChecks, setSiteChecks] = useState<Record<string, { ok: boolean; message: string }>>({});
  const [verifyingSiteId, setVerifyingSiteId] = useState("");
  const verifyAbortRef = useRef<AbortController | null>(null);
  const verifyRunRef = useRef(0);
  const handledIssueRef = useRef("");
  const repairNoticeRef = useRef("");
  // 分步网站修复：后端返回当前步骤，前端只提交用户确认。
  const [siteAssist, setSiteAssist] = useState<SiteAssistStatus | null>(null);
  const [assistBusy, setAssistBusy] = useState(false);
  const [assistSearchURL, setAssistSearchURL] = useState("");
  const assistBaselineRef = useRef(0);
  const assistResumeStartedRef = useRef(0);

  // 网站设置：行内编辑后点「保存」写回，新增走底部的一行输入。
  const patchSite = (id: string, patch: Partial<SearchSite>) => {
    setSites((current) => current.map((item) => (item.id === id ? { ...item, ...patch } : item)));
    // 域名或名称变了，之前的验证结论失效，需要重新验证。
    if (patch.domain !== undefined || patch.name !== undefined) {
      setSiteChecks((current) => {
        if (!current[id]) {
          return current;
        }
        const next = { ...current };
        delete next[id];
        return next;
      });
    }
  };

  // 验证网站：显示可取消的进度弹窗；必须真能下载到资料才算有效。
  const verifySiteRow = async (site: SearchSite) => {
    if (verifyingSiteId) {
      return;
    }
    const runID = ++verifyRunRef.current;
    const controller = new AbortController();
    verifyAbortRef.current = controller;
    setVerifyingSiteId(site.id);
    try {
      const result = await verifySite(site.domain, keyword.trim(), controller.signal);
      if (runID !== verifyRunRef.current) {
        return;
      }
      setSiteChecks((current) => ({
        ...current,
        [site.id]: { ok: result.ok, message: result.hint ? `${result.message}；${result.hint}` : result.message }
      }));
      // 验证没通过就进入分步修复，以真实下载为终点。
      if (!result.ok) {
        void openSiteAssist(site);
      }
      onNotice(
        result.ok
          ? `${site.name || site.domain} 验证通过：${result.message}`
          : `${site.name || site.domain} 验证未通过，已进入分步修复。`
      );
    } catch (err: unknown) {
      if (runID !== verifyRunRef.current || (err instanceof Error && err.name === "AbortError")) {
        return;
      }
      onNotice(err instanceof Error ? err.message : "验证网站失败");
    } finally {
      if (runID === verifyRunRef.current) {
        verifyAbortRef.current = null;
        setVerifyingSiteId("");
      }
    }
  };

  const cancelSiteVerify = () => {
    verifyRunRef.current += 1;
    verifyAbortRef.current?.abort();
    verifyAbortRef.current = null;
    setVerifyingSiteId("");
    onNotice("已取消网站验证。");
  };

  const openSiteAssist = async (site: SearchSite, scheduleID?: string) => {
    setAssistBusy(true);
    try {
      const taskKeyword = scheduleID ? schedules.find((item) => item.id === scheduleID)?.keyword : "";
      // 记录向导打开时的已搜集数，后续只要真实增加就判定下载恢复（避免 ready 时再取值的竞态）。
      if (scheduleID) {
        assistBaselineRef.current = schedules.find((item) => item.id === scheduleID)?.collected ?? 0;
      }
      const status = await startSiteAssist(site.id, taskKeyword || keyword.trim() || "测试", scheduleID);
      assistResumeStartedRef.current = 0;
      setAssistSearchURL(status.entryUrl);
      setSiteAssist(status);
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "打开网站修复向导失败");
    } finally {
      setAssistBusy(false);
    }
  };

  const runAssistAction = async (action: () => Promise<SiteAssistStatus>) => {
    setAssistBusy(true);
    try {
      const status = await action();
      setSiteAssist(status);
      if (status.state === "awaiting_search_url") {
        setAssistSearchURL(status.entryUrl);
      }
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "网站修复操作失败");
    } finally {
      setAssistBusy(false);
    }
  };

  const closeSiteAssist = (giveUp = false) => {
    const sessionID = siteAssist?.sessionId;
    const wasFailed = siteAssist?.state === "failed";
    assistResumeStartedRef.current = 0;
    setSiteAssist(null);
    if (sessionID) {
      // giveUp=true 才真正放弃本站；默认关闭时后端会让监督程序自动重开修复。
      void cancelSiteAssist(sessionID, { giveUp }).catch(() => undefined);
    }
    if (giveUp || wasFailed) {
      // 放弃或本次失败已处理：清空去重标记，允许下一次失败 / 再次点击开始重新弹出向导。
      handledIssueRef.current = "";
    }
  };



  // 监听后台搜集：进入自动修复时明确提示；需要登录则自动打开登录页。
  useEffect(() => {
    const repairing = schedules.find((item) => item.running && item.phase === "repair");
    if (repairing && repairNoticeRef.current !== repairing.id) {
      repairNoticeRef.current = repairing.id;
      onNotice("网址无法进入搜索，修正中…");
    } else if (!repairing) {
      repairNoticeRef.current = "";
    }

    if (siteAssist) {
      return;
    }
    const task = schedules.find((item) => item.issueSiteId);
    if (!task?.issueSiteId) {
      return;
    }
    const key = `${task.id}:${task.issueSiteId}:${task.issueNonce ?? 0}:${task.issueMessage ?? ""}`;
    if (handledIssueRef.current === key) {
      return;
    }
    const site = sites.find((item) => item.id === task.issueSiteId);
    if (!site) {
      return;
    }
    handledIssueRef.current = key;
    void openSiteAssist(site, task.id);
  }, [schedules, sites, onNotice, siteAssist]);



  // 轮询后端步骤；终态和等待真实下载期间不重复查询。
  useEffect(() => {
    if (!siteAssist || ["failed", "closed", "ready", "downloading"].includes(siteAssist.state)) {
      return;
    }
    const sessionID = siteAssist.sessionId;
    const timer = window.setInterval(() => {
      fetchSiteAssistStatus(sessionID)
        .then((status) => {
          setSiteAssist((current) => (current?.sessionId === sessionID ? status : current));
          if (status.state === "awaiting_search_url") {
            setAssistSearchURL(status.entryUrl);
          }
        })
        .catch(() => undefined);
    }, 1500);
    return () => window.clearInterval(timer);
  }, [siteAssist?.sessionId, siteAssist?.state]);

  // 搜索入口和资料都已验证，恢复原任务；站内结果由后端优先交给下载器。
  useEffect(() => {
    if (!siteAssist?.scheduleId || siteAssist.state !== "ready" || assistResumeStartedRef.current) {
      return;
    }
    const scheduleID = siteAssist.scheduleId;
    const task = schedules.find((item) => item.id === scheduleID);
    assistBaselineRef.current = task?.collected ?? 0;
    assistResumeStartedRef.current = Date.now();
    void setScheduleEnabled(scheduleID, true)
      .then(fetchSchedules)
      .then(setSchedules)
      .catch((err: unknown) => {
        assistResumeStartedRef.current = 0;
        setSiteAssist((current) => current ? { ...current, state: "failed", message: err instanceof Error ? err.message : "恢复任务失败" } : current);
      });
  }, [siteAssist, schedules]);

  // 只有已搜集数量真实增加才提示“下载中”，然后自动关闭弹窗。
  useEffect(() => {
    if (!siteAssist?.scheduleId || !assistResumeStartedRef.current || siteAssist.state !== "ready") {
      return;
    }
    const task = schedules.find((item) => item.id === siteAssist.scheduleId);
    if (!task) {
      return;
    }
    if (task.collected > assistBaselineRef.current) {
      const sessionID = siteAssist.sessionId;
      assistResumeStartedRef.current = 0;
      setSiteAssist((current) => current ? { ...current, state: "downloading", message: "下载中，网站修复完成" } : current);
      void cancelSiteAssist(sessionID).catch(() => undefined);
      return;
    }
    if (!task.running && Date.now() - assistResumeStartedRef.current > 8000) {
      assistResumeStartedRef.current = 0;
      setSiteAssist((current) => current ? { ...current, state: "failed", message: "任务已恢复，但没有新增下载；资料可能已下载过，或写盘失败" } : current);
    }
  }, [schedules, siteAssist]);

  useEffect(() => {
    if (siteAssist?.state !== "downloading") {
      return;
    }
    const sessionID = siteAssist.sessionId;
    const timer = window.setTimeout(() => {
      setSiteAssist((current) => (current?.sessionId === sessionID ? null : current));
    }, 1200);
    return () => window.clearTimeout(timer);
  }, [siteAssist?.state, siteAssist?.sessionId]);

  const assistSite = siteAssist ? sites.find((item) => item.id === siteAssist.siteId) : undefined;
  const assistStep = !siteAssist ? 0
    : ["starting", "awaiting_open"].includes(siteAssist.state) ? 1
    : ["detecting_search", "searching", "awaiting_entry_choice", "awaiting_search_url"].includes(siteAssist.state) ? 2
    : ["login_waiting", "login_verifying"].includes(siteAssist.state) ? 3
    : 4;

  const persistSite = async (site: SearchSite) => {
    try {
      const items = await saveSite({
        id: site.id,
        name: site.name,
        domain: site.domain,
        enabled: site.enabled
      });
      setSites(items);
      onNotice("网站设置已保存。");
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "保存网站失败");
    }
  };

  const addSite = async () => {
    if (!siteDraft.name.trim() || !siteDraft.domain.trim()) {
      onNotice("请填写网站名称与域名。");
      return;
    }
    try {
      const items = await saveSite({ name: siteDraft.name, domain: siteDraft.domain });
      setSites(items);
      setSiteDraft({ name: "", domain: "" });
      onNotice("已添加网站。");
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "添加网站失败");
    }
  };

  const removeSite = async (id: string) => {
    try {
      const items = await deleteSite(id);
      setSites(items);
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "删除网站失败");
    }
  };

  const load = useCallback(async () => {
    setState("loading");
    try {
      const [result, tree, kbDocs] = await Promise.all([
        fetchLibraryScan(),
        fetchLibraryTaxonomy().catch(() => null),
        fetchKBStatus().catch(() => [] as KBDocument[])
      ]);
      setScan(result);
      setTaxonomy(tree);
      const map: Record<string, KBDocument> = {};
      for (const doc of kbDocs) {
        map[doc.path] = doc;
      }
      setKb(map);
      setState("ready");
    } catch {
      setScan(null);
      setState("error");
    }
    void fetchPendingDocs().then(setPending).catch(() => setPending([]));
    void fetchSchedules().then(setSchedules).catch(() => setSchedules([]));
    void fetchSites().then(setSites).catch(() => setSites([]));
  }, []);

  // 读取某个定时任务搜集到的待审核资料；轮询刷新时不再显示加载态。
  const loadTaskDocs = useCallback(async (scheduleId: string, showLoading = true) => {
    if (showLoading) {
      setTaskLoading(true);
    }
    try {
      setTaskDocs(await fetchPendingDocs(scheduleId));
    } catch {
      setTaskDocs([]);
    } finally {
      if (showLoading) {
        setTaskLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  // 定时任务由 sidecar 在后台执行，页面定期刷新待审核与任务状态。
  // 任务执行时会有搜索动画与计数，刷新间隔缩短到 4 秒以便看到进度。
  useEffect(() => {
    const timer = window.setInterval(() => {
      void fetchPendingDocs().then(setPending).catch(() => undefined);
      void fetchSchedules().then(setSchedules).catch(() => undefined);
    }, 4000);
    return () => window.clearInterval(timer);
  }, []);

  // 待审核框打开时同样定时刷新，任务边搜边出结果。
  const taskPendingId = taskPending?.id ?? "";
  useEffect(() => {
    if (!taskPendingId) {
      return;
    }
    const timer = window.setInterval(() => {
      void loadTaskDocs(taskPendingId, false);
      void fetchSchedules()
        .then((items) => {
          setSchedules(items);
          const latest = items.find((item) => item.id === taskPendingId);
          if (latest) {
            setTaskPending(latest);
          }
        })
        .catch(() => undefined);
    }, 4000);
    return () => window.clearInterval(timer);
  }, [taskPendingId, loadTaskDocs]);

  const indexedCount = Object.values(kb).filter((doc) => doc.status === "indexed").length;
  const categories = taxonomy?.categories ?? [];
  const enabledSites = sites.filter((site) => site.enabled);
  const visibleItems = (scan?.items ?? []).filter((item) => {
    if (category === ALL) {
      return true;
    }
    if (item.category !== category) {
      return false;
    }
    return sub === "" || item.subCategory === sub;
  });

  const openCategory = (name: string) => {
    setCategory(name);
    setSub("");
  };

  // 点「全部资料」：筛选全部，同时收起 / 展开下方分类，状态记到本机。
  const toggleRail = () => {
    setRailExpanded((current) => {
      const next = !current;
      window.localStorage.setItem(RAIL_EXPANDED_KEY, next ? "1" : "0");
      return next;
    });
  };

  const openSub = (name: string, subName: string) => {
    setCategory(name);
    setSub(subName);
  };

  // 点「外网搜集」：带上输入框关键词与当前分类，弹出选项框，确认后建任务并立即执行。
  const openWebCollectDialog = async () => {
    const kw = keyword.trim();
    if (!kw) {
      onNotice("请先输入要搜集的关键词。");
      return;
    }
    // 网站列表用于默认勾选；还没加载到就补一次。
    let available = sites;
    if (available.length === 0) {
      try {
        available = await fetchSites();
        setSites(available);
      } catch {
        available = [];
      }
    }
    setDialogTab("schedule");
    setMode("web");
    setDraft({
      ...emptyDraft(),
      keyword: kw,
      category: category === ALL ? "未分类" : category,
      subCategory: sub,
      freq: "instant",
      siteIds: available.filter((site) => site.enabled).map((site) => site.id)
    });
  };

  const submitSearch = async (nextMode: SearchMode = mode) => {
    // 外网搜集统一走选项框，由用户确认资料类型 / 网站 / 发布时间 / 下载大小后再执行。
    if (nextMode === "web") {
      await openWebCollectDialog();
      return;
    }
    const kw = keyword.trim();
    if (!kw) {
      setHits(null);
      return;
    }
    setSearching(true);
    try {
      setHits(await searchKnowledgeBase(kw));
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "本地检索失败");
    } finally {
      setSearching(false);
    }
  };

  // 点任务卡：打开该任务的待审核框。
  const openTaskPending = (item: SearchSchedule) => {
    setTaskPending(item);
    setTaskDocs([]);
    void loadTaskDocs(item.id);
  };

  const handleApprove = async (doc: PendingDoc) => {
    try {
      const result = await approvePendingDoc(doc.id);
      onNotice(`已通过审核：${result.relPath}`);
      await load();
      if (taskPending) {
        void loadTaskDocs(taskPending.id, false);
      }
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "入库失败");
    }
  };

  // 预览：优先打开已下载到本地临时资料库的文件，没有本地文件时退回原文链接。
  const openPendingFile = async (doc: PendingDoc) => {
    if (doc.localPath) {
      try {
        await openLibraryFile(doc.localPath);
        return;
      } catch (err: unknown) {
        onNotice(err instanceof Error ? err.message : "打开本地文件失败");
        return;
      }
    }
    if (!doc.url) {
      onNotice("这条资料还没有下载到本地，也没有原文链接。");
      return;
    }
    try {
      await openExternalUrl(doc.url);
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "打开链接失败");
    }
  };

  // 打开原文：始终走系统默认浏览器。
  const openPreviewSource = async (doc: PendingDoc) => {
    if (!doc.url) {
      onNotice("这条资料没有原文链接。");
      return;
    }
    try {
      await openExternalUrl(doc.url);
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "打开链接失败");
    }
  };

  const handleReject = async (doc: PendingDoc) => {
    try {
      await rejectPendingDoc(doc.id);
      onNotice(`已忽略并加入黑名单：${doc.title}`);
      setPending((current) => current.filter((item) => item.id !== doc.id));
      setTaskDocs((current) => current.filter((item) => item.id !== doc.id));
      if (taxonomy) {
        setTaxonomy({ ...taxonomy, pending: Math.max(0, taxonomy.pending - 1) });
      }
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "忽略失败");
    }
  };

  const openScheduleDialog = (item?: SearchSchedule) => {
    setDialogTab("schedule");
    // 网站列表是异步加载的；打开弹窗时若还没拿到，补一次请求。
    if (sites.length === 0) {
      void fetchSites().then(setSites).catch(() => undefined);
    }
    if (item) {
      setDraft({
        id: item.id,
        keyword: item.keyword,
        category: item.category,
        subCategory: item.subCategory,
        freq: item.freq,
        hour: item.hour,
        weekday: item.weekday,
        day: item.day,
        weekdays: item.weekdays?.length ? item.weekdays : [...ALL_WEEKDAYS],
        days: item.days?.length ? item.days : [...ALL_DAYS],
        types: item.types?.length ? item.types : [...DEFAULT_TYPES],
        // siteIds 原样保留：用户勾了哪些网站就只搜哪些，全不勾 = 不搜索。
        siteIds: [...(item.siteIds ?? [])],
        publishWithin: item.publishWithin ?? "",
        downloadLimitBytes: item.downloadLimitBytes ?? 10e9,
        enabled: item.enabled
      });
      return;
    }
    // 新建任务默认勾选当前所有启用网站，用户可自行取消。
    setDraft({ ...emptyDraft(), siteIds: sites.filter((site) => site.enabled).map((site) => site.id) });
  };

  const submitSchedule = async () => {
    if (!draft) {
      return;
    }
    if (!draft.keyword.trim()) {
      onNotice("请输入定时搜索的关键词。");
      return;
    }
    if (draft.types.length === 0) {
      onNotice("请至少选择一种资料类型。");
      return;
    }
    try {
      await saveSchedule({
        id: draft.id,
        keyword: draft.keyword.trim(),
        category: draft.category,
        subCategory: draft.subCategory,
        freq: draft.freq,
        hour: draft.hour,
        weekday: draft.weekday,
        day: draft.day,
        weekdays: draft.weekdays,
        days: draft.days,
        types: draft.types,
        siteIds: draft.siteIds,
        publishWithin: draft.publishWithin,
        downloadLimitBytes: draft.downloadLimitBytes,
        enabled: draft.enabled,
        collected: 0,
        running: false,
        lastRunAt: "",
        createdAt: ""
      });
      onNotice(
        draft.id
          ? "定时任务已更新。"
          : draft.freq === "instant"
            ? "已开始搜集，资料会陆续进入待审核区。"
            : "定时任务已创建，应用运行时自动执行。"
      );
      setDraft(null);
      setSchedules(await fetchSchedules());
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "保存定时任务失败");
    }
  };

  const removeSchedule = async () => {
    if (!draft?.id) {
      return;
    }
    try {
      await deleteSchedule(draft.id);
      onNotice("定时任务已删除。");
      setDraft(null);
      setSchedules(await fetchSchedules());
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "删除定时任务失败");
    }
  };

  const removeScheduleById = async (id: string) => {
    try {
      await deleteSchedule(id);
      onNotice("定时任务已删除。");
      setSchedules(await fetchSchedules());
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "删除定时任务失败");
    }
  };

  // 暂停 / 开始：暂停后调度器不再触发，正在跑的一轮也会在下一个网站前停下。
  const toggleSchedule = async (item: SearchSchedule) => {
    const next = !item.enabled;
    if (next) {
      // 点了「开始」先在本地亮起搜索动图，任务跑得很快也不会一闪而过看不到。
      setRunHolds((current) => ({ ...current, [item.id]: Date.now() + 3500 }));
      setSchedules((current) =>
        current.map((row) => (row.id === item.id ? { ...row, enabled: true, running: true } : row))
      );
      // 每次手动「开始」都允许重新弹出修复向导：清掉去重标记，避免同一错误的新一轮被吞掉。
      handledIssueRef.current = "";
    }
    try {
      await setScheduleEnabled(item.id, next);
      onNotice(next ? "任务已开始，正在搜索…" : "任务已暂停。");
      setSchedules(await fetchSchedules());
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "更新任务状态失败");
    }
  };

  const openItem = async (item: LibraryItem) => {
    try {
      await openLibraryFile(item.path);
      onNotice(`已用默认程序打开：${item.name}`);
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "打开文件失败");
    }
  };

  const openHit = async (hit: KBSearchHit) => {
    try {
      await openLibraryFile(hit.path);
      onNotice(`已用默认程序打开：${hit.name}`);
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "打开文件失败");
    }
  };

  return (
    <div className="page page-library">
      <div className="page-head">
        <h1 className="page-title">本地资料</h1>
      </div>

      <div className="lib-shell">
        <aside className="card lib-rail" aria-label="资料分类">
          <div className="lib-rail-head">
            <span className="section-title">资料分类</span>
          </div>
          <button
            type="button"
            className="tree-group"
            aria-expanded={railExpanded}
            onClick={() => {
              openCategory(ALL);
              toggleRail();
            }}
            style={category === ALL ? { background: "var(--accent-light)", color: "var(--accent)" } : undefined}
          >
            <span>
              <span className="rail-caret">{railExpanded ? "▾" : "▸"}</span>
              {ALL}
            </span>
            <span className="caption">{scan?.total ?? 0} 份</span>
          </button>
          {railExpanded &&
            categories.map((cat) => (
            <div key={cat.key}>
              <div className="tree-cat" data-key={cat.key}>
                <button
                  type="button"
                  className="tree-cat-main"
                  onClick={() => openCategory(cat.key)}
                  style={
                    category === cat.key && sub === ""
                      ? { background: "var(--accent-light)", color: "var(--accent)" }
                      : undefined
                  }
                >
                  <span>{cat.name === "未分类" ? "其它" : cat.name}</span>
                  <span className="caption">{cat.count} 份</span>
                </button>
                {cat.subs.length > 0 && (
                  <button
                    type="button"
                    className="tree-arrow"
                    aria-label={`展开 ${cat.name === "未分类" ? "其它" : cat.name} 子分类`}
                    onClick={() => setExpanded((current) => ({ ...current, [cat.key]: !current[cat.key] }))}
                  >
                    {expanded[cat.key] ? "▾" : "▸"}
                  </button>
                )}
              </div>
              {expanded[cat.key] &&
                cat.subs.map((item) => (
                  <button
                    type="button"
                    className="tree-sub"
                    key={item.name}
                    onClick={() => openSub(cat.key, item.name)}
                    style={
                      category === cat.key && sub === item.name
                        ? { background: "var(--accent-light)", color: "var(--accent)" }
                        : undefined
                    }
                  >
                    <span>{item.name}</span>
                    <span className="caption">{item.count}</span>
                  </button>
                ))}
            </div>
          ))}
        </aside>

        <div className="lib-main">
      <div className="lib-board">
        <div className="card lib-board-card">
          <span className="lib-board-icon">总</span>
          <div>
            <div className="lib-board-label">资料总数</div>
            <div className="lib-board-value">
              {scan?.total ?? 0}
              <em className="unit">份</em>
            </div>
            <div className="lib-board-hint">{scan?.groups ?? 0} 个目录</div>
          </div>
        </div>
        <div className="card lib-board-card">
          <span className="lib-board-icon">库</span>
          <div>
            <div className="lib-board-label">已入知识库</div>
            <div className="lib-board-value">
              {indexedCount}
              <em className="unit">份</em>
            </div>
            <div className="lib-board-hint">可被本地检索命中</div>
          </div>
        </div>
        <div className="card lib-board-card">
          <span className="lib-board-icon">周</span>
          <div>
            <div className="lib-board-label">本周新增</div>
            <div className="lib-board-value">
              {scan?.thisWeek ?? 0}
              <em className="unit">份</em>
            </div>
            <div className="lib-board-hint">近 7 天修改</div>
          </div>
        </div>
        <div className="card lib-board-card">
          <span className="lib-board-icon">量</span>
          <div>
            <div className="lib-board-label">占用空间</div>
            <div className="lib-board-value">{scan?.sizeText ?? "0 B"}</div>
            <div className="lib-board-hint">{pending.length} 条待审核</div>
          </div>
        </div>
      </div>

      {state !== "error" && scan && !scan.exists && (
        <div className="card" style={{ marginTop: 16 }}>
          <div style={{ fontSize: 13, fontWeight: 600 }}>还没有资料目录</div>
          <p className="caption" style={{ marginTop: 6 }}>
            点「打开资料文件夹」会自动创建并打开，把文件按「工作 / 生活 / 学习 / 爱好」建子文件夹放进去。
          </p>
          <p className="caption" style={{ marginTop: 4 }}>目录：{scan.root}</p>
        </div>
      )}

      {state === "ready" && scan && scan.exists && (
        <div className="card lib-files">
          <div className="row" style={{ justifyContent: "space-between", marginBottom: 6 }}>
            <span className="section-title">
              {category === ALL
                ? ALL
                : `${category === "未分类" ? "其它" : category}${sub ? ` / ${sub}` : ""}`}{" "}
              · {visibleItems.length} 份
            </span>
            <span className="caption">点击文件名用默认程序打开</span>
          </div>
          {visibleItems.length === 0 ? (
            <p className="caption">
              该分类下暂无文件。把文件放进对应文件夹，或用「外网搜集」通过审核。
            </p>
          ) : (
            <div className="lib-file-list">
              {visibleItems.map((item) => (
                <button
                  type="button"
                  className="lib-hit"
                  key={item.relPath}
                  onClick={() => void openItem(item)}
                >
                  <div className="lib-hit-title">{item.name}</div>
                  <p className="lib-hit-snippet">
                    {item.relPath} · {item.sizeText} · {item.date || item.modTime}
                  </p>
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      <div className="lib-search">
        <div className="lib-search-bar">
          <input
            className="input"
            disabled={searching}
            placeholder={
              mode === "local"
                ? "输入关键词，检索已加入知识库的资料"
                : "输入关键词，点「外网搜集」设置后搜集"
            }
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                void submitSearch();
              }
            }}
          />
          <div className="seg">
            <button
              type="button"
              className={mode === "local" ? "seg-item active" : "seg-item"}
              onClick={() => {
                setMode("local");
                if (keyword.trim()) {
                  void submitSearch("local");
                }
              }}
            >
              本地
            </button>
            <button
              type="button"
              className={mode === "web" ? "seg-item active" : "seg-item"}
              onClick={() => {
                setMode("web");
                // 统一走弹窗入口：没输入关键词时也会给出提示，而不是毫无反应。
                void openWebCollectDialog();
              }}
            >
              外网搜集
            </button>
          </div>
          <button
            type="button"
            className="seg-gear"
            title="定时搜索"
            aria-label="定时搜索"
            onClick={() => openScheduleDialog()}
          >
            ⚙
          </button>
        </div>

        {schedules.length > 0 && (
          <div className="schedule-grid">
            {schedules.map((item) => {
              const running = item.running || (runHolds[item.id] ?? 0) > Date.now();
              return (
              <div
                key={item.id}
                className="card schedule-card"
                onClick={() => openTaskPending(item)}
                onContextMenu={(event) => {
                  event.preventDefault();
                  setMenu({ id: item.id, x: event.clientX, y: event.clientY });
                }}
                title="点击查看已搜集资料，右键可编辑或删除"
              >
                <div className="schedule-title">{item.keyword}</div>
                <div className="schedule-status">
                  {running ? (
                    item.phase === "repair" ? (
                      <span className="schedule-running">
                        <span className="search-anim" aria-hidden="true">
                          <span className="search-lens" />
                          <span className="search-sweep" />
                        </span>
                        网址无法进入搜索，修正中…
                      </span>
                    ) : item.phase === "download" ? (
                      <span className="schedule-running">
                        <span className="download-anim" aria-hidden="true">
                          <span className="dl-arrow" />
                          <span className="dl-tray" />
                        </span>
                        正在下载… 已搜集 {item.collected} 条
                      </span>
                    ) : (
                      <span className="schedule-running">
                        <span className="search-anim" aria-hidden="true">
                          <span className="search-lens" />
                          <span className="search-sweep" />
                        </span>
                        正在搜索… 已搜集 {item.collected} 条
                      </span>
                    )
                  ) : (
                    <span className="schedule-count">已搜集 {item.collected} 条</span>
                  )}
                  <button
                    type="button"
                    className="ghost-btn schedule-toggle"
                    onClick={(event) => {
                      event.stopPropagation();
                      void toggleSchedule(item);
                    }}
                  >
                    {running || item.enabled ? "暂停" : "开始"}
                  </button>
                </div>
              </div>
              );
            })}
          </div>
        )}

        {hits !== null && (
          <div className="card lib-hits">
            <div className="row" style={{ justifyContent: "space-between", marginBottom: 6 }}>
              <span className="section-title">检索结果 · {hits.length}</span>
              <button type="button" className="ghost-btn" onClick={() => setHits(null)}>
                清除
              </button>
            </div>
            {hits.length === 0 ? (
              <p className="caption">
                没有命中。外网搜集的资料通过审核后，即可被本地检索。
              </p>
            ) : (
              hits.map((hit) => (
                <button
                  type="button"
                  className="lib-hit"
                  key={`${hit.path}-${hit.idx}`}
                  onClick={() => void openHit(hit)}
                >
                  <div className="lib-hit-title">{hit.name}</div>
                  <p className="lib-hit-snippet">{hit.snippet}</p>
                </button>
              ))
            )}
          </div>
        )}
      </div>

      {pending.length > 0 && (
        <div className="card lib-pending">
          <div className="row" style={{ justifyContent: "space-between", marginBottom: 8 }}>
            <span className="section-title">待审核资料 · {pending.length}</span>
            <span className="caption">确认后写入对应分类并加入知识库</span>
          </div>
          <div className="lib-pending-list">
            {pending.map((doc) => (
              <div
                className="lib-pending-row clickable"
                key={doc.id}
                onClick={() => setPreview(doc)}
                title="点击预览"
              >
                <div className="lib-pending-main">
                  <div className="lib-pending-title">
                    {doc.title}
                    <span className="tag" style={{ marginLeft: 8 }}>
                      {doc.category}
                      {doc.subCategory ? ` / ${doc.subCategory}` : ""}
                    </span>
                    {doc.source === "demo" && (
                      <span className="tag" style={{ marginLeft: 6 }}>演示源</span>
                    )}
                  </div>
                  <p className="lib-pending-snippet">{doc.snippet}</p>
                  <p className="caption">
                    关键词：{doc.keyword} · 采集时间：{doc.createdAt}
                    {doc.localPath ? ` · 本地文件：${fileNameOf(doc.localPath)}` : ""}
                  </p>
                </div>
                <div className="lib-pending-actions" onClick={(event) => event.stopPropagation()}>
                  <button type="button" className="ghost-btn" onClick={() => void openPendingFile(doc)}>
                    预览
                  </button>
                  <button type="button" className="primary-btn" onClick={() => void handleApprove(doc)}>
                    通过审核
                  </button>
                  <button type="button" className="ghost-btn" onClick={() => void handleReject(doc)}>
                    忽略
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
        </div>
      </div>

      {taskPending && (
        <div className="dialog-mask" onClick={() => setTaskPending(null)}>
          <div className="dialog dialog-task-pending" onClick={(event) => event.stopPropagation()}>
            <button
              type="button"
              className="dialog-close"
              aria-label="关闭"
              title="关闭"
              onClick={() => setTaskPending(null)}
            >
              ×
            </button>
            <div className="preview-title">待审核资料 · {taskPending.keyword}</div>
            <div className="preview-meta">
              <span className="tag">
                {taskPending.category}
                {taskPending.subCategory ? ` / ${taskPending.subCategory}` : ""}
              </span>
              <span className="caption">
                {scheduleMeta(taskPending)} · 待审核 {taskDocs.length} 条
                {taskPending.running ? " · 正在搜索…" : ""}
              </span>
            </div>
            {taskLoading ? (
              <p className="caption">正在读取该任务的资料…</p>
            ) : taskDocs.length === 0 ? (
              <p className="caption">
                该任务还没有搜到可下载的资料。只收录能下载到本地的文档、图片和视频，任务执行中会自动出现在这里。
              </p>
            ) : (
              <div className="lib-pending-list task-pending-list">
                {taskDocs.map((doc) => (
                  <div
                    className="lib-pending-row clickable"
                    key={doc.id}
                    onClick={() => setPreview(doc)}
                    title="点击查看详情"
                  >
                    <div className="lib-pending-main">
                      <div className="lib-pending-title">
                        {doc.title}
                        <span className="tag" style={{ marginLeft: 8 }}>
                          {doc.category}
                          {doc.subCategory ? ` / ${doc.subCategory}` : ""}
                        </span>
                      </div>
                      <p className="lib-pending-snippet">{doc.snippet}</p>
                      <p className="caption">
                        来源：{doc.source || "未知"} · 采集时间：{doc.createdAt}
                        {doc.localPath ? ` · 本地文件：${fileNameOf(doc.localPath)}` : ""}
                      </p>
                    </div>
                    <div className="lib-pending-actions" onClick={(event) => event.stopPropagation()}>
                      <button type="button" className="ghost-btn" onClick={() => void openPendingFile(doc)}>
                        预览
                      </button>
                      <button type="button" className="primary-btn" onClick={() => void handleApprove(doc)}>
                        通过审核
                      </button>
                      <button type="button" className="ghost-btn" onClick={() => void handleReject(doc)}>
                        忽略
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {preview && (
        <div className="dialog-mask" onClick={() => setPreview(null)}>
          <div className="dialog dialog-preview" onClick={(event) => event.stopPropagation()}>
            <button type="button" className="dialog-close" aria-label="关闭" title="关闭" onClick={() => setPreview(null)}>
              ×
            </button>
            <div className="preview-title">{preview.title}</div>
            <div className="preview-meta">
              <span className="tag">
                {preview.category}
                {preview.subCategory ? ` / ${preview.subCategory}` : ""}
              </span>
              <span className="caption">
                来源：{preview.source || "未知"} · 关键词：{preview.keyword}
              </span>
            </div>
            <p className="preview-snippet">{preview.snippet || "（这条资料没有摘要）"}</p>
            <p className="caption preview-url">
              本地文件：{preview.localPath ? fileNameOf(preview.localPath) : "未下载"}
            </p>
            <p className="caption preview-url">原文链接：{preview.url || "无"}</p>
            <div className="preview-actions">
              {preview.localPath && (
                <button type="button" className="ghost-btn" onClick={() => void openPendingFile(preview)}>
                  打开本地文件
                </button>
              )}
              <button type="button" className="ghost-btn" onClick={() => void openPreviewSource(preview)}>
                打开原文
              </button>
              <button
                type="button"
                className="ghost-btn danger"
                onClick={() => {
                  setPreview(null);
                  void handleReject(preview);
                }}
              >
                忽略
              </button>
              <button
                type="button"
                className="primary-btn"
                onClick={() => {
                  setPreview(null);
                  void handleApprove(preview);
                }}
              >
                通过审核
              </button>
            </div>
          </div>
        </div>
      )}

      {verifyingSiteId && (
        <div className="dialog-mask verify-progress-mask">
          <div className="dialog dialog-verify" role="status" aria-live="polite">
            <div className="verify-dots" aria-hidden="true">
              <span />
              <span />
              <span />
            </div>
            <div className="preview-title">正在验证中</div>
            <p className="caption">
              {sites.find((site) => site.id === verifyingSiteId)?.name || "网站"}，请稍候…
            </p>
            <div className="preview-actions" style={{ justifyContent: "center" }}>
              <button type="button" className="ghost-btn" onClick={cancelSiteVerify}>
                取消
              </button>
            </div>
          </div>
        </div>
      )}

      {siteAssist && (
        <div className="dialog-mask login-assist-mask" onClick={() => closeSiteAssist()}>
          <div className="dialog dialog-login site-assist-dialog" onClick={(event) => event.stopPropagation()}>
            <button type="button" className="dialog-close" aria-label="取消修复" title="取消修复" onClick={() => closeSiteAssist()}>×</button>
            <div className="preview-title">网站修复：{assistSite?.name || assistSite?.domain || siteAssist.entryUrl}</div>
            <div className="assist-steps">
              {["打开网站", "定位搜索", "登录（如需）", "恢复下载"].map((label, index) => {
                const step = index + 1;
                return (
                  <div key={label} className={`assist-step ${assistStep === step ? "active" : assistStep > step ? "done" : ""}`}>
                    <span>{assistStep > step ? "✓" : step}</span>{label}
                  </div>
                );
              })}
            </div>
            <p className="assist-message">{siteAssist.message}</p>
            {["starting", "detecting_search", "searching", "login_verifying"].includes(siteAssist.state) && (
              <div className="verify-progress"><div className="verify-dots"><span /><span /><span /></div></div>
            )}
            {siteAssist.state === "awaiting_open" && (
              <div className="preview-actions">
                <button type="button" className="primary-btn" disabled={assistBusy} onClick={() => void runAssistAction(() => confirmSiteAssistOpen(siteAssist.sessionId, true))}>能打开，继续</button>
                <button type="button" className="ghost-btn" disabled={assistBusy} onClick={() => void runAssistAction(() => confirmSiteAssistOpen(siteAssist.sessionId, false))}>打不开</button>
              </div>
            )}
            {siteAssist.state === "awaiting_entry_choice" && (
              <div className="preview-actions assist-actions-column">
                <button type="button" className="primary-btn" disabled={assistBusy} onClick={() => void runAssistAction(() => chooseSiteAssistEntry(siteAssist.sessionId, "advanced"))}>有搜索入口，深度识别</button>
                <button type="button" className="ghost-btn" disabled={assistBusy} onClick={() => void runAssistAction(() => chooseSiteAssistEntry(siteAssist.sessionId, "provideUrl"))}>填写搜索页地址</button>
                <button type="button" className="ghost-btn" disabled={assistBusy} onClick={() => void runAssistAction(() => chooseSiteAssistEntry(siteAssist.sessionId, "unsupported"))}>此站没有搜索功能</button>
              </div>
            )}
            {siteAssist.state === "awaiting_search_url" && (
              <div className="assist-url-row">
                <input className="text-input" value={assistSearchURL} onChange={(event) => setAssistSearchURL(event.target.value)} placeholder="https://网站地址/search" />
                <button type="button" className="primary-btn" disabled={assistBusy || !assistSearchURL.trim()} onClick={() => void runAssistAction(async () => {
                  const status = await submitSiteAssistSearchURL(siteAssist.sessionId, assistSearchURL.trim());
                  setSites(await fetchSites());
                  return status;
                })}>保存并在此窗口打开</button>
              </div>
            )}
            {siteAssist.state === "login_waiting" && (
              <>
                <ol className="login-steps">
                  <li>请在已经打开的浏览器窗口中完成登录，程序会持续监控。</li>
                  <li>未登录成功时会继续等待，不会提前恢复下载。</li>
                  <li>登录成功后会自动返回原页面，重新搜索并验证资料。</li>
                </ol>
                <div className="preview-actions">
                  <button type="button" className="ghost-btn" disabled={assistBusy} onClick={() => void runAssistAction(() => beginSiteAssistLogin(siteAssist.sessionId))}>返回登录页</button>
                  <button type="button" className="primary-btn" disabled={assistBusy} onClick={() => void runAssistAction(() => confirmSiteAssistLogin(siteAssist.sessionId))}>我已完成登录</button>
                </div>
              </>
            )}
            {siteAssist.state === "failed" && (() => {
              const hints: Record<string, string> = {
                captcha: "检测到验证码或安全验证：请先在浏览器窗口里完成验证，再点「重新修复」（同一窗口，状态保留）。",
                no_search_box: "没找到搜索框：很可能被弹窗挡住，先在浏览器里关掉弹窗再点「重新修复」，或填写搜索页地址。",
                submit_blocked: "搜索没有返回结果：可能被弹窗、验证码或登录提示拦住，先在浏览器里处理再点「重新修复」。",
                no_results: "没搜到相关资料：尝试更换关键词，或在浏览器里关掉弹窗后点「重新修复」。",
                no_usable: "搜到了链接但前几条没有可下载内容：更换关键词，或确认此站是否需要登录。",
                exhausted: "已多次自动修复仍无真实下载：请更换关键词，或确认该站确实有可下载资料后再点开始。"
              };
              const hint = siteAssist.failureCode ? hints[siteAssist.failureCode] : "";
              return (
                <>
                  {hint && <p className="assist-message" style={{ color: "#b25a00" }}>{hint}</p>}
                  <p className="assist-message" style={{ fontSize: 12, opacity: 0.75 }}>关闭或失败后，监督程序会自动重新进入修复，直到成功或点击「放弃本站」。</p>
                  <div className="preview-actions assist-actions-column">
                    <button type="button" className="primary-btn" disabled={assistBusy} onClick={() => void runAssistAction(() => retrySiteAssist(siteAssist.sessionId))}>重新修复（同一浏览器窗口）</button>
                    <button type="button" className="ghost-btn" disabled={assistBusy} onClick={() => void runAssistAction(() => chooseSiteAssistEntry(siteAssist.sessionId, "provideUrl"))}>修改搜索页地址</button>
                    <button type="button" className="ghost-btn" disabled={assistBusy} onClick={() => void runAssistAction(() => beginSiteAssistLogin(siteAssist.sessionId))}>此站需要登录</button>
                    <button type="button" className="ghost-btn" onClick={() => closeSiteAssist(true)}>放弃本站</button>
                  </div>
                </>
              );
            })()}
            {siteAssist.state === "ready" && !siteAssist.scheduleId && (
              <div className="preview-actions"><button type="button" className="primary-btn" onClick={() => closeSiteAssist()}>完成</button></div>
            )}
            {siteAssist.state === "downloading" && <div className="assist-downloading">下载中…</div>}
            {siteAssist.state !== "downloading" && (
              <div className="preview-actions"><button type="button" className="ghost-btn" onClick={() => closeSiteAssist()}>取消修复</button></div>
            )}
            <div className="preview-actions">





            </div>
          </div>
        </div>
      )}

      {menu && (
        <div className="context-menu-mask" onClick={() => setMenu(null)} onContextMenu={(event) => event.preventDefault()}>
          <div
            className="context-menu"
            style={{ left: menu.x, top: menu.y }}
            onClick={(event) => event.stopPropagation()}
          >
            <button
              type="button"
              className="context-item"
              onClick={() => {
                const item = schedules.find((row) => row.id === menu.id);
                setMenu(null);
                if (item) {
                  openScheduleDialog(item);
                }
              }}
            >
              编辑
            </button>
            <button
              type="button"
              className="context-item danger"
              onClick={() => {
                setMenu(null);
                void removeScheduleById(menu.id);
              }}
            >
              删除
            </button>
          </div>
        </div>
      )}

      {draft && (
        <div className="dialog-mask" onClick={() => setDraft(null)}>
          <div className="dialog dialog-schedule" onClick={(event) => event.stopPropagation()} style={{ width: 560, maxWidth: "92vw" }}>
            <button type="button" className="dialog-close" aria-label="关闭" title="关闭" onClick={() => setDraft(null)}>
              ×
            </button>
            <div className="dialog-tabs" role="tablist" aria-label="定时搜索设置">
              <button
                type="button"
                role="tab"
                aria-selected={dialogTab === "schedule"}
                className={dialogTab === "schedule" ? "dialog-tab active" : "dialog-tab"}
                onClick={() => setDialogTab("schedule")}
              >
                定时设置
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={dialogTab === "sites"}
                className={dialogTab === "sites" ? "dialog-tab active" : "dialog-tab"}
                onClick={() => setDialogTab("sites")}
              >
                网页设置
              </button>
            </div>
            {dialogTab === "schedule" ? (
              <div className="dialog-pane">
                <span className="field-label" style={{ display: "block", marginBottom: 6 }}>定时内容</span>
                <label className="field-label">关键词</label>
                <input
                  className="input"
                  value={draft.keyword}
                  onChange={(event) => setDraft({ ...draft, keyword: event.target.value })}
                  placeholder="例如：短视频运营"
                />
                <label className="field-label" style={{ marginTop: 12 }}>执行频率</label>
                <div className="seg" style={{ marginTop: 4 }}>
                  {(["instant", "daily", "weekly", "monthly"] as ScheduleFreq[]).map((freq) => (
                    <button
                      type="button"
                      key={freq}
                      className={draft.freq === freq ? "seg-item active" : "seg-item"}
                      onClick={() => setDraft({ ...draft, freq })}
                    >
                      {FREQ_LABEL[freq]}
                    </button>
                  ))}
                </div>
                {draft.freq === "weekly" && (
                  <div className="multi-picker" style={{ marginTop: 10 }}>
                    {ALL_WEEKDAYS.map((value) => {
                      const active = draft.weekdays.includes(value);
                      return (
                        <button
                          type="button"
                          key={value}
                          className={active ? "chip active" : "chip"}
                          onClick={() =>
                            setDraft({
                              ...draft,
                              weekdays: active
                                ? draft.weekdays.filter((day) => day !== value)
                                : [...draft.weekdays, value].sort((a, b) => a - b)
                            })
                          }
                        >
                          {WEEKDAY_LABEL[value]}
                        </button>
                      );
                    })}
                  </div>
                )}
                {draft.freq === "monthly" && (
                  <div className="multi-picker" style={{ marginTop: 10 }}>
                    {ALL_DAYS.map((value) => {
                      const active = draft.days.includes(value);
                      return (
                        <button
                          type="button"
                          key={value}
                          className={active ? "chip active" : "chip"}
                          onClick={() =>
                            setDraft({
                              ...draft,
                              days: active
                                ? draft.days.filter((day) => day !== value)
                                : [...draft.days, value].sort((a, b) => a - b)
                            })
                          }
                        >
                          {value}
                        </button>
                      );
                    })}
                  </div>
                )}
                {draft.freq !== "instant" && (
                  <div style={{ marginTop: 10, maxWidth: 220 }}>
                    <label className="field-label">时间（小时）</label>
                    <select
                      className="input"
                      value={draft.hour}
                      onChange={(event) => setDraft({ ...draft, hour: Number(event.target.value) })}
                    >
                      {Array.from({ length: 24 }, (_, hour) => (
                        <option value={hour} key={hour}>
                          {String(hour).padStart(2, "0")}:00
                        </option>
                      ))}
                    </select>
                  </div>
                )}
                <label className="field-label" style={{ marginTop: 12 }}>资料选项</label>
                <div className="multi-picker" style={{ marginTop: 4 }}>
                  {TYPE_OPTIONS.map((type) => {
                    const active = draft.types.includes(type);
                    return (
                      <button
                        type="button"
                        key={type}
                        className={active ? "chip active" : "chip"}
                        data-tip={type === "全部" ? "表示把这个网站的所有资料都下载" : undefined}
                        onClick={() => setDraft({ ...draft, types: toggleType(draft.types, type) })}
                      >
                        {type}
                      </button>
                    );
                  })}
                </div>
                {enabledSites.length > 0 && (
                  <>
                    <label className="field-label" style={{ marginTop: 12 }}>网站选择</label>
                    <p className="caption" style={{ marginTop: 2 }}>
                      勾选哪些网站就只搜哪些；一个都不勾 = 不搜索任何资料。
                    </p>
                    <div className="multi-picker" style={{ marginTop: 4 }}>
                      {enabledSites.map((site) => {
                        const active = draft.siteIds.includes(site.id);
                        return (
                          <button
                            type="button"
                            key={site.id}
                            className={active ? "chip active" : "chip"}
                            onClick={() =>
                              setDraft({
                                ...draft,
                                siteIds: active
                                  ? draft.siteIds.filter((id) => id !== site.id)
                                  : [...draft.siteIds, site.id]
                              })
                            }
                          >
                            {site.name}
                          </button>
                        );
                      })}
                    </div>
                  </>
                )}
                <label className="field-label" style={{ marginTop: 12 }}>发布时间</label>
                <div className="seg" style={{ marginTop: 4 }}>
                  {PUBLISH_OPTIONS.map((option) => (
                    <button
                      type="button"
                      key={option.value}
                      className={draft.publishWithin === option.value ? "seg-item active" : "seg-item"}
                      onClick={() => setDraft({ ...draft, publishWithin: option.value })}
                    >
                      {option.label}
                    </button>
                  ))}
                </div>
                <div className="download-limit-head" style={{ marginTop: 12 }}>
                  <label className="field-label" style={{ margin: 0 }}>下载大小</label>
                  <span className="download-limit-value">{formatDownloadSize(draft.downloadLimitBytes)}</span>
                </div>
                <div className="download-limit" style={{ marginTop: 4 }}>
                  <input
                    className="download-limit-range"
                    type="range"
                    min={0}
                    max={DOWNLOAD_STEP * (DOWNLOAD_NODES.length - 1)}
                    step={1}
                    value={Math.round(bytesToSliderPos(draft.downloadLimitBytes) * DOWNLOAD_STEP)}
                    onChange={(event) => {
                      const rawPos = Number(event.target.value) / DOWNLOAD_STEP;
                      const nearest = Math.round(rawPos);
                      // 靠近节点时自动吸附，避免停在 0.98 这类位置。
                      const snapped = Math.abs(rawPos - nearest) <= DOWNLOAD_SNAP_RATIO ? nearest : rawPos;
                      setDraft({ ...draft, downloadLimitBytes: sliderPosToBytes(snapped) });
                    }}
                  />
                  <div className="download-limit-scale">
                    {DOWNLOAD_NODE_LABELS.map((label, index) => (
                      <span
                        key={label}
                        className={
                          Math.abs(bytesToSliderPos(draft.downloadLimitBytes) - index) < 0.02 ? "active" : undefined
                        }
                      >
                        {label}
                      </span>
                    ))}
                  </div>
                </div>
                <p className="caption" style={{ marginTop: 2 }}>
                  每轮最多下载这么多；0G 表示不下载任何资料。
                </p>
              </div>
            ) : (
              <div className="dialog-pane">
                <div className="lib-sites" style={{ marginTop: 0 }}>
                  <div className="row" style={{ justifyContent: "space-between", marginBottom: 6 }}>
                    <span className="field-label" style={{ margin: 0 }}>网站设置</span>
                    <span className="caption">外网搜集只在这些网站里找</span>
                  </div>
                  {sites.map((site) => {
                    const check = siteChecks[site.id];
                    const verifying = verifyingSiteId === site.id;
                    return (
                      <div className="lib-site-row" key={site.id}>
                        <input
                          className="input"
                          value={site.name}
                          onChange={(event) => patchSite(site.id, { name: event.target.value })}
                        />
                        <input
                          className="input"
                          value={site.domain}
                          title={site.domain}
                          onChange={(event) => patchSite(site.id, { domain: event.target.value })}
                        />
                        <span
                          className={`site-login-badge${site.hasCookie ? "" : " hidden"}`}
                          title={site.hasCookie ? "已保存该站登录信息" : ""}
                        >
                          已登录
                        </span>
                        <button type="button" className="ghost-btn" onClick={() => void persistSite(site)}>
                          保存
                        </button>
                        <button
                          type="button"
                          className={`verify-btn${check?.ok ? " ok" : check ? " fail" : ""}`}
                          disabled={verifying || Boolean(verifyingSiteId)}
                          title={check?.message ?? "验证该网站能否正常搜集资料"}
                          onClick={() => void verifySiteRow(site)}
                        >
                          {verifying ? "验证中…" : "验证"}
                        </button>
                        <button type="button" className="ghost-btn" onClick={() => void removeSite(site.id)}>
                          删除
                        </button>
                      </div>
                    );
                  })}
                  <div className="lib-site-row">
                    <input
                      className="input"
                      placeholder="网站名称"
                      value={siteDraft.name}
                      onChange={(event) => setSiteDraft({ ...siteDraft, name: event.target.value })}
                    />
                    <input
                      className="input"
                      placeholder="网址，如 https://example.com/search"
                      value={siteDraft.domain}
                      onChange={(event) => setSiteDraft({ ...siteDraft, domain: event.target.value })}
                    />
                    <button type="button" className="secondary-btn" onClick={() => void addSite()}>
                      增加
                    </button>
                  </div>
                </div>
              </div>
            )}
            <div className="dialog-actions">
              {draft.id && (
                <button type="button" className="ghost-btn" onClick={() => void removeSchedule()}>
                  删除
                </button>
              )}
              <button type="button" className="secondary-btn" onClick={() => setDraft(null)}>
                取消
              </button>
              <button type="button" className="primary-btn" onClick={() => void submitSchedule()}>
                {draft.id ? "保存" : "确认"}
              </button>
            </div>
          </div>
        </div>
      )}

    </div>
  );
}
