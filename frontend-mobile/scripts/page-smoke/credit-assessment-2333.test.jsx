// #2333 R8: 信用评测页入参迁移与标的展示契约（静态断言，跨端共用同一 .jsx 源）。
//
// 新流程（#2352 / #2333 Rev.3.1）：
//   1. 评测页入参只接受 order_id（可选 return_path）；
//   2. 评测标的金额 = 乐器原价，由后端 risk_amount_cents 返回，页面不自算、不接收参数；
//   3. 页面不再展示「押金档位」行（档位选择在支付页）。
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const pagePath = resolve(here, '../../src/pages/CreditAssessment.jsx')
const source = readFileSync(pagePath, 'utf8')

describe('credit assessment page (#2333 R8)', () => {
  it('reads only order_id (and optional return_path) from query params', () => {
    expect(source).toMatch(/const\s+orderId\s*=\s*params\.order_id/)
    expect(source).toMatch(/const\s+returnPath\s*=\s*params\.return_path/)
  })

  it('no longer reads the legacy tier / risk_amount_cents params', () => {
    expect(source).not.toMatch(/params\.tier\b/)
    expect(source).not.toMatch(/params\.risk_amount_cents\b/)
    expect(source).not.toMatch(/const\s+tier\s*=\s*params\./)
  })

  it('takes the assessment amount from the backend payload (instrument list price)', () => {
    expect(source).toMatch(/const\s+riskAmountCents\s*=\s*Number\(credit\?\.risk_amount_cents\s*\|\|\s*0\)/)
  })

  it('declares riskAmountCents after the credit state (no TDZ regression)', () => {
    const stateIdx = source.indexOf('const [credit, setCredit] = useState(null)')
    const amountIdx = source.indexOf('const riskAmountCents')
    expect(stateIdx).toBeGreaterThan(-1)
    expect(amountIdx).toBeGreaterThan(stateIdx)
  })

  it('renders the assessment amount row instead of the deposit tier row', () => {
    expect(source).toMatch(/t\('评测标的金额'\)/)
    // 档位标签行已移除（提示语「与所选押金档位无关」与「返回更换押金档位」按钮属正常文案）
    expect(source).not.toMatch(/t\('押金档位'\)/)
    expect(source).toMatch(/评测标的为乐器原价/)
  })
})
