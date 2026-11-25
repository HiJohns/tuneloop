// #2502: 移动端员工身份徽标的单一 owner。
//
// 口径（docs/spec/ui/ui.md §2.5a / docs/spec/api/api.md §2.4.1）：
//   员工上下文 → `商户名 · 网点名 · 职位`
//   任一组织段缺失 → 回退单项；两者都缺 → 只显示职位。
//
// 禁止在页面里各自拼 `filter(Boolean).join(...)`（#2455/#2502 漂移成因）。

export const ROLE_BADGE_LABELS = [
  ['site_admin', '管理员'],
  ['site_member', '员工'],
  ['repair_technician', '维修师傅'],
]

export function formatOrgIdentity(merchantName, siteName) {
  return [merchantName, siteName].filter(Boolean).join(' · ')
}

export function roleBadgeTextFromRoles(roles = [], isStaff = false) {
  if (!isStaff) return '顾客'
  const labels = ROLE_BADGE_LABELS.filter(([code]) => roles.includes(code)).map(([, label]) => label)
  return labels.length > 0 ? labels.join(' · ') : '员工'
}

export function identityBadgeText({ tenantName, siteName, roles = [], isStaff = false } = {}) {
  return [formatOrgIdentity(tenantName, siteName), roleBadgeTextFromRoles(roles, isStaff)].filter(Boolean).join(' · ')
}

// 徽标可见性（#2502 D6）：角色非空且非顾客/访客 —— 与 weapp `isStaff`、后端
// GetBusinessRole 一致。H5 的 `isStaffRole` 以 `oid`/`tid` 非空判定员工，遇到
// 遗留「USER + 组织声明」token 会把组织名当员工身份展示（#2502 R13），因此徽标
// 单独使用本谓词，不改动金刚区/BottomNav 的既有判定（D7）。
export function isStaffBadgeVisible(role) {
  return !!role && role !== 'USER' && role !== 'GUEST'
}
