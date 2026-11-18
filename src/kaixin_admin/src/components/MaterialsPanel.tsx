import { useCallback, useEffect, useState } from "react";
import { api } from "../api";
import type { MaterialProject } from "../types";

interface Props {
  notify: (message: string) => void;
}

const REVIEW_LABEL: Record<string, string> = {
  pending: "待评审",
  approved: "已通过",
  rejected: "已拒绝"
};

export function MaterialsPanel({ notify }: Props) {
  const [items, setItems] = useState<MaterialProject[]>([]);
  const [error, setError] = useState("");
  const [title, setTitle] = useState("");
  const [parentId, setParentId] = useState("");
  const [keywords, setKeywords] = useState("");
  const [sites, setSites] = useState("");
  const [schedule, setSchedule] = useState("");
  const [note, setNote] = useState("");

  const load = useCallback(async () => {
    try {
      const data = await api.materials();
      setItems(data.items);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "读取资料项目失败。");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const submit = async () => {
    if (!title.trim()) {
      notify("请填写资料项目名称。");
      return;
    }
    try {
      await api.saveMaterial({
        title: title.trim(),
        parentId: parentId ? Number(parentId) : null,
        keywords,
        sites,
        schedule,
        note,
        review: "pending"
      });
      setTitle("");
      setKeywords("");
      setSites("");
      setSchedule("");
      setNote("");
      setParentId("");
      notify("已新增资料项目（待评审）。");
      await load();
    } catch (err) {
      notify(err instanceof Error ? err.message : "保存失败");
    }
  };

  const setReview = async (item: MaterialProject, review: MaterialProject["review"]) => {
    try {
      await api.saveMaterial({ ...item, review });
      notify(review === "approved" ? "已通过评审。" : "已拒绝。");
      await load();
    } catch (err) {
      notify(err instanceof Error ? err.message : "更新评审状态失败");
    }
  };

  const runNow = async (item: MaterialProject) => {
    try {
      await api.runMaterial(item.id);
      notify("已记录一次检索任务。");
      await load();
    } catch (err) {
      notify(err instanceof Error ? err.message : "触发失败");
    }
  };

  const parents = items.filter((item) => !item.parentId);

  return (
    <section>
      <div className="card">
        <h3>新增资料项目</h3>
        <div className="grid">
          <label>
            名称
            <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="例如：主板维修资料" />
          </label>
          <label>
            上级项目（留空为顶级）
            <select value={parentId} onChange={(e) => setParentId(e.target.value)}>
              <option value="">（顶级项目）</option>
              {parents.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.title}
                </option>
              ))}
            </select>
          </label>
          <label>
            关键词（逗号分隔）
            <input value={keywords} onChange={(e) => setKeywords(e.target.value)} placeholder="主板, 故障码, 维修" />
          </label>
          <label>
            指定网站（逗号分隔）
            <input value={sites} onChange={(e) => setSites(e.target.value)} placeholder="bilibili.com, zhihu.com" />
          </label>
          <label>
            定时计划
            <input value={schedule} onChange={(e) => setSchedule(e.target.value)} placeholder="例如：每天 08:00" />
          </label>
          <label>
            备注
            <input value={note} onChange={(e) => setNote(e.target.value)} />
          </label>
        </div>
        <button onClick={() => void submit()}>保存</button>
      </div>

      {error && <p className="error">{error}</p>}

      <table className="table">
        <thead>
          <tr>
            <th>名称</th>
            <th>关键词</th>
            <th>站点</th>
            <th>定时</th>
            <th>评审</th>
            <th>检索次数</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.id}>
              <td>
                {item.parentId ? "└ " : ""}
                {item.title}
              </td>
              <td>{item.keywords || "—"}</td>
              <td>{item.sites || "—"}</td>
              <td>{item.schedule || "—"}</td>
              <td>
                <span className={`tag ${item.review}`}>{REVIEW_LABEL[item.review] ?? item.review}</span>
              </td>
              <td>{item.runs}</td>
              <td className="row-actions">
                <button onClick={() => void setReview(item, "approved")}>通过</button>
                <button onClick={() => void setReview(item, "rejected")}>拒绝</button>
                <button onClick={() => void runNow(item)}>立即检索</button>
                <button
                  className="danger"
                  onClick={async () => {
                    if (!window.confirm(`删除「${item.title}」及其子项目？`)) {
                      return;
                    }
                    await api.deleteMaterial(item.id);
                    notify("已删除。");
                    await load();
                  }}
                >
                  删除
                </button>
              </td>
            </tr>
          ))}
          {items.length === 0 && (
            <tr>
              <td colSpan={7}>还没有资料项目。</td>
            </tr>
          )}
        </tbody>
      </table>

      <p className="hint">
        定时计划只负责「到点记录一次检索任务」；抓取结果需由外部写入后再进入评审，
        通过评审的资料会带「待确认」标志进入客户端目录。
      </p>
    </section>
  );
}
