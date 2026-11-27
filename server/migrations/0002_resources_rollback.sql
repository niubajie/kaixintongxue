-- =====================================================================
-- 0002_resources_rollback.sql — 回滚 0002（生产先备份）
-- 注意：删除表不可恢复，仅在确认数据可丢弃时执行。
-- =====================================================================

SET search_path TO circle, public;

DROP TABLE IF EXISTS circle.resource_contacts;
DROP TABLE IF EXISTS circle.settings;
DROP TABLE IF EXISTS circle.resources;
