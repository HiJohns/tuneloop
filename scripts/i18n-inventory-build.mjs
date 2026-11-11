#!/usr/bin/env node
// #2323 界面文案清单：构建器。
//
// 数据源只有一个：`node scripts/i18n-extract.mjs --report <dir>`（当前仍硬编码、未抽取的中文残余）。
// 本脚本负责把它落成两份可核对的产物：
//   docs/ops/i18n-inventory.csv  — 机器可读（含人工行）
//   docs/ops/i18n-inventory.md   — 人工核对用（统计 / 分文件 / 待确认 / 人工备注 / 工具盲区）
//
// CSV 列：end,file,line,text,来源,修改,备注
//   来源=report  → 由生成器产出；`来源=report` 行与生成器输出**逐条相等**（verify --parity 断言）
//   来源=manual  → 人工补充行（待产品确认的 #NAME? 还原、样式备注、产品需求备注），不计入统计
//   修改/备注     → 来自产品/文案工作副本（i18n-inventory.csv）的人工列
//
// 用法：
//   node scripts/i18n-inventory-build.mjs                        # 重建两份产物
//   node scripts/i18n-inventory-build.mjs --self-test            # 重建 + 幂等/行数自检
//   node scripts/i18n-inventory-build.mjs --notes i18n-inventory.csv   # 首次从工作副本导入人工行
//   node scripts/i18n-inventory-build.mjs --evidence docs/evidence/2323/R1-build.txt
//
// 退出码：0 成功；1 自检失败。
import fs from 'fs'
import path from 'path'
import { execFileSync } from 'child_process'
import { fileURLToPath } from 'url'

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const DICT = path.join(ROOT, 'frontend-mobile/src/i18n/locales/zh-CN.json')
const EXTRACTOR = path.join(ROOT, 'scripts/i18n-extract.mjs')

const SOURCES = [
  { end: 'mobile', dir: 'frontend-mobile/src' },
  { end: 'pc', dir: 'frontend-pc/src' },
]

// 工作副本里被 Excel 当成公式解析的 8 条（文案以 `+` / `＋` 开头 → `#NAME?`）。
// 原文由 #2323 逐条人工核对：#NAME? 无法在代码中定位，故按代码还原 text（不猜、不删）。
const NAME_ERROR_REPAIRS = [
  { file: 'frontend-mobile/src/components/AddressManager.jsx', text: '+ 新地址' },
  { file: 'frontend-mobile/src/pages/Checkout.jsx', text: '+ 新增担保人' },
  { file: 'frontend-mobile/src/pages/Checkout.jsx', text: '+ 使用新地址' },
  { file: 'frontend-mobile/src/pages/RepairServiceCreate.jsx', text: '＋ 录制/选择试奏视频（≤60s）' },
  { file: 'frontend-mobile/src/pages/RepairServiceCreate.jsx', text: '＋ 选择试奏视频' },
  { file: 'frontend-mobile/src/pages-weapp/Checkout.jsx', text: '+ 新增担保人' },
  { file: 'frontend-mobile/src/pages-weapp/Checkout.jsx', text: '+ 使用新地址' },
  { file: 'frontend-pc/src/pages/admin/instrument/Detail.jsx', text: '+ 新增' },
]

const STYLE_NOTE_PREFIX_MIN = 20

// ---------------------------------------------------------------- argv
const argv = process.argv.slice(2)
const flag = (n) => argv.includes(n)
const opt = (n, d = '') => {
  const i = argv.indexOf(n)
  return i >= 0 && argv[i + 1] ? argv[i + 1] : d
}
const OUT_CSV = path.resolve(ROOT, opt('--out-csv', 'docs/ops/i18n-inventory.csv'))
const OUT_MD = path.resolve(ROOT, opt('--out-md', 'docs/ops/i18n-inventory.md'))
const NOTES = opt('--notes', '')
const EVIDENCE = opt('--evidence', '')
const SELF_TEST = flag('--self-test')
const EXPECT_ROWS = opt('--expect-rows', '')
const write = (p, s) => fs.writeFileSync(p, s)

