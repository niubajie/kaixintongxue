import { useCallback, useEffect, useRef, useState } from "react";
import { usePersistentState } from "../assistant";
import { SKILLS, type SkillOption } from "../data";
import {
  buildPlan,
  confirmSiteAuth,
  executeDeploy,
  cancelDeploy,
  fetchCatalog,
  fetchDeployStatus,
  fetchHardware,
  matchSoftware,
  openSiteLogin,
  type DeployJob,
  type DeployPlan,
  type DeployStepStatus,
  type HardwareInfo,
  type MatchReason,
  type PlanSiteAuth,
  type SoftwareEntry
} from "../deployApi";

interface Props {
  onNotice: (message: string) => void;
}

interface SkillDraft {
  name: string;
  content: string;
}

const ACTION_LABEL: Record<string, string> = {
  extract: "下载并解压",
  "pull-model": "命令行安装",
  manual: "手动安装"
};

const STEP_LABEL: Record<string, string> = {
  pending: "等待",
  downloading: "下载中",
  verifying: "校验中",
  extracting: "解压中",
  installing: "安装中",
  "verifying-install": "验证中",
  authorizing: "等待授权",
  done: "完成",
  failed: "失败",
  skipped: "跳过"
};

// 排队等待的步骤按 20 MB/s 估算，与方案里的时间预估保持一致。
const ESTIMATE_BYTES_PER_SECOND = 20 * 1024 * 1024;

