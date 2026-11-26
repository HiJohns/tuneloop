// #2563 R1：PC 客户端「非 2xx 失败」映射的纯函数回归（node --test）。
//
// 口径：404/409 返回对象（与改动前逐字一致）；其余非 2xx 抛 Error 且带 code/status/data。
// 输出 token 供 scripts/verify-2563.sh r1 做机器断言（同时保留 assert 作为真值）。
import test from 'node:test'
import assert from 'node:assert/strict'

import { resolveHttpFailure } from '../src/services/apiError.js'
import { isBusinessCorrectableConflict } from '../src/utils/formConflict.js'

const bit = (v) => (v ? '1' : '0')

test('400/403/429/500/502 throw an Error that keeps the business code', () => {
  const cases = [
    [400, { code: 40002, message: '分类名称已存在' }, 'THROW_40002', 40002],
    [400, { code: 40004 }, 'THROW_40004', 40004],
    [403, { code: 40300, message: 'access denied' }, 'THROW_403', 40300],
    [429, { code: 42900 }, 'THROW_429', 42900],
    [500, { code: 50000, message: 'db down' }, 'THROW_500', 50000],
  ]
  for (const [status, payload, token, expectedCode] of cases) {
    const out = resolveHttpFailure(status, payload)
    console.log(`${token}=${bit(out.kind === 'throw' && out.value instanceof Error && out.value.code === expectedCode)}`)
    assert.equal(out.kind, 'throw', `status ${status} must throw`)
    assert.ok(out.value instanceof Error)
    assert.equal(out.value.code, expectedCode)
    assert.equal(out.value.status, status)
  }

  // 502 + null payload：损坏输入也必须抛带 status 的 Error（不解析异常）
  const bad = resolveHttpFailure(502, null)
  console.log(`THROW_502=${bit(bad.kind === 'throw' && bad.value.status === 502)}`)
  assert.equal(bad.kind, 'throw')
  assert.equal(bad.value.status, 502)
  assert.equal(bad.value.message, '请求失败，请重试')
})

test('thrown Error carries data and keeps the backend CJK message', () => {
  const out = resolveHttpFailure(400, { code: 40002, message: '分类名称已存在', data: { name: 'x' } })
  console.log(`MSG_40002=${out.value.message === '分类名称已存在' ? 'CJK_PASSTHROUGH' : 'TRANSLATED'}`)
  assert.equal(out.value.message, '分类名称已存在')
  assert.deepEqual(out.value.data, { name: 'x' })
})

test('404 / 409 still return result objects (unchanged contract)', () => {
  const notFound = resolveHttpFailure(404, { message: 'not found' })
  console.log(`RESULT_404=${bit(notFound.kind === 'result' && notFound.value.code === 40400 && notFound.value.status === 404)}`)
  assert.equal(notFound.kind, 'result')
  assert.equal(notFound.value.code, 40400)
  assert.equal(notFound.value.status, 404)
  assert.equal(notFound.value.data, null)

  const conflict = resolveHttpFailure(409, { code: 40901, message: 'name exists' })
  console.log(`RESULT_409=${bit(conflict.kind === 'result' && conflict.value.code === 40901)}`)
  assert.equal(conflict.kind, 'result')
  assert.equal(conflict.value.code, 40901)
  assert.equal(conflict.value.status, 409)

  const conflictDefault = resolveHttpFailure(409, {})
  console.log(`RESULT_409_DEFAULT_CODE=${conflictDefault.value.code}`)
  assert.equal(conflictDefault.value.code, 40900)
})

test('numeric string code is coerced; malformed payload is safe', () => {
  const coerced = resolveHttpFailure(400, { code: '40002', message: '分类名称已存在' })
  console.log(`CODE_STRING_COERCED=${bit(coerced.value.code === 40002)}`)
  assert.equal(coerced.value.code, 40002)

  const garbage = [resolveHttpFailure(400, 'oops'), resolveHttpFailure(400, [1, 2]), resolveHttpFailure(400, null)]
  const safe = garbage.every((o) => o.kind === 'throw' && o.value.code === undefined && o.value.message === '请求失败，请重试')
  console.log(`BAD_PAYLOAD_SAFE=${bit(safe)}`)
  assert.ok(safe)
})

test('the enriched 40002 Error is a business-correctable conflict, a plain Error is not', () => {
  const enriched = resolveHttpFailure(400, { code: 40002, message: '分类名称已存在' }).value
  const plain = new Error('boom')
  console.log(`R1_CONFLICT_RUBRIC=${bit(isBusinessCorrectableConflict(enriched) === true && isBusinessCorrectableConflict(plain) === false)}`)
  assert.equal(isBusinessCorrectableConflict(enriched), true)
  assert.equal(isBusinessCorrectableConflict(plain), false)
})