// ---------------------------------------------------------------- helpers
function runReport(dir) {
  const out = execFileSync(process.execPath, [EXTRACTOR, '--report', dir], { cwd: ROOT, encoding: 'utf8' })
  const rows = []
  const shadowed = []
  for (const raw of out.split('\n')) {
    const line = raw.replace(/\r$/, '')
    if (!line) continue
    if (line.startsWith('[')) continue // 汇总行
    if (line.startsWith('  shadowed\t')) {
      const [, rest] = line.split('\t')
      const m = /^(.*):(\d+)$/.exec(rest || '')
      if (m) shadowed.push({ file: m[1], line: Number(m[2]) })
      continue
    }
    const m = /^(.*):(\d+)\t(.*)$/.exec(line)
    if (!m) continue
    rows.push({ file: m[1], line: Number(m[2]), text: m[3] })
  }
  return { rows, shadowed }
}

function readCsv(file) {
  if (!fs.existsSync(file)) return []
  const text = fs.readFileSync(file, 'utf8').trim()
  if (!text) return []
  const lines = text.split(/\r?\n/)
  const header = lines[0].split(',')
  return lines.slice(1).map((l) => {
    // 简单 CSV：本清单的字段不含逗号以外的复杂转义，text/备注可能含逗号 → 按列数从右往左切
    const cells = parseCsvLine(l)
    const o = {}
    header.forEach((h, i) => { o[h] = cells[i] ?? '' })
    return o
  })
}

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

