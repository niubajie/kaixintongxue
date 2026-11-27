-- 站点登录态：知乎 / CSDN / 小红书等站点要求登录后才放行内容。
-- 本地单用户库，Cookie 明文保存（不涉及多用户），仅本机可读。
ALTER TABLE search_sites ADD COLUMN cookie TEXT NOT NULL DEFAULT '';
