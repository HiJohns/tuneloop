import { describe, it, expect } from 'vitest'
import { buildTierOptions, DEPOSIT_TIER } from '../../src/utils/creditFlow.mjs'

// #2352 D1（Owner 口径）：full>0 → FULL+REDUCED（无 ZERO）；full==0 → 仅 ZERO 且需信用评测。
describe('buildTierOptions (#2352)', () => {
  it('full > 0 returns full + reduced and never zero', () => {
    const opts = buildTierOptions(6000, 2000)
    const tiers = opts.map((o) => o.tier)
    expect(tiers).toEqual([DEPOSIT_TIER.FULL, DEPOSIT_TIER.REDUCED])
    expect(tiers).not.toContain(DEPOSIT_TIER.ZERO)
    const full = opts.find((o) => o.tier === DEPOSIT_TIER.FULL)
    const reduced = opts.find((o) => o.tier === DEPOSIT_TIER.REDUCED)
    expect(full.requiresAssessment).toBe(false)
    expect(full.requiresGuarantors).toBe(false)
    expect(reduced.requiresAssessment).toBe(true)
    expect(reduced.requiresGuarantors).toBe(true)
    expect(reduced.amount).toBe(2000)
  })

  it('full > 0 without reduced config offers only full (no zero)', () => {
    const tiers = buildTierOptions(6000, null).map((o) => o.tier)
    expect(tiers).toEqual([DEPOSIT_TIER.FULL])
  })

  it('full == 0 offers only zero and requires credit assessment', () => {
    const opts = buildTierOptions(0, null)
    expect(opts).toHaveLength(1)
    expect(opts[0].tier).toBe(DEPOSIT_TIER.ZERO)
    expect(opts[0].amount).toBe(0)
    expect(opts[0].requiresAssessment).toBe(true)
    expect(opts[0].requiresGuarantors).toBe(false)
  })

  it('reduced equal to full is not offered', () => {
    const tiers = buildTierOptions(6000, 6000).map((o) => o.tier)
    expect(tiers).toEqual([DEPOSIT_TIER.FULL])
  })
})
