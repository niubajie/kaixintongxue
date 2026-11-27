-- 定时搜索：资料选项（电子书 / 文档 / 视频 / 杂志 / 留言 / 全部），逗号分隔
ALTER TABLE search_schedules ADD COLUMN doc_types TEXT NOT NULL DEFAULT '';
