import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ConfirmDialog } from "./ConfirmDialog";
import { matchSoftware } from "../deployApi";
import {
  DEPENDENCY_STATUSES,
  DEPENDENCY_TYPES,
  SCENARIO_TREE,
  scenarioKey,
  scenarioSummary,
  skillKey,
  type BusinessScenario,
  type Dependency,
  type DependencyStatus,
  type DependencyType,
  type ScenarioCategory,
  type SubSkill
} from "../data/scenarioSkills";

// 三层下钻：L1 业务场景 → L2 细分技能 → L3 依赖（软件 / 模型 / 网站）
//
// 布局约定（按需求方要求）：
//   · 每一层都是一个「大框」，标题在框内左上角，框外不放其它文字
//   · L1 大框里放三类场景 + 补充需求；生成按钮在大框外居中
//   · L2 同样的框，但只展示细分技能，不提供勾选/选择功能
//   · L3 同样的框，展示依赖并可编辑

interface CustomSkill {
  id: string;
  name: string;
  description: string;
  demand: number;
  /** 归属 L1 大类：工作类 / 学习类 / 生活类 */
  category?: string;
  /** 技能要求原文，按行拆成 L2 细分技能 */
  content?: string;
  /** 匹配出来的依赖（软件 / 网站），作为 L3 */
  matches?: { software?: string; site?: string; reasons?: string[] }[];
}

const CATEGORY_ALIASES: Record<string, string> = {
  工作: "工作类",
  学习: "学习类",
  生活: "生活类",
  工作类: "工作类",
  学习类: "学习类",
  生活类: "生活类"
};

/** 把自建技能展开成「场景 → 细分技能 → 依赖」 */
function buildCustomScenario(skill: CustomSkill) {
  const lines = (skill.content ?? "")
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
  const deps: Dependency[] = [];
  for (const match of skill.matches ?? []) {
    if (match.software && !deps.some((item) => item.name === match.software)) {
      deps.push({
        type: "本地软件",
        name: match.software,
        purpose: match.reasons?.[0] ?? "",
        status: "已就绪"
      });
    }
    if (match.site && !deps.some((item) => item.name === match.site)) {
      deps.push({
        type: "网站平台",
        name: match.site,
        purpose: "网站授权",
        status: "需授权"
      });
    }
  }
  return {
    id: skill.id,
    category: CATEGORY_ALIASES[skill.category ?? ""] ?? "工作类",
    name: skill.name,
    lines: lines.length > 0 ? lines : [skill.description || skill.name],
    deps
  };
}

interface Props {
  onNotice: (message: string) => void;
  onSelectionChange?: (summary: { scenarios: string[]; skills: number; deps: number }) => void;
  /** L1 大框右上角的操作（新增技能） */
  action?: React.ReactNode;
  /** 用户自建的技能（id 以 custom- 开头），显示在场景下方 */
  customSkills?: CustomSkill[];
  /** 已选中的自定义技能 id */
  selectedCustom?: string[];
  onToggleCustom?: (id: string) => void;
  onEditCustom?: (id: string) => void;
  /** 右键删除自建技能 */
  onDeleteCustom?: (id: string) => void;
  /** 保存自建技能的技能要求（L2 编辑弹窗） */
  onUpdateCustomContent?: (id: string, content: string) => void;
}

interface ScopeState {
  selected: string[];
  customDeps: Record<string, Dependency[]>;
  removedDeps: string[];
  editedDeps: Record<string, Dependency>;
}

const STORAGE_KEY = "kaixin.scenarioScope.v2";

const EMPTY_SCOPE: ScopeState = {
  selected: [],
  customDeps: {},
  removedDeps: [],
  editedDeps: {}
};

function loadScope(): ScopeState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return EMPTY_SCOPE;
    }
    const parsed = JSON.parse(raw) as Partial<ScopeState>;
    return {
      selected: parsed.selected ?? [],
      customDeps: parsed.customDeps ?? {},
      removedDeps: parsed.removedDeps ?? [],
      editedDeps: parsed.editedDeps ?? {}
    };
  } catch {
    return EMPTY_SCOPE;
  }
}

const STATUS_CLASS: Record<DependencyStatus, string> = {
  已就绪: "dep-ok",
  待接入: "dep-warn",
  需授权: "dep-auth"
};

