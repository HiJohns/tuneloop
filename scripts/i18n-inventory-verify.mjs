#!/usr/bin/env node
// #2323 界面文案清单：校验器。
//
// 断言四类不变量（每类一条摘要行，机器可判定）：
//   CHECK        — 每条 来源=report 的 `file:line` 处确实含有 `text`（`{msg}` 为合成后缀，断言前缀）
//   PARITY       — `来源=report` 行与 `i18n-extract.mjs --report` 实时输出逐条相等
//   SUPPLEMENTAL — 8 条「待产品确认（原 #NAME?）」/ 6 条需求备注 / 5 条样式备注 均在，且无丢失
//   MD           — `i18n-inventory.md` 的统计（端/文件数/条数）与 CSV 一致
//
// 用法：
//   node scripts/i18n-inventory-verify.mjs                                    # 只跑 CHECK
//   node scripts/i18n-inventory-verify.mjs --parity                            # + PARITY
//   node scripts/i18n-inventory-verify.mjs --check-supplemental                # + SUPPLEMENTAL
//   node scripts/i18n-inventory-verify.mjs --check-md docs/ops/i18n-inventory.md  # + MD
//   node scripts/i18n-inventory-verify.mjs --csv X --negative Y                # 夹具必须被检出（负例）
//   node scripts/i18n-inventory-verify.mjs --evidence docs/evidence/2323/R2-verify.txt
//
// `--negative` 与所选检查同类：对夹具跑同样的检查，必须发现 >=1 个问题，否则整体判 FAIL
// （即「校验器对坏数据不敏感」本身是缺陷）。
//
// 退出码：0 全部通过（含负例被检出）；1 有失败。
import fs from 'fs'
import path from 'path'
import { execFileSync } from 'child_process'
import { fileURLToPath } from 'url'

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const EXTRACTOR = path.join(ROOT, 'scripts/i18n-extract.mjs')
const SOURCES = [
  { end: 'mobile', dir: 'frontend-mobile/src' },
  { end: 'pc', dir: 'frontend-pc/src' },
]

const argv = process.argv.slice(2)
const flag = (n) => argv.includes(n)
const opt = (n, d = '') => {
  const i = argv.indexOf(n)
  return i >= 0 && argv[i + 1] ? argv[i + 1] : d
}
const CSV = path.resolve(ROOT, opt('--csv', 'docs/ops/i18n-inventory.csv'))
const MD = opt('--check-md', '')
const NEGATIVE = opt('--negative', '')
const EVIDENCE = opt('--evidence', '')
const DO_PARITY = flag('--parity')
const DO_SUPP = flag('--check-supplemental')
const DO_IDENTITY = flag('--check-name-identity')

const EXPECT = { name_error: 8, requirement: 6, style: 5 }

// 8 条 #NAME? 的 identity 真值：来自工作副本的 (file, 行号) + 与工作副本行号吻合的扫描快照。
// 计数相等不代表内容正确（2026-11-11 审计 Bug 1：一条被改成 + 新增担保人、另一条丢失）。
// 这里用 (file, text) 多重集比对；`pages-weapp/Checkout.jsx` 的两条都是 `+ 使用新地址`。
const NAME_ERROR_EXPECT = [
  { file: 'frontend-mobile/src/components/AddressManager.jsx', text: '+ 新地址' },
  { file: 'frontend-mobile/src/pages/Checkout.jsx', text: '+ 新增担保人' },
  { file: 'frontend-mobile/src/pages/Checkout.jsx', text: '+ 使用新地址' },
  { file: 'frontend-mobile/src/pages/RepairServiceCreate.jsx', text: '＋ 录制/选择试奏视频（≤60s）' },
  { file: 'frontend-mobile/src/pages/RepairServiceCreate.jsx', text: '＋ 选择试奏视频' },
  { file: 'frontend-mobile/src/pages-weapp/Checkout.jsx', text: '+ 使用新地址' },
  { file: 'frontend-mobile/src/pages-weapp/Checkout.jsx', text: '+ 使用新地址' },
  { file: 'frontend-pc/src/pages/admin/instrument/Detail.jsx', text: '+ 新增' },
]

// ---------------------------------------------------------------- io
function parseCsvLine(line) {
  const out = []
  let cur = ''
  let q = false
  for (let i = 0; i < line.length; i += 1) {
    const c = line[i]
    if (q) {
      if (c === '"' && line[i + 1] === '"') { cur += '"'; i += 1 } else if (c === '"') q = false
      else cur += c
    } else if (c === '"') q = true
    else if (c === ',') { out.push(cur); cur = '' } else cur += c
  }
  out.push(cur)
  return out
}

