import { MobileHeader } from "../components/MobileHeader";
import { PERMISSIONS } from "../data";

interface Props {
  onBack: () => void;
  onNotice: (message: string) => void;
}

export function SettingsPage({ onBack, onNotice }: Props) {
  return (
    <>
      <MobileHeader title="系统设置" onBack={onBack} />

      <div className="page-scroll">
        <div className="card account-card">
          <span className="user-avatar">1</span>
          <div className="account-meta">
            <strong>1hao</strong>
            <span className="muted">微信已绑定 · 手机号 138****8888</span>
            <span className="tag accent">免费体验</span>
          </div>
        </div>

        <div className="sheet-actions left">
          <button type="button" className="secondary-btn" onClick={() => onNotice("换绑手机号将在正式版接入。")}>
            换绑手机号
          </button>
          <button type="button" className="secondary-btn" onClick={() => onNotice("修改密码将在正式版接入。")}>
            修改密码
          </button>
        </div>

        <p className="section-label">角色权限</p>
        <div className="card">
          <div className="perm-head">
            <span>资源</span>
            <span>管理员</span>
            <span>成员</span>
            <span>访客</span>
          </div>
          {PERMISSIONS.map((item) => (
            <div key={item.resource} className="perm-row">
              <span>{item.resource}</span>
              <span>{item.owner}</span>
              <span>{item.member}</span>
              <span>{item.guest}</span>
            </div>
          ))}
        </div>

        <p className="section-label">其他</p>
        <button type="button" className="sheet-row card-like" onClick={() => onNotice("已清除本地缓存，演示数据保留。")}>
          清除缓存
        </button>
        <button type="button" className="sheet-row card-like" onClick={() => onNotice("当前为页面演示模式，未连接服务器。")}>
          连接状态
        </button>
      </div>
    </>
  );
}
