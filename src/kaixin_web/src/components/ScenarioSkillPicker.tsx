import { useCallback, useEffect, useMemo, useState } from "react";
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
// 需求要点：
//   · 普通用户停在 L1，点场景框即选中
//   · 场景框上的「修改」进入 L2；L2 可勾选去掉不需要的技能
//   · L2 每项技能的「修改」进入 L3；L3 可改依赖、删除、导入新依赖
//   · 用页面内切换视图 + 面包屑返回（桌面端最稳）

interface Props {
  onNotice: (message: string) => void;
  onSelectionChange?: (summary: { scenarios: string[]; skills: number; deps: number }) => void;
}

interface ScopeState {
  selected: string[];
  disabledSkills: string[];
  customDeps: Record<string, Dependency[]>;
  removedDeps: string[];
  editedDeps: Record<string, Dependency>;
}

const STORAGE_KEY = "kaixin.scenarioScope.v1";

const EMPTY_SCOPE: ScopeState = {
  selected: [],
  disabledSkills: [],
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
      disabledSkills: parsed.disabledSkills ?? [],
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

export function ScenarioSkillPicker({ onNotice, onSelectionChange }: Props) {
  const [scope, setScope] = useState<ScopeState>(() => loadScope());
  const [level, setLevel] = useState<1 | 2 | 3>(1);
  const [activeCategory, setActiveCategory] = useState("");
  const [activeScenario, setActiveScenario] = useState("");
  const [activeSkill, setActiveSkill] = useState("");

  // 导入/编辑依赖的表单
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
        skills += scenario.skills.filter(
          (skill) => !scope.disabledSkills.includes(skillKey(category.name, scenario.name, skill.name))
        ).length;
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

  const toggleSkill = useCallback((category: string, scenario: string, skill: string) => {
    const key = skillKey(category, scenario, skill);
    setScope((current) => ({
      ...current,
      disabledSkills: current.disabledSkills.includes(key)
        ? current.disabledSkills.filter((item) => item !== key)
        : [...current.disabledSkills, key]
    }));
  }, []);

  const goLevel2 = (category: string, scenario: string) => {
    setActiveCategory(category);
    setActiveScenario(scenario);
    setLevel(2);
  };

  const goLevel3 = (skill: string) => {
    setActiveSkill(skill);
    setLevel(3);
  };

  const backToLevel1 = () => {
    setLevel(1);
    setActiveCategory("");
    setActiveScenario("");
    setActiveSkill("");
  };

  const backToLevel2 = () => {
    setLevel(2);
    setActiveSkill("");
  };

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
      .map((dep) => {
        const edited = scope.editedDeps[`${key}::${dep.type}::${dep.name}`];
        return edited ?? dep;
      })
      .filter((dep) => !scope.removedDeps.includes(`${key}::${dep.type}::${dep.name}`));
    const extra = scope.customDeps[key] ?? [];
    return [...base, ...extra];
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
        return {
          ...current,
          editedDeps: { ...current.editedDeps, [depForm.editKey]: dep }
        };
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
    const fullKey = `${key}::${dep.type}::${dep.name}`;
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
      return { ...current, removedDeps: [...current.removedDeps, fullKey] };
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

  // ---------- L1 ----------
  if (level === 1) {
    return (
      <div>
        <p className="caption" style={{ marginBottom: 12 }}>
          先选业务场景即可；需要调整时点场景框上的「修改」，进入细分技能与依赖。
        </p>
        {SCENARIO_TREE.map((category) => (
          <section key={category.name} className="scenario-section">
            <div className="row" style={{ justifyContent: "space-between", marginTop: 16 }}>
              <h3 className="scenario-category">{category.name}</h3>
              <span className="caption">{category.scenarios.length} 个业务场景</span>
            </div>
            <div className="scenario-grid">
              {category.scenarios.map((scenario) => {
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
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === " ") {
                        event.preventDefault();
                        toggleScenario(category.name, scenario.name);
                      }
                    }}
                  >
                    <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start" }}>
                      <span className="scenario-name">{scenario.name}</span>
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
                    <span className="caption" style={{ display: "block", marginTop: 6 }}>
                      {summary.skills} 项技能 · {summary.deps} 个依赖
                      {summary.pending > 0 ? ` · ${summary.pending} 项待就绪` : " · 全部就绪"}
                    </span>
                    {active && <span className="scenario-check">✓ 已选择</span>}
                  </div>
                );
              })}
            </div>
          </section>
        ))}
        <p className="caption" style={{ marginTop: 14 }}>
          已选 {scope.selected.length} 个场景。
        </p>
      </div>
    );
  }

  // ---------- L2 ----------
  if (level === 2 && currentScenario) {
    const enabled = currentScenario.skills.filter(
      (skill) => !scope.disabledSkills.includes(skillKey(activeCategory, activeScenario, skill.name))
    ).length;
    return (
      <div>
        <button type="button" className="ghost-btn" onClick={backToLevel1}>
          ← 返回业务场景
        </button>
        <p className="picker-breadcrumb">
          <span onClick={backToLevel1} role="presentation">业务场景</span>
          <span> / </span>
          <b>{activeScenario}</b>
        </p>
        <p className="caption">
          取消勾选即从本次范围中去掉；点「修改」查看这项技能需要哪些软件、模型和网站。
          已启用 {enabled} / {currentScenario.skills.length} 项。
        </p>

        <div className="skill-scope-list">
          {currentScenario.skills.map((skill) => {
            const key = skillKey(activeCategory, activeScenario, skill.name);
            const on = !scope.disabledSkills.includes(key);
            const deps = depsOfSkill(activeCategory, activeScenario, skill);
            return (
              <div key={key} className={`skill-scope-row${on ? "" : " off"}`}>
                <label className="skill-scope-main">
                  <input type="checkbox" checked={on} onChange={() => toggleSkill(activeCategory, activeScenario, skill.name)} />
                  <span>
                    <b>{skill.name}</b>
                    <span className="caption" style={{ display: "block" }}>{skill.desc}</span>
                    <span className="caption" style={{ display: "block" }}>
                      {deps.length} 个依赖：{deps.map((dep) => dep.name).join("、")}
                    </span>
                  </span>
                </label>
                <button type="button" className="secondary-btn" onClick={() => goLevel3(skill.name)}>
                  修改
                </button>
              </div>
            );
          })}
        </div>
      </div>
    );
  }

  // ---------- L3 ----------
  if (level === 3 && currentSkill) {
    const deps = depsOfSkill(activeCategory, activeScenario, currentSkill);
    return (
      <div>
        <button type="button" className="ghost-btn" onClick={backToLevel2}>
          ← 返回细分技能
        </button>
        <p className="picker-breadcrumb">
          <span onClick={backToLevel1} role="presentation">业务场景</span>
          <span> / </span>
          <span onClick={backToLevel2} role="presentation">{activeScenario}</span>
          <span> / </span>
          <b>{activeSkill}</b>
        </p>
        <p className="caption">{currentSkill.desc}</p>

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
                      <button type="button" className="ghost-btn" onClick={() => removeDep(activeCategory, activeScenario, currentSkill, dep)}>
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
            <button type="button" className="ghost-btn" onClick={() => restoreSkillDeps(activeCategory, activeScenario, currentSkill)}>
              恢复默认依赖
            </button>
          </div>
        )}
      </div>
    );
  }

  return null;
}

/** 汇总一个场景下所有启用技能的去重依赖 */
function depsOfScenario(category: string, scenario: BusinessScenario, scope: ScopeState): Map<string, Dependency> {
  const out = new Map<string, Dependency>();
  for (const skill of scenario.skills) {
    const key = skillKey(category, scenario.name, skill.name);
    if (scope.disabledSkills.includes(key)) {
      continue;
    }
    const base = skill.deps
      .map((dep) => scope.editedDeps[`${key}::${dep.type}::${dep.name}`] ?? dep)
      .filter((dep) => !scope.removedDeps.includes(`${key}::${dep.type}::${dep.name}`));
    for (const dep of [...base, ...(scope.customDeps[key] ?? [])]) {
      out.set(`${dep.type}::${dep.name}`, dep);
    }
  }
  return out;
}
