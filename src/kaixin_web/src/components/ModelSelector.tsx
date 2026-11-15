import { useEffect, useRef, useState } from "react";
import { AI_MODELS, type AiModelOption } from "../data";

interface Props {
  value: AiModelOption;
  onChange: (model: AiModelOption) => void;
  width: number;
}

export function ModelSelector({ value, onChange, width }: Props) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) {
      return;
    }
    const onDocClick = (event: MouseEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", onDocClick);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDocClick);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div className="model-selector" ref={rootRef}>
      <button
        type="button"
        className="model-trigger"
        onClick={() => setOpen((current) => !current)}
        title={value.name}
        aria-expanded={open}
      >
        <span className="model-name">{value.name}</span>
        <span className="chevron">▾</span>
      </button>
      {open && (
        <div className="model-popup" style={{ width: Math.max(width, 360) }}>
          <div className="model-table-head">
            <span>模型 / 引擎</span>
            <span>费用说明</span>
            <span>能力强弱</span>
          </div>
          <div className="model-rows">
            {AI_MODELS.map((model) => (
              <button
                type="button"
                key={model.name}
                className={`model-row${model.name === value.name ? " selected" : ""}`}
                onClick={() => {
                  onChange(model);
                  setOpen(false);
                }}
              >
                <span className="model-row-name">{model.name}</span>
                <span className="model-row-detail">{model.cost}</span>
                <span className="model-row-detail">{model.capability}</span>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
