// #2000 S2: 金额单位转换共享函数。
//
// 契约（#1728）：数据库与应用层之间以**分**（整数）传输/存储，UI 输入与展示用**元**。
// 所有「分 ↔ 元」转换必须走本模块，禁止在页面内联 `/ 100` / `* 100`：
// 转换语义变化时只需改这里，调用点不动。
//
// 命名与 PC 端 frontend-pc/src/utils/money.js 保持一致（两端同语义）。

/** 分 → 元（number）。仅用于展示/边界转换，金额计算请保持整数分。 */
export function toYuan(cents) {
  return Number(cents || 0) / 100
}

/** 分 → 元字符串（两位小数，无货币符号）。用于模板：`¥${formatCents(x)}`。 */
export function formatCents(cents) {
  return toYuan(cents).toFixed(2)
}

/** 元 → 分（整数，四舍五入到分）。用于把表单输入的元提交给后端。 */
export function yuanToCents(yuan) {
  return Math.round(Number(yuan || 0) * 100)
}
