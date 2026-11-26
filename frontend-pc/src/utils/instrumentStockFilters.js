// #2577: 库存监控页筛选语义的唯一 owner（纯函数，不依赖 DOM / React / window）。
//
// 口径（Accepted Plan #2577 Decision Lock）：
//   D1 筛选状态唯一来源 = URL query；写回参数名用 `status`（`stock_status` 仅兼容读）。
//   D2 分类筛选 = filter-options 扁平列表 + 服务端后代展开（不做层级树控件）。
//   D4 本页状态词表（未知值原样返回）。
//   D5 显式状态优先：所选状态 ∈ {lost,archived,sold} 时不再追加 exclude_status。
//   D6 page/pageSize 仍为本地 state；筛选变化必须回第 1 页。
//
// 页面（InstrumentStock.jsx）只做接线，语义分支全部在此模块，便于 `node --test` 覆盖。

// 非管理级不可见的状态（服务端 exclude_status，见 #2140）。
export const HIDDEN_STOCK_STATUSES = ['lost', 'archived', 'sold']

// D4：本页状态词表。未登记的值原样展示，避免把未知状态吞成空白。
const STOCK_STATUS_LABELS = {
  available: '待租',
  rented: '在租',
  maintenance: '维修中',
  reserved: '已预订',
  shipping: '配送中',
  lost: '已丢失',
  archived: '已下架',
  sold: '已售出',
}

function readParam(source, key) {
  if (!source) return ''
  if (typeof source.get === 'function') {
    const value = source.get(key)
    return value == null ? '' : String(value).trim()
  }
  const value = source[key]
  return value == null ? '' : String(value).trim()
}

// 从 URL query 读取筛选条件（`status` 优先，`stock_status` 兼容读）。
export function readStockFilters(searchParams) {
  const status = readParam(searchParams, 'status') || readParam(searchParams, 'stock_status')
  return {
    sn: readParam(searchParams, 'sn'),
    categoryId: readParam(searchParams, 'category_id'),
    status,
  }
}

// 生成 `GET /api/instruments` 的列表请求参数（页面唯一取数入口）。
export function buildStockListParams({ filters, page, pageSize, isManagerLike } = {}) {
  const current = filters || {}
  const params = { page, pageSize }
  if (current.sn) params.sn = current.sn
  if (current.categoryId) params.category_id = current.categoryId
  if (current.status) params.stock_status = current.status
  // D5：显式选择 lost/archived/sold 时不叠加排除，否则恒为 0 行。
  const explicitHidden = HIDDEN_STOCK_STATUSES.includes(current.status)
  if (!isManagerLike && !explicitHidden) {
    params.exclude_status = HIDDEN_STOCK_STATUSES.join(',')
  }
  return params
}

// 生成写回 URL 的 query 对象（参数名 `status`，空值剔除）。
export function buildStockFilterQuery(filters) {
  const current = filters || {}
  const query = {}
  if (current.status) query.status = current.status
  if (current.sn) query.sn = current.sn
  if (current.categoryId) query.category_id = current.categoryId
  return query
}

// 筛选变化的唯一状态迁移：合并 patch → 回第 1 页 → 清空行选择。
// 行选择必须清空：批量设价/删除只作用于当前可见选中行，跨筛选残留会误伤。
export function applyFilterPatch(state, patch) {
  const current = state || {}
  const merged = { ...(current.filters || {}) }
  const incoming = patch || {}
  Object.keys(incoming).forEach((key) => {
    const value = incoming[key]
    merged[key] = value == null ? '' : String(value).trim()
  })
  return {
    filters: merged,
    page: 1,
    selectedRowKeys: [],
  }
}

// D4：状态码 → 中文词。
export function stockStatusLabel(value) {
  if (!value) return ''
  return STOCK_STATUS_LABELS[value] || value
}

// 状态下拉选项：非管理级剔除 lost/archived/sold；
// `current`（URL 带入）即使不在集合中也保留，避免「下拉空白但列表已过滤」。
export function stockStatusOptions({ statuses, isManagerLike, current } = {}) {
  const list = Array.isArray(statuses) ? statuses : []
  const seen = new Set()
  const options = []
  const push = (value) => {
    const key = value == null ? '' : String(value)
    if (!key || seen.has(key)) return
    if (!isManagerLike && HIDDEN_STOCK_STATUSES.includes(key)) return
    seen.add(key)
    options.push({ value: key, label: stockStatusLabel(key) })
  }
  list.forEach((entry) => {
    push(entry && typeof entry === 'object' ? entry.value : entry)
  })
  push(current)
  return options
}
