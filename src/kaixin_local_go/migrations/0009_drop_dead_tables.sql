-- 清理三模块下线后的死表（见 docs/17 X-10）
-- 技能部署与自建技能页面已下线，对应写入路径与查询均已删除，
-- 这里只新增迁移回收旧库中的残留表；历史迁移 0001 / 0003 / 0006 保持原样。
DROP TABLE IF EXISTS deploy_runs;
DROP TABLE IF EXISTS custom_skills;
