import { useState } from "react";
import { api } from "../api";
import type { Skill, SkillInput } from "../types";

interface Props {
  skills: Skill[];
  reload: () => Promise<void>;
  notify: (message: string) => void;
}

const EMPTY: SkillInput = {
  name: "",
  wingetId: "",
  purpose: "",
  summary: "",
  aiSoftware: "",
  notes: "",
  required: false
};

export function SkillsPanel({ skills, reload, notify }: Props) {
  const [editing, setEditing] = useState<Skill | null>(null);
  const [open, setOpen] = useState(false);

  const remove = async (skill: Skill) => {
    if (!window.confirm(`删除技能「${skill.name}」及其版本记录？`)) {
      return;
    }
    try {
      const data = await api.deleteSkill(skill.id);
      notify(data.message);
      await reload();
    } catch (error) {
      notify(error instanceof Error ? error.message : "删除失败。");
    }
  };

  return (
    <section>
      <div className="toolbar">
        <button
          type="button"
          className="btn"
          onClick={() => {
            setEditing(null);
            setOpen(true);
          }}
        >
          新增技能
        </button>
        <span className="mono">技能说明与 AI 软件两段描述会展示在客户端技能列表</span>
      </div>

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th style={{ width: 180 }}>技能名称</th>
              <th>说明</th>
              <th>AI 软件</th>
              <th>winget Id</th>
              <th>必装</th>
              <th style={{ textAlign: "right" }}>操作</th>
            </tr>
          </thead>
          <tbody>
            {skills.length === 0 ? (
              <tr>
                <td colSpan={6} className="empty">
                  暂无技能，点「新增技能」登记。
                </td>
              </tr>
            ) : (
              skills.map((skill) => (
                <tr key={skill.id}>
                  <td>
                    <strong>{skill.name}</strong>
                  </td>
                  <td>
                    <div className="desc">{skill.summary || skill.purpose || "—"}</div>
                  </td>
                  <td>
                    <div className="desc">{skill.aiSoftware || "—"}</div>
                  </td>
                  <td className="mono">{skill.wingetId || "—"}</td>
                  <td>
                    {skill.required ? (
                      <span className="badge ok">必装</span>
                    ) : (
                      <span className="badge muted">选装</span>
                    )}
                  </td>
                  <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>
                    <button
                      type="button"
                      className="btn ghost small"
                      onClick={() => {
                        setEditing(skill);
                        setOpen(true);
                      }}
                    >
                      修改
                    </button>
                    <button type="button" className="btn ghost small danger-text" onClick={() => remove(skill)}>
                      删除
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {open && (
        <SkillDialog
          skill={editing}
          onClose={() => setOpen(false)}
          notify={notify}
          onSaved={async () => {
            setOpen(false);
            await reload();
          }}
        />
      )}
    </section>
  );
}

interface DialogProps {
  skill: Skill | null;
  notify: (message: string) => void;
  onClose: () => void;
  onSaved: () => Promise<void>;
}

function SkillDialog({ skill, notify, onClose, onSaved }: DialogProps) {
  const [form, setForm] = useState<SkillInput>(
    skill
      ? {
          name: skill.name,
          wingetId: skill.wingetId,
          purpose: skill.purpose,
          summary: skill.summary,
          aiSoftware: skill.aiSoftware,
          notes: skill.notes,
          required: skill.required
        }
      : EMPTY
  );
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  const update = (patch: Partial<SkillInput>) => setForm((current) => ({ ...current, ...patch }));

  const save = async () => {
    if (!form.name.trim()) {
      setError("请填写技能名称。");
      return;
    }
    setSaving(true);
    try {
      const data = skill ? await api.updateSkill(skill.id, form) : await api.createSkill(form);
      notify(data.message);
      await onSaved();
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存失败。");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="modal-mask" onClick={onClose}>
      <div className="modal" onClick={(event) => event.stopPropagation()}>
        <h3>{skill ? "修改技能" : "新增技能"}</h3>
        <div className="hint">两段描述分别用于客户端「说明」与「AI 软件」区域。</div>

        <div className="field">
          <label>技能名称</label>
          <input
            type="text"
            value={form.name}
            placeholder="例如：电脑维修"
            onChange={(event) => update({ name: event.target.value })}
          />
        </div>
        <div className="field">
          <label>说明</label>
          <textarea
            rows={3}
            value={form.summary}
            placeholder="这个技能解决什么问题"
            onChange={(event) => update({ summary: event.target.value })}
          />
        </div>
        <div className="field">
          <label>AI 软件</label>
          <textarea
            rows={3}
            value={form.aiSoftware}
            placeholder="需要配合的 AI 软件与能力"
            onChange={(event) => update({ aiSoftware: event.target.value })}
          />
        </div>
        <div className="field">
          <label>winget Id（可选）</label>
          <input
            type="text"
            value={form.wingetId}
            placeholder="例如：Git.Git"
            onChange={(event) => update({ wingetId: event.target.value })}
          />
        </div>
        <div className="field">
          <label>用途一句话</label>
          <input
            type="text"
            value={form.purpose}
            placeholder="例如：技能代码与资料版本管理"
            onChange={(event) => update({ purpose: event.target.value })}
          />
        </div>
        <label className="check-line">
          <input
            type="checkbox"
            checked={form.required}
            onChange={(event) => update({ required: event.target.checked })}
          />
          必装
        </label>

        {error && <div className="form-error">{error}</div>}

        <div className="modal-actions">
          <button type="button" className="btn ghost" onClick={onClose}>
            取消
          </button>
          <button type="button" className="btn" disabled={saving} onClick={save}>
            {saving ? "保存中…" : "保存"}
          </button>
        </div>
      </div>
    </div>
  );
}
