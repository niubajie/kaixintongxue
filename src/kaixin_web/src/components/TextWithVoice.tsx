import { useSpeechToText } from "../assistant";

interface Props {
  value: string;
  onChange: (value: string) => void;
  rows?: number;
  placeholder?: string;
  onNotice: (message: string) => void;
}

/** 支持打字与语音录入的文本框，语音识别后直接转成文字追加。 */
export function TextWithVoice({ value, onChange, rows = 4, placeholder, onNotice }: Props) {
  const { supported, listening, toggle } = useSpeechToText((text) => {
    const next = value ? `${value}${/\s$/.test(value) ? "" : " "}${text}` : text;
    onChange(next);
    onNotice(`语音已转文字：${text}`);
  });

  return (
    <div>
      <textarea
        className="textarea"
        rows={rows}
        placeholder={placeholder}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
      <div className="row" style={{ marginTop: 8, justifyContent: "space-between" }}>
        <span className="caption">
          {listening ? "正在听，请说话…" : "可以直接打字，也可以点右侧按钮语音录入"}
        </span>
        <button
          type="button"
          className={`secondary-btn voice-btn${listening ? " listening" : ""}`}
          onClick={() => {
            if (!supported) {
              onNotice("当前运行环境不支持语音识别，请先用打字录入。");
              return;
            }
            toggle();
          }}
        >
          {listening ? "停止录音" : "🎤 语音录入"}
        </button>
      </div>
    </div>
  );
}
