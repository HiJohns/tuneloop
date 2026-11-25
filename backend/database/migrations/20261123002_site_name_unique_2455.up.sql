-- #2455: 同商户（tenant_id）网点名唯一（大小写不敏感，排除软删除）。
--
-- 语义：
--   1. 同一 tenant 下不允许两个同名网点（lower(name) 归一，避免「卡丹萨乐器/卡丹萨乐器 」
--      或大小写差异绕过）；不同 tenant 允许同名（跨商户同名是合法场景，UI 靠
--      「商户名 · 网点名」区分，见同一 Issue 的 R6–R8）。
--   2. 迁移遇存量同商户重复必须 fail-fast 并列出 tenant/name/ids，**不得静默跳过建索引**
--      （Decision Lock D3）。存量清理/改名由 ops 按只读清单执行，本迁移不改数据。
--   3. up 幂等：索引已存在时重复执行不报错；down 仅删索引，不动数据。
--
-- 说明：sites 体量小且本迁移在事务内（golang-migrate），故使用普通
-- CREATE UNIQUE INDEX（非 CONCURRENTLY，后者无法在事务内执行，见 D1）。
DO $$
DECLARE
    dup RECORD;
BEGIN
    FOR dup IN
        SELECT tenant_id,
               lower(name) AS lowered_name,
               count(*)    AS dup_count,
               string_agg(id::text, ',' ORDER BY id) AS ids
          FROM sites
         WHERE deleted_at IS NULL
         GROUP BY tenant_id, lower(name)
        HAVING count(*) > 1
    LOOP
        RAISE EXCEPTION
            'duplicate site name in tenant %: name=% count=% ids=%',
            dup.tenant_id, dup.lowered_name, dup.dup_count, dup.ids
            USING HINT = 'Owner must approve the rename list before this unique index can be created (#2455 D3).';
    END LOOP;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_sites_tenant_name
    ON sites (tenant_id, lower(name))
    WHERE deleted_at IS NULL;

COMMENT ON INDEX uq_sites_tenant_name IS
    '#2455: same-tenant site name is unique (case-insensitive, soft-deleted rows excluded)';
