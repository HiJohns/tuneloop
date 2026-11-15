import test from 'node:test'
import assert from 'node:assert/strict'

import {
  DEPOSIT_TIER,
  CREDIT_STATE,
  buildTierOptions,
  buildOrderDepositPayload,
  creditGate,
  resolveCreditLaunch,
  buildAssessmentRoute,
  pollDelay,
  isCreditSettled,
} from './creditFlow.mjs'

test('档位映射（#2352）：full>0 只有 full/reduced，且都需评测通过', () => {
  const opts = buildTierOptions(500, 200)
  // #2352 D1：全额押金>0 时不得出现 zero 档（不能凭信用免掉全部押金）
  assert.deepEqual(opts.map(o => o.tier), ['full', 'reduced'])
  assert.ok(!opts.some(o => o.tier === DEPOSIT_TIER.ZERO), 'full>0 不得出现零押金档')
  const full = opts.find(o => o.tier === DEPOSIT_TIER.FULL)
  const reduced = opts.find(o => o.tier === DEPOSIT_TIER.REDUCED)
  // #2352 F2：全额档同样要求评测通过
  assert.equal(full.requiresAssessment, true)
  assert.equal(reduced.requiresGuarantors, true)
  assert.equal(reduced.requiresAssessment, true)
  // 负例：full>0 时 zero 档必须缺席（#2352 D1，不能凭信用免掉全部押金）
  assert.equal(opts.find(o => o.tier === DEPOSIT_TIER.ZERO), undefined)
})

test('乐器本身零押金：只有 zero，且仍必须过评测', () => {
  const opts = buildTierOptions(0, null)
  assert.equal(opts.length, 1)
  assert.equal(opts[0].tier, DEPOSIT_TIER.ZERO)
  // #2352 D1：评测对所有订单必经（含零押金乐器），故这里恒为 true
  assert.equal(opts[0].requiresAssessment, true)
})

test('deposit_reduced = 0 视为未配置（#2357 修正）', () => {
  // 历史数据常以 0 表示「没有配置第二档」，不得据此提供「零差额免押」
  const opts = buildTierOptions(6000, 0)
  assert.deepEqual(opts.map(o => o.tier), [DEPOSIT_TIER.FULL])
})

test('reduced 缺少担保人时禁止下单', () => {
  const bad = buildOrderDepositPayload({ tier: 'reduced', guarantorIds: ['g1'] })
  assert.equal(bad.ok, false)
  assert.equal(bad.error, 'guarantors_required')
  const ok = buildOrderDepositPayload({ tier: 'reduced', guarantorIds: ['g1', 'g2'] })
  assert.equal(ok.ok, true)
  assert.equal(ok.body.deposit_credit_path, 'payscore')
  assert.deepEqual(ok.body.guarantor_ids, ['g1', 'g2'])
})

test('payscore 路径显式传递，full 走 internal', () => {
  const zero = buildOrderDepositPayload({ tier: 'zero' })
  assert.equal(zero.body.deposit_credit_path, 'payscore')
  const full = buildOrderDepositPayload({ tier: 'full' })
  assert.equal(full.body.deposit_credit_path, 'internal')
})

test('creditGate：四态 → 可支付/入口/原因', () => {
  const pending = creditGate({ requires_assessment: true, credit_state: CREDIT_STATE.PENDING })
  assert.equal(pending.canPay, false)
  assert.equal(pending.showEntry, true)
  assert.equal(pending.action, 'assessment')

  const approved = creditGate({ requires_assessment: true, credit_state: CREDIT_STATE.APPROVED })
  assert.equal(approved.canPay, true)
  assert.equal(approved.showEntry, false)

  const rejected = creditGate({ requires_assessment: true, credit_state: CREDIT_STATE.REJECTED })
  assert.equal(rejected.canPay, false)
  assert.equal(rejected.action, 'blocked')

  const expired = creditGate({ requires_assessment: true, credit_state: CREDIT_STATE.EXPIRED })
  assert.equal(expired.canPay, false)
  assert.equal(expired.action, 'assessment')

  const notRequired = creditGate({ requires_assessment: false, credit_state: '' })
  assert.equal(notRequired.canPay, true, '乐器零押金/全额档不得被拦截')
})

test('resolveCreditLaunch：mock / 缺 package / weapp / h5 / 微信外', () => {
  assert.equal(resolveCreditLaunch({ mock: true, creditPackage: 'p', isMiniProgram: true }).mode, 'mock')
  assert.equal(resolveCreditLaunch({ mock: false, creditPackage: '', isMiniProgram: true }).mode, 'missing-package')
  const weapp = resolveCreditLaunch({ mock: false, creditPackage: 'pkg', isMiniProgram: true })
  assert.equal(weapp.mode, 'weapp')
  assert.equal(weapp.canLaunch, true)
  assert.equal(resolveCreditLaunch({ mock: false, creditPackage: 'pkg', isWechatBrowser: true }).mode, 'h5')
  const outside = resolveCreditLaunch({ mock: false, creditPackage: 'pkg', isWechatBrowser: false })
  assert.equal(outside.mode, 'unsupported')
  assert.equal(outside.canLaunch, false, '微信外浏览器禁止展示可点拉起')
})

test('评测页路由：一律 query order_id，return_path 编码', () => {
  const route = buildAssessmentRoute({
    orderId: 'o-1', tier: 'reduced', riskAmountCents: 30000, returnPath: '/payment?type=rent&id=o-1',
  })
  assert.match(route, /^\/credit-assessment\?/)
  assert.match(route, /order_id=o-1/)
  assert.match(route, /tier=reduced/)
  assert.match(route, /risk_amount_cents=30000/)
  assert.ok(!route.includes('?type=rent&id=o-1'), 'return_path 必须编码')
})

test('轮询退避与终态判定', () => {
  assert.equal(pollDelay(0), 1500)
  assert.equal(pollDelay(1), 3000)
  assert.ok(pollDelay(10) <= 8000)
  assert.equal(isCreditSettled('approved'), true)
  assert.equal(isCreditSettled('rejected'), true)
  assert.equal(isCreditSettled('pending'), false)
  assert.equal(isCreditSettled(''), false)
})
