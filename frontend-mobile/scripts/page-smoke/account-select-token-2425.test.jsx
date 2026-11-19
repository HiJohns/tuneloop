import { describe, it, expect } from 'vitest'
import {
  isExchangeTokenExpired,
  isAccountPrefetchFresh,
  decideAfterSelectFailure,
} from '../../src/utils/accountSelect.mjs'

describe('account select token lifecycle (#2425)', () => {
  it('TestAccountSelectRetryDecision_2425 refreshes on 40110 and fails otherwise', () => {
    expect(isExchangeTokenExpired(40110)).toBe(true)
    expect(decideAfterSelectFailure(40110)).toBe('refresh-and-retry')
    expect(decideAfterSelectFailure(50000)).toBe('clear-and-fail')
    expect(decideAfterSelectFailure(40002)).toBe('clear-and-fail')
  })

  it('TestAccountSelectClearToken_2425 treats legacy 50000 as clear-and-fail', () => {
    expect(decideAfterSelectFailure('50000')).toBe('clear-and-fail')
    expect(decideAfterSelectFailure(undefined)).toBe('clear-and-fail')
  })

  it('TestAccountPrefetchFreshness_2425 expires after four minutes', () => {
    const now = 1_700_000_000_000
    expect(isAccountPrefetchFresh(now - 60_000, now)).toBe(true)
    expect(isAccountPrefetchFresh(now - 239_000, now)).toBe(true)
    expect(isAccountPrefetchFresh(now - 240_000, now)).toBe(false)
    expect(isAccountPrefetchFresh(now - 300_000, now)).toBe(false)
    expect(isAccountPrefetchFresh(0, now)).toBe(false)
    expect(isAccountPrefetchFresh(undefined, now)).toBe(false)
  })
})
