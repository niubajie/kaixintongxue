import { useState } from "react";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { BottomSheet, MobileHeader } from "../components/MobileHeader";
import { TextWithVoice } from "../components/TextWithVoice";
import { IconCheck, IconPlusPlain, IconSpark, IconTrash } from "../components/Icons";
import {
  SAMPLE_LEARNING,
  buildLearningPlan,
  createId,
  formatDate,
  generateLearningQuestions,
  progressOf,
  scoreOf,
  usePersistentState,
  type LearningModule
} from "../assistant";

interface Props {
  onBack: () => void;
  onNotice: (message: string) => void;
}

export function LearningPage({ onBack, onNotice }: Props) {
  const [modules, setModules] = usePersistentState<LearningModule[]>(
    "kaixin.mobile.learning",
    SAMPLE_LEARNING
  );
  const [detailId, setDetailId] = useState<string | null>(null);
  const [tab, setTab] = useState<"ask" | "plan" | "remind">("ask");

  const [creatorOpen, setCreatorOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [moreOpen, setMoreOpen] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [scheduleTime, setScheduleTime] = useState("20:00");
  const [scheduleText, setScheduleText] = useState("");

  const current = modules.find((item) => item.id === detailId) ?? null;
  const percent = current ? progressOf(current.stages) : 0;
  const score = current ? scoreOf(current.stages) : { score: 0, label: "待提升" };

  const patch = (id: string, updater: (item: LearningModule) => LearningModule) => {
    setModules((list) => list.map((item) => (item.id === id ? updater(item) : item)));
  };

  const createModule = () => {
    const name = title.trim();
    if (!name) {
      onNotice("请填写学习模块标题。");
      return;
    }
    const module: LearningModule = {
      id: createId("learn"),
      title: name,
      description: description.trim(),
      questions: generateLearningQuestions(name),
      stages: buildLearningPlan(name),
      schedules: [
        { id: createId("sch"), time: "20:00", text: `复习${name}当天内容`, done: false }
      ]
    };
    setModules((list) => [module, ...list]);
    setTitle("");
    setDescription("");
    setCreatorOpen(false);
    setDetailId(module.id);
    setTab("ask");
    onNotice(`已新建学习模块：${name}`);
  };

  const saveEdit = () => {
    if (!current) {
      return;
    }
    const name = title.trim();
    if (!name) {
      onNotice("学习模块标题不能为空。");
      return;
    }
    patch(current.id, (item) => ({ ...item, title: name, description: description.trim() }));
    setEditOpen(false);
    onNotice("学习模块已更新。");
  };

  const addSchedule = () => {
    if (!current) {
      return;
    }
    const text = scheduleText.trim();
    if (!text) {
      onNotice("请填写督促内容。");
      return;
    }
    patch(current.id, (item) => ({
      ...item,
      schedules: [
        ...item.schedules,
        { id: createId("sch"), time: scheduleTime || "20:00", text, done: false }
      ]
    }));
    setScheduleText("");
    onNotice("已添加定时督促。");
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
          <div className="score-card">
            <div className="score-ring">
              <strong>{score.score}</strong>
              <span>分</span>
            </div>
            <div className="score-meta">
              <span className={`score-label level-${score.label}`}>{score.label}</span>
              <p className="muted">阶段完成度 {percent}%，按每阶段步骤完成情况计算评分。</p>
            </div>
          </div>

          <div className="tabs three">
            <button
              type="button"
              className={`tab${tab === "ask" ? " active" : ""}`}
              onClick={() => setTab("ask")}
            >
              AI 提问
            </button>
            <button
              type="button"
              className={`tab${tab === "plan" ? " active" : ""}`}
              onClick={() => setTab("plan")}
            >
              阶段方案
            </button>
            <button
              type="button"
              className={`tab${tab === "remind" ? " active" : ""}`}
              onClick={() => setTab("remind")}
            >
              定时督促
            </button>
          </div>

          {tab === "ask" && (
            <>
              <div className="section-head">
                <span>学习提问 · {current.questions.length}</span>
                <button
                  type="button"
                  className="mini-primary"
                  onClick={() => {
                    patch(current.id, (item) => ({
                      ...item,
                      questions: generateLearningQuestions(item.title)
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

          {tab === "plan" &&
            current.stages.map((stage) => {
              const stageDone = stage.steps.filter((step) => step.done).length;
              return (
                <div key={stage.id} className="card">
                  <div className="stage-title">
                    {stage.name}
                    <span className="muted">
                      {stageDone}/{stage.steps.length}
                    </span>
                  </div>
                  {stage.steps.map((step) => (
                    <button
                      key={step.id}
                      type="button"
                      className={`step-row${step.done ? " done" : ""}`}
                      onClick={() =>
                        patch(current.id, (item) => ({
                          ...item,
                          stages: item.stages.map((target) =>
                            target.id === stage.id
                              ? {
                                  ...target,
                                  steps: target.steps.map((one) =>
                                    one.id === step.id ? { ...one, done: !one.done } : one
                                  )
                                }
                              : target
                          )
                        }))
                      }
                    >
                      <span className="check">{step.done && <IconCheck />}</span>
                      <span>{step.text}</span>
                    </button>
                  ))}
                </div>
              );
            })}

          {tab === "remind" && (
            <>
              <p className="hint">
                下一次督促：
                {current.schedules.find((item) => !item.done)?.time ?? "暂无"} ·{" "}
                {formatDate().slice(5)}
              </p>
              <div className="schedule-form">
                <input
                  className="input time"
                  value={scheduleTime}
                  onChange={(event) => setScheduleTime(event.target.value)}
                  placeholder="20:00"
                />
                <input
                  className="input"
                  value={scheduleText}
                  onChange={(event) => setScheduleText(event.target.value)}
                  placeholder="督促内容，例如：背 20 个单词"
                />
                <button type="button" className="mini-primary" onClick={addSchedule}>
                  添加
                </button>
              </div>

              {current.schedules.map((schedule) => (
                <div key={schedule.id} className="card schedule-row">
                  <button
                    type="button"
                    className={`check${schedule.done ? " on" : ""}`}
                    onClick={() =>
                      patch(current.id, (item) => ({
                        ...item,
                        schedules: item.schedules.map((one) =>
                          one.id === schedule.id ? { ...one, done: !one.done } : one
                        )
                      }))
                    }
                  >
                    {schedule.done && <IconCheck />}
                  </button>
                  <div className="schedule-meta">
                    <strong>{schedule.text}</strong>
                    <span className="muted">每天 {schedule.time}</span>
                  </div>
                  <button
                    type="button"
                    className="icon-btn"
                    onClick={() =>
                      patch(current.id, (item) => ({
                        ...item,
                        schedules: item.schedules.filter((one) => one.id !== schedule.id)
                      }))
                    }
                    aria-label="删除督促"
                  >
                    <IconTrash />
                  </button>
                </div>
              ))}
            </>
          )}
        </div>

        <BottomSheet open={moreOpen} title="学习模块操作" onClose={() => setMoreOpen(false)}>
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
            修改学习模块
          </button>
          <button
            type="button"
            className="sheet-row danger-text"
            onClick={() => {
              setMoreOpen(false);
              setConfirmOpen(true);
            }}
          >
            <IconTrash /> 删除学习模块
          </button>
        </BottomSheet>

        <BottomSheet
          open={editOpen}
          title="修改学习模块"
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
          <label className="field-label">学习说明</label>
          <TextWithVoice value={description} onChange={setDescription} rows={3} onNotice={onNotice} />
        </BottomSheet>

        <ConfirmDialog
          open={confirmOpen}
          title="删除这个学习模块？"
          detail={`${current.title} 删除后无法恢复。`}
          onCancel={() => setConfirmOpen(false)}
          onConfirm={() => {
            setModules((list) => list.filter((item) => item.id !== current.id));
            setDetailId(null);
            setConfirmOpen(false);
            onNotice("学习模块已删除。");
          }}
        />
      </>
    );
  }

  return (
    <>
      <MobileHeader
        title="学习助手"
        onBack={onBack}
        action={
          <button type="button" className="mini-primary" onClick={() => setCreatorOpen(true)}>
            <IconPlusPlain size={16} /> 新建
          </button>
        }
      />

      <div className="page-scroll">
        <p className="page-desc">每个学习模块管理提问、阶段方案、定时督促与阶段评分。</p>
        <p className="section-label">全部学习 · {modules.length}</p>

        {modules.map((item) => {
          const value = progressOf(item.stages);
          const result = scoreOf(item.stages);
          return (
            <button
              key={item.id}
              type="button"
              className="card tap"
              onClick={() => {
                setDetailId(item.id);
                setTab("ask");
              }}
            >
              <div className="card-title-row">
                <strong>{item.title}</strong>
                <span className="percent">{result.score} 分</span>
              </div>
              <div className="progress-track slim">
                <span style={{ width: `${value}%` }} />
              </div>
              <p className="muted">
                {result.label} · 督促 {item.schedules.filter((one) => !one.done).length} 项待完成
              </p>
            </button>
          );
        })}
      </div>

      <BottomSheet
        open={creatorOpen}
        title="新建学习模块"
        onClose={() => setCreatorOpen(false)}
        footer={
          <div className="sheet-actions">
            <button type="button" className="primary-btn" onClick={createModule}>
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
          placeholder="例如：高中物理"
          onNotice={onNotice}
        />
        <label className="field-label">学习说明</label>
        <TextWithVoice
          value={description}
          onChange={setDescription}
          rows={3}
          placeholder="说明基础、目标和每天可用时间"
          onNotice={onNotice}
        />
      </BottomSheet>
    </>
  );
}
