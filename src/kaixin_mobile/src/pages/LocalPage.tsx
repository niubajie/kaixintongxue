import { useState } from "react";
import { BottomSheet, MobileHeader } from "../components/MobileHeader";
import { IconDoc } from "../components/Icons";
import { CATEGORY_GROUPS, DOCS, type DocItem } from "../data";

interface Props {
  onBack: () => void;
  onNotice: (message: string) => void;
}

export function LocalPage({ onBack, onNotice }: Props) {
  const [group, setGroup] = useState("全部");
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<DocItem | null>(null);

  const groups = ["全部", ...CATEGORY_GROUPS.map((item) => item.name)];
  const list = DOCS.filter(
    (doc) =>
      (group === "全部" || doc.group === group) &&
      (query.trim() === "" || doc.title.includes(query.trim()))
  );

  return (
    <>
      <MobileHeader title="本地资料" onBack={onBack} />

      <div className="page-scroll">
        <div className="stat-row">
          <div className="stat">
            <strong>128</strong>
            <span>资料总数</span>
          </div>
          <div className="stat">
            <strong>96</strong>
            <span>已入知识库</span>
          </div>
          <div className="stat">
            <strong>2.6G</strong>
            <span>占用空间</span>
          </div>
        </div>

        <input
          className="input search"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="搜索资料标题"
        />

        <div className="chip-scroll">
          {groups.map((item) => (
            <button
              key={item}
              type="button"
              className={`chip${group === item ? " active" : ""}`}
              onClick={() => setGroup(item)}
            >
              {item}
            </button>
          ))}
        </div>

        {list.map((doc) => (
          <button key={doc.id} type="button" className="card tap doc-row" onClick={() => setSelected(doc)}>
            <span className="doc-icon">
              <IconDoc />
            </span>
            <span className="doc-main">
              <strong>{doc.title}</strong>
              <span className="muted">
                {doc.category} · {doc.type} · {doc.size} · {doc.date}
              </span>
            </span>
          </button>
        ))}

        {list.length === 0 && <p className="empty">没有匹配的资料，换个关键词试试。</p>}
      </div>

      <BottomSheet open={selected !== null} title="资料详情" onClose={() => setSelected(null)}>
        {selected && (
          <>
            <h2 className="sheet-title">{selected.title}</h2>
            <p className="muted">
              {selected.group} / {selected.category} · {selected.type} · {selected.size}
            </p>
            <p className="sheet-text">{selected.summary}</p>
            <div className="sheet-actions">
              <button
                type="button"
                className="primary-btn"
                onClick={() => {
                  onNotice("已加入知识库，可用于问答检索。");
                  setSelected(null);
                }}
              >
                加入知识库
              </button>
            </div>
          </>
        )}
      </BottomSheet>
    </>
  );
}
