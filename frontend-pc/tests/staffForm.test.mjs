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

// #2626 D1（Rev.3）：数据源 = IAM functional_roles 的本地镜像（后端读路径下发），
// 页面不得自造 roles，也不得回落单值。这三条同时是 ROLES_SOURCE=iam 的行为断言。
test('ROLES_SOURCE=iam: 只消费后端下发的 roles[] 镜像，页面不自造、不回退单值', () => {
  // 1) 镜像形态（后端 GetStaffUser 下发）→ 正常判定
  assert.deepEqual(roleSet({ roles: ['merchant_staff', 'repair_technician'] }), ['merchant_staff', 'repair_technician'])
  // 2) 单值来源（role / site_role / merchant_role）不参与集合构建 —— 兜底在后端而非页面
  assert.deepEqual(roleSet({ role: 'site_admin', site_role: 'site_admin', merchant_role: 'merchant_admin' }), [])
  assert.equal(primaryRole({ role: 'site_admin' }), '')
  // 3) 页面不写入 roles（roles 是只读镜像）
  const user = { roles: ['site_member'] }
  const snapshot = JSON.stringify(user)
  primaryRole(user)
  roleSet(user)
  resolveInitialRole(user)
  assert.equal(JSON.stringify(user), snapshot)
})
