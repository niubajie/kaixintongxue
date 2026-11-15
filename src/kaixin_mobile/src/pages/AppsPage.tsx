import { useState } from "react";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { BottomSheet, MobileHeader } from "../components/MobileHeader";
import { TextWithVoice } from "../components/TextWithVoice";
import { IconPlusPlain, IconTrash } from "../components/Icons";
import {
  SAMPLE_APPS,
  createId,
  formatDate,
  formatSize,
  usePersistentState,
  type AppModule,
  type DiaryEntry,
  type MaterialItem
} from "../assistant";

interface Props {
  onBack: () => void;
  onNotice: (message: string) => void;
}

export function AppsPage({ onBack, onNotice }: Props) {
  const [apps, setApps] = usePersistentState<AppModule[]>("kaixin.mobile.apps", SAMPLE_APPS);
  const [detailId, setDetailId] = useState<string | null>(null);
  const [tab, setTab] = useState<"notes" | "files">("notes");

  const [createOpen, setCreateOpen] = useState(false);
  const [newTitle, setNewTitle] = useState("");

  const [noteSheet, setNoteSheet] = useState<"new" | "edit" | null>(null);
  const [noteTitle, setNoteTitle] = useState("");
  const [noteContent, setNoteContent] = useState("");
  const [noteId, setNoteId] = useState("");

  const [confirm, setConfirm] = useState<{ kind: "app" | "note"; id: string; name: string } | null>(
    null
  );

  const current = apps.find((item) => item.id === detailId) ?? null;

  const patch = (id: string, updater: (app: AppModule) => AppModule) => {
    setApps((list) => list.map((item) => (item.id === id ? updater(item) : item)));
  };

  const createApp = () => {
    const title = newTitle.trim();
    if (!title) {
      onNotice("请填写应用标题。");
      return;
    }
    const app: AppModule = { id: createId("app"), title, diaries: [], materials: [] };
    setApps((list) => [app, ...list]);
    setNewTitle("");
    setCreateOpen(false);
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
    if (!current) {
      return;
    }
    const title = noteTitle.trim();
    if (!title) {
      onNotice("请填写笔记标题。");
      return;
    }
    if (noteId) {
      patch(current.id, (app) => ({
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
      patch(current.id, (app) => ({ ...app, diaries: [note, ...app.diaries] }));
      onNotice("已新增笔记。");
    }
    setNoteSheet(null);
  };

  const addFiles = (files: FileList | null) => {
    if (!current || !files || files.length === 0) {
      return;
    }
    const items: MaterialItem[] = Array.from(files).map((file) => ({
      id: createId("file"),
      name: file.name,
      size: formatSize(file.size),
      uploadedAt: formatDate()
    }));
    patch(current.id, (app) => ({ ...app, materials: [...items, ...app.materials] }));
    onNotice(`已上传 ${items.length} 个文件。`);
  };

  const removeConfirmed = () => {
    if (!confirm) {
      return;
    }
    if (confirm.kind === "app") {
      setApps((list) => list.filter((item) => item.id !== confirm.id));
      if (detailId === confirm.id) {
        setDetailId(null);
      }
      onNotice(`已删除应用：${confirm.name}`);
    } else if (current) {
      patch(current.id, (app) => ({
        ...app,
        diaries: app.diaries.filter((item) => item.id !== confirm.id)
      }));
      onNotice("已删除笔记。");
    }
    setConfirm(null);
  };

  if (current) {
    return (
      <>
        <MobileHeader
          title={current.title}
          onBack={() => setDetailId(null)}
          action={
            <button
              type="button"
              className="icon-btn"
              onClick={() => setConfirm({ kind: "app", id: current.id, name: current.title })}
              aria-label="删除应用"
            >
              <IconTrash />
            </button>
          }
        />

        <div className="page-scroll">
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
          </div>

          {tab === "notes" && (
            <>
              <div className="section-head">
                <span>历史笔记 · {current.diaries.length}</span>
                <button type="button" className="mini-primary" onClick={openNewNote}>
                  <IconPlusPlain size={16} /> 新增笔记
                </button>
              </div>
              {current.diaries.length === 0 && <p className="empty">还没有笔记，点右上角新增。</p>}
              {current.diaries.map((note) => (
                <button
                  key={note.id}
                  type="button"
                  className="card tap"
                  onClick={() => openNote(note)}
                >
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
                <span>资料 · {current.materials.length}</span>
                <label className="mini-primary">
                  <IconPlusPlain size={16} /> 新增资料
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
              {current.materials.length === 0 && <p className="empty">还没有资料，可上传文件。</p>}
              {current.materials.map((file) => (
                <div key={file.id} className="card file-row">
                  <span className="file-icon">文</span>
                  <div className="file-meta">
                    <strong>{file.name}</strong>
                    <span className="muted">
                      {file.size} · {file.uploadedAt}
                    </span>
                  </div>
                  <button
                    type="button"
                    className="icon-btn"
                    onClick={() => setConfirm({ kind: "note", id: file.id, name: file.name })}
                    aria-label="删除资料"
                  >
                    <IconTrash />
                  </button>
                </div>
              ))}
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
        <p className="page-desc">每个应用独立管理笔记与资料库，可新建或删除。</p>
        <p className="section-label">全部应用 · {apps.length}</p>

        {apps.map((app) => (
          <button
            key={app.id}
            type="button"
            className="card tap app-card"
            onClick={() => {
              setDetailId(app.id);
              setTab("notes");
            }}
          >
            <span className="app-badge">{app.title.slice(0, 1)}</span>
            <span className="app-main">
              <strong>{app.title}</strong>
              <span className="muted">
                笔记 {app.diaries.length} · 资料 {app.materials.length}
              </span>
            </span>
            <span className="chevron">›</span>
          </button>
        ))}
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
        <p className="hint">新应用固定包含「笔记」和「资料库」两个板块。</p>
      </BottomSheet>
    </>
  );
}
