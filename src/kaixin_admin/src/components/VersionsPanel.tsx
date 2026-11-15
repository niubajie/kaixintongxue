import { useEffect, useState } from "react";
import { api } from "../api";
import type { Skill, VersionInfo } from "../types";

interface Props {
  skills: Skill[];
  reload: () => Promise<void>;
  notify: (message: string) => void;
}

export function VersionsPanel({ skills, reload, notify }: Props) {
  const [softwareId, setSoftwareId] = useState(() => String(skills[0]?.id ?? ""));
  const [version, setVersion] = useState("");
  const [notes, setNotes] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [history, setHistory] = useState<{ skill: Skill; versions: VersionInfo[] } | null>(null);

  const current = skills.find((item) => String(item.id) === softwareId);

  // 技能列表异步加载完成后，默认选中第一个技能。
  useEffect(() => {
    if (skills.length > 0 && !skills.some((item) => String(item.id) === softwareId)) {
      setSoftwareId(String(skills[0].id));
    }
  }, [skills, softwareId]);

  const submit = async (mode: "draft" | "publish") => {
    setError("");
    if (!version.trim() || !file) {
      setError("请填写版本号并选择安装包。");
      return;
    }
    if (!softwareId) {
      setError("请先选择技能。");
      return;
    }
    const form = new FormData();
    form.append("version", version.trim());
    form.append("notes", notes.trim());
    form.append("draft", mode === "draft" ? "true" : "false");
    form.append("file", file);
    setBusy(true);
    notify(mode === "draft" ? "正在保存草稿…" : "正在上传并做安全审查…");
    try {
      const data = await api.publish(Number(softwareId), form);
      notify(data.message);
      setVersion("");
      setNotes("");
      setFile(null);
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : "上传失败。");
    } finally {
      setBusy(false);
    }
  };

  const showHistory = async (skill: Skill) => {
    try {
      setHistory({ skill, versions: await api.history(skill.id) });
    } catch (err) {
      notify(err instanceof Error ? err.message : "读取历史失败。");
    }
  };

  return (
    <section className="stack">
      <div className="panel">
        <h3>上传新版本</h3>
        <div className="hint">默认按当前版本自动加一位；可以先存草稿，确认后再对外发布。</div>
        <div className="form-grid">
          <div className="row">
            <select
              className="grow"
              value={softwareId}
              onChange={(event) => setSoftwareId(event.target.value)}
            >
              {skills.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
            <input
              type="text"
              style={{ width: 180 }}
              placeholder="版本号，例如 0.2.0"
              value={version}
              onChange={(event) => setVersion(event.target.value)}
            />
            <button
              type="button"
              className="btn ghost small"
              disabled={!current}
              onClick={async () => {
                if (!current) {
                  return;
                }
                try {
                  const data = await api.nextVersion(current.id);
                  setVersion(data.version);
                  notify(`已自动填写版本号 ${data.version}`);
                } catch (err) {
                  notify(err instanceof Error ? err.message : "自动版本号失败。");
                }
              }}
            >
              自动版本号
            </button>
          </div>
          <input
            type="text"
            placeholder="升级说明（可选）"
            value={notes}
            onChange={(event) => setNotes(event.target.value)}
          />
          <div className="row">
            <input
              type="file"
              className="grow"
              onChange={(event) => setFile(event.target.files?.[0] ?? null)}
            />
            <button type="button" className="btn ghost" disabled={busy} onClick={() => submit("draft")}>
              存为草稿
            </button>
            <button type="button" className="btn" disabled={busy} onClick={() => submit("publish")}>
              上传并发布
            </button>
          </div>
        </div>
        {error && <div className="form-error">{error}</div>}
      </div>

      <div className="panel">
        <h3>版本列表</h3>
        <div className="hint">「当前」为对外下载版本；草稿需要确认后才会替换当前版本。</div>
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>技能</th>
                <th>当前版本</th>
                <th>上一版本</th>
                <th>安全审查</th>
                <th style={{ textAlign: "right" }}>操作</th>
              </tr>
            </thead>
            <tbody>
              {skills.length === 0 ? (
                <tr>
                  <td colSpan={5} className="empty">
                    暂无技能。
                  </td>
                </tr>
              ) : (
                skills.map((skill) => (
                  <tr key={skill.id}>
                    <td>
                      <strong>{skill.name}</strong>
                      <div className="mono">{skill.wingetId || "本地安装包"}</div>
                    </td>
                    <td>
                      {skill.current ? `v${skill.current.version}` : <span className="badge muted">未发布</span>}
                    </td>
                    <td>{skill.previous ? `v${skill.previous.version}` : "—"}</td>
                    <td>
                      <ScanBadge status={skill.current?.scanStatus ?? "pending"} />
                      {skill.current?.scanDetail && <div className="mono">{skill.current.scanDetail}</div>}
                    </td>
                    <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>
                      <button
                        type="button"
                        className="btn ghost small"
                        onClick={async () => {
                          try {
                            const data = await api.rescan(skill.id);
                            notify(data.message);
                            await reload();
                          } catch (err) {
                            notify(err instanceof Error ? err.message : "审查失败。");
                          }
                        }}
                      >
                        重新审查
                      </button>
                      <button type="button" className="btn ghost small" onClick={() => showHistory(skill)}>
                        历史
                      </button>
                      <button
                        type="button"
                        className="btn ghost small"
                        onClick={async () => {
                          if (!window.confirm("确认回滚到上一版本？客户端之后下载到的是上一版。")) {
                            return;
                          }
                          try {
                            const data = await api.rollback(skill.id);
                            notify(data.message);
                            await reload();
                          } catch (err) {
                            notify(err instanceof Error ? err.message : "回滚失败。");
                          }
                        }}
                      >
                        回滚
                      </button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {history && (
          <div style={{ marginTop: 18 }}>
            <h4 className="history-title">{history.skill.name} · 版本历史</h4>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>版本</th>
                    <th>发布时间</th>
                    <th>大小</th>
                    <th>审查</th>
                    <th>状态</th>
                    <th style={{ textAlign: "right" }}>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {history.versions.map((item) => (
                    <tr key={item.version}>
                      <td>
                        v{item.version}
                        {item.releaseNotes && <div className="mono">{item.releaseNotes}</div>}
                      </td>
                      <td>{item.publishedAt}</td>
                      <td>{formatSize(item.fileSize)}</td>
                      <td>
                        <ScanBadge status={item.scanStatus} />
                      </td>
                      <td>
                        {item.isCurrent ? (
                          <span className="badge ok">当前</span>
                        ) : item.status === "draft" ? (
                          <span className="badge warn">草稿</span>
                        ) : (
                          <span className="badge muted">历史</span>
                        )}
                      </td>
                      <td style={{ textAlign: "right" }}>
                        {item.status === "draft" && (
                          <button
                            type="button"
                            className="btn ghost small"
                            onClick={async () => {
                              try {
                                const data = await api.confirm(history.skill.id, item.version);
                                notify(data.message);
                                await reload();
                                await showHistory(history.skill);
                              } catch (err) {
                                notify(err instanceof Error ? err.message : "确认失败。");
                              }
                            }}
                          >
                            确认发布
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>
    </section>
  );
}

export function ScanBadge({ status }: { status: string }) {
  const map: Record<string, [string, string]> = {
    clean: ["ok", "审查通过"],
    infected: ["danger", "发现风险"],
    unavailable: ["warn", "未完成审查"],
    pending: ["muted", "待审查"]
  };
  const [kind, label] = map[status] ?? map.pending;
  return <span className={`badge ${kind}`}>{label}</span>;
}

export function formatSize(size: number): string {
  if (!size) {
    return "—";
  }
  if (size < 1024) {
    return `${size} B`;
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  return `${(size / 1024 / 1024).toFixed(1)} MB`;
}
