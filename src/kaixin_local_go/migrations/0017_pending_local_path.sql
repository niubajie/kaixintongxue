-- 待审核资料下载到本地临时资料库后的文件路径；为空表示未下载
ALTER TABLE pending_docs ADD COLUMN local_path TEXT NOT NULL DEFAULT '';
