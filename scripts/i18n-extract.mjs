#!/usr/bin/env node
// #2213 文案抽取/改写（AST）——策略 docs/topics/i18n-strategy.md 闸 1。
// 用法：
//   node scripts/i18n-extract.mjs            # dry-run（默认；仅报告，不改文件）
//   node scripts/i18n-extract.mjs --report   # 打印 文件:行 → 文案 清单
//   node scripts/i18n-extract.mjs --write    # 改写源码并重建 zh-CN.json 清单
//   node scripts/i18n-extract.mjs <globdir>  # 限定子目录（默认 frontend-mobile/src）
//
// 改写规则（源串即 key，t() 缺失回退=恒等，故零行为变化）：
//   - JSXText（含中文、非纯空白）           → {t('…')}
//   - JSX 属性 placeholder/title/label/alt 值 → {t('…')}
//   - dialog.alert/toast/confirm、Taro.showToast/showModal、
//     message.*、resolveErrorMessage 第2参 中的中文字面量 → t('…')
//   - `'前缀' + x`（左端串含中文）          → t('前缀{msg}', { msg: x })
// 黑名单：console.*、已在 t() 内、纯空白 JSXText、非 UI 属性。
import fs from 'fs'
import path from 'path'
import { fileURLToPath, pathToFileURL } from 'url'
import { createRequire } from 'module'
import { mergeLocale } from './i18n-merge.mjs'

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const require = createRequire(pathToFileURL(path.join(ROOT, 'frontend-mobile/package.json')))
const { parse } = require('@babel/parser')
const traverse = require('@babel/traverse').default

const args = process.argv.slice(2)
const WRITE = args.includes('--write')
const REPORT = args.includes('--report')
const DIR = args.find((a) => !a.startsWith('--')) || 'frontend-mobile/src'
const TARGET = path.join(ROOT, DIR)
const ZH_JSON = path.join(ROOT, 'frontend-mobile/src/i18n/locales/zh-CN.json')

const ZH = /[\u4e00-\u9fff]/
const DIALOG_CALLS = new Set(['alert', 'toast', 'confirm'])
const TARO_CALLS = new Set(['showToast', 'showModal'])
const MSG_CALLS = new Set(['error', 'success', 'warning', 'info'])

function walk(dir, out = []) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name)
    if (e.isDirectory()) {
      if (['node_modules', 'dist', 'dist-weapp'].includes(e.name)) continue
      walk(p, out)
    } else if (/\.(jsx|js|ts|tsx)$/.test(e.name) && !/\.test\./.test(e.name)) {
      out.push(p)
    }
  }
  return out
}

function parseFile(src, file) {
  const plugins = ['jsx', 'classProperties', 'objectRestSpread', 'optionalChaining', 'nullishCoalescingOperator', 'dynamicImport']
  if (/\.tsx?$/.test(file)) plugins.push('typescript')
  return parse(src, { sourceType: 'module', plugins, errorRecovery: true })
}

// 已处于 t(...) 调用内？
function insideT(p) {
  let n = p
  while (n) {
    if (n.isCallExpression() && n.node.callee && n.node.callee.name === 't') return true
    n = n.parentPath
  }
  return false
}
function calleeName(p) {
  const c = p.node.callee
  if (!c) return ''
  if (c.name) return c.name
  if (c.property) return c.property.name || ''
  return ''
}
function objName(p) {
  const c = p.node.callee
  return c && c.object && c.object.name ? c.object.name : ''
}
function isConsole(p) {
  let n = p
  while (n) {
    if (n.isCallExpression() && objName(n) === 'console') return true
    n = n.parentPath
  }
  return false
}

const edits = [] // {file, start, end, text}
const keys = new Set()
const found = [] // {file, line, text}

function addEdit(node, replacement) {
  edits.push({ file: node.__file, start: node.start, end: node.end, text: replacement })
}

function collect(node, text) {
  keys.add(text)
  found.push({ file: node.__file, line: (node.loc && node.loc.start.line) || 0, text })
}

function esc(s) {
  return JSON.stringify(s) // 安全转义（含引号）
}

