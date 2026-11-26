// #2429：员工编辑字段模式单一出口。
// #2483：优先级翻转为 technician → site → direct —— 「有 site_members 行 / site_id」
// 这一事实必须优先于「有 merchant_members 行」，否则双归属员工编辑页被锁成
// 只读「直属商户」，无法调动网点（列表却显示所属网点，口径矛盾）。
export function resolveStaffFormMode(user = {}) {
  if (!user) return 'direct'
  if (user.is_technician || user.role === 'repair_technician') return 'technician'
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

export function resolveInitialRole(user) {
  const role = user?.role || user?.site_role || user?.merchant_role || ''
  const hasSite = Boolean(user?.site_id)
  if (!hasSite && SITE_DOMAIN_ROLES.includes(role)) return 'merchant_staff'
  return role
}
