import { useState } from "react";
import {
  createId,
  formatDate,
  formatSize,
  SAMPLE_APPS,
  usePersistentState,
  type AppKind,
  type AppModule,
  type DiaryEntry,
  type MaterialItem
} from "../assistant";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { TextWithVoice } from "../components/TextWithVoice";

interface Props {
  onNotice: (message: string) => void;
}

type Tab = "diary" | "material" | "dashboard";

const KIND_LABEL: Record<AppKind, string> = {
  repair: "电脑维修",
  douyin: "抖音短视频",
  generic: "通用"
};

// ---------- 纯函数 ----------

function parseMaterialSize(text: string): number {
  const match = /([\d.]+)\s*(KB|MB|GB|B)/i.exec(text ?? "");
  if (!match) {
    return 0;
  }
  const value = Number(match[1]);
  const unit = match[2].toUpperCase();
  const factor = unit === "GB" ? 1024 ** 3 : unit === "MB" ? 1024 ** 2 : unit === "KB" ? 1024 : 1;
  return Math.round(value * factor);
}

function formatBytes(bytes: number): string {
  if (bytes >= 1024 ** 3) {
    return `${(bytes / 1024 ** 3).toFixed(1)} GB`;
  }
  if (bytes >= 1024 ** 2) {
    return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
  }
  if (bytes >= 1024) {
    return `${Math.round(bytes / 1024)} KB`;
  }
  return `${bytes} B`;
}

function materialExt(name: string): string {
  const index = name.lastIndexOf(".");
  return index > 0 ? name.slice(index + 1).toUpperCase() : "其他";
}

/** 从文件名提取关键词区，供资料检索使用 */
function extractKeywords(name: string): string[] {
  const base = name.replace(/\.[^.]+$/, "");
  const tokens = base
    .split(/[\s_\-—·.,，、。()（）[\]【】]+/)
    .map((token) => token.trim())
    .filter((token) => token.length >= 2);
  return [...new Set(tokens)].slice(0, 5);
}