const files = (() => {
  try {
    return fs.statSync(TARGET).isDirectory() ? walk(TARGET) : [TARGET]
  } catch {
    return []
  }
})()
for (const file of files) {
  let src
  try { src = fs.readFileSync(file, 'utf8') } catch { continue }
  if (!ZH.test(src)) continue
  let ast
  try { ast = parseFile(src, file) } catch { continue }
  const rel = path.relative(ROOT, file)

  traverse(ast, {
    JSXText(p) {
      const raw = p.node.value
      const core = raw.trim()
      if (!core || !ZH.test(core) || insideT(p)) return
      const lead = raw.slice(0, raw.indexOf(core))
      const tail = raw.slice(raw.indexOf(core) + core.length)
      p.node.__file = rel
      collect(p.node, core)
      addEdit(p.node, `${lead}{t(${esc(core)})}${tail}`)
    },
    JSXAttribute(p) {
      const name = p.node.name && p.node.name.name
      if (!['placeholder', 'title', 'label', 'alt'].includes(name)) return
      const v = p.node.value
      if (!v || !v.value || typeof v.value !== 'string' || !ZH.test(v.value) || insideT(p)) return
      v.__file = rel
      collect(v, v.value)
      const nv = p.node.value
      addEdit(nv, `{t(${esc(nv.value)})}`)
    },
    CallExpression(p) {
      if (isConsole(p) || insideT(p)) return
      const fn = calleeName(p)
      const obj = objName(p)
      const isDialog = obj === 'dialog' && DIALOG_CALLS.has(fn)
      const isTaro = fn === 't' ? true : false // avoid: skip
      const isTaroCall = obj === 'Taro' && TARO_CALLS.has(fn)
      const isMsg = obj === 'message' && MSG_CALLS.has(fn)
      const isResolve = fn === 'resolveErrorMessage'
      if (!(isDialog || isTaroCall || isMsg || isResolve)) return
      // 处理字符串字面量参数（dialog/message/Taro 的首参；resolveErrorMessage 第 2 参）
      const strArgs = p.node.arguments.filter((a) => a && a.type === 'StringLiteral' && ZH.test(a.value))
      const wanted = isResolve ? strArgs.slice(1) : strArgs
      // Taro.showToast/showModal 的对象参数
      for (const a of p.node.arguments) {
        if (a.type === 'ObjectExpression') {
          for (const prop of a.properties) {
            if (!prop.key || !['title', 'content'].includes(prop.key.name)) continue
            const val = prop.value
            if (val.type === 'StringLiteral' && ZH.test(val.value)) {
              val.__file = rel
              collect(val, val.value)
              addEdit(val, `t(${esc(val.value)})`)
            }
          }
        }
      }
      for (const a of wanted) {
        a.__file = rel
        collect(a, a.value)
        addEdit(a, `t(${esc(a.value)})`)
      }
    },
    BinaryExpression(p) {
      // '前缀' + x  →  t('前缀{msg}', { msg: x })
      if (p.node.operator !== '+') return
      const left = p.node.left
      if (left.type !== 'StringLiteral' || !ZH.test(left.value) || insideT(p)) return
      // 仅在调用/属性实参或 JSX 表达式上下文中处理，避免误伤
      const parent = p.parentPath
      const inArg = parent && (parent.isCallExpression() || parent.isJSXExpressionContainer() || parent.isReturnStatement())
      if (!inArg) return
      left.__file = rel
      p.node.__file = rel
      const key = left.value + '{msg}'
      collect(left, key)
      const rightSrc = src.slice(p.node.right.start, p.node.right.end)
      addEdit(p.node, `t(${esc(key)}, { msg: ${rightSrc} })`)
    },
  })
}

// 应用编辑（按位置倒序，避免偏移错乱）
if (WRITE && edits.length) {
  const byFile = new Map()
  for (const e of edits) {
    if (!byFile.has(e.file)) byFile.set(e.file, [])
    byFile.get(e.file).push(e)
  }
  for (const [rel, list] of byFile) {
    const abs = path.join(ROOT, rel)
    let s = fs.readFileSync(abs, 'utf8')
    list.sort((a, b) => b.start - a.start)
    for (const e of list) s = s.slice(0, e.start) + e.text + s.slice(e.end)
    fs.writeFileSync(abs, s)
  }
  // 合并写回（保留既有 key，避免单文件覆盖整本字典；#2298）
  let existing = {}
  try { existing = JSON.parse(fs.readFileSync(ZH_JSON, 'utf8')) } catch {}
  const inv = mergeLocale(existing, [...keys])
  fs.writeFileSync(ZH_JSON, JSON.stringify(inv, null, 2) + '\n')
}

// 输出
if (REPORT) {
  for (const f of found) console.log(`${f.file}:${f.line}\t${f.text}`)
} else {
  const byFile = {}
  for (const f of found) byFile[f.file] = (byFile[f.file] || 0) + 1
  console.log(`[i18n-extract] ${WRITE ? 'WRITE' : 'dry-run'} | 扫描 ${files.length} 文件 | 命中 ${found.length} 处 / ${Object.keys(byFile).length} 文件 | 去重 key ${keys.size}`)
  const top = Object.entries(byFile).sort((a, b) => b[1] - a[1]).slice(0, 10)
  for (const [f, n] of top) console.log(`  ${n}\t${f}`)
  if (!WRITE) console.log('  （dry-run：未改文件；加 --write 执行改写）')
}
