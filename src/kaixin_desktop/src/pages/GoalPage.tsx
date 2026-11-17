import { useState } from "react";
import {
  buildGoalPlan,
  createId,
  generateGoalQuestions,
  progressOf,
  SAMPLE_GOALS,
  usePersistentState,
  type AssistantGoal
} from "../assistant";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { TextWithVoice } from "../components/TextWithVoice";

interface Props {
  onNotice: (message: string) => void;
}

export function GoalPage({ onNotice }: Props) {
  const [goals, setGoals] = usePersistentState<AssistantGoal[]>("kaixin.goals.v1", SAMPLE_GOALS);
  const [selectedId, setSelectedId] = useState(goals[0]?.id ?? "");

  const [formOpen, setFormOpen] = useState(false);

  // FR-GOAL-02：生成方案后提问自动缩成一行，点下三角再展开
  const [questionsOpen, setQuestionsOpen] = useState(false);
  const [stageEditing, setStageEditing] = useState<{ id: string; value: string } | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [confirmTarget, setConfirmTarget] = useState<AssistantGoal | null>(null);

  const selected = goals.find((goal) => goal.id === selectedId) ?? goals[0];

  const updateSelected = (updater: (goal: AssistantGoal) => AssistantGoal) => {
    if (!selected) {
      return;
    }
    setGoals((current) => current.map((goal) => (goal.id === selected.id ? updater(goal) : goal)));
  };

  const openCreate = () => {
    setEditingId(null);
    setTitle("");
    setDescription("");
    setFormOpen(true);
  };

  const openEdit = () => {
    if (!selected) {
      return;
    }
    setEditingId(selected.id);
    setTitle(selected.title);
    setDescription(selected.description);
    setFormOpen(true);
  };

  const save = () => {
    if (!title.trim()) {
      onNotice("请填写目标标题名称。");
      return;
    }
    if (editingId) {
      setGoals((current) =>
        current.map((goal) =>
          goal.id === editingId
            ? { ...goal, title: title.trim(), description: description.trim() }
            : goal
        )
      );
      onNotice("目标已保存修改。");
    } else {
      const next: AssistantGoal = {
        id: createId("goal"),
        title: title.trim(),
        description: description.trim(),
        questions: [],
        stages: []
      };
      setGoals((current) => [...current, next]);
      setSelectedId(next.id);
      onNotice(`已新建目标「${next.title}」，可继续生成提问和方案。`);
    }
    setFormOpen(false);
  };

  const generateQuestions = () => {
    if (!selected) {
      return;
    }
    updateSelected((goal) => ({ ...goal, questions: generateGoalQuestions(goal.title) }));
    onNotice("已生成 5 个用于澄清目标的提问。");
  };

  const generatePlan = () => {
    if (!selected) {
      return;
    }
    updateSelected((goal) => ({ ...goal, stages: buildGoalPlan(goal.title) }));
    onNotice("已按三阶段自动生成执行方案。");
  };

  const toggleStep = (stageId: string, stepId: string) => {
    updateSelected((goal) => ({
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

  const removeGoal = () => {
    if (!confirmTarget) {
      return;
    }
    setGoals((current) => current.filter((goal) => goal.id !== confirmTarget.id));
    onNotice(`已删除目标「${confirmTarget.title}」。`);
    setConfirmTarget(null);
  };

  // 方案可编辑：AI 生成的步骤允许人工增删（生成错时不必整体重来）
  const addStep = (stageId: string, text: string) => {
    const value = text.trim();
    if (!value) {
      return;
    }
    updateSelected((goal) => ({
      ...goal,
      stages: goal.stages.map((stage) =>
        stage.id === stageId
          ? {
              ...stage,
              steps: [...stage.steps, { id: `${stage.id}-${Date.now()}`, text: value, done: false }]
            }
          : stage
      )
    }));
    onNotice("已添加步骤。");
  };

  const removeStep = (stageId: string, stepId: string) => {
    updateSelected((goal) => ({
      ...goal,
      stages: goal.stages.map((stage) =>
        stage.id === stageId
          ? { ...stage, steps: stage.steps.filter((step) => step.id !== stepId) }
          : stage
      )
    }));
    onNotice("已删除步骤。");
  };

  const progress = selected ? progressOf(selected.stages) : 0;
  const collapsedQuestions = Boolean(selected && selected.stages.length > 0 && !questionsOpen);

  // FR-GOAL-06：阶段名称可调整
  const renameStage = (stageId: string, name: string) => {
    const value = name.trim();
    if (!value) {
      onNotice("阶段名称不能为空。");
      return;
    }
    updateSelected((goal) => ({
      ...goal,
      stages: goal.stages.map((stage) => (stage.id === stageId ? { ...stage, name: value } : stage))
    }));
    setStageEditing(null);
    onNotice("阶段名称已更新。");
  };

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="page-title">目标助手</h1>
          <p className="page-subtitle">每个目标独立管理 AI 提问与执行方案，进度按步骤自动计算</p>
        </div>
        <button type="button" className="secondary-btn" onClick={openCreate}>
          + 新建目标
        </button>
      </div>

      <div className="assistant-layout">
        <div className="assistant-side">
          <div className="side-title">
            全部目标 <span className="caption">{goals.length}</span>
          </div>
          <div className="list" style={{ marginTop: 8 }}>
            {goals.map((goal) => (
              <div key={goal.id} className={`side-item-wrap${goal.id === selected?.id ? " selected" : ""}`}>
                <button type="button" className="side-item" onClick={() => setSelectedId(goal.id)}>
                  <span className="side-item-title">{goal.title}</span>
                  <span className="side-item-meta">
                    {goal.stages.length > 0 ? `进度 ${progressOf(goal.stages)}%` : "尚未生成方案"}
                  </span>
                </button>
                <button
                  type="button"
                  className="side-item-remove"
                  title="删除目标"
                  onClick={() => setConfirmTarget(goal)}
                >
                  ×
                </button>
              </div>
            ))}
          </div>
          {goals.length === 0 && <div className="empty">还没有目标，点击右上角新建。</div>}
        </div>

        <div className="assistant-main">
          {selected ? (
            <>
              <div className="detail-head">
                <div>
                  <h2 className="detail-title">{selected.title}</h2>
                  <p className="list-card-detail">
                    {selected.description || "还没有填写目标说明，可点击“修改”补充。"}
                  </p>
                </div>
                <div className="row" style={{ gap: 8 }}>
                  <button type="button" className="secondary-btn" onClick={openEdit}>
                    修改
                  </button>
                  <button
                    type="button"
                    className="ghost-btn danger-text"
                    onClick={() => setConfirmTarget(selected)}
                  >
                    删除
                  </button>
                </div>
              </div>

              <div className="card" style={{ marginTop: 12 }}>
                <div className="row" style={{ justifyContent: "space-between" }}>
                  <span className="row" style={{ gap: 6 }}>
                    <h3 className="section-title">AI 生成提问</h3>
                    {collapsedQuestions && (
                      <span className="caption">
                        {selected.questions.length} 个提问 · 已用于生成方案
                      </span>
                    )}
                  </span>
                  <span className="row" style={{ gap: 6 }}>
                    <button type="button" className="secondary-btn" onClick={generateQuestions}>
                      {selected.questions.length > 0 ? "重新生成提问" : "生成提问"}
                    </button>
                    {selected.questions.length > 0 && selected.stages.length > 0 && (
                      <button
                        type="button"
                        className="ghost-btn"
                        title={questionsOpen ? "收起提问" : "展开提问"}
                        onClick={() => setQuestionsOpen((current) => !current)}
                      >
                        {questionsOpen ? "▲" : "▼"}
                      </button>
                    )}
                  </span>
                </div>
                {selected.questions.length === 0 ? (
                  <p className="caption" style={{ marginTop: 10 }}>
                    点击“生成提问”，AI 会围绕这个目标列出需要澄清的问题。
                  </p>
                ) : collapsedQuestions ? null : (
                  <div style={{ marginTop: 10 }}>
                    {selected.questions.map((question, index) => (
                      <div className="question-item" key={question}>
                        <span className="question-index">{index + 1}</span>
                        <span>{question}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              <div className="card" style={{ marginTop: 12 }}>
                <div className="row" style={{ justifyContent: "space-between" }}>
                  <h3 className="section-title">自动建方案</h3>
                  <button type="button" className="secondary-btn" onClick={generatePlan}>
                    {selected.stages.length > 0 ? "重新生成方案" : "自动建方案"}
                  </button>
                </div>

                {selected.stages.length > 0 ? (
                  <>
                    <div className="row" style={{ marginTop: 12 }}>
                      <div className="progress" style={{ flex: 1 }}>
                        <span style={{ width: `${progress}%` }} />
                      </div>
                      <span className="caption">{progress}%</span>
                    </div>
                    <div style={{ marginTop: 12 }}>
                      {selected.stages.map((stage) => (
                        <div className="stage" key={stage.id}>
                          <div className="stage-head">
                            {stageEditing?.id === stage.id ? (
                              <span className="row" style={{ gap: 6, flex: 1 }}>
                                <input
                                  className="input"
                                  value={stageEditing.value}
                                  onChange={(event) =>
                                    setStageEditing({ id: stage.id, value: event.target.value })
                                  }
                                  onKeyDown={(event) => {
                                    if (event.key === "Enter") {
                                      renameStage(stage.id, stageEditing.value);
                                    }
                                    if (event.key === "Escape") {
                                      setStageEditing(null);
                                    }
                                  }}
                                />
                                <button
                                  type="button"
                                  className="primary-btn"
                                  onClick={() => renameStage(stage.id, stageEditing.value)}
                                >
                                  保存
                                </button>
                              </span>
                            ) : (
                              <span className="row" style={{ gap: 6 }}>
                                <span className="stage-name">{stage.name}</span>
                                <button
                                  type="button"
                                  className="ghost-btn"
                                  title="修改阶段名称"
                                  onClick={() => setStageEditing({ id: stage.id, value: stage.name })}
                                >
                                  改名
                                </button>
                              </span>
                            )}
                            <span className="tag">
                              {stage.steps.filter((step) => step.done).length}/{stage.steps.length}
                            </span>
                          </div>
                          {stage.steps.map((step) => (
                            <div className="check-row" key={step.id}>
                              <label className="check-row" style={{ flex: 1, padding: 0, margin: 0 }}>
                                <input
                                  type="checkbox"
                                  checked={step.done}
                                  onChange={() => toggleStep(stage.id, step.id)}
                                />
                                <span className={step.done ? "done-text" : ""}>{step.text}</span>
                              </label>
                              <button
                                type="button"
                                className="ghost-btn"
                                title="删除这一步"
                                onClick={() => removeStep(stage.id, step.id)}
                              >
                                ✕
                              </button>
                            </div>
                          ))}
                          <div className="row" style={{ marginTop: 6, gap: 6 }}>
                            <input
                              className="input"
                              placeholder="补充一步…"
                              onKeyDown={(event) => {
                                if (event.key !== "Enter") {
                                  return;
                                }
                                const target = event.currentTarget;
                                addStep(stage.id, target.value);
                                target.value = "";
                              }}
                            />
                            <span className="caption">回车添加</span>
                          </div>
                        </div>
                      ))}
                    </div>
                  </>
                ) : (
                  <p className="caption" style={{ marginTop: 10 }}>
                    点击“自动建方案”，AI 会把目标拆成三个阶段并生成可勾选的步骤。
                  </p>
                )}
              </div>
            </>
          ) : (
            <div className="empty">请选择或新建一个目标。</div>
          )}
        </div>
      </div>

      {formOpen && (
        <div className="dialog-mask" onClick={() => setFormOpen(false)}>
          <div className="dialog" onClick={(event) => event.stopPropagation()}>
            <h2 className="dialog-title">{editingId ? "修改目标" : "新建目标"}</h2>
            <p className="caption" style={{ marginTop: 7 }}>
              新建后包含“AI 生成提问”和“自动建方案”两个板块，可语音或打字录入。
            </p>
            <label className="field-label">标题名称</label>
            <input
              className="input"
              placeholder="例如 暴瘦20斤、年赚100万计划"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
            />
            <label className="field-label">目标说明</label>
            <TextWithVoice
              value={description}
              onChange={setDescription}
              rows={4}
              placeholder="想达成什么，怎么衡量，什么时候完成…"
              onNotice={onNotice}
            />
            <div className="row" style={{ gap: 6, marginTop: 12 }}>
              <span className="tag accent">AI 生成提问</span>
              <span className="tag accent">自动建方案</span>
            </div>
            <div className="dialog-actions">
              <button type="button" className="secondary-btn" onClick={() => setFormOpen(false)}>
                取消
              </button>
              <button type="button" className="primary-btn" onClick={save}>
                {editingId ? "保存修改" : "创建目标"}
              </button>
            </div>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={confirmTarget !== null}
        title={`删除目标「${confirmTarget?.title ?? ""}」？`}
        detail="该目标的提问与方案会一起删除，删除后不可恢复。"
        onCancel={() => setConfirmTarget(null)}
        onConfirm={removeGoal}
      />
    </div>
  );
}
