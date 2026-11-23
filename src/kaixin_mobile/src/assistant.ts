import { useEffect, useRef, useState } from "react";

export interface DiaryEntry {
  id: string;
  title: string;
  content: string;
  createdAt: string;
}

export interface MaterialItem {
  id: string;
  name: string;
  size: string;
  uploadedAt: string;
  /** 新上传/新提炼的资料为待确认，确认后才算正式资料 */
  pending?: boolean;
  /** 关键词区：便于检索，自动从文件名提取，可手工增删 */
  keywords?: string[];
}

/** 应用分类：L1 顶部的工作 / 生活 / 学习标签 */
export type AppCategory = "work" | "life" | "study";

/** 应用内的计划任务：有未完成任务的应用会出现在「正在运行」 */
export interface AppTask {
  id: string;
  text: string;
  done: boolean;
}

export interface AppModule {
  id: string;
  title: string;
  category: AppCategory;
  diaries: DiaryEntry[];
  materials: MaterialItem[];
  tasks: AppTask[];
  /** 子应用：内容与主应用完全一致（笔记 + 资料库 + 计划任务） */
  children?: AppModule[];
}

function isAppCategory(value: unknown): value is AppCategory {
  return value === "work" || value === "life" || value === "study";
}

/** 旧数据迁移：缺分类默认「生活」，缺任务默认空数组，子应用递归归一。 */
export function normalizeApps(input: unknown): AppModule[] {
  if (!Array.isArray(input)) {
    return [];
  }
  const apps: AppModule[] = [];
  for (const item of input) {
    if (!item || typeof item !== "object") {
      continue;
    }
    const raw = item as Record<string, unknown>;
    if (typeof raw.id !== "string" || typeof raw.title !== "string") {
      continue;
    }
    const tasks: AppTask[] = Array.isArray(raw.tasks)
      ? (raw.tasks as unknown[])
          .filter((task): task is Record<string, unknown> => Boolean(task) && typeof task === "object")
          .filter((task) => typeof task.id === "string" && typeof task.text === "string")
          .map((task) => ({ id: String(task.id), text: String(task.text), done: task.done === true }))
      : [];
    apps.push({
      id: raw.id,
      title: raw.title,
      category: isAppCategory(raw.category) ? raw.category : "life",
      diaries: Array.isArray(raw.diaries) ? (raw.diaries as DiaryEntry[]) : [],
      materials: Array.isArray(raw.materials) ? (raw.materials as MaterialItem[]) : [],
      tasks,
      children: Array.isArray(raw.children) ? normalizeApps(raw.children) : undefined
    });
  }
  return apps;
}

export interface PlanStep {
  id: string;
  text: string;
  done: boolean;
}

export interface PlanStage {
  id: string;
  name: string;
  steps: PlanStep[];
}

export interface ScheduleItem {
  id: string;
  time: string;
  text: string;
  done: boolean;
}

export interface AssistantGoal {
  id: string;
  title: string;
  description: string;
  questions: string[];
  stages: PlanStage[];
}

export interface LearningModule {
  id: string;
  title: string;
  description: string;
  questions: string[];
  stages: PlanStage[];
  schedules: ScheduleItem[];
}

