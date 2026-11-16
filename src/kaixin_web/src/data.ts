export type PageKey =
  | "home"
  | "application"
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

export interface SkillMatch {
  text: string;
  softwareId: string;
  software: string;
  reasons: string[];
  siteId?: string;
  site?: string;
  siteUrl?: string;
}

export interface SkillOption {
  id: string;
  name: string;
  description: string;
  icon: string;
  demand: number;
  /** 技能内容，一行一条；自定义技能也保存在这里。 */
  content?: string;
  /** 「配置ai软件」的匹配结果，用于生成方案。 */
  matches?: SkillMatch[];
}

export interface TaskItem {
  id: string;
  time: string;
  title: string;
  detail: string;
  category: string;
  tag: string;
  progress: number;
}

export interface GoalItem {
  id: string;
  title: string;
  detail: string;
  deadline: string;
  progress: number;
  steps: string[];
}

export interface DocItem {
  id: string;
  title: string;
  group: string;
  category: string;
  type: string;
  size: string;
  date: string;
  summary: string;
}

export interface HistoryItem {
  id: string;
  title: string;
  time: string;
  state: string;
}

export const AI_MODELS: AiModelOption[] = [
  { name: "MiniCPM-o 4.5", cost: "免费开源，仅自部署硬件成本", capability: "中等，适配电脑维修视觉问答" },
  { name: "Qwen3-Omni-Flash Realtime", cost: "新用户免费试用额度，超量每千 token 约 0.01-0.03 元", capability: "中强，画面识别速度快" },
  { name: "豆包实时语音大模型", cost: "付费，每千 token 约 0.02-0.05 元", capability: "强，维修场景逻辑适配好" },
  { name: "火山引擎方舟实时方案", cost: "付费，按分钟约 0.1-0.2 元或 token 计费", capability: "强，主板元件识别精准" },
  { name: "OpenAI Realtime API", cost: "付费，每千 token 约 0.15 美元起", capability: "顶尖，推理能力强但成本高" },
  { name: "Gemini Live API", cost: "付费，按会话时长约 0.2 美元 / 分钟起", capability: "顶尖，视觉细节表现突出" }
];

export const SKILLS: SkillOption[] = [
  {
    id: "shortvideo",
    name: "商品的短视频制作",
    description: "商品图片/视频采集、剪辑、审核与定时发布",
    icon: "短",
    demand: 3,
    content: `1、指定平台
2、获取所有商家图片的功能；
3、获取某个商品每周、每月前10销量的功能；
4、获取商品视频中指定属性图片的功能；
5、获取商品月销量的功能；
6、获取本周新上商品的功能；
7、获取本月销量大于100的功能；
8、获取最近3个月销量大于1000的功能；
9、获取包含指定内容的图片；
10、获取视频中指定内容的图片，自动剪辑图片的功能。要求：要尽量显示在图片的70%的范围内。制作好的图片需要审核，通过审查就可以保存并发布到抖音上；没通过的，手动删除或者重新提要求重做；
11、商品图片转为视频，时长5秒，每个商品至少3张图片，少于则放弃此商品制作视频。制作好的视频需要提醒和待审查，通过审查就可以保存并发布到平台上；没通过的，手动删除或者重新提要求重做。图片制作视频：第一张图片左右旋转15度，第二张图片上下旋转15度，第三张图片做局部动作；
12、可定时发布视频到账号；
13、每天分三次做抖音短视频的播放记录：包含上线时长、播放量、点赞量、评论量以及评论。每一条前面可以打勾选择，可以增加可选功能，也可以删除其中一个或者多个功能。`
  },
  { id: "kb", name: "知识库问答", description: "把本地文档接入问答，随时检索工作资料", icon: "知", demand: 2, content: "本地离线大模型问答，检索工作资料" },
  { id: "office", name: "办公自动化", description: "自动整理文档、表格与周报草稿", icon: "办", demand: 2, content: "用大模型写作周报草稿并总结文档" },
  { id: "repair", name: "电脑维修视觉问答", description: "摄像头识别主板与元件，给出维修建议", icon: "维", demand: 3, content: "用大模型问答给出维修建议" },
  { id: "voice", name: "实时语音对话", description: "低延迟语音交互，边说边执行任务", icon: "语", demand: 3, content: "语音识别转成文字，再用大模型问答" },
  { id: "image", name: "本地图片生成", description: "离线生成配图与封面草稿", icon: "图", demand: 4, content: "离线生成图片与封面" },
  { id: "code", name: "代码辅助", description: "读写本地项目，生成与修改代码", icon: "码", demand: 3, content: "用大模型辅助写作代码" },
  { id: "data", name: "数据分析", description: "清洗数据并生成图表与结论", icon: "数", demand: 2, content: "用大模型总结数据并给出结论" },
  { id: "study", name: "学习陪练", description: "制定学习计划并每日陪练复盘", icon: "学", demand: 1, content: "用大模型批改作文并总结错题" }
];

