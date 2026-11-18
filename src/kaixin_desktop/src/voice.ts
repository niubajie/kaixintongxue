// 语音播报：部署进度与故障用中文播报（FR-SYS-04）。
// 用系统自带的 speechSynthesis，不联网、不引入依赖；播报失败静默忽略，绝不阻塞操作。

const VOICE_KEY = "kaixin.voiceBroadcast";

export function isVoiceEnabled(): boolean {
  // 需求是「默认播报、可静音」，所以没有存过设置时按开启处理
  try {
    const stored = localStorage.getItem(VOICE_KEY);
    return stored === null ? true : stored === "true";
  } catch {
    return true;
  }
}

export function setVoiceEnabled(enabled: boolean): void {
  try {
    localStorage.setItem(VOICE_KEY, enabled ? "true" : "false");
  } catch {
    // 存储不可用时只影响开关持久化，不影响本次会话
  }
  if (!enabled) {
    stopSpeaking();
  }
}

export function voiceSupported(): boolean {
  return typeof window !== "undefined" && "speechSynthesis" in window;
}

/** 播报一句中文。开关关闭、环境不支持或正在播报时都不打断当前操作。 */
export function speak(text: string): void {
  if (!text || !isVoiceEnabled() || !voiceSupported()) {
    return;
  }
  try {
    const synth = window.speechSynthesis;
    // 队列过长会让播报滞后，超过两条就直接替换
    if (synth.pending || synth.speaking) {
      synth.cancel();
    }
    const utterance = new SpeechSynthesisUtterance(text);
    utterance.lang = "zh-CN";
    utterance.rate = 1.05;
    utterance.volume = 1;
    const zh = synth.getVoices().find((voice) => voice.lang.toLowerCase().startsWith("zh"));
    if (zh) {
      utterance.voice = zh;
    }
    synth.speak(utterance);
  } catch {
    // 播报失败不影响部署本身
  }
}

export function stopSpeaking(): void {
  if (!voiceSupported()) {
    return;
  }
  try {
    window.speechSynthesis.cancel();
  } catch {
    // 忽略
  }
}
