import { useState } from "react";
import { TASKS, type TaskItem } from "../data";

interface Props {
  onNotice: (message: string) => void;
}

const CATEGORIES = ["工作任务", "学习任务", "生活任务"];

export function ApplicationPage({ onNotice }: Props) {
  const [tasks, setTasks] = useState<TaskItem[]>(TASKS);
  const [goal, setGoal] = useState("本周完成 3 篇公众号文章，并沉淀成知识库");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [category, setCategory] = useState(CATEGORIES[0]);
  const [date, setDate] = useState(new Date().toISOString().slice(0, 10));
  const [detail, setDetail] = useState("");
  const [progress, setProgress] = useState(0);

  const doneCount = tasks.filter((task) => task.progress >= 100).length;

  const generate = () => {
    if (!goal.trim()) {
      onNotice("请先输入本周目标，再生成计划。");
      return;
    }
    setTasks((current) => [
      ...current,
      {
        id: `t${current.length + 1}`,
        time: "17:30",
        title: "拆解目标步骤",
        detail: `围绕「${goal.trim()}」拆出 3 个可执行步骤`,
        category: "AI 建议",
        tag: "今天",
        progress: 0
      },
      {
        id: `t${current.length + 2}`,
        time: "20:00",
        title: "完成第一版草稿",
        detail: "按步骤完成第一版输出，留给次日复盘",
        category: "AI 建议",
        tag: "今天",
        progress: 0
      }
    ]);
    onNotice("已按演示规则追加 2 条计划，真实规划能力将在功能实现阶段接入。");
  };

  const createTask = () => {
    if (!title.trim()) {
      onNotice("请填写任务名称。");
      return;
    }
    setTasks((current) => [
      ...current,
      {
        id: `t${current.length + 1}`,
        time: date.slice(5),
        title: title.trim(),
        detail: detail.trim() || "未填写任务说明",
        category,
        tag: date,
        progress
      }
    ]);
    setDialogOpen(false);
    setTitle("");
    setDetail("");
    setProgress(0);
    onNotice(`已添加「${category}」：${title.trim()}（演示数据）。`);
  };

  return (
    <div className="page">
      <div className="page-narrow">
        <div className="page-head">
          <div>
            <h1 className="page-title">应用助手</h1>
            <p className="page-subtitle">规划工作、学习与生活任务，自动拆解为今日可执行安排（演示数据）</p>
          </div>
          <button type="button" className="secondary-btn" onClick={() => setDialogOpen(true)}>
            + 新增任务
          </button>
        </div>

        <div className="card">
          <h2 className="section-title">本周目标</h2>
          <input
            className="input"
            style={{ marginTop: 10 }}
            value={goal}
            onChange={(event) => setGoal(event.target.value)}
          />
          <div className="row" style={{ marginTop: 14 }}>
            <button type="button" className="primary-btn" onClick={generate}>
              生成今日规划
            </button>
            <span className="caption">演示规则：按目标追加 2 条计划</span>
          </div>
        </div>

        <div className="row" style={{ marginTop: 22, justifyContent: "space-between" }}>
          <h2 className="section-title">任务安排</h2>
          <div className="row" style={{ gap: 8 }}>
            <span className="tag">共 {tasks.length} 项</span>
            <span className="tag accent">已完成 {doneCount} 项</span>
          </div>
        </div>

        <div className="list">
          {tasks.map((task) => (
            <div className="plan-item" key={task.id}>
              <div className="plan-time">{task.time}</div>
              <div className="plan-body">
                <div style={{ flex: 1 }}>
                  <div className="list-card-title">{task.title}</div>
                  <p className="list-card-detail">{task.detail}</p>
                  <div className="row" style={{ marginTop: 10 }}>
                    <div className="progress slim" style={{ flex: 1 }}>
                      <span style={{ width: `${task.progress}%` }} />
                    </div>
                    <span className="caption">{task.progress}%</span>
                  </div>
                </div>
                <span className="tag" style={{ alignSelf: "flex-start" }}>
                  {task.category}
                </span>
              </div>
            </div>
          ))}
        </div>
        <p className="caption" style={{ marginTop: 6 }}>
          当前为页面演示，计划不会写入真实日历或任务系统。
        </p>
      </div>

      {dialogOpen && (
        <div className="dialog-mask" onClick={() => setDialogOpen(false)}>
          <div className="dialog" onClick={(event) => event.stopPropagation()}>
            <h2 className="dialog-title">新增任务</h2>
            <p className="caption" style={{ marginTop: 7 }}>
              选择任务类型与日期，填写任务说明和进度规划（演示数据）
            </p>

            <label className="field-label">任务名称</label>
            <input className="input" value={title} onChange={(event) => setTitle(event.target.value)} />

            <label className="field-label">任务类型</label>
            <div className="row" style={{ gap: 6 }}>
              {CATEGORIES.map((item) => (
                <button
                  type="button"
                  key={item}
                  className={`segment${category === item ? " active" : ""}`}
                  onClick={() => setCategory(item)}
                >
                  {item}
                </button>
              ))}
            </div>

            <label className="field-label">任务日期</label>
            <input
              className="input"
              type="date"
              value={date}
              onChange={(event) => setDate(event.target.value)}
            />

            <label className="field-label">任务说明</label>
            <textarea
              className="textarea"
              rows={3}
              value={detail}
              onChange={(event) => setDetail(event.target.value)}
            />

            <label className="field-label">进度规划：{progress}%</label>
            <input
              type="range"
              min={0}
              max={100}
              step={5}
              value={progress}
              onChange={(event) => setProgress(Number(event.target.value))}
            />

            <div className="dialog-actions">
              <button type="button" className="secondary-btn" onClick={() => setDialogOpen(false)}>
                取消
              </button>
              <button type="button" className="primary-btn" onClick={createTask}>
                添加任务
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
