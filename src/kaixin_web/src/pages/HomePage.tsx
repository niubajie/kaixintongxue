import { useEffect, useRef, useState } from "react";
import { ModelSelector } from "../components/ModelSelector";
import { AI_MODELS, type AiModelOption } from "../data";
import { bridge } from "../platform";
import { fetchASRStatus, streamChat, summarizeChat, transcribeAudio, type ChatResult } from "../deployApi";

interface Props {
  onNotice: (message: string) => void;
  onNavigateApps: () => void;
}


// 首页底部「最新待确认资料」：从应用助手里读待确认的资料（含子应用）。
// 数据源与「应用助手」同一个存储键，避免两处各存一份。
interface PendingMaterial {
  key: string;
  name: string;
  appTitle: string;
}

function collectPendingMaterials(): PendingMaterial[] {
  try {
    const raw = localStorage.getItem("kaixin.apps.v1");
    if (!raw) {
      return [];
    }
    const apps = JSON.parse(raw) as {
      id: string;
      title: string;
      materials?: { id: string; name: string; pending?: boolean; uploadedAt?: string }[];
      children?: { id: string; title: string; materials?: { id: string; name: string; pending?: boolean }[] }[];
    }[];
    const out: { item: PendingMaterial; uploadedAt: string }[] = [];
    const push = (appTitle: string, materials?: { id: string; name: string; pending?: boolean; uploadedAt?: string }[]) => {
      for (const material of materials ?? []) {
        if (material.pending) {
          out.push({
            item: { key: `${appTitle}-${material.id}`, name: material.name, appTitle },
            uploadedAt: material.uploadedAt ?? ""
          });
        }
      }
    };
    for (const app of apps) {
      push(app.title, app.materials);
      for (const child of app.children ?? []) {
        push(child.title, child.materials);
      }
    }
    out.sort((a, b) => (a.uploadedAt < b.uploadedAt ? 1 : -1));
    return out.slice(0, 6).map((entry) => entry.item);
  } catch {
    return [];
  }
}

function baseName(path: string): string {
  const parts = path.split(/[\\/]/);
  return parts[parts.length - 1] || path;
}

