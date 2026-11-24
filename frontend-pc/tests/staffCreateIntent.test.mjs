// #2478: PC 直属成员管理「创建员工」一次性意图的前端纯函数用例（node --test）。
//
// frontend-pc 没有测试运行器（仅 dev/build/lint/preview），因此把可判定逻辑收敛到
// utils/staffCreateIntent.js，用 Node 内置测试运行器覆盖（Accepted Plan R1 / D4）。
import test from 'node:test'
import assert from 'node:assert/strict'

import {
  CREATE_INTENT_OPEN,
  createIntentState,
  isConsumableIntent,
  isCreateIntent,
} from '../src/utils/staffCreateIntent.js'

test('null / undefined state is not a create intent', () => {
  assert.equal(isCreateIntent(null), false)
  assert.equal(isCreateIntent(undefined), false)
})

test('empty object is not a create intent', () => {
  assert.equal(isCreateIntent({}), false)
})

test('openCreate:true is a create intent', () => {
  assert.equal(isCreateIntent({ [CREATE_INTENT_OPEN]: true }), true)
})

test('string "true" is not a create intent (strict boolean)', () => {
  assert.equal(isCreateIntent({ [CREATE_INTENT_OPEN]: 'true' }), false)
  assert.equal(isCreateIntent({ [CREATE_INTENT_OPEN]: 1 }), false)
})

test('createIntentState carries open flag and merchant_direct user type', () => {
  const state = createIntentState()
  assert.equal(state[CREATE_INTENT_OPEN], true)
  assert.equal(state.userType, 'merchant_direct')
})

test('consumed intent (null) is not consumable twice', () => {
  assert.equal(isConsumableIntent(null), false)
  assert.equal(isConsumableIntent(createIntentState()), true)
})
