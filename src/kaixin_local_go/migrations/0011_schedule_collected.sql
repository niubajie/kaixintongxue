-- 定时搜索：累计搜集到的资料数量，用于任务卡展示进度
ALTER TABLE search_schedules ADD COLUMN collected INTEGER NOT NULL DEFAULT 0;
