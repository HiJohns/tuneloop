#!/usr/bin/env bash
# #2283 P5 历史数据修复脚本（默认 dry-run；apply 需显式 --apply --batch N）。
# 依赖：#2278（补缴入账）/#2280（total_spending 单位）上线后再 apply。
set -euo pipefail

MODE="dry-run"
BATCH=""
while [ $# -gt 0 ]; do
  case "$1" in
    --apply) MODE="apply" ;;
    --batch) BATCH="${2:-}"; shift ;;
    --batch=*) BATCH="${1#*=}" ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
  shift
done
if [ "$MODE" = "apply" ] && [ -z "$BATCH" ]; then
  echo "ERROR: --apply requires --batch 1|2" >&2; exit 2
fi

PSQL_CMD="${PSQL_CMD:-psql}"

run() { $PSQL_CMD -v ON_ERROR_STOP=1 -A -F'|' -P pager=off "$@"; }

batch1_dry() {
run <<'SQL'
WITH pay AS (
  SELECT order_id, COALESCE(SUM(CASE WHEN type='payment' AND status='paid' THEN amount ELSE 0 END),0) AS paid_sum,
         COALESCE(SUM(CASE WHEN type='payment' AND status='paid' AND order_type='payment_shortfall' THEN amount ELSE 0 END),0) AS shortfall_paid,
         COALESCE(SUM(CASE WHEN type='payment' AND status='paid' AND order_type='damage' THEN amount ELSE 0 END),0) AS damage_paid
  FROM order_payment_records GROUP BY order_id
), ref AS (
  SELECT pr.order_id, COALESCE(SUM(CASE WHEN rr.status='refunded' THEN rr.amount ELSE 0 END),0) AS refunded_sum
  FROM order_refund_records rr JOIN order_payment_records pr ON pr.id=rr.payment_record_id GROUP BY pr.order_id
)
SELECT o.order_no, o.cash_paid, (COALESCE(p.paid_sum,0)-COALESCE(f.refunded_sum,0)) AS ledger_cash,
       (COALESCE(p.paid_sum,0)-COALESCE(f.refunded_sum,0)-o.cash_paid) AS diff,
       CASE WHEN COALESCE(f.refunded_sum,0)>0 AND o.cash_paid > (COALESCE(p.paid_sum,0)-COALESCE(f.refunded_sum,0)) THEN 'refund_not_written'
            WHEN COALESCE(p.damage_paid,0)>0 THEN 'damage'
            WHEN COALESCE(p.shortfall_paid,0)>0 THEN 'shortfall'
            ELSE 'other' END AS classification
FROM orders o
LEFT JOIN pay p ON p.order_id=o.id
LEFT JOIN ref f ON f.order_id=o.id
WHERE (COALESCE(p.paid_sum,0)-COALESCE(f.refunded_sum,0)-o.cash_paid) <> 0
ORDER BY ABS(COALESCE(p.paid_sum,0)-COALESCE(f.refunded_sum,0)-o.cash_paid) DESC;
SQL
}

batch1_apply() {
run <<'SQL'
BEGIN;
UPDATE orders o SET cash_paid = t.ledger_cash, updated_at = NOW()
FROM (
  SELECT o2.id,
         (COALESCE(p.paid_sum,0)-COALESCE(f.refunded_sum,0)) AS ledger_cash,
         COALESCE(p.damage_paid,0) AS damage_paid,
         COALESCE(p.shortfall_paid,0) AS shortfall_paid,
         COALESCE(f.refunded_sum,0) AS refunded_sum,
         o2.cash_paid AS old_cash
  FROM orders o2
  LEFT JOIN (SELECT order_id, COALESCE(SUM(CASE WHEN type='payment' AND status='paid' THEN amount ELSE 0 END),0) AS paid_sum,
                    COALESCE(SUM(CASE WHEN type='payment' AND status='paid' AND order_type='damage' THEN amount ELSE 0 END),0) AS damage_paid,
                    COALESCE(SUM(CASE WHEN type='payment' AND status='paid' AND order_type='payment_shortfall' THEN amount ELSE 0 END),0) AS shortfall_paid
             FROM order_payment_records GROUP BY order_id) p ON p.order_id=o2.id
  LEFT JOIN (SELECT pr.order_id, COALESCE(SUM(CASE WHEN rr.status='refunded' THEN rr.amount ELSE 0 END),0) AS refunded_sum
             FROM order_refund_records rr JOIN order_payment_records pr ON pr.id=rr.payment_record_id GROUP BY pr.order_id) f ON f.order_id=o2.id
  WHERE COALESCE(p.damage_paid,0)=0
) t
WHERE o.id=t.id AND o.cash_paid=t.old_cash AND o.cash_paid <> t.ledger_cash
  -- 安全阀：仅自动修复「退款未回写」「补缴未入账」；ledger=0（历史流水已清理）与租金覆盖类一律人工
  AND t.ledger_cash > 0
  AND ( (t.refunded_sum > 0 AND t.old_cash > t.ledger_cash) OR t.shortfall_paid > 0 );
COMMIT;
SQL
}

batch2_dry() {
run <<'SQL'
SELECT u.id, u.total_spending, COALESCE(SUM(s.actual_rent_amount),0) AS rent_sum
FROM users u
LEFT JOIN orders o ON o.user_id=u.id
LEFT JOIN settlements s ON s.order_id=o.id
WHERE u.total_spending > 0
GROUP BY u.id, u.total_spending
ORDER BY u.total_spending DESC;
SQL
}

case "${MODE}:${BATCH}" in
  dry-run:) batch1_dry; batch2_dry ;;
  dry-run:1) batch1_dry ;;
  dry-run:2) batch2_dry ;;
  apply:1) batch1_apply ;;
  apply:2) echo "Batch2 需 #2280 上线后再执行（脚本待补）" >&2; exit 3 ;;
  *) echo "unsupported mode/batch: $MODE/$BATCH" >&2; exit 2 ;;
esac
