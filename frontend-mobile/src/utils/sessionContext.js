// #2111: 会话上下文漂移判定（纯函数——无平台依赖，便于 node 断言/复用）
// loginContext: 登录时记录的所选上下文 { type: 'customer'|'org', context: '<org_id>' }
// claims: 解析后的 access token JWT 载荷
export function isContextDrift(loginContext, claims) {
  if (!loginContext || !claims || typeof loginContext !== 'object') return false
  const claimedOrg = claims.oid || ''
  if (loginContext.type === 'customer') {
    // 顾客上下文：token 不应带 org，且角色应为 USER
    return !!claimedOrg || (!!claims.role && claims.role !== 'USER')
  }
  if (loginContext.type === 'org') {
    // 组织上下文：token 的 oid 必须与登录时选择的组织一致
    return claimedOrg !== (loginContext.context || '')
  }
  return false
}
