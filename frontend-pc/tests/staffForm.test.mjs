// #2626 R4：staffForm 集合判据（多值 roles[] + 组织级别），不读 users.role 单值。
import assert from 'node:assert/strict'
import test from 'node:test'

import { primaryRole, resolveInitialRole, resolveStaffFormMode, roleSet } from '../src/utils/staffForm.js'

test('PC_USES_ROLES_SET: 多值集合归一', () => {
  assert.deepEqual(roleSet({ roles: ['Site_Member', ' repair_technician '] }), ['site_member', 'repair_technician'])
  assert.deepEqual(roleSet({ roles: null }), [])
  assert.deepEqual(roleSet({}), [])
})

test('PRIMARY_PRIORITY: 多角色按派生优先级取主角色', () => {
  assert.equal(primaryRole({ roles: ['site_member', 'repair_technician'] }), 'repair_technician')
  assert.equal(primaryRole({ roles: ['site_member', 'site_admin'] }), 'site_admin')
  assert.equal(primaryRole({ roles: ['namespace_admin', 'merchant_admin'] }), 'namespace_admin')
  assert.equal(primaryRole({ roles: [] }), '')
})

test('FORM_MODE: 维修师傅 / 网点 / 直属 三态来自集合与归属', () => {
  assert.equal(resolveStaffFormMode({ roles: ['repair_technician'] }), 'technician')
  assert.equal(resolveStaffFormMode({ roles: ['site_member'], site_id: 's1' }), 'site')
  assert.equal(resolveStaffFormMode({ roles: ['merchant_staff'] }), 'direct')
  assert.equal(resolveStaffFormMode(null), 'direct')
})

test('INITIAL_ROLE: 无站点归属时网点级角色修正为 merchant_staff', () => {
  assert.equal(resolveInitialRole({ roles: ['site_member'] }), 'merchant_staff')
  assert.equal(resolveInitialRole({ roles: ['site_member'], site_id: 's1' }), 'site_member')
  assert.equal(resolveInitialRole({ roles: ['site_admin', 'repair_technician'], site_id: 's1' }), 'site_admin')
  assert.equal(resolveInitialRole({}), '')
})
