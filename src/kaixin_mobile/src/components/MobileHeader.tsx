import type { ReactNode } from "react";
import { IconBack } from "./Icons";

interface Props {
  title: string;
  onBack: () => void;
  action?: ReactNode;
}

export function MobileHeader({ title, onBack, action }: Props) {
  return (
    <header className="page-head">
      <button type="button" className="icon-btn" onClick={onBack} aria-label="返回">
        <IconBack />
      </button>
      <h1>{title}</h1>
      <div className="page-head-action">{action}</div>
    </header>
  );
}

interface SheetProps {
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
}

export function BottomSheet({ open, title, onClose, children, footer }: SheetProps) {
  if (!open) {
    return null;
  }

  return (
    <div className="sheet-layer">
      <div className="sheet-mask" onClick={onClose} />
      <div className="sheet">
        <div className="sheet-head">
          <strong>{title}</strong>
          <button type="button" className="link-btn" onClick={onClose}>
            关闭
          </button>
        </div>
        <div className="sheet-body">{children}</div>
        {footer && <div className="sheet-footer">{footer}</div>}
      </div>
    </div>
  );
}
