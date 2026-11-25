// #2480：mobile 错误消息解析核心（纯 .mjs，无 i18n/Taro 依赖）。
// 与 PC（frontend-pc/src/services/errorMessages.js）共用同一 8 条 fixture 与同一层序，
// 跨包平价由 scripts/check-error-message-parity-2480.mjs 兜住（D5）。
// 运行：node --test frontend-mobile/src/services/errorMessageResolution.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'

import {
  isHumanReadableMessage,
  resolveErrorCore,
} from './errorMessageResolution.mjs'

// 与 PC 测试（frontend-pc/tests/errorMessages.test.mjs）完全相同的 8 条 fixture。
const FIXTURES = [
  { key: 'CN', input: { code: 40900, message: '网点名称已存在' }, layer: 'human_message' },
  { key: 'ASCII', input: { code: 40900, message: 'site name conflict' }, layer: 'code_map' },
  { key: 'EMPTY', input: { code: 40900, message: '' }, layer: 'code_map' },
  { key: '40912', input: { code: 40912, message: '学生证作为第二证件需介绍信' }, layer: 'human_message' },
  { key: 'L1', input: { message: 'order not found' }, layer: 'message_map' },
  { key: 'NULL', input: null, layer: 'fallback' },
  { key: 'UNKNOWN', input: { code: 49999, message: 'unknown conflict' }, layer: 'raw_message' },
  { key: 'INVITE', input: { code: 40901, message: '该用户已有待处理的邀请申请' }, layer: 'human_message' },
]

// 与 errorMessages.js 的 map 同口径的极小样本（只覆盖本用例需要的键）。
const CODE_MAP = { 40900: '数据冲突，请刷新后重试', 40901: '数据冲突，请刷新后重试' }
const MESSAGE_MAP = { 'order not found': '未找到订单' }

function core(input, overrides = {}) {
  return resolveErrorCore(input, { codeMap: CODE_MAP, messageMap: MESSAGE_MAP, ...overrides })
}

test('CJK 判定：中文 true，ASCII / 非字符串 false', () => {
  assert.equal(isHumanReadableMessage('网点名称已存在'), true)
  assert.equal(isHumanReadableMessage('site name conflict'), false)
  assert.equal(isHumanReadableMessage(''), false)
  assert.equal(isHumanReadableMessage(null), false)
  assert.equal(isHumanReadableMessage({ a: 1 }), false)
})

test('可读中文 message 优先于 code 家族通用文案（缺陷版为 code_map）', () => {
  const result = core({ code: 40900, message: '网点名称已存在' })
  assert.equal(result.layer, 'human_message')
  assert.equal(result.text, '网点名称已存在')
  assert.notEqual(result.text, CODE_MAP[40900])
})

test('ASCII message 仍走 code map（不外溢英文技术串）', () => {
  const result = core({ code: 40900, message: 'site name conflict' })
  assert.equal(result.layer, 'code_map')
  assert.equal(result.text, '数据冲突，请刷新后重试')
})

test('空 / 缺 message 的 409 → code map 兜底', () => {
  assert.equal(core({ code: 40900, message: '' }).layer, 'code_map')
  assert.equal(core({ code: 40901 }).layer, 'code_map')
  assert.equal(core({ code: 40901 }).text, '数据冲突，请刷新后重试')
})

test('4091x 未入 map：中文 message 继续透传', () => {
  const result = core({ code: 40912, message: '学生证作为第二证件需介绍信' })
  assert.equal(result.layer, 'human_message')
})

test('L1 英文精确键 → 中文翻译；null → fallback', () => {
  assert.equal(core({ message: 'order not found' }).layer, 'message_map')
  assert.equal(core({ message: 'order not found' }).text, '未找到订单')
  assert.equal(core(null).layer, 'fallback')
  assert.equal(core(null, { fallback: '自定义兜底' }).text, '自定义兜底')
})

test('未知 code 的其它 message 透传（raw_message），不含 [object Object]', () => {
  assert.equal(core({ code: 49999, message: 'unknown conflict' }).layer, 'raw_message')
  assert.equal(core({ code: 49999, message: 'unknown conflict' }).text, 'unknown conflict')
  const objectMessage = core({ code: 40900, message: { detail: '网点名称已存在' } })
  assert.equal(objectMessage.layer, 'code_map')
  assert.equal(objectMessage.text, '数据冲突，请刷新后重试')
  assert.ok(!objectMessage.text.includes('[object Object]'))
})

test('八条 fixture 的层序与 PC 完全一致', () => {
  const layers = []
  for (const fixture of FIXTURES) {
    const result = core(fixture.input)
    assert.equal(result.layer, fixture.layer, `${fixture.key} layer`)
    layers.push(result.layer)
  }
  const byKey = Object.fromEntries(FIXTURES.map((f) => [f.key, core(f.input)]))
  // 供 scripts/verify-2480.sh r2 断言
  console.log(`L_CN=${byKey.CN.layer}`)
  console.log(`L_ASCII=${byKey.ASCII.layer}`)
  console.log(`L_EMPTY=${byKey.EMPTY.layer}`)
  console.log(`L_40912=${byKey['40912'].layer}`)
  console.log(`L_NULL=${byKey.NULL.layer}`)
  console.log(`LAYERS=${layers.join(',')}`)
})
