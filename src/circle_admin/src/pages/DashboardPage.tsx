import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";

import { listResources } from "../api/admin";
import { STATUS_LABEL } from "../api/types";
import type { ResourceStatus } from "../api/types";
import { ErrorState } from "../components/States";

const CARDS: { status: ResourceStatus; hint: string }[] = [
  { status: "pending", hint: "需要尽快处理" },
  { status: "online", hint: "当前公开展示" },
  { status: "rejected", hint: "可查看拒绝原因" },
  { status: "offline", hint: "已下架资源" }
];

function StatCard({ status, hint }: { status: ResourceStatus; hint: string }) {
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["stat", status],
    queryFn: () => listResources({ status, page: 1, size: 1 })
  });

  if (isError) {
    return (
      <div className="stat-card">
        <div className="stat-label">{STATUS_LABEL[status]}</div>
        <ErrorState message={(error as Error).message} onRetry={() => void refetch()} />
      </div>
    );
  }

  return (
    <Link className="stat-card" to={`/resources?status=${status}`}>
      <div className="stat-label">{STATUS_LABEL[status]}</div>
      <div className="stat-value">{isLoading ? "—" : data?.total ?? 0}</div>
      <div className="stat-hint">{hint}</div>
    </Link>
  );
}

export default function DashboardPage() {
  return (
    <div className="page">
      <section className="panel">
        <div className="panel-head">
          <h2>资源概览</h2>
          <span className="muted">数据来自 /api/v1/admin/resources（实时）</span>
        </div>
        <div className="stat-grid">
          {CARDS.map((c) => (
            <StatCard key={c.status} status={c.status} hint={c.hint} />
          ))}
        </div>
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>常用操作</h2>
        </div>
        <div className="quick-links">
          <Link className="quick-link" to="/resources?status=pending">
            审核待处理资源
          </Link>
          <Link className="quick-link" to="/categories">
            调整分类定价与启停
          </Link>
          <Link className="quick-link" to="/audit">
            查看审计日志（待接入）
          </Link>
        </div>
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>模块进度</h2>
        </div>
        <p className="muted">
          已接入：管理员登录、分类管理、资源审核。用户管理、风控、订单退款、投诉、系统配置、审计查询的接口尚未实现，
          对应页面仅展示规划，不展示模拟数据。
        </p>
      </section>
    </div>
  );
}
