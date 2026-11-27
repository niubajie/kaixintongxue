import type { ReactNode } from "react";
import type { ResourceStatus } from "../api/types";
import { STATUS_LABEL } from "../api/types";

export function Loading({ text = "加载中…" }: { text?: string }) {
  return (
    <div className="state-box">
      <div className="spinner" />
      <p>{text}</p>
    </div>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="state-box state-error">
      <p>{message}</p>
      {onRetry && (
        <button className="btn" onClick={onRetry}>
          重试
        </button>
      )}
    </div>
  );
}

export function EmptyState({
  title,
  desc,
  children
}: {
  title: string;
  desc?: string;
  children?: ReactNode;
}) {
  return (
    <div className="state-box state-empty">
      <p className="empty-title">{title}</p>
      {desc && <p className="empty-desc">{desc}</p>}
      {children}
    </div>
  );
}

const STATUS_CLASS: Record<ResourceStatus, string> = {
  draft: "badge-gray",
  pending: "badge-warning",
  online: "badge-success",
  rejected: "badge-danger",
  offline: "badge-gray",
  expired: "badge-gray"
};

export function StatusBadge({ status }: { status: ResourceStatus }) {
  return <span className={`badge ${STATUS_CLASS[status]}`}>{STATUS_LABEL[status]}</span>;
}
