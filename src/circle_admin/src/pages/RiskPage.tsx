import { useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { listRiskEvents } from "../api/admin";
import { SEVERITY_LABEL } from "../api/types";
import type { RiskSeverity } from "../api/types";
import Pagination from "../components/Pagination";
import { EmptyState, ErrorState, Loading } from "../components/States";

const PAGE_SIZE = 20;

const SEVERITY_CLASS: Record<RiskSeverity, string> = {
  low: "badge-gray",
  medium: "badge-warning",
  high: "badge-warning",
  critical: "badge-danger"
};

function fmtTime(v: string): string {
  return new Date(v).toLocaleString("zh-CN", { hour12: false });
}

function brief(detail: unknown): string {
  if (detail === null || detail === undefined) return "—";
  const text = JSON.stringify(detail);
  return text.length > 90 ? `${text.slice(0, 90)}…` : text;
}

export default function RiskPage() {
  const [severity, setSeverity] = useState("");
  const [eventType, setEventType] = useState("");
  const [eventTypeInput, setEventTypeInput] = useState("");
  const [page, setPage] = useState(1);

  const query = useQuery({
    queryKey: ["admin-risk-events", { severity, eventType, page }],
    queryFn: () => listRiskEvents({ severity, eventType, page, size: PAGE_SIZE }),
    placeholderData: keepPreviousData
  });

  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;

  return (
    <div className="page">
      <section className="panel">
        <div className="filters">
          <label className="field-inline">
            <span>级别</span>
            <select
              value={severity}
              onChange={(e) => {
                setSeverity(e.target.value);
                setPage(1);
              }}
            >
              <option value="">全部</option>
              {(Object.keys(SEVERITY_LABEL) as RiskSeverity[]).map((s) => (
                <option key={s} value={s}>
                  {SEVERITY_LABEL[s]}
                </option>
              ))}
            </select>
          </label>

          <label className="field-inline grow">
            <span>事件类型</span>
            <input
              value={eventTypeInput}
              onChange={(e) => setEventTypeInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  setEventType(eventTypeInput.trim());
                  setPage(1);
                }
              }}
              placeholder="如 IP_REGISTER_LIMIT"
            />
          </label>

          <button
            className="btn btn-primary"
            onClick={() => {
              setEventType(eventTypeInput.trim());
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
          <EmptyState title="暂无风险事件" desc="当前没有命中风控规则的事件" />
        )}

        {items.length > 0 && (
          <>
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th className="num">ID</th>
                    <th>时间</th>
                    <th>事件类型</th>
                    <th>级别</th>
                    <th className="num">用户 ID</th>
                    <th>IP</th>
                    <th>设备指纹</th>
                    <th>详情</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((e) => (
                    <tr key={e.id}>
                      <td className="num">{e.id}</td>
                      <td>{fmtTime(e.createdAt)}</td>
                      <td>
                        <code>{e.eventType}</code>
                      </td>
                      <td>
                        <span className={`badge ${SEVERITY_CLASS[e.severity]}`}>
                          {SEVERITY_LABEL[e.severity]}
                        </span>
                      </td>
                      <td className="num">{e.userId ?? "—"}</td>
                      <td>{e.ip || "—"}</td>
                      <td>{e.deviceHash || "—"}</td>
                      <td>
                        <span className="muted" title={JSON.stringify(e.detail)}>
                          {brief(e.detail)}
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
          风险事件为只追加日志，当前版本只读展示；规则命中统计、IP/设备黑名单与告警处置在 S3 之后接入。
        </p>
      </section>
    </div>
  );
}
