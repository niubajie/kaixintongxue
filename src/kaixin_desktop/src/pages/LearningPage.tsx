import { useMemo, useState } from "react";

interface Props {
  onNotice: (message: string) => void;
}

interface Lesson {
  id: string;
  title: string;
  topic: string;
  minutes: number;
  state: "已完成" | "进行中" | "未开始";
}

const LESSONS: Lesson[] = [
  { id: "l1", title: "AI 基础概念", topic: "模型与提示词", minutes: 20, state: "已完成" },
  { id: "l2", title: "提示词结构拆解", topic: "提示词", minutes: 30, state: "进行中" },
  { id: "l3", title: "用 AI 整理资料", topic: "知识管理", minutes: 25, state: "未开始" },
  { id: "l4", title: "数据分析入门", topic: "数据分析", minutes: 40, state: "未开始" },
  { id: "l5", title: "口语陪练 15 分钟", topic: "语言学习", minutes: 15, state: "未开始" }
];

export function LearningPage({ onNotice }: Props) {
  const [lessons, setLessons] = useState<Lesson[]>(LESSONS);
  const [topic, setTopic] = useState("");

  const done = lessons.filter((lesson) => lesson.state === "已完成").length;
  const totalMinutes = useMemo(
    () => lessons.reduce((sum, lesson) => sum + lesson.minutes, 0),
    [lessons]
  );

  return (
    <div className="page">
      <div className="page-narrow">
        <div className="page-head">
          <div>
            <h1 className="page-title">学习助手</h1>
            <p className="page-subtitle">按主题生成学习清单，记录时长并每日复盘（演示数据）</p>
          </div>
          <button
            type="button"
            className="secondary-btn"
            onClick={() => onNotice("学习报告导出将在功能实现阶段接入。")}
          >
            导出学习记录
          </button>
        </div>

        <div className="stats" style={{ gridTemplateColumns: "repeat(3, 1fr)" }}>
          <div className="card">
            <div className="caption">今日学习时长</div>
            <div className="stat-value">65 分钟</div>
          </div>
          <div className="card">
            <div className="caption">清单完成</div>
            <div className="stat-value">
              {done} / {lessons.length}
            </div>
          </div>
          <div className="card">
            <div className="caption">计划总时长</div>
            <div className="stat-value">{totalMinutes} 分钟</div>
          </div>
        </div>

        <div className="card" style={{ marginTop: 14 }}>
          <h2 className="section-title">今天想学什么</h2>
          <div className="row" style={{ marginTop: 10 }}>
            <input
              className="input"
              placeholder="例如：提示词优化、Excel 数据分析"
              value={topic}
              onChange={(event) => setTopic(event.target.value)}
            />
            <button
              type="button"
              className="primary-btn"
              onClick={() => {
                if (!topic.trim()) {
                  onNotice("请先输入学习主题。");
                  return;
                }
                setLessons((current) => [
                  ...current,
                  {
                    id: `l${current.length + 1}`,
                    title: `学习：${topic.trim()}`,
                    topic: topic.trim(),
                    minutes: 30,
                    state: "未开始"
                  }
                ]);
                setTopic("");
                onNotice("已按演示规则加入学习清单。");
              }}
            >
              加入清单
            </button>
          </div>
        </div>

        <h2 className="section-title" style={{ marginTop: 22 }}>
          学习清单
        </h2>
        <div className="list">
          {lessons.map((lesson) => (
            <div className="list-card" key={lesson.id}>
              <div className="list-card-head">
                <div>
                  <div className="list-card-title">{lesson.title}</div>
                  <p className="list-card-detail">
                    {lesson.topic} · 预计 {lesson.minutes} 分钟
                  </p>
                </div>
                <div className="row" style={{ gap: 8 }}>
                  <span className={`tag${lesson.state === "已完成" ? " accent" : ""}`}>{lesson.state}</span>
                  {lesson.state !== "已完成" && (
                    <button
                      type="button"
                      className="ghost-btn"
                      onClick={() => {
                        setLessons((current) =>
                          current.map((item) =>
                            item.id === lesson.id ? { ...item, state: "已完成" } : item
                          )
                        );
                        onNotice(`已完成「${lesson.title}」（演示记录）。`);
                      }}
                    >
                      标记完成
                    </button>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
        <p className="caption" style={{ marginTop: 10 }}>
          当前为页面演示，学习记录不会同步到真实账号。
        </p>
      </div>
    </div>
  );
}
