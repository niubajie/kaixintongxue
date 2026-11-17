import { useEffect, useRef, useState } from "react";
import { ModelSelector } from "../components/ModelSelector";
import { AI_MODELS, RECENT_QUESTIONS, type AiModelOption } from "../data";
import { bridge } from "../platform";
import { fetchASRStatus, transcribeAudio } from "../deployApi";

interface Props {
  onNotice: (message: string) => void;
  onNavigateSkills: () => void;
}

function baseName(path: string): string {
  const parts = path.split(/[\\/]/);
  return parts[parts.length - 1] || path;
}

export function HomePage({ onNotice, onNavigateSkills }: Props) {
  const [input, setInput] = useState("");
  const [model, setModel] = useState<AiModelOption>(AI_MODELS[0]);
  const panelRef = useRef<HTMLDivElement>(null);
  const [panelWidth, setPanelWidth] = useState(680);

  const [attachments, setAttachments] = useState<string[]>([]);
  const [recording, setRecording] = useState(false);
  const [seconds, setSeconds] = useState(0);
  const [wave, setWave] = useState<number[]>([]);
  const [voiceHint, setVoiceHint] = useState("");
  const [transcribing, setTranscribing] = useState(false);
  const [asrReady, setAsrReady] = useState<boolean | null>(null);

  const recorderRef = useRef<MediaRecorder | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const audioCtxRef = useRef<AudioContext | null>(null);
  const rafRef = useRef(0);
  const timerRef = useRef(0);
  const chunksRef = useRef<Blob[]>([]);

  // 启动时探测本地转写是否就绪（二进制 + 模型 + ffmpeg）
  useEffect(() => {
    let alive = true;
    fetchASRStatus()
      .then((status) => {
        if (alive) {
          setAsrReady(status.available);
        }
      })
      .catch(() => {
        if (alive) {
          setAsrReady(null);
        }
      });
    return () => {
      alive = false;
    };
  }, []);

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

  // 组件卸载时释放麦克风与音频上下文，避免设备一直被占用
  useEffect(() => {
    return () => {
      stopTracks();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function stopTracks() {
    window.clearInterval(timerRef.current);
    cancelAnimationFrame(rafRef.current);
    streamRef.current?.getTracks().forEach((track) => track.stop());
    streamRef.current = null;
    void audioCtxRef.current?.close();
    audioCtxRef.current = null;
  }

  const pickAttachments = async () => {
    try {
      const files = await bridge.pickFiles();
      if (files.length === 0) {
        return;
      }
      setAttachments((current) => [...new Set([...current, ...files])]);
      onNotice(`已添加 ${files.length} 个附件。`);
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "选择附件失败");
    }
  };

  const removeAttachment = (path: string) => {
    setAttachments((current) => current.filter((item) => item !== path));
  };

  const startRecording = async () => {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      streamRef.current = stream;

      const ctx = new AudioContext();
      audioCtxRef.current = ctx;
      const source = ctx.createMediaStreamSource(stream);
      const analyser = ctx.createAnalyser();
      analyser.fftSize = 256;
      source.connect(analyser);
      const buffer = new Uint8Array(analyser.frequencyBinCount);

      const tick = () => {
        analyser.getByteTimeDomainData(buffer);
        let sum = 0;
        for (let i = 0; i < buffer.length; i += 1) {
          const v = (buffer[i] - 128) / 128;
          sum += v * v;
        }
        const level = Math.min(1, Math.sqrt(sum / buffer.length) * 6);
        setWave((current) => [...current.slice(-39), level]);
        rafRef.current = requestAnimationFrame(tick);
      };
      tick();

      chunksRef.current = [];
      const recorder = new MediaRecorder(stream);
      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) {
          chunksRef.current.push(event.data);
        }
      };
      recorder.start();
      recorderRef.current = recorder;

      setSeconds(0);
      setVoiceHint("");
      setRecording(true);
      timerRef.current = window.setInterval(() => setSeconds((s) => s + 1), 1000);
    } catch (err: unknown) {
      onNotice(
        err instanceof Error && err.name === "NotAllowedError"
          ? "没有麦克风权限，请在系统设置里允许本应用使用麦克风。"
          : "无法开始录音，请检查麦克风是否可用。"
      );
    }
  };

  const stopRecording = () => {
    recorderRef.current?.stop();
    recorderRef.current = null;
    stopTracks();
    setRecording(false);
    setWave([]);
    const chunks = chunksRef.current;
    chunksRef.current = [];
    if (chunks.length === 0) {
      setVoiceHint(`已录音 ${seconds} 秒，但没有采集到音频数据。`);
      return;
    }
    const blob = new Blob(chunks, { type: "audio/webm" });
    setTranscribing(true);
    setVoiceHint(`已录音 ${seconds} 秒，正在本地转写…`);

    const reader = new FileReader();
    reader.onloadend = () => {
      const dataUrl = typeof reader.result === "string" ? reader.result : "";
      const base64 = dataUrl.includes(",") ? dataUrl.split(",")[1] : "";
      if (!base64) {
        setTranscribing(false);
        setVoiceHint("音频读取失败。");
        return;
      }
      transcribeAudio(base64, "zh")
        .then((result) => {
          const text = result.text.trim();
          if (text) {
            setInput((current) => (current ? `${current} ${text}` : text));
            setVoiceHint(`已转写（${Math.round(result.elapsedMs / 100) / 10} 秒，本地 Whisper）：${text}`);
            onNotice("语音已转成文字，并插入输入框。");
          } else {
            setVoiceHint("转写完成，但没有识别到语音内容。");
          }
        })
        .catch((err: unknown) => {
          setVoiceHint(err instanceof Error ? err.message : "转写失败");
        })
        .finally(() => setTranscribing(false));
    };
    reader.readAsDataURL(blob);
  };

  const send = () => {
    if (!input.trim() && attachments.length === 0) {
      onNotice("请先输入内容，再发送。");
      return;
    }
    if (attachments.length > 0) {
      onNotice(
        `已提交：${input.trim().slice(0, 20) || "（无文字）"}，附带 ${attachments.length} 个附件。模型调用尚未接入。`
      );
    } else {
      onNotice("已提交。模型调用尚未接入，当前仅记录输入。");
    }
    setInput("");
    setAttachments([]);
    setVoiceHint("");
  };

  return (
    <div className="page home-page">
      <div className="home-wrap">
        <h1 className="home-title">开心同学 AI 助手</h1>
        <p className="home-subtitle">把需求交给智能体，让电脑真正为你工作</p>

        <div className="input-panel" ref={panelRef}>
          {attachments.length > 0 && (
            <div className="row" style={{ flexWrap: "wrap", gap: 6, padding: "2px 0 6px" }}>
              {attachments.map((path) => (
                <span
                  key={path}
                  className="tag"
                  style={{ display: "inline-flex", alignItems: "center", gap: 6 }}
                  title={path}
                >
                  附件：{baseName(path)}
                  <button
                    type="button"
                    onClick={() => removeAttachment(path)}
                    style={{ border: "none", background: "transparent", cursor: "pointer", padding: 0 }}
                    aria-label={`移除 ${baseName(path)}`}
                  >
                    ✕
                  </button>
                </span>
              ))}
            </div>
          )}

          <textarea
            value={input}
            placeholder="给开心同学发送消息，或描述你想实现的 AI 技能…"
            onChange={(event) => setInput(event.target.value)}
          />

          {recording && (
            <div className="row" style={{ alignItems: "center", gap: 8, padding: "2px 0 6px" }}>
              <span className="tag accent">录音中 {seconds}s</span>
              <div className="row" style={{ alignItems: "flex-end", gap: 2, height: 22 }}>
                {wave.map((level, index) => (
                  <span
                    key={index}
                    style={{
                      width: 3,
                      height: `${Math.max(3, level * 22)}px`,
                      background: "var(--accent)",
                      borderRadius: 2,
                      opacity: 0.35 + level * 0.65
                    }}
                  />
                ))}
              </div>
            </div>
          )}

          {transcribing && <p className="caption" style={{ padding: "0 2px 6px" }}>正在转写…</p>}

          {voiceHint && (
            <p className="caption" style={{ padding: "0 2px 6px" }}>{voiceHint}</p>
          )}

          <div className="panel-tools">
            <button type="button" className="icon-btn" title="添加附件" onClick={() => void pickAttachments()}>
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
                title={recording ? "停止录音" : asrReady === false ? "语音输入（本地转写不可用，仅录音）" : "语音输入"}
                onClick={() => (recording ? stopRecording() : void startRecording())}
                style={recording ? { background: "var(--accent-light)" } : undefined}
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
                onClick={() =>
                  onNotice(
                    recording
                      ? "波形正在实时显示。"
                      : "波形只在录音时显示，点麦克风开始录音。"
                  )
                }
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
              <button type="button" className="send-btn" title="发送" onClick={send}>
                <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
                  <path
                    d="M3.2 9 L14.6 3.4 L11.8 14.6 L8.9 10.4 Z"
                    stroke="#fff"
                    strokeWidth="1.3"
                    strokeLinejoin="round"
                    strokeLinecap="round"
                  />
                </svg>
              </button>
            </div>
          </div>
        </div>

        <p className="caption" style={{ marginTop: 18 }}>
          最近提问（点一下填入输入框；带「技能」的直接跳到技能部署）
        </p>
        <div className="row" style={{ marginTop: 8, gap: 8, flexWrap: "wrap" }}>
          {RECENT_QUESTIONS.map((item) => (
            <button
              key={item}
              type="button"
              className="ghost-btn"
              onClick={() => {
                if (item.includes("技能")) {
                  onNavigateSkills();
                  return;
                }
                setInput(item);
              }}
            >
              {item}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
