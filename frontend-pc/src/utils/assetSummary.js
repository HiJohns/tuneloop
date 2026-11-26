// #2538: 乐器详情页页头摘要（唯一详情页）。
//
// 背景：`/site/stock/:id` 曾渲染一份存根页（AssetDetail.jsx），只有 资产ID/类别/级别/
// 状态/网点/标准日租金，另有「流转轨迹」「关联单据」两个空占位。Owner 要求详情页
// 一屏可见标准日租金 / 逾期租金 / 全额押金 / 差额押金 / 估值 / 加入日期，并把入口
// 统一到 `/instruments/detail/:id`。本模块把摘要口径收敛成纯函数，页面只做渲染。
//
// 契约（与 backend/services/pricing.go 的 `FormatPricingResult` 同源，金额**一律为分**）：
//   - 标准日租金 = pricing-v2 `base_daily_rate`；缺失 → `-`
//   - 逾期租金   = pricing-v2 `overdue_daily_fee`；缺失 → `-`
//   - 全额押金   = pricing-v2 `deposit`（显式 0 有效）；缺失 → `-`
//   - 差额押金   = pricing-v2 `deposit_reduced`；缺失 → `未配置`
//   - 估值       = `instruments.total_price`（分）；缺失 → `-`
//   - 加入日期   = `instruments.created_at`（北京时间短格式）；缺失 → `-`

import { formatCents } from './money.js'
import { formatBeijingDateTimeShort } from './date.js'

function centsOrNull(value) {
  if (value === null || value === undefined || value === '') return null
  const cents = Number(value)
  return Number.isFinite(cents) ? cents : null
}

/**
 * 详情页页头摘要（固定 6 项，顺序固定）。
 *
 * @param {object|null} instrument GET /instruments/:id 的 data
 * @param {object|null} pricingV2  GET /instruments/:id/pricing-v2 的 data（可为空）
 * @returns {Array<{key: string, label: string, value: string}>}
 */
export function buildAssetSummary(instrument, pricingV2) {
  const inst = instrument || {}
  const pricing = pricingV2 || {}

  const dailyRate = centsOrNull(pricing.base_daily_rate)
  const overdue = centsOrNull(pricing.overdue_daily_fee)
  const deposit = centsOrNull(pricing.deposit)
  const depositReduced = centsOrNull(pricing.deposit_reduced)
  const totalPrice = centsOrNull(inst.total_price)

  return [
    {
      key: 'daily_rate',
      label: '标准日租金',
      value: dailyRate === null ? '-' : `¥${formatCents(dailyRate)}/天`,
    },
    {
      key: 'overdue_daily_fee',
      label: '逾期租金',
      value: overdue === null ? '-' : `¥${formatCents(overdue)}/天`,
    },
    {
      key: 'deposit',
      label: '全额押金',
      value: deposit === null ? '-' : `¥${formatCents(deposit)}`,
    },
    {
      key: 'deposit_reduced',
      label: '差额押金',
      value: depositReduced === null ? '未配置' : `¥${formatCents(depositReduced)}`,
    },
    {
      key: 'total_price',
      label: '估值',
      value: totalPrice === null ? '-' : `¥${formatCents(totalPrice)}`,
    },
    {
      key: 'created_at',
      label: '加入日期',
      value: formatBeijingDateTimeShort(inst.created_at),
    },
  ]
}
