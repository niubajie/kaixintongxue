import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { listSettings, updateSetting } from "../api/admin";
import type { Setting } from "../api/types";
import { ErrorState, Loading } from "../components/States";
import { toast } from "../components/Toast";

const LABEL: Record<string, string> = {
  "resource.default_expire_days": "资源默认有效期（天）",
  "resource.simhash_hanning_threshold": "弱去重汉明距离阈值",
  "resource.quality_threshold": "质量分检索阈值",
  "resource.review_sla_hours": "审核 SLA（小时）"
};

const HINT: Record<string, string> = {
  "resource.default_expire_days": "提交资源未指定有效期时使用，范围 7–365。",
  "resource.simhash_hanning_threshold": "越小越严格；调整后只影响新提交资源，范围 0–10。",
  "resource.quality_threshold": "低于该分数的资源在检索中降权（S3 生效），范围 0–100。",
  "resource.review_sla_hours": "待审核资源超过该时长需提醒，范围 1–168。"
};

function fmtTime(v: string): string {
  return new Date(v).toLocaleString("zh-CN", { hour12: false });
}

export default function SettingsPage() {
  const qc = useQueryClient();
  const query = useQuery({ queryKey: ["admin-settings"], queryFn: listSettings });
  const [drafts, setDrafts] = useState<Record<string, string>>({});

  const save = useMutation({
    mutationFn: (v: { key: string; value: number }) => updateSetting(v.key, v.value),
    onSuccess: (s) => {
      toast.success(`已保存：${LABEL[s.key] ?? s.key}`);
      setDrafts((prev) => {
        const next = { ...prev };
        delete next[s.key];
        return next;
      });
      void qc.invalidateQueries({ queryKey: ["admin-settings"] });
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "保存失败")
  });

  const submit = (s: Setting) => {
    const text = drafts[s.key] ?? String(s.value);
    const n = Number(text);
    if (!Number.isInteger(n)) {
      toast.error("请输入整数");
      return;
    }
    if (n === s.value) {
      toast.error("数值未变化");
      return;
    }
    save.mutate({ key: s.key, value: n });
  };

  const items = query.data?.items ?? [];

  return (
    <div className="page">
      <section className="panel">
        <div className="panel-head">
          <h2>系统配置</h2>
          <span className="muted">仅白名单配置项可改 · 每次保存写入审计日志</span>
        </div>

        {query.isLoading && <Loading />}
        {query.isError && (
          <ErrorState message={(query.error as Error).message} onRetry={() => void query.refetch()} />
        )}

        {items.length > 0 && (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>配置项</th>
                  <th>配置键</th>
                  <th className="num">当前值</th>
                  <th>新值</th>
                  <th>说明</th>
                  <th>最近修改</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {items.map((s) => {
                  const draft = drafts[s.key] ?? String(s.value);
                  const dirty = draft !== String(s.value);
                  return (
                    <tr key={s.key}>
                      <td className="cell-title">{LABEL[s.key] ?? s.key}</td>
                      <td>
                        <code>{s.key}</code>
                      </td>
                      <td className="num">{s.value}</td>
                      <td>
                        <input
                          className="input-sm"
                          type="number"
                          step={1}
                          value={draft}
                          onChange={(e) =>
                            setDrafts((prev) => ({ ...prev, [s.key]: e.target.value }))
                          }
                        />
                      </td>
                      <td className="muted">{HINT[s.key] ?? "—"}</td>
                      <td className="muted">
                        {fmtTime(s.updatedAt)}
                        <br />
                        {s.updatedBy ? `由 ${s.updatedBy} 修改` : ""}
                      </td>
                      <td>
                        <button
                          className="btn btn-sm btn-primary"
                          disabled={!dirty || save.isPending}
                          onClick={() => submit(s)}
                        >
                          保存
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}

        <p className="muted">
          配置读取自 circle.settings；修改会立即写入数据库。涉及金额与计费的参数不在本页，避免误改。
        </p>
      </section>
    </div>
  );
}
