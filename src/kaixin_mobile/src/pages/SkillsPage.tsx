import { useEffect, useRef, useState } from "react";
import { MobileHeader, BottomSheet } from "../components/MobileHeader";
import { SKILLS, type SkillOption } from "../data";
import { usePersistentState } from "../assistant";
import { IconCheck } from "../components/Icons";
import { buildPlan, matchSoftware, type DeployPlan, type MatchReason } from "../deployApi";

interface Props {
  onBack: () => void;
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

export function SkillsPage({ onBack, onNotice }: Props) {
  const [picked, setPicked] = usePersistentState<string[]>("kaixin.mobile.skills", ["kb", "office"]);
  const [skills, setSkills] = usePersistentState<SkillOption[]>("kaixin.skills.v2", SKILLS);
  const [editing, setEditing] = useState<SkillOption | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [draft, setDraft] = useState<SkillDraft>({ name: "", content: "" });
  const [draftMatches, setDraftMatches] = useState<MatchReason[]>([]);
  const [matching, setMatching] = useState(false);
  const [plan, setPlan] = useState<DeployPlan | null>(null);
  const [planning, setPlanning] = useState(false);
  const [planError, setPlanError] = useState("");
  const planResultRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // 旧版本地缓存迁移：保留自建技能，内置技能使用新种子。
    const legacy = localStorage.getItem("kaixin.mobile.skillList");
    if (legacy) {
      try {
        const parsed = JSON.parse(legacy) as SkillOption[];
        const customs = parsed.filter((item) => item.id.startsWith("custom-"));
        if (customs.length > 0) {
          setSkills((list) => [...list, ...customs.filter((custom) => !list.some((item) => item.id === custom.id))]);
        }
      } catch {
        // 旧数据损坏时忽略。
      }
      localStorage.removeItem("kaixin.mobile.skillList");
    }
  }, [setSkills]);

  useEffect(() => {
    if (plan || planError) {
      planResultRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, [plan, planError]);

  const toggle = (id: string) => {
    setPlan(null);
    setPicked((list) => (list.includes(id) ? list.filter((one) => one !== id) : [id, ...list]));
  };

  const openCreate = () => {
    setEditing(null);
    setDraft({ name: "", content: "" });
    setDraftMatches([]);
    setSheetOpen(true);
  };

  const openEdit = (skill: SkillOption) => {
    setEditing(skill);
    setDraft({ name: skill.name, content: skill.content ?? "" });
    setDraftMatches([]);
    setSheetOpen(true);
  };

  const applyAISoftware = async () => {
    if (!draft.content.trim()) {
      onNotice("请先填写技能内容，再配置 AI 软件。");
      return;
    }
    setMatching(true);
    try {
      const result = await matchSoftware(draft.content);
      setDraft({ ...draft, content: result.content });
      setDraftMatches(result.lines);
      const unmatched = result.lines.filter((line) => line.text && !line.softwareId && line.software !== "已配置").length;
      onNotice(unmatched > 0 ? `匹配完成，${unmatched} 条未找到软件（已标【无】）。` : "已为每条内容配置 AI 软件。");
    } catch (error) {
      onNotice(error instanceof Error ? error.message : "匹配失败");
    } finally {
      setMatching(false);
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
      setPicked((list) => [id, ...list]);
      onNotice(`已新增技能「${name}」，并加入已选。`);
    }
    setSheetOpen(false);
    setPlan(null);
  };

  const generate = async () => {
    if (picked.length === 0) {
      const message = "请至少选择一项技能，再生成方案。";
      setPlanError(message);
      onNotice(message);
      return;
    }
    setPlanning(true);
    setPlanError("");
    try {
      const selections = [];
      for (const skill of skills.filter((item) => picked.includes(item.id))) {
        let matches = skill.matches;
        if ((!matches || matches.length === 0) && skill.content?.trim()) {
          const result = await matchSoftware(skill.content);
          matches = result.lines;
          setSkills((list) => list.map((item) => (item.id === skill.id ? { ...item, matches: result.lines, content: result.content } : item)));
        }
        const ids = [...new Set((matches ?? []).filter((line) => line.softwareId).map((line) => line.softwareId))];
        selections.push({ id: skill.id, name: skill.name, softwareIds: ids });
      }
      const result = await buildPlan(selections, []);
      setPlan(result);
      onNotice("方案已生成；部署请在 Windows 桌面端执行。");
    } catch (error) {
      const message = error instanceof Error ? error.message : "生成方案失败";
      setPlan(null);
      setPlanError(message);
      onNotice(message);
    } finally {
      setPlanning(false);
    }
  };

  const orderedSkills = [...skills].sort(
    (left, right) => Number(picked.includes(right.id)) - Number(picked.includes(left.id))
  );

  return (
    <>
      <MobileHeader title="技能部署" onBack={onBack} />

      <div className="page-scroll">
        <div className="card">
          <div className="stage-title">本机硬件与部署</div>
          <p className="muted" style={{ lineHeight: 1.7 }}>
            硬件检测与便携部署在 Windows 桌面端执行（本地服务 127.0.0.1:5085）。本端可编辑技能、匹配 AI 软件并查看方案。
          </p>
        </div>

        <div className="skill-tools">
          <p className="section-label">AI 技能 · 已选 {picked.length}</p>
          <button type="button" className="mini-btn" onClick={openCreate}>
            + 新增技能
          </button>
        </div>

        {orderedSkills.map((skill) => {
          const active = picked.includes(skill.id);
          return (
            <div
              key={skill.id}
              className={`card tap skill-row${active ? " active" : ""}`}
              role="button"
              tabIndex={0}
              onClick={() => toggle(skill.id)}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault();
                  toggle(skill.id);
                }
              }}
            >
              <span className="skill-icon">{skill.icon}</span>
              <span className="skill-main">
                <strong>{skill.name}</strong>
                <span className="muted">{skill.description}</span>
              </span>
              <span className="skill-side">
                <span className={`check${active ? " on" : ""}`}>{active && <IconCheck />}</span>
                <button
                  type="button"
                  className="skill-edit-btn"
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

        {planError && (
          <div className="plan-error" role="alert" ref={planResultRef}>
            <strong>方案生成失败：</strong>{planError}
          </div>
        )}

        {plan && (
          <div className="card" ref={planResultRef}>
            <div className="stage-title">部署方案 · 涉及软件与网站</div>
            <p className="muted">共 {plan.steps.length} 步 · 预计下载 {(plan.totalSizeMb / 1024).toFixed(1)} GB · 约 {plan.estimatedMinutes} 分钟</p>
            {plan.warnings.map((warning) => (
              <div className="plan-warning" key={warning}>⚠ {warning}</div>
            ))}
            {plan.steps.map((step) => (
              <div className="plan-step" key={step.softwareId}>
                <span className="plan-order">[{step.order}]</span>
                <span className="plan-name">{step.name}</span>
                <span className="muted">{ACTION_LABEL[step.action] ?? step.action}</span>
                <span className="muted">{step.sizeMb > 0 ? `${step.sizeMb} MB` : "—"}</span>
                <span className="muted plan-url">软件网站：{step.url || "未提供"}</span>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="sticky-bottom">
        <button type="button" className="primary-btn block" onClick={() => void generate()} disabled={planning}>
          {planning ? "正在生成…" : `生成技能方案（${picked.length}）`}
        </button>
      </div>

      <BottomSheet
        open={sheetOpen}
        title={editing ? "修改技能" : "新增技能"}
        onClose={() => setSheetOpen(false)}
        footer={
          <div className="sheet-actions">
            <button type="button" className="secondary-btn" style={{ marginRight: "auto" }} onClick={() => void applyAISoftware()} disabled={matching}>
              {matching ? "匹配中…" : "配置ai软件"}
            </button>
            <button type="button" className="secondary-btn" onClick={() => setSheetOpen(false)}>
              取消
            </button>
            <button type="button" className="primary-btn" onClick={saveSkill}>
              保存
            </button>
          </div>
        }
      >
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
          rows={7}
          value={draft.content}
          placeholder={"一行一条，例如：\n1、剪辑商品视频\n2、把录音转成文字"}
          onChange={(event) => setDraft({ ...draft, content: event.target.value })}
        />
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
                  <div className="caption">建议换个说法，或在桌面端手动添加软件。</div>
                )}
              </div>
            ))}
          </div>
        )}
      </BottomSheet>
    </>
  );
}
