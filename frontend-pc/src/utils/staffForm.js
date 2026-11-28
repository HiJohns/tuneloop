// #2429：员工编辑字段模式单一出口。
// #2483：优先级翻转为 technician → site → direct —— 「有 site_members 行 / site_id」
// 这一事实必须优先于「有 merchant_members 行」，否则双归属员工编辑页被锁成
// 只读「直属商户」，无法调动网点（列表却显示所属网点，口径矛盾）。
// #2626 D2/D4：判据只用多值集合 users.roles（镜像 IAM functional_roles[]），
// 不再以 users.role 单值作为判据来源（单值字段装不下多角色）。
const ROLE_PRIORITY = ['merchant_admin', 'site_admin', 'repair_technician', 'merchant_staff', 'site_member']
const PLATFORM_ROLES = ['namespace_admin', 'sys_admin']

export function roleSet(user = {}) {
  const roles = Array.isArray(user?.roles) ? user.roles : []
  return roles
    .filter((role) => typeof role === 'string' && role.trim() !== '')
    .map((role) => role.trim().toLowerCase())
}

// 派生主角色（与后端 services/role_derive.go 的 DeriveRoles 同序）。
export function primaryRole(user = {}) {
  const roles = roleSet(user)
  const platform = PLATFORM_ROLES.find((role) => roles.includes(role))
  if (platform) return platform
  const business = ROLE_PRIORITY.find((role) => roles.includes(role))
  return business || roles[0] || ''
}

export function resolveStaffFormMode(user = {}) {
  if (!user) return 'direct'
  if (user.is_technician || roleSet(user).includes('repair_technician')) return 'technician'
  if (user.is_site_staff === true || user.site_id) return 'site'
  return 'direct'
}

export function staffFormRules(mode) {
  return {
    showPosition: mode !== 'technician',
    showSite: mode === 'site',
    requireSite: mode === 'site',
    showMerchantReadonly: mode === 'direct',
  }
}

// #2483：双归属（merchant_members + site_members 并存）。仅用于编辑页的显式提示，
// 不参与字段模式判定（判定只看是否有网点归属）。
export function isDualIdentity(user = {}) {
  if (!user) return false
  return user.is_direct_staff === true && (user.is_site_staff === true || Boolean(user.site_id))
}

// #2462 D6 + #2483：编辑页角色初值的域判定与回退链。
// - 回退链：users.role → site_role → merchant_role（存量 users.role='' 时下拉不再为空）；
// - 直属员工（无网点归属）的历史值可能被写成网点角色（存量脏数据），
//   直接预填会让「保存」把它再次持久化成网点角色 → 仅修正显示/初始值：
//   无 site_id 且 role ∈ {site_member, site_admin} → merchant_staff；
// - 只影响表单初值，不回写后端、不改列表返回。
const SITE_DOMAIN_ROLES = ['site_member', 'site_admin']

export function resolveInitialRole(user = {}) {
  // #2626：初值取多值集合的派生主角色；无站点归属时的网点级角色修正保持原语义。
  const role = primaryRole(user)
  const hasSite = Boolean(user?.site_id)
  if (!hasSite && SITE_DOMAIN_ROLES.includes(role)) return 'merchant_staff'
  return role
}
