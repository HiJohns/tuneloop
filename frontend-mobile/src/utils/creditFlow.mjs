// #2327 S4: 支付分信用评测的纯逻辑（两端共用，禁止把业务规则写进页面）。
//
// 约定：
//   - 押金档位（#2352 Owner 口径）：
//       full>0  → full（无需评测/0 担保）+ reduced（需信用 + 2 担保人）；**不提供 zero**
//       full==0 → 仅 zero（需信用评测，0 担保人）
//   - payscore 路径下单必须显式传 deposit_credit_path='payscore'

export const DEPOSIT_TIER = {
  FULL: 'full',
  REDUCED: 'reduced',
  ZERO: 'zero',
}

export const CREDIT_STATE = {
  NONE: '',
  PENDING: 'pending',
  APPROVED: 'approved',
  REJECTED: 'rejected',
  EXPIRED: 'expired',
}

const SETTLED = [CREDIT_STATE.APPROVED, CREDIT_STATE.REJECTED, CREDIT_STATE.EXPIRED]

export function isCreditSettled(state) {
  return SETTLED.includes(state || '')
}

// buildTierOptions 返回结算页可展示的档位（金额单位：元，与页面 pricing 同口径）。
// requiresAssessment=false 仅用于全额档；zero（乐器本身零押金）也需要信用评测（#2352）。
export function buildTierOptions(depositYuan, depositReducedYuan) {
  const full = Number(depositYuan) || 0
  const reduced = depositReducedYuan == null ? null : Number(depositReducedYuan)
  if (full <= 0) {
    return [{
      tier: DEPOSIT_TIER.ZERO,
      amount: 0,
      requiresGuarantors: false,
      requiresAssessment: true,
      reason: 'instrument_zero_deposit',
    }]
  }
  const options = [{
    tier: DEPOSIT_TIER.FULL,
    amount: full,
    requiresGuarantors: false,
    requiresAssessment: false,
  }]
  if (reduced != null && reduced >= 0 && reduced < full) {
    options.push({
      tier: DEPOSIT_TIER.REDUCED,
      amount: reduced,
      requiresGuarantors: true,
      requiresAssessment: true,
    })
  }
  // #2352 D1: full>0 不得出现 zero 档（不能只凭信用免掉全部押金）。
  return options
}

// buildOrderDepositPayload 生成下单押金字段（payscore 路径）。
// reduced 必须至少 2 位担保人；zero/full 不要求担保人。
export function buildOrderDepositPayload({ tier, guarantorIds = [] }) {
  const t = tier || DEPOSIT_TIER.FULL
  if (!Object.values(DEPOSIT_TIER).includes(t)) {
    return { ok: false, error: 'invalid_deposit_tier' }
  }
  if (t === DEPOSIT_TIER.REDUCED) {
    const ids = (guarantorIds || []).filter(Boolean)
    if (ids.length < 2) {
      return { ok: false, error: 'guarantors_required' }
    }
    return {
      ok: true,
      body: {
        deposit_tier: t,
        deposit_credit_path: 'payscore',
        guarantor_ids: ids,
      },
    }
  }
  return {
    ok: true,
    body: {
      deposit_tier: t,
      deposit_credit_path: t === DEPOSIT_TIER.ZERO ? 'payscore' : 'internal',
    },
  }
}

// creditGate 把 credit-status 响应映射为支付页展示/禁用状态（D5）。
export function creditGate(payload) {
  const state = payload?.credit_state || CREDIT_STATE.NONE
  const requires = !!payload?.requires_assessment
  if (!requires) {
    return { canPay: true, showEntry: false, state, reason: '', action: 'none' }
  }
  switch (state) {
    case CREDIT_STATE.APPROVED:
      return { canPay: true, showEntry: false, state, reason: '', action: 'none' }
    case CREDIT_STATE.PENDING:
      return {
        canPay: false, showEntry: true, state,
        reason: '信用评测进行中，完成后即可支付',
        action: 'assessment',
      }
    case CREDIT_STATE.REJECTED:
      return {
        canPay: false, showEntry: true, state,
        reason: '信用评测未通过，请改用全额押金或更换乐器',
        action: 'fallback_full',
      }
    case CREDIT_STATE.EXPIRED:
      return {
        canPay: false, showEntry: true, state,
        reason: '信用评测已过期，请重新评测',
        action: 'assessment',
      }
    default:
      return {
        canPay: false, showEntry: true, state,
        reason: '该押金档位需要先完成微信信用评测',
        action: 'assessment',
      }
  }
}

// resolveCreditLaunch 决定评测页如何拉起微信（D0/D6/D9）。
//   mock             → 模拟态，不拉起微信（仅 dev/test）
//   missing-package  → 无 package，禁止拉起（不得伪造成功）
//   weapp            → 小程序 wx.openBusinessView
//   h5               → 微信内 H5 jweixin.openBusinessView（需 APIv2 密钥灰度）
//   unsupported      → 微信外浏览器，展示「请在微信小程序完成评估」
export function resolveCreditLaunch({ mock, creditPackage, isMiniProgram, isWechatBrowser }) {
  if (mock) return { mode: 'mock', canLaunch: false }
  if (!creditPackage) return { mode: 'missing-package', canLaunch: false }
  if (isMiniProgram) return { mode: 'weapp', canLaunch: true }
  if (isWechatBrowser) return { mode: 'h5', canLaunch: true }
  return { mode: 'unsupported', canLaunch: false }
}

// buildAssessmentRoute 生成评测页地址（一律 query order_id，#1674）。
export function buildAssessmentRoute({ orderId, tier, riskAmountCents, returnPath }) {
  const params = new URLSearchParams()
  params.set('order_id', orderId || '')
  if (tier) params.set('tier', tier)
  if (riskAmountCents != null) params.set('risk_amount_cents', String(riskAmountCents))
  if (returnPath) params.set('return_path', returnPath)
  return `/credit-assessment?${params.toString()}`
}

// pollDelay 轮询退避：1.5s 起，最长 8s（避免打爆 status 接口）。
export function pollDelay(attempt) {
  const base = 1500 * Math.pow(2, Math.max(0, attempt || 0))
  return Math.min(base, 8000)
}
