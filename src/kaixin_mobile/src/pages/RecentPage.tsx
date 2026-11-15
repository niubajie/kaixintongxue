import { MobileHeader } from "../components/MobileHeader";
import { HISTORY } from "../data";
import { IconCloud, IconMonitor } from "../components/Icons";

interface Props {
  onBack: () => void;
  onNotice: (message: string) => void;
}

export function RecentPage({ onBack, onNotice }: Props) {
  return (
    <>
      <MobileHeader title="最近" onBack={onBack} />

      <div className="page-scroll">
        <p className="page-desc">按时间倒序展示最近的对话与任务，点击可继续。</p>

        {HISTORY.map((item, index) => (
          <button
            key={item.id}
            type="button"
            className="card tap doc-row"
            onClick={() => onNotice(`已打开：${item.title}`)}
          >
            <span className="doc-icon">{index % 2 === 0 ? <IconCloud /> : <IconMonitor />}</span>
            <span className="doc-main">
              <strong>{item.title}</strong>
              <span className="muted">
                {item.time} · {item.state}
              </span>
            </span>
          </button>
        ))}
      </div>
    </>
  );
}
