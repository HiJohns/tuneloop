import test from 'node:test'
import assert from 'node:assert/strict'

import {
  clampRentDays,
  rentDaysInputState,
  rentDaysCommitState,
  rentDaysStepState,
} from './rentDays.mjs'

const CHECKOUT = { min: 1, max: 730, fallback: 1 }
const RENEWAL = { min: 45, fallback: 45 }

test('输入期保留空串，同时 days 回退到最小值', () => {
  const state = rentDaysInputState('', CHECKOUT)
  assert.deepEqual(state, { text: '', days: 1 })
  assert.deepEqual(rentDaysInputState('', RENEWAL), { text: '', days: 45 })
})

test('清空后可连续输入 6 -> 60，文本不被覆盖', () => {
  let state = rentDaysInputState('', CHECKOUT)
  state = rentDaysInputState('6', CHECKOUT)
  assert.deepEqual(state, { text: '6', days: 6 })
  state = rentDaysInputState('60', CHECKOUT)
  assert.deepEqual(state, { text: '60', days: 60 })
  assert.deepEqual(rentDaysCommitState(state.text, CHECKOUT), { text: '60', days: 60 })
})

test('失焦空串归一化：Checkout=1，Renewal=minDays', () => {
  assert.deepEqual(rentDaysCommitState('', CHECKOUT), { text: '1', days: 1 })
  assert.deepEqual(rentDaysCommitState('', RENEWAL), { text: '45', days: 45 })
})

test('Checkout 上限 730：编辑期保留 999，失焦归一为 730', () => {
  assert.deepEqual(rentDaysInputState('999', CHECKOUT), { text: '999', days: 730 })
  assert.deepEqual(rentDaysCommitState('999', CHECKOUT), { text: '730', days: 730 })
  assert.deepEqual(rentDaysCommitState('730', CHECKOUT), { text: '730', days: 730 })
})

test('0、负数、非数字不会产生非法 days', () => {
  assert.equal(clampRentDays('0', CHECKOUT), 1)
  assert.equal(clampRentDays('-5', CHECKOUT), 1)
  assert.equal(clampRentDays('abc', CHECKOUT), 1)
  assert.equal(clampRentDays('', RENEWAL), 45)
})

test('+/- 步进同步文本与数字，并在边界封顶', () => {
  assert.deepEqual(rentDaysStepState(60, 1, CHECKOUT), { text: '61', days: 61 })
  assert.deepEqual(rentDaysStepState(60, -1, CHECKOUT), { text: '59', days: 59 })
  assert.deepEqual(rentDaysStepState(730, 1, CHECKOUT), { text: '730', days: 730 })
  assert.deepEqual(rentDaysStepState(1, -1, CHECKOUT), { text: '1', days: 1 })
})
