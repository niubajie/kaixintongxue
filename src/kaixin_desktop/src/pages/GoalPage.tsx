import { useState } from "react";
import { GOALS, type GoalItem } from "../data";

interface Props {
  onNotice: (message: string) => void;
}

export function GoalPage({ onNotice }: Props) {
  const [goals, setGoals] = useState<GoalItem[]>(GOALS);
  const [selectedId, setSelectedId] = useState(GOALS[0].id);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [detail, setDetail] = useState("");
  const [deadline, setDeadline] = useState("");
  const [plan, setPlan] = useState("");

  const selected = goals.find((goal) => goal.id === selectedId) ?? goals[0];

  const updateProgress = (value: number) => {
    setGoals((current) =>
      current.map((goal) => (goal.id === selected.id ? { ...goal, progress: value } : goal))
    );
  };

  const createGoal = () => {
    if (!title.trim()) {
      onNotice("请填写目标名称。");
      return;
    }
    const steps = (plan || "第一步\n第二步\n第三步")
      .split("\n")
      .map((step) => step.trim())
      .filter(Boolean);
    const next: GoalItem = {
      id: `g${goals.length + 1}`,
      title: title.trim(),
      detail: detail.trim() || "未填写目标说明",
      deadline: deadline || "未设置",
      progress: 0,
      steps
    };
    setGoals((current) => [...current, next]);
    setSelectedId(next.id);
    setDialogOpen(false);
    setTitle("");
    setDetail("");
    setDeadline("");
    setPlan("");
    onNotice(`已新增目标「${next.title}」（演示数据）。`);
  };

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="page-title">目标助手</h1>
          <p className="page-subtitle">设定目标、规划步骤并跟踪完成进度（演示数据）</p>
        </div>
        <button type="button" className="secondary-btn" onClick={() => setDialogOpen(true)}>
          + 新增目标
        </button>
      </div>

      <div className="goal-layout">
        <div className="list" style={{ marginTop: 0 }}>
          {goals.map((goal) => (
            <button
              type="button"
              key={goal.id}
              className={`list-card${goal.id === selected.id ? " selected" : ""}`}
              onClick={() => setSelectedId(goal.id)}
            >
              <div className="list-card-head">
                <span className="list-card-title">{goal.title}</span>
                <span className="tag">{goal.deadline}</span>
              </div>
              <p className="list-card-detail">{goal.detail}</p>
              <div className="row" style={{ marginTop: 10 }}>
                <div className="progress" style={{ flex: 1 }}>
                  <span style={{ width: `${goal.progress}%` }} />
                </div>
                <span className="caption">{goal.progress}%</span>
              </div>
            </button>
          ))}
        </div>

        <div className="goal-side">
          <h2 className="section-title">{selected.title}</h2>
          <p className="list-card-detail">{selected.detail}</p>

          <div className="field-label">目标规划</div>
          {selected.steps.map((step, index) => (
            <div className="step" key={step}>
              <span className="step-dot">{index + 1}</span>
              <span>{step}</span>
            </div>
          ))}

          <div className="field-label">完成进度：{selected.progress}%</div>
          <input
            type="range"
            min={0}
            max={100}
            step={5}
            value={selected.progress}
            onChange={(event) => updateProgress(Number(event.target.value))}
          />
          <div className="row" style={{ marginTop: 12 }}>
            <button
              type="button"
              className="primary-btn"
              onClick={() => {
                updateProgress(Math.min(100, selected.progress + 10));
                onNotice(`「${selected.title}」进度已推进 10%（演示，不影响真实目标）。`);
              }}
            >
              推进 10%
            </button>
            <span className="tag">截止 {selected.deadline}</span>
          </div>
        </div>
      </div>

      {dialogOpen && (
        <div className="dialog-mask" onClick={() => setDialogOpen(false)}>
          <div className="dialog" onClick={(event) => event.stopPropagation()}>
            <h2 className="dialog-title">新增目标</h2>
            <p className="caption" style={{ marginTop: 7 }}>
              填写目标信息，并按行拆解步骤（演示数据）
            </p>

            <label className="field-label">目标名称</label>
            <input className="input" value={title} onChange={(event) => setTitle(event.target.value)} />

            <label className="field-label">目标说明</label>
            <textarea
              className="textarea"
              rows={2}
              value={detail}
              onChange={(event) => setDetail(event.target.value)}
            />

            <label className="field-label">截止时间</label>
            <input
              className="input"
              placeholder="例如 12 月 31 日"
              value={deadline}
              onChange={(event) => setDeadline(event.target.value)}
            />

            <label className="field-label">步骤规划（每行一步）</label>
            <textarea
              className="textarea"
              rows={3}
              placeholder={"整理资料\n制定计划\n执行并复盘"}
              value={plan}
              onChange={(event) => setPlan(event.target.value)}
            />

            <div className="dialog-actions">
              <button type="button" className="secondary-btn" onClick={() => setDialogOpen(false)}>
                取消
              </button>
              <button type="button" className="primary-btn" onClick={createGoal}>
                保存目标
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
