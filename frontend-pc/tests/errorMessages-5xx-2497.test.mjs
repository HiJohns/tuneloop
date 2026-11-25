// #2497：5xx 内部诊断不得透传（PC + mobile 双端同口径回归，node --test）。
//
// 背景：#2480 把「含 CJK 的 message」提升为 human_message 优先透传，于是后端
// 「中文前缀 + err.Error()」的内部诊断（如「标记删除失败：pq: ...」）会被原样展示。
// 本单在后端收敛文案（固定用户文案 + log.Printf）之外，前端再加窄守卫：
//   code >= 50000 且 message 含 CJK → 走 code_map（有映射）或 fallback（无映射），
//   绝不回显内部串；业务码（40002/40004/40005/40900 等）继续透传（#2480 不回退）。
//
// 双端各一份核心实现：PC = frontend-pc/src/services/errorMessages.js，
// mobile = frontend-mobile/src/services/errorMessageResolution.mjs（纯核心）。
import test from 'node:test'
import assert from 'node:assert/strict'

import {
  ERROR_CODE_MAP as PC_CODE_MAP,
  resolveErrorCore as resolvePcCore,
} from '../src/services/errorMessages.js'
import { resolveErrorCore as resolveMobileCore } from '../../frontend-mobile/src/services/errorMessageResolution.mjs'

// mobile 的 code map 由 mobile errorMessages.js 注入；这里用与 mobile 同源的子集
// （50000/50001 文案两端一致，平价脚本 check-error-message-parity-2480.mjs 兜住漂移）。
const MOBILE_CODE_MAP = {
  40900: PC_CODE_MAP[40900],
  50000: PC_CODE_MAP[50000],
  50001: PC_CODE_MAP[50001],
}

const INTERNAL_TAG = 'pq: duplicate key value violates unique constraint'

const CASES = [
  {
    key: '5xx_cjk_50000',
    input: { code: 50000, message: `标记删除失败：${INTERNAL_TAG}` },
    pc: { text: '服务器开小差了，请稍后再试', layer: 'code_map' },
    mobile: { text: '服务器开小差了，请稍后再试', layer: 'code_map' },
  },
  {
    key: '5xx_cjk_50001',
    input: { code: 50001, message: '级联解除商户成员失败：rpc error: connection refused' },
    pc: { text: '服务暂不可用，请稍后再试', layer: 'code_map' },
    mobile: { text: '服务暂不可用，请稍后再试', layer: 'code_map' },
  },
  {
    key: '5xx_cjk_unmapped',
    input: { code: 50999, message: '内部诊断：dial tcp 10.0.0.1: i/o timeout' },
    pc: { text: '操作失败，请重试', layer: 'fallback' },
    mobile: { text: '操作失败，请重试', layer: 'fallback' },
  },
  {
    key: 'business_40900_cjk_passthrough',
    input: { code: 40900, message: '网点名称已存在' },
    pc: { text: '网点名称已存在', layer: 'human_message' },
    mobile: { text: '网点名称已存在', layer: 'human_message' },
  },
  {
    key: 'business_40002_cjk_passthrough',
    input: { code: 40002, message: '信息不完整，请补充必填项' },
    pc: { text: '信息不完整，请补充必填项', layer: 'human_message' },
    mobile: { text: '信息不完整，请补充必填项', layer: 'human_message' },
  },
]

test('PC: 5xx CJK 内部串不泄漏，业务码仍透传 (#2497)', () => {
  for (const c of CASES) {
    const got = resolvePcCore(c.input)
    assert.equal(got.text, c.pc.text, `${c.key}: text`)
    assert.equal(got.layer, c.pc.layer, `${c.key}: layer`)
    if (c.input?.message) {
      assert.ok(!got.text.includes(INTERNAL_TAG), `${c.key}: 内部串不得回显`)
      assert.ok(!got.text.includes('rpc error'), `${c.key}: 内部串不得回显`)
      assert.ok(!got.text.includes('i/o timeout'), `${c.key}: 内部串不得回显`)
    }
  }
})

test('mobile: 5xx CJK 内部串不泄漏，业务码仍透传 (#2497)', () => {
  for (const c of CASES) {
    const got = resolveMobileCore(c.input, { codeMap: MOBILE_CODE_MAP })
    assert.equal(got.text, c.mobile.text, `${c.key}: text`)
    assert.equal(got.layer, c.mobile.layer, `${c.key}: layer`)
  }
})

test('PC/mobile 平价：同 fixture 同文本同层 (#2497)', () => {
  for (const c of CASES) {
    const pc = resolvePcCore(c.input)
    const mobile = resolveMobileCore(c.input, { codeMap: MOBILE_CODE_MAP })
    assert.equal(pc.text, mobile.text, `${c.key}: 两端文本一致`)
    assert.equal(pc.layer, mobile.layer, `${c.key}: 两端层一致`)
  }
  console.log(`PARITY_CASES=${CASES.length}`)
})

test('5xx 守卫不影响 L1 精确英文键映射 (#2497)', () => {
  const got = resolvePcCore({ code: 50000, message: 'order not found' })
  assert.equal(got.text, '未找到订单')
  assert.equal(got.layer, 'message_map')
})
