// #2597：中转中心错误文案解析链（PC + mobile 双端同口径回归，node --test）。
//
// 背景：#2597 的后端把「命名空间主组织配置异常」（持久配置不变量违反，重试永不成功）
// 从瞬时故障里拆出来，改用专用码 50014 + 可行动文案；前端该页 5 处错误分支改为走
// `resolveErrorMessage` 唯一出口。本测试锁定：
//   1) 50014 层胜出 = code_map（5xx 中文内部诊断不透传，#2497）；
//   2) 后端若误把英文内部串放进 message（`namespace has 2 primary organizations…`），
//      界面文案仍为中文（ASCII_LEAK=0）；
//   3) 真实抛出链 `resolveHttpFailure(500, {code:50014})` 保留 code（页面 catch 可判码）；
//   4) 业务码 40002 中文串继续走 human_message（停用被拒文案）；
//   5) PC / mobile 两端的 50014 文案逐字一致（漂移会让「用户看到的」≠「后端写的」）。
import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import {
  ERROR_CODE_MAP as PC_CODE_MAP,
  resolveErrorCore as resolvePcCore,
} from '../src/services/errorMessages.js'
import { resolveHttpFailure } from '../src/services/apiError.js'
import { resolveErrorCore as resolveMobileCore } from '../../frontend-mobile/src/services/errorMessageResolution.mjs'

const HERE = path.dirname(fileURLToPath(import.meta.url))
const REPO_ROOT = path.resolve(HERE, '../..')

// 后端 handler 的 message（与 ERROR_CODE_MAP[50014] 必须逐字一致）。
const BACKEND_MSG = '命名空间主组织配置异常（应为 1 个），请联系平台管理员处理'
const INTERNAL_ASCII = 'namespace has 2 primary organizations (expected exactly 1)'
const RETRY_HINT = '请稍后再试'

// mobile 的 ERROR_CODE_MAP 由 mobile errorMessages.js 注入（其 API 是纯核心 + 注入 map），
// 这里从源文件静态抽取，与 scripts/check-error-message-parity-2480.mjs 同口径。
function mobileMapText(code) {
  const src = fs.readFileSync(
    path.join(REPO_ROOT, 'frontend-mobile/src/services/errorMessages.js'),
    'utf8',
  )
  const match = src.match(new RegExp(`\\b${code}:\\s*'([^']*)'`))
  return match ? match[1] : undefined
}

const MOBILE_MAP = { 50014: mobileMapText(50014) }

test('PC: 50014 走 code_map，文案可行动且不含「请稍后再试」 (#2597)', () => {
  const got = resolvePcCore({ code: 50014, message: BACKEND_MSG })
  assert.equal(got.layer, 'code_map', '5xx 中文内部诊断不得走 human_message')
  assert.equal(got.text, BACKEND_MSG, 'code_map 文案必须与后端 message 一致')
  assert.ok(!got.text.includes(RETRY_HINT), '配置类错误不得出现「请稍后再试」')
  console.log(`E50014_LAYER=${got.layer}`)
  console.log(`E50014_RETRY_HINT=${got.text.includes(RETRY_HINT) ? 1 : 0}`)
})

test('PC: 后端误传英文内部串时不外溢（含 code 缺失回退） (#2597)', () => {
  const withCode = resolvePcCore({ code: 50014, message: INTERNAL_ASCII })
  assert.equal(withCode.layer, 'code_map')
  assert.equal(withCode.text, BACKEND_MSG)
  // 无 code（损坏载荷）时退到 fallback，同样不得回显英文内部串。
  const noCode = resolvePcCore({ message: INTERNAL_ASCII }, { fallback: '操作失败，请重试' })
  assert.equal(noCode.layer, 'fallback')
  assert.equal(noCode.text, '操作失败，请重试')
  const leaked = [withCode.text, noCode.text].filter(
    (t) => t.includes('primary organizations') || t.includes('expected exactly 1'),
  )
  console.log(`ASCII_LEAK=${leaked.length}`)
  assert.equal(leaked.length, 0, '后端英文内部串不得出现在界面文案中')
})

test('PC: 真实抛出链保留 code=50014 (#2597)', () => {
  const failure = resolveHttpFailure(500, { code: 50014, message: BACKEND_MSG })
  assert.equal(failure.kind, 'throw', '5xx 必须抛错（页面 catch 才能接管）')
  assert.equal(failure.value.name, 'ApiError')
  assert.equal(failure.value.code, 50014, '抛出的 Error 必须带 code，页面才能判码')
  assert.equal(failure.value.message, BACKEND_MSG)
  assert.ok(!failure.value.message.includes(RETRY_HINT))
  console.log(`HTTP_THROW_CODE=${failure.value.code}`)
})

test('PC: 业务码 40002 中文串继续透传（停用被拒） (#2597)', () => {
  const stopMsg = '该中转网点仍被路由引用，请先解除相关路由'
  const got = resolvePcCore({ code: 40002, message: stopMsg })
  assert.equal(got.layer, 'human_message', '业务可纠正冲突仍原样透传（#2480 不回退）')
  assert.equal(got.text, stopMsg)
  console.log(`HUMAN_40002=${got.layer === 'human_message' ? 1 : 0}`)
})

test('PC/mobile: 50014 文案逐字一致 (#2597)', () => {
  assert.ok(PC_CODE_MAP[50014], 'PC ERROR_CODE_MAP 必须含 50014')
  assert.ok(MOBILE_MAP[50014], 'mobile ERROR_CODE_MAP 必须含 50014')
  assert.equal(MOBILE_MAP[50014], PC_CODE_MAP[50014], '双端 50014 文案必须逐字一致')

  const mobile = resolveMobileCore(
    { code: 50014, message: INTERNAL_ASCII },
    { codeMap: MOBILE_MAP },
  )
  assert.equal(mobile.layer, 'code_map')
  assert.equal(mobile.text, PC_CODE_MAP[50014], 'mobile 与 PC 解析结果一致')
  console.log(`MOBILE_MAP_MATCH=${MOBILE_MAP[50014] === PC_CODE_MAP[50014] ? 1 : 0}`)
})
