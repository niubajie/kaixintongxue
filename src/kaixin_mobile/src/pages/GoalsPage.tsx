import { useState } from "react";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { BottomSheet, MobileHeader } from "../components/MobileHeader";
import { TextWithVoice } from "../components/TextWithVoice";
import { IconCheck, IconPlusPlain, IconSpark, IconTrash } from "../components/Icons";
import {
  SAMPLE_GOALS,
  buildGoalPlan,
  createId,
  generateGoalQuestions,
  progressOf,
  usePersistentState,
  type AssistantGoal
} from "../assistant";

interface Props {
  onBack: () => void;
  onNotice: (message: string) => void;
}

export function GoalsPage({ onBack, onNotice }: Props) {
  const [goals, setGoals] = usePersistentState<AssistantGoal[]>("kaixin.mobile.goals", SAMPLE_GOALS);
  const [detailId, setDetailId] = useState<string | null>(null);
  const [tab, setTab] = useState<"ask" | "plan">("ask");

  const [creatorOpen, setCreatorOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [moreOpen, setMoreOpen] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");

  const current = goals.find((item) => item.id === detailId) ?? null;
  const percent = current ? progressOf(current.stages) : 0;

  const patch = (id: string, updater: (goal: AssistantGoal) => AssistantGoal) => {
    setGoals((list) => list.map((item) => (item.id === id ? updater(item) : item)));
  };

  const createGoal = () => {
    const name = title.trim();
    if (!name) {
      onNotice("请填写目标标题。");
      return;
    }
    const goal: AssistantGoal = {
      id: createId("goal"),
      title: name,
      description: description.trim(),
      questions: generateGoalQuestions(name),
      stages: buildGoalPlan(name)
    };
    setGoals((list) => [goal, ...list]);
    setTitle("");
    setDescription("");
    setCreatorOpen(false);
    setDetailId(goal.id);
    setTab("ask");
    onNotice(`已新建目标：${name}`);
  };

  const saveEdit = () => {
    if (!current) {
      return;
    }
    const name = title.trim();
    if (!name) {
      onNotice("目标标题不能为空。");
      return;
    }
    patch(current.id, (goal) => ({ ...goal, title: name, description: description.trim() }));
    setEditOpen(false);
    onNotice("目标已更新。");
  };

  const toggleStep = (stageId: string, stepId: string) => {
    if (!current) {
      return;
    }
    patch(current.id, (goal) => ({
      ...goal,
      stages: goal.stages.map((stage) =>
        stage.id === stageId
          ? {
              ...stage,
              steps: stage.steps.map((step) =>
                step.id === stepId ? { ...step, done: !step.done } : step
              )
            }
          : stage
      )
    }));
  };

  if (current) {
    return (
      <>
        <MobileHeader
          title={current.title}
          onBack={() => setDetailId(null)}
          action={
            <button
              type="button"
              className="icon-btn"
              onClick={() => setMoreOpen(true)}
              aria-label="更多操作"
            >
              <span className="dots">···</span>
            </button>
          }
        />

        <div className="page-scroll">
          <div className="progress-card">
            <div className="progress-head">
              <span>完成进度</span>
              <strong>{percent}%</strong>
            </div>
            <div className="progress-track">
              <span style={{ width: `${percent}%` }} />
            </div>
            <p className="muted">{current.description || "尚未填写目标说明"}</p>
          </div>

          <div className="tabs">
            <button
              type="button"
              className={`tab${tab === "ask" ? " active" : ""}`}
              onClick={() => setTab("ask")}
            >
              AI 生成提问
            </button>
            <button
              type="button"
              className={`tab${tab === "plan" ? " active" : ""}`}
              onClick={() => setTab("plan")}
            >
              自动建方案
            </button>
          </div>

          {tab === "ask" && (
            <>
              <div className="section-head">
                <span>针对目标的提问 · {current.questions.length}</span>
                <button
                  type="button"
                  className="mini-primary"
                  onClick={() => {
                    patch(current.id, (goal) => ({
                      ...goal,
                      questions: generateGoalQuestions(goal.title)
                    }));
                    onNotice("已重新生成提问。");
                  }}
                >
                  <IconSpark size={15} /> 重新生成
                </button>
              </div>
              {current.questions.map((question, index) => (
                <div key={`${question}-${index}`} className="card ask-card">
                  <span className="ask-index">{index + 1}</span>
                  <p>{question}</p>
                </div>
              ))}
            </>
          )}

          {tab === "plan" && (
            <>
              {current.stages.map((stage) => (
                <div key={stage.id} className="card">
                  <div className="stage-title">{stage.name}</div>
                  {stage.steps.map((step) => (
                    <button
                      key={step.id}
                      type="button"
                      className={`step-row${step.done ? " done" : ""}`}
                      onClick={() => toggleStep(stage.id, step.id)}
                    >
                      <span className="check">{step.done && <IconCheck />}</span>
                      <span>{step.text}</span>
                    </button>
                  ))}
                </div>
              ))}
            </>
          )}
        </div>

        <BottomSheet open={moreOpen} title="目标操作" onClose={() => setMoreOpen(false)}>
          <button
            type="button"
            className="sheet-row"
            onClick={() => {
              setTitle(current.title);
              setDescription(current.description);
              setMoreOpen(false);
              setEditOpen(true);
            }}
          >
            修改目标
          </button>
          <button
            type="button"
            className="sheet-row danger-text"
            onClick={() => {
              setMoreOpen(false);
              setConfirmOpen(true);
            }}
          >
            <IconTrash /> 删除目标
          </button>
        </BottomSheet>

        <BottomSheet
          open={editOpen}
          title="修改目标"
          onClose={() => setEditOpen(false)}
          footer={
            <div className="sheet-actions">
              <button type="button" className="primary-btn" onClick={saveEdit}>
                保存修改
              </button>
            </div>
          }
        >
          <label className="field-label">标题名称</label>
          <TextWithVoice value={title} onChange={setTitle} rows={2} onNotice={onNotice} />
          <label className="field-label">目标说明</label>
          <TextWithVoice value={description} onChange={setDescription} rows={3} onNotice={onNotice} />
        </BottomSheet>

        <ConfirmDialog
          open={confirmOpen}
          title="删除这个目标？"
          detail={`${current.title} 删除后无法恢复。`}
          onCancel={() => setConfirmOpen(false)}
          onConfirm={() => {
            setGoals((list) => list.filter((item) => item.id !== current.id));
            setDetailId(null);
            setConfirmOpen(false);
            onNotice("目标已删除。");
          }}
        />
      </>
    );
  }

  return (
    <>
      <MobileHeader
        title="目标助手"
        onBack={onBack}
        action={
          <button type="button" className="mini-primary" onClick={() => setCreatorOpen(true)}>
            <IconPlusPlain size={16} /> 新建
          </button>
        }
      />

      <div className="page-scroll">
        <p className="page-desc">每个目标独立管理 AI 提问与执行方案，可新建或删除。</p>
        <p className="section-label">全部目标 · {goals.length}</p>

        {goals.map((goal) => {
          const value = progressOf(goal.stages);
          return (
            <button
              key={goal.id}
              type="button"
              className="card tap"
              onClick={() => {
                setDetailId(goal.id);
                setTab("ask");
              }}
            >
              <div className="card-title-row">
                <strong>{goal.title}</strong>
                <span className="percent">{value}%</span>
              </div>
              <div className="progress-track slim">
                <span style={{ width: `${value}%` }} />
              </div>
              <p className="muted">{goal.description || "尚未填写目标说明"}</p>
            </button>
          );
        })}
      </div>

      <BottomSheet
        open={creatorOpen}
        title="新建目标"
        onClose={() => setCreatorOpen(false)}
        footer={
          <div className="sheet-actions">
            <button type="button" className="primary-btn" onClick={createGoal}>
              生成提问与方案
            </button>
          </div>
        }
      >
        <label className="field-label">标题名称</label>
        <TextWithVoice
          value={title}
          onChange={setTitle}
          rows={2}
          placeholder="例如：三个月减重 10 斤"
          onNotice={onNotice}
        />
        <label className="field-label">目标说明</label>
        <TextWithVoice
          value={description}
          onChange={setDescription}
          rows={3}
          placeholder="说明目标背景、期望结果和时间"
          onNotice={onNotice}
        />
      </BottomSheet>
    </>
  );
}
