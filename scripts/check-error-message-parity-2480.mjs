#!/usr/bin/env node
// #2480：PC ↔ mobile 错误消息解析平价（漂移检测，D5）。
//
// 两包无法共享模块（各自独立 map），因此用同一 fixture 表分别调用：
//   PC     → frontend-pc/src/services/errorMessages.js 的 resolveErrorMessage
//   mobile → frontend-mobile/src/services/errorMessageResolution.mjs 的 resolveErrorCore
// 只使用两端 map 文本一致/无依赖的 code（40900/40901）与未知 code（49999）+ message 组合。
//
// mobile 的 ERROR_CODE_MAP 不能直接 import（errorMessages.js 依赖 ../i18n → JSON import），
// 因此用同源静态抽取 40900/40901 文本，任何一侧改文案都会让平价失败。
//
// 输出（机器可判定）：PARITY=1 CASES=10 DIFFS=0
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
  40900: extractMapText(mobileSource, 40900),
  40901: extractMapText(mobileSource, 40901),
}
for (const [code, text] of Object.entries(mobileCodeMap)) {
  if (typeof text !== 'string' || text === '') {
    console.error(`MAP_EXTRACT_FAIL code=${code}`)
    process.exit(1)
  }
}

const CASES = [
  { id: '40900+中文', input: { code: 40900, message: '网点名称已存在' } },
  { id: '40900+ASCII', input: { code: 40900, message: 'site name conflict' } },
  { id: '40900+空串', input: { code: 40900, message: '' } },
  { id: '40900+无message', input: { code: 40900 } },
  { id: '40901+中文', input: { code: 40901, message: '该用户已有待处理的邀请申请' } },
  { id: '40901+空白', input: { code: 40901, message: '   ' } },
  { id: '49999+ASCII', input: { code: 49999, message: 'unknown conflict' } },
  { id: '49999+中文', input: { code: 49999, message: '未知冲突' } },
  { id: 'null', input: null },
  { id: '无code+空串', input: { message: '' } },
]

let diffs = 0
for (const testCase of CASES) {
  const pcText = pcModule.resolveErrorMessage(testCase.input)
  const mobile = mobileModule.resolveErrorCore(testCase.input, { codeMap: mobileCodeMap, messageMap: {} })
  const same = pcText === mobile.text
  if (!same) diffs += 1
  console.log(`CASE=${testCase.id} PARITY=${same ? 1 : 0} LAYER=${mobile.layer} PC=${JSON.stringify(pcText)} MOBILE=${JSON.stringify(mobile.text)}`)
}

console.log(`CASES=${CASES.length}`)
console.log(`DIFFS=${diffs}`)
console.log(`PARITY=${diffs === 0 ? 1 : 0}`)
process.exit(diffs === 0 ? 0 : 1)
