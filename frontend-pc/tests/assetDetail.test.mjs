// #2529: 资产详情页纯函数测试（node --test，无 DOM）。
//
//   node --test tests/assetDetail.test.mjs

import test from 'node:test'
import assert from 'node:assert/strict'

import { ASSET_DETAIL_STATUS, assetDetailState, assetHeroImage } from '../src/utils/assetDetail.js'
import { INSTRUMENT_IMAGE_PLACEHOLDER } from '../src/utils/instrumentDisplay.js'

test('assetDetailState returns ok for a 20000 response with data', () => {
  const asset = { id: 'abc', sn: 'CVZ-104德国' }
  const state = assetDetailState({ code: 20000, data: asset })
  assert.equal(state.status, ASSET_DETAIL_STATUS.OK)
  assert.equal(state.asset.id, 'abc')
})

test('assetDetailState returns missing for 40400 (unknown id)', () => {
  const state = assetDetailState({ code: 40400, data: null, message: 'instrument not found' })
  assert.equal(state.status, ASSET_DETAIL_STATUS.MISSING)
})

test('assetDetailState returns missing for 20000 without data', () => {
  const state = assetDetailState({ code: 20000, data: null })
  assert.equal(state.status, ASSET_DETAIL_STATUS.MISSING)
})

test('assetDetailState returns error with message for other codes', () => {
  const state = assetDetailState({ code: 50000, message: 'failed to fetch instrument: boom' })
  assert.equal(state.status, ASSET_DETAIL_STATUS.ERROR)
  assert.match(state.message, /boom/)
})

test('assetDetailState returns missing for null/array payloads', () => {
  assert.equal(assetDetailState(null).status, ASSET_DETAIL_STATUS.MISSING)
  assert.equal(assetDetailState([]).status, ASSET_DETAIL_STATUS.MISSING)
})

test('assetHeroImage prefers media.display[0].url', () => {
  const src = assetHeroImage({
    media: { display: [{ url: '/uploads/media/display-1.jpg' }] },
    poster: '/uploads/poster.jpg',
    images: ['/uploads/legacy.jpg'],
  })
  assert.equal(src, '/uploads/media/display-1.jpg')
})

test('assetHeroImage falls back to poster when media is empty', () => {
  const src = assetHeroImage({ media: { display: [] }, poster: '/uploads/poster.jpg', images: ['/uploads/legacy.jpg'] })
  assert.equal(src, '/uploads/poster.jpg')
})

test('assetHeroImage falls back to legacy images[0]', () => {
  const src = assetHeroImage({ media: { display: [] }, poster: '', images: ['/uploads/legacy.jpg'] })
  assert.equal(src, '/uploads/legacy.jpg')
})

test('assetHeroImage returns built-in placeholder when nothing is available', () => {
  assert.equal(assetHeroImage({}), INSTRUMENT_IMAGE_PLACEHOLDER)
  assert.equal(assetHeroImage(null), INSTRUMENT_IMAGE_PLACEHOLDER)
})
