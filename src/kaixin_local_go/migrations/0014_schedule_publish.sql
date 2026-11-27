-- 定时搜索：发布时间（不限留空），取值 1d / 3d / 1w / 1m / 3m
ALTER TABLE search_schedules ADD COLUMN publish_within TEXT NOT NULL DEFAULT '';