const csvCell = (s) => {
  const v = s == null ? '' : String(s)
  return /[",\n]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v
}

function fileLines(file) {
  try { return fs.readFileSync(path.join(ROOT, file), 'utf8').split('\n') } catch { return null }
}

/** 首次出现某字面量的行号（1-based），未找到返回 0 */
function findLiteralLine(file, literal) {
  const lines = fileLines(file)
  if (!lines) return 0
  for (let i = 0; i < lines.length; i += 1) if (lines[i].includes(literal)) return i + 1
  return 0
}

/** 首次出现 t('文案') / t("文案") 的行号 */
function findTCallLine(file, text) {
  const lines = fileLines(file)
  if (!lines) return 0
  const needles = [`t('${text}')`, `t("${text}")`]
  for (let i = 0; i < lines.length; i += 1) if (needles.some((n) => lines[i].includes(n))) return i + 1
  return 0
}

function commonPrefixLen(a, b) {
  let i = 0
  while (i < a.length && i < b.length && a[i] === b[i]) i += 1
  return i
}

function loadDictKeys() {
  try { return Object.keys(JSON.parse(fs.readFileSync(DICT, 'utf8'))) } catch { return [] }
}

/** 样式备注：工作副本文案可能被截断/改写过，用「最长公共前缀 ≥ 20」在字典里定位代码原文 */
function matchStyleKey(noteText, dictKeys) {
  if (dictKeys.includes(noteText)) return noteText
  let best = ''
  let bestLen = 0
  for (const k of dictKeys) {
    const n = commonPrefixLen(noteText, k)
    if (n > bestLen && n >= STYLE_NOTE_PREFIX_MIN) { best = k; bestLen = n }
  }
  return bestLen >= STYLE_NOTE_PREFIX_MIN ? best : ''
}

// ---------------------------------------------------------------- build
function normalizeEnd(file) {
  return file.startsWith('frontend-pc/') ? 'pc' : 'mobile'
}

/** 全库（两目录）检索字面量，返回首个命中 `file:line`，未找到返回 '' */
function grepSourceTree(needle) {
  if (!needle) return ''
  for (const s of SOURCES) {
    const base = path.join(ROOT, s.dir)
    const stack = [base]
    while (stack.length) {
      const dir = stack.pop()
      let entries = []
      try { entries = fs.readdirSync(dir, { withFileTypes: true }) } catch { continue }
      for (const e of entries) {
        const p = path.join(dir, e.name)
        if (e.isDirectory()) {
          if (!['node_modules', 'dist', 'dist-weapp'].includes(e.name)) stack.push(p)
        } else if (/\.(jsx|js|ts|tsx)$/.test(e.name)) {
          const hit = findLiteralLine(path.relative(ROOT, p), needle)
          if (hit) return `${path.relative(ROOT, p)}:${hit}`
        }
      }
    }
  }
  return ''
}

function build(manualRows) {
  const report = []
  const shadowed = []
  for (const s of SOURCES) {
    const r = runReport(s.dir)
    for (const row of r.rows) report.push({ ...row, end: s.end, source: 'report' })
    shadowed.push(...r.shadowed)
  }
  const rows = report.concat(manualRows)
  return { report, manualRows, rows, shadowed }
}

function renderCsv(rows) {
  const header = 'end,file,line,text,来源,修改,备注'
  const body = rows.map((r) => [
    r.end, r.file, r.line === '' || r.line == null ? '' : r.line, r.text, r.source, r.edit || '', r.note || '',
  ].map(csvCell).join(','))
  return `${[header, ...body].join('\n')}\n`
}

function renderMd(report, rows, shadowed) {
  const byEnd = { mobile: report.filter((r) => r.end === 'mobile'), pc: report.filter((r) => r.end === 'pc') }
  const filesOf = (list) => new Set(list.map((r) => r.file))
  const stats = {
    mobile: { files: filesOf(byEnd.mobile).size, rows: byEnd.mobile.length },
    pc: { files: filesOf(byEnd.pc).size, rows: byEnd.pc.length },
  }
  const nameErrors = rows.filter((r) => (r.note || '').startsWith('待产品确认'))
  const styleNotes = rows.filter((r) => (r.note || '').startsWith('人工备注'))
  const reqNotes = rows.filter((r) => (r.note || '').startsWith('人工需求'))

  const out = []
  out.push('# 界面文案全量清单（人工核对用）')
  out.push('')
  out.push('> 生成：`node scripts/i18n-inventory-build.mjs`（**禁止手改**，本文件由脚本逐字节重建）')
  out.push('> 校验：`node scripts/i18n-inventory-verify.mjs`')
  out.push('> 数据源：`node scripts/i18n-extract.mjs --report <dir>`，即**尚未抽取**的硬编码中文残余（已 `t()` 的不在此表）')
  out.push('')
  out.push('| 端 | 文件数 | 文案条数 |')
  out.push('|---|---:|---:|')
  out.push(`| frontend-mobile | ${stats.mobile.files} | ${stats.mobile.rows} |`)
  out.push(`| frontend-pc | ${stats.pc.files} | ${stats.pc.rows} |`)
  out.push(`| 合计 | ${stats.mobile.files + stats.pc.files} | ${stats.mobile.rows + stats.pc.rows} |`)
  out.push('')
  out.push(`> 上表只统计残余文案（来源=report）。另有 ${rows.length - report.length} 条人工行（待产品确认 / 样式备注 / 需求备注），见文末。`)
  out.push('')

  for (const [label, list] of [['frontend-mobile', byEnd.mobile], ['frontend-pc', byEnd.pc]]) {
    out.push(`## ${label}`)
    out.push('')
    const byFile = new Map()
    for (const r of list) {
      if (!byFile.has(r.file)) byFile.set(r.file, [])
      byFile.get(r.file).push(r)
    }
    for (const [file, items] of byFile) {
      out.push(`### \`${file}\``)
      out.push('')
      for (const it of items) out.push(`- L${it.line}: ${it.text}`)
      out.push('')
    }
  }

  out.push(`## 待产品确认（原 #NAME?，${nameErrors.length} 条）`)
  out.push('')
  out.push('> Excel 把 `+` / `＋` 开头的文案当公式解析成了 `#NAME?`。文案按代码还原，未猜、未删；措辞待产品确认。')
  out.push('')
  out.push('| 端 | 文件 | 行 | 文案 | 备注 |')
  out.push('|---|---|---:|---|---|')
  for (const r of nameErrors) out.push(`| ${r.end} | \`${r.file}\` | ${r.line || '-'} | ${r.text} | ${r.note} |`)
  out.push('')

  out.push(`## 人工备注与需求（${styleNotes.length + reqNotes.length} 条）`)
  out.push('')
  out.push('> 来自产品/文案工作副本 `i18n-inventory.csv`（未入 git）。`修改` 列实测 0 条非空。')
  out.push('')
  out.push('| 类型 | 文件 | 行 | 文案 | 备注 |')
  out.push('|---|---|---:|---|---|')
  for (const r of styleNotes) out.push(`| 样式 | \`${r.file}\` | ${r.line || '-'} | ${r.text} | ${r.note} |`)
  for (const r of reqNotes) out.push(`| 需求 | - | - | ${r.text} | ${r.note} |`)
  out.push('')

  out.push(`## 工具盲区（未纳入清单，${shadowed.length} 条）`)
  out.push('')
  out.push('> `t` 被局部变量遮蔽（如 `map((t, i))`）时 AST 改写会误伤，生成器主动跳过；这些文案仍为硬编码，需人工 `t()`。')
  out.push('')
  out.push('| 文件:行 | 原因 |')
  out.push('|---|---|')
  for (const s of shadowed) out.push(`| \`${s.file}:${s.line}\` | t 被局部变量遮蔽 |`)
  out.push('')
  return `${out.join('\n')}\n`
}

function sortRows(rows) {
  const key = (r) => [r.end, r.source === 'report' ? '0' : '1', r.file, String(r.line).padStart(6, '0'), r.text]
  return rows.slice().sort((a, b) => {
    const ka = key(a); const kb = key(b)
    for (let i = 0; i < ka.length; i += 1) {
      if (ka[i] < kb[i]) return -1
      if (ka[i] > kb[i]) return 1
    }
    return 0
  })
}

// ---------------------------------------------------------------- manual rows
function deriveManualFromNotes(notesPath) {
  const notes = readCsv(path.resolve(ROOT, notesPath))
  const dictKeys = loadDictKeys()
  const manual = []

  for (const n of notes) {
    const file = (n.file || '').trim()
    const note = (n['备注'] || '').trim()
    const text = (n.text || '').trim()

    if (!file) {
      // 无 file/line = 产品需求备注
      const desc = (n.end || '').trim()
      const body = text || desc
      const implemented = text ? Boolean(grepSourceTree(text)) : false
      manual.push({
        end: 'note', file: '', line: '', text: body, source: 'manual', edit: '',
        note: `人工需求：${implemented ? '已实现' : '未实现'}${text && desc ? `；原描述：${desc}` : ''}`,
      })
      continue
    }
    if (!note) continue

    // 有 file + 备注 = 样式建议（红/黑加粗）；工作副本文案可能被截断或改写过，用字典最长公共前缀定位代码原文
    const key = matchStyleKey(text, dictKeys)
    if (key) {
      const line = findTCallLine(file, key)
      const drift = key === text ? '' : '（工作副本文案与代码不一致，已按代码校正）'
      manual.push({
        end: normalizeEnd(file), file, line: line || '', text: key, source: 'manual', edit: '',
        note: `人工备注：${note}；状态=已抽取${drift}`,
      })
    } else {
      manual.push({
        end: normalizeEnd(file), file, line: '', text, source: 'manual', edit: '',
        note: `人工备注：${note}；未匹配到代码文案（待产品确认）`,
      })
    }
  }

  // 8 条 #NAME?：4 条仍为残余（由 report 行承载，稍后原地打标）、4 条不在报告（已抽取/三元盲区）
  const reportKeys = new Set()
  for (const s of SOURCES) {
    for (const r of runReport(s.dir).rows) reportKeys.add(`${r.file}\u0000${r.text}`)
  }
  for (const rep of NAME_ERROR_REPAIRS) {
    if (reportKeys.has(`${rep.file}\u0000${rep.text}`)) continue
    const tLine = findTCallLine(rep.file, rep.text)
    const rawLine = findLiteralLine(rep.file, rep.text)
    let status = '未定位'
    let line = ''
    if (tLine) { status = '已抽取'; line = tLine } else if (rawLine) { status = '三元盲区'; line = rawLine }
    manual.push({
      end: normalizeEnd(rep.file), file: rep.file, line, text: rep.text, source: 'manual', edit: '',
      note: `待产品确认：原 #NAME?（Excel 公式损坏）；状态=${status}`,
    })
  }
  return manual
}

function markNameErrors(rows) {
  const reportKeys = new Set(rows.filter((r) => r.source === 'report').map((r) => `${r.file}\u0000${r.text}`))
  for (const rep of NAME_ERROR_REPAIRS) {
    if (!reportKeys.has(`${rep.file}\u0000${rep.text}`)) continue
    for (const r of rows) {
      if (r.source === 'report' && r.file === rep.file && r.text === rep.text) {
        r.note = '待产品确认：原 #NAME?（Excel 公式损坏）；状态=残余'
      }
    }
  }
  return rows
}

// ---------------------------------------------------------------- main
function toRow(o) {
  return {
    end: o.end || '',
    file: o.file || '',
    line: o.line === '' || o.line == null ? '' : Number(o.line),
    text: o.text || '',
    source: o['来源'] || o.source || 'report',
    edit: o['修改'] || '',
    note: o['备注'] || '',
  }
}

let exitCode = 0
const log = []

try {
  const existing = readCsv(OUT_CSV).map(toRow)
  let manualRows = existing.filter((r) => r.source === 'manual')

  if (NOTES && manualRows.length === 0) {
    manualRows = deriveManualFromNotes(NOTES)
  } else if (NOTES) {
    // 已导入过：保留库内人工行（幂等），不重复导入
    manualRows = existing.filter((r) => r.source === 'manual')
  }

  const built = build(manualRows)
  built.rows = markNameErrors(built.rows)
  const sorted = sortRows(built.rows)
  const csv = renderCsv(sorted)
  const md = renderMd(built.report, sorted, built.shadowed)

  write(OUT_CSV, csv)
  write(OUT_MD, md)

  const mobile = built.report.filter((r) => r.end === 'mobile').length
  const pc = built.report.filter((r) => r.end === 'pc').length
  const manual = sorted.filter((r) => r.source === 'manual').length
  const total = sorted.length

  log.push(`BUILD: rows=${total} mobile=${mobile} pc=${pc} manual=${manual}`)

  const checkExpected = (expected, actual) => Number(expected) === Number(actual)
  if (EXPECT_ROWS && !checkExpected(EXPECT_ROWS, total)) {
    log.push(`EXPECT-ROWS: FAIL (expected ${EXPECT_ROWS} actual ${total})`)
    exitCode = 1
  } else if (EXPECT_ROWS) {
    log.push(`EXPECT-ROWS: PASS (${total})`)
  }

  if (SELF_TEST) {
    // 幂等：重新跑一遍生成器并重建，产物必须逐字节相同
    const again = build(manualRows)
    again.rows = markNameErrors(again.rows)
    const sorted2 = sortRows(again.rows)
    const same = renderCsv(sorted2) === csv && renderMd(again.report, sorted2, again.shadowed) === md
    log.push(`IDEMPOTENT: ${same ? 'yes' : 'no'}`)
    if (!same) exitCode = 1

    // 负例：故意把期望行数写错，必须被判失败
    const negDetected = !checkExpected(total + 1, total) ? 1 : 0
    log.push(`NEGATIVE(expect-rows)=${negDetected}`)
    if (negDetected !== 1) exitCode = 1
  }

  log.push(`RESULT: ${exitCode === 0 ? 'PASS' : 'FAIL'}`)
} catch (e) {
  log.push(`ERROR: ${e && e.message ? e.message : e}`)
  log.push('RESULT: FAIL')
  exitCode = 1
}

const report = `${log.join('\n')}\n`
process.stdout.write(report)
if (EVIDENCE) {
  const p = path.resolve(ROOT, EVIDENCE)
  fs.mkdirSync(path.dirname(p), { recursive: true })
  fs.writeFileSync(p, report)
}
process.exit(exitCode)
