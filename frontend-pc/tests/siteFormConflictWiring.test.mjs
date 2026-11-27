// #2569 R14 接线守卫：网点表单的 400/40002 分支必须落到「表单内常驻提示」。
//
// 背景：预生产手工复验第 14 项时，用 DevTools「Edit and Resend」重放请求——那条请求
// 不经过应用 JS，所以界面必然没有反应（方法本身错误，不是代码缺陷）。
// 正确的手工验证法是「在 Console 里拦截出站请求、仍然点击页面按钮提交」
// （见 docs/ops/verify-1.0.102-runbook.md 第 14 项）。
//
// 本守卫锁死代码侧接线：apiError 抛出的带 code Error 必须被 catch 里的 rubric 捕获，
// 并写入 formConflict（由 FormConflictAlert 呈现，testId=site-form-conflict）。
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const page = readFileSync(join(here, '../src/pages/SiteManagement.jsx'), 'utf8')
const apiError = readFileSync(join(here, '../src/services/apiError.js'), 'utf8')

const bit = (v) => (v ? '1' : '0')

test('handleSubmit catch 用 rubric 判定并把文案写进 formConflict', () => {
  // 定位 handleSubmit 函数体，再取其中的 catch 段（文件里还有多个 catch，不能取第一个）
  const submitStart = page.indexOf('const handleSubmit = async () => {')
  assert.notEqual(submitStart, -1, 'SiteManagement 必须有 handleSubmit')
  const catchStart = page.indexOf('} catch (err) {', submitStart)
  assert.notEqual(catchStart, -1, 'handleSubmit 必须有自己的 catch 分支')
  const catchEnd = page.indexOf('} finally', catchStart)
  assert.notEqual(catchEnd, -1, 'handleSubmit 的 catch 之后必须有 finally')
  const catchBlock = page.slice(catchStart, catchEnd)

  const usesRubric = catchBlock.includes('isBusinessCorrectableConflict(err)')
  const setsConflict = catchBlock.includes('setFormConflict(conflictText(err')
  console.log(`SITE_CATCH_USES_RUBRIC=${bit(usesRubric)}`)
  console.log(`SITE_CATCH_SETS_FORM_CONFLICT=${bit(setsConflict)}`)
  assert.equal(usesRubric, true, 'catch 必须用 isBusinessCorrectableConflict(err) 判定 400 族')
  assert.equal(setsConflict, true, 'catch 必须把消息写入 formConflict（常驻），而不是 message.error toast')
})

test('FormConflictAlert 以 site-form-conflict 渲染（与复验口径一致）', () => {
  const wired = page.includes('testId="site-form-conflict"')
  console.log(`SITE_ALERT_MOUNTED=${bit(wired)}`)
  assert.equal(wired, true)
})

test('400 族在传输层是「抛错并带 code」，因此能进入 catch', () => {
  // 400 返回 kind='throw'（apiError.js 契约）——否则 catch 分支不可达，常驻提示无从触发。
  const throws400 = /status === 4\d\d|resolveHttpFailure/.test(apiError)
  assert.equal(throws400, true)
  const comment = apiError.includes('其余非 2xx（400 / 403 / 429 / 5xx）→ 抛 Error')
  console.log(`API_ERROR_400_THROWS=${bit(comment)}`)
  assert.equal(comment, true, 'apiError 必须仍把 400 族作为抛错处理（#2563 契约）')
})
