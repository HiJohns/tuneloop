// #2497 / #2560：5xx 文案按**形态**判定（PC + mobile 双端同口径回归，node --test）。
//
// #2497 引入的是粗粒度守卫（code >= 50000 且含 CJK → 一律 code_map/fallback），它挡住了
// 「中文前缀 + err.Error()」的内部诊断，但也把后端**面向用户、可操作**的 5xx 固定文案一并
// 吞掉。本单（#2560）把守卫收窄为「内部诊断形态」判定：
//   - 命中 marker 表或「中文前缀 + 冒号 + ASCII 尾串」→ code_map（有映射）或 fallback；
//   - 未命中（固定用户文案，如 IAM 回滚提示 / SMTP 配置提示）→ human_message 原样透传；
//   - 业务码（40002/40004/40005/40900 等）行为不变（#2480 不回退）。
//
// 双端各一份核心实现：PC = frontend-pc/src/services/errorMessages.js，
// mobile = frontend-mobile/src/services/errorMessageResolution.mjs（纯核心）。
import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import {
  ERROR_CODE_MAP as PC_CODE_MAP,
  resolveErrorCore as resolvePcCore,
} from '../src/services/errorMessages.js'
import { resolveErrorCore as resolveMobileCore } from '../../frontend-mobile/src/services/errorMessageResolution.mjs'

const HERE = path.dirname(fileURLToPath(import.meta.url))
const REPO_ROOT = path.resolve(HERE, '../..')

// mobile 的 code map 由 mobile errorMessages.js 注入；这里用与 mobile 同源的子集
// （50000/50001 文案两端一致，平价脚本 check-error-message-parity-2480.mjs 兜住漂移）。
const MOBILE_CODE_MAP = {
  40900: PC_CODE_MAP[40900],
  50000: PC_CODE_MAP[50000],
  50001: PC_CODE_MAP[50001],
}

const INTERNAL_TAG = 'pq: duplicate key value violates unique constraint'
const LEAK_TAGS = [INTERNAL_TAG, 'rpc error', 'i/o timeout']

// #2497 负例：内部诊断形态必须继续屏蔽（D3：不得回退）。
const NEG_CASES = [
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
]

// #2560 正例：后端面向用户、可操作的 5xx 固定文案必须原样透传（双端一致）。
const ACTIONABLE_IAM = '切换网点失败且 IAM 回滚失败，请联系管理员人工核对 IAM 组织关系'
const ACTIONABLE_SMTP = 'SMTP 配置校验失败，请检查端口与凭据后重试'
const POS_CASES = [
  {
    key: '5xx_actionable_iam_rollback',
    input: { code: 50000, message: ACTIONABLE_IAM },
    pc: { text: ACTIONABLE_IAM, layer: 'human_message' },
    mobile: { text: ACTIONABLE_IAM, layer: 'human_message' },
  },
  {
    key: '5xx_actionable_smtp',
    input: { code: 50000, message: ACTIONABLE_SMTP },
    pc: { text: ACTIONABLE_SMTP, layer: 'human_message' },
    mobile: { text: ACTIONABLE_SMTP, layer: 'human_message' },
  },
]

