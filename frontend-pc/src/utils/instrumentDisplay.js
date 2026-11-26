// #2528: 库存监控 / 乐器列表的展示层单一取源 helper。
//
// 背景：两个列表页各自挑数据源，导致三类缺陷：
//   1. 图片读 `instruments.images` 旧 JSONB，忽略后端已算好的 `thumbnail`
//      （pre 生产 219 台里 170 台 images 为空）；
//   2. 「估值」列裸渲染 `base_daily_rate`（单位是**分**）→ 100 倍；
//   3. 占位图指向仓库中并不存在的静态资源 → 回退本身 404（二次空图）。
//
// 契约（与后端同源，见 docs/spec/api/api.md §5.4.0）：
//   - 图片：`thumbnail` → `poster` → `images[0]` → 内置 data-URI 占位；
//   - 日租：`resolved_daily_rate`（pricing-v2 生效值，分）> 0 优先，
//           回落 `base_daily_rate`（分）；两者皆无/≤0 → `-`；
//   - 全额押金：`deposit` 为分，`null` → `-`（不伪造 0），显式 0 → `¥0.00`；
//   - 差额押金：`deposit_reduced` 为分，`null`/缺失 → `未配置`，显式 0 → `¥0.00`
//     （#2537 D5 与详情页同文案）；
//   - 逾期租金：`overdue_daily_fee` 为分（列表新字段，与 pricing-v2 同链），
//     `null`/≤0 → `-`。

import { formatCents } from './money.js'

// 内置占位（不新增二进制资源，避免再引用不存在的静态文件）。
export const INSTRUMENT_IMAGE_PLACEHOLDER =
  'data:image/svg+xml;charset=utf-8,' +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="50" height="50">' +
      '<rect width="50" height="50" fill="#f2f2f2"/>' +
      '<text x="25" y="29" font-size="9" text-anchor="middle" fill="#b0b0b0">无图</text>' +
      '</svg>'
  )

// #2529: 通用别名（同一常量，不复制 data URI）。非乐器场景（身份证照片等）
// 复用它即可，避免再引用未入库的静态占位文件。
export const IMAGE_PLACEHOLDER = INSTRUMENT_IMAGE_PLACEHOLDER

function firstImageFromImages(images) {
  if (Array.isArray(images)) {
    return typeof images[0] === 'string' ? images[0] : ''
  }
  if (typeof images !== 'string' || images.trim() === '') {
    return ''
  }
  try {
    const parsed = JSON.parse(images)
    if (Array.isArray(parsed) && typeof parsed[0] === 'string') {
      return parsed[0]
    }
  } catch {
    // 非法 JSON → 交给占位兜底（不抛错，列表不能因单行脏数据整页失败）
    return ''
  }
  return ''
}

/** 图片取源：thumbnail → poster → images[0] → 内置占位。 */
export function instrumentImageSrc(record) {
  if (!record) return INSTRUMENT_IMAGE_PLACEHOLDER
  if (record.thumbnail) return record.thumbnail
  if (record.poster) return record.poster
  const fromImages = firstImageFromImages(record.images)
  return fromImages || INSTRUMENT_IMAGE_PLACEHOLDER
}

function positiveCents(value) {
  const cents = Number(value)
  return Number.isFinite(cents) && cents > 0 ? cents : null
}

/** 标准日租金（分 → 元）；无法解析返回 `-`。 */
export function formatDailyRate(record) {
  const cents =
    positiveCents(record?.resolved_daily_rate) ?? positiveCents(record?.base_daily_rate)
  return cents === null ? '-' : `¥${formatCents(cents)}`
}

/** 全额押金（分 → 元）；`null`/缺失返回 `-`，显式 0 显示 `¥0.00`（#2537 D5）。 */
export function formatDepositFull(record) {
  const raw = record?.deposit
  if (raw === null || raw === undefined || raw === '') return '-'
  const cents = Number(raw)
  if (!Number.isFinite(cents)) return '-'
  return `¥${formatCents(cents)}`
}

/** 差额押金（分 → 元）；`null`/缺失返回 `未配置`，显式 0 显示 `¥0.00`（#2537 D5）。 */
export function formatDepositReduced(record) {
  const raw = record?.deposit_reduced
  if (raw === null || raw === undefined || raw === '') return '未配置'
  const cents = Number(raw)
  if (!Number.isFinite(cents)) return '未配置'
  return `¥${formatCents(cents)}`
}

/** 逾期租金（分 → 元）；`null`/缺失/≤0 返回 `-`（#2537 D4）。 */
export function formatOverdueDailyFee(record) {
  const cents = positiveCents(record?.overdue_daily_fee)
  return cents === null ? '-' : `¥${formatCents(cents)}`
}
