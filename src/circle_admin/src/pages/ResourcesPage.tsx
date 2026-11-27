import { useMemo, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";

import { listCategories, listResources, reviewResource } from "../api/admin";
import type { ReviewAction } from "../api/admin";
import type { Resource, ResourceStatus } from "../api/types";
import { STATUS_LABEL } from "../api/types";
import Modal from "../components/Modal";
import Pagination from "../components/Pagination";
import { EmptyState, ErrorState, Loading, StatusBadge } from "../components/States";
import { toast } from "../components/Toast";

const PAGE_SIZE = 20;

function fmtTime(v: string | null): string {
  if (!v) return "—";
  return new Date(v).toLocaleString("zh-CN", { hour12: false });
}

export default function ResourcesPage() {
  const [searchParams] = useSearchParams();
  const [status, setStatus] = useState<string>(searchParams.get("status") ?? "");
  const [category, setCategory] = useState("");
  const [keyword, setKeyword] = useState("");
  const [keywordInput, setKeywordInput] = useState("");
  const [page, setPage] = useState(1);

  const [selected, setSelected] = useState<Resource | null>(null);
  const [rejecting, setRejecting] = useState(false);
  const [reason, setReason] = useState("");

  const qc = useQueryClient();

  const categoriesQuery = useQuery({ queryKey: ["admin-categories"], queryFn: listCategories });
  const listQuery = useQuery({
    queryKey: ["admin-resources", { status, category, keyword, page }],
    queryFn: () => listResources({ status, category, keyword, page, size: PAGE_SIZE }),
    placeholderData: keepPreviousData
  });

  const categoryNames = useMemo(() => {
    const map: Record<string, string> = {};
    for (const c of categoriesQuery.data?.items ?? []) map[c.code] = c.name;
    return map;
  }, [categoriesQuery.data]);

  const review = useMutation({
    mutationFn: (v: { uuid: string; action: ReviewAction; reason?: string }) =>
      reviewResource(v.uuid, v.action, v.reason),
    onSuccess: (res) => {
      toast.success(`操作成功，当前状态：${STATUS_LABEL[res.status]}`);
      void qc.invalidateQueries({ queryKey: ["admin-resources"] });
      void qc.invalidateQueries({ queryKey: ["stat"] });
      setSelected(null);
      setRejecting(false);
      setReason("");
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "操作失败")
  });

  const applyKeyword = () => {
    setKeyword(keywordInput.trim());
    setPage(1);
  };

  const openDetail = (r: Resource) => {
    setSelected(r);
    setRejecting(false);
    setReason("");
  };

  const items = listQuery.data?.items ?? [];
  const total = listQuery.data?.total ?? 0;

  return (
    <div className="page">
      <section className="panel">
        <div className="filters">
          <label className="field-inline">
            <span>状态</span>
            <select
              value={status}
              onChange={(e) => {
                setStatus(e.target.value);
                setPage(1);
              }}
            >
              <option value="">全部</option>
              {(Object.keys(STATUS_LABEL) as ResourceStatus[]).map((s) => (
                <option key={s} value={s}>
                  {STATUS_LABEL[s]}
                </option>
              ))}
            </select>
          </label>

          <label className="field-inline">
            <span>分类</span>
            <select
              value={category}
              onChange={(e) => {
                setCategory(e.target.value);
                setPage(1);
              }}
            >
              <option value="">全部</option>
              {(categoriesQuery.data?.items ?? []).map((c) => (
                <option key={c.code} value={c.code}>
                  {c.name}
                </option>
              ))}
            </select>
          </label>

          <label className="field-inline grow">
            <span>关键词</span>
            <input
              value={keywordInput}
              onChange={(e) => setKeywordInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") applyKeyword();
              }}
              placeholder="标题或摘要"
            />
          </label>

          <button className="btn btn-primary" onClick={applyKeyword}>
            查询
          </button>
        </div>

        {listQuery.isLoading && <Loading />}
        {listQuery.isError && (
          <ErrorState
            message={(listQuery.error as Error).message}
            onRetry={() => void listQuery.refetch()}
          />
        )}
        {!listQuery.isLoading && !listQuery.isError && items.length === 0 && (
          <EmptyState title="暂无资源" desc="调整筛选条件后再试" />
        )}

        {items.length > 0 && (
          <>
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>标题</th>
                    <th>分类</th>
                    <th>地区</th>
                    <th className="num">质量分</th>
                    <th>状态</th>
                    <th>创建时间</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((r) => (
                    <tr key={r.uuid}>
                      <td className="cell-title">{r.title}</td>
                      <td>{categoryNames[r.categoryCode] ?? r.categoryCode}</td>
                      <td>{r.regionCode || "—"}</td>
                      <td className="num">{r.qualityScore}</td>
                      <td>
                        <StatusBadge status={r.status} />
                      </td>
                      <td>{fmtTime(r.createdAt)}</td>
                      <td>
                        <button className="btn btn-sm" onClick={() => openDetail(r)}>
                          查看
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <Pagination page={page} size={PAGE_SIZE} total={total} onChange={setPage} />
          </>
        )}
      </section>

      <Modal
        open={selected !== null}
        title="资源详情"
        onClose={() => setSelected(null)}
        footer={
          selected && (
            <div className="modal-actions">
              {selected.status === "pending" && !rejecting && (
                <>
                  <button
                    className="btn"
                    onClick={() => setRejecting(true)}
                    disabled={review.isPending}
                  >
                    拒绝
                  </button>
                  <button
                    className="btn btn-primary"
                    onClick={() => review.mutate({ uuid: selected.uuid, action: "approve" })}
                    disabled={review.isPending}
                  >
                    通过并上线
                  </button>
                </>
              )}
              {selected.status === "pending" && rejecting && (
                <>
                  <button className="btn" onClick={() => setRejecting(false)} disabled={review.isPending}>
                    取消
                  </button>
                  <button
                    className="btn btn-danger"
                    disabled={review.isPending || reason.trim() === ""}
                    onClick={() =>
                      review.mutate({ uuid: selected.uuid, action: "reject", reason: reason.trim() })
                    }
                  >
                    确认拒绝
                  </button>
                </>
              )}
              {selected.status === "online" && (
                <button
                  className="btn btn-danger"
                  disabled={review.isPending}
                  onClick={() => review.mutate({ uuid: selected.uuid, action: "offline" })}
                >
                  下架
                </button>
              )}
            </div>
          )
        }
      >
        {selected && (
          <div className="detail">
            <h3>{selected.title}</h3>
            <div className="detail-meta">
              <StatusBadge status={selected.status} />
              <span>分类：{categoryNames[selected.categoryCode] ?? selected.categoryCode}</span>
              <span>地区：{selected.regionCode || "—"}</span>
              <span>质量分：{selected.qualityScore}</span>
              <span>提供方：{selected.providerType}</span>
            </div>
            <div className="detail-section">
              <div className="detail-label">摘要</div>
              <p>{selected.summary || "—"}</p>
            </div>
            <div className="detail-section">
              <div className="detail-label">详情</div>
              <p className="pre-wrap">{selected.detailMd || "—"}</p>
            </div>
            <div className="detail-grid">
              <span>资源 UUID：{selected.uuid}</span>
              <span>提交时间：{fmtTime(selected.createdAt)}</span>
              <span>上线时间：{fmtTime(selected.publishedAt)}</span>
              <span>到期时间：{fmtTime(selected.expiresAt)}</span>
            </div>
            {selected.rejectReason && (
              <div className="detail-section">
                <div className="detail-label">拒绝原因</div>
                <p>{selected.rejectReason}</p>
              </div>
            )}
            {rejecting && (
              <label className="field">
                <span>拒绝原因（必填）</span>
                <textarea
                  rows={3}
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  placeholder="请说明拒绝原因，将展示给提交人"
                />
              </label>
            )}
            <p className="muted">
              联系方式为独立加密表存储，后台列表与详情不展示明文；解密需走 S4 的 contact-vault 并写审计。
            </p>
          </div>
        )}
      </Modal>
    </div>
  );
}
