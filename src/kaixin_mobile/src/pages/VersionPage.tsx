import { MobileHeader } from "../components/MobileHeader";
import { HISTORY } from "../data";

interface Props {
  onBack: () => void;
  onNotice: (message: string) => void;
}

export function VersionPage({ onBack, onNotice }: Props) {
  return (
    <>
      <MobileHeader title="版本更新" onBack={onBack} />

      <div className="page-scroll">
        <div className="card version-card">
          <div>
            <strong>1.0.0</strong>
            <span className="muted">手机端 · TypeScript + React + Vite + Capacitor</span>
          </div>
          <span className="tag accent">已是最新</span>
        </div>

        <button
          type="button"
          className="primary-btn block"
          onClick={() => onNotice("已检查更新：当前已是最新版本。")}
        >
          检查更新
        </button>

        <p className="section-label">历史版本</p>
        {HISTORY.map((item) => (
          <div key={item.id} className="card">
            <div className="card-title-row">
              <strong>{item.title}</strong>
              <span className="muted">{item.time}</span>
            </div>
            <p className="muted">{item.state}</p>
          </div>
        ))}
      </div>
    </>
  );
}
