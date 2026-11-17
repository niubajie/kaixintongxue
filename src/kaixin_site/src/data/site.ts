// 站点配置集中在这里：改文案、改备案号、改下载地址都不用翻页面。
// 未确定的值保留空串，页面会显示「未配置」而不是编一个假的。

export const SITE = {
  name: "开心同学 AI 助手",
  tagline: "把需求交给智能体，让电脑真正为你工作",
  description:
    "开心同学 AI 助手是一款 Windows 桌面 AI 智能体：用自然语言描述需求，自动检测硬件、匹配技能、生成部署方案并执行；支持本地资料知识库、离线语音转写。",
  /** 线上域名，部署后替换 */
  url: "https://kaixintongxue.example.com",
  /** ICP 备案号：备案通过后填入，页面会自动带链接 */
  icp: "",
  /** 公安备案号（可选） */
  police: "",
  contactEmail: "",
  copyrightFrom: 2026
} as const;

export const DOWNLOAD = {
  /** 安装包地址；留空时下载页显示「尚未发布」，不会给出坏链接 */
  windowsUrl: "",
  version: "0.5.0",
  size: "约 12 MB（含内置运行时则更大）",
  releasedAt: "2026-11-17",
  requirements: [
    "Windows 10 64 位（1809 及以上）或 Windows 11",
    "4 GB 内存起，8 GB 以上建议",
    "首次启动需要联网下载运行组件",
    "WebView2 运行时（Win11 自带，Win10 可能需装）"
  ]
} as const;

export interface ChangelogEntry {
  version: string;
  date: string;
  items: string[];
}

export const CHANGELOG: ChangelogEntry[] = [
  {
    version: "0.5.0",
    date: "2026-11-17",
    items: [
      "本地资料改为读取真实文件夹，支持选择任意目录",
      "新增本地知识库：文本切片入库与关键词检索",
      "语音输入接入本地 Whisper，转写不上传",
      "应用助手支持子应用、资料待确认与关键词",
      "系统设置可配置服务器并测试连通性"
    ]
  },
  {
    version: "0.4.0",
    date: "2026-11-13",
    items: [
      "本地 SQLite 数据库：自定义技能、部署记录、设置持久化",
      "技能部署支持取消与进度回传",
      "技能部署页显示「需要访问的网站」并可一键直达"
    ]
  },
  {
    version: "0.3.0",
    date: "2026-11-10",
    items: [
      "技能部署三步流程：硬件检测 → 技能匹配 → 方案生成",
      "系统设置与版本更新页面",
      "单实例保护与独立 WebView2 数据目录"
    ]
  }
];

export interface FaqItem {
  question: string;
  answer: string;
}

export const FAQ: FaqItem[] = [
  {
    question: "安装时 Windows 提示「已保护你的电脑」怎么办？",
    answer:
      "这是因为安装包还没有代码签名证书。点「更多信息」→「仍要运行」即可。我们不建议长期保持未签名，正式版会补上签名；在那之前请只从本站下载。"
  },
  {
    question: "数据会被上传吗？",
    answer:
      "默认只有本机数据：资料、笔记、技能与部署记录都存在本地。语音转写走本机 Whisper，音频不出本机。只有在你自己配置了云端模型或服务器时，相关请求才会离开电脑，界面会明确提示。"
  },
  {
    question: "为什么需要联网？",
    answer:
      "首次启动要下载运行组件（如 ffmpeg、Whisper 等按需组件）。此外，使用云端大模型时需要网络。纯本地能力（资料浏览、知识库检索、语音转写）在组件就绪后可离线使用。"
  },
  {
    question: "支持 macOS 或手机吗？",
    answer:
      "当前只发布了 Windows 桌面版。网页版与手机/平板版在规划中，同一套页面代码通过平台抽象层复用，但目前没有可下载的版本。"
  },
  {
    question: "部署技能时会自动帮我点网站上的按钮吗？",
    answer:
      "发布类操作默认只做到「打开正确的页面并填好内容」，最后一步需要你自己点。只有纯读取类操作才会自动化。这是为了避免误操作造成不可挽回的后果。"
  }
];
