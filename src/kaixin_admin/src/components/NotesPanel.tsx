import { useCallback, useEffect, useState } from "react";
import { api } from "../api";
import type { VersionNote } from "../types";

interface Props {
  notify: (message: string) => void;
}

export function NotesPanel({ notify }: Props) {
  const [version, setVersion] = useState("");
  const [title, setTitle] = useState("");
  const [detail, setDetail] = useState("");
  const [error, setError] = useState("");
  const [notes, setNotes] = useState<VersionNote[]>([]);

  const load = useCallback(async () => {
    try {
      setNotes(await api.notes());
    } catch (err) {
      setError(err instanceof Error ? err.message : "读取说明失败。");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <section className="stack">
      <div className="panel">
        <h3>发布版本升级说明</h3>
        <div className="hint">会显示在官网首页的「版本升级说明」中。</div>
        <div className="form-grid">
          <div className="row">
            <input
              type="text"
              style={{ width: 160 }}
              placeholder="版本号"
              value={version}
              onChange={(event) => setVersion(event.target.value)}
            />
            <input
              type="text"
              className="grow"
              placeholder="标题，例如：部署流程优化"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
            />
          </div>
          <input
            type="text"
            placeholder="说明内容"
            value={detail}
            onChange={(event) => setDetail(event.target.value)}
          />
          <div className="row">
            <button
              type="button"
              className="btn ghost"
              onClick={async () => {
                setError("");
                try {
                  const data = await api.addNote({ version: version.trim(), title: title.trim(), detail: detail.trim() });
                  notify(data.message);
                  setVersion("");
                  setTitle("");
                  setDetail("");
                  await load();
                } catch (err) {
                  setError(err instanceof Error ? err.message : "发布失败。");
                }
              }}
            >
              发布说明
            </button>
          </div>
        </div>
        {error && <div className="form-error">{error}</div>}
      </div>

      <div className="panel">
        <h3>已发布说明</h3>
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>版本</th>
                <th>标题</th>
                <th>内容</th>
                <th>发布时间</th>
              </tr>
            </thead>
            <tbody>
              {notes.length === 0 ? (
                <tr>
                  <td colSpan={4} className="empty">
                    暂无说明。
                  </td>
                </tr>
              ) : (
                notes.map((note) => (
                  <tr key={`${note.version}-${note.publishedAt}`}>
                    <td>v{note.version}</td>
                    <td>
                      <strong>{note.title}</strong>
                    </td>
                    <td>
                      <div className="desc">{note.detail || "—"}</div>
                    </td>
                    <td className="mono">{note.publishedAt}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  );
}
