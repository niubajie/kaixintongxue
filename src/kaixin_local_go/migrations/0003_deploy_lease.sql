-- 部署任务租约（v3）
-- 借鉴 Rakazo 的 run lease 思路：任务有租约与心跳，进程崩溃后靠租约过期回收，
-- 而不是让界面永远停在「进行中」。

ALTER TABLE deploy_runs ADD COLUMN attempt INTEGER NOT NULL DEFAULT 1;
ALTER TABLE deploy_runs ADD COLUMN heartbeat_at TEXT NOT NULL DEFAULT '';
ALTER TABLE deploy_runs ADD COLUMN lease_until TEXT NOT NULL DEFAULT '';
ALTER TABLE deploy_runs ADD COLUMN error TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_deploy_runs_state ON deploy_runs(state);
