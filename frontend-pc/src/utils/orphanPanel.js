// #2469: IAM 孤儿账号面板的展示判定（纯函数，供 node --test 直接覆盖）。
// 数据仅来自 GET /admin/iam-orphans（{ list, scanned, min_age_minute }），本模块不做请求。

const isFiniteNumber = (v) => typeof v === 'number' && Number.isFinite(v)

// 只有真的命中孤儿才需要警示（空列表不常驻红条）。
export function shouldShowWarning(list) {
  return Array.isArray(list) && list.length > 0
}

// 面板标题：空态保持固定标题，有数据时展示扫描量/命中量。
export function orphanTitle(list, scanned) {
  const hit = Array.isArray(list) ? list.length : 0
  if (hit === 0) return 'IAM 孤儿账号'
  if (!isFiniteNumber(scanned)) return `命中 ${hit} 个`
  return `扫描 ${scanned} 个，命中 ${hit} 个`
}

// 空态副文案：扫描数与挂起门槛缺失时降级，不抛异常、不编造数字。
export function orphanEmptyText(scanned, minAgeMinute) {
  const total = isFiniteNumber(scanned) ? `${scanned}` : '—'
  const base = `本次扫描 ${total} 个 pending 账号；孤儿判定：无本地记录 + 无组织`
  if (!isFiniteNumber(minAgeMinute)) return base
  return `${base} + 挂起 ≥ ${minAgeMinute} 分钟`
}

// 空态互斥：加载中或请求失败时不得显示「未发现孤儿账号」。
export function isEmptyState({ loading, error, list } = {}) {
  if (loading) return false
  if (error) return false
  if (!Array.isArray(list)) return true
  return list.length === 0
}
