import { useState } from "react";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { BottomSheet, MobileHeader } from "../components/MobileHeader";
import { TextWithVoice } from "../components/TextWithVoice";
import { IconCheck, IconPlusPlain, IconTrash } from "../components/Icons";
import {
  createId,
  formatDate,
  formatSize,
  normalizeApps,
  SAMPLE_APPS,
  usePersistentState,
  type AppCategory,
  type AppModule,
  type AppTask,
  type DiaryEntry,
  type MaterialItem
} from "../assistant";

interface Props {
  onBack: () => void;
  onNotice: (message: string) => void;
}

type Tab = "notes" | "files" | "tasks" | "data";

const CATEGORY_TABS: { key: AppCategory; label: string }[] = [
  { key: "work", label: "工作" },
  { key: "life", label: "生活" },
  { key: "study", label: "学习" }
];

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

function walk(nodes: AppModule[], visit: (node: AppModule) => void): void {
  for (const node of nodes) {
    visit(node);
    if (node.children && node.children.length > 0) {
      walk(node.children, visit);
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

function flattenApps(nodes: AppModule[]): AppModule[] {
  const out: AppModule[] = [];
  walk(nodes, (node) => out.push(node));
  return out;
}

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

interface DashboardData {
  totalBytes: number;
  thisMonth: number;
  pendingCount: number;
  types: { ext: string; count: number; sizeText: string; percent: number }[];
  week: { key: string; label: string; count: number; height: number }[];
  lastUpdated: string;
  tasksTotal: number;
  tasksDone: number;
}

/** 数据窗计算：L1 运行卡片与 L2 详情共用，避免两套口径。 */
function computeDashboard(app: AppModule): DashboardData {
  const diaries = app.diaries;
  const materials = app.materials;
  const totalBytes = materials.reduce((sum, item) => sum + parseMaterialSize(item.size), 0);

  const thisMonth = materials.filter((item) => {
    const ago = daysAgo(item.uploadedAt);
    return ago !== null && ago <= 30;
  }).length;
  const pendingCount = materials.filter((item) => item.pending).length;

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
    day.height = day.count === 0 ? 4 : Math.round((day.count / maxCount) * 36) + 6;
  }

  const sorted = [...diaries].sort((a, b) => (a.createdAt < b.createdAt ? 1 : -1));
  const lastUpdated = sorted.length > 0 ? sorted[0].createdAt : "暂无笔记";

  return {
    totalBytes,
    thisMonth,
    pendingCount,
    types,
    week: days,
    lastUpdated,
    tasksTotal: app.tasks.length,
    tasksDone: app.tasks.filter((task) => task.done).length
  };
}

/** L1「正在运行」卡片：紧凑版数据窗 */
function DataCard({ app, onClick }: { app: AppModule; onClick: () => void }) {
  const d = computeDashboard(app);
  const taskPercent = d.tasksTotal > 0 ? Math.round((d.tasksDone / d.tasksTotal) * 100) : 0;
  return (
    <button type="button" className="card data-card-mobile" onClick={onClick}>
      <div className="data-card-head">
        <strong>{app.title}</strong>
        <span className="tag accent">运行中</span>
      </div>
      <p className="caption">最近更新 {d.lastUpdated}</p>

      <div className="data-stats-mobile">
        <div className="data-stat-mobile">
          <span>笔记</span>
          <strong>{app.diaries.length}</strong>
        </div>
        <div className="data-stat-mobile">
          <span>资料</span>
          <strong>{app.materials.length}</strong>
        </div>
        <div className="data-stat-mobile">
          <span>待确认</span>
          <strong>{d.pendingCount}</strong>
        </div>
        <div className="data-stat-mobile">
          <span>任务</span>
          <strong>
            {d.tasksDone}/{d.tasksTotal}
          </strong>
        </div>
      </div>

      <div className="mini-progress">
        <span style={{ width: `${taskPercent}%` }} />
      </div>

      <div className="mini-bars">
        {d.week.map((day) => (
          <div key={day.key} className="mini-bar">
            <i
              title={`${day.key}：${day.count} 篇`}
              style={{ height: `${day.height}px`, opacity: day.count > 0 ? 0.85 : 0.2 }}
            />
            <em>{day.label}</em>
          </div>
        ))}
      </div>
    </button>
  );
}

// ---------- 页面 ----------

export function AppsPage({ onBack, onNotice }: Props) {
  const [storedApps, setStoredApps] = usePersistentState<AppModule[]>("kaixin.mobile.apps", SAMPLE_APPS);
  const apps = normalizeApps(storedApps);
  const [detailId, setDetailId] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>("notes");
  const [category, setCategory] = useState<AppCategory>("work");

  const [createOpen, setCreateOpen] = useState(false);
  const [newTitle, setNewTitle] = useState("");
  const [newCategory, setNewCategory] = useState<AppCategory>("work");

  const [noteSheet, setNoteSheet] = useState<"new" | "edit" | null>(null);
  const [noteTitle, setNoteTitle] = useState("");
  const [noteContent, setNoteContent] = useState("");
  const [noteId, setNoteId] = useState("");

  const [taskInput, setTaskInput] = useState("");

  const [confirm, setConfirm] = useState<{
    kind: "app" | "note" | "material";
    id: string;
    name: string;
  } | null>(null);

  const selected = detailId ? findNode(apps, detailId) : undefined;

  const updateApps = (updater: (list: AppModule[]) => AppModule[]) => {
    setStoredApps((current) => updater(normalizeApps(current)));
  };

  const updateSelected = (updater: (app: AppModule) => AppModule) => {
    if (!selected) {
      return;
    }
    setStoredApps((current) => mapNode(normalizeApps(current), selected.id, updater));
  };

  const closeDetail = () => {
    setDetailId(null);
    setTab("notes");
  };

  const createApp = () => {
    const title = newTitle.trim();
    if (!title) {
      onNotice("请填写应用标题。");
      return;
    }
    const app: AppModule = {
      id: createId("app"),
      title,
      category: newCategory,
      diaries: [],
      materials: [],
      tasks: []
    };
    updateApps((list) => [app, ...list]);
    setNewTitle("");
    setCreateOpen(false);
    setCategory(newCategory);
    setDetailId(app.id);
    setTab("notes");
    onNotice(`已新建应用：${title}`);
  };

  const openNewNote = () => {
    setNoteId("");
    setNoteTitle("");
    setNoteContent("");
    setNoteSheet("new");
  };

  const openNote = (note: DiaryEntry) => {
    setNoteId(note.id);
    setNoteTitle(note.title);
    setNoteContent(note.content);
    setNoteSheet("edit");
  };

  const saveNote = () => {
    if (!selected) {
      return;
    }
    const title = noteTitle.trim();
    if (!title) {
      onNotice("请填写笔记标题。");
      return;
    }
    if (noteId) {
      updateSelected((app) => ({
        ...app,
        diaries: app.diaries.map((item) =>
          item.id === noteId ? { ...item, title, content: noteContent } : item
        )
      }));
      onNotice("笔记已修改。");
    } else {
      const note: DiaryEntry = {
        id: createId("note"),
        title,
        content: noteContent,
        createdAt: formatDate()
      };
      updateSelected((app) => ({ ...app, diaries: [note, ...app.diaries] }));
      onNotice("已新增笔记。");
    }
    setNoteSheet(null);
  };

  const addFiles = (files: FileList | null) => {
    if (!selected || !files || files.length === 0) {
      return;
    }
    const items: MaterialItem[] = Array.from(files).map((file) => ({
      id: createId("file"),
      name: file.name,
      size: formatSize(file.size),
      uploadedAt: formatDate(),
      pending: true
    }));
    updateSelected((app) => ({ ...app, materials: [...items, ...app.materials] }));
    onNotice(`已上传 ${items.length} 个文件，均为待确认状态。`);
  };

  const confirmMaterial = (id: string) => {
    updateSelected((app) => ({
      ...app,
      materials: app.materials.map((item) => (item.id === id ? { ...item, pending: false } : item))
    }));
    onNotice("资料已确认。");
  };

  const addTask = () => {
    if (!selected) {
      return;
    }
    const text = taskInput.trim();
    if (!text) {
      onNotice("请先输入任务内容。");
      return;
    }
    const task: AppTask = { id: createId("task"), text, done: false };
    updateSelected((app) => ({ ...app, tasks: [...app.tasks, task] }));
    setTaskInput("");
    onNotice("已添加计划任务。");
  };

  const toggleTask = (id: string) => {
    updateSelected((app) => ({
      ...app,
      tasks: app.tasks.map((task) => (task.id === id ? { ...task, done: !task.done } : task))
    }));
  };

  const deleteTask = (id: string) => {
    updateSelected((app) => ({ ...app, tasks: app.tasks.filter((task) => task.id !== id) }));
    onNotice("已删除任务。");
  };

  const removeConfirmed = () => {
    if (!confirm) {
      return;
    }
    if (confirm.kind === "app") {
      updateApps((list) => removeNode(list, confirm.id));
      if (detailId === confirm.id) {
        closeDetail();
      }
      onNotice(`已删除应用：${confirm.name}`);
    } else if (selected) {
      updateSelected((app) =>
        confirm.kind === "note"
          ? { ...app, diaries: app.diaries.filter((item) => item.id !== confirm.id) }
          : { ...app, materials: app.materials.filter((item) => item.id !== confirm.id) }
      );
      onNotice(confirm.kind === "note" ? "已删除笔记。" : "已删除资料。");
    }
    setConfirm(null);
  };

  // ---------- L2：应用详情 ----------

  if (selected) {
    const counts = rollup(selected);
    const dashboard = computeDashboard(selected);
    const isSubApp = !apps.some((root) => root.id === selected.id);

    return (
      <>
        <MobileHeader
          title={selected.title}
          onBack={closeDetail}
          action={
            <>
              <button type="button" className="link-btn" onClick={closeDetail}>
                关闭
              </button>
              <button
                type="button"
                className="icon-btn"
                onClick={() => setConfirm({ kind: "app", id: selected.id, name: selected.title })}
                aria-label="删除应用"
              >
                <IconTrash />
              </button>
            </>
          }
        />

        <div className="page-scroll">
          <div className="row" style={{ gap: 6, flexWrap: "wrap", margin: "2px 0 10px" }}>
            <span className="tag accent">{isSubApp ? "子应用" : "应用"}</span>
            <span className="tag">笔记 {counts.diaries}</span>
            <span className="tag">资料 {counts.materials}</span>
            {counts.pending > 0 && <span className="tag accent">待确认 {counts.pending}</span>}
            {selected.tasks.some((task) => !task.done) && <span className="tag accent">运行中</span>}
          </div>

          <div className="tabs">
            <button
              type="button"
              className={`tab${tab === "notes" ? " active" : ""}`}
              onClick={() => setTab("notes")}
            >
              笔记
            </button>
            <button
              type="button"
              className={`tab${tab === "files" ? " active" : ""}`}
              onClick={() => setTab("files")}
            >
              资料库
            </button>
            <button
              type="button"
              className={`tab${tab === "tasks" ? " active" : ""}`}
              onClick={() => setTab("tasks")}
            >
              计划任务
            </button>
            <button
              type="button"
              className={`tab${tab === "data" ? " active" : ""}`}
              onClick={() => setTab("data")}
            >
              数据窗
            </button>
          </div>

          {tab === "notes" && (
            <>
              <div className="section-head">
                <span>历史笔记 · {selected.diaries.length}</span>
                <button type="button" className="mini-primary" onClick={openNewNote}>
                  <IconPlusPlain size={16} /> 新增笔记
                </button>
              </div>
              {selected.diaries.length === 0 && <p className="empty">还没有笔记，点右上角新增。</p>}
              {selected.diaries.map((note) => (
                <button key={note.id} type="button" className="card tap" onClick={() => openNote(note)}>
                  <div className="card-title-row">
                    <strong>{note.title}</strong>
                    <span className="muted">{note.createdAt}</span>
                  </div>
                  <p className="card-text">{note.content || "（无内容）"}</p>
                </button>
              ))}
            </>
          )}

          {tab === "files" && (
            <>
              <div className="section-head">
                <span>资料 · {selected.materials.length}</span>
                <label className="mini-primary">
                  <IconPlusPlain size={16} /> 上传资料
                  <input
                    type="file"
                    multiple
                    hidden
                    onChange={(event) => {
                      addFiles(event.target.files);
                      event.target.value = "";
                    }}
                  />
                </label>
              </div>
              {selected.materials.length === 0 && <p className="empty">还没有资料，可上传文件。</p>}
              {selected.materials.map((file) => (
                <div key={file.id} className="card file-row">
                  <span className="file-icon">文</span>
                  <div className="file-meta">
                    <strong>
                      {file.name}
                      {file.pending && <span className="tag accent" style={{ marginLeft: 6 }}>待确认</span>}
                    </strong>
                    <span className="muted">
                      {file.size} · {file.uploadedAt}
                    </span>
                  </div>
                  {file.pending && (
                    <button
                      type="button"
                      className="icon-btn"
                      onClick={() => confirmMaterial(file.id)}
                      aria-label="确认资料"
                    >
                      <IconCheck />
                    </button>
                  )}
                  <button
                    type="button"
                    className="icon-btn"
                    onClick={() => setConfirm({ kind: "material", id: file.id, name: file.name })}
                    aria-label="删除资料"
                  >
                    <IconTrash />
                  </button>
                </div>
              ))}
            </>
          )}

          {tab === "tasks" && (
            <>
              <div className="section-head">
                <span>
                  计划任务 · 完成 {selected.tasks.filter((task) => task.done).length} / {selected.tasks.length}
                </span>
              </div>
              <div className="row" style={{ gap: 6, marginBottom: 10 }}>
                <input
                  className="input"
                  style={{ flex: 1 }}
                  placeholder="添加一条计划任务，回车保存"
                  value={taskInput}
                  onChange={(event) => setTaskInput(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") {
                      addTask();
                    }
                  }}
                />
                <button type="button" className="primary-btn" onClick={addTask}>
                  添加
                </button>
              </div>
              {selected.tasks.length === 0 && (
                <p className="empty">还没有计划任务，添加后应用会出现在「正在运行」。</p>
              )}
              {selected.tasks.map((task) => (
                <div key={task.id} className="card task-row-mobile">
                  <label className="task-main">
                    <input type="checkbox" checked={task.done} onChange={() => toggleTask(task.id)} />
                    <span className={task.done ? "done" : ""}>{task.text}</span>
                  </label>
                  <button
                    type="button"
                    className="icon-btn"
                    onClick={() => deleteTask(task.id)}
                    aria-label="删除任务"
                  >
                    <IconTrash />
                  </button>
                </div>
              ))}
            </>
          )}

          {tab === "data" && (
            <>
              <div className="data-stats-mobile" style={{ marginBottom: 10 }}>
                <div className="data-stat-mobile">
                  <span>资料总数</span>
                  <strong>{selected.materials.length}</strong>
                </div>
                <div className="data-stat-mobile">
                  <span>本月新增</span>
                  <strong>{dashboard.thisMonth}</strong>
                </div>
                <div className="data-stat-mobile">
                  <span>待确认</span>
                  <strong>{dashboard.pendingCount}</strong>
                </div>
                <div className="data-stat-mobile">
                  <span>资料占用</span>
                  <strong>{formatBytes(dashboard.totalBytes)}</strong>
                </div>
              </div>

              <div className="card">
                <div className="caption">最近 7 天笔记数</div>
                <div className="mini-bars large">
                  {dashboard.week.map((day) => (
                    <div key={day.key} className="mini-bar">
                      <i
                        title={`${day.key}：${day.count} 篇`}
                        style={{ height: `${day.height}px`, opacity: day.count > 0 ? 0.85 : 0.2 }}
                      />
                      <em>{day.label}</em>
                    </div>
                  ))}
                </div>
              </div>

              <div className="card">
                <div className="caption">资料类型分布</div>
                {dashboard.types.length === 0 ? (
                  <p className="caption" style={{ marginTop: 6 }}>
                    还没有资料。
                  </p>
                ) : (
                  dashboard.types.map((item) => (
                    <div key={item.ext} className="row" style={{ gap: 8, marginTop: 8 }}>
                      <span className="tag" style={{ minWidth: 52, textAlign: "center" }}>
                        {item.ext}
                      </span>
                      <div className="mini-progress" style={{ flex: 1 }}>
                        <span style={{ width: `${item.percent}%` }} />
                      </div>
                      <span className="caption" style={{ minWidth: 68, textAlign: "right" }}>
                        {item.count} 份 · {item.sizeText}
                      </span>
                    </div>
                  ))
                )}
              </div>

              <p className="caption" style={{ margin: "0 2px" }}>
                按本地资料实际统计；最近更新：{dashboard.lastUpdated}
              </p>
            </>
          )}
        </div>

        <BottomSheet
          open={noteSheet !== null}
          title={noteSheet === "edit" ? "笔记详情" : "新增笔记"}
          onClose={() => setNoteSheet(null)}
          footer={
            <div className="sheet-actions">
              {noteSheet === "edit" && (
                <button
                  type="button"
                  className="danger-btn"
                  onClick={() => {
                    setNoteSheet(null);
                    setConfirm({ kind: "note", id: noteId, name: noteTitle });
                  }}
                >
                  删除笔记
                </button>
              )}
              <button type="button" className="primary-btn" onClick={saveNote}>
                {noteSheet === "edit" ? "保存修改" : "保存笔记"}
              </button>
            </div>
          }
        >
          <label className="field-label">笔记标题</label>
          <input
            className="input"
            value={noteTitle}
            onChange={(event) => setNoteTitle(event.target.value)}
            placeholder="例如：修一台进水的笔记本"
          />
          <label className="field-label">笔记内容</label>
          <TextWithVoice
            value={noteContent}
            onChange={setNoteContent}
            rows={5}
            placeholder="记录现象、排查过程和处理结果"
            onNotice={onNotice}
          />
        </BottomSheet>

        <ConfirmDialog
          open={confirm !== null}
          title={confirm?.kind === "app" ? "删除这个应用？" : "删除这条内容？"}
          detail={confirm ? `${confirm.name} 删除后无法恢复。` : undefined}
          onCancel={() => setConfirm(null)}
          onConfirm={removeConfirmed}
        />
      </>
    );
  }

  // ---------- L1：应用列表 ----------

  const categoryApps = apps.filter((app) => app.category === category);
  const runningApps = flattenApps(apps).filter((app) => app.tasks.some((task) => !task.done));

  return (
    <>
      <MobileHeader
        title="应用助手"
        onBack={onBack}
        action={
          <button type="button" className="mini-primary" onClick={() => setCreateOpen(true)}>
            <IconPlusPlain size={16} /> 新建
          </button>
        }
      />

      <div className="page-scroll">
        <div className="cat-tabs">
          {CATEGORY_TABS.map((item) => (
            <button
              key={item.key}
              type="button"
              className={`cat-tab${category === item.key ? " active" : ""}`}
              onClick={() => setCategory(item.key)}
            >
              {item.label}
            </button>
          ))}
        </div>

        <div className="card">
          <div className="section-head">
            <span>{CATEGORY_TABS.find((item) => item.key === category)?.label} · 已建立的应用</span>
            <span className="tag">{categoryApps.length} 个</span>
          </div>
          {categoryApps.length === 0 && (
            <p className="empty">该分类还没有应用，点右上角「新建」。</p>
          )}
          <div className="app-grid-mobile">
            {categoryApps.map((app) => (
              <div key={app.id}>
                <button
                  type="button"
                  className="app-row-mobile"
                  onClick={() => {
                    setDetailId(app.id);
                    setTab("notes");
                  }}
                >
                  <span className="app-badge">{app.title.slice(0, 1)}</span>
                  <span className="app-main">
                    <strong>{app.title}</strong>
                    <span className="muted">
                      笔记 {rollup(app).diaries} · 资料 {rollup(app).materials}
                      {rollup(app).pending > 0 ? ` · 待确认 ${rollup(app).pending}` : ""}
                      {app.tasks.some((task) => !task.done) ? " · 运行中" : ""}
                    </span>
                  </span>
                  <span className="chevron">›</span>
                </button>
                {(app.children ?? []).map((child) => (
                  <button
                    key={child.id}
                    type="button"
                    className="app-row-mobile sub"
                    onClick={() => {
                      setDetailId(child.id);
                      setTab("notes");
                    }}
                  >
                    <span className="app-badge">{child.title.slice(0, 1)}</span>
                    <span className="app-main">
                      <strong>└ {child.title}</strong>
                      <span className="muted">
                        子应用 · 笔记 {child.diaries.length} · 资料 {child.materials.length}
                      </span>
                    </span>
                    <span className="chevron">›</span>
                  </button>
                ))}
              </div>
            ))}
          </div>
        </div>

        <div className="card">
          <div className="section-head">
            <span>正在运行</span>
            <span className="tag">{runningApps.length} 个</span>
          </div>
          {runningApps.length === 0 ? (
            <p className="empty">没有正在运行的应用。给应用添加未完成的计划任务，它就会出现在这里。</p>
          ) : (
            <div className="running-grid-mobile">
              {runningApps.map((app) => (
                <DataCard
                  key={app.id}
                  app={app}
                  onClick={() => {
                    setDetailId(app.id);
                    setTab("tasks");
                  }}
                />
              ))}
            </div>
          )}
        </div>
      </div>

      <BottomSheet
        open={createOpen}
        title="新建应用"
        onClose={() => setCreateOpen(false)}
        footer={
          <div className="sheet-actions">
            <button type="button" className="primary-btn" onClick={createApp}>
              创建应用
            </button>
          </div>
        }
      >
        <label className="field-label">标题名称</label>
        <TextWithVoice
          value={newTitle}
          onChange={setNewTitle}
          rows={2}
          placeholder="例如：摄影技巧"
          onNotice={onNotice}
        />
        <label className="field-label">所属分类</label>
        <div className="cat-tabs create">
          {CATEGORY_TABS.map((item) => (
            <button
              key={item.key}
              type="button"
              className={`cat-tab${newCategory === item.key ? " active" : ""}`}
              onClick={() => setNewCategory(item.key)}
            >
              {item.label}
            </button>
          ))}
        </div>
        <p className="hint">新应用固定包含「笔记」「资料库」「计划任务」三个板块。</p>
      </BottomSheet>

      <ConfirmDialog
        open={confirm !== null}
        title={confirm?.kind === "app" ? "删除这个应用？" : "删除这条内容？"}
        detail={confirm ? `${confirm.name} 删除后无法恢复。` : undefined}
        onCancel={() => setConfirm(null)}
        onConfirm={removeConfirmed}
      />
    </>
  );
}