function formatSize(bytes: number): string {
  if (bytes >= 1024 * 1024 * 1024) {
    return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`;
  }
  if (bytes >= 1024 * 1024) {
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  }
  return `${Math.max(1, Math.round(bytes / 1024))} KB`;
}

function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return "几秒";
  }
  if (seconds < 60) {
    return `${Math.ceil(seconds)} 秒`;
  }
  if (seconds < 3600) {
    return `${Math.floor(seconds / 60)} 分 ${Math.ceil(seconds % 60)} 秒`;
  }
  return `${Math.floor(seconds / 3600)} 小时 ${Math.round((seconds % 3600) / 60)} 分`;
}

// 下载中的步骤返回 0-100 的百分比，其余步骤返回 null。
function downloadPercent(step: DeployStepStatus): number | null {
  if (step.status !== "downloading" || !step.totalBytes) {
    return null;
  }
  return Math.min(100, Math.round(((step.downloadedBytes ?? 0) / step.totalBytes) * 100));
}

// 等待下载的步骤显示文件大小与预计时长。
function pendingHint(step: DeployStepStatus): string {
  if (step.status !== "pending" || !step.totalBytes) {
    return "";
  }
  const seconds = step.totalBytes / ESTIMATE_BYTES_PER_SECOND;
  return `文件大小 ${formatSize(step.totalBytes)} · 预计约 ${formatDuration(seconds)}`;
}

export function SkillsPage({ onNotice }: Props) {
  const [mode, setMode] = useState<"existing" | "upgrade">("existing");
  const [skills, setSkills] = usePersistentState<SkillOption[]>("kaixin.skills.v2", SKILLS);
  const [selected, setSelected] = useState<string[]>(["kb", "office"]);
  const [supplement, setSupplement] = useState("");

  const [hwState, setHwState] = useState<"detecting" | "ready" | "failed">("detecting");
  const [hardware, setHardware] = useState<HardwareInfo | null>(null);
  const [hwError, setHwError] = useState("");

  const [catalog, setCatalog] = useState<SoftwareEntry[]>([]);
  const [editing, setEditing] = useState<SkillOption | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [draft, setDraft] = useState<SkillDraft>({ name: "", content: "" });
  const [draftMatches, setDraftMatches] = useState<MatchReason[]>([]);
  const [matching, setMatching] = useState(false);
  // 弹窗内的匹配结果提示：底部状态栏容易被忽略，错误直接显示在按钮上方。
  const [draftNotice, setDraftNotice] = useState<{ type: "ok" | "error"; text: string } | null>(null);
  const draftNoticeRef = useRef<HTMLDivElement>(null);

  const [plan, setPlan] = useState<DeployPlan | null>(null);
  const [planning, setPlanning] = useState(false);
  const [planError, setPlanError] = useState("");
  const planResultRef = useRef<HTMLDivElement>(null);
  const [extraIds, setExtraIds] = useState<string[]>([]);
  const [extraPick, setExtraPick] = useState("");

  const [confirming, setConfirming] = useState(false);
  const [force, setForce] = useState(false);
  const [job, setJob] = useState<DeployJob | null>(null);
  const [resultOpen, setResultOpen] = useState(false);
  // 部署到需要对接的网站时，弹出登录框等待用户完成授权。
  const [authSite, setAuthSite] = useState<PlanSiteAuth | null>(null);
  const [starting, setStarting] = useState(false);

  const detect = useCallback(async () => {
    setHwState("detecting");
    setHwError("");
    try {
      const info = await fetchHardware();
      setHardware(info);
      setHwState("ready");
    } catch (error) {
      setHardware(null);
      setHwError(error instanceof Error ? error.message : "硬件检测失败");
      setHwState("failed");
    }
  }, []);

  useEffect(() => {
    void detect();
    fetchCatalog().then(setCatalog).catch(() => setCatalog([]));
    // 旧版本地缓存迁移：保留用户自建技能，内置技能使用新种子。
    const legacy = localStorage.getItem("kaixin.skills");
    if (legacy) {
      try {
        const parsed = JSON.parse(legacy) as SkillOption[];
        const customs = parsed.filter((item) => item.id.startsWith("custom-"));
        if (customs.length > 0) {
          setSkills((list) => [...list, ...customs.filter((custom) => !list.some((item) => item.id === custom.id))]);
        }
      } catch {
        // 旧数据损坏时忽略，使用新种子。
      }
      localStorage.removeItem("kaixin.skills");
    }
  }, [detect, setSkills]);

  useEffect(() => {
    if (plan || planError) {
      planResultRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, [plan, planError]);

  // 部署任务轮询：running 时每 1.5 秒刷新一次。
  useEffect(() => {
    if (!job || job.state !== "running") {
      return;
    }
    const timer = window.setInterval(() => {
      fetchDeployStatus()
        .then((status) => {
          if (status.state === "idle") {
            return;
          }
          const next = status as DeployJob;
          setJob(next);
          // 有网站等待授权时弹出登录框；任务结束后关闭残留弹窗。
          const authorizing = next.steps.find((step) => step.action === "authorize" && step.status === "authorizing");
          if (authorizing?.siteId) {
            setAuthSite({
              siteId: authorizing.siteId,
              name: authorizing.name,
              url: authorizing.loginUrl ?? "",
              loginUrl: authorizing.loginUrl ?? "",
              note: authorizing.message,
              authorized: false
            });
          }
          // 部署结束（完成或失败）时弹窗提示。
          if (next.state !== "running") {
            setAuthSite(null);
            setResultOpen(true);
          }
        })
        .catch(() => undefined);
    }, 1500);
    return () => window.clearInterval(timer);
  }, [job]);

  // 取消部署：只发信号，真正的清理由 sidecar 在各步骤边界完成。
  const handleCancel = async () => {
    try {
      await cancelDeploy();
      onNotice("已发出取消请求，正在停止部署…");
    } catch (error) {
      onNotice(error instanceof Error ? error.message : "取消失败");
    }
  };

  const toggleSkill = (skill: SkillOption) => {
    setPlan(null);
    setSelected((current) =>
      current.includes(skill.id) ? current.filter((id) => id !== skill.id) : [skill.id, ...current]
    );
  };

  const openCreate = () => {
    setEditing(null);
    setDraft({ name: "", content: "" });
    setDraftMatches([]);
    setDraftNotice(null);
    setDialogOpen(true);
  };

  const openEdit = (skill: SkillOption) => {
    setEditing(skill);
    setDraft({ name: skill.name, content: skill.content ?? "" });
    setDraftMatches([]);
    setDraftNotice(null);
    setDialogOpen(true);
  };

  const applyAISoftware = async () => {
    const content = draft.content.trim();
    if (!content) {
      setDraftNotice({ type: "error", text: "请先填写技能内容，再配置 AI 软件。" });
      return;
    }
    setMatching(true);
    setDraftNotice(null);
    try {
      const result = await matchSoftware(content);
      setDraft({ ...draft, content: result.content });
      setDraftMatches(result.lines);
      const unmatched = result.lines.filter((line) => line.text && !line.softwareId && line.software !== "已配置").length;
      setDraftNotice(
        unmatched > 0
          ? { type: "error", text: `匹配完成，${unmatched} 条未找到软件（已标【无】），可换个说法或手动添加。` }
          : { type: "ok", text: "已为每条内容配置 AI 软件。" }
      );
    } catch (error) {
      setDraftNotice({ type: "error", text: error instanceof Error ? error.message : "匹配失败，请稍后重试。" });
    } finally {
      setMatching(false);
      window.requestAnimationFrame(() => draftNoticeRef.current?.scrollIntoView({ block: "nearest" }));
    }
  };

  const saveSkill = () => {
    const name = draft.name.trim();
    if (!name) {
      onNotice("请填写技能名称。");
      return;
    }
    const content = draft.content.trim();
    const matches = draftMatches.length > 0 ? draftMatches : editing?.matches;
    if (editing) {
      setSkills((list) =>
        list.map((item) =>
          item.id === editing.id
            ? { ...item, name, content, matches, icon: name.slice(0, 1), description: content ? content.split("\n")[0] : item.description }
            : item
        )
      );
      onNotice(`已更新技能「${name}」。`);
    } else {
      const id = `custom-${Date.now()}`;
      setSkills((list) => [
        ...list,
        { id, name, content, matches, icon: name.slice(0, 1), description: content ? content.split("\n")[0] : "自建技能", demand: 1 }
      ]);
      setSelected((current) => [id, ...current]);
      onNotice(`已新增技能「${name}」，并加入已选。`);
    }
    setDialogOpen(false);
    setPlan(null);
  };

  const generate = async () => {
    if (selected.length === 0) {
      const message = "请至少选择一项技能，再生成方案。";
      setPlanError(message);
      onNotice(message);
      return;
    }
    setPlanning(true);
    setPlanError("");
    try {
      const selections = [];
      const siteIds = new Set<string>();
      for (const skill of skills.filter((item) => selected.includes(item.id))) {
        let matches = skill.matches;
        if ((!matches || matches.length === 0) && skill.content?.trim()) {
          const result = await matchSoftware(skill.content);
          matches = result.lines;
          setSkills((list) => list.map((item) => (item.id === skill.id ? { ...item, matches: result.lines, content: result.content } : item)));
        }
        (matches ?? []).forEach((line) => {
          if (line.siteId) {
            siteIds.add(line.siteId);
          }
        });
        const ids = [...new Set((matches ?? []).filter((line) => line.softwareId).map((line) => line.softwareId))];
        selections.push({ id: skill.id, name: skill.name, softwareIds: ids });
      }
      const result = await buildPlan(selections, extraIds, [...siteIds]);
      setPlan(result);
      setJob(null);
      onNotice("方案已生成：已合并重复软件并完成冲突检测。");
    } catch (error) {
      const message = error instanceof Error ? error.message : "生成方案失败";
      setPlan(null);
      setPlanError(message);
      onNotice(message);
    } finally {
      setPlanning(false);
    }
  };

  const startDeploy = async () => {
    if (!plan) {
      return;
    }
    setStarting(true);
    try {
      await executeDeploy(plan.steps, force, plan.authSteps);
      const status = await fetchDeployStatus();
      if (status.state !== "idle") {
        const next = status as DeployJob;
        setJob(next);
        if (next.state !== "running") {
          setResultOpen(true);
        }
      }
      setConfirming(false);
      onNotice("部署已开始，正在下载与安装…");
    } catch (error) {
      onNotice(error instanceof Error ? error.message : "部署启动失败");
    } finally {
      setStarting(false);
    }
  };

  const confirmAuth = async () => {
    if (!authSite) {
      return;
    }
    try {
      await confirmSiteAuth(authSite.siteId);
      onNotice(`已完成 ${authSite.name} 授权，继续部署…`);
      setAuthSite(null);
    } catch (error) {
      onNotice(error instanceof Error ? error.message : "授权确认失败");
    }
  };

  const orderedSkills = [...skills].sort(
    (left, right) => Number(selected.includes(right.id)) - Number(selected.includes(left.id))
  );

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="page-title">技能部署</h1>
          <p className="page-subtitle">选择需要的 AI 技能，生成方案并部署到本机（本地服务 127.0.0.1:5085）</p>
        </div>
        <span className="tag">前三步只读，部署才写入本机</span>
      </div>

      <div className="mode-cards">
        <button type="button" className={`mode-card${mode === "existing" ? " active" : ""}`} onClick={() => setMode("existing")}>
          <div className="list-card-title">现有电脑 + AI 技能</div>
          <p className="list-card-detail">基于当前硬件配置，选择可运行的 AI 技能</p>
        </button>
        <button type="button" className={`mode-card${mode === "upgrade" ? " active" : ""}`} onClick={() => setMode("upgrade")}>
          <div className="list-card-title">AI 技能 + 硬件配置</div>
          <p className="list-card-detail">按所选技能反推推荐硬件，方案中给出警告</p>
        </button>
      </div>

      <div className="card" style={{ marginTop: 14 }}>
        <div className="row" style={{ justifyContent: "space-between" }}>
          <h2 className="section-title">本机硬件配置</h2>
          <button type="button" className="ghost-btn" onClick={() => void detect()} disabled={hwState === "detecting"}>
            {hwState === "detecting" ? "检测中…" : "重新检测"}
          </button>
        </div>
        {hwState === "detecting" && <div className="empty">正在读取本机硬件（WMI），通常需要几秒…</div>}
        {hwState === "failed" && (
          <div className="empty">
            <p>{hwError}</p>
            <button type="button" className="secondary-btn" onClick={() => void detect()}>
              重试
            </button>
          </div>
        )}
        {hwState === "ready" && hardware && (
          <div className="hw-grid">
            <div className="card hw-card">
              <div className="caption">处理器</div>
              <div className="hw-value">{hardware.cpuName.trim()}</div>
              <div className="caption">{hardware.cpuCores} 核 / {hardware.cpuLogical} 线程</div>
            </div>
            <div className="card hw-card">
              <div className="caption">内存</div>
              <div className="hw-value">{hardware.memoryTotalGb.toFixed(1)} GB</div>
              <div className="caption">可用 {hardware.memoryFreeGb.toFixed(1)} GB</div>
            </div>
            <div className="card hw-card">
              <div className="caption">显卡</div>
              <div className="hw-value">{hardware.gpuName || "未读取到"}</div>
              <div className="caption">{hardware.gpuVramGb ? `显存 ${hardware.gpuVramGb.toFixed(1)} GB` : "显存容量未读取"}</div>
            </div>
            <div className="card hw-card">
              <div className="caption">系统 / 磁盘</div>
              <div className="hw-value">{hardware.osCaption}</div>
              <div className="caption">
                {hardware.disks.map((disk) => `${disk.name} 可用 ${disk.freeGb.toFixed(0)}/${disk.totalGb.toFixed(0)} GB`).join("；")}
              </div>
            </div>
          </div>
        )}
      </div>

      <div className="row" style={{ justifyContent: "space-between", marginTop: 22 }}>
        <h2 className="section-title">选择技能</h2>
        <div className="row" style={{ gap: 10 }}>
          <span className="caption">已选 {selected.length} 项</span>
          <button type="button" className="secondary-btn" style={{ padding: "6px 12px" }} onClick={openCreate}>
            + 新增技能
          </button>
        </div>
      </div>
      <div className="skill-grid">
        {orderedSkills.map((skill) => {
          const active = selected.includes(skill.id);
          const siteUrls = [...new Set((skill.matches ?? []).filter((line) => line.siteUrl).map((line) => line.siteUrl as string))];
          return (
            <div
              key={skill.id}
              className={`skill-card${active ? " selected" : ""}`}
              role="button"
              tabIndex={0}
              onClick={() => toggleSkill(skill)}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault();
                  toggleSkill(skill);
                }
              }}
            >
              <span className="skill-icon">{skill.icon}</span>
              <span style={{ flex: 1 }}>
                <span className="skill-name">{skill.name}</span>
                <span className="skill-desc" style={{ display: "block" }}>{skill.description}</span>
                {siteUrls.length > 0 && (
                  <span className="caption" style={{ display: "block" }}>网址：{siteUrls.join("、")}</span>
                )}
              </span>
              <span className="skill-side">
                <span className="tag">{active ? "已选" : `需 ${skill.demand} 核`}</span>
                <button
                  type="button"
                  className="skill-edit"
                  onClick={(event) => {
                    event.stopPropagation();
                    openEdit(skill);
                  }}
                >
                  修改
                </button>
              </span>
            </div>
          );
        })}
      </div>

      <label className="field-label">补充需求（可选）</label>
      <textarea
        className="textarea"
        rows={2}
        placeholder="例如：希望全程离线运行，优先保证响应速度"
        value={supplement}
        onChange={(event) => setSupplement(event.target.value)}
      />

      <div className="row" style={{ marginTop: 16 }}>
        <button type="button" className="primary-btn" onClick={() => void generate()} disabled={planning}>
          {planning ? "正在生成…" : "生成技能方案"}
        </button>
        <span className="caption">自动合并重复软件、按依赖排序，并检测磁盘/显存/内存冲突</span>
      </div>

      {planError && (
        <div className="plan-error" role="alert" ref={planResultRef}>
          <strong>方案生成失败</strong>
          <span>{planError}</span>
        </div>
      )}

      {plan && (
        <div className="card" style={{ marginTop: 18 }} ref={planResultRef}>
          <div className="row" style={{ justifyContent: "space-between" }}>
            <h2 className="section-title">部署方案 · 涉及软件与网站</h2>
            <span className="tag accent">共 {plan.steps.length} 步 · 预计下载 {(plan.totalSizeMb / 1024).toFixed(1)} GB · 约 {plan.estimatedMinutes} 分钟</span>
          </div>
          {supplement.trim() && <p className="list-card-detail">补充需求：{supplement.trim()}</p>}
          {plan.warnings.map((warning) => (
            <div className="plan-warning" key={warning}>⚠ {warning}</div>
          ))}
          <div className="plan-steps">
            {plan.steps.map((step) => (
              <div className="plan-step" key={step.softwareId}>
                <span className="plan-order">[{step.order}]</span>
                <span className="plan-name">{step.name}</span>
                <span className="caption">{ACTION_LABEL[step.action] ?? step.action}</span>
                <span className="caption">{step.sizeMb > 0 ? `${step.sizeMb} MB` : "—"}</span>
                {step.alreadyInstalled && <span className="tag accent">已安装</span>}
                <span className="caption plan-url">软件网站：{step.url || "未提供"}</span>
              </div>
            ))}
          </div>
          {plan.authSteps.length > 0 && (
            <div style={{ marginTop: 12 }}>
              <div className="caption">网站授权（部署时弹窗登录，已授权的自动跳过）</div>
              <div className="plan-steps" style={{ marginTop: 8 }}>
                {plan.authSteps.map((site) => (
                  <div className="plan-step" key={site.siteId}>
                    <span className="plan-name">{site.name}</span>
                    <span className="caption plan-url">{site.url}</span>
                    <span className={`tag${site.authorized ? " accent" : ""}`}>
                      {site.authorized ? "已授权" : "待授权"}
                    </span>
                    <span className="caption" style={{ flex: 1 }}>{site.note}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
          <div className="row" style={{ marginTop: 12, flexWrap: "wrap" }}>
            <select className="segment" value={extraPick} onChange={(event) => setExtraPick(event.target.value)}>
              <option value="">手动添加软件…</option>
              {catalog.map((entry) => (
                <option key={entry.id} value={entry.id}>{entry.name}（{entry.sizeMb} MB）</option>
              ))}
            </select>
            <button
              type="button"
              className="secondary-btn"
              onClick={() => {
                if (!extraPick) {
                  return;
                }
                setExtraIds((ids) => [...new Set([...ids, extraPick])]);
                setExtraPick("");
                setPlan(null);
                onNotice("已加入清单，请重新生成方案。");
              }}
            >
              添加
            </button>
            {extraIds.length > 0 && <span className="caption">手动添加：{extraIds.join("、")}</span>}
          </div>
          <div className="row" style={{ marginTop: 14 }}>
            <button type="button" className="primary-btn" onClick={() => setConfirming(true)}>
              部署方案
            </button>
            <button type="button" className="secondary-btn" onClick={() => void generate()} disabled={planning}>
              重新生成
            </button>
          </div>
        </div>
      )}

      {job && job.steps && (
        <div className="card" style={{ marginTop: 18 }}>
          <div className="row" style={{ justifyContent: "space-between" }}>
            <h2 className="section-title">部署进度</h2>
            <div className="row" style={{ gap: 8 }}>
              {job.state === "running" && (
                <button type="button" className="secondary-btn" onClick={() => void handleCancel()}>
                  取消部署
                </button>
              )}
              <span className={`tag${job.state === "failed" || job.state === "cancelled" ? "" : " accent"}`}>
                {job.state === "running"
                  ? "进行中"
                  : job.state === "done"
                    ? "已完成"
                    : job.state === "cancelled"
                      ? "已取消"
                      : "已失败"}
              </span>
            </div>
          </div>
          <div className="plan-steps">
            {job.steps.map((step) => {
              const percent = downloadPercent(step);
              return (
                <div className={`plan-step deploy-${step.status}`} key={step.softwareId}>
                  <span className="plan-order">[{step.order}]</span>
                  <span className="plan-name">{step.name}</span>
                  <span className="tag">{STEP_LABEL[step.status] ?? step.status}</span>
                  <span className="caption" style={{ flex: 1 }}>{step.message || pendingHint(step)}</span>
                  {step.status === "downloading" && step.bytesPerSecond ? (
                    <span className="caption">{formatSize(step.bytesPerSecond)}/s</span>
                  ) : null}
                  {percent !== null && (
                    <div className="deploy-progress">
                      <div className="progress slim" style={{ flex: 1 }}>
                        <span style={{ width: `${percent}%` }} />
                      </div>
                      <span className="caption">{percent}%</span>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
          <p className="caption" style={{ marginTop: 10 }}>日志：{job.logPath}</p>
        </div>
      )}

      {resultOpen && job && (
        <div className="dialog-mask" onClick={() => setResultOpen(false)}>
          <div className="dialog" onClick={(event) => event.stopPropagation()}>
            <h2 className="dialog-title">{job.state === "failed" ? "部署未完成" : "部署完成"}</h2>
            <p className="caption" style={{ marginTop: 6 }}>
              {job.state === "failed"
                ? "部分软件安装失败，请查看下方步骤与日志后重新部署。"
                : "全部软件已安装完成，现在可以使用对应技能了。"}
            </p>
            <div className="plan-steps" style={{ marginTop: 12 }}>
              {job.steps.map((step) => (
                <div className={`plan-step deploy-${step.status}`} key={step.softwareId}>
                  <span className="plan-order">[{step.order}]</span>
                  <span className="plan-name">{step.name}</span>
                  <span className="tag">{STEP_LABEL[step.status] ?? step.status}</span>
                  <span className="caption" style={{ flex: 1 }}>{step.message}</span>
                </div>
              ))}
            </div>
            <div className="dialog-actions">
              <button type="button" className="primary-btn" onClick={() => setResultOpen(false)}>
                知道了
              </button>
            </div>
          </div>
        </div>
      )}

      {authSite && (
        <div className="dialog-mask">
          <div className="dialog" onClick={(event) => event.stopPropagation()}>
            <h2 className="dialog-title">登录 {authSite.name}</h2>
            <p className="caption" style={{ marginTop: 6 }}>
              部署需要对接{authSite.name}，请先在浏览器完成登录授权，再回到这里点击「我已完成授权」。
            </p>
            <div className="plan-steps" style={{ marginTop: 10 }}>
              <div className="plan-step">
                <span className="plan-name">{authSite.name}</span>
                <span className="caption plan-url">{authSite.loginUrl}</span>
              </div>
            </div>
            <div className="dialog-actions">
              <button
                type="button"
                className="secondary-btn"
                onClick={() => {
                  openSiteLogin(authSite.siteId).catch(() => {
                    window.open(authSite.loginUrl, "_blank", "noopener");
                  });
                }}
              >
                打开登录页
              </button>
              <button type="button" className="primary-btn" onClick={() => void confirmAuth()}>
                我已完成授权
              </button>
            </div>
          </div>
        </div>
      )}

      {dialogOpen && (
        <div className="dialog-mask" onClick={() => setDialogOpen(false)}>
          <div className="dialog" onClick={(event) => event.stopPropagation()}>
            <h2 className="dialog-title">{editing ? "修改技能" : "新增技能"}</h2>
            <label className="field-label">技能名称</label>
            <input
              className="input"
              value={draft.name}
              placeholder="例如：发布短视频技能"
              onChange={(event) => setDraft({ ...draft, name: event.target.value })}
            />
            <label className="field-label">技能内容</label>
            <textarea
              className="textarea"
              rows={8}
              value={draft.content}
              placeholder={"一行一条，例如：\n1、剪辑商品视频\n2、把录音转成文字"}
              onChange={(event) => setDraft({ ...draft, content: event.target.value })}
            />
            {draftNotice && (
              <div ref={draftNoticeRef} className={`dialog-hint ${draftNotice.type}`}>
                {draftNotice.text}
              </div>
            )}
            {draftMatches.length > 0 && (
              <div className="match-list">
                {draftMatches.filter((line) => line.text && line.software !== "已配置").map((line) => (
                  <div className="match-item" key={line.text}>
                    <div className="match-head">
                      <span className="match-text">{line.text}</span>
                      <span className={`tag${line.softwareId ? " accent" : ""}`}>{line.softwareId ? line.software : "无匹配"}</span>
                    </div>
                    {line.softwareId ? (
                      <div className="caption">{line.reasons.join("；")}</div>
                    ) : (
                      <div className="caption">建议：换个说法重新匹配，或在生成方案后手动添加软件。</div>
                    )}
                    {line.siteUrl && <div className="caption">网址：{line.siteUrl}</div>}
                  </div>
                ))}
              </div>
            )}
            <div className="dialog-actions">
              <button type="button" className="secondary-btn" style={{ marginRight: "auto" }} onClick={() => void applyAISoftware()} disabled={matching}>
                {matching ? "匹配中…" : "配置ai软件"}
              </button>
              <button type="button" className="secondary-btn" onClick={() => setDialogOpen(false)}>
                取消
              </button>
              <button type="button" className="primary-btn" onClick={saveSkill}>
                保存
              </button>
            </div>
          </div>
        </div>
      )}

      {confirming && plan && (
        <div className="dialog-mask" onClick={() => setConfirming(false)}>
          <div className="dialog" onClick={(event) => event.stopPropagation()}>
            <h2 className="dialog-title">确认部署到本机</h2>
            <p className="caption" style={{ marginTop: 6 }}>
              将下载便携包并解压到 %LOCALAPPDATA%\Kaixin\runtime，不需要管理员权限，不修改系统环境变量。
            </p>
            {plan.warnings.map((warning) => (
              <div className="plan-warning" key={warning}>⚠ {warning}</div>
            ))}
            <div className="plan-steps" style={{ marginTop: 10 }}>
              {plan.steps.map((step) => (
                <div className="plan-step" key={step.softwareId}>
                  <span className="plan-order">[{step.order}]</span>
                  <span className="plan-name">{step.name}</span>
                  <span className="caption">{step.sizeMb > 0 ? `${step.sizeMb} MB` : "—"}</span>
                  {step.url && <span className="caption plan-url">{step.url}</span>}
                </div>
              ))}
            </div>
            {plan.authSteps.length > 0 && (
              <div className="plan-steps" style={{ marginTop: 8 }}>
                {plan.authSteps.map((site) => (
                  <div className="plan-step" key={site.siteId}>
                    <span className="plan-name">{site.name}授权</span>
                    <span className="caption plan-url">{site.url}</span>
                    <span className={`tag${site.authorized ? " accent" : ""}`}>
                      {site.authorized ? "已授权" : "待登录"}
                    </span>
                  </div>
                ))}
              </div>
            )}
            <label className="row" style={{ marginTop: 10, gap: 6 }}>
              <input type="checkbox" checked={force} onChange={(event) => setForce(event.target.checked)} />
              <span className="caption">强制重新安装已安装的软件</span>
            </label>
            <div className="dialog-actions">
              <button type="button" className="secondary-btn" onClick={() => setConfirming(false)}>
                取消
              </button>
              <button type="button" className="primary-btn" onClick={() => void startDeploy()} disabled={starting}>
                {starting ? "正在启动…" : "确认部署"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
