// #2429：员工编辑字段模式单一出口。
// 双身份（merchant_members + site_members）按 direct 优先（D3）：
// 隐藏网点必填、显示只读直属商户，site_memberships 仍由详情接口完整返回。
export function resolveStaffFormMode(user = {}) {
  if (!user) return 'direct'
  if (user.is_technician || user.role === 'repair_technician') return 'technician'
  if (user.is_direct_staff === true) return 'direct'
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
