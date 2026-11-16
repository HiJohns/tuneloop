// #2370 R1：validatePasswordStrength 5 例断言（唯一前端实现）。
import assert from 'node:assert'
import { validatePasswordStrength } from '../frontend-pc/src/utils/password.js'

const cases = [
  ['abc12345', false, '密码必须包含大写字母'],
  ['Abcd1234', true, ''],
  ['ABCD1234', false, '密码必须包含小写字母'],
  // Architect Rev.2 勘误：补「无数字」负例，覆盖四项规则各一条。
  ['AbcdEfgh', false, '密码必须包含数字'],
  ['Abcd', false, '密码长度不能少于 8 位'],
  // Architect Rev.2 裁定（D4）：空格/符号不限制，与后端 validatePassword 一致 → true。
  ['Abcd 1234', true, ''],
]

let passed = 0
for (const [input, expectedOk, expectedMsg] of cases) {
  const r = validatePasswordStrength(input)
  assert.strictEqual(r.ok, expectedOk, `${JSON.stringify(input)} ok`)
  if (!expectedOk) assert.strictEqual(r.message, expectedMsg, `${JSON.stringify(input)} message`)
  passed += 1
}
console.log(`${passed}/${cases.length} PASS`)
