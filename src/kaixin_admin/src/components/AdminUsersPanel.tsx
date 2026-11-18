import { useCallback, useEffect, useState } from "react";
import { api } from "../api";
import type { AdminUser, Role } from "../types";

interface Props {
  selfId: number;
  notify: (message: string) => void;
}

// 管理员管理（FR-CLD-02 第二行）：
// 单独一页管 Owner/Admin，避免和「用户管理」混在一起找不到。
// 规则：只有 Owner 能指派 Admin；Admin 只能管 Member，不能动其他 Admin 与 Owner；
// 任何人都不能删除/停用自己。
export function AdminUsersPanel({ selfId, notify }: Props) {
  const [items, setItems] = useState<AdminUser[]>([]);
  const [members, setMembers] = useState<AdminUser[]>([]);
  const [keyword, setKeyword] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      const data = await api.users({ keyword: "", role: "", page: 1, size: 100 });
      const all = data.items;
      setItems(all.filter((user) => user.role === "Owner" || user.role === "Admin"));
      setMembers(all.filter((user) => user.role === "Member"));
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "读取管理员失败。");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const changeRole = async (user: AdminUser, role: Role) => {
    try {
      const result = await api.setRole(user.id, role);
      notify(result.message);
      await load();
    } catch (err) {
      notify(err instanceof Error ? err.message : "调整角色失败。");
    }
  };

  const toggleStatus = async (user: AdminUser) => {
    const next = user.status === "active" ? "disabled" : "active";
    if (next === "disabled" && !window.confirm(`停用 ${user.account} 后该账号立即退出登录，确认吗？`)) {
      return;
    }
    try {
      const result = await api.setStatus(user.id, next);
      notify(result.message);
      await load();
    } catch (err) {
      notify(err instanceof Error ? err.message : "操作失败。");
    }
  };

  const remove = async (user: AdminUser) => {
    if (!window.confirm(`删除 ${user.account}？该操作不可撤销。`)) {
      return;
    }
    try {
      const result = await api.deleteUser(user.id);
      notify(result.message);
      await load();
    } catch (err) {
      notify(err instanceof Error ? err.message : "删除失败。");
    }
  };

  const promote = async (account: string) => {
    const target = members.find((user) => user.account === account);
    if (!target) {
      notify("请选择要提升的成员。");
      return;
    }
    await changeRole(target, "Admin");
  };

  const filteredMembers = members.filter((user) =>
    keyword.trim() ? user.account.includes(keyword.trim()) || user.displayName.includes(keyword.trim()) : true
  );

  return (
    <section>
      <p className="hint">
        只有拥有者能指派管理员；管理员之间不能互相调整，也不能操作拥有者。
        所有操作都会写入审计日志。
      </p>

      {error && <p className="error">{error}</p>}

      <h3>当前管理员</h3>
      <table className="table">
        <thead>
          <tr>
            <th>账号</th>
            <th>昵称</th>
            <th>角色</th>
            <th>状态</th>
            <th>最后登录</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          {items.map((user) => (
            <tr key={user.id}>
              <td>
                {user.account}
                {user.id === selfId && <span className="badge">自己</span>}
              </td>
              <td>{user.displayName}</td>
              <td>
                <select
                  value={user.role}
                  disabled={user.id === selfId}
                  onChange={(event) => void changeRole(user, event.target.value as Role)}
                >
                  <option value="Owner">拥有者</option>
                  <option value="Admin">管理员</option>
                  <option value="Member">成员</option>
                </select>
              </td>
              <td>
                {user.status === "active" ? (
                  <span className="badge ok">正常</span>
                ) : (
                  <span className="badge danger">已停用</span>
                )}
              </td>
              <td>{user.lastLoginAt ?? "—"}</td>
              <td className="row-actions">
                <button onClick={() => void toggleStatus(user)} disabled={user.id === selfId}>
                  {user.status === "active" ? "停用" : "启用"}
                </button>
                <button className="danger" onClick={() => void remove(user)} disabled={user.id === selfId}>
                  删除
                </button>
              </td>
            </tr>
          ))}
          {items.length === 0 && (
            <tr>
              <td colSpan={6}>还没有管理员账号。</td>
            </tr>
          )}
        </tbody>
      </table>

      <h3>提升成员为管理员</h3>
      <div className="toolbar">
        <input
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
          placeholder="搜索账号或昵称"
        />
      </div>
      <table className="table">
        <thead>
          <tr>
            <th>账号</th>
            <th>昵称</th>
            <th>状态</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          {filteredMembers.map((user) => (
            <tr key={user.id}>
              <td>{user.account}</td>
              <td>{user.displayName}</td>
              <td>{user.status === "active" ? "正常" : "已停用"}</td>
              <td className="row-actions">
                <button onClick={() => void promote(user.account)}>设为管理员</button>
              </td>
            </tr>
          ))}
          {filteredMembers.length === 0 && (
            <tr>
              <td colSpan={4}>没有匹配的成员。</td>
            </tr>
          )}
        </tbody>
      </table>
    </section>
  );
}
