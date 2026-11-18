import { useEffect, useState } from "react";
import {
  fetchAccount,
  fetchDataInfo,
  moveDataLocation,
  type DataLocation,
  fetchAutostart,
  fetchServerConfig,
  saveServerConfig,
  setAutostart,
  signIn,
  signOut,
  testServer,
  type AccountState
} from "../deployApi";
import { bridge } from "../platform";
import { isVoiceEnabled, setVoiceEnabled, speak, voiceSupported } from "../voice";
import { PERMISSIONS } from "../data";

interface Props {
  onNotice: (message: string) => void;
}

const ROLE_LABEL: Record<string, string> = {
  Owner: "拥有者",
  Admin: "管理员",
  Member: "成员"
};

export function SystemPage({ onNotice }: Props) {
  const [speech, setSpeech] = useState(() => isVoiceEnabled());
  const [startup, setStartup] = useState(false);
  const [autostartBusy, setAutostartBusy] = useState(false);
  const [serverURL, setServerURL] = useState("");
  const [serverSaved, setServerSaved] = useState(false);
  const [serverBusy, setServerBusy] = useState(false);
  const [serverResult, setServerResult] = useState("");
  const [account, setAccount] = useState<AccountState | null>(null);
  const [dataLocation, setDataLocation] = useState<DataLocation | null>(null);
  const [dataBusy, setDataBusy] = useState(false);
  const [accountBusy, setAccountBusy] = useState(false);
  const [loginAccount, setLoginAccount] = useState("");
  const [loginPassword, setLoginPassword] = useState("");
  const [loginName, setLoginName] = useState("");

  useEffect(() => {
    let alive = true;
    fetchDataInfo()
      .then((info) => {
        if (alive) {
          setDataLocation(info.location);
        }
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, []);

  const changeDataLocation = async () => {
    const picked = await bridge.pickFolder();
    if (!picked) {
      return;
    }
    setDataBusy(true);
    try {
      const message = await moveDataLocation(picked);
      onNotice(message);
      setDataLocation((current) => (current ? { ...current, path: picked, firstRun: false } : current));
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "迁移失败");
    } finally {
      setDataBusy(false);
    }
  };

  useEffect(() => {
    let alive = true;
    fetchAccount()
      .then((state) => {
        if (alive) {
          setAccount(state);
        }
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, []);

  const doSignIn = async (register: boolean) => {
    if (!loginAccount.trim() || !loginPassword) {
      onNotice("请填写账号与密码。");
      return;
    }
    setAccountBusy(true);
    try {
      const state = await signIn(loginAccount.trim(), loginPassword, register, loginName.trim());
      setAccount(state);
      setLoginPassword("");
      onNotice(register ? "注册并登录成功。" : "登录成功。");
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "登录失败");
    } finally {
      setAccountBusy(false);
    }
  };

  const doSignOut = async () => {
    setAccountBusy(true);
    try {
      await signOut();
      setAccount({ loggedIn: false, id: 0, account: "", displayName: "", role: "", message: "已退出登录" });
      onNotice("已退出登录。");
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "退出失败");
    } finally {
      setAccountBusy(false);
    }
  };

  useEffect(() => {
    let alive = true;
    fetchServerConfig()
      .then((state) => {
        if (alive) {
          setServerURL(state.url);
          setServerSaved(state.saved);
        }
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, []);

  const saveServer = async () => {
    setServerBusy(true);
    try {
      const state = await saveServerConfig(serverURL);
      setServerSaved(state.saved);
      setServerResult(state.message);
      onNotice(state.message);
    } catch (err: unknown) {
      setServerResult(err instanceof Error ? err.message : "保存失败");
      onNotice(err instanceof Error ? err.message : "保存失败");
    } finally {
      setServerBusy(false);
    }
  };

  const checkServer = async () => {
    setServerBusy(true);
    try {
      const result = await testServer();
      setServerResult(`${result.message}${result.body ? "：" + result.body : ""}`);
      onNotice(result.message);
    } catch (err: unknown) {
      setServerResult(err instanceof Error ? err.message : "测试失败");
    } finally {
      setServerBusy(false);
    }
  };

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
          <div className="row" style={{ justifyContent: "space-between" }}>
            <h2 className="section-title">账号</h2>
            <span className={`tag${account?.loggedIn ? " accent" : ""}`}>
              {account?.loggedIn ? "已登录" : "未登录"}
            </span>
          </div>

          {account?.loggedIn ? (
            <>
              <div className="row" style={{ gap: 10, marginTop: 10, flexWrap: "wrap" }}>
                <span className="tag">名称 {account.displayName || account.account}</span>
                <span className="tag">账号 {account.account}</span>
                <span className="tag">角色 {ROLE_LABEL[account.role] ?? account.role}</span>
              </div>
              <p className="caption" style={{ marginTop: 8 }}>
                会话由本地服务保管，页面不持有登录凭据；退出后需重新登录才能使用云端功能。
              </p>
              <div className="row" style={{ marginTop: 12 }}>
                <button type="button" className="secondary-btn" onClick={() => void doSignOut()} disabled={accountBusy}>
                  {accountBusy ? "处理中…" : "退出登录"}
                </button>
              </div>
            </>
          ) : (
            <>
              <p className="caption" style={{ marginTop: 6 }}>
                登录后可同步目录、查看成员与权限。未登录不影响本地功能。
              </p>
              <div className="row" style={{ gap: 8, marginTop: 10, flexWrap: "wrap" }}>
                <input
                  className="input"
                  style={{ flex: 1, minWidth: 160 }}
                  placeholder="账号"
                  value={loginAccount}
                  onChange={(event) => setLoginAccount(event.target.value)}
                />
                <input
                  className="input"
                  style={{ flex: 1, minWidth: 160 }}
                  type="password"
                  placeholder="密码（至少 6 位）"
                  value={loginPassword}
                  onChange={(event) => setLoginPassword(event.target.value)}
                />
                <input
                  className="input"
                  style={{ flex: 1, minWidth: 140 }}
                  placeholder="显示名称（注册时用）"
                  value={loginName}
                  onChange={(event) => setLoginName(event.target.value)}
                />
              </div>
              <div className="row" style={{ gap: 8, marginTop: 10 }}>
                <button type="button" className="primary-btn" onClick={() => void doSignIn(false)} disabled={accountBusy}>
                  {accountBusy ? "处理中…" : "登录"}
                </button>
                <button type="button" className="secondary-btn" onClick={() => void doSignIn(true)} disabled={accountBusy}>
                  注册并登录
                </button>
                {account?.message && <span className="caption">{account.message}</span>}
              </div>
            </>
          )}

          <div className="row" style={{ marginTop: 14 }}>
            <button type="button" className="secondary-btn" disabled title="换绑需要服务端下发验证码，尚未接入">
              绑定微信 / 手机号
            </button>
            <button type="button" className="ghost-btn" disabled title="成员邀请需要服务端管理员权限与邮件/短信通道，尚未接入">
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
              onClick={() => {
                const next = !speech;
                setSpeech(next);
                setVoiceEnabled(next);
                if (next) {
                  speak("语音播报已开启");
                }
                onNotice(
                  next
                    ? voiceSupported()
                      ? "语音播报已开启。"
                      : "语音播报已开启，但当前环境不支持语音合成。"
                    : "语音播报已关闭。"
                );
              }}
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
          <div className="row" style={{ justifyContent: "space-between" }}>
            <h2 className="section-title">数据位置</h2>
            <span className="tag">
              {dataLocation ? `硬盘 ${dataLocation.drive}` : "读取中…"}
            </span>
          </div>
          {dataLocation ? (
            <>
              <p className="list-card-detail" style={{ marginTop: 6 }}>
                数据库保存在 <b>{dataLocation.drive}</b> 盘：{dataLocation.path}
              </p>
              <p className="caption" style={{ marginTop: 4 }}>
                该盘剩余 {dataLocation.freeGb.toFixed(0)} GB / 共 {dataLocation.totalGb.toFixed(0)} GB ·
                数据库文件 {(dataLocation.dbSizeBytes / 1024).toFixed(0)} KB
              </p>
              <p className="caption" style={{ marginTop: 4 }}>
                右上角「硬盘剩余」显示的就是这块盘，换盘后两者会一起变。
              </p>
              <div className="row" style={{ marginTop: 10, gap: 8 }}>
                <button type="button" className="secondary-btn" onClick={() => void changeDataLocation()} disabled={dataBusy}>
                  {dataBusy ? "迁移中…" : "更换到其他硬盘"}
                </button>
                <span className="caption">迁移会复制数据库，重启后生效；旧目录需手动删除</span>
              </div>
            </>
          ) : (
            <p className="caption" style={{ marginTop: 6 }}>读取数据位置失败。</p>
          )}
        </div>

        <div className="card">
          <h2 className="section-title">服务器</h2>
          <p className="list-card-detail">
            {serverSaved
              ? "已配置服务器：目录下发、账号与更新检查都依赖它。"
              : "未配置服务器：使用内置目录与本地数据，功能不受影响。"}
          </p>
          <div className="row" style={{ marginTop: 12, gap: 8, flexWrap: "wrap" }}>
            <input
              className="input"
              style={{ flex: 1, minWidth: 240 }}
              placeholder="例如 http://127.0.0.1:8080"
              value={serverURL}
              onChange={(event) => setServerURL(event.target.value)}
            />
            <button type="button" className="primary-btn" onClick={() => void saveServer()} disabled={serverBusy}>
              保存
            </button>
            <button
              type="button"
              className="secondary-btn"
              onClick={() => void checkServer()}
              disabled={serverBusy || !serverSaved}
            >
              测试连接
            </button>
            <span className={`tag${serverSaved ? " accent" : ""}`}>{serverSaved ? "已配置" : "纯本地"}</span>
          </div>
          {serverResult && (
            <p className="caption" style={{ marginTop: 8 }}>{serverResult}</p>
          )}
        </div>
      </div>
    </div>
  );
}
