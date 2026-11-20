import test from 'node:test'
import assert from 'node:assert/strict'
import { secondaryButtonStyle, primaryButtonStyle } from './buttonStyles.mjs'

test('secondary button is 36px with no native margin', () => {
  const s = secondaryButtonStyle()
  assert.equal(s.height, 36)
  assert.equal(s.margin, 0)
  assert.equal(s.display, 'flex')
})

test('primary button is 48px with no native margin', () => {
  const s = primaryButtonStyle()
  assert.equal(s.height, 48)
  assert.equal(s.margin, 0)
  assert.equal(s.display, 'flex')
})

test('danger theme uses red palette', () => {
  assert.equal(secondaryButtonStyle('danger').color, '#ef4444')
})
