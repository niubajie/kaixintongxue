import { useCallback, useEffect, useState } from "react";
import { api } from "../api";
import type { AdminUser, Role } from "../types";

interface Props {
  selfId: number;
  notify: (message: string) => void;
}

const ROLES: Role[] = ["Owner", "Admin", "Member"];

export function UsersPanel({ selfId, notify }: Props) {
  const [keyword, setKeyword] = useState("");
  const [role, setRole] = useState("");
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [items, setItems] = useState<AdminUser[]>([]);
  const [error, setError] = useState("");
  const size = 20;

  const load = useCallback(async () => {
    try {
      const data = await api.users({ keyword, role, page, size });
      setItems(data.items);
      setTotal(data.total);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "读取用户失败。");
    }
  }, [keyword, role, page]);

  useEffect(() => {
    void load();
  }, [load]);

  const pages = Math.max(1, Math.ceil(total / size));

  return (
    <section>
      <div className="toolbar">
        <input
          type="text"
          placeholder="搜索账号 / 昵称 / 手机号"
          style={{ width: 280 }}
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              setPage(1);
              void load();
            }
          }}
        />
        <select
          value={role}
          onChange={(event) => {
            setRole(event.target.value);
            setPage(1);
          }}
        >
          <option value="">全部角色</option>
          {ROLES.map((item) => (
            <option key={item} value={item}>
              {item}
            </option>
          ))}
        </select>
        <button
          type="button"
          className="btn ghost"
          onClick={() => {
            setPage(1);
            void load();
          }}
        >
          查询
        </button>
        <span className="mono">共 {total} 位用户</span>
      </div>

      {error && <div className="form-error">{error}</div>}

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>账号</th>
              <th>手机号</th>
              <th>角色</th>
              <th>状态</th>
              <th>注册时间</th>
              <th>最后登录</th>
              <th style={{ textAlign: "right" }}>操作</th>
            </tr>
          </thead>
          <tbody>
            {items.length === 0 ? (
              <tr>
                <td colSpan={7} className="empty">
                  没有匹配的用户。
                </td>
              </tr>
            ) : (
              items.map((user) => (
                <tr key={user.id}>
                  <td>
                    <strong>{user.account}</strong>
                    <div className="mono">{user.displayName}</div>
                  </td>
                  <td>{user.phoneText}</td>
                  <td>
                    <select
                      value={user.role}
                      disabled={user.id === selfId}
                      onChange={async (event) => {
                        try {
                          const data = await api.setRole(user.id, event.target.value as Role);
                          notify(data.message);
                        } catch (err) {
                          notify(err instanceof Error ? err.message : "调整失败。");
                        }
                        await load();
                      }}
                    >
                      {ROLES.map((item) => (
                        <option key={item} value={item}>
                          {item}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td>
                    {user.status === "active" ? (
                      <span className="badge ok">正常</span>
                    ) : (
                      <span className="badge danger">已停用</span>
                    )}
                  </td>
                  <td className="mono">{user.createdAt}</td>
                  <td className="mono">{user.lastLoginAt || "—"}</td>
                  <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>
                    <button
                      type="button"
                      className="btn ghost small"
                      onClick={async () => {
                        const next = user.status === "active" ? "disabled" : "active";
                        if (
                          next === "disabled" &&
                          !window.confirm(`停用 ${user.account} 后该账号立即退出登录，确认吗？`)
                        ) {
                          return;
                        }
                        try {
                          const data = await api.setStatus(user.id, next);
                          notify(data.message);
                        } catch (err) {
                          notify(err instanceof Error ? err.message : "操作失败。");
                        }
                        await load();
                      }}
                    >
                      {user.status === "active" ? "停用" : "启用"}
                    </button>
                    <button
                      type="button"
                      className="btn ghost small danger-text"
                      onClick={async () => {
                        if (!window.confirm(`删除用户 ${user.account}？该操作不可撤销。`)) {
                          return;
                        }
                        try {
                          const data = await api.deleteUser(user.id);
                          notify(data.message);
                        } catch (err) {
                          notify(err instanceof Error ? err.message : "删除失败。");
                        }
                        await load();
                      }}
                    >
                      删除
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="pager">
        <button type="button" className="btn ghost small" disabled={page <= 1} onClick={() => setPage(page - 1)}>
          上一页
        </button>
        <span>
          {page} / {pages}
        </span>
        <button
          type="button"
          className="btn ghost small"
          disabled={page >= pages}
          onClick={() => setPage(page + 1)}
        >
          下一页
        </button>
      </div>
    </section>
  );
}
