// ui.md §1.6 日期与时间显示规范（强制）：北京时间（UTC+8）+ 中文格式，
// 禁止 MM-DD / YYYY-MM-DD 等连字符格式出现在用户可见展示。
// 当年省略年份；跨年保留年份。「当年」判定同样基于北京时间。
// 与 frontend-mobile/src/utils/format.js 的同名 helper 保持语义一致。

function beijingParts(dateStr) {
  if (!dateStr) return null
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return null
  const bj = new Date(d.getTime() + 8 * 3600000)
  return {
    y: bj.getUTCFullYear(),
    m: bj.getUTCMonth() + 1,
    d: bj.getUTCDate(),
    hh: String(bj.getUTCHours()).padStart(2, '0'),
    mi: String(bj.getUTCMinutes()).padStart(2, '0'),
  }
}

function beijingNowYear() {
  return new Date(Date.now() + 8 * 3600000).getUTCFullYear()
}

// formatBeijingDate — 仅日期展示：当年 "M月D日"，跨年 "YYYY年M月D日"。
export function formatBeijingDate(dateStr) {
  if (!dateStr) return '-'
  const p = beijingParts(dateStr)
  if (!p) return dateStr
  return p.y === beijingNowYear() ? `${p.m}月${p.d}日` : `${p.y}年${p.m}月${p.d}日`
}

// formatBeijingDateTimeShort — 日期+时间展示：当年 "M月D日 HH:mm"，跨年 "YYYY年M月D日 HH:mm"。
export function formatBeijingDateTimeShort(dateStr) {
  if (!dateStr) return '-'
  const p = beijingParts(dateStr)
  if (!p) return dateStr
  const datePart = p.y === beijingNowYear() ? `${p.m}月${p.d}日` : `${p.y}年${p.m}月${p.d}日`
  return `${datePart} ${p.hh}:${p.mi}`
}
