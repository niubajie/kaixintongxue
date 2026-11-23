import { useCallback, useEffect, useRef, useState } from "react";
import { bridge } from "../platform";

/**
 * 实时语音（MiniCPM-o 4.5 Realtime，云端）：
 * 前端只连本机 Go sidecar 的 /api/duplex，由 sidecar 代理到 MAP 服务并注入密钥，
 * 密钥不会出现在页面里。
 *
 * 协议：session.queue_done → session.init；随后持续 input.append 音频；
 * 下行 response.output.delta 按 kind 区分 listen / text / audio。
 */

const MIC_RATE = 16000; // 上行 PCM：16kHz 单声道 float32
const SERVER_RATE = 24000; // 下行音频：24kHz 单声道 float32
const CHUNK_MS = 100; // 每包约 100ms
const BARGE_RMS = 0.08; // 本地打断阈值
const BARGE_CHUNKS = 3; // 连续 3 包（约 300ms）才算打断

interface Bubble {
  id: number;
  role: "assistant" | "user";
  text: string;
}

interface DuplexStatus {
  configured: boolean;
  model: string;
}

/** sidecar 下发的实时事件；只保留前端需要的字段。 */
interface DuplexEvent {
  type: string;
  kind?: string;
  text?: string;
  audio?: string;
  message?: string;
  position?: number;
  estimated_wait_s?: number;
  error?: { message?: string };
}

function parseDuplexEvent(raw: string): DuplexEvent | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return null;
  }
  if (typeof parsed !== "object" || parsed === null) {
    return null;
  }
  const record = parsed as Record<string, unknown>;
  if (typeof record.type !== "string" || !record.type) {
    return null;
  }
  const event: DuplexEvent = { type: record.type };
  if (typeof record.kind === "string") event.kind = record.kind;
  if (typeof record.text === "string") event.text = record.text;
  if (typeof record.audio === "string") event.audio = record.audio;
  if (typeof record.message === "string") event.message = record.message;
  if (typeof record.position === "number") event.position = record.position;
  if (typeof record.estimated_wait_s === "number") event.estimated_wait_s = record.estimated_wait_s;
  if (typeof record.error === "object" && record.error !== null) {
    const error = record.error as Record<string, unknown>;
    event.error = { message: typeof error.message === "string" ? error.message : undefined };
  }
  return event;
}

/** Float32 PCM → base64（分块拼接，避免大数组展开导致栈溢出）。 */
function float32ToBase64(samples: Float32Array): string {
  const bytes = new Uint8Array(samples.buffer, samples.byteOffset, samples.byteLength);
  let binary = "";
  const chunk = 0x2000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(binary);
}

/** base64 → Float32 PCM。 */
function base64ToFloat32(base64: string): Float32Array {
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  const count = Math.floor(bytes.length / 4);
  const samples = new Float32Array(count);
  samples.set(new Float32Array(bytes.buffer, 0, count));
  return samples;
}

/** 线性重采样；拿到设备采样率后再统一成模型要求的 16kHz。 */
function resample(input: Float32Array, srcRate: number, dstRate: number): Float32Array {
  if (srcRate === dstRate || input.length === 0) {
    return input;
  }
  const ratio = srcRate / dstRate;
  const outLength = Math.max(1, Math.round(input.length / ratio));
  const out = new Float32Array(outLength);
  for (let i = 0; i < outLength; i += 1) {
    const pos = i * ratio;
    const index = Math.floor(pos);
    const frac = pos - index;
    const a = input[index] ?? 0;
    const b = input[index + 1] ?? a;
    out[i] = a + (b - a) * frac;
  }
  return out;
}

