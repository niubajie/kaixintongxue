interface Props {
  open: boolean;
  title: string;
  detail?: string;
  confirmText?: string;
  onCancel: () => void;
  onConfirm: () => void;
}

export function ConfirmDialog({ open, title, detail, confirmText = "删除", onCancel, onConfirm }: Props) {
  if (!open) {
    return null;
  }

  return (
    <div className="dialog-mask" onClick={onCancel}>
      <div className="dialog dialog-small" onClick={(event) => event.stopPropagation()}>
        <h2 className="dialog-title">{title}</h2>
        {detail && <p className="caption" style={{ marginTop: 8 }}>{detail}</p>}
        <div className="dialog-actions">
          <button type="button" className="secondary-btn" onClick={onCancel}>
            取消
          </button>
          <button type="button" className="danger-btn" onClick={onConfirm}>
            {confirmText}
          </button>
        </div>
      </div>
    </div>
  );
}
