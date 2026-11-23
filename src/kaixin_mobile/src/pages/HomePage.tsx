import { useState } from "react";
import { useSpeechToText } from "../assistant";
import { AI_MODELS, RECENT_QUESTIONS } from "../data";
import type { View } from "../App";
import {
  IconCamera,
  IconHeadset,
  IconMenu,
  IconPlus,
  IconSpeaker,
  IconWave
} from "../components/Icons";

interface Props {
  onOpen: (view: View) => void;
  onOpenDrawer: () => void;
  onNotice: (message: string) => void;
}

const QUICK: { view: View; label: string }[] = [
  { view: "apps", label: "应用助手" },
  { view: "voice", label: "实时语音" }
];

export function HomePage({ onOpen, onOpenDrawer, onNotice }: Props) {
  const [draft, setDraft] = useState("");
  const [model, setModel] = useState(AI_MODELS[0].name);
  const [modelOpen, setModelOpen] = useState(false);
  const [speak, setSpeak] = useState(false);
  const { supported, listening, toggle } = useSpeechToText((text) => {
    setDraft((prev) => (prev ? `${prev} ${text}` : text));
    onNotice(`语音已转文字：${text}`);
  });

  const send = (text: string) => {
    const value = text.trim();
    if (!value) {
      onNotice("请先输入内容或按住说话。");
      return;
    }
    setDraft("");
    onNotice(`已发送：${value.slice(0, 18)}${value.length > 18 ? "…" : ""}（演示）`);
  };

  return (
    <div className="home">
      <header className="home-head">
        <button type="button" className="icon-btn" onClick={onOpenDrawer} aria-label="打开菜单">
          <IconMenu />
        </button>
        <span className="head-spacer" />
        <button
          type="button"
          className="icon-btn"
          onClick={() => onNotice("语音通话将在正式版接入。")}
          aria-label="语音通话"
        >
          <IconHeadset />
        </button>
        <button
          type="button"
          className={`icon-btn accent${speak ? " on" : ""}`}
          onClick={() => {
            setSpeak((prev) => !prev);
            onNotice(speak ? "已关闭语音播报。" : "已开启语音播报，回答会自动朗读。");
          }}
          aria-label="语音播报"
        >
          <IconSpeaker />
        </button>
      </header>

      <div className="home-body">
        <div className="greeting">
          <h1>AI 助你一臂之力！</h1>
          <p>说一句需求，开心同学帮你拆成可执行的技能与步骤</p>
        </div>

        <div className="pending">
          <p className="section-label">最新待确认资料</p>
          {RECENT_QUESTIONS.map((question) => (
            <button
              key={question}
              type="button"
              className="pending-pill"
              onClick={() => setDraft(question)}
            >
              {question}
            </button>
          ))}
        </div>

        <div className="quick-row">
          {QUICK.map((item) => (
            <button key={item.view} type="button" className="quick-chip" onClick={() => onOpen(item.view)}>
              {item.label}
            </button>
          ))}
        </div>
      </div>

      {modelOpen && (
        <>
          <div className="model-mask" onClick={() => setModelOpen(false)} />
          <div className="model-sheet">
            <div className="sheet-head">
              <strong>选择模型</strong>
              <button type="button" className="link-btn" onClick={() => setModelOpen(false)}>
                收起
              </button>
            </div>
            {AI_MODELS.map((item) => (
              <button
                key={item.name}
                type="button"
                className={`model-row${model === item.name ? " active" : ""}`}
                onClick={() => {
                  setModel(item.name);
                  setModelOpen(false);
                  onNotice(`已选择模型：${item.name}`);
                }}
              >
                <span className="model-name">{item.name}</span>
                <span className="model-cost">{item.cost}</span>
                <span className="model-cap">{item.capability}</span>
              </button>
            ))}
          </div>
        </>
      )}

      <div className="composer-wrap">
        <button
          type="button"
          className="composer-model"
          onClick={() => setModelOpen((prev) => !prev)}
        >
          {model} <span className="caret">▾</span>
        </button>
        <div className="composer">
          <button
            type="button"
            className="icon-btn"
            onClick={() => onNotice("拍照识图将在正式版接入。")}
            aria-label="拍照"
          >
            <IconCamera />
          </button>
          <input
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            placeholder={listening ? "正在听，请说话…" : "发消息或按住说话…"}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                send(draft);
              }
            }}
          />
          <button
            type="button"
            className={`icon-btn${listening ? " recording" : ""}`}
            onClick={() => {
              if (!supported) {
                onNotice("当前环境不支持语音识别，请先用打字输入。");
                return;
              }
              toggle();
            }}
            aria-label="语音输入"
          >
            <IconWave />
          </button>
          <button type="button" className="composer-plus" onClick={() => send(draft)} aria-label="发送">
            <IconPlus />
          </button>
        </div>
      </div>
    </div>
  );
}
