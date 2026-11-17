#!/usr/bin/env node
// #2404 运行时证据（dev 生成，无需 Owner 签）：
// 1) 用 esbuild 打包真实的 InitialPasswordModal + antd，在 headless Chromium 中渲染；
// 2) 断言弹窗标题为「已创建 · 初始密码仅显示一次」且只出现一次；
// 3) 静态断言三个调用点各自恰好一个 title 且取统一文案。
import { build } from '../frontend-pc/node_modules/vite/node_modules/esbuild/lib/main.js'
import { readFileSync, writeFileSync, mkdirSync, readdirSync, existsSync } from 'node:fs'
import { execFileSync } from 'node:child_process'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const PC = resolve(ROOT, 'frontend-pc')
const OUT = resolve(ROOT, 'tmp/verify-2404')
mkdirSync(OUT, { recursive: true })

const TITLE = '已创建 · 初始密码仅显示一次'

const entry = `
import React from 'react'
import { createRoot } from 'react-dom/client'
import InitialPasswordModal from './src/components/InitialPasswordModal'
createRoot(document.getElementById('root')).render(
  React.createElement(InitialPasswordModal, {
    open: true,
    passwords: [{ identifier: 'tech_a', password: 'Abcd1234Wxyz' }],
    title: ${JSON.stringify(TITLE)},
    onClose: () => {},
  })
)
`
const bundle = resolve(OUT, 'bundle.js')
await build({
  stdin: { contents: entry, resolveDir: PC, loader: 'jsx' },
  bundle: true,
  outfile: bundle,
  platform: 'browser',
  format: 'iife',
  jsx: 'automatic',
  define: { 'process.env.NODE_ENV': '"production"' },
  logLevel: 'silent',
})

writeFileSync(resolve(OUT, 'index.html'), `<!doctype html><html><body><div id="root"></div><script src="./bundle.js"></script></body></html>`)

function findChrome() {
  const base = resolve(process.env.HOME || '/root', '.cache/ms-playwright')
  if (!existsSync(base)) return null
  const dirs = readdirSync(base)
  const ordered = [
    ...dirs.filter(d => d.startsWith('chromium_headless_shell-')),
    ...dirs.filter(d => d.startsWith('chromium-')),
  ]
  for (const d of ordered) {
    for (const p of [
      'chrome-headless-shell-linux64/chrome-headless-shell',
      'chrome-linux/chrome',
      'chrome-linux64/chrome',
    ]) {
      const f = resolve(base, d, p)
      if (existsSync(f)) return f
    }
  }
  return null
}
const chrome = findChrome()
if (!chrome) {
  console.error('CHROMIUM_NOT_FOUND')
  process.exit(2)
}
const dom = execFileSync(chrome, [
  '--headless=new', '--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage',
  '--virtual-time-budget=5000', '--dump-dom', `file://${resolve(OUT, 'index.html')}`,
], { encoding: 'utf8', maxBuffer: 32 * 1024 * 1024 })

const occurrences = dom.split(TITLE).length - 1
if (occurrences !== 1) {
  console.error(`FAIL: title occurrences=${occurrences}\n${dom.slice(0, 800)}`)
  process.exit(1)
}
if (!dom.includes('Abcd1234Wxyz')) {
  console.error('FAIL: password row not rendered')
  process.exit(1)
}
console.log(`RUNTIME PASS: title rendered exactly once (${TITLE}); password row rendered`)

const callSites = [
  'src/pages/DirectMemberManagement.jsx',
  'src/components/MerchantMemberManagement.jsx',
  'src/components/SiteMemberManagement.jsx',
  'src/pages/StaffManagement.jsx',
]
for (const rel of callSites) {
  const src = readFileSync(resolve(PC, rel), 'utf8')
  const start = src.indexOf('<InitialPasswordModal')
  if (start < 0) { console.error(`FAIL: ${rel} has no InitialPasswordModal`); process.exit(1) }
  const end = src.indexOf('/>', start)
  const block = src.slice(start, end)
  const titles = [...block.matchAll(/title="([^"]*)"/g)].map(m => m[1])
  if (titles.length !== 1 || titles[0] !== TITLE) {
    console.error(`FAIL: ${rel} titles=${JSON.stringify(titles)}`)
    process.exit(1)
  }
  console.log(`SOURCE PASS: ${rel} exactly one title (${titles[0]})`)
}
