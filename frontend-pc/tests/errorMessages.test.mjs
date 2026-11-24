// #2480：PC 错误消息解析器四层顺序回归（node --test；frontend-pc 无测试运行器）。
//
// 核心断言：后端返回可读中文 message 时，必须原样到达用户；不能被 40900/40901
// 通用文案（「数据冲突，请刷新后重试」）遮蔽。ASCII message 仍走 code 家族映射。
import test from 'node:test'
import assert from 'node:assert/strict'

import {
  ERROR_CODE_MAP,
  ERROR_MESSAGE_MAP,
  isHumanReadableMessage,
  resolveErrorCore,
  resolveErrorMessage,
} from '../src/services/errorMessages.js'

// 与 mobile（errorMessageResolution.test.mjs）共用同一 8 条 fixture。
const FIXTURES = [
  { key: 'CN', input: { code: 40900, message: '网点名称已存在' }, layer: 'human_message', text: '网点名称已存在' },
  { key: 'ASCII', input: { code: 40900, message: 'site name conflict' }, layer: 'code_map', text: '数据冲突，请刷新后重试' },
  { key: 'EMPTY', input: { code: 40900, message: '' }, layer: 'code_map', text: '数据冲突，请刷新后重试' },
  { key: '40912', input: { code: 40912, message: '学生证作为第二证件需介绍信' }, layer: 'human_message', text: '学生证作为第二证件需介绍信' },
  { key: 'L1', input: { message: 'order not found' }, layer: 'message_map', text: '未找到订单' },
  { key: 'NULL', input: null, layer: 'fallback', text: '操作失败，请重试' },
  { key: 'UNKNOWN', input: { code: 49999, message: 'unknown conflict' }, layer: 'raw_message', text: 'unknown conflict' },
  { key: 'INVITE', input: { code: 40901, message: '该用户已有待处理的邀请申请' }, layer: 'human_message', text: '该用户已有待处理的邀请申请' },
]

test('409 + 可读中文 message：具体文案优先于通用 code 映射（缺陷版会得到「数据冲突」）', () => {
  const core = resolveErrorCore({ code: 40900, message: '网点名称已存在' })
  assert.equal(core.layer, 'human_message')
  assert.equal(core.text, '网点名称已存在')
  assert.notEqual(core.text, ERROR_CODE_MAP[40900])
  // 负例口径：仍被遮蔽 → 失败
  assert.notEqual(resolveErrorMessage({ code: 40900, message: '网点名称已存在' }), '数据冲突，请刷新后重试')
})

test('ASCII message 视为机器码：仍走 code 家族通用文案（不外溢英文技术串）', () => {
  const core = resolveErrorCore({ code: 40900, message: 'site name conflict' })
  assert.equal(core.layer, 'code_map')
  assert.equal(core.text, '数据冲突，请刷新后重试')
})

test('无 message 的 409：通用文案兜底不变', () => {
  assert.deepEqual(resolveErrorCore({ code: 40900, message: '' }), {
    text: '数据冲突，请刷新后重试',
    layer: 'code_map',
  })
  assert.deepEqual(resolveErrorCore({ code: 40901 }), {
    text: '数据冲突，请刷新后重试',
    layer: 'code_map',
  })
})

test('4091x（不在 code map）继续透传中文 message', () => {
  const core = resolveErrorCore({ code: 40912, message: '学生证作为第二证件需介绍信' })
  assert.equal(core.layer, 'human_message')
  assert.equal(core.text, '学生证作为第二证件需介绍信')
})

test('L1 英文精确键 → 中文翻译（#1675 行为不变）', () => {
  assert.equal(resolveErrorMessage({ message: 'order not found' }), '未找到订单')
  assert.ok(ERROR_MESSAGE_MAP['order not found'])
})

test('null / 缺字段 → fallback；自定义 fallback 生效', () => {
  assert.equal(resolveErrorMessage(null), '操作失败，请重试')
  assert.equal(resolveErrorMessage(undefined), '操作失败，请重试')
  assert.equal(resolveErrorMessage({}, '自定义兜底'), '自定义兜底')
  assert.equal(resolveErrorMessage(null, '自定义兜底'), '自定义兜底')
})

test('非字符串 message（对象/数组）不渲染 [object Object]，也不抛错', () => {
  assert.equal(isHumanReadableMessage({ a: 1 }), false)
  assert.equal(isHumanReadableMessage(['中文']), false)
  assert.equal(isHumanReadableMessage('中文'), true)
  assert.equal(isHumanReadableMessage('ascii only'), false)
  const core = resolveErrorCore({ code: 40900, message: { detail: '网点名称已存在' } })
  assert.equal(core.text, '数据冲突，请刷新后重试')
  assert.ok(!String(resolveErrorMessage({ code: 40900, message: { detail: 'x' } })).includes('[object Object]'))
})

test('八条 fixture 的文本与层序（LAYERS 序列须与 mobile 完全一致）', () => {
  const layers = []
  for (const fixture of FIXTURES) {
    const core = resolveErrorCore(fixture.input)
    assert.equal(core.text, fixture.text, `${fixture.key} text`)
    assert.equal(core.layer, fixture.layer, `${fixture.key} layer`)
    layers.push(core.layer)
  }
  const byKey = Object.fromEntries(
    FIXTURES.map((fixture) => [fixture.key, resolveErrorCore(fixture.input)]),
  )
  // 供 scripts/verify-2480.sh r1 断言
  console.log(`C40900_CN=${byKey.CN.text}`)
  console.log(`L40900_CN=${byKey.CN.layer}`)
  console.log(`C40900_ASCII=${byKey.ASCII.text}`)
  console.log(`L40900_ASCII=${byKey.ASCII.layer}`)
  console.log(`C40900_EMPTY=${byKey.EMPTY.text}`)
  console.log(`C40912_CN=${byKey['40912'].text}`)
  console.log(`C_L1=${byKey.L1.text}`)
  console.log(`C_NULL=${byKey.NULL.text}`)
  console.log(`C49999_ASCII=${byKey.UNKNOWN.text}`)
  console.log(`L49999_ASCII=${byKey.UNKNOWN.layer}`)
  console.log(`LAYERS=${layers.join(',')}`)
})
