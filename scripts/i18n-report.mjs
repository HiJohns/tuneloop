#!/usr/bin/env node
// #2213 文案覆盖率报告（闸 3 仪表）：按目录统计「硬编码中文字符数」与「t() 调用数」。
// 用法：node scripts/i18n-report.mjs [frontend-mobile/src]
import fs from 'fs'
import path from 'path'
import { fileURLToPath } from 'url'

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const DIR = process.argv[2] || 'frontend-mobile/src'
const BASE = path.join(ROOT, DIR)
const ZH = /[\u4e00-\u9fff]/g

function walk(dir, out = []) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name)
    if (e.isDirectory()) {
      if (['node_modules', 'dist', 'dist-weapp'].includes(e.name)) continue
      walk(p, out)
    } else if (/\.(jsx|js|ts|tsx)$/.test(e.name)) out.push(p)
  }
  return out
}

const rows = new Map()
for (const f of walk(BASE)) {
  const s = fs.readFileSync(f, 'utf8')
  const zh = (s.match(ZH) || []).length
  const tCalls = (s.match(/\bt\(/g) || []).length
  if (!zh && !tCalls) continue
  const rel = path.relative(ROOT, f)
  const top = rel.split('/').slice(0, 3).join('/') // frontend-mobile/src/<subdir>
  const r = rows.get(top) || { files: 0, zh: 0, t: 0 }
  r.files += 1; r.zh += zh; r.t += tCalls
  rows.set(top, r)
}

console.log(`# i18n 覆盖率报告 — ${DIR}`)
console.log('| 目录 | 含中文/t() 文件 | 中文字符数 | t() 调用数 |')
console.log('|------|---------------:|----------:|----------:|')
let tz = 0, tt = 0
for (const [k, v] of [...rows.entries()].sort((a, b) => b[1].zh - a[1].zh)) {
  console.log(`| ${k} | ${v.files} | ${v.zh} | ${v.t} |`)
  tz += v.zh; tt += v.t
}
console.log(`| **合计** | — | **${tz}** | **${tt}** |`)
console.log('\n> 指标：抽取推进 = 中文字符数↓ + t() 调用数↑（DoD：UI 类残留=0，由 ESLint `i18n/no-hardcoded-ui-string` 判定）。')
