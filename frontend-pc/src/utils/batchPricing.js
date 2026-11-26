// #2514/#2519：批量设价的唯一实现（开关模型 + 请求 payload + 结果反馈）。
//
// 历史缺陷：两页各自拼 `{ids, price_type, amount, operator}` 打到不存在的
// `/instruments/batch/price`。#2514 收敛为共享构建器，但 UI 仍保留「价格类型 + 操作」
// 两个选择器——Owner 判定无意义（#2519 D2），改为**三项并列 + 每项一个开关**：
// 日租金 / 逾期日租金 / 押金；开关关闭的项不提交、不改动。
//
// 后端契约（#2519 Rev.5）：`PUT /instruments/batch-pricing`
// `{items:[{id, base_daily_rate?, overdue_daily_fee?, deposit?}]}`，金额单位元；
// 每项至少一个字段，否则该条进 `failed[]`（reason=no_fields）。金额为 0 只在
// 「押金」上合法（显式零押金），日租金/逾期日租金必须 > 0。
//
// 两页只允许调用本模块，禁止各自拼 payload 或维护自己的一份字段列表（D2）。

export const BATCH_PRICE_FIELDS = [
  { key: 'base_daily_rate', label: '日租金', unit: '元/天' },
  { key: 'overdue_daily_fee', label: '逾期日租金', unit: '元/天' },
  { key: 'deposit', label: '押金', unit: '元' },
]

// isNonNegativeIntegerLike 仅用于说明：金额允许小数，这里只排除 NaN/负值。
function toFiniteNumber(value) {
  if (value === '' || value === null || value === undefined) return null
  const numeric = Number(value)
  return Number.isFinite(numeric) ? numeric : null
}

// emptyBatchPricingValues() → 新表单初值（全部关闭）。
export function emptyBatchPricingValues() {
  const values = {}
  BATCH_PRICE_FIELDS.forEach((field) => {
    values[field.key] = { enabled: false, value: undefined }
  })
  return values
}

// buildBatchPricingPayload(ids, values) → { payload } | { error }
//
// values[key] = { enabled, value }；只把 enabled 为真的项写进 items[]。
export function buildBatchPricingPayload(ids, values = {}) {
  if (!Array.isArray(ids) || ids.length === 0) {
    return { error: '请先选择乐器' }
  }

  const fields = {}
  for (const field of BATCH_PRICE_FIELDS) {
    const entry = values?.[field.key]
    if (!entry || !entry.enabled) continue

    const numeric = toFiniteNumber(entry.value)
    if (numeric === null) {
      return { error: `${field.label}必须填写数字` }
    }
    // 押金 0 是显式零押金；日租金/逾期日租金必须 > 0（后端把它们当「未配置」）。
    if (field.key === 'deposit') {
      if (numeric < 0) return { error: `${field.label}不能为负数` }
    } else if (numeric <= 0) {
      return { error: `${field.label}必须大于 0` }
    }
    fields[field.key] = numeric
  }

  if (Object.keys(fields).length === 0) {
    return { error: '请至少开启一项' }
  }

  return {
    payload: {
      items: ids.map((id) => ({ id, ...fields })),
    },
  }
}

// batchPricingFeedback(res, requested) → { level, text }
//
// 优先读后端 `failed[]`（#2519 D3：单条失败必须上报），其次才用 updated 与请求条数比对。
export function batchPricingFeedback(res, requested) {
  const rawUpdated = Number(res?.data?.updated)
  const updated = Number.isFinite(rawUpdated) ? rawUpdated : 0
  const failed = Array.isArray(res?.data?.failed) ? res.data.failed : []
  const rawRequested = Number(requested)
  const requestedCount = Number.isFinite(rawRequested) && rawRequested > 0 ? rawRequested : 0

  if (failed.length > 0) {
    const reasons = {}
    failed.forEach((item) => {
      const reason = item?.reason || 'unknown'
      reasons[reason] = (reasons[reason] || 0) + 1
    })
    const detail = Object.entries(reasons)
      .map(([reason, count]) => `${reason}×${count}`)
      .join('，')
    return {
      level: 'warning',
      text: `已修改 ${updated} 个乐器，失败 ${failed.length} 个（${detail}）`,
    }
  }

  if (requestedCount > 0 && updated >= requestedCount) {
    return { level: 'success', text: `已修改 ${updated} 个乐器价格` }
  }

  return {
    level: 'warning',
    text: `已修改 ${updated}/${requestedCount} 个乐器，其余未更新`,
  }
}

// batchStatusFeedback(res) → { level, text }（批量上架/下架，#2519）
export function batchStatusFeedback(res) {
  const rawUpdated = Number(res?.data?.updated)
  const updated = Number.isFinite(rawUpdated) ? rawUpdated : 0
  const failed = Array.isArray(res?.data?.failed) ? res.data.failed : []
  if (failed.length > 0) {
    const reasons = {}
    failed.forEach((item) => {
      const reason = item?.reason || 'unknown'
      reasons[reason] = (reasons[reason] || 0) + 1
    })
    const detail = Object.entries(reasons)
      .map(([reason, count]) => `${reason}×${count}`)
      .join('，')
    return { level: 'warning', text: `已更新 ${updated} 个乐器状态，失败 ${failed.length} 个（${detail}）` }
  }
  return { level: 'success', text: `已更新 ${updated} 个乐器状态` }
}
