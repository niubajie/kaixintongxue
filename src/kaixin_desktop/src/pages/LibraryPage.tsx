import { useCallback, useEffect, useState } from "react";
import {
  approvePendingDoc,
  collectFromWeb,
  deleteSchedule,
  deleteSite,
  fetchKBStatus,
  fetchLibraryScan,
  fetchLibraryTaxonomy,
  fetchPendingDocs,
  fetchSchedules,
  fetchSites,
  rejectPendingDoc,
  saveSchedule,
  saveSite,
  searchKnowledgeBase,
  openLibraryFile,
  type KBDocument,
  type KBSearchHit,
  type LibraryItem,
  type LibraryScan,
  type LibraryTaxonomy,
  type PendingDoc,
  type ScheduleFreq,
  type SearchSchedule,
  type SearchSite
} from "../deployApi";

interface Props {
  onNotice: (message: string) => void;
}

const ALL = "全部资料";

type SearchMode = "local" | "web";

const FREQ_LABEL: Record<ScheduleFreq, string> = {
  daily: "每天",
  weekly: "每周",
  monthly: "每月"
};

const WEEKDAY_LABEL = ["", "周一", "周二", "周三", "周四", "周五", "周六", "周日"];

interface ScheduleDraft {
  id: string;
  keyword: string;
  category: string;
  subCategory: string;
  freq: ScheduleFreq;
  hour: number;
  weekday: number;
  day: number;
}

function emptyDraft(category: string, sub: string): ScheduleDraft {
  return {
    id: "",
    keyword: "",
    category: category === ALL ? "未分类" : category,
    subCategory: sub,
    freq: "daily",
    hour: 9,
    weekday: 1,
    day: 1
  };
}