export const HARDWARE = [
  { label: "处理器", value: "i7-12700" },
  { label: "内存", value: "32 GB" },
  { label: "显卡", value: "RTX 3060 12G" },
  { label: "硬盘", value: "1 TB SSD" }
];

export interface HardwareTier {
  /** 档位名称，如「入门」「主流」。 */
  level: string;
  /** 全新组装整套价格，静态参考值。 */
  newPrice: string;
  /** 旧配件组装整套价格，静态参考值。 */
  usedPrice: string;
  cpu: string;
  gpu: string;
  memory: string;
  storage: string;
  /** 该档位综合性能等级，1 最低、5 最高，用于和技能 demand 比较。 */
  capability: number;
  summary: string;
}

/** 「AI 技能 + 硬件配置」的价格节点，与旧版桌面端保持一致。 */
export const HARDWARE_TIERS: HardwareTier[] = [
  {
    level: "入门",
    newPrice: "¥4,000",
    usedPrice: "¥2,400",
    cpu: "i3-12100F",
    gpu: "GTX 1650 4 GB",
    memory: "16 GB DDR4",
    storage: "512 GB NVMe",
    capability: 1,
    summary: "只能运行 3B 以下小模型，适合文字类技能；图像、视频与复杂推理基本依赖云端 API。"
  },
  {
    level: "基础",
    newPrice: "¥6,500",
    usedPrice: "¥3,800",
    cpu: "i5-12400F",
    gpu: "RTX 3050 8 GB",
    memory: "16 GB DDR4",
    storage: "1 TB NVMe",
    capability: 2,
    summary: "可本地运行 7B 量化模型，知识库与语音技能可用；编程、数据分析开始吃力，图像生成受限。"
  },
  {
    level: "主流",
    newPrice: "¥8,500",
    usedPrice: "¥5,200",
    cpu: "i5-13400F",
    gpu: "RTX 4060 8 GB",
    memory: "32 GB DDR4",
    storage: "1 TB NVMe",
    capability: 3,
    summary: "知识库、办公、编程与数据分析均可稳定运行；8 GB 显存限制了图像生成的分辨率与批量。"
  },
  {
    level: "进阶",
    newPrice: "¥10,750",
    usedPrice: "¥6,260",
    cpu: "i7-12700",
    gpu: "RTX 3060 12 GB",
    memory: "32 GB DDR4",
    storage: "1 TB NVMe",
    capability: 3,
    summary: "12 GB 显存可本地运行 7B~14B 模型，图像生成可用但速度一般，视频生成仍需云端完成。"
  },
  {
    level: "高性能",
    newPrice: "¥15,000",
    usedPrice: "¥9,800",
    cpu: "i7-14700F",
    gpu: "RTX 4070 Super 12 GB",
    memory: "32 GB DDR5",
    storage: "2 TB NVMe",
    capability: 4,
    summary: "14B 级模型与图像生成流畅，视频生成可用但耗时较长，适合多技能并行使用。"
  },
  {
    level: "旗舰",
    newPrice: "¥24,000",
    usedPrice: "¥16,500",
    cpu: "i9-14900K",
    gpu: "RTX 4080 Super 16 GB",
    memory: "64 GB DDR5",
    storage: "2 TB NVMe",
    capability: 5,
    summary: "本地大模型、图像与视频生成均可流畅运行，支持多任务并行与高分辨率生成。"
  }
];

