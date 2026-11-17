import { useState } from "react";
import {
  createId,
  formatDate,
  formatSize,
  SAMPLE_APPS,
  usePersistentState,
  type AppModule,
  type DiaryEntry
} from "../assistant";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { TextWithVoice } from "../components/TextWithVoice";


// 数据看板用的纯函数：全部按当前应用的真实数据算，不做任何写死。
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
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

interface Props {
  onNotice: (message: string) => void;
}

export function ApplicationPage({ onNotice }: Props) {
  const [apps, setApps] = usePersistentState<AppModule[]>("kaixin.apps.v1", SAMPLE_APPS);
  const [selectedId, setSelectedId] = useState(apps[0]?.id ?? "");
  const [tab, setTab] = useState<"diary" | "material">("diary");

  const [newOpen, setNewOpen] = useState(false);
  const [newTitle, setNewTitle] = useState("");

  const [diaryOpen, setDiaryOpen] = useState(false);
  const [diaryEditingId, setDiaryEditingId] = useState<string | null>(null);
  const [diaryTitle, setDiaryTitle] = useState("");
  const [diaryContent, setDiaryContent] = useState("");

  const [confirm, setConfirm] = useState<{ kind: "app" | "diary" | "material"; id: string; name: string } | null>(
    null
  );

  const selected = apps.find((app) => app.id === selectedId) ?? apps[0];

  const updateSelected = (updater: (app: AppModule) => AppModule) => {
    if (!selected) {
      return;
    }
    setApps((current) => current.map((app) => (app.id === selected.id ? updater(app) : app)));
  };

  const createApp = () => {
    if (!newTitle.trim()) {
      onNotice("请填写应用标题名称。");
      return;
    }
    const next: AppModule = {
      id: createId("app"),
      title: newTitle.trim(),
      diaries: [],
      materials: []
    };
    setApps((current) => [...current, next]);
    setSelectedId(next.id);
    setNewOpen(false);
    setNewTitle("");
    setTab("diary");
    onNotice(`已新建应用「${next.title}」，包含笔记与资料库两个板块。`);
  };

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

  const addMaterials = (files: FileList | null) => {
    if (!selected || !files || files.length === 0) {
      return;
    }
    const items = Array.from(files).map((file) => ({
      id: createId("mat"),
      name: file.name,
      size: formatSize(file.size),
      uploadedAt: formatDate()
    }));
    updateSelected((app) => ({ ...app, materials: [...items, ...app.materials] }));
    onNotice(`已上传 ${items.length} 份资料到「${selected.title}」资料库。`);
  };

  const confirmRemove = () => {
    if (!confirm) {
      return;
    }
    if (confirm.kind === "app") {
      setApps((current) => current.filter((app) => app.id !== confirm.id));
      setConfirm(null);
      onNotice(`已删除应用「${confirm.name}」及其笔记与资料。`);
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

  const dashboard = (() => {
    const diaries = selected?.diaries ?? [];
    const materials = selected?.materials ?? [];
    const totalBytes = materials.reduce((sum, item) => sum + parseMaterialSize(item.size), 0);

    const sorted = [...diaries].sort((a, b) => (a.createdAt < b.createdAt ? 1 : -1));
    const lastUpdated = sorted.length > 0 ? dayKey(sorted[0].createdAt) || "—" : "暂无笔记";

    const days: { key: string; label: string; count: number; height: number }[] = [];
    for (let offset = 6; offset >= 0; offset -= 1) {
      const date = new Date();
      date.setDate(date.getDate() - offset);
      const pad = (n: number) => String(n).padStart(2, "0");
      const key = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
      days.push({ key, label: `${date.getMonth() + 1}/${date.getDate()}`, count: 0, height: 4 });
    }
    for (const entry of diaries) {
      const key = dayKey(entry.createdAt);
      const hit = days.find((day) => day.key === key);
      if (hit) {
        hit.count += 1;
      }
    }
    const maxCount = Math.max(1, ...days.map((day) => day.count));
    for (const day of days) {
      day.height = day.count === 0 ? 4 : Math.round((day.count / maxCount) * 60) + 8;
    }

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

    return { totalBytes, lastUpdated, week: days, types };
  })();

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="page-title">应用助手</h1>
          <p className="page-subtitle">每个应用独立管理笔记与资料库，内容保存在本机</p>
        </div>
        <button type="button" className="secondary-btn" onClick={() => setNewOpen(true)}>
          + 新建应用
        </button>
      </div>

      <div className="assistant-layout">
        <div className="assistant-side">
          <div className="side-title">
            全部应用 <span className="caption">{apps.length}</span>
          </div>
          <div className="list" style={{ marginTop: 8 }}>
            {apps.map((app) => (
              <div key={app.id} className={`side-item-wrap${app.id === selected?.id ? " selected" : ""}`}>
                <button type="button" className="side-item" onClick={() => setSelectedId(app.id)}>
                  <span className="side-item-title">{app.title}</span>
                  <span className="side-item-meta">
                    笔记 {app.diaries.length} · 资料 {app.materials.length}
                  </span>
                </button>
                <button
                  type="button"
                  className="side-item-remove"
                  title="删除应用"
                  onClick={() => setConfirm({ kind: "app", id: app.id, name: app.title })}
                >
                  ×
                </button>
              </div>
            ))}
          </div>
          {apps.length === 0 && <div className="empty">还没有应用，点击右上角新建。</div>}
        </div>

        <div className="assistant-main">
          {selected ? (
            <>
              <div className="detail-head">
                <div>
                  <h2 className="detail-title">{selected.title}</h2>
                  <div className="row" style={{ gap: 6, marginTop: 6 }}>
                    <span className="tag accent">应用</span>
                    <span className="tag">笔记 {selected.diaries.length}</span>
                    <span className="tag">资料 {selected.materials.length}</span>
                  </div>
                </div>
              </div>


              {/* 数据看板：按当前应用的真实笔记与资料计算 */}
              <div className="card" style={{ marginTop: 12 }}>
                <div className="row" style={{ justifyContent: "space-between" }}>
                  <h3 className="section-title">数据看板</h3>
                  <span className="caption">按当前应用实时统计</span>
                </div>

                <div className="stats" style={{ marginTop: 10 }}>
                  <div className="card">
                    <div className="caption">笔记数</div>
                    <div className="stat-value">{selected.diaries.length}</div>
                  </div>
                  <div className="card">
                    <div className="caption">资料数</div>
                    <div className="stat-value">{selected.materials.length}</div>
                  </div>
                  <div className="card">
                    <div className="caption">资料占用</div>
                    <div className="stat-value" style={{ fontSize: 18 }}>{formatBytes(dashboard.totalBytes)}</div>
                  </div>
                  <div className="card">
                    <div className="caption">最近更新</div>
                    <div className="stat-value" style={{ fontSize: 14 }}>{dashboard.lastUpdated}</div>
                  </div>
                </div>

                <div style={{ marginTop: 14 }}>
                  <div className="caption">最近 7 天笔记数</div>
                  <div className="row" style={{ alignItems: "flex-end", gap: 8, marginTop: 8, height: 72 }}>
                    {dashboard.week.map((day) => (
                      <div key={day.key} style={{ flex: 1, textAlign: "center" }}>
                        <div
                          title={`${day.key}：${day.count} 篇`}
                          style={{
                            height: `${Math.max(4, day.height)}px`,
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
                    <p className="caption" style={{ marginTop: 6 }}>还没有资料，去「资料库」上传后这里会有统计。</p>
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
              </div>

              {tab === "diary" ? (
                <div className="tab-body">
                  <div className="row" style={{ justifyContent: "space-between" }}>
                    <h3 className="section-title">历史笔记</h3>
                    <button type="button" className="primary-btn" onClick={openNewDiary}>
                      + 新增笔记
                    </button>
                  </div>
                  <div className="list" style={{ marginTop: 12 }}>
                    {selected.diaries.map((entry) => (
                      <button
                        type="button"
                        key={entry.id}
                        className="list-card"
                        onClick={() => openDiary(entry)}
                      >
                        <div className="list-card-head">
                          <span className="list-card-title">{entry.title}</span>
                          <span className="tag">{entry.createdAt}</span>
                        </div>
                        <p className="list-card-detail">{entry.content}</p>
                      </button>
                    ))}
                    {selected.diaries.length === 0 && (
                      <div className="empty">还没有笔记，点击“新增笔记”开始记录。</div>
                    )}
                  </div>
                </div>
              ) : (
                <div className="tab-body">
                  <div className="row" style={{ justifyContent: "space-between" }}>
                    <h3 className="section-title">资料</h3>
                    <label className="primary-btn file-btn">
                      + 新增资料
                      <input
                        type="file"
                        multiple
                        onChange={(event) => {
                          addMaterials(event.target.files);
                          event.target.value = "";
                        }}
                      />
                    </label>
                  </div>
                  <div className="list" style={{ marginTop: 12 }}>
                    {selected.materials.map((item) => (
                      <div className="material-row" key={item.id}>
                        <div>
                          <div className="list-card-title">{item.name}</div>
                          <p className="list-card-detail">
                            {item.size} · 上传于 {item.uploadedAt}
                          </p>
                        </div>
                        <button
                          type="button"
                          className="ghost-btn danger-text"
                          onClick={() => setConfirm({ kind: "material", id: item.id, name: item.name })}
                        >
                          删除
                        </button>
                      </div>
                    ))}
                    {selected.materials.length === 0 && (
                      <div className="empty">还没有资料，点击“新增资料”上传文件。</div>
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
            <h2 className="dialog-title">新建应用</h2>
            <p className="caption" style={{ marginTop: 7 }}>
              新建后自动包含“笔记”和“资料库”两个板块。
            </p>
            <label className="field-label">标题名称</label>
            <input
              className="input"
              placeholder="例如 电脑维修、恋爱宝典"
              value={newTitle}
              onChange={(event) => setNewTitle(event.target.value)}
            />
            <div className="row" style={{ gap: 6, marginTop: 12 }}>
              <span className="tag accent">笔记</span>
              <span className="tag accent">资料库</span>
            </div>
            <div className="dialog-actions">
              <button type="button" className="secondary-btn" onClick={() => setNewOpen(false)}>
                取消
              </button>
              <button type="button" className="primary-btn" onClick={createApp}>
                创建应用
              </button>
            </div>
          </div>
        </div>
      )}

      {diaryOpen && (
        <div className="dialog-mask" onClick={() => setDiaryOpen(false)}>
          <div className="dialog" onClick={(event) => event.stopPropagation()}>
            <h2 className="dialog-title">{diaryEditingId ? "笔记详情" : "新增笔记"}</h2>
            <p className="caption" style={{ marginTop: 7 }}>
              可以打字，也可以语音录入，语音会直接转成文字。
            </p>
            <label className="field-label">笔记标题</label>
            <input
              className="input"
              value={diaryTitle}
              onChange={(event) => setDiaryTitle(event.target.value)}
            />
            <label className="field-label">笔记内容</label>
            <TextWithVoice
              value={diaryContent}
              onChange={setDiaryContent}
              rows={6}
              placeholder="今天发生了什么，有什么收获…"
              onNotice={onNotice}
            />
            <div className="dialog-actions">
              {diaryEditingId && (
                <button
                  type="button"
                  className="ghost-btn danger-text"
                  style={{ marginRight: "auto" }}
                  onClick={() => {
                    const target = selected?.diaries.find((entry) => entry.id === diaryEditingId);
                    setDiaryOpen(false);
                    if (target) {
                      setConfirm({ kind: "diary", id: target.id, name: target.title });
                    }
                  }}
                >
                  删除笔记
                </button>
              )}
              <button type="button" className="secondary-btn" onClick={() => setDiaryOpen(false)}>
                取消
              </button>
              <button type="button" className="primary-btn" onClick={saveDiary}>
                {diaryEditingId ? "保存修改" : "保存笔记"}
              </button>
            </div>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={confirm !== null}
        title={
          confirm?.kind === "app"
            ? `删除应用「${confirm.name}」？`
            : confirm?.kind === "diary"
              ? "删除这篇笔记？"
              : "删除这份资料？"
        }
        detail={confirm?.kind === "app" ? "应用下的笔记和资料会一起删除，删除后不可恢复。" : "删除后不可恢复。"}
        onCancel={() => setConfirm(null)}
        onConfirm={confirmRemove}
      />
    </div>
  );
}
