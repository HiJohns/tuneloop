import test from 'node:test'
import assert from 'node:assert/strict'
import { resolveStaffFormMode, staffFormRules } from './staffForm.js'

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

test('dual identity resolves to direct (direct wins)', () => {
  const mode = resolveStaffFormMode({ is_direct_staff: true, is_site_staff: true, site_id: 's1' })
  assert.equal(mode, 'direct')
  assert.equal(staffFormRules(mode).showSite, false)
})
