import { useEffect, useState } from "react";
import { fetchAutostart, setAutostart } from "../deployApi";
import { bridge } from "../platform";
import { PERMISSIONS } from "../data";

interface Props {
  onNotice: (message: string) => void;
}

const MEMBERS = [
  { name: "开心同学", account: "kaixin", phone: "138****8866", wechat: "已绑定", role: "管理员" },
  { name: "小林", account: "xiaolin", phone: "139****2210", wechat: "已绑定", role: "成员" },
  { name: "访客账号", account: "guest", phone: "未绑定", wechat: "未绑定", role: "访客" }
];

export function SystemPage({ onNotice }: Props) {
  const [speech, setSpeech] = useState(true);
  const [startup, setStartup] = useState(false);
  const [autostartBusy, setAutostartBusy] = useState(false);

  // 真实读取注册表状态，而不是凭默认值
  useEffect(() => {
    let alive = true;
    fetchAutostart()
      .then((state) => {
        if (alive) {
          setStartup(state.enabled);
        }
      })
      .catch(() => {
        if (alive) {
          setStartup(false);
        }
      });
    return () => {
      alive = false;
    };
  }, []);

  const toggleAutostart = async () => {
    setAutostartBusy(true);
    try {
      const exePath = await bridge.getAppExePath();
      const next = await setAutostart(!startup, exePath);
      setStartup(next.enabled);
      onNotice(next.enabled ? "已设置开机自动启动。" : "已取消开机启动。");
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "设置开机启动失败");
    } finally {
      setAutostartBusy(false);
    }
  };

  return (
    <div className="page">
      <div className="page-narrow">
        <h1 className="page-title">系统设置</h1>
        <p className="page-subtitle">账号、权限、安装模式与本地运行参数</p>

        <div className="card" style={{ marginTop: 18 }}>
          <h2 className="section-title">账号与权限</h2>
          <table className="table">
            <thead>
              <tr>
                <th>成员</th>
                <th>账号</th>
                <th>手机号</th>
                <th>微信</th>
                <th>角色</th>
              </tr>
            </thead>
            <tbody>
              {MEMBERS.map((member) => (
                <tr key={member.account}>
                  <td>{member.name}</td>
                  <td>{member.account}</td>
                  <td>{member.phone}</td>
                  <td>{member.wechat}</td>
                  <td>{member.role}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="row" style={{ marginTop: 14 }}>
            <button
              type="button"
              className="secondary-btn"
              onClick={() => onNotice("微信与手机号换绑将在接入服务端后开放。")}
            >
              绑定微信 / 手机号
            </button>
            <button
              type="button"
              className="ghost-btn"
              onClick={() => onNotice("邀请成员将在功能实现阶段接入。")}
            >
              邀请成员
            </button>
          </div>
        </div>

        <div className="card">
          <h2 className="section-title">角色权限</h2>
          <table className="table">
            <thead>
              <tr>
                <th>资源</th>
                <th>管理员</th>
                <th>成员</th>
                <th>访客</th>
              </tr>
            </thead>
            <tbody>
              {PERMISSIONS.map((item) => (
                <tr key={item.resource}>
                  <td>{item.resource}</td>
                  <td>{item.owner}</td>
                  <td>{item.member}</td>
                  <td>{item.guest}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="card">
          <h2 className="section-title">运行设置</h2>
          <div className="switch-row" style={{ marginTop: 6 }}>
            <div>
              <div style={{ fontSize: 12.5, fontWeight: 600 }}>语音播报</div>
              <div className="caption" style={{ marginTop: 4 }}>
                部署进度与故障提示使用中文语音播报
              </div>
            </div>
            <button
              type="button"
              className={`segment${speech ? " active" : ""}`}
              onClick={() => setSpeech(!speech)}
            >
              {speech ? "已开启" : "已关闭"}
            </button>
          </div>
          <div className="switch-row">
            <div>
              <div style={{ fontSize: 12.5, fontWeight: 600 }}>开机启动</div>
              <div className="caption" style={{ marginTop: 4 }}>
                登录 Windows 后自动启动开心同学
              </div>
            </div>
            <button
              type="button"
              className={`segment${startup ? " active" : ""}`}
              onClick={() => void toggleAutostart()}
              disabled={autostartBusy}
            >
              {autostartBusy ? "处理中…" : startup ? "已开启" : "已关闭"}
            </button>
          </div>
        </div>

        <div className="card">
          <h2 className="section-title">服务器</h2>
          <p className="list-card-detail">当前未配置服务器地址，使用内置技能清单与本地演示数据。</p>
          <div className="row" style={{ marginTop: 12 }}>
            <button
              type="button"
              className="secondary-btn"
              onClick={() => onNotice("服务器地址配置将在接入服务端后开放。")}
            >
              配置服务器
            </button>
            <span className="tag">离线可用</span>
          </div>
        </div>
      </div>
    </div>
  );
}
