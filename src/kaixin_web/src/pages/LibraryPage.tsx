import { useMemo, useState } from "react";
import { CATEGORY_GROUPS, DOCS, type DocItem } from "../data";

interface Props {
  onNotice: (message: string) => void;
}

export function LibraryPage({ onNotice }: Props) {
  const [group, setGroup] = useState("全部资料");
  const [category, setCategory] = useState("全部");
  const [expanded, setExpanded] = useState<string[]>(["工作资料"]);
  const [selectedId, setSelectedId] = useState(DOCS[0].id);
  const [keyword, setKeyword] = useState("");

  const filtered = useMemo(() => {
    return DOCS.filter((doc) => {
      const groupMatch = group === "全部资料" || doc.group === group;
      const categoryMatch = category === "全部" || doc.category === category;
      const keywordMatch =
        !keyword.trim() ||
        doc.title.includes(keyword.trim()) ||
        doc.summary.includes(keyword.trim());
      return groupMatch && categoryMatch && keywordMatch;
    });
  }, [group, category, keyword]);

  const selected = filtered.find((doc) => doc.id === selectedId) ?? filtered[0] ?? docFallback;

  const toggleGroup = (name: string) => {
    setExpanded((current) =>
      current.includes(name) ? current.filter((item) => item !== name) : [...current, name]
    );
  };

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="page-title">本地资料</h1>
          <p className="page-subtitle">按大类与小类浏览本机资料，查看统计看板与文档详情</p>
        </div>
        <button
          type="button"
          className="secondary-btn"
          onClick={() => onNotice("导入本地文件夹将在功能实现阶段接入。")}
        >
          导入资料
        </button>
      </div>

      <div className="stats">
        <div className="card">
          <div className="caption">资料总数</div>
          <div className="stat-value">128 份</div>
        </div>
        <div className="card">
          <div className="caption">已接入知识库</div>
          <div className="stat-value">96 份</div>
        </div>
        <div className="card">
          <div className="caption">本周新增</div>
          <div className="stat-value">12 份</div>
        </div>
        <div className="card">
          <div className="caption">占用空间</div>
          <div className="stat-value">2.6 GB</div>
        </div>
      </div>

      <div className="library-layout">
        <div className="card" style={{ padding: 12 }}>
          <div className="row" style={{ justifyContent: "space-between", padding: "0 4px 8px" }}>
            <span className="section-title">资料分类</span>
            <span className="tag">5 大类</span>
          </div>
          <button
            type="button"
            className="tree-group"
            onClick={() => {
              setGroup("全部资料");
              setCategory("全部");
            }}
            style={group === "全部资料" ? { background: "var(--accent-light)", color: "var(--accent)" } : undefined}
          >
            <span>全部资料</span>
            <span className="caption">128 份</span>
          </button>
          {CATEGORY_GROUPS.map((item) => (
            <div key={item.name}>
              <button type="button" className="tree-group" onClick={() => toggleGroup(item.name)}>
                <span className="row" style={{ gap: 8 }}>
                  <span className="skill-icon" style={{ width: 20, height: 20, flex: "0 0 20px", fontSize: 10 }}>
                    {item.icon}
                  </span>
                  {item.name}
                </span>
                <span className="caption">
                  {item.count} {expanded.includes(item.name) ? "▾" : "▸"}
                </span>
              </button>
              {expanded.includes(item.name) &&
                item.children.map((child) => (
                  <button
                    type="button"
                    key={child.name}
                    className={`tree-sub${group === item.name && category === child.name ? " active" : ""}`}
                    onClick={() => {
                      setGroup(item.name);
                      setCategory(child.name);
                    }}
                  >
                    <span>{child.name}</span>
                    <span>{child.count}</span>
                  </button>
                ))}
            </div>
          ))}
        </div>

        <div className="card">
          <div className="row" style={{ justifyContent: "space-between" }}>
            <div>
              <span className="section-title">
                {group} / {category}
              </span>
              <div className="caption" style={{ marginTop: 5 }}>
                共 {filtered.length} 份资料
              </div>
            </div>
            <input
              className="input"
              style={{ width: 200 }}
              placeholder="搜索标题或摘要"
              value={keyword}
              onChange={(event) => setKeyword(event.target.value)}
            />
          </div>

          <div style={{ marginTop: 12, display: "grid", gridTemplateColumns: "1fr 300px", gap: 14 }}>
            <div>
              {filtered.length === 0 && <div className="empty">该分类下暂无资料</div>}
              {filtered.map((doc) => (
                <button
                  type="button"
                  key={doc.id}
                  className={`doc-row${selected.id === doc.id ? " selected" : ""}`}
                  onClick={() => setSelectedId(doc.id)}
                >
                  <div className="doc-name">{doc.title}</div>
                  <div className="doc-meta">
                    {doc.type} · {doc.size} · {doc.date}
                  </div>
                </button>
              ))}
            </div>

            <div
              className="card"
              style={{ background: "var(--subtle)", border: "none", padding: 14, marginTop: 0 }}
            >
              <div className="doc-name">{selected.title}</div>
              <div className="doc-meta">
                {selected.group} / {selected.category} · {selected.type}
              </div>
              <p className="list-card-detail">{selected.summary}</p>
              <div className="row" style={{ marginTop: 12 }}>
                <button
                  type="button"
                  className="secondary-btn"
                  onClick={() => onNotice("全文阅读将在功能实现阶段接入。")}
                >
                  打开文档
                </button>
                <button
                  type="button"
                  className="ghost-btn"
                  onClick={() => onNotice("加入知识库将在功能实现阶段接入。")}
                >
                  加入知识库
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

const docFallback: DocItem = {
  id: "none",
  title: "未选择资料",
  group: "-",
  category: "-",
  type: "-",
  size: "-",
  date: "-",
  summary: "请选择左侧资料查看详情。"
};
