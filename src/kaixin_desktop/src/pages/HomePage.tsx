import { useEffect, useRef, useState } from "react";
import { ModelSelector } from "../components/ModelSelector";
import { AI_MODELS, RECENT_QUESTIONS, type AiModelOption } from "../data";

interface Props {
  onNotice: (message: string) => void;
  onNavigateSkills: () => void;
}

export function HomePage({ onNotice, onNavigateSkills }: Props) {
  const [input, setInput] = useState("");
  const [model, setModel] = useState<AiModelOption>(AI_MODELS[0]);
  const panelRef = useRef<HTMLDivElement>(null);
  const [panelWidth, setPanelWidth] = useState(680);

  useEffect(() => {
    const element = panelRef.current;
    if (!element) {
      return;
    }
    const update = () => setPanelWidth(element.clientWidth);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  return (
    <div className="page home-page">
      <div className="home-wrap">
        <h1 className="home-title">开心同学 AI 助手</h1>
        <p className="home-subtitle">把需求交给智能体，让电脑真正为你工作</p>

        <div className="input-panel" ref={panelRef}>
          <textarea
            value={input}
            placeholder="给开心同学发送消息，或描述你想实现的 AI 技能…"
            onChange={(event) => setInput(event.target.value)}
          />
          <div className="panel-tools">
            <button
              type="button"
              className="icon-btn"
              title="附件"
              onClick={() => onNotice("附件上传将在功能实现阶段接入。")}
            >
              <span style={{ fontSize: 19, fontWeight: 300 }}>+</span>
            </button>
            <div className="panel-tools-right">
              <ModelSelector
                value={model}
                width={panelWidth}
                onChange={(next) => {
                  setModel(next);
                  onNotice(`已选择模型：${next.name}。`);
                }}
              />
              <button
                type="button"
                className="icon-btn"
                title="语音输入"
                onClick={() => onNotice("语音输入将在功能实现阶段接入。")}
              >
                <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
                  <path
                    d="M7.5 5.2 A1.5 1.5 0 0 1 10.5 5.2 V8.2 A1.5 1.5 0 0 1 7.5 8.2 Z M5.5 8.6 C5.5 11 7 12.6 9 12.6 C11 12.6 12.5 11 12.5 8.6 M9 12.6 V14.3 M6.9 14.3 H11.1"
                    stroke="#303632"
                    strokeWidth="1.25"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  />
                </svg>
              </button>
              <button
                type="button"
                className="icon-btn"
                title="语音波形"
                onClick={() => onNotice("语音波形展示将在功能实现阶段接入。")}
              >
                <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
                  <path
                    d="M3 8 V10 M6 5 V13 M9 3 V15 M12 6 V12 M15 8 V10"
                    stroke="#303632"
                    strokeWidth="1.4"
                    strokeLinecap="round"
                  />
                </svg>
              </button>
              <button
                type="button"
                className="send-btn"
                title="发送"
                onClick={() => {
                  if (!input.trim()) {
                    onNotice("请先输入内容，再发送。");
                    return;
                  }
                  onNotice(`已发送到「${model.name}」：${input.trim()}`);
                  setInput("");
                }}
              >
                ↑
              </button>
            </div>
          </div>
        </div>

        <div className="recent-title">最新收录待确认资料</div>
        <div className="recent-list">
          {RECENT_QUESTIONS.map((question) => (
            <button
              type="button"
              key={question}
              className="recent-item"
              onClick={onNavigateSkills}
            >
              ›&nbsp;&nbsp;{question}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