export function ScenarioSkillPicker({
  onNotice,
  onSelectionChange,
  action,
  customSkills = [],
  selectedCustom = [],
  onToggleCustom,
  onEditCustom,
  onDeleteCustom,
  onUpdateCustomContent
}: Props) {
  const [scope, setScope] = useState<ScopeState>(() => loadScope());
  const [level, setLevel] = useState<1 | 2 | 3>(1);
  const [activeCategory, setActiveCategory] = useState("");
  const [activeScenario, setActiveScenario] = useState("");
  const [activeSkill, setActiveSkill] = useState("");
  const [activeCustomId, setActiveCustomId] = useState<string | null>(null);

  // 右键删除：自建技能真删；业务场景与细分技能是内置数据，只做「本地隐藏」
  const [hiddenScenarios, setHiddenScenarios] = useState<string[]>(() => {
    try {
      return JSON.parse(localStorage.getItem("kaixin.hiddenScenarios.v1") ?? "[]") as string[];
    } catch {
      return [];
    }
  });
  const [hiddenSkills, setHiddenSkills] = useState<string[]>(() => {
    try {
      return JSON.parse(localStorage.getItem("kaixin.hiddenSkills.v1") ?? "[]") as string[];
    } catch {
      return [];
    }
  });

  useEffect(() => {
    localStorage.setItem("kaixin.hiddenScenarios.v1", JSON.stringify(hiddenScenarios));
  }, [hiddenScenarios]);

  useEffect(() => {
    localStorage.setItem("kaixin.hiddenSkills.v1", JSON.stringify(hiddenSkills));
  }, [hiddenSkills]);

  // 右键 → 应用内确认框（Tauri 的 window.confirm 部分环境不弹，统一用组件）
  // 右键菜单：只放「删除」一项，点它才弹确认框
  const [cardMenu, setCardMenu] = useState<
    | { x: number; y: number; kind: "scenario"; category: string; scenario: string; label: string }
    | { x: number; y: number; kind: "skill"; category: string; scenario: string; skill: string; label: string }
    | { x: number; y: number; kind: "custom"; id: string; label: string }
    | null
  >(null);

  // 单击 = 选中，双击 = 进入下一层；用定时器区分，避免双击触发两次选中
  const clickTimer = useRef<number | null>(null);
  const runSingleClick = (action: () => void) => {
    if (clickTimer.current) {
      window.clearTimeout(clickTimer.current);
    }
    clickTimer.current = window.setTimeout(() => {
      action();
      clickTimer.current = null;
    }, 220);
  };
  const runDoubleClick = (action: () => void) => {
    if (clickTimer.current) {
      window.clearTimeout(clickTimer.current);
      clickTimer.current = null;
    }
    action();
  };

  // L2「编辑」弹窗：列出全部技能细分与依赖，底部可跑 AI 技能匹配
  const [editOpen, setEditOpen] = useState(false);
  const [draftLines, setDraftLines] = useState<string[]>([]);
  const [initialLines, setInitialLines] = useState<string[]>([]);
  const [matches, setMatches] = useState<{ text: string; software?: string; site?: string }[]>([]);
  const [hasMatched, setHasMatched] = useState(false);
  const [matching, setMatching] = useState(false);
  // 内置场景的编辑结果存本地（Excel 是只读数据源）
  const [skillOverrides, setSkillOverrides] = useState<
    Record<
      string,
      {
        texts: Record<string, string>;
        extras: string[];
        removed: string[];
        matches?: { text: string; software?: string; site?: string }[];
      }
    >
  >(() => {
    try {
      return JSON.parse(localStorage.getItem("kaixin.skillOverrides.v1") ?? "{}");
    } catch {
      return {};
    }
  });

  useEffect(() => {
    localStorage.setItem("kaixin.skillOverrides.v1", JSON.stringify(skillOverrides));
  }, [skillOverrides]);

  // 右键菜单：点空白处、滚动、按 Esc 都要收起
  useEffect(() => {
    if (!cardMenu) {
      return;
    }
    const closeMenu = () => setCardMenu(null);
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        closeMenu();
      }
    };
    window.addEventListener("click", closeMenu);
    window.addEventListener("contextmenu", closeMenu);
    window.addEventListener("wheel", closeMenu, { passive: true });
    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("click", closeMenu);
      window.removeEventListener("contextmenu", closeMenu);
      window.removeEventListener("wheel", closeMenu);
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [cardMenu]);

  const [pendingDelete, setPendingDelete] = useState<
    | { kind: "scenario"; category: string; scenario: string; label: string }
    | { kind: "skill"; category: string; scenario: string; skill: string; label: string }
    | { kind: "custom"; id: string; label: string }
    | null
  >(null);

  const performDelete = () => {
    const target = pendingDelete;
    if (!target) {
      return;
    }
    if (target.kind === "scenario") {
      const key = scenarioKey(target.category, target.scenario);
      setHiddenScenarios((current) => [...current, key]);
      setScope((current) => ({
        ...current,
        selected: current.selected.filter((item) => item !== key)
      }));
      onNotice(`已删除「${target.label}」，可在列表下方恢复。`);
    } else if (target.kind === "skill") {
      setHiddenSkills((current) => [...current, skillKey(target.category, target.scenario, target.skill)]);
      onNotice(`已删除「${target.label}」，可在列表下方恢复。`);
    } else {
      onDeleteCustom?.(target.id);
    }
    setPendingDelete(null);
  };

  const menuNode = cardMenu ? (
    <div
      className="card-menu"
      style={{ left: cardMenu.x, top: cardMenu.y }}
      onMouseLeave={() => setCardMenu(null)}
    >
      <button
        type="button"
        onClick={() => {
          if (cardMenu.kind === "scenario") {
            setPendingDelete({
              kind: "scenario",
              category: cardMenu.category,
              scenario: cardMenu.scenario,
              label: cardMenu.label
            });
          } else if (cardMenu.kind === "skill") {
            setPendingDelete({
              kind: "skill",
              category: cardMenu.category,
              scenario: cardMenu.scenario,
              skill: cardMenu.skill,
              label: cardMenu.label
            });
          } else {
            setPendingDelete({ kind: "custom", id: cardMenu.id, label: cardMenu.label });
          }
          setCardMenu(null);
        }}
      >
        删除
      </button>
    </div>
  ) : null;

  const confirmNode = (
    <ConfirmDialog
      open={pendingDelete !== null}
      title={pendingDelete ? `删除「${pendingDelete.label}」？` : ""}
      detail={
        pendingDelete?.kind === "custom"
          ? "将同时从本地库删除，删除后不可恢复。"
          : "删除后不再显示在列表中，可在列表下方「恢复」找回。"
      }
      onCancel={() => setPendingDelete(null)}
      onConfirm={performDelete}
    />
  );

  const [depForm, setDepForm] = useState<{
    mode: "add" | "edit";
    type: DependencyType;
    name: string;
    purpose: string;
    status: DependencyStatus;
    editKey: string;
  } | null>(null);

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(scope));
  }, [scope]);

  useEffect(() => {
    if (!onSelectionChange) {
      return;
    }
    let skills = 0;
    let deps = 0;
    for (const category of SCENARIO_TREE) {
      for (const scenario of category.scenarios) {
        if (!scope.selected.includes(scenarioKey(category.name, scenario.name))) {
          continue;
        }
        skills += scenario.skills.length;
        deps += depsOfScenario(category.name, scenario, scope).size;
      }
    }
    onSelectionChange({ scenarios: scope.selected, skills, deps });
  }, [scope, onSelectionChange]);

  const toggleScenario = useCallback((category: string, scenario: string) => {
    const key = scenarioKey(category, scenario);
    setScope((current) => ({
      ...current,
      selected: current.selected.includes(key)
        ? current.selected.filter((item) => item !== key)
        : [...current.selected, key]
    }));
  }, []);

  const goLevel2 = (category: string, scenario: string) => {
    setActiveCustomId(null);
    setActiveCategory(category);
    setActiveScenario(scenario);
    setLevel(2);
  };

  const goLevel3 = (skill: string) => {
    setActiveSkill(skill);
    setLevel(3);
  };

  const backToLevel1 = () => {
    setActiveCustomId(null);
    setLevel(1);
    setActiveCategory("");
    setActiveScenario("");
    setActiveSkill("");
  };

  const backToLevel2 = () => {
    setLevel(2);
    setActiveSkill("");
  };

  // 自建技能按大类展开，和内置场景排在一起
  const customScenarios = useMemo(() => (customSkills ?? []).map(buildCustomScenario), [customSkills]);

  const activeCustom = activeCustomId
    ? customScenarios.find((item) => item.id === activeCustomId) ?? null
    : null;

  const currentCategory: ScenarioCategory | undefined = useMemo(
    () => SCENARIO_TREE.find((item) => item.name === activeCategory),
    [activeCategory]
  );
  const currentScenario: BusinessScenario | undefined = useMemo(
    () => currentCategory?.scenarios.find((item) => item.name === activeScenario),
    [currentCategory, activeScenario]
  );
  const currentSkill: SubSkill | undefined = useMemo(
    () => currentScenario?.skills.find((item) => item.name === activeSkill),
    [currentScenario, activeSkill]
  );

  const depsOfSkill = (category: string, scenario: string, skill: SubSkill): Dependency[] => {
    const key = skillKey(category, scenario, skill.name);
    const base = skill.deps
      .map((dep) => scope.editedDeps[`${key}::${dep.type}::${dep.name}`] ?? dep)
      .filter((dep) => !scope.removedDeps.includes(`${key}::${dep.type}::${dep.name}`));
    return [...base, ...(scope.customDeps[key] ?? [])];
  };

  const saveDep = (category: string, scenario: string, skill: SubSkill) => {
    if (!depForm || !depForm.name.trim()) {
      onNotice("请填写依赖名称。");
      return;
    }
    const key = skillKey(category, scenario, skill.name);
    const dep: Dependency = {
      type: depForm.type,
      name: depForm.name.trim(),
      purpose: depForm.purpose.trim(),
      status: depForm.status
    };
    setScope((current) => {
      if (depForm.mode === "edit" && depForm.editKey) {
        const [, originalType, originalName] = depForm.editKey.split("::");
        const isCustom = (current.customDeps[key] ?? []).some(
          (item) => item.type === originalType && item.name === originalName
        );
        if (isCustom) {
          return {
            ...current,
            customDeps: {
              ...current.customDeps,
              [key]: (current.customDeps[key] ?? []).map((item) =>
                item.type === originalType && item.name === originalName ? dep : item
              )
            }
          };
        }
        return { ...current, editedDeps: { ...current.editedDeps, [depForm.editKey]: dep } };
      }
      return {
        ...current,
        customDeps: { ...current.customDeps, [key]: [...(current.customDeps[key] ?? []), dep] }
      };
    });
    setDepForm(null);
    onNotice(depForm.mode === "edit" ? "依赖已更新。" : "已导入新依赖。");
  };

  const removeDep = (category: string, scenario: string, skill: SubSkill, dep: Dependency) => {
    const key = skillKey(category, scenario, skill.name);
    setScope((current) => {
      const isCustom = (current.customDeps[key] ?? []).some(
        (item) => item.type === dep.type && item.name === dep.name
      );
      if (isCustom) {
        return {
          ...current,
          customDeps: {
            ...current.customDeps,
            [key]: (current.customDeps[key] ?? []).filter(
              (item) => !(item.type === dep.type && item.name === dep.name)
            )
          }
        };
      }
      return { ...current, removedDeps: [...current.removedDeps, `${key}::${dep.type}::${dep.name}`] };
    });
    onNotice("依赖已移除。");
  };

  const restoreSkillDeps = (category: string, scenario: string, skill: SubSkill) => {
    const key = skillKey(category, scenario, skill.name);
    setScope((current) => ({
      ...current,
      removedDeps: current.removedDeps.filter((item) => !item.startsWith(key + "::")),
      editedDeps: Object.fromEntries(
        Object.entries(current.editedDeps).filter(([itemKey]) => !itemKey.startsWith(key + "::"))
      )
    }));
    onNotice("已恢复该技能的默认依赖。");
  };

  // 当前 L2 场景的技能与依赖（内置 / 自建都适用）
  // 把本地编辑叠加到内置数据上（改文字 / 删除 / 新增）
  const effectiveSkills = (category: string, scenarioName: string, base: SubSkill[]): SubSkill[] => {
    const override = skillOverrides[scenarioKey(category, scenarioName)];
    const kept = base
      .filter((skill) => !(override?.removed ?? []).includes(skill.name))
      .map((skill) => ({ ...skill, desc: override?.texts?.[skill.name] ?? skill.desc, deps: skill.deps ?? [] }));
    const extras = (override?.extras ?? [])
      .filter(Boolean)
      .map((text) => ({ name: text.slice(0, 24), desc: text, deps: [] as Dependency[] }));
    return [...kept, ...extras];
  };

  const activeSkills = activeCustom
    ? activeCustom.lines.map((line) => ({ name: line, desc: line }))
    : currentScenario
      ? effectiveSkills(activeCategory, activeScenario, currentScenario.skills)
      : [];

  const runSkillMatch = async () => {
    const content = draftLines.map((line) => line.trim()).filter(Boolean).join("\n");
    if (!content) {
      return;
    }
    setMatching(true);
    try {
      const result = await matchSoftware(content);
      const lines = (result.lines ?? []).map((line) => ({
        text: line.text,
        software: line.software,
        site: line.site
      }));
      setMatches(lines);
      setHasMatched(true);
      if (lines.length === 0) {
        onNotice("没有匹配到软件，可到依赖页手动导入。");
      }
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "AI 技能匹配失败");
    } finally {
      setMatching(false);
    }
  };

  const openEditDialog = () => {
    const lines = activeSkills.map((skill) => skill.desc || skill.name);
    const stored = activeCustom
      ? []
      : skillOverrides[scenarioKey(activeCategory, activeScenario)]?.matches ?? [];
    setDraftLines(lines);
    setInitialLines(lines);
    setMatches(stored);
    setHasMatched(false);
    setEditOpen(true);
  };

  const saveEdits = () => {
    const cleaned = draftLines.map((line) => line.trim()).filter(Boolean);
    if (activeCustom) {
      onUpdateCustomContent?.(activeCustom.id, cleaned.join("\n"));
      setEditOpen(false);
      onNotice("已保存技能细分。");
      return;
    }
    const key = scenarioKey(activeCategory, activeScenario);
    const base = currentScenario?.skills ?? [];
    const texts: Record<string, string> = {};
    const removed: string[] = [];
    base.forEach((skill, index) => {
      const next = cleaned[index]?.trim();
      if (!next) {
        removed.push(skill.name);
        return;
      }
      if (next !== (skill.desc || skill.name)) {
        texts[skill.name] = next;
      }
    });
    const extras = cleaned.slice(base.length).filter(Boolean);
    setSkillOverrides((current) => ({ ...current, [key]: { texts, extras, removed, matches } }));
    setEditOpen(false);
    onNotice("已保存技能细分。");
  };

  const editDialogNode = editOpen ? (
    <div className="dialog-mask" onClick={() => setEditOpen(false)}>
      <div
        className="dialog"
        style={{ maxWidth: 720, width: "92%", maxHeight: "82vh", overflow: "auto" }}
        onClick={(event) => event.stopPropagation()}
      >
        <h2 className="dialog-title">编辑 · {activeCustom ? activeCustom.name : activeScenario}</h2>
        <p className="caption" style={{ marginTop: 6 }}>
          一行一条技能要求，回车可新增一行；删掉整行即移除该条。改完点「保存」。
        </p>

        <div className="edit-lines">
          {draftLines.map((line, index) => {
            const hit = matches.find((item) => item.text.trim() === line.trim());
            return (
              <div key={`line-${index}`} className="edit-line">
                <input
                  className="edit-line-input"
                  value={line}
                  placeholder="输入技能要求，回车新增一行"
                  onChange={(event) => {
                    const next = [...draftLines];
                    next[index] = event.target.value;
                    setDraftLines(next);
                  }}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") {
                      event.preventDefault();
                      const next = [...draftLines];
                      next.splice(index + 1, 0, "");
                      setDraftLines(next);
                    } else if (event.key === "Backspace" && line === "" && draftLines.length > 1) {
                      event.preventDefault();
                      setDraftLines(draftLines.filter((_, i) => i !== index));
                    }
                  }}
                />
                {hit && (
                  <span className="edit-line-tag">
                    [{hit.software ?? "无软件"}]
                    {hit.site ? `[${hit.site}]` : ""}
                  </span>
                )}
              </div>
            );
          })}
        </div>

        <div className="row" style={{ marginTop: 6 }}>
          <span
            role="button"
            className="caption"
            style={{ color: "var(--accent)", cursor: "pointer" }}
            onClick={() => {
              if (activeCustom) {
                return;
              }
              const key = scenarioKey(activeCategory, activeScenario);
              setSkillOverrides((current) => {
                const next = { ...current };
                delete next[key];
                return next;
              });
              const original = (currentScenario?.skills ?? []).map((skill) => skill.desc || skill.name);
              setDraftLines(original);
              setInitialLines(original);
              setMatches([]);
              setHasMatched(false);
              onNotice("已恢复默认内容，点「保存」生效。");
            }}
          >
            恢复默认内容
          </span>
        </div>

        <div
          className="row"
          style={{ marginTop: 16, justifyContent: "space-between", alignItems: "center" }}
        >
          <button
            type="button"
            className="secondary-btn"
            style={{ minWidth: 120 }}
            onClick={() => void runSkillMatch()}
            disabled={matching}
          >
            {matching ? "匹配中…" : "AI 技能匹配"}
          </button>
          <button
            type="button"
            className="save-btn"
            style={{ minWidth: 120 }}
            onClick={saveEdits}
            disabled={!hasMatched && draftLines.join("\n") === initialLines.join("\n")}
          >
            保存
          </button>
          <button
            type="button"
            className="secondary-btn"
            style={{ minWidth: 120 }}
            onClick={() => setEditOpen(false)}
          >
            关闭
          </button>
        </div>
      </div>
    </div>
  ) : null;


  // ---------- L1：一个大框装三类场景 ----------
  if (level === 1) {
    return (
      <section className="scope-box">
        {confirmNode}
        {menuNode}
        {editDialogNode}
        <div className="scope-box-head">
          <h2 className="scope-box-title">选择业务场景</h2>
          {action}
        </div>
        <div className="scope-groups">
        {SCENARIO_TREE.map((category) => (
          <section key={category.name} className="scope-group">
            <h3 className="scope-group-title">{category.name}</h3>
            <div className="scenario-grid">
              {category.scenarios
                .filter((scenario) => !hiddenScenarios.includes(scenarioKey(category.name, scenario.name)))
                .sort(
                  (left, right) =>
                    Number(scope.selected.includes(scenarioKey(category.name, right.name))) -
                    Number(scope.selected.includes(scenarioKey(category.name, left.name)))
                )
                .map((scenario) => {
                  const key = scenarioKey(category.name, scenario.name);
                  const active = scope.selected.includes(key);
                  const summary = scenarioSummary(scenario);
                  return (
                    <div
                      key={key}
                      className={`scenario-card${active ? " selected" : ""}`}
                      role="button"
                      tabIndex={0}
                      onClick={() => runSingleClick(() => toggleScenario(category.name, scenario.name))}
                      onDoubleClick={() => runDoubleClick(() => goLevel2(category.name, scenario.name))}
                      onContextMenu={(event) => {
                        event.preventDefault();
                        event.stopPropagation();
                        setCardMenu({
                          x: event.clientX,
                          y: event.clientY,
                          kind: "scenario",
                          category: category.name,
                          scenario: scenario.name,
                          label: scenario.name
                        });
                      }}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" || event.key === " ") {
                          event.preventDefault();
                          toggleScenario(category.name, scenario.name);
                        }
                      }}
                    >
                      <div className="scenario-text">
                        <span className="scenario-name">
                          {active && <span className="scenario-tick">✓</span>}
                          {scenario.name}
                        </span>
                        <span className="scenario-meta">
                          {summary.skills} 技能 · {summary.deps} 依赖
                          {summary.pending > 0 ? ` · ${summary.pending} 待就绪` : " · 全部就绪"}
                        </span>
                      </div>
                    </div>
                  );
                })}

              {customScenarios
                .filter((item) => item.category === category.name)
                .map((item) => {
                  const active = selectedCustom.includes(item.id);
                  return (
                    <div
                      key={item.id}
                      className={`scenario-card${active ? " selected" : ""}`}
                      role="button"
                      tabIndex={0}
                      onClick={() => runSingleClick(() => onToggleCustom?.(item.id))}
                      onDoubleClick={() =>
                        runDoubleClick(() => {
                          setActiveCustomId(item.id);
                          setActiveCategory(category.name);
                          setActiveScenario(item.name);
                          setLevel(2);
                        })
                      }
                      onContextMenu={(event) => {
                        event.preventDefault();
                        event.stopPropagation();
                        setCardMenu({
                          x: event.clientX,
                          y: event.clientY,
                          kind: "custom",
                          id: item.id,
                          label: item.name
                        });
                      }}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" || event.key === " ") {
                          event.preventDefault();
                          onToggleCustom?.(item.id);
                        }
                      }}
                    >
                      <div className="scenario-text">
                        <span className="scenario-name">
                          {active && <span className="scenario-tick">✓</span>}
                          {item.name}
                        </span>
                        <span className="scenario-meta">
                          {item.lines.length} 要求 · {item.deps.length} 依赖 · 自建
                        </span>
                      </div>
                    </div>
                  );
                })}
            </div>
          </section>
        ))}

        {hiddenScenarios.length > 0 && (
          <p className="caption" style={{ marginTop: 6 }}>
            已隐藏 {hiddenScenarios.length} 个业务场景 ·{" "}
            <span
              role="button"
              style={{ color: "var(--accent)", cursor: "pointer" }}
              onClick={() => {
                setHiddenScenarios([]);
                onNotice("已恢复全部业务场景。");
              }}
            >
              恢复
            </span>
          </p>
        )}

        </div>
      </section>
    );
  }

  // ---------- L2（自建技能）：技能要求 = 细分技能 ----------
  if (level === 2 && activeCustom) {
    return (
      <section className="scope-box">
        {confirmNode}
        {menuNode}
        {editDialogNode}
        <div className="scope-box-head">
          <h2 className="scope-box-title">技能细分 · {activeCustom.name}</h2>
          <div className="row" style={{ gap: 8 }}>
            <button type="button" className="secondary-btn" onClick={openEditDialog}>
              编辑
            </button>
            <button type="button" className="ghost-btn" onClick={() => onEditCustom?.(activeCustom.id)}>
              编辑技能定义
            </button>
            <button
              type="button"
              className="ghost-btn"
              onClick={() => {
                setActiveCustomId(null);
                setLevel(1);
              }}
            >
              ← 返回业务场景
            </button>
          </div>
        </div>
        <div className="scope-inner">
          <div className="skill-cards">
            {activeCustom.lines.map((line, index) => (
              <div
                key={`${index}-${line.slice(0, 8)}`}
                className="scenario-card skill-card"
                onDoubleClick={() => setLevel(3)}
              >
                <div className="scenario-text">
                  <span className="scenario-name">
                    {line.length > 22 ? `${line.slice(0, 22)}…` : line}
                  </span>
                  <span className="scenario-meta">
                    {activeCustom.deps.length} 依赖 · 自建要求
                  </span>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>
    );
  }

  // ---------- L3（自建技能）：匹配到的依赖 ----------
  if (level === 3 && activeCustom) {
    return (
      <section className="scope-box">
        {confirmNode}
        {menuNode}
        {editDialogNode}
        <div className="scope-box-head">
          <h2 className="scope-box-title">依赖 · {activeCustom.name}</h2>
          <button type="button" className="ghost-btn" onClick={() => setLevel(2)}>
            ← 返回细分技能
          </button>
        </div>
        <table className="dep-table">
          <thead>
            <tr>
              <th>依赖类型</th>
              <th>依赖名称</th>
              <th>用途</th>
              <th>就绪状态</th>
            </tr>
          </thead>
          <tbody>
            {activeCustom.deps.map((dep) => (
              <tr key={`${dep.type}-${dep.name}`}>
                <td>{dep.type}</td>
                <td>{dep.name}</td>
                <td className="caption">{dep.purpose || "—"}</td>
                <td>
                  <span className={`dep-status ${STATUS_CLASS[dep.status]}`}>{dep.status}</span>
                </td>
              </tr>
            ))}
            {activeCustom.deps.length === 0 && (
              <tr>
                <td colSpan={4}>这条技能还没有匹配到依赖，可在技能编辑里点「配置ai软件」。</td>
              </tr>
            )}
          </tbody>
        </table>
      </section>
    );
  }

  // ---------- L2：同样的框，只展示细分技能（无选择） ----------
  if (level === 2 && currentScenario) {
    return (
      <section className="scope-box">
        {confirmNode}
        {menuNode}
        {editDialogNode}
        <div className="scope-box-head">
          <h2 className="scope-box-title">技能细分 · {activeScenario}</h2>
          <div className="row" style={{ gap: 8 }}>
            <button type="button" className="secondary-btn" onClick={openEditDialog}>
              编辑
            </button>
            <button type="button" className="ghost-btn" onClick={backToLevel1}>
              ← 返回业务场景
            </button>
          </div>
        </div>
        <div className="scope-inner">
          <div className="skill-cards">
            {effectiveSkills(activeCategory, activeScenario, currentScenario.skills)
              .filter((skill) => !hiddenSkills.includes(skillKey(activeCategory, activeScenario, skill.name)))
              .map((skill) => {
              const deps = depsOfSkill(activeCategory, activeScenario, skill);
              const pending = deps.filter((dep) => dep.status !== "已就绪").length;
              return (
                <div
                  key={skill.name}
                  className="scenario-card skill-card"
                  onDoubleClick={() => goLevel3(skill.name)}
                  onContextMenu={(event) => {
                    event.preventDefault();
                    event.stopPropagation();
                    setCardMenu({
                      x: event.clientX,
                      y: event.clientY,
                      kind: "skill",
                      category: activeCategory,
                      scenario: activeScenario,
                      skill: skill.name,
                      label: skill.name
                    });
                  }}
                >
                  <div className="scenario-text">
                    <span className="scenario-name">{skill.name}</span>
                    <span
                      className="scenario-meta"
                      title={skill.desc}
                      style={{ overflow: "hidden", textOverflow: "ellipsis" }}
                    >
                      {skill.desc}
                    </span>
                    <span className="scenario-meta">
                      {deps.length} 依赖 · {pending > 0 ? `${pending} 待就绪` : "全部就绪"}
                    </span>
                  </div>
                </div>
              );
              })}
          </div>
          {hiddenSkills.filter((item) => item.startsWith(`${skillKey(activeCategory, activeScenario, "")}`)).length > 0 && (
            <p className="caption" style={{ marginTop: 10 }}>
              已隐藏该场景下{" "}
              {hiddenSkills.filter((item) => item.startsWith(`${skillKey(activeCategory, activeScenario, "")}`)).length} 项技能 ·{" "}
              <span
                role="button"
                style={{ color: "var(--accent)", cursor: "pointer" }}
                onClick={() => {
                  setHiddenSkills((current) =>
                    current.filter((item) => !item.startsWith(`${skillKey(activeCategory, activeScenario, "")}`))
                  );
                  onNotice("已恢复该场景的细分技能。");
                }}
              >
                恢复
              </span>
            </p>
          )}
        </div>
      </section>
    );
  }

  // ---------- L3：同样的框，展示依赖并可编辑 ----------
  if (level === 3 && currentSkill) {
    const deps = depsOfSkill(activeCategory, activeScenario, currentSkill);
    return (
      <section className="scope-box">
        {confirmNode}
        {menuNode}
        {editDialogNode}
        <div className="scope-box-head">
          <h2 className="scope-box-title">依赖 · {activeSkill}</h2>
          <button type="button" className="ghost-btn" onClick={backToLevel2}>
            ← 返回细分技能
          </button>
        </div>
        <p className="caption" style={{ marginTop: 4 }}>{currentSkill.desc}</p>

        <table className="dep-table">
          <thead>
            <tr>
              <th>依赖类型</th>
              <th>依赖名称</th>
              <th>用途</th>
              <th>就绪状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {deps.map((dep) => {
              const key = `${skillKey(activeCategory, activeScenario, activeSkill)}::${dep.type}::${dep.name}`;
              return (
                <tr key={key}>
                  <td>{dep.type}</td>
                  <td>{dep.name}</td>
                  <td className="caption">{dep.purpose || "—"}</td>
                  <td>
                    <span className={`dep-status ${STATUS_CLASS[dep.status]}`}>{dep.status}</span>
                  </td>
                  <td>
                    <div className="row" style={{ gap: 6 }}>
                      <button
                        type="button"
                        className="ghost-btn"
                        onClick={() =>
                          setDepForm({
                            mode: "edit",
                            type: dep.type,
                            name: dep.name,
                            purpose: dep.purpose,
                            status: dep.status,
                            editKey: key
                          })
                        }
                      >
                        编辑
                      </button>
                      <button
                        type="button"
                        className="ghost-btn"
                        onClick={() => removeDep(activeCategory, activeScenario, currentSkill, dep)}
                      >
                        移除
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
            {deps.length === 0 && (
              <tr>
                <td colSpan={5}>还没有依赖，可在下面导入。</td>
              </tr>
            )}
          </tbody>
        </table>

        {depForm ? (
          <div className="dep-form">
            <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
              <select
                className="input"
                value={depForm.type}
                onChange={(event) => setDepForm({ ...depForm, type: event.target.value as DependencyType })}
              >
                {DEPENDENCY_TYPES.map((type) => (
                  <option key={type} value={type}>{type}</option>
                ))}
              </select>
              <input
                className="input"
                placeholder="依赖名称"
                value={depForm.name}
                onChange={(event) => setDepForm({ ...depForm, name: event.target.value })}
              />
              <input
                className="input"
                placeholder="用途说明"
                value={depForm.purpose}
                onChange={(event) => setDepForm({ ...depForm, purpose: event.target.value })}
              />
              <select
                className="input"
                value={depForm.status}
                onChange={(event) => setDepForm({ ...depForm, status: event.target.value as DependencyStatus })}
              >
                {DEPENDENCY_STATUSES.map((status) => (
                  <option key={status} value={status}>{status}</option>
                ))}
              </select>
            </div>
            <div className="row" style={{ gap: 8, marginTop: 8 }}>
              <button type="button" className="primary-btn" onClick={() => saveDep(activeCategory, activeScenario, currentSkill)}>
                {depForm.mode === "edit" ? "保存修改" : "导入依赖"}
              </button>
              <button type="button" className="secondary-btn" onClick={() => setDepForm(null)}>取消</button>
            </div>
          </div>
        ) : (
          <div className="row" style={{ gap: 8, marginTop: 12 }}>
            <button
              type="button"
              className="secondary-btn"
              onClick={() =>
                setDepForm({ mode: "add", type: "本地软件", name: "", purpose: "", status: "待接入", editKey: "" })
              }
            >
              + 导入新依赖
            </button>
            <button
              type="button"
              className="ghost-btn"
              onClick={() => restoreSkillDeps(activeCategory, activeScenario, currentSkill)}
            >
              恢复默认依赖
            </button>
          </div>
        )}
      </section>
    );
  }

  return null;
}

/** 汇总一个场景下所有技能的去重依赖 */
function depsOfScenario(category: string, scenario: BusinessScenario, scope: ScopeState): Map<string, Dependency> {
  const out = new Map<string, Dependency>();
  for (const skill of scenario.skills) {
    const key = skillKey(category, scenario.name, skill.name);
    const base = skill.deps
      .map((dep) => scope.editedDeps[`${key}::${dep.type}::${dep.name}`] ?? dep)
      .filter((dep) => !scope.removedDeps.includes(`${key}::${dep.type}::${dep.name}`));
    for (const dep of [...base, ...(scope.customDeps[key] ?? [])]) {
      out.set(`${dep.type}::${dep.name}`, dep);
    }
  }
  return out;
}
