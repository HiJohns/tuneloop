// #2529: 资产详情页（/site/stock/:id）的取数与展示纯函数。
//
// 背景：详情页原先用 `inventoryApi.list()`（无参 = 服务端默认第 1 页 20 条）
// 取回首页快照后 `find(id)`，导致第 2 页之后的乐器一律显示「资产不存在」；
// 且网络失败与「不存在」同表现（都 setAsset(null)）。本模块把「响应 → 页面状态」
// 与「详情图取源」收敛成可单测的纯函数，页面只负责渲染分支。
//
// 契约：
//   - assetDetailState(resp) → { status: 'ok', asset } | { status: 'missing' } | { status: 'error', message }
//       code 20000 且有 data → ok；code 40400 或 data 为空 → missing；其它/异常 → error
//   - assetHeroImage(asset) → 复用 #2528 `instrumentImageSrc`：
//       media.display[0].url（展示图）→ poster → images[0] → 内置占位

import { instrumentImageSrc } from './instrumentDisplay.js'

export const ASSET_DETAIL_STATUS = {
  OK: 'ok',
  MISSING: 'missing',
  ERROR: 'error',
}

/**
 * 把 GET /instruments/:id 的响应映射为页面三态。
 *
 * @param {{code?: number, data?: object|null, message?: string}} resp
 * @returns {{status: string, asset?: object, message?: string}}
 */
export function assetDetailState(resp) {
  if (!resp || Array.isArray(resp)) {
    return { status: ASSET_DETAIL_STATUS.MISSING }
  }

  const { code, data, message } = resp

  if (code === 40400) {
    return { status: ASSET_DETAIL_STATUS.MISSING }
  }
  if (code === 20000) {
    return data ? { status: ASSET_DETAIL_STATUS.OK, asset: data } : { status: ASSET_DETAIL_STATUS.MISSING }
  }
  return {
    status: ASSET_DETAIL_STATUS.ERROR,
    message: message || '请求失败，请重试',
  }
}

/**
 * 详情页头图取源（与 #2528 列表同源）：
 * media.display[0].url → poster → images[0] → 内置 data-URI 占位。
 *
 * @param {object} asset
 * @returns {string}
 */
export function assetHeroImage(asset) {
  const display = asset && asset.media && Array.isArray(asset.media.display) ? asset.media.display : []
  const displayUrl = display.length > 0 && display[0] && typeof display[0].url === 'string' ? display[0].url : ''
  return instrumentImageSrc({
    thumbnail: displayUrl,
    poster: asset ? asset.poster : '',
    images: asset ? asset.images : undefined,
  })
}
