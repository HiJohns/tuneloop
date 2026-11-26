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
  // #2504：ASCII 机器串不再经 L4 直出（旧期望 raw_message 已翻转）
  { key: 'UNKNOWN', input: { code: 49999, message: 'unknown conflict' }, layer: 'fallback' },
  { key: 'INVITE', input: { code: 40901, message: '该用户已有待处理的邀请申请' }, layer: 'human_message' },
]

// 与 errorMessages.js 的 map 同口径的极小样本（只覆盖本用例需要的键）。
const CODE_MAP = {
  40303: '暂无权限访问该页面',
  40900: '数据冲突，请刷新后重试',
  40901: '数据冲突，请刷新后重试',
  50000: '服务器开小差了，请稍后再试',
  50001: '服务暂不可用，请稍后再试',
}
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

test('未知 code 的 ASCII message 不外溢（#2504 收紧 L4 → fallback），不含 [object Object]', () => {
  assert.equal(core({ code: 49999, message: 'unknown conflict' }).layer, 'fallback')
  assert.equal(core({ code: 49999, message: 'unknown conflict' }).text, '操作失败，请重试')
  const objectMessage = core({ code: 40900, message: { detail: '网点名称已存在' } })
  assert.equal(objectMessage.layer, 'code_map')
  assert.equal(objectMessage.text, '数据冲突，请刷新后重试')
  assert.ok(!objectMessage.text.includes('[object Object]'))
  console.log(`L49999_ASCII=${core({ code: 49999, message: 'unknown conflict' }).layer}`)
  console.log(`C49999_ASCII=${core({ code: 49999, message: 'unknown conflict' }).text}`)
})

test('40303 + access denied → code_map 中文（#2504 主诉，与 PC 同文案）', () => {
  const result = core({ code: 40303, message: 'access denied' })
  assert.equal(result.layer, 'code_map')
  assert.equal(result.text, '暂无权限访问该页面')
  // 负例：英文 `access denied` 直出 → 失败
  assert.ok(!result.text.includes('access denied'))
  console.log(`L40303=${result.layer}`)
  console.log(`C40303=${result.text}`)
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

// #2560：5xx 改按「内部诊断形态」判定——固定用户文案透传，诊断形态仍屏蔽。
// 5 条 fixture 与 frontend-pc/tests/errorMessages-5xx-2497.test.mjs 逐字一致（双端平价）。
const FIVE_XX_FIXTURES = [
  {
    key: '5xx_actionable_iam_rollback',
    input: { code: 50000, message: '切换网点失败且 IAM 回滚失败，请联系管理员人工核对 IAM 组织关系' },
    layer: 'human_message',
  },
  {
    key: '5xx_actionable_smtp',
    input: { code: 50000, message: 'SMTP 配置校验失败，请检查端口与凭据后重试' },
    layer: 'human_message',
  },
  {
    key: '5xx_diag_pq',
    input: { code: 50000, message: '标记删除失败：pq: duplicate key value violates unique constraint' },
    layer: 'code_map',
  },
  {
    key: '5xx_diag_rpc',
    input: { code: 50001, message: '级联解除商户成员失败：rpc error: connection refused' },
    layer: 'code_map',
  },
  {
    key: '5xx_diag_timeout',
    input: { code: 50999, message: '内部诊断：dial tcp 10.0.0.1: i/o timeout' },
    layer: 'fallback',
  },
]

test('5xx 形态判定：可操作文案透传、内部诊断屏蔽（#2560）', () => {
  let pos = 0
  let neg = 0
  for (const fixture of FIVE_XX_FIXTURES) {
    const result = core(fixture.input)
    assert.equal(result.layer, fixture.layer, `${fixture.key} layer`)
    if (fixture.layer === 'human_message') {
      assert.equal(result.text, fixture.input.message, `${fixture.key}: 文案必须逐字透传`)
      pos += 1
    } else {
      assert.ok(!result.text.includes('pq:'), `${fixture.key}: 内部串不得回显`)
      assert.ok(!result.text.includes('rpc error'), `${fixture.key}: 内部串不得回显`)
      assert.ok(!result.text.includes('i/o timeout'), `${fixture.key}: 内部串不得回显`)
      neg += 1
    }
  }
  console.log(`M2560_POS=${pos}`)
  console.log(`M2560_NEG=${neg}`)
})
