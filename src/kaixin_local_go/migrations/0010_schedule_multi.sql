-- 定时任务支持多选：每周多个星期几、每月多个日期。
-- 空字符串表示全选（周一至周日 / 1-28 日）。
ALTER TABLE search_schedules ADD COLUMN weekdays TEXT NOT NULL DEFAULT '';
ALTER TABLE search_schedules ADD COLUMN days TEXT NOT NULL DEFAULT '';
