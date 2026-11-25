-- 外网搜集的网站限定：用户可编辑、可新增，默认给出常用站点
CREATE TABLE IF NOT EXISTS search_sites (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  domain     TEXT NOT NULL,
  enabled    INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL
);

INSERT OR IGNORE INTO search_sites (id, name, domain, enabled, created_at) VALUES
  ('site-zhihu',      '知乎',      'zhihu.com',        1, '2026-11-25T00:00:00Z'),
  ('site-bilibili',   '哔哩哔哩',  'bilibili.com',     1, '2026-11-25T00:00:01Z'),
  ('site-xiaohongshu','小红书',    'xiaohongshu.com',  1, '2026-11-25T00:00:02Z'),
  ('site-douyin',     '抖音',      'douyin.com',       1, '2026-11-25T00:00:03Z'),
  ('site-weixin',     '微信公众号','mp.weixin.qq.com', 1, '2026-11-25T00:00:04Z'),
  ('site-csdn',       'CSDN',      'blog.csdn.net',    1, '2026-11-25T00:00:05Z');
