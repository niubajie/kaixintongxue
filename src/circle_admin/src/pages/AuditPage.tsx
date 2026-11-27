import { useState } from "react";
import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";

import { listAuditLogs, verifyAuditChain } from "../api/admin";
import type { AuditLog } from "../api/types";
import Pagination from "../components/Pagination";
import { EmptyState, ErrorState, Loading } from "../components/States";
import { toast } from "../components/Toast";

const PAGE_SIZE = 20;

const ACTOR_LABEL: Record<string, string> = {
  user: "用户",
  admin: "管理员",
  system: "系统"
};

function fmtTime(v: string): string {
  return new Date(v).toLocaleString("zh-CN", { hour12: false });
}

function brief(log: AuditLog): string {
  if (log.detail === null || log.detail === undefined) return "—";
  const text = JSON.stringify(log.detail);
  return text.length > 80 ? `${text.slice(0, 80)}…` : text;
}

export default function AuditPage() {
  const [actorType, setActorType] = useState("");
  const [actionInput, setActionInput] = useState("");
  const [action, setAction] = useState("");
  const [keywordInput, setKeywordInput] = useState("");
  const [keyword, setKeyword] = useState("");
  const [page, setPage] = useState(1);

  const query = useQuery({
    queryKey: ["admin-audit-logs", { actorType, action, keyword, page }],
    queryFn: () => listAuditLogs({ actorType, action, keyword, page, size: PAGE_SIZE }),
    placeholderData: keepPreviousData
  });

  const verify = useMutation({
    mutationFn: verifyAuditChain,
    onSuccess: (r) =>
      r.ok
        ? toast.success(`哈希链完整，已校验 ${r.checked} 条`)
        : toast.error(`哈希链断裂，首个异常记录 id=${r.brokenAt ?? "?"}`),
    onError: (e) => toast.error(e instanceof Error ? e.message : "校验失败")
  });

  const apply = () => {
    setAction(actionInput.trim());
    setKeyword(keywordInput.trim());
    setPage(1);
  };

  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;

  return (
    <div className="page">
      <section className="panel">
        <div className="filters">
          <label className="field-inline">
            <span>操作人类型</span>
            <select
              value={actorType}
              onChange={(e) => {
                setActorType(e.target.value);
                setPage(1);
              }}
            >
              <option value="">全部</option>
              {Object.keys(ACTOR_LABEL).map((k) => (
                <option key={k} value={k}>
                  {ACTOR_LABEL[k]}
                </option>
              ))}
            </select>
          </label>

          <label className="field-inline">
            <span>动作</span>
            <input
              value={actionInput}
              onChange={(e) => setActionInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") apply();
              }}
              placeholder="如 RESOURCE_REVIEW"
            />
          </label>

          <label className="field-inline grow">
            <span>关键词</span>
            <input
              value={keywordInput}
              onChange={(e) => setKeywordInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") apply();
              }}
              placeholder="操作人 / 对象 / traceId"
            />
          </label>

          <button className="btn btn-primary" onClick={apply}>
            查询
          </button>
          <button
            className="btn"
            disabled={verify.isPending}
            onClick={() => verify.mutate()}
          >
            {verify.isPending ? "校验中…" : "校验哈希链"}
          </button>
        </div>

        {verify.data && (
          <div className={verify.data.ok ? "state-box" : "state-box state-error"}>
            {verify.data.ok
              ? `哈希链完整，已校验 ${verify.data.checked} 条记录。`
              : `哈希链断裂，首个异常记录 id=${verify.data.brokenAt ?? "?"}，请立即排查。`}
          </div>
        )}

        {query.isLoading && <Loading />}
        {query.isError && (
          <ErrorState message={(query.error as Error).message} onRetry={() => void query.refetch()} />
        )}
        {!query.isLoading && !query.isError && items.length === 0 && (
          <EmptyState title="暂无审计记录" desc="调整筛选条件后再试" />
        )}

        {items.length > 0 && (
          <>
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th className="num">ID</th>
                    <th>时间</th>
                    <th>操作人</th>
                    <th>动作</th>
                    <th>对象</th>
                    <th>traceId</th>
                    <th>详情</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((log) => (
                    <tr key={log.id}>
                      <td className="num">{log.id}</td>
                      <td>{fmtTime(log.createdAt)}</td>
                      <td>
                        {ACTOR_LABEL[log.actorType] ?? log.actorType} · {log.actorId}
                      </td>
                      <td>
                        <code>{log.action}</code>
                      </td>
                      <td>
                        {log.targetType} · {log.targetId.slice(0, 8)}
                      </td>
                      <td>
                        <code>{log.traceId ? `${log.traceId.slice(0, 8)}…` : "—"}</code>
                      </td>
                      <td>
                        <span className="muted" title={JSON.stringify(log.detail)}>
                          {brief(log)}
                        </span>
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
          审计日志只追加、不提供删除接口；每条记录包含 prev_hash 与 hash，可校验是否被篡改。
        </p>
      </section>
    </div>
  );
}