export function HomePage({ onNotice, onNavigateApps }: Props) {
  const [input, setInput] = useState("");
  const [model, setModel] = useState<AiModelOption>(AI_MODELS[0]);
  const panelRef = useRef<HTMLDivElement>(null);
  const [panelWidth, setPanelWidth] = useState(680);

  const [attachments, setAttachments] = useState<string[]>([]);
  const [pendingMaterials, setPendingMaterials] = useState<PendingMaterial[]>([]);
  const [messages, setMessages] = useState<{ role: "user" | "assistant" | "system"; content: string }[]>([]);
  const [compressing, setCompressing] = useState(false);
  const [sending, setSending] = useState(false);
  const [lastResult, setLastResult] = useState<ChatResult | null>(null);
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
    setPendingMaterials(collectPendingMaterials());
  }, []);

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

  const send = async () => {
    if (!input.trim() && attachments.length === 0) {
      onNotice("请先输入内容，再发送。");
      return;
    }
    const text = input.trim();
    if (!text) {
      onNotice("附件已附上，请在输入框补充一句需求再发送。");
      return;
    }

    const history = [...messages, { role: "user" as const, content: text }];
    // 先放一条空的助手消息，边生成边填内容（流式）
    const withPlaceholder = [...history, { role: "assistant" as const, content: "" }];
    setMessages(withPlaceholder);
    setInput("");
    setSending(true);
    setLastResult(null);
    setAttachments([]);

    const appendDelta = (delta: string) => {
      setMessages((current) => {
        const next = [...current];
        const last = next[next.length - 1];
        if (last && last.role === "assistant") {
          next[next.length - 1] = { ...last, content: last.content + delta };
        }
        return next;
      });
    };

    let fullText = "";
    try {
      await streamChat(
        history,
        (delta) => {
          fullText += delta;
          appendDelta(delta);
        },
        (meta) => {
          setLastResult({
            text: "",
            model: meta.model,
            promptTokens: meta.promptTokens,
            completionTokens: meta.completionTokens,
            costCents: meta.costCents,
            elapsedMs: meta.elapsedMs,
            note: meta.estimated ? "token 为估算值（模型未返回用量）" : ""
          });
          onNotice(`已由 ${meta.model} 回答，耗时 ${(meta.elapsedMs / 1000).toFixed(1)} 秒。`);
        }
      );
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "调用模型失败";
      setMessages((current) => {
        const next = [...current];
        const last = next[next.length - 1];
        if (last && last.role === "assistant" && !last.content) {
          next[next.length - 1] = { ...last, content: `（调用失败）${message}` };
        }
        return next;
      });
      onNotice(message);
    } finally {
      setSending(false);
    }

    // 对话超过 7 轮就把早期部分压成摘要，避免越长越慢
    const finished = [...history, { role: "assistant" as const, content: fullText }];
    void compressContext(finished);
  };

  // compressContext 把前一半对话换成一条摘要，保留最近 8 条
  const compressContext = async (list: { role: string; content: string }[]) => {
    if (list.length < 14) {
      return;
    }
    const keep = 8;
    const older = list.slice(0, list.length - keep);
    const recent = list.slice(list.length - keep);
    // 已经压缩过一次就不再重复压缩同一段
    if (list[0]?.role === "system") {
      return;
    }
    setCompressing(true);
    try {
      const result = await summarizeChat(older);
      setMessages([
        { role: "system", content: `（更早对话摘要）${result.summary}` },
        ...recent.map((item) => ({
          role: item.role === "assistant" ? ("assistant" as const) : ("user" as const),
          content: item.content
        }))
      ]);
      onNotice(`已把早期 ${older.length} 条对话压缩成摘要（${result.model}）。`);
    } catch {
      // 压缩失败不影响继续聊，后端还有滑动窗口兜底
    } finally {
      setCompressing(false);
    }
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
              <button type="button" className="send-btn" title="发送" onClick={() => void send()}
                disabled={sending}>
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

        {(messages.length > 0 || sending) && (
          <div className="chat-log">
            {messages.map((message, index) => (
              <div key={index} className={`chat-msg ${message.role === "system" ? "system" : message.role}`}>
                <span className="chat-role">{message.role === "user" ? "我" : message.role === "system" ? "摘要" : "AI"}</span>
                <div className="chat-text">{message.content}</div>
              </div>
            ))}
            {sending && <p className="caption">正在生成…（内容会边生成边显示）</p>}
            {compressing && <p className="caption">正在压缩早期对话…</p>}
            <div className="row" style={{ gap: 8, alignItems: "center" }}>
              <span className="caption">上下文 {messages.length} 条</span>
              <button
                type="button"
                className="ghost-btn"
                onClick={() => {
                  setMessages([]);
                  setLastResult(null);
                  onNotice("已清空对话。");
                }}
              >
                清空对话
              </button>
            </div>
            {lastResult && (
              <p className="caption chat-meta">
                模型 {lastResult.model} · 输入 {lastResult.promptTokens} / 输出 {lastResult.completionTokens} tokens ·
                耗时 {(lastResult.elapsedMs / 1000).toFixed(1)}s · 费用
                {lastResult.costCents > 0 ? ` ¥${(lastResult.costCents / 100).toFixed(4)}` : " ¥0"}
                {lastResult.note ? `（${lastResult.note}）` : ""}
              </p>
            )}
          </div>
        )}

        <p className="caption" style={{ marginTop: 18 }}>
          最新待确认资料{pendingMaterials.length > 0 ? `（${pendingMaterials.length} 份）` : ""}
        </p>
        {pendingMaterials.length === 0 ? (
          <p className="caption" style={{ marginTop: 6 }}>
            暂无待确认资料。在「应用助手」上传或从笔记提炼资料后会出现在这里。
          </p>
        ) : (
          <div className="row" style={{ marginTop: 8, gap: 8, flexWrap: "wrap" }}>
            {pendingMaterials.map((item) => (
              <button
                key={item.key}
                type="button"
                className="ghost-btn"
                title={`${item.appTitle} · ${item.name}`}
                onClick={() => onNavigateApps()}
              >
                {item.name}
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
