import { useEffect, useState } from "react";
import { fetchComputerStatus, type ComputerStatus } from "../deployApi";

// FR-GLB-07：窗口右上角显示硬盘剩余、内存剩余、联网状态。
// 不显示温度——读 CPU 温度需要内核驱动与管理员授权，本应用走免管理员路线。
// 15 秒刷新一次。

function formatGb(value: number): string {
  if (!value || value <= 0) {
    return "—";
  }
  return value >= 100 ? `${Math.round(value)}G` : `${value.toFixed(1)}G`;
}

export function ComputerStatusBar() {
  const [status, setStatus] = useState<ComputerStatus | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let alive = true;
    const load = () => {
      fetchComputerStatus()
        .then((next) => {
          if (alive) {
            setStatus(next);
            setFailed(false);
          }
        })
        .catch(() => {
          if (alive) {
            setFailed(true);
          }
        });
    };
    load();
    const timer = window.setInterval(load, 15000);
    return () => {
      alive = false;
      window.clearInterval(timer);
    };
  }, []);

  if (failed) {
    return (
      <span className="computer-status" title="本地服务未启动，读不到电脑状态">
        <span className="computer-status-item">电脑状态不可用</span>
      </span>
    );
  }
  if (!status) {
    return (
      <span className="computer-status" title="正在读取电脑状态">
        <span className="computer-status-item">读取中…</span>
      </span>
    );
  }

  return (
    <span className="computer-status">
      <span
        className="computer-status-item"
        title={`数据目录所在盘 ${status.diskName || "?"}：可用 ${status.diskFreeGb.toFixed(1)} GB / 共 ${status.diskTotalGb.toFixed(0)} GB`}
      >
        硬盘剩余 {status.diskName || ""} {formatGb(status.diskFreeGb)}/{formatGb(status.diskTotalGb)}
      </span>
      <span
        className="computer-status-item"
        title={`内存可用 ${status.memFreeGb.toFixed(1)} GB / 共 ${status.memTotalGb.toFixed(1)} GB`}
      >
        内存剩余 {formatGb(status.memFreeGb)}/{formatGb(status.memTotalGb)}
      </span>
      <span
        className="computer-status-item"
        title={status.online ? "联网正常" : "未检测到联网"}
        style={{ color: status.online ? "var(--accent)" : "var(--danger)" }}
      >
        {status.online ? "● 在线" : "● 离线"}
      </span>
    </span>
  );
}