export function LibraryPage({ onNotice }: Props) {
  const [state, setState] = useState<"loading" | "ready" | "error">("loading");
  const [scan, setScan] = useState<LibraryScan | null>(null);
  const [taxonomy, setTaxonomy] = useState<LibraryTaxonomy | null>(null);
  const [error, setError] = useState("");
  const [pending, setPending] = useState<PendingDoc[]>([]);
  const [schedules, setSchedules] = useState<SearchSchedule[]>([]);

  const [mode, setMode] = useState<SearchMode>("local");
  const [keyword, setKeyword] = useState("");
  const [searching, setSearching] = useState(false);
  const [hits, setHits] = useState<KBSearchHit[] | null>(null);

  const [category, setCategory] = useState(ALL);
  const [sub, setSub] = useState("");
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  const [kb, setKb] = useState<Record<string, KBDocument>>({});

  const [draft, setDraft] = useState<ScheduleDraft | null>(null);

  const [sites, setSites] = useState<SearchSite[]>([]);
  const [siteDraft, setSiteDraft] = useState({ name: "", domain: "" });

  // 网站设置：行内编辑后点「保存」写回，新增走底部的一行输入。
  const patchSite = (id: string, patch: Partial<SearchSite>) => {
    setSites((current) => current.map((item) => (item.id === id ? { ...item, ...patch } : item)));
  };

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
    setError("");
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
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "扫描资料目录失败");
      setScan(null);
      setState("error");
    }
    void fetchPendingDocs().then(setPending).catch(() => setPending([]));
    void fetchSchedules().then(setSchedules).catch(() => setSchedules([]));
    void fetchSites().then(setSites).catch(() => setSites([]));
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  // 定时任务由 sidecar 在后台执行，页面定期刷新待审核与任务状态。
  useEffect(() => {
    const timer = window.setInterval(() => {
      void fetchPendingDocs().then(setPending).catch(() => undefined);
      void fetchSchedules().then(setSchedules).catch(() => undefined);
    }, 60000);
    return () => window.clearInterval(timer);
  }, []);

  const indexedCount = Object.values(kb).filter((doc) => doc.status === "indexed").length;
  const categories = taxonomy?.categories ?? [];
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

  const openSub = (name: string, subName: string) => {
    setCategory(name);
    setSub(subName);
  };

  const submitSearch = async (nextMode: SearchMode = mode) => {
    const kw = keyword.trim();
    if (nextMode === "web") {
      if (!kw) {
        onNotice("请先输入要搜集的关键词。");
        return;
      }
      setSearching(true);
      try {
        const result = await collectFromWeb({
          keyword: kw,
          category: category === ALL ? "未分类" : category,
          subCategory: sub,
          limit: 5
        });
        onNotice(
          result.provider === "demo"
            ? `已用演示源搜集 ${result.count} 条，进待审核区（配置 BING_SEARCH_KEY 后为真实结果）。`
            : `已搜集 ${result.count} 条，进待审核区。`
        );
        setHits(null);
        await load();
      } catch (err: unknown) {
        onNotice(err instanceof Error ? err.message : "外网搜集失败");
      } finally {
        setSearching(false);
      }
      return;
    }

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

  const handleApprove = async (doc: PendingDoc) => {
    try {
      const result = await approvePendingDoc(doc.id);
      onNotice(`已入库：${result.relPath}`);
      await load();
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "入库失败");
    }
  };

  const handleReject = async (doc: PendingDoc) => {
    try {
      await rejectPendingDoc(doc.id);
      onNotice(`已忽略：${doc.title}`);
      setPending((current) => current.filter((item) => item.id !== doc.id));
      if (taxonomy) {
        setTaxonomy({ ...taxonomy, pending: Math.max(0, taxonomy.pending - 1) });
      }
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "忽略失败");
    }
  };

  const openScheduleDialog = (item?: SearchSchedule) => {
    if (item) {
      setDraft({
        id: item.id,
        keyword: item.keyword,
        category: item.category,
        subCategory: item.subCategory,
        freq: item.freq,
        hour: item.hour,
        weekday: item.weekday,
        day: item.day
      });
      return;
    }
    setDraft(emptyDraft(category, sub));
  };

  const submitSchedule = async () => {
    if (!draft) {
      return;
    }
    if (!draft.keyword.trim()) {
      onNotice("请输入定时搜索的关键词。");
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
        enabled: true,
        lastRunAt: "",
        createdAt: ""
      });
      onNotice(draft.id ? "定时任务已更新。" : "定时任务已创建，应用运行时自动执行。");
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
            onClick={() => openCategory(ALL)}
            style={category === ALL ? { background: "var(--accent-light)", color: "var(--accent)" } : undefined}
          >
            <span>{ALL}</span>
            <span className="caption">{scan?.total ?? 0} 份</span>
          </button>
          {categories.map((cat) => (
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

      {state === "error" && (
        <div className="card" style={{ marginTop: 16 }}>
          <div style={{ fontSize: 13, fontWeight: 600 }}>扫描失败</div>
          <p className="list-card-detail" style={{ marginTop: 6 }}>{error}</p>
          <p className="caption" style={{ marginTop: 6 }}>
            请确认本地服务已启动；资料目录不存在时会自动按空目录处理。
          </p>
        </div>
      )}

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
              该分类下暂无文件。把文件放进对应文件夹，或用「外网搜集」确认入库。
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

      <div className={`lib-search ${pending.length > 0 ? "is-top" : "is-center"}`}>
        <div className="lib-search-bar">
          <input
            className="input"
            disabled={searching}
            placeholder={
              mode === "local"
                ? "输入关键词，检索已加入知识库的资料"
                : "输入关键词，立即搜集到待审核区"
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
                if (keyword.trim()) {
                  void submitSearch("web");
                }
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
            {schedules.map((item) => (
              <div
                key={item.id}
                className="card schedule-card"
                onDoubleClick={() => openScheduleDialog(item)}
                title="双击修改"
              >
                <div className="schedule-title">{item.keyword}</div>
                <div className="schedule-meta">
                  {FREQ_LABEL[item.freq]}
                  {item.freq === "weekly" ? ` ${WEEKDAY_LABEL[item.weekday]}` : ""}
                  {item.freq === "monthly" ? ` ${item.day} 日` : ""}
                  {` ${String(item.hour).padStart(2, "0")}:00`}
                </div>
                <div className="schedule-meta">
                  {item.category}
                  {item.subCategory ? ` / ${item.subCategory}` : ""} · 双击修改
                </div>
              </div>
            ))}
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
                没有命中。外网搜集的资料确认入库后，即可被本地检索。
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
              <div className="lib-pending-row" key={doc.id}>
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
                  </p>
                </div>
                <div className="lib-pending-actions">
                  <button type="button" className="primary-btn" onClick={() => void handleApprove(doc)}>
                    确认入库
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

      {draft && (
        <div className="dialog-mask" onClick={() => setDraft(null)}>
          <div className="dialog" onClick={(event) => event.stopPropagation()} style={{ width: 720, maxWidth: "92vw" }}>
            <h2 className="dialog-title">{draft.id ? "修改定时搜索" : "新建定时搜索"}</h2>
            <div className="dialog-cols">
              <div className="dialog-col">
                <span className="field-label" style={{ display: "block", marginBottom: 6 }}>定时内容</span>
                <label className="field-label">关键词</label>
                <input
                  className="input"
                  value={draft.keyword}
                  onChange={(event) => setDraft({ ...draft, keyword: event.target.value })}
                  placeholder="例如：短视频运营"
                />
                <div className="row" style={{ gap: 10, marginTop: 10 }}>
                  <div style={{ flex: 1 }}>
                    <label className="field-label">归入大类</label>
                    <select
                      className="input"
                      value={draft.category}
                      onChange={(event) => setDraft({ ...draft, category: event.target.value })}
                    >
                      <option value="工作">工作</option>
                      <option value="生活">生活</option>
                      <option value="学习">学习</option>
                      <option value="爱好">爱好</option>
                      <option value="未分类">未分类</option>
                    </select>
                  </div>
                  <div style={{ flex: 1 }}>
                    <label className="field-label">子类（可空）</label>
                    <input
                      className="input"
                      value={draft.subCategory}
                      onChange={(event) => setDraft({ ...draft, subCategory: event.target.value })}
                      placeholder="如：短视频"
                    />
                  </div>
                </div>
                <label className="field-label" style={{ marginTop: 10 }}>执行频率</label>
                <div className="seg" style={{ marginTop: 4 }}>
                  {(["daily", "weekly", "monthly"] as ScheduleFreq[]).map((freq) => (
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
                <div className="row" style={{ gap: 10, marginTop: 10 }}>
                  <div style={{ flex: 1 }}>
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
                  {draft.freq === "weekly" && (
                    <div style={{ flex: 1 }}>
                      <label className="field-label">星期</label>
                      <select
                        className="input"
                        value={draft.weekday}
                        onChange={(event) => setDraft({ ...draft, weekday: Number(event.target.value) })}
                      >
                        {WEEKDAY_LABEL.slice(1).map((label, index) => (
                          <option value={index + 1} key={label}>
                            {label}
                          </option>
                        ))}
                      </select>
                    </div>
                  )}
                  {draft.freq === "monthly" && (
                    <div style={{ flex: 1 }}>
                      <label className="field-label">日期</label>
                      <select
                        className="input"
                        value={draft.day}
                        onChange={(event) => setDraft({ ...draft, day: Number(event.target.value) })}
                      >
                        {Array.from({ length: 28 }, (_, index) => (
                          <option value={index + 1} key={index + 1}>
                            {index + 1} 日
                          </option>
                        ))}
                      </select>
                    </div>
                  )}
                </div>
                <p className="caption" style={{ marginTop: 8 }}>
                  定时任务在应用运行期间由本地服务执行；应用关闭时暂停，下次启动会补跑当天未完成的任务。
                </p>
              </div>
              <div className="dialog-col">
                <div className="lib-sites" style={{ marginTop: 0 }}>
                  <div className="row" style={{ justifyContent: "space-between", marginBottom: 6 }}>
                    <span className="field-label" style={{ margin: 0 }}>网站设置</span>
                    <span className="caption">外网搜集只在这些网站里找</span>
                  </div>
                  {sites.map((site) => (
                    <div className="lib-site-row" key={site.id}>
                      <input
                        className="input"
                        value={site.name}
                        onChange={(event) => patchSite(site.id, { name: event.target.value })}
                      />
                      <input
                        className="input"
                        value={site.domain}
                        onChange={(event) => patchSite(site.id, { domain: event.target.value })}
                      />
                      <button type="button" className="ghost-btn" onClick={() => void persistSite(site)}>
                        保存
                      </button>
                      <button type="button" className="ghost-btn" onClick={() => void removeSite(site.id)}>
                        删除
                      </button>
                    </div>
                  ))}
                  <div className="lib-site-row">
                    <input
                      className="input"
                      placeholder="网站名称"
                      value={siteDraft.name}
                      onChange={(event) => setSiteDraft({ ...siteDraft, name: event.target.value })}
                    />
                    <input
                      className="input"
                      placeholder="域名，如 zhihu.com"
                      value={siteDraft.domain}
                      onChange={(event) => setSiteDraft({ ...siteDraft, domain: event.target.value })}
                    />
                    <button type="button" className="secondary-btn" onClick={() => void addSite()}>
                      增加
                    </button>
                  </div>
                </div>
              </div>
            </div>
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
                {draft.id ? "保存" : "创建"}
              </button>
            </div>
          </div>
        </div>
      )}

    </div>
  );
}