function dayKey(iso: string): string {
  const date = new Date(iso.replace(" ", "T"));
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

function daysAgo(iso: string): number | null {
  const date = new Date(iso.replace(" ", "T"));
  if (Number.isNaN(date.getTime())) {
    return null;
  }
  const diff = Date.now() - date.getTime();
  return Math.floor(diff / (24 * 60 * 60 * 1000));
}

// ---------- 树操作 ----------

function walk(nodes: AppModule[], visit: (node: AppModule, depth: number) => void, depth = 0): void {
  for (const node of nodes) {
    visit(node, depth);
    if (node.children && node.children.length > 0) {
      walk(node.children, visit, depth + 1);
    }
  }
}

function findNode(nodes: AppModule[], id: string): AppModule | undefined {
  for (const node of nodes) {
    if (node.id === id) {
      return node;
    }
    const hit = node.children ? findNode(node.children, id) : undefined;
    if (hit) {
      return hit;
    }
  }
  return undefined;
}

function mapNode(nodes: AppModule[], id: string, update: (node: AppModule) => AppModule): AppModule[] {
  return nodes.map((node) => {
    if (node.id === id) {
      return update(node);
    }
    if (node.children && node.children.length > 0) {
      return { ...node, children: mapNode(node.children, id, update) };
    }
    return node;
  });
}

function removeNode(nodes: AppModule[], id: string): AppModule[] {
  return nodes
    .filter((node) => node.id !== id)
    .map((node) => (node.children ? { ...node, children: removeNode(node.children, id) } : node));
}

/** 统计包含子应用在内的总数 */
function rollup(node: AppModule): { diaries: number; materials: number; pending: number } {
  let diaries = node.diaries.length;
  let materials = node.materials.length;
  let pending = node.materials.filter((item) => item.pending).length;
  for (const child of node.children ?? []) {
    const sub = rollup(child);
    diaries += sub.diaries;
    materials += sub.materials;
    pending += sub.pending;
  }
  return { diaries, materials, pending };
}

// ---------- 页面 ----------

export function ApplicationPage({ onNotice }: Props) {
  const [apps, setApps] = usePersistentState<AppModule[]>("kaixin.apps.v1", SAMPLE_APPS);
  const [selectedId, setSelectedId] = useState(apps[0]?.id ?? "");
  const [tab, setTab] = useState<Tab>("diary");

  const [newOpen, setNewOpen] = useState(false);
  const [newTitle, setNewTitle] = useState("");
  const [newKind, setNewKind] = useState<AppKind>("generic");
  const [newParent, setNewParent] = useState<string | null>(null);

  const [renameOpen, setRenameOpen] = useState(false);
  const [renameTitle, setRenameTitle] = useState("");

  const [diaryOpen, setDiaryOpen] = useState(false);
  const [diaryEditingId, setDiaryEditingId] = useState<string | null>(null);
  const [diaryTitle, setDiaryTitle] = useState("");
  const [diaryContent, setDiaryContent] = useState("");

  const [materialKeyword, setMaterialKeyword] = useState("");
  const [dashboardFull, setDashboardFull] = useState(false);
  const [keywordEditing, setKeywordEditing] = useState<{ id: string; value: string } | null>(null);

  const [confirm, setConfirm] = useState<{ kind: "app" | "diary" | "material"; id: string; name: string } | null>(
    null
  );

  const selected = findNode(apps, selectedId) ?? apps[0];
  const isSubApp = (() => {
    let found = false;
    walk(apps, (node) => {
      if (node.id === selected?.id && apps.every((root) => root.id !== node.id)) {
        found = true;
      }
    });
    return found;
  })();

  const updateSelected = (updater: (app: AppModule) => AppModule) => {
    if (!selected) {
      return;
    }
    setApps((current) => mapNode(current, selected.id, updater));
  };

  // ---------- 应用与子应用 ----------

  const openCreate = (parentId: string | null) => {
    setNewParent(parentId);
    setNewTitle("");
    setNewKind(parentId ? findNode(apps, parentId)?.kind ?? "generic" : "generic");
    setNewOpen(true);
  };

  const createApp = () => {
    if (!newTitle.trim()) {
      onNotice("请填写应用标题名称。");
      return;
    }
    const next: AppModule = {
      id: createId("app"),
      title: newTitle.trim(),
      kind: newKind,
      diaries: [],
      materials: [],
      children: []
    };
    if (newParent) {
      setApps((current) =>
        mapNode(current, newParent, (node) => ({ ...node, children: [...(node.children ?? []), next] }))
      );
      onNotice(`已新建子应用「${next.title}」，内容与主应用一致。`);
    } else {
      setApps((current) => [...current, next]);
      onNotice(`已新建应用「${next.title}」，包含笔记与资料库两个板块。`);
    }
    setSelectedId(next.id);
    setNewOpen(false);
    setTab("diary");
  };

  const renameApp = () => {
    if (!selected) {
      return;
    }
    if (!renameTitle.trim()) {
      onNotice("名称不能为空。");
      return;
    }
    updateSelected((app) => ({ ...app, title: renameTitle.trim() }));
    setRenameOpen(false);
    onNotice("已重命名。");
  };

  // ---------- 笔记 ----------

  const openNewDiary = () => {
    setDiaryEditingId(null);
    setDiaryTitle("");
    setDiaryContent("");
    setDiaryOpen(true);
  };

  const openDiary = (entry: DiaryEntry) => {
    setDiaryEditingId(entry.id);
    setDiaryTitle(entry.title);
    setDiaryContent(entry.content);
    setDiaryOpen(true);
  };

  const saveDiary = () => {
    if (!selected) {
      return;
    }
    if (!diaryTitle.trim() || !diaryContent.trim()) {
      onNotice("请填写笔记标题与内容。");
      return;
    }
    if (diaryEditingId) {
      updateSelected((app) => ({
        ...app,
        diaries: app.diaries.map((entry) =>
          entry.id === diaryEditingId
            ? { ...entry, title: diaryTitle.trim(), content: diaryContent.trim() }
            : entry
        )
      }));
      onNotice("笔记已保存修改。");
    } else {
      const entry: DiaryEntry = {
        id: createId("diary"),
        title: diaryTitle.trim(),
        content: diaryContent.trim(),
        createdAt: formatDate()
      };
      updateSelected((app) => ({ ...app, diaries: [entry, ...app.diaries] }));
      onNotice("已新增笔记。");
    }
    setDiaryOpen(false);
  };

  /** 把笔记内容提炼成一份待确认资料 */
  const extractDiary = (entry: DiaryEntry) => {
    if (!selected) {
      return;
    }
    const summary = entry.content.replace(/\s+/g, " ").slice(0, 60);
    const material: MaterialItem = {
      id: createId("mat"),
      name: `${entry.title}（笔记提炼）.md`,
      size: formatSize(new Blob([entry.content]).size),
      uploadedAt: formatDate(),
      pending: true,
      keywords: extractKeywords(entry.title)
    };
    updateSelected((app) => ({ ...app, materials: [material, ...app.materials] }));
    onNotice(`已从笔记提炼资料：${material.name}（待确认，摘要：${summary}…）`);
    setTab("material");
  };

  // ---------- 资料 ----------

  const addMaterials = (files: FileList | null) => {
    if (!selected || !files || files.length === 0) {
      return;
    }
    const items: MaterialItem[] = Array.from(files).map((file) => ({
      id: createId("mat"),
      name: file.name,
      size: formatSize(file.size),
      uploadedAt: formatDate(),
      pending: true,
      keywords: extractKeywords(file.name)
    }));
    updateSelected((app) => ({ ...app, materials: [...items, ...app.materials] }));
    onNotice(`已上传 ${items.length} 份资料，均为待确认状态。`);
  };

  const confirmMaterial = (id: string) => {
    updateSelected((app) => ({
      ...app,
      materials: app.materials.map((item) => (item.id === id ? { ...item, pending: false } : item))
    }));
    onNotice("资料已确认。");
  };

  const saveKeywords = (id: string, value: string) => {
    const keywords = value
      .split(/[,，\s]+/)
      .map((item) => item.trim())
      .filter(Boolean);
    updateSelected((app) => ({
      ...app,
      materials: app.materials.map((item) => (item.id === id ? { ...item, keywords } : item))
    }));
    setKeywordEditing(null);
    onNotice("关键词已更新。");
  };

  const confirmRemove = () => {
    if (!confirm) {
      return;
    }
    if (confirm.kind === "app") {
      setApps((current) => removeNode(current, confirm.id));
      setConfirm(null);
      onNotice(`已删除「${confirm.name}」及其子应用、笔记与资料。`);
      return;
    }
    updateSelected((app) =>
      confirm.kind === "diary"
        ? { ...app, diaries: app.diaries.filter((entry) => entry.id !== confirm.id) }
        : { ...app, materials: app.materials.filter((item) => item.id !== confirm.id) }
    );
    setConfirm(null);
    onNotice("已删除。");
  };

  // ---------- 数据窗 ----------

  const dashboard = (() => {
    const diaries = selected?.diaries ?? [];
    const materials = selected?.materials ?? [];
    const kind: AppKind = selected?.kind ?? "generic";
    const totalBytes = materials.reduce((sum, item) => sum + parseMaterialSize(item.size), 0);

    const thisMonth = materials.filter((item) => daysAgo(item.uploadedAt) !== null && (daysAgo(item.uploadedAt) as number) <= 30).length;
    const thisWeek = materials.filter((item) => daysAgo(item.uploadedAt) !== null && (daysAgo(item.uploadedAt) as number) <= 7).length;
    const today = materials.filter((item) => dayKey(item.uploadedAt) === dayKey(formatDate())).length;
    const pendingCount = materials.filter((item) => item.pending).length;

    // 抖音类：以「视频」资料作为发布记录
    const videos = materials.filter((item) => materialExt(item.name) === "MP4" || materialExt(item.name) === "MOV");

    const byExt = new Map<string, { count: number; bytes: number }>();
    for (const item of materials) {
      const ext = materialExt(item.name);
      const current = byExt.get(ext) ?? { count: 0, bytes: 0 };
      current.count += 1;
      current.bytes += parseMaterialSize(item.size);
      byExt.set(ext, current);
    }
    const maxBytes = Math.max(1, ...[...byExt.values()].map((item) => item.bytes));
    const types = [...byExt.entries()]
      .map(([ext, value]) => ({
        ext,
        count: value.count,
        sizeText: formatBytes(value.bytes),
        percent: Math.max(4, Math.round((value.bytes / maxBytes) * 100))
      }))
      .sort((a, b) => b.count - a.count);

    const days: { key: string; label: string; count: number; height: number }[] = [];
    for (let offset = 6; offset >= 0; offset -= 1) {
      const date = new Date();
      date.setDate(date.getDate() - offset);
      const pad = (n: number) => String(n).padStart(2, "0");
      const key = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
      days.push({ key, label: `${date.getMonth() + 1}/${date.getDate()}`, count: 0, height: 4 });
    }
    for (const entry of diaries) {
      const hit = days.find((day) => day.key === dayKey(entry.createdAt));
      if (hit) {
        hit.count += 1;
      }
    }
    const maxCount = Math.max(1, ...days.map((day) => day.count));
    for (const day of days) {
      day.height = day.count === 0 ? 4 : Math.round((day.count / maxCount) * 60) + 8;
    }

    const sorted = [...diaries].sort((a, b) => (a.createdAt < b.createdAt ? 1 : -1));
    const lastUpdated = sorted.length > 0 ? sorted[0].createdAt : "暂无笔记";

    return {
      kind,
      totalBytes,
      thisMonth,
      thisWeek,
      today,
      pendingCount,
      videos,
      types,
      week: days,
      lastUpdated
    };
  })();

  const counts = selected ? rollup(selected) : { diaries: 0, materials: 0, pending: 0 };

  const filteredMaterials = (selected?.materials ?? []).filter((item) => {
    const keyword = materialKeyword.trim().toLowerCase();
    if (!keyword) {
      return true;
    }
    return (
      item.name.toLowerCase().includes(keyword) ||
      (item.keywords ?? []).some((tag) => tag.toLowerCase().includes(keyword))
    );
  });

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="page-title">应用助手</h1>
          <p className="page-subtitle">每个应用可建子应用，子应用内容与主应用一致；资料保存在本机</p>
        </div>
        <button type="button" className="primary-btn" onClick={() => openCreate(null)}>
          + 新建应用
        </button>
      </div>

      <div className="assistant-layout">
        <div className="card" style={{ padding: 12 }}>
          <div className="row" style={{ justifyContent: "space-between", padding: "0 4px 8px" }}>
            <span className="section-title">应用列表</span>
            <span className="tag">{apps.length} 个</span>
          </div>
          {apps.map((app) => (
            <div key={app.id}>
              <button
                type="button"
                className="list-card"
                style={selected?.id === app.id ? { borderColor: "var(--accent)" } : undefined}
                onClick={() => {
                  setSelectedId(app.id);
                  setTab("diary");
                }}
              >
                <div className="list-card-head">
                  <div>
                    <div className="list-card-title">{app.title}</div>
                    <p className="list-card-detail">
                      {KIND_LABEL[app.kind ?? "generic"]} · 笔记 {rollup(app).diaries} · 资料{" "}
                      {rollup(app).materials}
                      {rollup(app).pending > 0 ? ` · 待确认 ${rollup(app).pending}` : ""}
                    </p>
                  </div>
                </div>
              </button>
              {(app.children ?? []).map((child) => (
                <button
                  key={child.id}
                  type="button"
                  className="list-card"
                  style={{
                    marginLeft: 16,
                    ...(selected?.id === child.id ? { borderColor: "var(--accent)" } : {})
                  }}
                  onClick={() => {
                    setSelectedId(child.id);
                    setTab("diary");
                  }}
                >
                  <div className="list-card-head">
                    <div>
                      <div className="list-card-title">└ {child.title}</div>
                      <p className="list-card-detail">
                        子应用 · 笔记 {child.diaries.length} · 资料 {child.materials.length}
                      </p>
                    </div>
                  </div>
                </button>
              ))}
            </div>
          ))}
          {apps.length === 0 && <p className="caption" style={{ padding: 8 }}>还没有应用，点右上角新建。</p>}
        </div>

        <div className="card">
          {selected ? (
            <>
              <div className="detail-head">
                <div>
                  <h2 className="detail-title">{selected.title}</h2>
                  <div className="row" style={{ gap: 6, marginTop: 6 }}>
                    <span className="tag accent">{isSubApp ? "子应用" : "应用"}</span>
                    <span className="tag">{KIND_LABEL[selected.kind ?? "generic"]}</span>
                    <span className="tag">笔记 {counts.diaries}</span>
                    <span className="tag">资料 {counts.materials}</span>
                    {counts.pending > 0 && <span className="tag accent">待确认 {counts.pending}</span>}
                  </div>
                </div>
                <div className="row" style={{ gap: 6 }}>
                  {!isSubApp && (
                    <button type="button" className="secondary-btn" onClick={() => openCreate(selected.id)}>
                      + 新建子应用
                    </button>
                  )}
                  <button
                    type="button"
                    className="secondary-btn"
                    onClick={() => {
                      setRenameTitle(selected.title);
                      setRenameOpen(true);
                    }}
                  >
                    重命名
                  </button>
                  <button
                    type="button"
                    className="danger-btn"
                    onClick={() => setConfirm({ kind: "app", id: selected.id, name: selected.title })}
                  >
                    删除
                  </button>
                </div>
              </div>

              <div className="tabs">
                <button
                  type="button"
                  className={`tab${tab === "diary" ? " active" : ""}`}
                  onClick={() => setTab("diary")}
                >
                  笔记
                </button>
                <button
                  type="button"
                  className={`tab${tab === "material" ? " active" : ""}`}
                  onClick={() => setTab("material")}
                >
                  资料库
                </button>
                <button
                  type="button"
                  className={`tab${tab === "dashboard" ? " active" : ""}`}
                  onClick={() => setTab("dashboard")}
                >
                  数据窗
                </button>
              </div>

              {tab === "diary" && (
                <div className="tab-body">
                  <div className="row" style={{ justifyContent: "space-between" }}>
                    <h3 className="section-title">历史笔记</h3>
                    <button type="button" className="primary-btn" onClick={openNewDiary}>
                      + 新增笔记
                    </button>
                  </div>
                  <div className="list" style={{ marginTop: 12 }}>
                    {selected.diaries.map((entry) => (
                      <div className="list-card" key={entry.id}>
                        <div className="list-card-head">
                          <div onClick={() => openDiary(entry)} style={{ cursor: "pointer", flex: 1 }}>
                            <div className="list-card-title">{entry.title}</div>
                            <p className="list-card-detail">
                              {entry.createdAt} · {entry.content.slice(0, 40)}
                            </p>
                          </div>
                          <div className="row" style={{ gap: 6 }}>
                            <button type="button" className="ghost-btn" onClick={() => extractDiary(entry)}>
                              提炼到资料
                            </button>
                            <button
                              type="button"
                              className="ghost-btn"
                              onClick={() => setConfirm({ kind: "diary", id: entry.id, name: entry.title })}
                            >
                              删除
                            </button>
                          </div>
                        </div>
                      </div>
                    ))}
                    {selected.diaries.length === 0 && (
                      <p className="caption">还没有笔记，点「+ 新增笔记」开始记录。</p>
                    )}
                  </div>
                </div>
              )}

              {tab === "material" && (
                <div className="tab-body">
                  <div className="row" style={{ justifyContent: "space-between", flexWrap: "wrap", gap: 8 }}>
                    <h3 className="section-title">资料库</h3>
                    <label className="secondary-btn" style={{ cursor: "pointer" }}>
                      上传资料
                      <input
                        type="file"
                        multiple
                        hidden
                        onChange={(event) => {
                          addMaterials(event.target.files);
                          event.target.value = "";
                        }}
                      />
                    </label>
                  </div>
                  <input
                    className="input"
                    style={{ marginTop: 10 }}
                    placeholder="按文件名或关键词搜索"
                    value={materialKeyword}
                    onChange={(event) => setMaterialKeyword(event.target.value)}
                  />
                  <div className="list" style={{ marginTop: 12 }}>
                    {filteredMaterials.map((item) => (
                      <div className="list-card" key={item.id}>
                        <div className="list-card-head">
                          <div style={{ flex: 1 }}>
                            <div className="list-card-title">
                              {item.name}
                              {item.pending && <span className="tag accent" style={{ marginLeft: 6 }}>待确认</span>}
                            </div>
                            <p className="list-card-detail">
                              {item.size} · {item.uploadedAt}
                            </p>
                            {keywordEditing?.id === item.id ? (
                              <div className="row" style={{ gap: 6, marginTop: 6 }}>
                                <input
                                  className="input"
                                  value={keywordEditing.value}
                                  onChange={(event) =>
                                    setKeywordEditing({ id: item.id, value: event.target.value })
                                  }
                                  placeholder="用逗号分隔多个关键词"
                                />
                                <button
                                  type="button"
                                  className="primary-btn"
                                  onClick={() => saveKeywords(item.id, keywordEditing.value)}
                                >
                                  保存
                                </button>
                              </div>
                            ) : (
                              <div className="row" style={{ gap: 4, marginTop: 6, flexWrap: "wrap" }}>
                                {(item.keywords ?? []).length === 0 ? (
                                  <span className="caption">暂无关键词</span>
                                ) : (
                                  (item.keywords ?? []).map((tag) => (
                                    <span key={tag} className="tag">{tag}</span>
                                  ))
                                )}
                                <button
                                  type="button"
                                  className="ghost-btn"
                                  onClick={() =>
                                    setKeywordEditing({
                                      id: item.id,
                                      value: (item.keywords ?? []).join(", ")
                                    })
                                  }
                                >
                                  编辑关键词
                                </button>
                              </div>
                            )}
                          </div>
                          <div className="row" style={{ gap: 6 }}>
                            {item.pending && (
                              <button type="button" className="secondary-btn" onClick={() => confirmMaterial(item.id)}>
                                确认
                              </button>
                            )}
                            <button
                              type="button"
                              className="ghost-btn"
                              onClick={() => setConfirm({ kind: "material", id: item.id, name: item.name })}
                            >
                              删除
                            </button>
                          </div>
                        </div>
                      </div>
                    ))}
                    {filteredMaterials.length === 0 && (
                      <p className="caption">
                        {materialKeyword.trim() ? "没有匹配的资料。" : "还没有资料，可上传或从笔记提炼。"}
                      </p>
                    )}
                  </div>
                </div>
              )}

              {tab === "dashboard" && (
                <div className={dashboardFull ? "tab-body dashboard-full" : "tab-body"}>
                  <div className="row" style={{ justifyContent: "space-between" }}>
                    <h3 className="section-title">数据窗</h3>
                    <div className="row" style={{ gap: 6, alignItems: "center" }}>
                      <span className="caption">{KIND_LABEL[dashboard.kind]}类指标</span>
                      <button
                        type="button"
                        className="secondary-btn"
                        onClick={() => setDashboardFull((current) => !current)}
                      >
                        {dashboardFull ? "退出全屏" : "全屏"}
                      </button>
                      <button
                        type="button"
                        className="ghost-btn"
                        onClick={() => {
                          setDashboardFull(false);
                          setTab("diary");
                        }}
                      >
                        返回
                      </button>
                    </div>
                  </div>

                  <div className="stats" style={{ marginTop: 10 }}>
                    {dashboard.kind === "douyin" ? (
                      <>
                        <div className="card">
                          <div className="caption">总发布数</div>
                          <div className="stat-value">{dashboard.videos.length}</div>
                        </div>
                        <div className="card">
                          <div className="caption">本月发布</div>
                          <div className="stat-value">{dashboard.thisMonth}</div>
                        </div>
                        <div className="card">
                          <div className="caption">本周发布</div>
                          <div className="stat-value">{dashboard.thisWeek}</div>
                        </div>
                        <div className="card">
                          <div className="caption">今天发布</div>
                          <div className="stat-value">{dashboard.today}</div>
                        </div>
                      </>
                    ) : (
                      <>
                        <div className="card">
                          <div className="caption">资料总数</div>
                          <div className="stat-value">{selected.materials.length}</div>
                        </div>
                        <div className="card">
                          <div className="caption">本月新增</div>
                          <div className="stat-value">{dashboard.thisMonth}</div>
                        </div>
                        <div className="card">
                          <div className="caption">待确认</div>
                          <div className="stat-value">{dashboard.pendingCount}</div>
                        </div>
                        <div className="card">
                          <div className="caption">资料占用</div>
                          <div className="stat-value" style={{ fontSize: 18 }}>{formatBytes(dashboard.totalBytes)}</div>
                        </div>
                      </>
                    )}
                  </div>

                  {dashboard.kind === "douyin" ? (
                    <p className="caption" style={{ marginTop: 10 }}>
                      播放量与点赞量需要接入平台数据（抖音开放平台授权）后再展示；
                      当前只统计到本地资料层面的发布数量。
                    </p>
                  ) : (
                    <p className="caption" style={{ marginTop: 10 }}>
                      按本地资料实际统计；最近更新：{dashboard.lastUpdated}
                    </p>
                  )}

                  <div style={{ marginTop: 14 }}>
                    <div className="caption">最近 7 天笔记数</div>
                    <div className="row" style={{ alignItems: "flex-end", gap: 8, marginTop: 8, height: 72 }}>
                      {dashboard.week.map((day) => (
                        <div key={day.key} style={{ flex: 1, textAlign: "center" }}>
                          <div
                            title={`${day.key}：${day.count} 篇`}
                            style={{
                              height: `${day.height}px`,
                              background: "var(--accent)",
                              opacity: day.count > 0 ? 0.85 : 0.2,
                              borderRadius: 4
                            }}
                          />
                          <div className="caption" style={{ fontSize: 10, marginTop: 4 }}>{day.label}</div>
                        </div>
                      ))}
                    </div>
                  </div>

                  <div style={{ marginTop: 14 }}>
                    <div className="caption">资料类型分布</div>
                    {dashboard.types.length === 0 ? (
                      <p className="caption" style={{ marginTop: 6 }}>还没有资料。</p>
                    ) : (
                      <div style={{ marginTop: 8 }}>
                        {dashboard.types.map((item) => (
                          <div key={item.ext} className="row" style={{ gap: 8, marginTop: 6 }}>
                            <span className="tag" style={{ minWidth: 56, textAlign: "center" }}>{item.ext}</span>
                            <div style={{ flex: 1, height: 8, background: "var(--line)", borderRadius: 4 }}>
                              <div
                                style={{
                                  width: `${item.percent}%`,
                                  height: "100%",
                                  background: "var(--accent)",
                                  borderRadius: 4
                                }}
                              />
                            </div>
                            <span className="caption" style={{ minWidth: 72, textAlign: "right" }}>
                              {item.count} 份 · {item.sizeText}
                            </span>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </>
          ) : (
            <div className="empty">请选择或新建一个应用。</div>
          )}
        </div>
      </div>

      {newOpen && (
        <div className="dialog-mask" onClick={() => setNewOpen(false)}>
          <div className="dialog" onClick={(event) => event.stopPropagation()}>
            <h2 className="dialog-title">{newParent ? "新建子应用" : "新建应用"}</h2>
            {newParent && (
              <p className="caption" style={{ marginTop: 6 }}>
                子应用属于「{findNode(apps, newParent)?.title}」，内容与主应用一致（笔记 + 资料库）。
              </p>
            )}
            <label className="field-label" style={{ marginTop: 12 }}>应用名称</label>
            <input
              className="input"
              value={newTitle}
              onChange={(event) => setNewTitle(event.target.value)}
              placeholder="例如：主板维修记录"
            />
            <label className="field-label" style={{ marginTop: 10 }}>应用类型（决定数据窗指标）</label>
            <select
              className="input"
              value={newKind}
              onChange={(event) => setNewKind(event.target.value as AppKind)}
            >
              <option value="generic">通用</option>
              <option value="repair">电脑维修</option>
              <option value="douyin">抖音短视频</option>
            </select>
            <div className="dialog-actions">
              <button type="button" className="secondary-btn" onClick={() => setNewOpen(false)}>
                取消
              </button>
              <button type="button" className="primary-btn" onClick={createApp}>
                创建
              </button>
            </div>
          </div>
        </div>
      )}

      {renameOpen && (
        <div className="dialog-mask" onClick={() => setRenameOpen(false)}>
          <div className="dialog" onClick={(event) => event.stopPropagation()}>
            <h2 className="dialog-title">重命名</h2>
            <input
              className="input"
              style={{ marginTop: 12 }}
              value={renameTitle}
              onChange={(event) => setRenameTitle(event.target.value)}
            />
            <div className="dialog-actions">
              <button type="button" className="secondary-btn" onClick={() => setRenameOpen(false)}>
                取消
              </button>
              <button type="button" className="primary-btn" onClick={renameApp}>
                保存
              </button>
            </div>
          </div>
        </div>
      )}

      {diaryOpen && (
        <div className="dialog-mask" onClick={() => setDiaryOpen(false)}>
          <div className="dialog" onClick={(event) => event.stopPropagation()}>
            <h2 className="dialog-title">{diaryEditingId ? "编辑笔记" : "新增笔记"}</h2>
            <label className="field-label" style={{ marginTop: 12 }}>标题</label>
            <input
              className="input"
              value={diaryTitle}
              onChange={(event) => setDiaryTitle(event.target.value)}
            />
            <label className="field-label" style={{ marginTop: 10 }}>内容</label>
            <TextWithVoice
              value={diaryContent}
              onChange={setDiaryContent}
              rows={6}
              placeholder="写点什么，也可以点麦克风说"
              onNotice={onNotice}
            />
            <div className="dialog-actions">
              <button type="button" className="secondary-btn" onClick={() => setDiaryOpen(false)}>
                取消
              </button>
              <button type="button" className="primary-btn" onClick={saveDiary}>
                保存
              </button>
            </div>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={confirm !== null}
        title={
          confirm?.kind === "app"
            ? `删除「${confirm.name}」？`
            : confirm?.kind === "diary"
              ? "删除这篇笔记？"
              : "删除这份资料？"
        }
        detail={
          confirm?.kind === "app"
            ? "笔记和资料会一起删除，不可恢复；子应用也会一起删除。"
            : "删除后不可恢复。"
        }
        confirmText="删除"
        onCancel={() => setConfirm(null)}
        onConfirm={confirmRemove}
      />
    </div>
  );
}
