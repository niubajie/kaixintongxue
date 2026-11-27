-- 历史待审核资料补齐来源任务：按关键词匹配同名定时任务，任务卡点开即可看到旧资料
UPDATE pending_docs
SET schedule_id = (
  SELECT s.id FROM search_schedules s
  WHERE s.keyword = pending_docs.keyword
  ORDER BY s.created_at DESC LIMIT 1
)
WHERE schedule_id = ''
  AND EXISTS (
    SELECT 1 FROM search_schedules s WHERE s.keyword = pending_docs.keyword
  );
