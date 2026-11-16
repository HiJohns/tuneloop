// #2370 R1：validatePasswordStrength 5 例断言（唯一前端实现）。
import assert from 'node:assert'
import { validatePasswordStrength } from '../frontend-pc/src/utils/password.js'

const cases = [
  ['abc12345', false, '密码必须包含大写字母'],
  ['Abcd1234', true, ''],
  ['ABCD1234', false, '密码必须包含小写字母'],
  ['Abcd', false, '密码长度不能少于 8 位'],
  // 计划 R1 将本例写成 false；但后端 validatePassword 无空格限制（仅查长度/大小写/数字），
  // 实测后端接受该密码 → 按语义应为 true，已回 Architect 裁定（见 Issue 评论）。
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
