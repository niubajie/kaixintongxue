-- 定时搜索：本次任务勾选的网站（来自网站设置），逗号分隔；空表示全部启用网站
ALTER TABLE search_schedules ADD COLUMN site_ids TEXT NOT NULL DEFAULT '';
