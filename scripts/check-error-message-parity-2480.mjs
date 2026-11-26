#!/usr/bin/env node
// #2480：PC ↔ mobile 错误消息解析平价（漂移检测，D5）。
//
// 两包无法共享模块（各自独立 map），因此用同一 fixture 表分别调用：
//   PC     → frontend-pc/src/services/errorMessages.js 的 resolveErrorMessage
//   mobile → frontend-mobile/src/services/errorMessageResolution.mjs 的 resolveErrorCore
// 只使用两端 map 文本一致/无依赖的 code（40303/40900/40901）与未知 code（49999）+ message 组合。
//
// mobile 的 ERROR_CODE_MAP 不能直接 import（errorMessages.js 依赖 ../i18n → JSON import），
// 因此用同源静态抽取 40303/40900/40901 文本，任何一侧改文案都会让平价失败。
//
// #2504：新增 40303（`access denied` 双端 → 「暂无权限访问该页面」），并断言 ASCII 机器串
// 在两端都落到 fallback（L4 收紧后的负例锚点）。
//
// #2560：新增 5 条 5xx fixture（2 条可操作固定文案 → human_message 透传；3 条内部诊断形态 →
// code_map/fallback 屏蔽）。5xx 判定改为「形态」后，双端必须逐字一致，故 parity 是主要防线。
//
// 输出（机器可判定）：PARITY=1 CASES=12 DIFFS=0 LAYER_DIFFS=0
// 用法：node scripts/check-error-message-parity-2480.mjs
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const HERE = path.dirname(fileURLToPath(import.meta.url))
const REPO_ROOT = path.resolve(HERE, '..')

const pcModule = await import(
  pathToFileURL(path.join(REPO_ROOT, 'frontend-pc/src/services/errorMessages.js')).href
)
const mobileModule = await import(
  pathToFileURL(path.join(REPO_ROOT, 'frontend-mobile/src/services/errorMessageResolution.mjs')).href
)

const MOBILE_SRC = path.join(REPO_ROOT, 'frontend-mobile/src/services/errorMessages.js')

function extractMapText(source, code) {
  const match = source.match(new RegExp(`\\b${code}:\\s*'([^']*)'`))
  return match ? match[1] : undefined
}

const mobileSource = fs.readFileSync(MOBILE_SRC, 'utf8')
const mobileCodeMap = {
  40303: extractMapText(mobileSource, 40303),
  40900: extractMapText(mobileSource, 40900),
  40901: extractMapText(mobileSource, 40901),
  // #2560：5xx 负例需要通用文案（内部诊断形态 → code_map）
  50000: extractMapText(mobileSource, 50000),
  50001: extractMapText(mobileSource, 50001),
}
for (const [code, text] of Object.entries(mobileCodeMap)) {
  if (typeof text !== 'string' || text === '') {
    console.error(`MAP_EXTRACT_FAIL code=${code}`)
    process.exit(1)
  }
}

const CASES = [
  // #2504：40303（权限范围不匹配）双端同文案；ASCII 机器串不得直出
  { id: '40303+access denied', input: { code: 40303, message: 'access denied' }, expectLayer: 'code_map' },
  { id: '40303+空串', input: { code: 40303, message: '' }, expectLayer: 'code_map' },
  { id: '40900+中文', input: { code: 40900, message: '网点名称已存在' } },
  { id: '40900+ASCII', input: { code: 40900, message: 'site name conflict' } },
  { id: '40900+空串', input: { code: 40900, message: '' } },
  { id: '40900+无message', input: { code: 40900 } },
  { id: '40901+中文', input: { code: 40901, message: '该用户已有待处理的邀请申请' } },
  { id: '40901+空白', input: { code: 40901, message: '   ' } },
  // #2504：收紧 L4 后的负例锚点（两端都必须 fallback）
  { id: '49999+ASCII', input: { code: 49999, message: 'unknown conflict' }, expectLayer: 'fallback' },
  { id: '49999+中文', input: { code: 49999, message: '未知冲突' } },
  { id: 'null', input: null },
  { id: '无code+空串', input: { message: '' } },
  // #2560：5xx 形态判定（2 正 3 负）——正例必须两端都透传原文，负例必须两端都屏蔽。
  {
    id: '5xx_50000_iam_rollback',
    input: { code: 50000, message: '切换网点失败且 IAM 回滚失败，请联系管理员人工核对 IAM 组织关系' },
    expectLayer: 'human_message',
  },
  {
    id: '5xx_50000_smtp',
    input: { code: 50000, message: 'SMTP 配置校验失败，请检查端口与凭据后重试' },
    expectLayer: 'human_message',
  },
  {
    id: '5xx_50000_pq',
    input: { code: 50000, message: '标记删除失败：pq: duplicate key value violates unique constraint' },
    expectLayer: 'code_map',
  },
  {
    id: '5xx_50001_rpc',
    input: { code: 50001, message: '级联解除商户成员失败：rpc error: connection refused' },
    expectLayer: 'code_map',
  },
  {
    id: '5xx_50999_timeout',
    input: { code: 50999, message: '内部诊断：dial tcp 10.0.0.1: i/o timeout' },
    expectLayer: 'fallback',
  },
]

let diffs = 0
let layerDiffs = 0
for (const testCase of CASES) {
  const pcText = pcModule.resolveErrorMessage(testCase.input)
  const mobile = mobileModule.resolveErrorCore(testCase.input, { codeMap: mobileCodeMap, messageMap: {} })
  const same = pcText === mobile.text
  if (!same) diffs += 1
  // #2504：ASCII 机器串不外溢——期望层序（若声明）必须两端一致
  const pcCore = pcModule.resolveErrorCore(testCase.input)
  if (pcCore.layer !== mobile.layer) layerDiffs += 1
  if (testCase.expectLayer && (mobile.layer !== testCase.expectLayer || pcCore.layer !== testCase.expectLayer)) {
    layerDiffs += 1
  }
  console.log(`CASE=${testCase.id} PARITY=${same ? 1 : 0} LAYER=${mobile.layer} PC_LAYER=${pcCore.layer} PC=${JSON.stringify(pcText)} MOBILE=${JSON.stringify(mobile.text)}`)
}

console.log(`CASES=${CASES.length}`)
console.log(`DIFFS=${diffs}`)
console.log(`LAYER_DIFFS=${layerDiffs}`)
console.log(`PARITY=${diffs === 0 && layerDiffs === 0 ? 1 : 0}`)
process.exit(diffs === 0 && layerDiffs === 0 ? 0 : 1)
