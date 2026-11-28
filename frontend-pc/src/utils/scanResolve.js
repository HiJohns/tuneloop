// #2634：PC 库管工作台「扫码验收」弹窗的匹配口径（纯函数，便于 node --test）。
//
// 设计约束（D4）：
//   精确（order_no / instrument_sn，忽略大小写）优先 → 模糊包含兜底 → 无匹配；
//   候选上限 5（避免一次录入把整屏列表塞进弹窗）；
//   空录入 / 破损输入一律返回 none，不抛错。
const CANDIDATE_LIMIT = 5

function toList(orders) {
  return Array.isArray(orders) ? orders.filter((o) => o && typeof o === 'object') : []
}

function field(value) {
  return typeof value === 'string' ? value.trim() : ''
}

function byUpdatedAtDesc(a, b) {
  const lhs = field(a?.updated_at)
  const rhs = field(b?.updated_at)
  if (lhs === rhs) return 0
  return lhs < rhs ? 1 : -1
}

/**
 * 把「手动录入的订单号 / 乐器 SN」解析为候选订单。
 *
 * @param {Array} orders 服务端返回的订单列表（已按需筛选）
 * @param {string} rawCode 用户录入
 * @returns {{kind: 'exact'|'fuzzy'|'none', list: Array}}
 */
export function resolveScanCandidates(orders, rawCode) {
  const code = String(rawCode ?? '').trim()
  if (!code) return { kind: 'none', list: [] }

  const list = toList(orders)
  const target = code.toLowerCase()

  const exact = list.filter((order) => {
    const orderNo = field(order.order_no).toLowerCase()
    const sn = field(order.instrument_sn).toLowerCase()
    return orderNo === target || sn === target
  })
  if (exact.length > 0) {
    return { kind: 'exact', list: exact.sort(byUpdatedAtDesc).slice(0, CANDIDATE_LIMIT) }
  }

  const fuzzy = list.filter((order) => {
    const orderNo = field(order.order_no).toLowerCase()
    const sn = field(order.instrument_sn).toLowerCase()
    return (orderNo !== '' && orderNo.includes(target)) || (sn !== '' && sn.includes(target))
  })
  if (fuzzy.length > 0) {
    return { kind: 'fuzzy', list: fuzzy.sort(byUpdatedAtDesc).slice(0, CANDIDATE_LIMIT) }
  }

  return { kind: 'none', list: [] }
}

export const SCAN_CANDIDATE_LIMIT = CANDIDATE_LIMIT
