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