export const TASKS: TaskItem[] = [
  { id: "t1", time: "09:00", title: "整理需求清单", detail: "汇总本周新增的 AI 技能需求，标注优先级", category: "工作任务", tag: "今天", progress: 100 },
  { id: "t2", time: "10:30", title: "知识库资料清洗", detail: "把 12 份 PDF 转为可检索文本", category: "工作任务", tag: "今天", progress: 60 },
  { id: "t3", time: "14:00", title: "生成周报草稿", detail: "基于本周任务记录生成周报初稿", category: "工作任务", tag: "今天", progress: 30 },
  { id: "t4", time: "16:30", title: "学习提示词模板", detail: "完成 3 个提示词模板练习", category: "学习任务", tag: "今天", progress: 0 }
];

export const GOALS: GoalItem[] = [
  {
    id: "g1",
    title: "搭建个人 AI 知识库",
    detail: "把 30 份工作文档整理进知识库，支持问答检索",
    deadline: "11 月 30 日",
    progress: 35,
    steps: ["整理文档清单", "清洗与分类", "导入知识库", "验证问答效果"]
  },
  {
    id: "g2",
    title: "每周自动生成工作周报",
    detail: "每周五汇总任务与产出，输出周报草稿",
    deadline: "每周五",
    progress: 60,
    steps: ["连接任务记录", "定义周报模板", "定时生成草稿"]
  },
  {
    id: "g3",
    title: "学会用 AI 做数据分析",
    detail: "掌握数据清洗、图表生成与结论输出",
    deadline: "12 月 20 日",
    progress: 15,
    steps: ["准备示例数据", "练习提问模板", "产出第一份分析"]
  },
  {
    id: "g4",
    title: "英语口语每日练习",
    detail: "每天 15 分钟，AI 陪练并纠正发音",
    deadline: "长期",
    progress: 80,
    steps: ["选择话题库", "每日打卡", "月度复盘"]
  }
];

export const CATEGORY_GROUPS = [
  {
    name: "工作资料",
    count: "46 份",
    icon: "工",
    children: [
      { name: "需求文档", count: "18 份" },
      { name: "项目周报", count: "14 份" },
      { name: "方案设计", count: "9 份" },
      { name: "合同资料", count: "5 份" }
    ]
  },
  {
    name: "学习资料",
    count: "32 份",
    icon: "学",
    children: [
      { name: "AI 课程", count: "12 份" },
      { name: "提示词模板", count: "9 份" },
      { name: "读书笔记", count: "11 份" }
    ]
  },
  {
    name: "健康资料",
    count: "18 份",
    icon: "健",
    children: [
      { name: "运动记录", count: "8 份" },
      { name: "体检报告", count: "4 份" },
      { name: "饮食计划", count: "6 份" }
    ]
  },
  {
    name: "生活资料",
    count: "20 份",
    icon: "生",
    children: [
      { name: "家庭账单", count: "9 份" },
      { name: "旅行计划", count: "5 份" },
      { name: "购物清单", count: "6 份" }
    ]
  },
  {
    name: "娱乐资料",
    count: "12 份",
    icon: "娱",
    children: [
      { name: "影视清单", count: "5 份" },
      { name: "音乐收藏", count: "4 份" },
      { name: "游戏攻略", count: "3 份" }
    ]
  }
];

