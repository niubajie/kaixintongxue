import { useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { listUsers, updateUserStatus } from "../api/admin";
import type { AdminUser } from "../api/types";
import { RISK_LEVEL_LABEL, USER_STATUS_LABEL } from "../api/types";
import Modal from "../components/Modal";
import Pagination from "../components/Pagination";
import { EmptyState, ErrorState, Loading } from "../components/States";
import { toast } from "../components/Toast";

const PAGE_SIZE = 20;

const STATUS_CLASS: Record<string, string> = {
  active: "badge-success",
  blocked: "badge-danger",
  pending_delete: "badge-gray"
};

function fmtTime(v: string | null): string {
  if (!v) return "—";
  return new Date(v).toLocaleString("zh-CN", { hour12: false });
}

export default function UsersPage() {
  const qc = useQueryClient();
  const [keywordInput, setKeywordInput] = useState("");
  const [keyword, setKeyword] = useState("");
  const [status, setStatus] = useState("");
  const [riskLevel, setRiskLevel] = useState("");
  const [page, setPage] = useState(1);
  const [confirming, setConfirming] = useState<AdminUser | null>(null);

  const query = useQuery({
    queryKey: ["admin-users", { keyword, status, riskLevel, page }],
    queryFn: () => listUsers({ keyword, status, riskLevel, page, size: PAGE_SIZE }),
    placeholderData: keepPreviousData
  });

  const nextStatus: "active" | "blocked" = confirming?.status === "blocked" ? "active" : "blocked";

  const mutation = useMutation({
    mutationFn: (v: { uuid: string; status: "active" | "blocked" }) =>
      updateUserStatus(v.uuid, v.status),
    onSuccess: (u) => {
      toast.success(u.status === "blocked" ? "用户已封禁" : "用户已解封");
      void qc.invalidateQueries({ queryKey: ["admin-users"] });
      setConfirming(null);
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "操作失败")
  });

  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;

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
              {(Object.keys(USER_STATUS_LABEL) as Array<keyof typeof USER_STATUS_LABEL>).map((s) => (
                <option key={s} value={s}>
                  {USER_STATUS_LABEL[s]}
                </option>
              ))}
            </select>
          </label>

          <label className="field-inline">
            <span>风险等级</span>
            <select
              value={riskLevel}
              onChange={(e) => {
                setRiskLevel(e.target.value);
                setPage(1);
              }}
            >
              <option value="">全部</option>
              {(Object.keys(RISK_LEVEL_LABEL) as Array<keyof typeof RISK_LEVEL_LABEL>).map((k) => (
                <option key={k} value={k}>
                  {RISK_LEVEL_LABEL[k]}
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
                if (e.key === "Enter") {
                  setKeyword(keywordInput.trim());
                  setPage(1);
                }
              }}
              placeholder="昵称或 UUID"
            />
          </label>

          <button
            className="btn btn-primary"
            onClick={() => {
              setKeyword(keywordInput.trim());
              setPage(1);
            }}
          >
            查询
          </button>
        </div>

        {query.isLoading && <Loading />}
        {query.isError && (
          <ErrorState message={(query.error as Error).message} onRetry={() => void query.refetch()} />
        )}
        {!query.isLoading && !query.isError && items.length === 0 && (
          <EmptyState title="暂无用户" desc="调整筛选条件后再试" />
        )}

        {items.length > 0 && (
          <>
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>昵称</th>
                    <th>UUID</th>
                    <th>状态</th>
                    <th>风险</th>
                    <th>手机绑定</th>
                    <th className="num">资源数</th>
                    <th>最近登录</th>
                    <th>注册时间</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((u) => (
                    <tr key={u.uuid}>
                      <td className="cell-title">{u.nickname || "—"}</td>
                      <td>
                        <code>{u.uuid.slice(0, 8)}…</code>
                      </td>
                      <td>
                        <span className={`badge ${STATUS_CLASS[u.status] ?? "badge-gray"}`}>
                          {USER_STATUS_LABEL[u.status]}
                        </span>
                      </td>
                      <td>{RISK_LEVEL_LABEL[u.riskLevel]}</td>
                      <td>{u.phoneBound ? "已绑定" : "未绑定"}</td>
                      <td className="num">{u.resourceCount}</td>
                      <td>{fmtTime(u.lastLoginAt)}</td>
                      <td>{fmtTime(u.createdAt)}</td>
                      <td>
                        <button
                          className={u.status === "blocked" ? "btn btn-sm" : "btn btn-sm btn-danger"}
                          onClick={() => setConfirming(u)}
                        >
                          {u.status === "blocked" ? "解封" : "封禁"}
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

        <p className="muted">
          封禁立即生效，用户后续请求会被拒绝；手机号等敏感信息不在后台展示，解密需走单独授权与审计。
        </p>
      </section>

      <Modal
        open={confirming !== null}
        title={nextStatus === "blocked" ? "确认封禁用户" : "确认解封用户"}
        onClose={() => setConfirming(null)}
        footer={
          <div className="modal-actions">
            <button className="btn" onClick={() => setConfirming(null)} disabled={mutation.isPending}>
              取消
            </button>
            <button
              className={nextStatus === "blocked" ? "btn btn-danger" : "btn btn-primary"}
              disabled={mutation.isPending}
              onClick={() =>
                confirming && mutation.mutate({ uuid: confirming.uuid, status: nextStatus })
              }
            >
              {nextStatus === "blocked" ? "确认封禁" : "确认解封"}
            </button>
          </div>
        }
      >
        {confirming && (
          <div className="detail">
            <p>
              用户：<strong>{confirming.nickname || confirming.uuid.slice(0, 8)}</strong>
            </p>
            <p className="muted">
              当前状态：{USER_STATUS_LABEL[confirming.status]} · 资源数 {confirming.resourceCount}
            </p>
            <p>
              {nextStatus === "blocked"
                ? "封禁后该用户无法登录与提交资源，已上线资源不受影响，可随时解封。"
                : "解封后该用户恢复正常使用。"}
            </p>
          </div>
        )}
      </Modal>
    </div>
  );
}
