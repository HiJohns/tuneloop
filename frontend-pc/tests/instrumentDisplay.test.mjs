// #2528: instrumentDisplay 纯函数单测（`node --test`）。
// PC 端无组件测试运行器（frontend-pc/package.json 仅 dev/build/lint/preview），
// 按 Decision Lock D5 以纯函数 + 源码结构断言提供证据（evidence_type: static）。

import test from 'node:test'
import assert from 'node:assert/strict'

import {
  INSTRUMENT_IMAGE_PLACEHOLDER,
  instrumentImageSrc,
  formatDailyRate,
  formatDepositFull,
  formatDepositReduced,
  formatOverdueDailyFee,
} from '../src/utils/instrumentDisplay.js'

test('instrumentImageSrc prefers thumbnail over other sources', () => {
  const src = instrumentImageSrc({
    thumbnail: '/uploads/media/a_thumb.jpg',
    poster: '/uploads/media/poster.jpg',
    images: ['/uploads/media/legacy.jpg'],
  })
  assert.equal(src, '/uploads/media/a_thumb.jpg')
})

test('instrumentImageSrc falls back to poster when thumbnail is missing', () => {
  const src = instrumentImageSrc({
    thumbnail: '',
    poster: '/uploads/media/poster.jpg',
    images: ['/uploads/media/legacy.jpg'],
  })
  assert.equal(src, '/uploads/media/poster.jpg')
})

test('instrumentImageSrc supports legacy images array', () => {
  assert.equal(instrumentImageSrc({ images: ['/uploads/media/legacy.jpg'] }), '/uploads/media/legacy.jpg')
})

test('instrumentImageSrc supports legacy images JSON string', () => {
  assert.equal(
    instrumentImageSrc({ images: '["/uploads/media/legacy.jpg","/uploads/media/b.jpg"]' }),
    '/uploads/media/legacy.jpg'
  )
})

test('instrumentImageSrc returns built-in placeholder for empty images', () => {
  assert.equal(instrumentImageSrc({ images: '[]' }), INSTRUMENT_IMAGE_PLACEHOLDER)
  assert.ok(INSTRUMENT_IMAGE_PLACEHOLDER.startsWith('data:image/svg+xml'))
})

test('instrumentImageSrc degrades to placeholder on malformed images without throwing', () => {
  assert.equal(instrumentImageSrc({ images: 'not-json' }), INSTRUMENT_IMAGE_PLACEHOLDER)
  assert.equal(instrumentImageSrc({ images: { unexpected: true } }), INSTRUMENT_IMAGE_PLACEHOLDER)
  assert.equal(instrumentImageSrc(null), INSTRUMENT_IMAGE_PLACEHOLDER)
})

test('formatDailyRate converts resolved cents to yuan', () => {
  assert.equal(formatDailyRate({ resolved_daily_rate: 1000 }), '¥10.00')
})

test('formatDailyRate falls back to base_daily_rate when resolved is 0', () => {
  assert.equal(formatDailyRate({ resolved_daily_rate: 0, base_daily_rate: 1000 }), '¥10.00')
})

test('formatDailyRate shows dash when no rate can be resolved', () => {
  assert.equal(formatDailyRate({ resolved_daily_rate: 0 }), '-')
  assert.equal(formatDailyRate({}), '-')
  assert.equal(formatDailyRate({ resolved_daily_rate: -100 }), '-')
})

// #2537 D5: 押金拆成「全额押金」与「差额押金」两列，口径与详情页一致。
test('formatDepositFull renders cents as yuan', () => {
  assert.equal(formatDepositFull({ deposit: 50000 }), '¥500.00')
})

test('formatDepositFull keeps explicit zero deposit visible', () => {
  assert.equal(formatDepositFull({ deposit: 0 }), '¥0.00')
})

test('formatDepositFull shows dash for null or missing deposit', () => {
  assert.equal(formatDepositFull({ deposit: null }), '-')
  assert.equal(formatDepositFull({}), '-')
})

test('formatDepositReduced renders the difference tier', () => {
  assert.equal(formatDepositReduced({ deposit_reduced: 20000 }), '¥200.00')
})

test('formatDepositReduced keeps explicit zero difference visible', () => {
  assert.equal(formatDepositReduced({ deposit_reduced: 0 }), '¥0.00')
})

test('formatDepositReduced shows 未配置 for null or missing value', () => {
  assert.equal(formatDepositReduced({ deposit_reduced: null }), '未配置')
  assert.equal(formatDepositReduced({}), '未配置')
})

test('formatOverdueDailyFee renders cents as yuan', () => {
  assert.equal(formatOverdueDailyFee({ overdue_daily_fee: 3600 }), '¥36.00')
})

test('formatOverdueDailyFee shows dash for null, missing or non-positive value', () => {
  assert.equal(formatOverdueDailyFee({ overdue_daily_fee: null }), '-')
  assert.equal(formatOverdueDailyFee({}), '-')
  assert.equal(formatOverdueDailyFee({ overdue_daily_fee: 0 }), '-')
  assert.equal(formatOverdueDailyFee({ overdue_daily_fee: -100 }), '-')
})

test('formatOverdueDailyFee ignores non-numeric garbage without throwing', () => {
  assert.equal(formatOverdueDailyFee({ overdue_daily_fee: 'n/a' }), '-')
})
