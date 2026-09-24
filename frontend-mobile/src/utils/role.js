import { parseJWT } from '../platform/init'
import { getToken } from './auth'

// #2050 维修区角色互斥 — 统一角色判定口径。
//
// 员工 = `oid`/`tid` 非空 **或** 员工角色（非 USER / 非 GUEST）；
// 顾客 = 其余（USER，无组织；不含匿名 GUEST）。
// 与后端 middleware.GetBusinessRole（#1700）一致：「oid/tid 非空的顾客」已废弃
// （不再建顾客组织），故 oid/tid 非空即员工；GUEST 为本地匿名访客，按顾客处理。
export function isStaffRole(token = getToken()) {
  const claims = parseJWT(token)
  if (!claims) return false
  const hasOrg = !!(claims.oid && claims.oid !== '')
  const hasTenant = !!(claims.tid && claims.tid !== '')
  const hasStaffRole = !!claims.role && claims.role !== 'USER' && claims.role !== 'GUEST'
  return hasOrg || hasTenant || hasStaffRole
}
