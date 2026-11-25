// #2514：批量设价的唯一实现（UI 选项 + 请求 payload + 结果反馈）。
//
// 历史缺陷：库存监控页与乐器列表页各自拼 `{ids, price_type, amount, operator}`
// 打到 `PUT /instruments/batch/price`——该路径不存在（后端只有
// `PUT /instruments/batch-pricing`），且 payload 与后端契约
// `{items:[{id, base_daily_rate(元), overrides?}]}` 完全不同（即使路径修对也 400）。
//
// 因此这里把「后端真正支持的语义」收敛成单一来源：选项、payload 映射、部分失败
// 提示都从这里出。两页只允许调用本模块，禁止各自拼 payload 或维护自己的一份选项
// 列表（D2）。后端当前只消费 `base_daily_rate`（日租金，单位：元），周/月租、
// 押金以及 increase/decrease/multiply 语义未实现（见 #2519），故 UI 只暴露
// 「日租金 + 设置为」。

export const BATCH_PRICE_TYPE_OPTIONS = [{ value: 'daily_rate', label: '日租金' }]

export const BATCH_PRICE_OPERATOR_OPTIONS = [{ value: 'set', label: '设置为' }]

const SUPPORTED_PRICE_TYPE = 'daily_rate'

// buildBatchPricingPayload(ids, { price_type, amount }) → { payload } | { error }
//
// 金额单位是「元」（后端 ToCentsPtr 再 ×100），且后端对 `<= 0` 会静默跳过，
// 所以必须先在前端拦截，否则会出现「提示成功但没改」。
export function buildBatchPricingPayload(ids, values = {}) {
  if (!Array.isArray(ids) || ids.length === 0) {
    return { error: '请先选择乐器' }
  }

  if (values.price_type !== SUPPORTED_PRICE_TYPE) {
    return { error: '批量设价当前仅支持「日租金」+「设置为」' }
  }

  const amount = values.amount
  if (amount === '' || amount === null || amount === undefined) {
    return { error: '金额必须大于 0' }
  }

  const numericAmount = Number(amount)
  if (!Number.isFinite(numericAmount) || numericAmount <= 0) {
    return { error: '金额必须大于 0' }
  }

  return {
    payload: {
      items: ids.map((id) => ({ id, base_daily_rate: numericAmount })),
    },
  }
}

// batchPricingFeedback(res, requested) → { level, text }
//
// 后端逐条失败只记服务端日志、仍返回 20000，只能用 data.updated 与请求条数比对；
// 少于请求条数时必须给 warning，不能报「全部成功」。
export function batchPricingFeedback(res, requested) {
  const rawUpdated = Number(res?.data?.updated)
  const updated = Number.isFinite(rawUpdated) ? rawUpdated : 0
  const rawRequested = Number(requested)
  const requestedCount = Number.isFinite(rawRequested) && rawRequested > 0 ? rawRequested : 0

  if (requestedCount > 0 && updated >= requestedCount) {
    return { level: 'success', text: `已修改 ${updated} 个乐器价格` }
  }

  return {
    level: 'warning',
    text: `已修改 ${updated}/${requestedCount} 个乐器，其余未更新（详见后端日志）`,
  }
}
