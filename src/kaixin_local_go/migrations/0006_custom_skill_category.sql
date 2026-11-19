-- 自建技能归类：work / study / life 对应 L1 三大类
ALTER TABLE custom_skills ADD COLUMN category TEXT NOT NULL DEFAULT '';
