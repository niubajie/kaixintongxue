import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { listCategories, updateCategory } from "../api/admin";
import type { Category, CategoryPatch } from "../api/types";
import Modal from "../components/Modal";
import { EmptyState, ErrorState, Loading } from "../components/States";
import { toast } from "../components/Toast";

const LEVEL_LABEL: Record<Category["sensitiveLevel"], string> = {
  low: "低",
  medium: "中",
  high: "高",
  extreme: "极高"
};

function yuan(cents: number): string {
  return (cents / 100).toFixed(2);
}

function toCents(text: string): number | null {
  const n = Number(text);
  if (!Number.isFinite(n) || n < 0) return null;
  return Math.round(n * 100);
}

export default function CategoriesPage() {
  const qc = useQueryClient();
  const query = useQuery({ queryKey: ["admin-categories"], queryFn: listCategories });

  const [editing, setEditing] = useState<Category | null>(null);
  const [enabled, setEnabled] = useState(true);
  const [sortOrder, setSortOrder] = useState("0");
  const [contactYuan, setContactYuan] = useState("0");
  const [accessYuan, setAccessYuan] = useState("0");
  const [level, setLevel] = useState<Category["sensitiveLevel"]>("low");
  const [disclaimer, setDisclaimer] = useState("");

  const open = (c: Category) => {
    setEditing(c);
    setEnabled(c.enabled);
    setSortOrder(String(c.sortOrder));
    setContactYuan(yuan(c.contactPriceCents));
    setAccessYuan(yuan(c.accessPriceCents));
    setLevel(c.sensitiveLevel);
    setDisclaimer(c.disclaimerText);
  };

  const save = useMutation({
    mutationFn: (patch: CategoryPatch) => updateCategory(editing?.code ?? "", patch),
    onSuccess: () => {
      toast.success("分类已更新");
      void qc.invalidateQueries({ queryKey: ["admin-categories"] });
      setEditing(null);
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "保存失败")
  });

  const submit = () => {
    const contact = toCents(contactYuan);
    const access = toCents(accessYuan);
    const sort = Number(sortOrder);
    if (contact === null || access === null) {
      toast.error("价格必须是非负数字");
      return;
    }
    if (!Number.isInteger(sort) || sort < 0) {
      toast.error("排序必须是非负整数");
      return;
    }
    save.mutate({
      enabled,
      sortOrder: sort,
      contactPriceCents: contact,
      accessPriceCents: access,
      sensitiveLevel: level,
      disclaimerText: disclaimer
    });
  };

  const items = query.data?.items ?? [];

  return (
    <div className="page">
      <section className="panel">
        <div className="panel-head">
          <h2>分类字典</h2>
          <span className="muted">共 {items.length} 个分类 · 改价与启停会写入审计日志</span>
        </div>

        {query.isLoading && <Loading />}
        {query.isError && (
          <ErrorState message={(query.error as Error).message} onRetry={() => void query.refetch()} />
        )}
        {!query.isLoading && !query.isError && items.length === 0 && <EmptyState title="暂无分类" />}

        {items.length > 0 && (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th className="num">排序</th>
                  <th>分类</th>
                  <th>编码</th>
                  <th className="num">联系方式价（元）</th>
                  <th className="num">包期价（元）</th>
                  <th>敏感级</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {items.map((c) => (
                  <tr key={c.code}>
                    <td className="num">{c.sortOrder}</td>
                    <td className="cell-title">{c.name}</td>
                    <td>
                      <code>{c.code}</code>
                    </td>
                    <td className="num">{yuan(c.contactPriceCents)}</td>
                    <td className="num">{yuan(c.accessPriceCents)}</td>
                    <td>{LEVEL_LABEL[c.sensitiveLevel]}</td>
                    <td>
                      <span className={c.enabled ? "badge badge-success" : "badge badge-gray"}>
                        {c.enabled ? "启用" : "停用"}
                      </span>
                    </td>
                    <td>
                      <button className="btn btn-sm" onClick={() => open(c)}>
                        编辑
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        <p className="muted">
          金融类（finance）必须保留免责声明；人力资源类需公示资质申请中。停用分类后，其资源不再对外检索与展示。
        </p>
      </section>

      <Modal
        open={editing !== null}
        title={`编辑分类 · ${editing?.name ?? ""}`}
        onClose={() => setEditing(null)}
        footer={
          <div className="modal-actions">
            <button className="btn" onClick={() => setEditing(null)} disabled={save.isPending}>
              取消
            </button>
            <button className="btn btn-primary" onClick={submit} disabled={save.isPending}>
              保存
            </button>
          </div>
        }
      >
        {editing && (
          <div className="form-grid">
            <label className="field-inline">
              <span>启用</span>
              <input
                type="checkbox"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
              />
            </label>

            <label className="field">
              <span>排序（越小越靠前）</span>
              <input
                type="number"
                min={0}
                step={1}
                value={sortOrder}
                onChange={(e) => setSortOrder(e.target.value)}
              />
            </label>

            <label className="field">
              <span>联系方式价格（元）</span>
              <input
                type="number"
                min={0}
                step={0.01}
                value={contactYuan}
                onChange={(e) => setContactYuan(e.target.value)}
              />
            </label>

            <label className="field">
              <span>包期价格（元）</span>
              <input
                type="number"
                min={0}
                step={0.01}
                value={accessYuan}
                onChange={(e) => setAccessYuan(e.target.value)}
              />
            </label>

            <label className="field">
              <span>敏感级别</span>
              <select
                value={level}
                onChange={(e) => setLevel(e.target.value as Category["sensitiveLevel"])}
              >
                {Object.keys(LEVEL_LABEL).map((k) => (
                  <option key={k} value={k}>
                    {LEVEL_LABEL[k as Category["sensitiveLevel"]]}
                  </option>
                ))}
              </select>
            </label>

            <label className="field">
              <span>免责声明 / 资质说明</span>
              <textarea
                rows={3}
                value={disclaimer}
                onChange={(e) => setDisclaimer(e.target.value)}
                placeholder="如：本平台仅发布企业信息，不提供投融资/借贷中介服务"
              />
            </label>
          </div>
        )}
      </Modal>
    </div>
  );
}
