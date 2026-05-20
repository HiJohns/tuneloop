// MenuPermission: standardized menu visibility rules
// sys_perm bits (0-24) defined by BeaconIAM, see backend/middleware/permissions.go
// cus_perm codes registered by TuneLoop with IAM

const SysPermBits = {
  namespace_view: 0,
  namespace_list: 1,
  namespace_create: 2,
  namespace_update: 3,
  namespace_delete: 4,
  tenant_view: 5,
  tenant_list: 6,
  tenant_create: 7,
  tenant_update: 8,
  tenant_delete: 9,
  organization_view: 10,
  organization_list: 11,
  organization_create: 12,
  organization_update: 13,
  organization_delete: 14,
  user_view: 15,
  user_list: 16,
  user_create: 17,
  user_update: 18,
  user_delete: 19,
  role_view: 20,
  role_list: 21,
  role_create: 22,
  role_update: 23,
  role_delete: 24,
}

function checkPermission(perm, sysPerm, cusPerm, cusPermMapping) {
  if (!perm) return true
  const { sysPermBits, cusPermCodes, requireAll } = perm
  if ((!sysPermBits || sysPermBits.length === 0) && (!cusPermCodes || cusPermCodes.length === 0)) return true

  let sysMatch = false
  let cusMatch = false

  if (sysPermBits && sysPermBits.length > 0) {
    sysMatch = sysPermBits.some(bit => (sysPerm & (1 << bit)) !== 0)
  }

  if (cusPermCodes && cusPermCodes.length > 0) {
    cusMatch = cusPermCodes.some(code => {
      const bit = cusPermMapping[code]
      if (bit === undefined || bit < 0) return false
      return (cusPerm & (1 << bit)) !== 0
    })
  }

  if (requireAll) return sysMatch && cusMatch
  if (sysPermBits?.length && cusPermCodes?.length) return sysMatch || cusMatch
  if (sysPermBits?.length) return sysMatch
  return cusMatch
}

function isNamespaceAdmin(sysPerm, cusPerm) {
  return sysPerm > 0 && cusPerm === 0
}

function getNamespaceAdminMenuKeys() {
  return ['/', '/merchants', '/system/audit-logs']
}

export {
  SysPermBits,
  checkPermission,
  isNamespaceAdmin,
  getNamespaceAdminMenuKeys,
}
