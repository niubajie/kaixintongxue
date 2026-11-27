-- 待审核资料记录来源任务：定时任务产生时写入 schedule_id，手动搜集留空
ALTER TABLE pending_docs ADD COLUMN schedule_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_pending_docs_schedule ON pending_docs (schedule_id);
