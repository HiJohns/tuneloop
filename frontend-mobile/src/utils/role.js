import { parseJWT } from '../platform/init'
import { getToken } from './auth'

// #2050 维修区角色互斥 — 统一角色判定口径。
//
// 与后端 middleware.GetBusinessRole 对齐（#1700）：
//   role 为空或 'USER' → 顾客；其余（STAFF/WORKER/ADMIN/...）→ 员工。
// 注意：oid/tid 不参与判定——「oid/tid 非空的顾客」仍是顾客
// （见 pages-weapp/Profile.jsx #1639/#1700 结论、middleware/iam.go:626）。
export function isStaffRole(token = getToken()) {
  const claims = parseJWT(token)
  return !!claims?.role && claims.role !== 'USER'
}

export function isCustomerRole(token = getToken()) {
  return !isStaffRole(token)
}
