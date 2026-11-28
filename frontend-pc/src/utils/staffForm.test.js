import test from 'node:test'
import assert from 'node:assert/strict'
import { resolveStaffFormMode, staffFormRules, isDualIdentity, resolveInitialRole } from './staffForm.js'

// #2626 翻转（Round 1 audit 失败项 2）：本文件原以**单值 `role`** 作判据输入。
// 语义变更后判据只读多值集合 `roles[]`（镜像 IAM functional_roles[]），
// 输入随之改为集合；单值输入不再产生 technician/site 判定（见文末负例）。
test('technician hides position', () => {
  const mode = resolveStaffFormMode({ roles: ['repair_technician'] })
  assert.equal(mode, 'technician')
  assert.equal(staffFormRules(mode).showPosition, false)
})

test('direct employee has no site requirement', () => {
  const mode = resolveStaffFormMode({ is_direct_staff: true })
  assert.equal(mode, 'direct')
  assert.deepEqual(staffFormRules(mode), {
    showPosition: true, showSite: false, requireSite: false, showMerchantReadonly: true,
  })
})

test('site employee keeps site required', () => {
  const mode = resolveStaffFormMode({ is_site_staff: true, site_id: 's1' })
  assert.equal(mode, 'site')
  assert.equal(staffFormRules(mode).requireSite, true)
})

// #2483：翻转 #2429 R6 的既有断言——双归属按 site 优先（原断言为 direct wins）。
// 列表已显示「所属网点」，编辑页锁成直属会让网点调动不可操作。
test('dual identity resolves to site (site wins, #2483)', () => {
  const mode = resolveStaffFormMode({ is_direct_staff: true, is_site_staff: true, site_id: 's1' })
  assert.equal(mode, 'site')
  const rules = staffFormRules(mode)
  assert.equal(rules.showSite, true)
  assert.equal(rules.requireSite, true)
  assert.equal(rules.showMerchantReadonly, false)
})

test('technician keeps priority over site (#2429)', () => {
  const mode = resolveStaffFormMode({ is_technician: true, is_site_staff: true, site_id: 's1' })
  assert.equal(mode, 'technician')
})

test('isDualIdentity detects both domains only', () => {
  assert.equal(isDualIdentity({ is_direct_staff: true, is_site_staff: true }), true)
  assert.equal(isDualIdentity({ is_direct_staff: true, site_id: 's1' }), true)
  assert.equal(isDualIdentity({ is_direct_staff: true }), false)
  assert.equal(isDualIdentity({ is_site_staff: true, site_id: 's1' }), false)
  assert.equal(isDualIdentity(null), false)
})

// #2626 翻转（Round 1 audit 失败项 2）：原断言为「roles 为空时前端回退
// role/site_role/merchant_role」。该兜底已上移到后端读路径
// （`GetStaffUser.mirrorRolesForRead`），前端只消费 `roles[]`；
// 因此断言改为「后端把兜底值填进 roles[] 后，初值正确」，并保留
// 「无网点 + 网点角色 → merchant_staff」的既有语义（#2462 D6）。
// 旧期望：resolveInitialRole({ role: '', site_role: 'site_member' }) === 'site_member'
// 新期望：resolveInitialRole({ roles: ['site_member'] }) === 'site_member'
test('resolveInitialRole derives from roles[] set, no single-value fallback (#2626)', () => {
  assert.equal(resolveInitialRole({ roles: ['site_member'], is_site_staff: true, site_id: 's1' }), 'site_member')
  assert.equal(resolveInitialRole({ roles: ['merchant_staff'] }), 'merchant_staff')
  // #2462 D6 既有语义保持：无网点 + 网点角色 → 显示为商户直属员工
  assert.equal(resolveInitialRole({ roles: ['site_member'] }), 'merchant_staff')
  // 负例：单值 role 不再驱动初值（回退在后端，不在前端）
  assert.equal(resolveInitialRole({ role: 'site_member' }), '')
  assert.equal(resolveInitialRole({}), '')
})
