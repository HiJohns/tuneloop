// #2598 R6：双端码表 `50015`（身份服务配置/权限异常）与后端常量一致性。
//
// 背景：新增错误码 50015（HTTP 500）表达「上游 IAM 401/403/404 —— 持久失败，
// 需平台管理员改数据/权限，不可重试」。§1.3 要求新码同步双端码表，且后端
// `iamConfigErrorMessage` 与 PC/mobile `ERROR_CODE_MAP[50015]` **逐字一致**
// （#2497 规则：code >= 50000 且含 CJK 的 message 走 code_map，不透传内部诊断）。
//
// PC 无渲染测试运行器（`frontend-pc/package.json` 无 vitest/jsdom）→ 本文件用
// `node --test` 覆盖纯函数解析 + 静态码表/常量抽取（Owner 豁免，沿用 #2597 D7）。
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
const MOBILE_MESSAGES = path.join(REPO_ROOT, 'frontend-mobile/src/services/errorMessages.js')
const BACKEND_HELPER = path.join(REPO_ROOT, 'backend/handlers/iam_failure.go')

const EXPECTED = '身份服务配置异常，请联系平台管理员处理'
const INTERNAL = 'CreateOrganization returned status 403: {"message":"permission denied: missing org:create"}'

function mapText(source, code) {
  const match = source.match(new RegExp(`\\b${code}:\\s*'([^']*)'`))
  return match ? match[1] : undefined
}

const mobileSource = fs.readFileSync(MOBILE_MESSAGES, 'utf8')
const MOBILE_CODE_MAP = { 50015: mapText(mobileSource, 50015) }
const backendSource = fs.readFileSync(BACKEND_HELPER, 'utf8')

test('PC 码表含 50015 且文案逐字匹配 (#2598)', () => {
  assert.equal(PC_CODE_MAP[50015], EXPECTED)
})

test('mobile 码表含 50015 且文案逐字匹配 (#2598)', () => {
  assert.equal(MOBILE_CODE_MAP[50015], EXPECTED)
})

test('后端 iamConfigErrorMessage 与双端码表逐字一致 (#2598)', () => {
  const constMatch = backendSource.match(/iamConfigErrorMessage\s*=\s*"([^"]*)"/)
  assert.ok(constMatch, 'backend/handlers/iam_failure.go 必须定义 iamConfigErrorMessage')
  assert.equal(constMatch[1], EXPECTED)
  assert.equal(constMatch[1], PC_CODE_MAP[50015])
  assert.equal(constMatch[1], MOBILE_CODE_MAP[50015])
})

test('PC 解析：50015 落到 code_map，不透传上游内部串 (#2598)', () => {
  const withInternal = resolvePcCore({ code: 50015, message: INTERNAL })
  assert.equal(withInternal.text, EXPECTED)
  assert.equal(withInternal.layer, 'code_map')
  assert.ok(!withInternal.text.includes('returned status'), '内部诊断不得回显')
  assert.ok(!withInternal.text.includes('请稍后再试'), '持久失败不得提示可重试')

  const codeOnly = resolvePcCore({ code: 50015 })
  assert.equal(codeOnly.text, EXPECTED)
  assert.equal(codeOnly.layer, 'code_map')

  // 中文内部诊断（5xx + CJK）同样走 code_map，不得作为 human_message 透传。
  const cjkInternal = resolvePcCore({ code: 50015, message: '身份服务调用失败：缺少 org:create 权限' })
  assert.equal(cjkInternal.text, EXPECTED)
  assert.equal(cjkInternal.layer, 'code_map')
})

test('mobile 解析：50015 落到 code_map，不透传上游内部串 (#2598)', () => {
  const withInternal = resolveMobileCore(
    { code: 50015, message: INTERNAL },
    { codeMap: MOBILE_CODE_MAP },
  )
  assert.equal(withInternal.text, EXPECTED)
  assert.equal(withInternal.layer, 'code_map')
  assert.ok(!withInternal.text.includes('returned status'))
  assert.ok(!withInternal.text.includes('请稍后再试'))
})

test('双端 50015 平价（同 fixture 同文本同层）(#2598)', () => {
  const payloads = [
    { code: 50015 },
    { code: 50015, message: INTERNAL },
    { code: 50015, message: '身份服务调用失败：缺少 org:create 权限' },
    { code: 50015, message: '' },
  ]
  for (const payload of payloads) {
    const pc = resolvePcCore(payload)
    const mobile = resolveMobileCore(payload, { codeMap: MOBILE_CODE_MAP })
    assert.equal(pc.text, mobile.text, `payload=${JSON.stringify(payload)}: 文本一致`)
    assert.equal(pc.layer, mobile.layer, `payload=${JSON.stringify(payload)}: 层一致`)
  }
})

test('负例：50015 不得映射为「数据冲突」语义 (#2598)', () => {
  const pc = resolvePcCore({ code: 50015, message: 'bind failed' })
  const mobile = resolveMobileCore({ code: 50015, message: 'bind failed' }, { codeMap: MOBILE_CODE_MAP })
  for (const got of [pc, mobile]) {
    assert.ok(!got.text.includes('数据冲突'), '50015 不是冲突语义')
    assert.ok(!got.text.includes('请稍后再试'), '50015 不可重试')
    assert.ok(got.text.includes('联系平台管理员'), '50015 必须指向平台管理员')
  }
})
