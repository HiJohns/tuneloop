// #2436：禁止用户可见的裸大写英文标签（i18n 约定：中文源串即 key）。
// 真机发现确认订单页合计显示 "TOTAL PAYABLE"（硬编码英文）。
import { describe, it, expect } from 'vitest'
import fs from 'fs'
import path from 'path'

const ROOT = path.resolve(process.cwd(), 'src')
const ALLOW_EXT = new Set(['.jsx', '.js'])
// 允许的例外（技术标识，非用户文案）：如需新增必须在本表登记原因
const ALLOWLIST = []

function walk(dir, out = []) {
  for (const name of fs.readdirSync(dir)) {
    const p = path.join(dir, name)
    const st = fs.statSync(p)
    if (st.isDirectory()) walk(p, out)
    else if (ALLOW_EXT.has(path.extname(p))) out.push(p)
  }
  return out
}

// 匹配 JSX 文本节点里的裸大写英文短语，例如 >TOTAL PAYABLE<
const RE = />\s*[A-Z][A-Z0-9 &/_-]{3,}\s*</

describe('no hardcoded uppercase English label (#2436)', () => {
  it('src 下无裸大写英文文案（确认订单页 TOTAL PAYABLE 已中文经 t()）', () => {
    const hits = []
    for (const file of walk(ROOT)) {
      const rel = path.relative(process.cwd(), file)
      if (ALLOWLIST.some((a) => rel.includes(a))) continue
      const lines = fs.readFileSync(file, 'utf8').split('\n')
      lines.forEach((line, i) => {
        if (RE.test(line)) hits.push(`${rel}:${i + 1}: ${line.trim()}`)
      })
    }
    expect(hits).toEqual([])
  })

  it('两个确认订单页的合计标签均走 t()', () => {
    for (const rel of ['src/pages/Checkout.jsx', 'src/pages-weapp/Checkout.jsx']) {
      const src = fs.readFileSync(path.resolve(process.cwd(), rel), 'utf8')
      expect(src).toContain("t('应付合计')")
      expect(src).not.toContain('TOTAL PAYABLE')
    }
  })
})
