import { useEffect, useState } from "react";
import { fetchDeployHistory, type DeployHistoryRun } from "../deployApi";

interface Props {
  onNotice: (message: string) => void;
}

const STATE_LABEL: Record<string, string> = {
  done: "已完成",
  failed: "已失败",
  cancelled: "已取消",
  running: "进行中",
  interrupted: "已中断"
};

function formatTime(iso: string): string {
  if (!iso) {
    return "—";
  }
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return iso;
  }
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

export function HistoryPage({ onNotice }: Props) {
  const [state, setState] = useState<"loading" | "ready" | "error">("loading");
  const [runs, setRuns] = useState<DeployHistoryRun[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    let alive = true;
    fetchDeployHistory()
      .then((items) => {
        if (!alive) {
          return;
        }
        setRuns(items);
        setState("ready");
      })
      .catch((err: unknown) => {
        if (!alive) {
          return;
        }
        setError(err instanceof Error ? err.message : "读取部署历史失败");
        setState("error");
      });
    return () => {
      alive = false;
    };
  }, []);

  return (
    <div className="page">
      <div className="page-narrow">
        <h1 className="page-title">历史任务</h1>
        <p className="page-subtitle">每次「部署方案」执行完都会在这里留一条记录</p>

        {state === "loading" && (
          <div className="card" style={{ marginTop: 18 }}>
            <span className="caption">正在读取本地记录…</span>
          </div>
        )}

        {state === "error" && (
          <div className="card" style={{ marginTop: 18 }}>
            <div style={{ fontSize: 13, fontWeight: 600 }}>读取失败</div>
            <p className="list-card-detail" style={{ marginTop: 6 }}>{error}</p>
            <p className="caption" style={{ marginTop: 6 }}>
              请确认本地服务已启动（部署过一次之后才会有记录）。
            </p>
          </div>
        )}

        {state === "ready" && runs.length === 0 && (
          <div className="card" style={{ marginTop: 18 }}>
            <div style={{ fontSize: 13, fontWeight: 600 }}>还没有部署记录</div>
            <p className="caption" style={{ marginTop: 6 }}>
              到「技能部署」生成方案并执行一次，这里就会出现记录。
            </p>
          </div>
        )}

        {state === "ready" && runs.length > 0 && (
          <div className="list" style={{ marginTop: 18 }}>
            {runs.map((run) => (
              <button
                type="button"
                className="list-card"
                key={run.id}
                onClick={() => onNotice(run.logPath ? `日志位置：${run.logPath}` : "该记录没有日志文件")}
              >
                <div className="list-card-head">
                  <div>
                    <div className="list-card-title">
                      {formatTime(run.startedAt)} · 部署 {run.totalSizeMb} MB
                    </div>
                    <p className="list-card-detail">
                      {STATE_LABEL[run.state] ?? run.state}
                      {run.finishedAt ? ` · 结束于 ${formatTime(run.finishedAt)}` : ""}
                      {run.attempt > 1 ? ` · 第 ${run.attempt} 次尝试` : ""}
                    </p>
                    {run.error && (
                      <p className="list-card-detail" style={{ color: "var(--danger)" }}>
                        {run.error}
                      </p>
                    )}
                  </div>
                  <span className={`tag${run.state === "done" ? " accent" : ""}`}>
                    {STATE_LABEL[run.state] ?? run.state}
                  </span>
                </div>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
