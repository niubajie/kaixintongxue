import { useCallback, useEffect, useMemo, useState } from "react";
import { bridge } from "../platform";
import {
  fetchKBStatus,
  fetchLibraryScan,
  indexKnowledgeBase,
  removeKnowledgeBase,
  setLibraryDir,
  openLibraryFile,
  openLibraryFolder,
  type KBDocument,
  type LibraryItem,
  type LibraryScan
} from "../deployApi";

interface Props {
  onNotice: (message: string) => void;
}

const ALL = "全部资料";

export function LibraryPage({ onNotice }: Props) {
  const [state, setState] = useState<"loading" | "ready" | "error">("loading");
  const [scan, setScan] = useState<LibraryScan | null>(null);
  const [error, setError] = useState("");
  const [group, setGroup] = useState(ALL);
  const [keyword, setKeyword] = useState("");
  const [selectedPath, setSelectedPath] = useState("");
  const [kb, setKb] = useState<Record<string, KBDocument>>({});
  const [kbBusy, setKbBusy] = useState(false);

  const load = useCallback(async () => {
    setState("loading");
    setError("");
    try {
      const [result, kbDocs] = await Promise.all([
        fetchLibraryScan(),
        fetchKBStatus().catch(() => [] as KBDocument[])
      ]);
      setScan(result);
      const map: Record<string, KBDocument> = {};
      for (const doc of kbDocs) {
        map[doc.path] = doc;
      }
      setKb(map);
      setState("ready");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "扫描资料目录失败");
      setScan(null);
      setState("error");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const groups = useMemo(() => {
    const map = new Map<string, number>();
    for (const item of scan?.items ?? []) {
      map.set(item.group, (map.get(item.group) ?? 0) + 1);
    }
    return [...map.entries()].sort((a, b) => b[1] - a[1]);
  }, [scan]);

  const filtered = useMemo(() => {
    const items = scan?.items ?? [];
    const kw = keyword.trim();
    return items.filter((item) => {
      const groupMatch = group === ALL || item.group === group;
      const keywordMatch = !kw || item.name.includes(kw) || item.relPath.includes(kw);
      return groupMatch && keywordMatch;
    });
  }, [scan, group, keyword]);

  const selected: LibraryItem | undefined =
    filtered.find((item) => item.path === selectedPath) ?? filtered[0];

  const handleOpenFile = async (item: LibraryItem) => {
    try {
      await openLibraryFile(item.path);
      onNotice(`已用默认程序打开：${item.name}`);
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "打开文件失败");
    }
  };

  const selectedKB = selected ? kb[selected.path] : undefined;
  const indexedCount = Object.values(kb).filter((doc) => doc.status === "indexed").length;

  const handleIndex = async () => {
    if (!selected) {
      return;
    }
    setKbBusy(true);
    try {
      const result = await indexKnowledgeBase(selected.path);
      const item = result.results[0];
      if (item) {
        setKb((current) => ({ ...current, [item.path]: item }));
        onNotice(
          item.status === "indexed"
            ? `已加入知识库：${item.name}（${item.chunks} 个切片）`
            : `未能加入：${item.error || item.status}`
        );
      }
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "加入知识库失败");
    } finally {
      setKbBusy(false);
    }
  };

  const handleUnindex = async () => {
    if (!selected) {
      return;
    }
    setKbBusy(true);
    try {
      await removeKnowledgeBase(selected.path);
      setKb((current) => {
        const next = { ...current };
        delete next[selected.path];
        return next;
      });
      onNotice(`已移出知识库：${selected.name}`);
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "移出知识库失败");
    } finally {
      setKbBusy(false);
    }
  };

  const handlePickFolder = async () => {
    let picked: string | null = null;
    try {
      picked = await bridge.pickFolder();
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "无法打开文件夹选择器");
      return;
    }
    if (!picked) {
      onNotice("未选择资料文件夹。");
      return;
    }
    try {
      const result = await setLibraryDir(picked);
      setScan(result);
      setState("ready");
      setGroup(ALL);
      onNotice(`已切换资料目录：${picked}`);
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "切换资料目录失败");
    }
  };

  const handleOpenFolder = async () => {
    try {
      await openLibraryFolder();
      onNotice("已打开资料文件夹，把文件放进去后点「重新扫描」。");
    } catch (err: unknown) {
      onNotice(err instanceof Error ? err.message : "打开文件夹失败");
    }
  };

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="page-title">本地资料</h1>
          <p className="page-subtitle">选择任意文件夹作为资料库，按大类浏览并打开原文件</p>
        </div>
        <div className="row" style={{ gap: 8 }}>
          <button type="button" className="primary-btn" onClick={() => void handlePickFolder()}>
            选择资料文件夹
          </button>
          <button type="button" className="secondary-btn" onClick={() => void handleOpenFolder()}>
            打开当前目录
          </button>
          <button type="button" className="primary-btn" onClick={() => void load()} disabled={state === "loading"}>
            {state === "loading" ? "扫描中…" : "重新扫描"}
          </button>
        </div>
      </div>

      <div className="stats">
        <div className="card">
          <div className="caption">资料总数</div>
          <div className="stat-value">{scan?.total ?? 0} 份</div>
        </div>
        <div className="card">
          <div className="caption">已入知识库</div>
          <div className="stat-value">{indexedCount} 份</div>
        </div>
        <div className="card">
          <div className="caption">本周新增</div>
          <div className="stat-value">{scan?.thisWeek ?? 0} 份</div>
        </div>
        <div className="card">
          <div className="caption">占用空间</div>
          <div className="stat-value">{scan?.sizeText ?? "0 B"}</div>
        </div>
      </div>

      {state === "error" && (
        <div className="card" style={{ marginTop: 16 }}>
          <div style={{ fontSize: 13, fontWeight: 600 }}>扫描失败</div>
          <p className="list-card-detail" style={{ marginTop: 6 }}>{error}</p>
          <p className="caption" style={{ marginTop: 6 }}>
            请确认本地服务已启动；资料目录不存在时会自动按空目录处理。
          </p>
        </div>
      )}

      {state !== "error" && scan && !scan.exists && (
        <div className="card" style={{ marginTop: 16 }}>
          <div style={{ fontSize: 13, fontWeight: 600 }}>还没有资料目录</div>
          <p className="caption" style={{ marginTop: 6 }}>
            点「打开资料文件夹」会自动创建并打开，把文件放进去后点「重新扫描」。
          </p>
          <p className="caption" style={{ marginTop: 4 }}>目录：{scan.root}</p>
        </div>
      )}

      {state !== "error" && scan?.exists && (
        <div className="library-layout">
          <div className="card" style={{ padding: 12 }}>
            <div className="row" style={{ justifyContent: "space-between", padding: "0 4px 8px" }}>
              <span className="section-title">资料分类</span>
              <span className="tag">{scan.groups} 大类</span>
            </div>
            <button
              type="button"
              className="tree-group"
              onClick={() => setGroup(ALL)}
              style={group === ALL ? { background: "var(--accent-light)", color: "var(--accent)" } : undefined}
            >
              <span>{ALL}</span>
              <span className="caption">{scan.total} 份</span>
            </button>
            {groups.map(([name, count]) => (
              <button
                type="button"
                className="tree-group"
                key={name}
                onClick={() => setGroup(name)}
                style={group === name ? { background: "var(--accent-light)", color: "var(--accent)" } : undefined}
              >
                <span>{name}</span>
                <span className="caption">{count} 份</span>
              </button>
            ))}
          </div>

          <div className="card" style={{ padding: 12 }}>
            <input
              className="input"
              placeholder="按文件名或路径搜索"
              value={keyword}
              onChange={(event) => setKeyword(event.target.value)}
              style={{ marginBottom: 10 }}
            />
            {filtered.length === 0 ? (
              <p className="caption" style={{ padding: "8px 4px" }}>
                {scan.total === 0 ? "资料目录是空的，放文件进去后重新扫描。" : "没有匹配的资料。"}
              </p>
            ) : (
              <div className="list">
                {filtered.slice(0, 200).map((item) => (
                  <button
                    type="button"
                    className="list-card"
                    key={item.path}
                    onClick={() => setSelectedPath(item.path)}
                    style={selected?.path === item.path ? { borderColor: "var(--accent)" } : undefined}
                  >
                    <div className="list-card-head">
                      <div>
                        <div className="list-card-title">{item.name}</div>
                        <p className="list-card-detail">
                          {item.group} · {item.type} · {item.sizeText} · {item.date}
                        </p>
                      </div>
                      <span className="tag">{item.type}</span>
                    </div>
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className="card" style={{ padding: 14 }}>
            {selected ? (
              <>
                <div className="doc-name">{selected.name}</div>
                <div className="doc-meta" style={{ marginTop: 8 }}>
                  <div>大类：{selected.group}</div>
                  <div>类型：{selected.type}</div>
                  <div>大小：{selected.sizeText}</div>
                  <div>修改时间：{selected.modTime}</div>
                  <div style={{ wordBreak: "break-all" }}>路径：{selected.relPath}</div>
                </div>
                <div className="row" style={{ marginTop: 12, gap: 8 }}>
                  <button type="button" className="primary-btn" onClick={() => void handleOpenFile(selected)}>
                    打开文件
                  </button>
                  {selectedKB?.status === "indexed" ? (
                    <button type="button" className="secondary-btn" onClick={() => void handleUnindex()} disabled={kbBusy}>
                      移出知识库
                    </button>
                  ) : (
                    <button type="button" className="secondary-btn" onClick={() => void handleIndex()} disabled={kbBusy}>
                      {kbBusy ? "处理中…" : "加入知识库"}
                    </button>
                  )}
                </div>
                <p className="caption" style={{ marginTop: 8 }}>
                  {selectedKB?.status === "indexed"
                    ? `已入知识库 · ${selectedKB.chunks} 个切片 · ${selectedKB.indexedAt}`
                    : selectedKB?.status === "unsupported"
                      ? `未入知识库：${selectedKB.error}`
                      : selectedKB?.status === "failed"
                        ? `上次索引失败：${selectedKB.error}`
                        : "未加入知识库。当前支持纯文本类文件（txt / md / csv / json 等）。"}
                </p>
              </>
            ) : (
              <p className="caption">左侧选择一份资料查看详情。</p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
