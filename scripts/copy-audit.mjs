#!/usr/bin/env node
// #2213 敏感词扫描（闸 4）——提审前必跑。扫「集中清单 zh-CN.json」+ 两端源码 UI 串。
// 用法：node scripts/copy-audit.mjs [目录...]   （默认 frontend-mobile/src frontend-pc/src）
// 词表：scripts/copy-audit-words.txt（运营/法务维护）。
import fs from 'fs'
import path from 'path'
import { fileURLToPath } from 'url'

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const WORDS_FILE = path.join(ROOT, 'scripts/copy-audit-words.txt')
const ZH_MANIFEST = path.join(ROOT, 'frontend-mobile/src/i18n/locales/zh-CN.json')

const words = fs.readFileSync(WORDS_FILE, 'utf8')
  .split('\n').map((l) => l.trim()).filter((l) => l && !l.startsWith('#'))

const targets = process.argv.slice(2)
const DIRS = targets.length ? targets : ['frontend-mobile/src', 'frontend-pc/src']

function walk(dir, out = []) {
  if (!fs.existsSync(dir)) return out
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name)
    if (e.isDirectory()) {
      if (['node_modules', 'dist', 'dist-weapp'].includes(e.name)) continue
      walk(p, out)
    } else if (/\.(jsx|js|ts|tsx|json)$/.test(e.name)) out.push(p)
  }
  return out
}

const files = []
for (const d of DIRS) walk(path.join(ROOT, d), files)
if (fs.existsSync(ZH_MANIFEST)) files.push(ZH_MANIFEST)

const hits = new Map() // word -> [{file,line,text}]
for (const f of files) {
  let lines
  try { lines = fs.readFileSync(f, 'utf8').split('\n') } catch { continue }
  const rel = path.relative(ROOT, f)
  lines.forEach((line, i) => {
    for (const w of words) {
      if (line.includes(w)) {
        if (!hits.has(w)) hits.set(w, [])
        hits.get(w).push({ file: rel, line: i + 1, text: line.trim().slice(0, 100) })
      }
    }
  })
}

console.log(`# 敏感词扫描（词表 ${words.length} 词 / 扫描 ${files.length} 文件）`)
if (hits.size === 0) {
  console.log('✅ 无命中。')
} else {
  let total = 0
  for (const [w, list] of [...hits.entries()]) {
    total += list.length
    console.log(`\n## 「${w}」— ${list.length} 处`)
    for (const h of list) console.log(`  ${h.file}:${h.line}\t${h.text}`)
  }
  console.log(`\n⚠️ 命中合计 ${total} 处（**提审前需人工复核**；非 UI 文案的注释命中可忽略）。`)
}
