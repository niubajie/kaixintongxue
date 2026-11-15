import { useState } from "react";
import { HARDWARE, SKILLS, type SkillOption } from "../data";

interface Props {
  onNotice: (message: string) => void;
}

export function SkillsPage({ onNotice }: Props) {
  const [mode, setMode] = useState<"existing" | "upgrade">("existing");
  const [selected, setSelected] = useState<string[]>(["kb", "office"]);
  const [supplement, setSupplement] = useState("");
  const [generated, setGenerated] = useState(false);

  const toggleSkill = (skill: SkillOption) => {
    setGenerated(false);
    setSelected((current) =>
      current.includes(skill.id) ? current.filter((id) => id !== skill.id) : [...current, skill.id]
    );
  };

  const generate = () => {
    if (selected.length === 0) {
      onNotice("请至少选择一项技能，再生成方案。");
      return;
    }
    setGenerated(true);
    onNotice("方案为演示数据；部署会执行真实环境检测与软件安装。");
  };

  const deploy = () => {
    onNotice("部署需要先登录或注册，并完成真实安装包的部署。");
  };

  const selectedSkills = SKILLS.filter((skill) => selected.includes(skill.id));

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="page-title">技能部署</h1>
          <p className="page-subtitle">选择需要的 AI 技能，生成方案并部署到本机</p>
        </div>
        <span className="tag">本机检测结果（演示）</span>
      </div>

      <div className="mode-cards">
        <button
          type="button"
          className={`mode-card${mode === "existing" ? " active" : ""}`}
          onClick={() => setMode("existing")}
        >
          <div className="list-card-title">现有电脑 + AI 技能</div>
          <p className="list-card-detail">基于当前硬件配置，选择可运行的 AI 技能</p>
        </button>
        <button
          type="button"
          className={`mode-card${mode === "upgrade" ? " active" : ""}`}
          onClick={() => setMode("upgrade")}
        >
          <div className="list-card-title">AI 技能 + 硬件配置</div>
          <p className="list-card-detail">按所选技能反推推荐硬件与价格（演示）</p>
        </button>
      </div>

      <div className="card" style={{ marginTop: 14 }}>
        <div className="row" style={{ justifyContent: "space-between" }}>
          <h2 className="section-title">{mode === "existing" ? "本机硬件配置" : "推荐硬件配置（演示）"}</h2>
          <button
            type="button"
            className="ghost-btn"
            onClick={() => onNotice("重新检测硬件将在功能实现阶段接入。")}
          >
            重新检测
          </button>
        </div>
        <div className="hw-grid">
          {HARDWARE.map((item) => (
            <div key={item.label} className="card" style={{ padding: 12, borderRadius: 9 }}>
              <div className="caption">{item.label}</div>
              <div style={{ marginTop: 5, fontSize: 13, fontWeight: 600 }}>
                {mode === "existing" ? item.value : `推荐 ${item.value}`}
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="row" style={{ justifyContent: "space-between", marginTop: 22 }}>
        <h2 className="section-title">选择技能</h2>
        <span className="caption">已选 {selected.length} 项</span>
      </div>
      <div className="skill-grid">
        {SKILLS.map((skill) => (
          <button
            type="button"
            key={skill.id}
            className={`skill-card${selected.includes(skill.id) ? " selected" : ""}`}
            onClick={() => toggleSkill(skill)}
          >
            <span className="skill-icon">{skill.icon}</span>
            <span style={{ flex: 1 }}>
              <span className="skill-name">{skill.name}</span>
              <span className="skill-desc" style={{ display: "block" }}>
                {skill.description}
              </span>
            </span>
            <span className="tag">{selected.includes(skill.id) ? "已选" : `需 ${skill.demand} 核`}</span>
          </button>
        ))}
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
        <button type="button" className="primary-btn" onClick={generate}>
          生成技能方案
        </button>
        <span className="caption">方案内容为演示数据，不构成实际采购建议</span>
      </div>

      {generated && (
        <div className="card" style={{ marginTop: 18 }}>
          <div className="row" style={{ justifyContent: "space-between" }}>
            <h2 className="section-title">{mode === "existing" ? "现有电脑 AI 技能方案" : "AI 技能与硬件配置方案"}</h2>
            <span className="tag accent">演示方案</span>
          </div>
          <div className="chip-row" style={{ display: "flex", flexWrap: "wrap", gap: 6, marginTop: 10 }}>
            {selectedSkills.map((skill) => (
              <span className="tag" key={skill.id}>
                {skill.name}
              </span>
            ))}
          </div>
          {supplement.trim() && (
            <p className="list-card-detail">补充需求：{supplement.trim()}</p>
          )}
          <p className="list-card-detail">
            演示结论：所选技能可在当前配置下运行，其中实时语音与视觉问答对显存要求较高，建议优先保障内存与显卡。
          </p>
          <div className="row" style={{ marginTop: 14 }}>
            <button type="button" className="primary-btn" onClick={deploy}>
              部署到本机
            </button>
            <button
              type="button"
              className="secondary-btn"
              onClick={() => onNotice("已保存方案到本机（演示）。")}
            >
              保存方案
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
