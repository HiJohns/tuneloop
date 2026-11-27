// #2609 — `userFacingErrorMessage` 运行时语义（node --test，纯函数）。
//
// 目的：钉死「断网时不得把浏览器 ASCII 技术串当用户文案」这条语义，
// 并证明本 fixture 能区分「守卫实现」与「朴素实现」(e?.message || fb)——
// 后者在缺陷版本上恒真，正是 #2609 的验收探针盲点。
//
// 输出：末尾打印 token 行，供 scripts/verify-2609.sh r2 断言（非 node:test 断言本身）。
import test from 'node:test'
import assert from 'node:assert/strict'

import { userFacingErrorMessage } from '../src/services/apiError.js'

const FALLBACK = '操作失败'

// 浏览器 fetch reject 的真实 message（ASCII 技术串）。
const NETWORK_MESSAGES = [
  'Failed to fetch',
  'NetworkError when attempting to fetch resource.',
  'Load failed',
]

function networkError(message) {
  const e = new TypeError(message)
  return e
}

// 8 类输入 → 期望（见 Accepted Plan 变更点 4 表）。
const CASES = [
  { name: 'network:Failed to fetch', error: networkError(NETWORK_MESSAGES[0]), fallback: FALLBACK, expected: FALLBACK },
  { name: 'network:NetworkError', error: networkError(NETWORK_MESSAGES[1]), fallback: FALLBACK, expected: FALLBACK },
  { name: 'network:Load failed', error: networkError(NETWORK_MESSAGES[2]), fallback: FALLBACK, expected: FALLBACK },
  { name: 'apierror:message', error: Object.assign(new Error('分类名称已存在'), { name: 'ApiError' }), fallback: FALLBACK, expected: '分类名称已存在' },
  { name: 'apierror:empty', error: Object.assign(new Error(''), { name: 'ApiError' }), fallback: FALLBACK, expected: FALLBACK },
  { name: 'plain-error', error: new Error('导出失败'), fallback: FALLBACK, expected: FALLBACK },
  { name: 'non-object:string', error: 'boom', fallback: FALLBACK, expected: FALLBACK },
  { name: 'non-object:null', error: null, fallback: FALLBACK, expected: FALLBACK },
  { name: 'fallback:verbatim', error: networkError(NETWORK_MESSAGES[0]), fallback: '登记失败，请调整后重试', expected: '登记失败，请调整后重试' },
]

// 朴素实现（#2609 修前写法）：把异常 message 无条件当文案。
const naive = (e, fb) => (e && e.message) || fb

function safeCall(fn, error, fallback) {
  try {
    return { value: fn(error, fallback), threw: false }
  } catch (err) {
    return { value: undefined, threw: true }
  }
}

function hasAsciiTech(value) {
  if (typeof value !== 'string') return false
  return NETWORK_MESSAGES.some((m) => value.includes(m))
}

test('userFacingErrorMessage matches every fixture', () => {
  for (const c of CASES) {
    const got = userFacingErrorMessage(c.error, c.fallback)
    assert.equal(got, c.expected, `fixture ${c.name}`)
  }
})

test('userFacingErrorMessage never leaks a browser ASCII technical string', () => {
  for (const c of CASES) {
    const got = userFacingErrorMessage(c.error, c.fallback)
    assert.equal(hasAsciiTech(got), false, `fixture ${c.name} leaked ASCII: ${got}`)
  }
})

test('userFacingErrorMessage never throws on non-object throw values', () => {
  for (const c of CASES) {
    const r = safeCall(userFacingErrorMessage, c.error, c.fallback)
    assert.equal(r.threw, false, `fixture ${c.name} threw`)
  }
})

test('naive implementation is distinguishable (fixture is falsifiable)', () => {
  const naiveMismatches = CASES.filter((c) => safeCall(naive, c.error, c.fallback).value !== c.expected)
  assert.ok(naiveMismatches.length > 0, 'fixture cannot distinguish naive from guarded implementation')
})

// ---- token 汇总（供 verify-2609.sh r2 断言，不参与 node:test 判定）---------
const actual = CASES.map((c) => safeCall(userFacingErrorMessage, c.error, c.fallback).value)
const naiveActual = CASES.map((c) => safeCall(naive, c.error, c.fallback).value)

const networkCases = CASES.filter((c) => c.name.startsWith('network:'))
const networkFallback = networkCases.every((c) => actual[CASES.indexOf(c)] === FALLBACK) ? 1 : 0
const apiErrorCase = CASES.find((c) => c.name === 'apierror:message')
const apiErrorPassthrough = actual[CASES.indexOf(apiErrorCase)] === '分类名称已存在' ? 1 : 0
const apiErrorEmptyCase = CASES.find((c) => c.name === 'apierror:empty')
const apiErrorEmptyFallback = actual[CASES.indexOf(apiErrorEmptyCase)] === FALLBACK ? 1 : 0
const plainErrorCase = CASES.find((c) => c.name === 'plain-error')
const plainErrorFallback = actual[CASES.indexOf(plainErrorCase)] === FALLBACK ? 1 : 0
const nonObjectCases = CASES.filter((c) => c.name.startsWith('non-object:'))
const nonObjectFallback = nonObjectCases.every((c) => actual[CASES.indexOf(c)] === FALLBACK) ? 1 : 0
const verbatimCase = CASES.find((c) => c.name === 'fallback:verbatim')
const fallbackVerbatim = actual[CASES.indexOf(verbatimCase)] === verbatimCase.fallback ? 1 : 0
const asciiLeak = actual.filter(hasAsciiTech).length
const naiveLeak = naiveActual.filter(hasAsciiTech).length > 0 ? 1 : 0
const naiveMismatchCount = CASES.filter((c, i) => naiveActual[i] !== c.expected).length
const runtimeInjectRed = naiveMismatchCount > 0 && asciiLeak === 0 ? 1 : 0

console.log('HELPER_IMPL=userFacingErrorMessage')
console.log(`NETWORK_FALLBACK=${networkFallback}`)
console.log(`APIERROR_PASSTHROUGH=${apiErrorPassthrough}`)
console.log(`APIERROR_EMPTY_FALLBACK=${apiErrorEmptyFallback}`)
console.log(`PLAIN_ERROR_FALLBACK=${plainErrorFallback}`)
console.log(`NON_OBJECT_FALLBACK=${nonObjectFallback}`)
console.log(`FALLBACK_VERBATIM=${fallbackVerbatim}`)
console.log(`ASCII_LEAK=${asciiLeak}`)
console.log(`NAIVE_LEAK=${naiveLeak}`)
console.log(`NAIVE_MISMATCH=${naiveMismatchCount}`)
console.log(`RUNTIME_INJECT_RED=${runtimeInjectRed}`)
