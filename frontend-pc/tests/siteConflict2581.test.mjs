// #2581 R3：网点重名 409 的「传输层 → rubric → 文案」链路回归（node --test）。
//
// 背景：CreateSite 曾返回 ASCII 英文 message（`organization name already exists: ...`），
// 被 resolveErrorMessage 的「含 CJK 原样透传」口径判为「无具体原因」→ 落 40900 通用文案
// 「数据冲突，请刷新后重试」。本用例把三段口径钉死：
//   1. 409 + 中文具体原因 → kind=result，message 逐字透传（不被 ERROR_CODE_MAP[40900] 遮蔽），
//      共享 rubric 判为业务可纠正冲突，conflictText 原样返回；
//   2. 负例：409 + 英文 message（旧实现形态）→ 落通用文案（缺陷的用户可见形态，红样保留）；
//   3. catch 路径：上游抛出的 ApiError（带 code=40900）同样命中 rubric（#2563 语义不回退）。
import test from 'node:test'
import assert from 'node:assert/strict'

import { resolveHttpFailure } from '../src/services/apiError.js'
import { conflictText, isBusinessCorrectableConflict } from '../src/utils/formConflict.js'

const bit = (v) => (v ? '1' : '0')

const DUP_MSG = '该商户下已存在同名网点「海淀店」，请换一个名称'
const GENERIC_CONFLICT = '数据冲突，请刷新后重试'
const LEGACY_ENGLISH_MSG = 'organization name already exists: {"error":"organization name already exists"}'

test('409 + CJK message: passthrough, rubric hits, conflictText keeps the reason', () => {
  const out = resolveHttpFailure(409, { code: 40900, message: DUP_MSG })

  console.log(`CHAIN_KIND=${out.kind}`)
  console.log(`CHAIN_CODE=${out.value.code}`)
  console.log(`CHAIN_MSG_CJK=${bit(/[\u4e00-\u9fff]/.test(out.value.message))}`)
  console.log(`CHAIN_TEXT_MATCH=${bit(out.value.message === DUP_MSG)}`)
  console.log(`CHAIN_RUBRIC=${bit(isBusinessCorrectableConflict(out.value))}`)
  console.log(`CHAIN_CONFLICT_TEXT=${conflictText(out.value, '网点名称已存在')}`)

  assert.equal(out.kind, 'result', '409 必须返回对象，不得抛异常')
  assert.equal(out.value.code, 40900)
  assert.equal(out.value.message, DUP_MSG, '中文具体原因必须逐字透传，不得被 40900 通用文案遮蔽')
  assert.equal(out.value.message === GENERIC_CONFLICT, false)
  assert.equal(isBusinessCorrectableConflict(out.value), true)
  assert.equal(conflictText(out.value, '网点名称已存在'), DUP_MSG)
})

test('negative: 409 + ASCII message still degrades to the generic conflict text', () => {
  const out = resolveHttpFailure(409, { code: 40900, message: LEGACY_ENGLISH_MSG })

  console.log(`NEG_ENGLISH_KIND=${out.kind}`)
  console.log(`NEG_ENGLISH_GENERIC=${bit(out.value.message === GENERIC_CONFLICT)}`)

  assert.equal(out.kind, 'result')
  assert.equal(out.value.code, 40900)
  assert.equal(out.value.message, GENERIC_CONFLICT, '英文 message 不得外溢（这正是 #2581 的缺陷形态）')
})

test('catch path: ApiError carrying 40900 still hits the shared rubric', () => {
  // #2563：400/403/429/5xx 抛 Error 并把 code 挂到 Error 上；页面 catch 分支据此判定。
  const thrown = resolveHttpFailure(400, { code: 40900, message: DUP_MSG })
  assert.equal(thrown.kind, 'throw', '400 族仍抛异常（#2563 语义不变）')

  console.log(`CATCH_KIND=${thrown.kind}`)
  console.log(`CATCH_RUBRIC=${bit(isBusinessCorrectableConflict(thrown.value))}`)
  console.log(`CATCH_TEXT_MATCH=${bit(conflictText(thrown.value, '请修改后重新提交') === DUP_MSG)}`)

  assert.ok(thrown.value instanceof Error)
  assert.equal(thrown.value.code, 40900)
  assert.equal(isBusinessCorrectableConflict(thrown.value), true)
  assert.equal(conflictText(thrown.value, '请修改后重新提交'), DUP_MSG)
})
