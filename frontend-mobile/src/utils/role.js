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

// #2083: 纯维修师傅（repair_technician 且无网点角色）——#1884 租赁 Tab 互斥、
// 维修工作台入口等按此统一判定。roles 来自 GET /site-members/me
//（v1.0.19 起 = 网点角色 ∪ JWT fn_roles，故直属商户师傅（无 site_members 行）也含 repair_technician）。
export function isPureTechnician(roles = []) {
  return roles.includes('repair_technician') && !roles.some(r => ['site_admin', 'site_member'].includes(r))
}
