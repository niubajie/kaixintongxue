import { HISTORY } from "../data";

interface Props {
  onNotice: (message: string) => void;
}

export function HistoryPage({ onNotice }: Props) {
  return (
    <div className="page">
      <div className="page-narrow">
        <h1 className="page-title">历史任务</h1>
        <p className="page-subtitle">查看最近创建的任务（演示数据）</p>

        <div className="list" style={{ marginTop: 18 }}>
          {HISTORY.map((item) => (
            <button
              type="button"
              className="list-card"
              key={item.id}
              onClick={() => onNotice(`「${item.title}」详情将在功能实现阶段接入。`)}
            >
              <div className="list-card-head">
                <div>
                  <div className="list-card-title">{item.title}</div>
                  <p className="list-card-detail">
                    {item.time} · {item.state}
                  </p>
                </div>
                <span className="tag">查看</span>
              </div>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