export const DOCS: DocItem[] = [
  { id: "d1", title: "开心同学 AI 助手需求说明", group: "工作资料", category: "需求文档", type: "Markdown", size: "128 KB", date: "11-13", summary: "页面原型阶段的需求拆解，包含两种方案模式与部署流程说明。" },
  { id: "d2", title: "2026 年 Q4 第 45 周工作周报", group: "工作资料", category: "项目周报", type: "Word", size: "86 KB", date: "11-12", summary: "本周完成页面原型与部署演示，下周进入功能实现阶段。" },
  { id: "d3", title: "页面原型与交互方案设计", group: "工作资料", category: "方案设计", type: "PDF", size: "1.8 MB", date: "11-11", summary: "首页、设置页与部署流程的交互说明与视觉规范。" },
  { id: "d4", title: "本地大模型部署手册", group: "学习资料", category: "AI 课程", type: "PDF", size: "2.4 MB", date: "11-10", summary: "Ollama 与 AnythingLLM 的安装、模型选择与常见故障排查。" },
  { id: "d5", title: "提示词模板合集", group: "学习资料", category: "提示词模板", type: "Markdown", size: "64 KB", date: "11-08", summary: "整理 18 个常用提示词模板，覆盖写作、分析与编程场景。" },
  { id: "d6", title: "数据分析入门笔记", group: "学习资料", category: "读书笔记", type: "Markdown", size: "38 KB", date: "11-04", summary: "数据清洗、可视化与结论输出的基础方法。" },
  { id: "d7", title: "11 月体检报告", group: "健康资料", category: "体检报告", type: "PDF", size: "1.2 MB", date: "11-09", summary: "血常规、肝肾功能与视力检查结果汇总。" },
  { id: "d8", title: "每周运动记录", group: "健康资料", category: "运动记录", type: "Excel", size: "46 KB", date: "11-13", summary: "近 8 周跑步与力量训练的距离、时长与心率统计。" },
  { id: "d9", title: "家庭月度账单", group: "生活资料", category: "家庭账单", type: "Excel", size: "92 KB", date: "11-12", summary: "10 月家庭收支明细与分类占比统计。" },
  { id: "d10", title: "周末短途旅行计划", group: "生活资料", category: "旅行计划", type: "Word", size: "33 KB", date: "11-07", summary: "两天一夜行程、住宿与预算安排。" },
  { id: "d11", title: "年度观影清单", group: "娱乐资料", category: "影视清单", type: "Markdown", size: "28 KB", date: "11-05", summary: "2026 年已看影片评分与待看片单。" },
  { id: "d12", title: "游戏攻略合集", group: "娱乐资料", category: "游戏攻略", type: "Markdown", size: "52 KB", date: "11-03", summary: "主线任务流程与隐藏要素收集提示。" }
];

export const HISTORY: HistoryItem[] = [
  { id: "h1", title: "电脑维修视觉问答方案", time: "今天 14:30", state: "已完成" },
  { id: "h2", title: "搭建个人 AI 知识库", time: "昨天 16:20", state: "进行中" },
  { id: "h3", title: "每周自动生成工作周报", time: "11 月 13 日 09:15", state: "已完成" }
];

export const RECENT_QUESTIONS = [
  "最近7天，短视频都在流行什么",
  "把开箱流程整理成 SOP 清单",
  "第一次用 AI 办公，可以先做哪3件事",
  "10个主流 AI 模型该怎么选"
];

export const PERMISSIONS = [
  { resource: "问答与技能运行", owner: "可读写", member: "可读写", guest: "只读" },
  { resource: "目标与任务数据", owner: "全部成员", member: "仅本人", guest: "只读" },
  { resource: "本地资料文档", owner: "可读写", member: "可读写", guest: "只读" },
  { resource: "成员与权限管理", owner: "可管理", member: "不可", guest: "不可" },
  { resource: "软件安装与系统设置", owner: "可管理", member: "不可", guest: "不可" },
  { resource: "数据库备份与恢复", owner: "可管理", member: "不可", guest: "不可" }
];
