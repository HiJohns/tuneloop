// #2456: PC 后台 idle timeout 前端纯函数用例（node --test）。
//
// frontend-pc 没有测试运行器，因此把可判定逻辑收敛到 utils/pcSession.js，
// 用 Node 内置测试运行器覆盖（Accepted Plan R8 / D7）。
import test from 'node:test'
import assert from 'node:assert/strict'

import {
  PC_ACTIVITY_THROTTLE_MS,
  PC_IDLE_TIMEOUT_CODE,
  isIdleTimeoutPayload,
  shouldSendActivity,
} from '../src/utils/pcSession.js'

const LAST_SENT_AT = 1_000_000

test('activity throttle window is 60s', () => {
  assert.equal(PC_ACTIVITY_THROTTLE_MS, 60000)
})

test('shouldSendActivity is false at 59s', () => {
  assert.equal(shouldSendActivity(LAST_SENT_AT + 59_000, LAST_SENT_AT), false)
})

test('shouldSendActivity is true at 60s', () => {
  assert.equal(shouldSendActivity(LAST_SENT_AT + 60_000, LAST_SENT_AT), true)
})

test('isIdleTimeoutPayload recognises code 40110', () => {
  assert.equal(PC_IDLE_TIMEOUT_CODE, 40110)
  assert.equal(isIdleTimeoutPayload({ code: 40110 }), true)
})

test('isIdleTimeoutPayload ignores other codes and null', () => {
  assert.equal(isIdleTimeoutPayload({ code: 40105 }), false)
  assert.equal(isIdleTimeoutPayload(null), false)
})
