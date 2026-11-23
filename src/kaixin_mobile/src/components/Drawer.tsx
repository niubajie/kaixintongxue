import type { View } from "../App";
import {
  IconBell,
  IconChat,
  IconCloud,
  IconClose,
  IconFolder,
  IconGear,
  IconHeadset,
  IconMonitor,
  IconRefresh,
  IconScan,
  IconSearch
} from "./Icons";

interface Props {
  open: boolean;
  current: View;
  onClose: () => void;
  onNavigate: (view: View) => void;
  onNewChat: () => void;
  onNotice: (message: string) => void;
}

const NAV: { view: View; label: string; icon: JSX.Element }[] = [
  { view: "local", label: "本地资料", icon: <IconFolder /> },
  { view: "voice", label: "实时语音", icon: <IconHeadset /> },
  { view: "settings", label: "系统设置", icon: <IconGear /> },
  { view: "version", label: "版本更新", icon: <IconRefresh /> }
];

const RECENT: { title: string; cloud: boolean }[] = [
  { title: "AI 大模型能力分类及收费分析", cloud: true },
  { title: "电脑关机提醒", cloud: false },
  { title: "技能节点数量优化", cloud: false },
  { title: "github 注册问题", cloud: false },
  { title: "检查电脑端联网阻断", cloud: true },
  { title: "完善软件方案", cloud: true },
  { title: "寻找 MAS 下载链接", cloud: true },
  { title: "电脑蓝屏排查记录", cloud: false }
];

export function Drawer({ open, current, onClose, onNavigate, onNewChat, onNotice }: Props) {
  return (
    <div className={`drawer-layer${open ? " open" : ""}`} aria-hidden={!open}>
      <div className="drawer-mask" onClick={onClose} />

      <aside className="drawer">
        <div className="drawer-top">
          <span className="drawer-brand">开心同学</span>
          <button
            type="button"
            className="icon-btn"
            onClick={() => onNotice("搜索对话将在正式版接入。")}
            aria-label="搜索"
          >
            <IconSearch />
          </button>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="收起菜单">
            <IconClose />
          </button>
        </div>

        <button type="button" className="drawer-new" onClick={onNewChat}>
          <IconChat />
          <span>新对话</span>
        </button>

        <nav className="drawer-nav">
          {NAV.map((item) => (
            <button
              key={item.view}
              type="button"
              className={`drawer-nav-item${current === item.view ? " active" : ""}`}
              onClick={() => onNavigate(item.view)}
            >
              <span className="drawer-nav-icon">{item.icon}</span>
              <span>{item.label}</span>
            </button>
          ))}
        </nav>

        <div className="drawer-divider" />

        <div className="drawer-section-head">
          <span>最近</span>
          <button type="button" className="more-dots" onClick={() => onNavigate("recent")} aria-label="全部最近">
            ···
          </button>
        </div>

        <div className="drawer-recent">
          {RECENT.map((item) => (
            <button
              key={item.title}
              type="button"
              className="recent-row"
              onClick={() => onNavigate("recent")}
            >
              <span className="recent-icon">
                {item.cloud ? <IconCloud /> : <IconMonitor />}
              </span>
              <span className="recent-title">{item.title}</span>
            </button>
          ))}
        </div>

        <button type="button" className="back-to-chat" onClick={onNewChat}>
          <span className="back-avatar">开</span>
          <span>回到主对话</span>
          <span className="back-badge">1</span>
        </button>

        <footer className="drawer-user">
          <span className="user-avatar">1</span>
          <span className="user-meta">
            <strong>1hao</strong>
            <em>免费体验</em>
          </span>
          <span className="user-actions">
            <button type="button" onClick={() => onNotice("扫一扫将在正式版接入。")}>
              <IconScan />
            </button>
            <button type="button" onClick={() => onNotice("暂无新的通知。")}>
              <IconBell />
            </button>
            <button type="button" onClick={() => onNavigate("settings")}>
              <IconGear />
            </button>
          </span>
        </footer>
      </aside>
    </div>
  );
}
