// #2436：禁止用户可见的裸大写英文标签（i18n 约定：中文源串即 key）。
// 真机发现确认订单页合计显示 "TOTAL PAYABLE"（硬编码英文）。
// #2436 Rev.3：扫描范围从“单行 JSX 文本节点”扩到
//   ① JSX 文本节点；② 用户文案属性中的字符串/模板字面量；③ `uppercase` class；
// 判定：字面量中的 ASCII 字母 ≥4 且全大写 → 命中（`EMS` 之类 ≤3 缩写不误报）。
// SCAN_ROOT 环境变量用于 R0 红例（指向 pre-fix 文件树）。
import { describe, it, expect } from 'vitest'
import fs from 'fs'
import path from 'path'

const SCAN_ROOT = process.env.SCAN_ROOT
  ? path.resolve(process.cwd(), process.env.SCAN_ROOT)
  : path.resolve(process.cwd(), 'src')
const ALLOW_EXT = new Set(['.jsx', '.js'])
// 技术常量白名单（非用户文案）：登记路径片段 + 原因。当前无例外；
// 新增例外必须在此表写清原因，否则扫描命中即失败。
const ALLOWLIST = []

const TEXT_NODE = />([^<>{}]*)</g
const USER_COPY_PROP = /\b(label|title|placeholder|content|message|text|okText|cancelText|description|header|confirmText|children)\s*=/
const LITERAL = /(['"`])([^'"`]*)\1/g

function isUpperPhrase(value) {
  const letters = String(value).replace(/[^A-Za-z]/g, '')
  return letters.length >= 4 && letters === letters.toUpperCase()
}

function walk(dir, out = []) {
  for (const name of fs.readdirSync(dir)) {
    const p = path.join(dir, name)
    const st = fs.statSync(p)
    if (st.isDirectory()) walk(p, out)
    else if (ALLOW_EXT.has(path.extname(p))) out.push(p)
  }
  return out
}

function scanFile(file) {
  const hits = []
  const rel = path.relative(process.cwd(), file)
  if (ALLOWLIST.some((a) => rel.includes(a))) return hits
  const lines = fs.readFileSync(file, 'utf8').split('\n')
  lines.forEach((line, i) => {
    let m
    while ((m = TEXT_NODE.exec(line)) !== null) {
      if (isUpperPhrase(m[1])) hits.push(`${rel}:${i + 1}: jsx-text: ${m[1].trim()}`)
    }
    TEXT_NODE.lastIndex = 0

    if (USER_COPY_PROP.test(line)) {
      while ((m = LITERAL.exec(line)) !== null) {
        if (isUpperPhrase(m[2])) hits.push(`${rel}:${i + 1}: user-copy-literal: ${m[2].slice(0, 60)}`)
      }
      LITERAL.lastIndex = 0
    }

    if (line.includes('uppercase')) hits.push(`${rel}:${i + 1}: uppercase-class: ${line.trim().slice(0, 80)}`)
  })
  return hits
}

describe('no hardcoded uppercase English label (#2436)', () => {
  it('R1: 两页确认订单页合计标签走 t() 且无 TOTAL PAYABLE', () => {
    for (const rel of ['pages/Checkout.jsx', 'pages-weapp/Checkout.jsx']) {
      const src = fs.readFileSync(path.resolve(SCAN_ROOT, rel), 'utf8')
      expect(src).toContain("t('应付合计')")
      expect(src).not.toContain('TOTAL PAYABLE')
    }
  })

  it('R2: 无未登记的用户可见大写英文（JSX 文本/文案字面量/uppercase）', () => {
    const hits = []
    for (const file of walk(SCAN_ROOT)) hits.push(...scanFile(file))
    expect(hits).toEqual([])
  })
})
