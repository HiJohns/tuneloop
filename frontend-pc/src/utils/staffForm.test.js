import test from 'node:test'
import assert from 'node:assert/strict'
import { resolveStaffFormMode, staffFormRules, isDualIdentity, resolveInitialRole } from './staffForm.js'

test('technician hides position', () => {
  const mode = resolveStaffFormMode({ role: 'repair_technician' })
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

test('resolveInitialRole falls back when users.role is empty (#2483 D4)', () => {
  assert.equal(resolveInitialRole({ role: '', site_role: 'site_member', is_site_staff: true, site_id: 's1' }), 'site_member')
  assert.equal(resolveInitialRole({ role: '', site_role: '', merchant_role: 'merchant_staff' }), 'merchant_staff')
  // #2462 D6 既有语义保持：无网点 + 网点角色 → 显示为商户直属员工
  assert.equal(resolveInitialRole({ role: 'site_member' }), 'merchant_staff')
  assert.equal(resolveInitialRole({}), '')
})