export function VoicePage() {
  const [config, setConfig] = useState<DuplexStatus | null>(null);
  const [configError, setConfigError] = useState("");
  const [error, setError] = useState("");
  const [statusText, setStatusText] = useState("");
  const [busy, setBusy] = useState(false);
  const [bubbles, setBubbles] = useState<Bubble[]>([]);

  const wsRef = useRef<WebSocket | null>(null);
  const activeRef = useRef(false);
  const streamRef = useRef<MediaStream | null>(null);
  const micCtxRef = useRef<AudioContext | null>(null);
  const micSourceRef = useRef<MediaStreamAudioSourceNode | null>(null);
  const processorRef = useRef<ScriptProcessorNode | null>(null);
  const micSinkRef = useRef<GainNode | null>(null);
  const nativeRef = useRef<number[]>([]);
  const micRateRef = useRef(MIC_RATE);
  const loudRef = useRef(0);
  const playCtxRef = useRef<AudioContext | null>(null);
  const sourcesRef = useRef<AudioBufferSourceNode[]>([]);
  const nextStartRef = useRef(0);
  const openBubbleRef = useRef<number | null>(null);
  const bubbleSeqRef = useRef(0);
  const stageRef = useRef<HTMLDivElement | null>(null);

  // 读取 sidecar 的代理状态：没有配置 MAP_API_KEY 时如实提示，不假装可用。
  useEffect(() => {
    let alive = true;
    const load = async () => {
      try {
        const port = await bridge.getLocalServicePort();
        const response = await fetch(`http://127.0.0.1:${port}/api/duplex/status`);
        if (!response.ok) {
          throw new Error("status");
        }
        const record = (await response.json()) as Record<string, unknown>;
        if (!alive) {
          return;
        }
        setConfig({
          configured: record.configured === true,
          model: typeof record.model === "string" ? record.model : ""
        });
      } catch {
        if (alive) {
          setConfigError("未检测到本地服务：桌面端会自动启动，若仍失败请重启应用。");
        }
      }
    };
    void load();
    return () => {
      alive = false;
    };
  }, []);

  // 资源清理：麦克风、两个 AudioContext、WebSocket 统一在这里收口。
  const teardown = useCallback((notifyServer: boolean) => {
    activeRef.current = false;

    const ws = wsRef.current;
    wsRef.current = null;
    if (ws) {
      if (notifyServer && ws.readyState === WebSocket.OPEN) {
        try {
          ws.send(JSON.stringify({ type: "session.close", reason: "user_stop" }));
        } catch {
          // 连接已不可写时忽略
        }
      }
      ws.close();
    }

    const processor = processorRef.current;
    if (processor) {
      processor.onaudioprocess = null;
      processor.disconnect();
      processorRef.current = null;
    }
    micSourceRef.current?.disconnect();
    micSourceRef.current = null;
    micSinkRef.current?.disconnect();
    micSinkRef.current = null;

    if (micCtxRef.current) {
      void micCtxRef.current.close();
      micCtxRef.current = null;
    }
    if (playCtxRef.current) {
      void playCtxRef.current.close();
      playCtxRef.current = null;
    }

    streamRef.current?.getTracks().forEach((track) => track.stop());
    streamRef.current = null;

    nativeRef.current = [];
    loudRef.current = 0;
    sourcesRef.current = [];
    nextStartRef.current = 0;
    openBubbleRef.current = null;
  }, []);

  useEffect(() => {
    return () => {
      teardown(true);
    };
  }, [teardown]);

  useEffect(() => {
    const stage = stageRef.current;
    if (stage) {
      stage.scrollTop = stage.scrollHeight;
    }
  }, [bubbles]);

  const appendAssistantText = useCallback((text: string) => {
    if (openBubbleRef.current === null) {
      bubbleSeqRef.current += 1;
      const id = bubbleSeqRef.current;
      openBubbleRef.current = id;
      setBubbles((prev) => [...prev, { id, role: "assistant", text }]);
      return;
    }
    const id = openBubbleRef.current;
    setBubbles((prev) => prev.map((bubble) => (bubble.id === id ? { ...bubble, text: bubble.text + text } : bubble)));
  }, []);

  /** 立即停止所有已排队与正在播放的语音（本地打断 / 结束时用）。 */
  const stopPlayback = useCallback(() => {
    for (const source of sourcesRef.current) {
      try {
        source.stop();
      } catch {
        // 已停止的节点会抛错，忽略
      }
    }
    sourcesRef.current = [];
    nextStartRef.current = 0;
  }, []);

  const playPcm = useCallback((base64: string, sampleRate: number) => {
    const samples = base64ToFloat32(base64);
    if (samples.length === 0) {
      return;
    }
    if (!playCtxRef.current) {
      playCtxRef.current = new AudioContext();
    }
    const ctx = playCtxRef.current;
    if (ctx.state === "suspended") {
      void ctx.resume();
    }
    const buffer = ctx.createBuffer(1, samples.length, sampleRate);
    const channel = new Float32Array(samples.length);
    channel.set(samples);
    buffer.copyToChannel(channel, 0);
    const source = ctx.createBufferSource();
    source.buffer = buffer;
    source.connect(ctx.destination);
    sourcesRef.current.push(source);
    source.onended = () => {
      sourcesRef.current = sourcesRef.current.filter((item) => item !== source);
    };
    const startAt = Math.max(ctx.currentTime + 0.06, nextStartRef.current);
    source.start(startAt);
    nextStartRef.current = startAt + buffer.duration;
  }, []);

  const start = useCallback(async () => {
    if (activeRef.current) {
      return;
    }
    setError("");
    setConfigError("");
    setBubbles([]);
    setBusy(true);
    setStatusText("正在连接实时语音…");

    let initialized = false;

    const send = (payload: unknown) => {
      const ws = wsRef.current;
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(payload));
      }
    };

    try {
      const port = await bridge.getLocalServicePort();
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true }
      });
      streamRef.current = stream;

      const ws = new WebSocket(`ws://127.0.0.1:${port}/api/duplex?mode=audio`);
      wsRef.current = ws;
      activeRef.current = true;

      const startCapture = () => {
        const micStream = streamRef.current;
        if (!micStream || processorRef.current) {
          return;
        }
        // 本机 WebView2 指定 16kHz 会导致音频图不运行，这里用默认采样率再重采样。
        const ctx = new AudioContext();
        micCtxRef.current = ctx;
        const rate = ctx.sampleRate || MIC_RATE;
        micRateRef.current = rate;
        const source = ctx.createMediaStreamSource(micStream);
        micSourceRef.current = source;
        const processor = ctx.createScriptProcessor(4096, 1, 1);
        processorRef.current = processor;
        // ScriptProcessor 需要接到 destination 才会持续触发；用 0 增益避免回声。
        const sink = ctx.createGain();
        sink.gain.value = 0;
        micSinkRef.current = sink;
        source.connect(processor);
        processor.connect(sink);
        sink.connect(ctx.destination);

        const chunkNative = Math.max(1, Math.round((rate * CHUNK_MS) / 1000));

        processor.onaudioprocess = (event) => {
          const input = event.inputBuffer.getChannelData(0);
          const pending = nativeRef.current;
          let sum = 0;
          for (let i = 0; i < input.length; i += 1) {
            pending.push(input[i]);
            sum += input[i] * input[i];
          }
          const rms = Math.sqrt(sum / Math.max(1, input.length));
          // 本地打断：用户持续说话时立刻停掉正在播报的回答，避免双方抢话。
          if (rms > BARGE_RMS) {
            loudRef.current += 1;
            if (loudRef.current >= BARGE_CHUNKS && sourcesRef.current.length > 0) {
              stopPlayback();
              openBubbleRef.current = null;
            }
          } else {
            loudRef.current = 0;
          }
          while (pending.length >= chunkNative) {
            const block = Float32Array.from(pending.splice(0, chunkNative));
            const pcm = resample(block, rate, MIC_RATE);
            send({ type: "input.append", input: { audio: float32ToBase64(pcm) } });
          }
        };

        setStatusText("在听你说");
      };

      ws.onmessage = (event: MessageEvent) => {
        if (typeof event.data !== "string") {
          return;
        }
        const message = parseDuplexEvent(event.data);
        if (!message) {
          return;
        }
        switch (message.type) {
          case "session.queued":
          case "session.queue_update":
            setStatusText(
              message.position && message.position > 0
                ? `排队中，前面还有 ${message.position} 位`
                : "正在等待空闲会话…"
            );
            break;
          case "session.queue_done":
            // 服务端分配好 worker 后才能初始化会话
            send({
              type: "session.init",
              payload: {
                system_prompt: "你是开心同学，用简洁自然的中文口语回答，不要使用 Markdown。",
                config: { generate_audio: true, tts_enabled: true }
              }
            });
            break;
          case "session.created":
            if (!initialized) {
              initialized = true;
              try {
                startCapture();
              } catch {
                setError("无法开启音频采集，请检查系统麦克风设备。");
                teardown(true);
                setBusy(false);
                setStatusText("");
              }
            }
            break;
          case "response.output.delta":
            if (message.kind === "audio") {
              if (message.audio) {
                playPcm(message.audio, SERVER_RATE);
              }
            } else if (message.kind === "text") {
              if (message.text) {
                appendAssistantText(message.text);
              }
            } else if (message.kind === "listen") {
              openBubbleRef.current = null;
              setStatusText("在听你说");
            }
            break;
          case "session.closed":
            teardown(false);
            setBusy(false);
            setStatusText("会话已结束");
            break;
          case "error":
            setError(`实时语音出错：${message.error?.message ?? message.message ?? "未知错误"}`);
            break;
          default:
            break;
        }
      };

      ws.onerror = () => {
        if (activeRef.current) {
          setError("实时语音连接异常，请重试。");
        }
      };

      ws.onclose = () => {
        if (!activeRef.current) {
          return;
        }
        teardown(false);
        setBusy(false);
        setStatusText("");
        setError((prev) => prev || "实时语音连接已断开，请重新开始对话。");
      };
    } catch (err: unknown) {
      teardown(false);
      setBusy(false);
      setStatusText("");
      const reason = err instanceof Error ? err.message : "无法开启麦克风";
      if (err instanceof DOMException && err.name === "NotAllowedError") {
        setError("麦克风权限被拒绝：请在系统设置中允许开心同学使用麦克风。");
      } else {
        setError(`启动实时语音失败：${reason}`);
      }
    }
  }, [appendAssistantText, playPcm, stopPlayback, teardown]);

  const stop = useCallback(() => {
    if (!activeRef.current) {
      return;
    }
    stopPlayback();
    teardown(true);
    setBusy(false);
    setStatusText("已结束");
  }, [stopPlayback, teardown]);

  const renderBody = () => {
    if (configError) {
      return (
        <div className="card">
          <h2 className="section-title">实时语音不可用</h2>
          <p className="caption" style={{ marginTop: 10, lineHeight: 1.7 }}>
            {configError}
          </p>
        </div>
      );
    }
    if (!config) {
      return (
        <div className="card">
          <p className="caption">正在读取实时语音状态…</p>
        </div>
      );
    }
    if (!config.configured) {
      return (
        <div className="card">
          <h2 className="section-title">实时语音未配置</h2>
          <p className="caption" style={{ marginTop: 10, lineHeight: 1.8 }}>
            需要 MiniCPM-o 4.5 Realtime 的 MAP API Key：在系统环境变量里设置 <code>MAP_API_KEY</code>，
            然后重启本应用。密钥只由本机服务读取，不会出现在页面里。
          </p>
        </div>
      );
    }
    return (
      <div className="card">
        <div className="voice-stage" ref={stageRef}>
          {bubbles.length === 0 ? (
            <p className="caption">还没有字幕。点击「开始对话」，对着麦克风说话即可。</p>
          ) : (
            bubbles.map((bubble) => (
              <div key={bubble.id} className={`voice-bubble ${bubble.role}`}>
                {bubble.text}
              </div>
            ))
          )}
        </div>
        <p className="voice-status">{busy ? statusText || "正在连接…" : statusText || "准备就绪"}</p>
        {error && (
          <p className="caption" style={{ color: "#c0392b", margin: "0 0 10px" }}>
            {error}
          </p>
        )}
        <div className="voice-actions">
          <button type="button" className="primary-btn" disabled={busy} onClick={() => void start()}>
            开始对话
          </button>
          <button type="button" className="secondary-btn" disabled={!busy} onClick={stop}>
            结束
          </button>
        </div>
      </div>
    );
  };

  return (
    <div className="page page-narrow">
      <div className="page-head">
        <div>
          <h1 className="page-title">实时语音</h1>
          <p className="page-subtitle">
            {config?.model ? `${config.model}：` : "MiniCPM-o 实时语音："}说完即答，可直接插话打断。
          </p>
        </div>
      </div>
      {renderBody()}
    </div>
  );
}
