import { useCallback, useEffect, useState } from "react";
import { api } from "../api";
import type { Optimization } from "../types";

interface Props {
  notify: (message: string) => void;
}

const CATEGORIES = ["资料归纳整理", "信息搜集整理", "外接模型更新", "回答融合"];

export function OptimizationsPanel({ notify }: Props) {
  const [items, setItems] = useState<Optimization[]>([]);
  const [version, setVersion] = useState("");
  const [category, setCategory] = useState(CATEGORIES[0]);
  const [detail, setDetail] = useState("");

  const load = useCallback(async () => {
    try {
      const data = await api.optimizations();
      setItems(data.items);
    } catch (err) {
      notify(err instanceof Error ? err.message : "读取优化记录失败");
    }
  }, [notify]);

  useEffect(() => {
    void load();
  }, [load]);

  const submit = async () => {
    if (!version.trim() || !detail.trim()) {
      notify("请填写版本号与优化内容。");
      return;
    }
    try {
      await api.addOptimization({ version: version.trim(), category, detail: detail.trim() });
      setVersion("");
      setDetail("");
      notify("已记录优化。");
      await load();
    } catch (err) {
      notify(err instanceof Error ? err.message : "记录失败");
    }
  };

  return (
    <section>
      <div className="card">
        <h3>记录一次自主优化</h3>
        <div className="grid">
          <label>
            版本号
            <input value={version} onChange={(e) => setVersion(e.target.value)} placeholder="0.5.2" />
          </label>
          <label>
            分类
            <select value={category} onChange={(e) => setCategory(e.target.value)}>
              {CATEGORIES.map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
          </label>
          <label>
            优化内容
            <input value={detail} onChange={(e) => setDetail(e.target.value)} placeholder="提升了资料归纳的摘要质量" />
          </label>
        </div>
        <button onClick={() => void submit()}>记录</button>
      </div>

      <table className="table">
        <thead>
          <tr>
            <th>时间</th>
            <th>版本</th>
            <th>分类</th>
            <th>内容</th>
            <th>来源</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.id}>
              <td>{item.createdAt}</td>
              <td>{item.version}</td>
              <td>{item.category}</td>
              <td>{item.detail}</td>
              <td>{item.source === "external" ? "第三方写入" : "手动"}</td>
            </tr>
          ))}
          {items.length === 0 && (
            <tr>
              <td colSpan={5}>还没有优化记录。</td>
            </tr>
          )}
        </tbody>
      </table>

      <p className="hint">
        第三方软件可通过 POST /api/optimizations/external 写入优化记录（需携带 X-Optimization-Key）。
      </p>
    </section>
  );
}