function readRows(file) {
  if (!fs.existsSync(file)) return []
  const text = fs.readFileSync(file, 'utf8').trim()
  if (!text) return []
  const lines = text.split(/\r?\n/)
  const header = lines[0].split(',')
  return lines.slice(1).map((l) => {
    const cells = parseCsvLine(l)
    const o = {}
    header.forEach((h, i) => { o[h.trim()] = cells[i] ?? '' })
    return {
      end: o.end || '',
      file: o.file || '',
      line: o.line || '',
      text: o.text || '',
      source: o['来源'] || '',
      note: o['备注'] || '',
    }
  })
}

function runReport(dir) {
  const out = execFileSync(process.execPath, [EXTRACTOR, '--report', dir], { cwd: ROOT, encoding: 'utf8' })
  const rows = []
  for (const raw of out.split('\n')) {
    const line = raw.replace(/\r$/, '')
    if (!line || line.startsWith('[') || line.startsWith('  shadowed\t')) continue
    const m = /^(.*):(\d+)\t(.*)$/.exec(line)
    if (m) rows.push({ end: '', file: m[1], line: Number(m[2]), text: m[3] })
  }
  return rows
}

function fileLines(rel) {
  try { return fs.readFileSync(path.join(ROOT, rel), 'utf8').split('\n') } catch { return null }
}

// ---------------------------------------------------------------- checks
function checkRows(rows) {
  const problems = []
  let checked = 0
  let exact = 0
  let prefix = 0
  for (const r of rows) {
    if (r.source !== 'report') continue
    checked += 1
    const lines = fileLines(r.file)
    if (!lines) { problems.push(`${r.file}: file missing`); continue }
    const ln = Number(r.line)
    if (!ln || ln > lines.length) { problems.push(`${r.file}:${r.line}: line out of range`); continue }
    const isPrefix = r.text.includes('{msg}')
    const needle = isPrefix ? r.text.split('{msg}')[0] : r.text
    if (lines[ln - 1].includes(needle)) { if (isPrefix) prefix += 1; else exact += 1 }
    else problems.push(`${r.file}:${ln}: text not found`)
  }
  return { summary: `CHECK: checked=${checked} exact=${exact} prefix=${prefix} failures=${problems.length}`, problems, checked, exact, prefix }
}

function checkParity(rows) {
  const csvRows = rows.filter((r) => r.source === 'report')
  const live = []
  for (const s of SOURCES) for (const r of runReport(s.dir)) live.push({ end: s.end, file: r.file, line: String(r.line), text: r.text })
  const key = (r) => `${r.end}\u0000${r.file}\u0000${Number(r.line)}\u0000${r.text}`
  const csvSet = new Set(csvRows.map(key))
  const liveSet = new Set(live.map(key))
  let diff = 0
  const problems = []
  for (const k of csvSet) if (!liveSet.has(k)) { diff += 1; problems.push(`only-in-csv: ${k.replace(/\u0000/g, ' | ')}`) }
  for (const k of liveSet) if (!csvSet.has(k)) { diff += 1; problems.push(`only-in-report: ${k.replace(/\u0000/g, ' | ')}`) }
  return { summary: `PARITY: report_rows=${live.length} csv_report_rows=${csvRows.length} diff=${diff}`, problems, diff }
}

function countMarks(rows) {
  let nameError = 0
  let requirement = 0
  let style = 0
  for (const r of rows) {
    if (r.note.startsWith('待产品确认')) nameError += 1
    if (r.note.startsWith('人工需求')) requirement += 1
    if (r.note.startsWith('人工备注')) style += 1
  }
  return { nameError, requirement, style }
}

function checkSupplemental(rows) {
  const { nameError, requirement, style } = countMarks(rows)
  const lost = Math.max(0, EXPECT.name_error - nameError)
    + Math.max(0, EXPECT.requirement - requirement)
    + Math.max(0, EXPECT.style - style)
  const problems = []
  if (lost > 0) problems.push(`supplemental rows lost=${lost}`)
  return {
    summary: `SUPPLEMENTAL: name_error=${nameError} requirement=${requirement} style=${style} lost=${lost}`,
    problems,
    lost,
  }
}

function checkNameIdentity(rows) {
  const actual = new Map()
  for (const r of rows) {
    if (!r.note.startsWith('待产品确认')) continue
    const k = `${r.file}\u0000${r.text}`
    actual.set(k, (actual.get(k) || 0) + 1)
  }
  const expect = new Map()
  for (const e of NAME_ERROR_EXPECT) {
    const k = `${e.file}\u0000${e.text}`
    expect.set(k, (expect.get(k) || 0) + 1)
  }
  let diff = 0
  let matched = 0
  const problems = []
  for (const [k, n] of expect) {
    const a = actual.get(k) || 0
    matched += Math.min(a, n)
    if (a !== n) {
      diff += Math.abs(a - n)
      problems.push(`identity mismatch ${k.replace(/\u0000/g, ' | ')} expected=${n} actual=${a}`)
    }
  }
  for (const [k, a] of actual) {
    if (expect.has(k)) continue
    diff += a
    problems.push(`unexpected name_error row ${k.replace(/\u0000/g, ' | ')}`)
  }
  return {
    summary: `NAME-IDENTITY: expected=${NAME_ERROR_EXPECT.length} matched=${matched} identity_diff=${diff}`,
    problems,
    diff,
  }
}

