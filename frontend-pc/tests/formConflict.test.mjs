// #2480 v2 R7：业务可纠正冲突判定 / 文案的纯函数用例（node --test）。
//
// frontend-pc 无组件测试运行器（devDependencies 无 vitest/@testing-library），
// 因此把可判定逻辑收敛到 utils/formConflict.js，用 Node 内置测试运行器覆盖。
import test from 'node:test'
import assert from 'node:assert/strict'

import {
  BUSINESS_CORRECTABLE_CODES,
  conflictText,
  isBusinessCorrectableConflict,
} from '../src/utils/formConflict.js'

const bit = (v) => (v ? '1' : '0')

test('40900 / 40002 are business-correctable conflicts', () => {
  console.log(`TRUE_40900=${bit(isBusinessCorrectableConflict({ code: 40900 }))}`)
  console.log(`TRUE_40002=${bit(isBusinessCorrectableConflict({ code: 40002 }))}`)
  console.log(`TRUE_40004=${bit(isBusinessCorrectableConflict({ code: 40004 }))}`)
  console.log(`TRUE_40005=${bit(isBusinessCorrectableConflict({ code: 40005 }))}`)
  assert.equal(isBusinessCorrectableConflict({ code: 40900 }), true)
  assert.equal(isBusinessCorrectableConflict({ code: 40002 }), true)
  assert.equal(isBusinessCorrectableConflict({ code: 40004 }), true)
  assert.equal(isBusinessCorrectableConflict({ code: 40005 }), true)
})

test('server faults / success / null are not correctable conflicts', () => {
  console.log(`FALSE_50000=${bit(isBusinessCorrectableConflict({ code: 50000 }))}`)
  console.log(`FALSE_20000=${bit(isBusinessCorrectableConflict({ code: 20000 }))}`)
  console.log(`FALSE_NULL=${bit(isBusinessCorrectableConflict(null))}`)
  console.log(`FALSE_UNDEFINED=${bit(isBusinessCorrectableConflict(undefined))}`)
  assert.equal(isBusinessCorrectableConflict({ code: 50000 }), false)
  assert.equal(isBusinessCorrectableConflict({ code: 20000 }), false)
  assert.equal(isBusinessCorrectableConflict(null), false)
  assert.equal(isBusinessCorrectableConflict(undefined), false)
})

test('40901 (existing account, Modal guided) is excluded', () => {
  console.log(`FALSE_40901=${bit(isBusinessCorrectableConflict({ code: 40901 }))}`)
  console.log(`FALSE_40902=${bit(isBusinessCorrectableConflict({ code: 40902 }))}`)
  assert.equal(BUSINESS_CORRECTABLE_CODES.includes(40901), false)
  assert.equal(BUSINESS_CORRECTABLE_CODES.includes(40902), false)
  assert.equal(isBusinessCorrectableConflict({ code: 40901 }), false)
  assert.equal(isBusinessCorrectableConflict({ code: 40902 }), false)
})

test('conflictText prefers the backend message', () => {
  console.log(`TEXT_MESSAGE=${conflictText({ code: 40900, message: '网点名称已存在' })}`)
  assert.equal(conflictText({ code: 40900, message: '网点名称已存在' }), '网点名称已存在')
  assert.equal(conflictText({ code: 40900, message: '  网点名称已存在  ' }), '网点名称已存在')
})

test('conflictText falls back when message is empty or not a string', () => {
  console.log(`TEXT_FALLBACK=${conflictText({ code: 40900, message: '' })}`)
  console.log(`TEXT_OBJECT_FALLBACK=${conflictText({ code: 40900, message: { x: 1 } }, '请修改后重新提交')}`)
  assert.equal(conflictText({ code: 40900, message: '' }), '请修改后重新提交')
  assert.equal(conflictText({ code: 40900, message: '   ' }), '请修改后重新提交')
  assert.equal(conflictText({ code: 40900, message: { x: 1 } }), '请修改后重新提交')
  assert.equal(conflictText({ code: 40900, message: '' }, '网点名称已存在'), '网点名称已存在')
  assert.equal(conflictText(null), '请修改后重新提交')
})
