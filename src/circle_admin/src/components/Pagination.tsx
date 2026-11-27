interface PaginationProps {
  page: number;
  size: number;
  total: number;
  onChange: (page: number) => void;
}

export default function Pagination({ page, size, total, onChange }: PaginationProps) {
  const pages = Math.max(1, Math.ceil(total / size));
  return (
    <div className="pagination">
      <span className="pagination-info">
        共 {total} 条 · 第 {page}/{pages} 页
      </span>
      <div className="pagination-actions">
        <button className="btn" disabled={page <= 1} onClick={() => onChange(page - 1)}>
          上一页
        </button>
        <button className="btn" disabled={page >= pages} onClick={() => onChange(page + 1)}>
          下一页
        </button>
      </div>
    </div>
  );
}
