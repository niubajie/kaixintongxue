export type PageKey =
  | "home"
  | "application"
  | "voice"
  | "goal"
  | "learning"
  | "library"
  | "skills"
  | "system"
  | "update"
  | "history";

export interface AiModelOption {
  name: string;
  cost: string;
  capability: string;
}

export const AI_MODELS: AiModelOption[] = [
  { name: "MiniCPM-o 4.5", cost: "免费开源，仅自部署硬件成本", capability: "中等，适配电脑维修视觉问答" },
  { name: "Qwen3-Omni-Flash Realtime", cost: "新用户免费试用额度，超量每千 token 约 0.01-0.03 元", capability: "中强，画面识别速度快" },
  { name: "豆包实时语音大模型", cost: "付费，每千 token 约 0.02-0.05 元", capability: "强，维修场景逻辑适配好" },
  { name: "火山引擎方舟实时方案", cost: "付费，按分钟约 0.1-0.2 元或 token 计费", capability: "强，主板元件识别精准" },
  { name: "OpenAI Realtime API", cost: "付费，每千 token 约 0.15 美元起", capability: "顶尖，推理能力强但成本高" },
  { name: "Gemini Live API", cost: "付费，按会话时长约 0.2 美元 / 分钟起", capability: "顶尖，视觉细节表现突出" }
];

export const PERMISSIONS = [
  { resource: "问答与技能运行", owner: "可读写", member: "可读写", guest: "只读" },
  { resource: "目标与任务数据", owner: "全部成员", member: "仅本人", guest: "只读" },
  { resource: "本地资料文档", owner: "可读写", member: "可读写", guest: "只读" },
  { resource: "成员与权限管理", owner: "可管理", member: "不可", guest: "不可" },
  { resource: "软件安装与系统设置", owner: "可管理", member: "不可", guest: "不可" },
  { resource: "数据库备份与恢复", owner: "可管理", member: "不可", guest: "不可" }
];
