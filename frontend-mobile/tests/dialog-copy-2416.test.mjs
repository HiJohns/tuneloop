// #2416 R4「弹窗重签」的自定义文案静态守卫（node --test）。
//
// 背景：真机回执里有两组「自定义按钮文案」，要构造失败态才能点到（弱网上传视频 / 证件照上传失败），
// 人工成本高。这里用**源码断言**锁死文案不被改写——i18n 的 t() 以中文源串为 key、缺失即回退原串
// （src/i18n/index.js），因此源码里字面量即为最终展示文案（若 locales/zh-CN.json 覆盖，必须覆盖成同值）。
//
// 人工回执只保留「弹窗可见 + 可滚 + 深色不反色」，不再依赖构造失败态。
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const src = (p) => readFileSync(join(here, '..', p), 'utf8')

const faceVerify = src('src/pages/FaceVerify.jsx')
const profileComplete = src('src/pages-weapp/profile-complete/ProfileComplete.jsx')
const zhCN = JSON.parse(src('src/i18n/locales/zh-CN.json'))

const bit = (v) => (v ? '1' : '0')

// 文案在源码里逐字存在，且绑定在 confirmText/cancelText 上（不是别处的普通文本）
function boundText(file, key, value) {
  const re = new RegExp(`${key}:\\s*t\\(\\s*['"]${value}['"]\\s*\\)`)
  return re.test(file)
}

// 若 i18n 字典覆盖了该 key，覆盖值必须与原串相同（防止「同 key 换了文案」）
function dictKeeps(key, value) {
  return zhCN[key] === undefined || zhCN[key] === value
}

test('维修流程：重试视频 / 仅提交照片 逐字绑定在弹窗按钮上', () => {
  const retry = boundText(faceVerify, 'confirmText', '重试视频')
  const photoOnly = boundText(faceVerify, 'cancelText', '仅提交照片')
  console.log(`COPY_RETRY_VIDEO_BOUND=${bit(retry)}`)
  console.log(`COPY_PHOTO_ONLY_BOUND=${bit(photoOnly)}`)
  console.log(`COPY_RETRY_VIDEO_DICT_OK=${bit(dictKeeps('重试视频', '重试视频'))}`)
  console.log(`COPY_PHOTO_ONLY_DICT_OK=${bit(dictKeeps('仅提交照片', '仅提交照片'))}`)
  assert.equal(retry, true, 'confirmText 必须是 t(\'重试视频\')')
  assert.equal(photoOnly, true, 'cancelText 必须是 t(\'仅提交照片\')')
  assert.equal(dictKeeps('重试视频', '重试视频'), true)
  assert.equal(dictKeeps('仅提交照片', '仅提交照片'), true)
})

test('支付分支：继续支付 / 重试 逐字绑定在弹窗按钮上', () => {
  const goOn = boundText(profileComplete, 'confirmText', '继续支付')
  const retry = boundText(profileComplete, 'cancelText', '重试')
  console.log(`COPY_CONTINUE_PAY_BOUND=${bit(goOn)}`)
  console.log(`COPY_RETRY_PAY_BOUND=${bit(retry)}`)
  console.log(`COPY_CONTINUE_PAY_DICT_OK=${bit(dictKeeps('继续支付', '继续支付'))}`)
  console.log(`COPY_RETRY_DICT_OK=${bit(dictKeeps('重试', '重试'))}`)
  assert.equal(goOn, true, 'confirmText 必须是 t(\'继续支付\')')
  assert.equal(retry, true, 'cancelText 必须是 t(\'重试\')')
})

test('这些弹窗走统一 dialog 出口（不是直连 Taro.showModal 的自绘分支）', () => {
  // #2416 S2：维修/支付两处必须使用 dialog.showDialog（统一组件），否则真机会出现不可见弹窗
  const usesDialog = /dialog\.showDialog\(/.test(faceVerify) && /dialog\.showDialog\(/.test(profileComplete)
  console.log(`COPY_USES_UNIFIED_DIALOG=${bit(usesDialog)}`)
  assert.equal(usesDialog, true)
})
