// #2527 R4：resetPasswordSummary 纯函数用例（node --test，见 frontend-pc/package.json 无 test runner）。

import test from 'node:test'
import assert from 'node:assert/strict'

import { resetPasswordSummary } from './resetPasswordSummary.js'

test('all sent → success', () => {
  const summary = resetPasswordSummary({ sent: 3, skipped: 0 })
  assert.equal(summary.level, 'success')
  assert.equal(summary.sent, 3)
  assert.equal(summary.skipped, 0)
  assert.match(summary.text, /已发送重设密码邮件（3 个用户）/)
})

test('mixed batch → warning lists every skip reason', () => {
  const summary = resetPasswordSummary({
    sent: 1,
    skipped: 0,
    skipped_invisible: ['a'],
    skipped_missing: ['b', 'c'],
    skipped_no_email: ['d'],
  })
  assert.equal(summary.level, 'warning')
  assert.equal(summary.sent, 1)
  assert.equal(summary.skipped, 4)
  assert.equal(summary.reasons.length, 3)
  assert.match(summary.text, /跳过 4 个/)
  assert.match(summary.text, /1 个账号不在可管理范围/)
  assert.match(summary.text, /2 个账号不存在或未同步 IAM 身份/)
  assert.match(summary.text, /1 个账号未配置邮箱/)
})

test('nothing sent (all invisible) → warning with the concrete reason', () => {
  const summary = resetPasswordSummary({ sent: 0, skipped: 0, skipped_invisible: ['a', 'b'] })
  assert.equal(summary.level, 'warning')
  assert.equal(summary.sent, 0)
  assert.equal(summary.skipped, 2)
  assert.match(summary.text, /未发送任何重设密码邮件/)
  assert.match(summary.text, /2 个账号不在可管理范围/)
})

test('legacy response without classification falls back to skipped count', () => {
  const summary = resetPasswordSummary({ sent: 2, skipped: 1 })
  assert.equal(summary.level, 'warning')
  assert.equal(summary.skipped, 1)
  assert.match(summary.text, /跳过 1 个/)
})

test('empty payload → warning, never a fake success', () => {
  const summary = resetPasswordSummary(undefined)
  assert.equal(summary.level, 'warning')
  assert.equal(summary.sent, 0)
  assert.match(summary.text, /未发送任何重设密码邮件：没有可处理的账号/)
})
