import { describe, it, expect } from 'vitest'
import fs from 'fs'
import path from 'path'
import { secondaryButtonStyle, primaryButtonStyle } from '../../src/utils/buttonStyles.mjs'

describe('address buttons (#2432)', () => {
  it('styles use inline fixed heights', () => {
    expect(secondaryButtonStyle().height).toBe(36)
    expect(primaryButtonStyle().height).toBe(48)
    expect(secondaryButtonStyle().margin).toBe(0)
    expect(primaryButtonStyle().margin).toBe(0)
  })

  it('AddressManager uses the shared styles (no py-only buttons)', () => {
    const src = fs.readFileSync(path.resolve(process.cwd(), 'src/components/AddressManager.jsx'), 'utf8')
    expect(src).toContain('secondaryButtonStyle')
    expect(src).toContain('primaryButtonStyle')
    expect(src).not.toContain('py-1.5 bg-zinc-100')
  })
})
