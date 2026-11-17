import { useEffect, useState } from "react";
import {
  buildLearningPlan,
  createId,
  generateLearningQuestions,
  progressOf,
  SAMPLE_LEARNING,
  scoreOf,
  usePersistentState,
  type LearningModule
} from "../assistant";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { TextWithVoice } from "../components/TextWithVoice";

interface Props {
  onNotice: (message: string) => void;
}

export function LearningPage({ onNotice }: Props) {
  const [modules, setModules] = usePersistentState<LearningModule[]>("kaixin.learning.v1", SAMPLE_LEARNING);
  const [selectedId, setSelectedId] = useState(modules[0]?.id ?? "");

  const [notifyState, setNotifyState] = useState<string>("");

  // 定时提醒：每 30 秒检查一次，命中「每天 HH:MM」就提醒一次。
  // 只在应用运行期间生效；关掉应用后的提醒需要 Windows 计划任务，另行实现。
  useEffect(() => {
    const firedKey = "kaixin.reminders.fired";
    const readFired = (): Record<string, string> => {
      try {
        return JSON.parse(localStorage.getItem(firedKey) ?? "{}") as Record<string, string>;
      } catch {
        return {};
      }
    };

    const tick = () => {
      const now = new Date();
      const hhmm = `${String(now.getHours()).padStart(2, "0")}:${String(now.getMinutes()).padStart(2, "0")}`;
      const today = now.toISOString().slice(0, 10);
      const fired = readFired();
      let changed = false;

      for (const module of modules) {
        for (const schedule of module.schedules ?? []) {
          const match = /(\d{1,2})[:：](\d{2})/.exec(schedule.time ?? "");
          if (!match) {
            continue;
          }
          const target = `${match[1].padStart(2, "0")}:${match[2]}`;
          if (target !== hhmm) {
            continue;
          }
          const key = `${module.id}|${today}|${target}`;
          if (fired[key]) {
            continue;
          }
          fired[key] = new Date().toISOString();
          changed = true;

          const title = `该学习「${module.title}」了`;
          const body = schedule.text || "按计划完成今天的一步。";
          if (typeof Notification !== "undefined" && Notification.permission === "granted") {
            try {
              new Notification(title, { body });
            } catch {
              // 部分环境不支持构造函数式通知，退回应用内提示
            }
          }
          onNotice(`${title}：${body}`);
        }
      }

      if (changed) {
        try {
          localStorage.setItem(firedKey, JSON.stringify(fired));
        } catch {
          // 存储满时忽略，最多重复提醒一次
        }
      }
    };

    tick();
    const timer = window.setInterval(tick, 30000);
    return () => window.clearInterval(timer);
  }, [modules, onNotice]);

  const requestNotify = async () => {
    if (typeof Notification === "undefined") {
      setNotifyState("当前环境不支持系统通知，将只在应用内提醒");
      onNotice("当前环境不支持系统通知，将只在应用内提醒。");
      return;
    }
    try {
      const permission = await Notification.requestPermission();
      setNotifyState(
        permission === "granted"
          ? "系统通知已开启"
          : permission === "denied"
            ? "系统通知被拒绝，可稍后到浏览器/系统设置里开启"
            : "未授权系统通知"
      );
      onNotice(permission === "granted" ? "系统通知已开启。" : "系统通知未开启，仍会在应用内提醒。");
    } catch {
      setNotifyState("申请通知权限失败");
    }
  };



  const [formOpen, setFormOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");

  const [scheduleOpen, setScheduleOpen] = useState(false);
  const [scheduleTime, setScheduleTime] = useState("每天 20:00");
  const [scheduleText, setScheduleText] = useState("");

  const [confirmModule, setConfirmModule] = useState<LearningModule | null>(null);
  const [confirmSchedule, setConfirmSchedule] = useState<{ id: string; text: string } | null>(null);

  const selected = modules.find((item) => item.id === selectedId) ?? modules[0];

  const updateSelected = (updater: (item: LearningModule) => LearningModule) => {
    if (!selected) {
      return;
    }
    setModules((current) => current.map((item) => (item.id === selected.id ? updater(item) : item)));
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
      onNotice("请填写学习模块标题名称。");
      return;
    }
    if (editingId) {
      setModules((current) =>
        current.map((item) =>
          item.id === editingId
            ? { ...item, title: title.trim(), description: description.trim() }
            : item
        )
      );
      onNotice("学习模块已保存修改。");
    } else {
      const next: LearningModule = {
        id: createId("learn"),
        title: title.trim(),
        description: description.trim(),
        questions: [],
        stages: [],
        schedules: []
      };
      setModules((current) => [...current, next]);
      setSelectedId(next.id);
      onNotice(`已新建学习模块「${next.title}」。`);
    }
    setFormOpen(false);
  };

  const generateQuestions = () => {
    if (!selected) {
      return;
    }
    updateSelected((item) => ({ ...item, questions: generateLearningQuestions(item.title) }));
    onNotice("已生成 5 个用于明确学习目标的提问。");
  };

  const generatePlan = () => {
    if (!selected) {
      return;
    }
    updateSelected((item) => ({ ...item, stages: buildLearningPlan(item.title) }));
    onNotice("已生成三阶段学习方案，完成度会换算成绩。");
  };

  const toggleStep = (stageId: string, stepId: string) => {
    updateSelected((item) => ({
      ...item,
      stages: item.stages.map((stage) =>
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

  const addSchedule = () => {
    if (!selected) {
      return;
    }
    if (!scheduleText.trim()) {
      onNotice("请填写定时任务内容。");
      return;
    }
    updateSelected((item) => ({
      ...item,
      schedules: [
        ...item.schedules,
        {
          id: createId("sc"),
          time: scheduleTime.trim() || "每天 20:00",
          text: scheduleText.trim(),
          done: false
        }
      ]
    }));
    setScheduleOpen(false);
    setScheduleText("");
    onNotice("已添加定时任务，到点会提醒你学习和记录进度。");
  };

  const toggleSchedule = (scheduleId: string) => {
    updateSelected((item) => ({
      ...item,
      schedules: item.schedules.map((schedule) =>
        schedule.id === scheduleId ? { ...schedule, done: !schedule.done } : schedule
      )
    }));
  };

  const removeModule = () => {
    if (!confirmModule) {
      return;
    }
    setModules((current) => current.filter((item) => item.id !== confirmModule.id));
    onNotice(`已删除学习模块「${confirmModule.title}」。`);
    setConfirmModule(null);
  };

  const removeSchedule = () => {
    if (!confirmSchedule) {
      return;
    }
    updateSelected((item) => ({
      ...item,
      schedules: item.schedules.filter((schedule) => schedule.id !== confirmSchedule.id)
    }));
    onNotice("已删除定时任务。");
    setConfirmSchedule(null);
  };

  const progress = selected ? progressOf(selected.stages) : 0;
  const score = selected ? scoreOf(selected.stages) : { score: 0, label: "待提升" };
  const nextSchedule = selected?.schedules.find((schedule) => !schedule.done);

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="page-title">学习助手</h1>
          <p className="page-subtitle">每个学习模块独立管理提问、方案、定时任务与阶段评分</p>
        </div>
        <button type="button" className="secondary-btn" onClick={openCreate}>
          + 新建学习模块
        </button>
      </div>

      <div className="assistant-layout">
        <div className="assistant-side">
          <div className="side-title">
            全部学习 <span className="caption">{modules.length}</span>
          </div>
          <div className="list" style={{ marginTop: 8 }}>
            {modules.map((item) => (
              <div key={item.id} className={`side-item-wrap${item.id === selected?.id ? " selected" : ""}`}>
                <button type="button" className="side-item" onClick={() => setSelectedId(item.id)}>
                  <span className="side-item-title">{item.title}</span>
                  <span className="side-item-meta">
                    {item.stages.length > 0 ? `评分 ${scoreOf(item.stages).score} 分` : "尚未生成方案"}
                  </span>
                </button>
                <button
                  type="button"
                  className="side-item-remove"
                  title="删除学习模块"
                  onClick={() => setConfirmModule(item)}
                >
                  ×
                </button>
              </div>
            ))}
          </div>
          {modules.length === 0 && <div className="empty">还没有学习模块，点击右上角新建。</div>}
        </div>

        <div className="assistant-main">
          {selected ? (
            <>
              <div className="detail-head">
                <div>
                  <h2 className="detail-title">{selected.title}</h2>
                  <p className="list-card-detail">
                    {selected.description || "还没有填写学习说明，可点击“修改”补充。"}
                  </p>
                </div>
                <div className="row" style={{ gap: 8 }}>
                  <button type="button" className="secondary-btn" onClick={openEdit}>
                    修改
                  </button>
                  <button
                    type="button"
                    className="ghost-btn danger-text"
                    onClick={() => setConfirmModule(selected)}
                  >
                    删除
                  </button>
                </div>
              </div>

              <div className="stats" style={{ gridTemplateColumns: "repeat(3, 1fr)", marginTop: 14 }}>
                <div className="card">
                  <div className="caption">阶段完成度</div>
                  <div className="stat-value">{progress}%</div>
                </div>
                <div className="card">
                  <div className="caption">阶段评分</div>
                  <div className="stat-value">
                    {score.score} <span className="caption">{score.label}</span>
                  </div>
                </div>
                <div className="card">
                  <div className="caption">下一次督促</div>
                  <div className="stat-value" style={{ fontSize: 14 }}>
                    {nextSchedule ? nextSchedule.time : "暂无定时任务"}
                  </div>
                </div>
              </div>

              <div className="card" style={{ marginTop: 12 }}>
                <div className="row" style={{ justifyContent: "space-between" }}>
                  <h3 className="section-title">AI 生成提问</h3>
                  <button type="button" className="secondary-btn" onClick={generateQuestions}>
                    {selected.questions.length > 0 ? "重新生成提问" : "生成提问"}
                  </button>
                </div>
                {selected.questions.length > 0 ? (
                  <div style={{ marginTop: 10 }}>
                    {selected.questions.map((question, index) => (
                      <div className="question-item" key={question}>
                        <span className="question-index">{index + 1}</span>
                        <span>{question}</span>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="caption" style={{ marginTop: 10 }}>
                    点击“生成提问”，AI 会先问清基础、时间与检验方式。
                  </p>
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
                  <div style={{ marginTop: 12 }}>
                    {selected.stages.map((stage) => {
                      const stageDone = stage.steps.filter((step) => step.done).length;
                      return (
                        <div className="stage" key={stage.id}>
                          <div className="stage-head">
                            <span className="stage-name">{stage.name}</span>
                            <span className="tag">
                              {stageDone}/{stage.steps.length} · 得分{" "}
                              {Math.round((stageDone / stage.steps.length) * 100)}
                            </span>
                          </div>
                          {stage.steps.map((step) => (
                            <label className="check-row" key={step.id}>
                              <input
                                type="checkbox"
                                checked={step.done}
                                onChange={() => toggleStep(stage.id, step.id)}
                              />
                              <span className={step.done ? "done-text" : ""}>{step.text}</span>
                            </label>
                          ))}
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  <p className="caption" style={{ marginTop: 10 }}>
                    点击“自动建方案”，AI 会把学习目标拆成三个阶段并可逐项勾选。
                  </p>
                )}
              </div>

              <div className="card" style={{ marginTop: 12 }}>
                <div className="row" style={{ justifyContent: "space-between" }}>
                  <h3 className="section-title">定时任务</h3>
                  <button type="button" className="secondary-btn" onClick={() => setScheduleOpen(true)}>
                    + 新增定时任务
                  </button>
                </div>
                <div style={{ marginTop: 6 }}>
                  {selected.schedules.map((schedule) => (
                    <div className="schedule-row" key={schedule.id}>
                      <input
                        type="checkbox"
                        checked={schedule.done}
                        onChange={() => toggleSchedule(schedule.id)}
                      />
                      <span className="tag accent">{schedule.time}</span>
                      <button
                        type="button"
                        className="ghost-btn"
                        style={{ marginLeft: 6 }}
                        onClick={() => void requestNotify()}
                      >
                        {notifyState || "开启系统通知"}
                      </button>
                      <span className={schedule.done ? "done-text" : ""} style={{ flex: 1 }}>
                        {schedule.text}
                      </span>
                      <button
                        type="button"
                        className="ghost-btn danger-text"
                        onClick={() => setConfirmSchedule({ id: schedule.id, text: schedule.text })}
                      >
                        删除
                      </button>
                    </div>
                  ))}
                  {selected.schedules.length === 0 && (
                    <p className="caption" style={{ marginTop: 10 }}>
                      还没有定时任务，添加后可按时间督促学习并匹配进度。
                    </p>
                  )}
                </div>
              </div>
            </>
          ) : (
            <div className="empty">请选择或新建一个学习模块。</div>
          )}
        </div>
      </div>

      {formOpen && (
        <div className="dialog-mask" onClick={() => setFormOpen(false)}>
          <div className="dialog" onClick={(event) => event.stopPropagation()}>
            <h2 className="dialog-title">{editingId ? "修改学习模块" : "新建学习模块"}</h2>
            <p className="caption" style={{ marginTop: 7 }}>
              新建后包含“AI 生成提问”“自动建方案”，并支持定时任务与阶段评分。
            </p>
            <label className="field-label">标题名称</label>
            <input
              className="input"
              placeholder="例如 初中英语、初中数学、C语言"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
            />
            <label className="field-label">学习说明</label>
            <TextWithVoice
              value={description}
              onChange={setDescription}
              rows={4}
              placeholder="学到什么程度，每周能投入多少时间…"
              onNotice={onNotice}
            />
            <div className="row" style={{ gap: 6, marginTop: 12 }}>
              <span className="tag accent">AI 生成提问</span>
              <span className="tag accent">自动建方案</span>
              <span className="tag accent">定时督促</span>
            </div>
            <div className="dialog-actions">
              <button type="button" className="secondary-btn" onClick={() => setFormOpen(false)}>
                取消
              </button>
              <button type="button" className="primary-btn" onClick={save}>
                {editingId ? "保存修改" : "创建模块"}
              </button>
            </div>
          </div>
        </div>
      )}

      {scheduleOpen && (
        <div className="dialog-mask" onClick={() => setScheduleOpen(false)}>
          <div className="dialog dialog-small" onClick={(event) => event.stopPropagation()}>
            <h2 className="dialog-title">新增定时任务</h2>
            <label className="field-label">提醒时间</label>
            <input
              className="input"
              placeholder="例如 每天 20:00"
              value={scheduleTime}
              onChange={(event) => setScheduleTime(event.target.value)}
            />
            <label className="field-label">任务内容</label>
            <TextWithVoice
              value={scheduleText}
              onChange={setScheduleText}
              rows={3}
              placeholder="例如 背 20 个单词并复习错词"
              onNotice={onNotice}
            />
            <div className="dialog-actions">
              <button type="button" className="secondary-btn" onClick={() => setScheduleOpen(false)}>
                取消
              </button>
              <button type="button" className="primary-btn" onClick={addSchedule}>
                添加
              </button>
            </div>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={confirmModule !== null}
        title={`删除学习模块「${confirmModule?.title ?? ""}」？`}
        detail="该模块的提问、方案与定时任务会一起删除，删除后不可恢复。"
        onCancel={() => setConfirmModule(null)}
        onConfirm={removeModule}
      />

      <ConfirmDialog
        open={confirmSchedule !== null}
        title="删除这个定时任务？"
        detail={confirmSchedule?.text}
        onCancel={() => setConfirmSchedule(null)}
        onConfirm={removeSchedule}
      />
    </div>
  );
}
