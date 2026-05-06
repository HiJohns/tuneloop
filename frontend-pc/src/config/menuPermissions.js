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

/**
 * Menu visibility rules:
 * - sysPermBits: required sys_perm bits (OR within group)
 * - cusPermCodes: required cus_perm codes (OR within group)
 * - requireAllGroups: if true, BOTH sys_perm AND cus_perm groups must match
 *   (default false = either group matching is sufficient)
 */
const menuRules = [
  // Dashboard — always visible for logged-in users
  { path: '/', visibleWhen: {} },

  // Merchant management — now under system management menu (namespace admin, merchant admin)
  {
    path: '/merchants',
    visibleWhen: { sysPermBits: [SysPermBits.tenant_view] }
  },

  // Client management — needs namespace_view sys_perm
  {
    path: '/system/clients',
    visibleWhen: { sysPermBits: [SysPermBits.namespace_view] }
  },

  // Instrument management — needs any business cus_perm OR instrument:view
  {
    path: '/instruments/list',
    visibleWhen: { cusPermCodes: ['instrument:create', 'instrument:edit', 'instrument:delete', 'inventory:view', 'instrument:view'] }
  },
  {
    path: '/instruments/categories',
    visibleWhen: { cusPermCodes: ['category:manage'] }
  },
  {
    path: '/instruments/properties',
    visibleWhen: { cusPermCodes: ['property:manage'] }
  },

  // Inventory — needs inventory cus_perm
  {
    path: '/inventory/rent-setting',
    visibleWhen: { cusPermCodes: ['rent:setting'] }
  },
  {
    path: '/warehouse',
    visibleWhen: { cusPermCodes: ['inventory:view', 'inventory:manage'] }
  },

  // Maintenance — needs maintenance cus_perm
  {
    path: '/maintenance/workers',
    visibleWhen: { cusPermCodes: ['maintenance:assign'] }
  },
  {
    path: '/maintenance/sessions',
    visibleWhen: { cusPermCodes: ['maintenance:view', 'maintenance:assign', 'maintenance:complete'] }
  },

  // Organization — needs BOTH sys_perm AND cus_perm
  {
    path: '/organization/sites',
    visibleWhen: {
      sysPermBits: [SysPermBits.organization_view],
      cusPermCodes: ['instrument:create', 'inventory:view', 'maintenance:view'],
      requireAllGroups: true
    }
  },
  {
    path: '/staff',
    visibleWhen: {
      sysPermBits: [SysPermBits.user_view],
      cusPermCodes: ['instrument:create', 'inventory:view', 'maintenance:view'],
      requireAllGroups: true
    }
  },
  {
    path: '/staff/bulk-import',
    visibleWhen: {
      sysPermBits: [SysPermBits.user_create],
      cusPermCodes: ['instrument:create', 'inventory:view', 'maintenance:view'],
      requireAllGroups: true
    }
  },
  {
    path: '/organization/sites/bulk-import',
    visibleWhen: {
      sysPermBits: [SysPermBits.organization_create],
      cusPermCodes: ['instrument:create', 'inventory:view', 'maintenance:view'],
      requireAllGroups: true
    }
  },

  // System management — needs sys_perm
  {
    path: '/system/tenants',
    visibleWhen: { sysPermBits: [SysPermBits.tenant_list] }
  },
  {
    path: '/appeals',
    visibleWhen: { cusPermCodes: ['appeal:handle'] }
  },
]

/**
 * Check if a menu rule is satisfied by current user permissions.
 */
function checkRule(rule, sysPerm, cusPerm, cusPermMapping) {
  const { sysPermBits, cusPermCodes, requireAllGroups } = rule.visibleWhen

  // If no conditions, always visible
  if (!sysPermBits && !cusPermCodes) return true
  if ((!sysPermBits || sysPermBits.length === 0) && (!cusPermCodes || cusPermCodes.length === 0)) return true

  let sysMatch = false
  let cusMatch = false

  if (sysPermBits && sysPermBits.length > 0) {
    sysMatch = sysPermBits.some(bit => (sysPerm & (1 << bit)) !== 0)
  }

  if (cusPermCodes && cusPermCodes.length > 0) {
    cusMatch = cusPermCodes.some(code => {
      const bit = cusPermMapping[code]
      if (bit === undefined || bit < 0) return false // code not registered yet
      // For bits >= 64, check cus_perm_ext (not yet implemented)
      if (bit >= 64) return false
      return (cusPerm & (1 << bit)) !== 0
    })
  }

  // Grace period: cus_perm not yet initialized from IAM (cusPerm === 0)
  // but user has system permissions (sysPerm > 0, i.e. admin-like accounts).
  // Allow full menu access so they can bootstrap their own permissions.
  if (cusPerm === 0 && sysPerm > 0) {
    return true
  }

  if (requireAllGroups) {
    return sysMatch && cusMatch
  }
  return sysMatch || cusMatch
}

/**
 * Determine if user is a namespace admin (has sys_perm but no cus_perm).
 * Namespace admins can only see merchant + client management + dashboard.
 */
function isNamespaceAdmin(sysPerm, cusPerm) {
  return sysPerm > 0 && cusPerm === 0
}

/**
 * Get namespace admin visible menu keys.
 */
function getNamespaceAdminMenuKeys() {
  return ['/', '/merchants', '/system/clients']
}

export {
  SysPermBits,
  menuRules,
  checkRule,
  isNamespaceAdmin,
  getNamespaceAdminMenuKeys,
}
