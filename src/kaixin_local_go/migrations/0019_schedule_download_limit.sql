-- 定时搜索：单轮下载总大小上限（字节）。0 表示不下载任何资料；默认 1T 保护已有任务的行为不变。
ALTER TABLE search_schedules ADD COLUMN download_limit_bytes INTEGER NOT NULL DEFAULT 1000000000000;
