// #2516 R1：API 结果终判用例（node --test）。
//
// frontend-pc 无组件测试运行器（devDependencies 无 vitest/@testing-library），
// 因此把可判定逻辑收敛到 utils/apiResult.js，用 Node 内置运行器覆盖。
//
// 单一 test 用例：node --test 的汇总固定为「# pass 1 / # fail 0」，
// 与本单 Evidence Contract R1 的断言（`^# pass 1$`）一致。
import test from 'node:test'
import assert from 'node:assert/strict'

import { isApiOk, apiFailureMessage } from '../src/utils/apiResult.js'

const bit = (v) => (v ? '1' : '0')

test('apiResult terminal contract: only 20000 is success, messages never render undefined', () => {
  // 20000 → 成功
  assert.equal(isApiOk({ code: 20000 }), true)
  console.log(`OK_20000=${bit(isApiOk({ code: 20000 }))}`)

  // 404/500 → 失败（request() 对 404 返回对象、不抛错，必须由终判拦截）
  assert.equal(isApiOk({ code: 40400, message: '未找到相关数据' }), false)
  assert.equal(isApiOk({ code: 50000 }), false)
  console.log(`FALSE_40400=${bit(!isApiOk({ code: 40400 }))}`)
  console.log(`FALSE_50000=${bit(!isApiOk({ code: 50000 }))}`)

  // 损坏输入：不抛错、一律判失败
  assert.equal(isApiOk({}), false)
  assert.equal(isApiOk(null), false)
  assert.equal(isApiOk(undefined), false)
  assert.equal(isApiOk('20000'), false)
  assert.equal(isApiOk({ code: '40400' }), false)
  assert.equal(isApiOk({ code: 'not-a-code' }), false)
  console.log(`FALSE_EMPTY=${bit(!isApiOk({}))}`)
  console.log(`FALSE_NULL=${bit(!isApiOk(null))}`)
  console.log(`FALSE_UNDEFINED=${bit(!isApiOk(undefined))}`)
  console.log(`FALSE_STRING_CODE=${bit(!isApiOk({ code: '40400' }))}`)
  console.log(`FALSE_GARBAGE_CODE=${bit(!isApiOk({ code: 'not-a-code' }))}`)

  // 提示文案：后端 message 优先
  assert.equal(apiFailureMessage({ code: 40400, message: '未找到相关数据' }), '未找到相关数据')
  assert.equal(apiFailureMessage({ code: 40400, message: '  未找到相关数据  ' }), '  未找到相关数据  ')
  console.log('MSG_PREFERS_BACKEND=1')

  // 空 / 非字符串 → 兜底，不得渲染 undefined
  assert.equal(apiFailureMessage({ code: 40400, message: '' }), '请求失败，请重试')
  assert.equal(apiFailureMessage({ code: 40400, message: '   ' }), '请求失败，请重试')
  assert.equal(apiFailureMessage({ code: 40400, message: 123 }), '请求失败，请重试')
  assert.equal(apiFailureMessage(null), '请求失败，请重试')
  assert.equal(apiFailureMessage({ code: 40400 }, '加载租户列表失败'), '加载租户列表失败')
  console.log('MSG_FALLBACK=1')
})
