import { MobileHeader } from "../components/MobileHeader";
import { HARDWARE, SKILLS } from "../data";
import { usePersistentState } from "../assistant";
import { IconCheck } from "../components/Icons";

interface Props {
  onBack: () => void;
  onNotice: (message: string) => void;
}

export function SkillsPage({ onBack, onNotice }: Props) {
  const [picked, setPicked] = usePersistentState<string[]>("kaixin.mobile.skills", ["kb", "office"]);

  const toggle = (id: string) => {
    setPicked((list) => (list.includes(id) ? list.filter((one) => one !== id) : [...list, id]));
  };

  return (
    <>
      <MobileHeader title="技能部署" onBack={onBack} />

      <div className="page-scroll">
        <div className="card">
          <div className="stage-title">本机硬件</div>
          <div className="hardware-grid">
            {HARDWARE.map((item) => (
              <div key={item.label}>
                <span className="muted">{item.label}</span>
                <strong>{item.value}</strong>
              </div>
            ))}
          </div>
        </div>

        <p className="section-label">AI 技能 · 已选 {picked.length}</p>

        {SKILLS.map((skill) => {
          const active = picked.includes(skill.id);
          return (
            <button
              key={skill.id}
              type="button"
              className={`card tap skill-row${active ? " active" : ""}`}
              onClick={() => toggle(skill.id)}
            >
              <span className="skill-icon">{skill.icon}</span>
              <span className="skill-main">
                <strong>{skill.name}</strong>
                <span className="muted">{skill.description}</span>
              </span>
              <span className={`check${active ? " on" : ""}`}>{active && <IconCheck />}</span>
            </button>
          );
        })}
      </div>

      <div className="sticky-bottom">
        <button
          type="button"
          className="primary-btn block"
          onClick={() => onNotice(`已生成部署计划：${picked.length} 项技能，演示环境不执行安装。`)}
        >
          部署所选技能（{picked.length}）
        </button>
      </div>
    </>
  );
}