function mdStats(mdPath, rows) {
  const text = fs.readFileSync(mdPath, 'utf8')
  const pick = (label) => {
    const m = new RegExp(`^\\|\\s*${label}\\s*\\|\\s*(\\d+)\\s*\\|\\s*(\\d+)\\s*\\|$`, 'm').exec(text)
    return m ? { files: Number(m[1]), rows: Number(m[2]) } : null
  }
  const mobile = pick('frontend-mobile')
  const pc = pick('frontend-pc')
  const total = pick('合计')
  const report = rows.filter((r) => r.source === 'report')
  const exp = {
    mobile: { files: new Set(report.filter((r) => r.end === 'mobile').map((r) => r.file)).size, rows: report.filter((r) => r.end === 'mobile').length },
    pc: { files: new Set(report.filter((r) => r.end === 'pc').map((r) => r.file)).size, rows: report.filter((r) => r.end === 'pc').length },
  }
  exp.total = { files: exp.mobile.files + exp.pc.files, rows: exp.mobile.rows + exp.pc.rows }
  const problems = []
  for (const [name, got] of [['frontend-mobile', mobile], ['frontend-pc', pc], ['合计', total]]) {
    const want = name === '合计' ? exp.total : name === 'frontend-mobile' ? exp.mobile : exp.pc
    if (!got) problems.push(`${name}: stats row missing`)
    else if (got.files !== want.files || got.rows !== want.rows) problems.push(`${name}: got ${got.files}/${got.rows} want ${want.files}/${want.rows}`)
  }
  return {
    summary: `MD: mobile=${exp.mobile.rows} files_mobile=${exp.mobile.files} pc=${exp.pc.rows} files_pc=${exp.pc.files} total=${exp.total.rows} files_total=${exp.total.files}`,
    problems,
  }
}

// 负例强度：同类检查在夹具上必须报出问题
function negativeCount(kind, file) {
  if (kind === 'md') return mdStats(path.resolve(ROOT, file), readRows(CSV)).problems.length
  const fixture = readRows(path.resolve(ROOT, file))
  if (kind === 'parity') return checkParity(fixture).problems.length
  if (kind === 'supp') return checkSupplemental(fixture).problems.length
  if (kind === 'identity') return checkNameIdentity(fixture).problems.length
  return checkRows(fixture).problems.length
}

// ---------------------------------------------------------------- main
const log = []
let fail = false
const rows = readRows(CSV)

const main = checkRows(rows)
log.push(main.summary)
if (main.problems.length) {
  fail = true
  for (const p of main.problems.slice(0, 10)) log.push(`  FAIL ${p}`)
}

if (DO_PARITY) {
  const r = checkParity(rows)
  log.push(r.summary)
  if (r.problems.length) {
    fail = true
    for (const p of r.problems.slice(0, 10)) log.push(`  FAIL ${p}`)
  }
}

if (DO_SUPP) {
  const r = checkSupplemental(rows)
  log.push(r.summary)
  if (r.problems.length) { fail = true; for (const p of r.problems) log.push(`  FAIL ${p}`) }
}

if (DO_IDENTITY) {
  const r = checkNameIdentity(rows)
  log.push(r.summary)
  if (r.problems.length) { fail = true; for (const p of r.problems.slice(0, 10)) log.push(`  FAIL ${p}`) }
}

if (MD) {
  const r = mdStats(path.resolve(ROOT, MD), rows)
  log.push(r.summary)
  if (r.problems.length) { fail = true; for (const p of r.problems) log.push(`  FAIL ${p}`) }
}

if (NEGATIVE) {
  const kind = MD ? 'md' : DO_PARITY ? 'parity' : DO_IDENTITY ? 'identity' : DO_SUPP ? 'supp' : 'rows'
  const n = negativeCount(kind, NEGATIVE)
  log.push(`NEGATIVE(${path.basename(NEGATIVE, path.extname(NEGATIVE))})=${n}`)
  if (!n) { fail = true; log.push('  FAIL negative fixture was not detected') }
}

log.push(`RESULT: ${fail ? 'FAIL' : 'PASS'}`)
const text = `${log.join('\n')}\n`
process.stdout.write(text)
if (EVIDENCE) {
  const p = path.resolve(ROOT, EVIDENCE)
  fs.mkdirSync(path.dirname(p), { recursive: true })
  fs.writeFileSync(p, text)
}
process.exit(fail ? 1 : 0)
