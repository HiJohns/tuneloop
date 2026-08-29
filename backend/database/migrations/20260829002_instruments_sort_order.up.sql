-- #1797: 乐器子分类内排序 — instruments 新增 sort_order 列
-- 0 = 未排序（退化 created_at 序）；排序操作为同 category_id 组内交换 sort_order。
ALTER TABLE instruments ADD COLUMN IF NOT EXISTS sort_order INT DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_instruments_category_sort ON instruments (category_id, sort_order);