// 业务码（非 5xx）行为不变（#2480 不回退）。
const BUSINESS_CASES = [
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

const CASES = [...NEG_CASES, ...POS_CASES, ...BUSINESS_CASES]

test('PC: 可操作 5xx 文案原样透传 (#2560)', () => {
  let pos = 0
  for (const c of POS_CASES) {
    const got = resolvePcCore(c.input)
    assert.equal(got.text, c.pc.text, `${c.key}: text`)
    assert.equal(got.layer, c.pc.layer, `${c.key}: layer`)
    // 负例钉子：不得退化为 code 家族通用文案
    assert.notEqual(got.text, PC_CODE_MAP[50000], `${c.key}: 不得被通用文案遮蔽`)
    pos += 1
  }
  console.log(`PC_POS=${pos}`)
})

test('mobile: 可操作 5xx 文案原样透传 (#2560)', () => {
  let pos = 0
  for (const c of POS_CASES) {
    const got = resolveMobileCore(c.input, { codeMap: MOBILE_CODE_MAP })
    assert.equal(got.text, c.mobile.text, `${c.key}: text`)
    assert.equal(got.layer, c.mobile.layer, `${c.key}: layer`)
    pos += 1
  }
  console.log(`MOBILE_POS=${pos}`)
})

test('PC: 5xx 内部诊断仍屏蔽（#2497 不回退）', () => {
  let neg = 0
  let leak = 0
  for (const c of NEG_CASES) {
    const got = resolvePcCore(c.input)
    assert.equal(got.text, c.pc.text, `${c.key}: text`)
    assert.equal(got.layer, c.pc.layer, `${c.key}: layer`)
    const leaked = LEAK_TAGS.filter((tag) => got.text.includes(tag))
    leak += leaked.length
    assert.equal(leaked.length, 0, `${c.key}: 内部串不得回显 (${leaked.join(',')})`)
    neg += 1
  }
  console.log(`PC_NEG=${neg}`)
  console.log(`LEAK=${leak}`)
})

test('mobile: 5xx 内部诊断仍屏蔽（#2497 不回退）', () => {
  let neg = 0
  for (const c of NEG_CASES) {
    const got = resolveMobileCore(c.input, { codeMap: MOBILE_CODE_MAP })
    assert.equal(got.text, c.mobile.text, `${c.key}: text`)
    assert.equal(got.layer, c.mobile.layer, `${c.key}: layer`)
    neg += 1
  }
  console.log(`MOBILE_NEG=${neg}`)
})

test('业务码（非 5xx）中文文案继续透传（#2480 不回退）', () => {
  for (const c of BUSINESS_CASES) {
    const pc = resolvePcCore(c.input)
    const mobile = resolveMobileCore(c.input, { codeMap: MOBILE_CODE_MAP })
    assert.equal(pc.text, c.pc.text, `${c.key}: pc text`)
    assert.equal(pc.layer, c.pc.layer, `${c.key}: pc layer`)
    assert.equal(mobile.text, c.mobile.text, `${c.key}: mobile text`)
    assert.equal(mobile.layer, c.mobile.layer, `${c.key}: mobile layer`)
  }
})

test('PC/mobile 平价：同 fixture 同文本同层 (#2497/#2560)', () => {
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

// mobile 生产路径（Round 1 Bug 2）：解析核心只有在 api.js 的响应分支把 code 一并交出去时
// 才可达。这里同时断言「接线」与「展示文本」：
//   1) api.js 的 !response.ok 分支必须把 {code, message} 交给 resolveErrorMessage，
//      并抛出解析结果（而不是原样 throw 后端 message）；
//   2) 用 mobile 真实 code map（从源文件静态抽取，与 check-error-message-parity-2480.mjs
//      同口径）跑同一 5xx 载荷，模拟 apiFetch 抛错后页面拿到的展示文本。
test('mobile 生产路径：api.js 非 2xx 分支经解析层，内部诊断不上屏 (#2497)', () => {
  const apiSrc = fs.readFileSync(
    path.join(REPO_ROOT, 'frontend-mobile/src/services/api.js'),
    'utf8',
  )
  assert.match(
    apiSrc,
    /resolveErrorMessage\(\s*\{\s*code:\s*error\.code,\s*message:\s*error\.message\s*\}/,
    'mobile chokepoint 必须把 code+message 交给解析层',
  )
  assert.match(
    apiSrc,
    /const requestError = new Error\(resolvedMessage\)/,
    '抛出的 Error 必须是解析后的文案',
  )
  assert.doesNotMatch(
    apiSrc,
    /throw new Error\(error\.message \|\| error\.code/,
    '不得原样抛出后端 message（会绕过 5xx 守卫）',
  )

  const mobileSrc = fs.readFileSync(
    path.join(REPO_ROOT, 'frontend-mobile/src/services/errorMessages.js'),
    'utf8',
  )
  const mobileMapText = (code) => {
    const match = mobileSrc.match(new RegExp(`\\b${code}:\\s*'([^']*)'`))
    return match ? match[1] : undefined
  }
  const generic = mobileMapText(50000)
  assert.ok(generic, 'mobile ERROR_CODE_MAP 必须含 50000')

  // 模拟 apiFetch 收到 500 + {code:50000, message:'标记删除失败：pq: ...'}
  const resolved = resolveMobileCore(
    { code: 50000, message: `标记删除失败：${INTERNAL_TAG}` },
    { codeMap: { 50000: generic } },
  )
  assert.equal(resolved.text, generic, 'mobile 展示文本 = 通用文案')
  assert.equal(resolved.layer, 'code_map')
  assert.ok(!resolved.text.includes(INTERNAL_TAG), '内部串不得回显')
  console.log('MOBILE_PROD_PATH=1')
})

// #2560 生产路径：可操作 5xx 文案经同一解析层到达页面（不得停留在通用文案）。
test('mobile 生产路径：可操作 5xx 文案上屏 (#2560)', () => {
  const mobileSrc = fs.readFileSync(
    path.join(REPO_ROOT, 'frontend-mobile/src/services/errorMessages.js'),
    'utf8',
  )
  const generic = mobileSrc.match(/\b50000:\s*'([^']*)'/)?.[1]
  assert.ok(generic, 'mobile ERROR_CODE_MAP 必须含 50000')
  const resolved = resolveMobileCore(
    { code: 50000, message: ACTIONABLE_IAM },
    { codeMap: { 50000: generic } },
  )
  assert.equal(resolved.text, ACTIONABLE_IAM)
  assert.equal(resolved.layer, 'human_message')
  assert.notEqual(resolved.text, generic)
  console.log('MOBILE_ACTIONABLE_PATH=1')
})
