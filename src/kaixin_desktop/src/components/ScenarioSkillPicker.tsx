import { useCallback, useEffect, useMemo, useState } from "react";
import { ConfirmDialog } from "./ConfirmDialog";
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
  /** L1 大框底部的内容（补充需求） */
  footer?: React.ReactNode;
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
  footer,
  action,
  customSkills = [],
  selectedCustom = [],
  onToggleCustom,
  onEditCustom,
  onDeleteCustom
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

  // ---------- L1：一个大框装三类场景 ----------
  if (level === 1) {
    return (
      <section className="scope-box">
        {confirmNode}
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
                      onClick={() => toggleScenario(category.name, scenario.name)}
                      onContextMenu={(event) => {
                        event.preventDefault();
                        setPendingDelete({
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
                      <button
                        type="button"
                        className="scenario-edit"
                        onClick={(event) => {
                          event.stopPropagation();
                          goLevel2(category.name, scenario.name);
                        }}
                      >
                        修改
                      </button>
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
                      onClick={() => onToggleCustom?.(item.id)}
                      onContextMenu={(event) => {
                        event.preventDefault();
                        setPendingDelete({ kind: "custom", id: item.id, label: item.name });
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
                      <button
                        type="button"
                        className="scenario-edit"
                        onClick={(event) => {
                          event.stopPropagation();
                          setActiveCustomId(item.id);
                          setActiveCategory(category.name);
                          setActiveScenario(item.name);
                          setLevel(2);
                        }}
                      >
                        修改
                      </button>
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
        {footer && <div className="scope-supplement">{footer}</div>}
      </section>
    );
  }

  // ---------- L2（自建技能）：技能要求 = 细分技能 ----------
  if (level === 2 && activeCustom) {
    return (
      <section className="scope-box">
        {confirmNode}
        <div className="scope-box-head">
          <h2 className="scope-box-title">技能细分 · {activeCustom.name}</h2>
          <div className="row" style={{ gap: 8 }}>
            <button type="button" className="secondary-btn" onClick={() => onEditCustom?.(activeCustom.id)}>
              编辑技能
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
              <div key={`${index}-${line.slice(0, 8)}`} className="scenario-card skill-card">
                <div className="scenario-text">
                  <span className="scenario-name">
                    {line.length > 22 ? `${line.slice(0, 22)}…` : line}
                  </span>
                  <span className="scenario-meta">
                    {activeCustom.deps.length} 依赖 · 自建要求
                  </span>
                </div>
                <button type="button" className="scenario-edit" onClick={() => setLevel(3)}>
                  修改
                </button>
              </div>
            ))}
          </div>
        </div>
        {footer && <div className="scope-supplement">{footer}</div>}
      </section>
    );
  }

  // ---------- L3（自建技能）：匹配到的依赖 ----------
  if (level === 3 && activeCustom) {
    return (
      <section className="scope-box">
        {confirmNode}
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
        {footer && <div className="scope-supplement">{footer}</div>}
      </section>
    );
  }

  // ---------- L2：同样的框，只展示细分技能（无选择） ----------
  if (level === 2 && currentScenario) {
    return (
      <section className="scope-box">
        {confirmNode}
        <div className="scope-box-head">
          <h2 className="scope-box-title">技能细分 · {activeScenario}</h2>
          <button type="button" className="ghost-btn" onClick={backToLevel1}>
            ← 返回业务场景
          </button>
        </div>
        <div className="scope-inner">
          <div className="skill-cards">
            {currentScenario.skills
              .filter((skill) => !hiddenSkills.includes(skillKey(activeCategory, activeScenario, skill.name)))
              .map((skill) => {
              const deps = depsOfSkill(activeCategory, activeScenario, skill);
              const pending = deps.filter((dep) => dep.status !== "已就绪").length;
              return (
                <div
                  key={skill.name}
                  className="scenario-card skill-card"
                  onContextMenu={(event) => {
                    event.preventDefault();
                    setPendingDelete({
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
                    <span className="scenario-meta">
                      {deps.length} 依赖 · {pending > 0 ? `${pending} 待就绪` : "全部就绪"}
                    </span>
                  </div>
                  <button
                    type="button"
                    className="scenario-edit"
                    onClick={() => goLevel3(skill.name)}
                  >
                    修改
                  </button>
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
        {footer && <div className="scope-supplement">{footer}</div>}
      </section>
    );
  }

  // ---------- L3：同样的框，展示依赖并可编辑 ----------
  if (level === 3 && currentSkill) {
    const deps = depsOfSkill(activeCategory, activeScenario, currentSkill);
    return (
      <section className="scope-box">
        {confirmNode}
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
        {footer && <div className="scope-supplement">{footer}</div>}
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