export function createId(prefix: string): string {
  return `${prefix}${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
}

export function formatDate(value = new Date()): string {
  const pad = (input: number) => String(input).padStart(2, "0");
  return `${value.getFullYear()}-${pad(value.getMonth() + 1)}-${pad(value.getDate())} ${pad(
    value.getHours()
  )}:${pad(value.getMinutes())}`;
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

export function progressOf(stages: PlanStage[]): number {
  const steps = stages.flatMap((stage) => stage.steps);
  if (steps.length === 0) {
    return 0;
  }
  return Math.round((steps.filter((step) => step.done).length / steps.length) * 100);
}

/** 阶段学习评分：按完成度换算成百分制，并给出对应评价。 */
export function scoreOf(stages: PlanStage[]): { score: number; label: string } {
  const score = progressOf(stages);
  if (score >= 90) {
    return { score, label: "优秀" };
  }
  if (score >= 70) {
    return { score, label: "良好" };
  }
  if (score >= 40) {
    return { score, label: "合格" };
  }
  return { score, label: "待提升" };
}

function makeStage(name: string, steps: string[]): PlanStage {
  return {
    id: createId("st"),
    name,
    steps: steps.map((text) => ({ id: createId("sp"), text, done: false }))
  };
}

export function generateGoalQuestions(title: string): string[] {
  return [
    `「${title}」达成时，用什么具体指标判断成功？`,
    `目前离「${title}」的最大差距是什么？`,
    `每天能稳定投入多少时间，安排在哪个时段？`,
    `可以借助哪些人、工具或资源？`,
    `最可能让计划中断的两个风险是什么？`
  ];
}

export function generateLearningQuestions(title: string): string[] {
  return [
    `你现在的「${title}」基础处于什么水平？`,
    `每周能安排几次学习，每次多长时间？`,
    `学完「${title}」要达到什么可检验的目标？`,
    `希望用做题、复述还是实操来检验掌握程度？`,
    `遇到卡点时，希望 AI 怎样提醒和讲解？`
  ];
}

export function buildGoalPlan(title: string): PlanStage[] {
  return [
    makeStage("第一阶段 · 明确现状", [
      `写下「${title}」的起点数据与成功标准`,
      "列出已有资源与主要缺口",
      "确定每日可执行的最小动作"
    ]),
    makeStage("第二阶段 · 稳定执行", [
      "把最小动作排入固定时段",
      "每周记录一次执行情况",
      "根据反馈调整节奏"
    ]),
    makeStage("第三阶段 · 复盘冲刺", [
      "对照成功标准逐项检查",
      "补齐未达标项",
      "输出最终复盘与经验沉淀"
    ])
  ];
}

export function buildLearningPlan(title: string): PlanStage[] {
  return [
    makeStage("第一阶段 · 基础打底", [
      `梳理「${title}」的核心概念清单`,
      "完成基础练习并标记错点",
      "用一句话复述每个概念"
    ]),
    makeStage("第二阶段 · 专项训练", [
      "按薄弱点做专项练习",
      "每完成一组记录正确率",
      "把错题整理成复习清单"
    ]),
    makeStage("第三阶段 · 综合检验", [
      "完成一次限时综合测试",
      "对照目标检查掌握程度",
      "生成下一阶段学习计划"
    ])
  ];
}

/** 本地持久化：数据只存本机，刷新或重启后仍然保留。 */
export function usePersistentState<T>(key: string, initial: T) {
  const [value, setValue] = useState<T>(() => {
    try {
      const raw = localStorage.getItem(key);
      if (!raw) {
        return initial;
      }
      return JSON.parse(raw) as T;
    } catch {
      return initial;
    }
  });

  useEffect(() => {
    try {
      localStorage.setItem(key, JSON.stringify(value));
    } catch {
      // 存储不可用时忽略，页面仍可正常使用
    }
  }, [key, value]);

  return [value, setValue] as const;
}

interface SpeechRecognitionLike {
  lang: string;
  continuous: boolean;
  interimResults: boolean;
  start: () => void;
  stop: () => void;
  onresult: ((event: SpeechResultLike) => void) | null;
  onerror: (() => void) | null;
  onend: (() => void) | null;
}

interface SpeechResultLike {
  resultIndex: number;
  results: ArrayLike<{ isFinal: boolean; 0: { transcript: string } }>;
}

type SpeechCtor = new () => SpeechRecognitionLike;

/** 语音录入：识别结果直接转成文字追加到输入框。 */
export function useSpeechToText(onText: (text: string) => void) {
  const [listening, setListening] = useState(false);
  const [supported] = useState(() => {
    const win = window as unknown as { SpeechRecognition?: SpeechCtor; webkitSpeechRecognition?: SpeechCtor };
    return Boolean(win.SpeechRecognition ?? win.webkitSpeechRecognition);
  });
  const recognitionRef = useRef<SpeechRecognitionLike | null>(null);
  const callbackRef = useRef(onText);
  callbackRef.current = onText;

  useEffect(() => {
    const win = window as unknown as { SpeechRecognition?: SpeechCtor; webkitSpeechRecognition?: SpeechCtor };
    const Ctor = win.SpeechRecognition ?? win.webkitSpeechRecognition;
    if (!Ctor) {
      return;
    }
    const recognition = new Ctor();
    recognition.lang = "zh-CN";
    recognition.continuous = true;
    recognition.interimResults = false;
    recognition.onresult = (event) => {
      let text = "";
      for (let index = event.resultIndex; index < event.results.length; index += 1) {
        const result = event.results[index];
        if (result.isFinal) {
          text += result[0].transcript;
        }
      }
      if (text.trim()) {
        callbackRef.current(text.trim());
      }
    };
    recognition.onerror = () => setListening(false);
    recognition.onend = () => setListening(false);
    recognitionRef.current = recognition;
    return () => {
      recognition.onresult = null;
      recognition.onerror = null;
      recognition.onend = null;
      try {
        recognition.stop();
      } catch {
        // 忽略停止异常
      }
      recognitionRef.current = null;
    };
  }, []);

  const toggle = () => {
    const recognition = recognitionRef.current;
    if (!recognition) {
      return false;
    }
    if (listening) {
      recognition.stop();
      setListening(false);
      return true;
    }
    try {
      recognition.start();
      setListening(true);
    } catch {
      setListening(false);
    }
    return true;
  };

  return { supported, listening, toggle };
}

export const SAMPLE_APPS: AppModule[] = [
  {
    id: "app-repair",
    title: "电脑维修",
    category: "work",
    diaries: [
      {
        id: "d-repair-1",
        title: "修一台进水的笔记本",
        content: "拆机后发现主板南桥附近有水渍，先用无水酒精清洗并烘干，静置 12 小时后通电正常。",
        createdAt: "2026-11-13 20:15"
      },
      {
        id: "d-repair-2",
        title: "客户反馈开机蓝屏",
        content: "蓝屏代码 0x0000007B，判断为硬盘模式设置问题，进 BIOS 改 AHCI 后恢复正常。",
        createdAt: "2026-11-10 09:40"
      }
    ],
    materials: [
      { id: "m-repair-1", name: "主板元件识别对照表.pdf", size: "1.6 MB", uploadedAt: "2026-11-12 10:20" },
      { id: "m-repair-2", name: "常见蓝屏代码速查.md", size: "48 KB", uploadedAt: "2026-11-08 15:05" }
    ],
    tasks: [
      { id: "t-repair-1", text: "整理本月主板故障案例", done: false },
      { id: "t-repair-2", text: "更新蓝屏代码速查表", done: true }
    ]
  },
  {
    id: "app-love",
    title: "恋爱宝典",
    category: "life",
    diaries: [
      {
        id: "d-love-1",
        title: "第一次约会的聊天复盘",
        content: "少问连环问题，多聊对方最近在做的事，结尾自然约定下次。",
        createdAt: "2026-11-09 22:05"
      }
    ],
    materials: [{ id: "m-love-1", name: "沟通话题清单.md", size: "22 KB", uploadedAt: "2026-11-09 21:40" }],
    tasks: [{ id: "t-love-1", text: "整理 10 个约会话题", done: false }]
  },
  { id: "app-massage", title: "推拿按摩", category: "life", diaries: [], materials: [], tasks: [] },
  { id: "app-herb", title: "古法单方", category: "life", diaries: [], materials: [], tasks: [] },
  {
    id: "app-basketball",
    title: "篮球训练",
    category: "life",
    diaries: [
      {
        id: "d-ball-1",
        title: "投篮手型练习",
        content: "三分线内一步定点 100 球，命中 63 球；手肘外翻问题比上周改善。",
        createdAt: "2026-11-14 18:30"
      }
    ],
    materials: [],
    tasks: [{ id: "t-ball-1", text: "本周完成 3 次投篮训练", done: false }]
  },
  { id: "app-daily", title: "日常妙法", category: "life", diaries: [], materials: [], tasks: [] },
  { id: "app-exam", title: "高中试题", category: "study", diaries: [], materials: [], tasks: [] },
  { id: "app-child", title: "小孩教育", category: "study", diaries: [], materials: [], tasks: [] }
];

export const SAMPLE_GOALS: AssistantGoal[] = [
  {
    id: "goal-weight",
    title: "暴瘦20斤",
    description: "三个月内减重 20 斤，保持肌肉量，形成可持续的饮食与运动习惯。",
    questions: generateGoalQuestions("暴瘦20斤"),
    stages: buildGoalPlan("暴瘦20斤")
  },
  {
    id: "goal-money",
    title: "年赚100万计划",
    description: "一年内通过主业提升与副业组合，实现可核算的 100 万收入。",
    questions: generateGoalQuestions("年赚100万计划"),
    stages: buildGoalPlan("年赚100万计划")
  },
  {
    id: "goal-marry",
    title: "1年把自己嫁出去",
    description: "一年内认真经营关系，找到价值观契合、愿意共同生活的伴侣。",
    questions: generateGoalQuestions("1年把自己嫁出去"),
    stages: buildGoalPlan("1年把自己嫁出去")
  }
];

export const SAMPLE_LEARNING: LearningModule[] = [
  {
    id: "learn-english",
    title: "初中英语",
    description: "补齐初中英语语法与词汇，能独立完成中考难度阅读与写作。",
    questions: generateLearningQuestions("初中英语"),
    stages: buildLearningPlan("初中英语"),
    schedules: [
      { id: "sc-en-1", time: "每天 07:30", text: "背 20 个单词并复习昨天错词", done: false },
      { id: "sc-en-2", time: "每周一 / 三 / 五 20:00", text: "语法专项练习 30 分钟", done: false },
      { id: "sc-en-3", time: "每周日 19:00", text: "完成一套阅读限时训练并复盘", done: false }
    ]
  },
  {
    id: "learn-math",
    title: "初中数学",
    description: "掌握代数与几何核心题型，错题能独立归因。",
    questions: generateLearningQuestions("初中数学"),
    stages: buildLearningPlan("初中数学"),
    schedules: [
      { id: "sc-ma-1", time: "每天 19:30", text: "完成 10 道计算题保持手感", done: false },
      { id: "sc-ma-2", time: "每周六 10:00", text: "整理本周错题并重做", done: false }
    ]
  },
  {
    id: "learn-c",
    title: "C语言",
    description: "从语法到指针，能独立写出结构清晰的小程序。",
    questions: generateLearningQuestions("C语言"),
    stages: buildLearningPlan("C语言"),
    schedules: [
      { id: "sc-c-1", time: "每周二 / 四 21:00", text: "跟着教程写 1 个小程序", done: false },
      { id: "sc-c-2", time: "每周日 15:00", text: "复盘本周代码并整理笔记", done: false }
    ]
  }
];
