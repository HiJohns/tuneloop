// #2000 S1: 金额单位转换共享函数（PC 端）。
//
// 契约（#1728）：数据库存**分**（整数），API 传输多为**分**（`models.Cents` MarshalJSON → int64），
// 表单输入/展示用**元**。所有「分 ↔ 元」转换必须走本模块，禁止页面内联 `/ 100` / `* 100`。
//
// 与移动端 frontend-mobile/src/utils/money.js 同语义。

/** 分 → 元（number）。用于表单预填、边界转换；金额计算请保持整数分。 */
export function toYuan(cents) {
  return Number(cents || 0) / 100
}

/** 分 → 元字符串（两位小数，无符号）。用于展示：`¥${formatCents(x)}`。 */
export function formatCents(cents) {
  return toYuan(cents).toFixed(2)
}

/** 元 → 分（整数，四舍五入）。用于提交给后端（后端若按元接收则不要调用）。 */
export function yuanToCents(yuan) {
  return Math.round(Number(yuan || 0) * 100)
}
