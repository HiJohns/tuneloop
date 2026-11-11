// #2325 回归测试：会话过期时间推导 + token 写入点不变式 + 接线校验。
// 运行：node --test frontend-mobile/src/services/sessionToken.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { computeExpiryMs } from './sessionToken.mjs'

const HERE = path.dirname(fileURLToPath(import.meta.url))
const SRC_ROOT = path.resolve(HERE, '..')
const REPO_ROOT = path.resolve(HERE, '../../..')
const FIXTURES = path.join(REPO_ROOT, 'docs/evidence/2325/fixtures')

function makeJwt(expSeconds) {
  const b64 = Buffer.from(JSON.stringify({ exp: expSeconds }))
    .toString('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '')
  return `header.${b64}.sig`
}

test('computeExpiryMs 规则：expires_in 优先 / JWT exp 回退 / 兜底 3600', async () => {
  const now = 1700000000000

  assert.equal(computeExpiryMs(900, undefined, now), now + 900000, 'expires_in 优先')

  const expSeconds = Math.floor(now / 1000) + 1000
  assert.equal(computeExpiryMs(undefined, makeJwt(expSeconds), now), expSeconds * 1000, 'JWT exp 回退')

  assert.equal(computeExpiryMs(undefined, 'not-a-jwt', now), now + 3600000, '兜底 3600s')
  assert.equal(computeExpiryMs(0, '', now), now + 3600000, '0/空值同样兜底')

  // 负例：buggy 实现忽略 expires_in，必须与本规则结果不同（否则本测试失去鉴别力）
  const buggy = await import(pathToFileURL(path.join(FIXTURES, 'sessionToken.buggy.mjs')).href)
  assert.notEqual(buggy.computeExpiryMs(900, undefined, now), now + 900000)
})

function scanTokenWrites(root) {
  const violations = []
  const walk = (dir) => {
    let entries = []
    try {
      entries = fs.readdirSync(dir, { withFileTypes: true })
    } catch {
      return
    }
    for (const e of entries) {
      const p = path.join(dir, e.name)
      if (e.isDirectory()) {
        if (e.name !== 'node_modules') walk(p)
        continue
      }
      if (!/\.(js|jsx|mjs|ts|tsx)$/.test(e.name)) continue
      if (/\.test\.(mjs|js|ts)$/.test(e.name)) continue
      const lines = fs.readFileSync(p, 'utf8').split('\n')
      lines.forEach((line, i) => {
        if (!line.includes("setItem('token'")) return
        const windowText = lines.slice(i, i + 6).join('\n')
        if (!/token_expiry|computeExpiryMs\(/.test(windowText)) violations.push(`${p}:${i + 1}`)
      })
    }
  }
  walk(root)
  return violations
}

test('token 写入点成对：所有 setItem(token) 均有 expiry', () => {
  const real = scanTokenWrites(SRC_ROOT)
  assert.deepEqual(real, [], `未成对写入 expiry: ${real.join(', ')}`)

  // 负例：夹具中的未成对写入必须被检出（证明扫描器有鉴别力）
  const fixtureViolations = scanTokenWrites(FIXTURES)
  assert.ok(fixtureViolations.length >= 1, 'unpaired fixture 必须被检出')
})

function extractFunction(src, header) {
  const start = src.indexOf(header)
  if (start < 0) return ''
  const end = src.indexOf('\n}', start)
  return end < 0 ? src.slice(start) : src.slice(start, end + 2)
}

test('接线：api.js 刷新与 Login.jsx 微信登录均使用 computeExpiryMs', () => {
  const api = fs.readFileSync(path.join(SRC_ROOT, 'services/api.js'), 'utf8')
  const login = fs.readFileSync(path.join(SRC_ROOT, 'pages-weapp/login/Login.jsx'), 'utf8')

  const refresh = extractFunction(api, 'async function refreshAccessToken(')
  assert.match(refresh, /computeExpiryMs\(/, 'refreshAccessToken 必须写 expiry')

  const wxLogin = extractFunction(login, 'async function handleWxLogin(')
  assert.match(wxLogin, /computeExpiryMs\(/, 'handleWxLogin 必须写 expiry')
  assert.match(wxLogin, /removeItem\('refresh_token'\)/, 'wx-login 须清除陈旧 refresh_token')

  // 负例：未接线夹具必须被判为未接线
  const unwired = fs.readFileSync(path.join(FIXTURES, 'api-unwired.js'), 'utf8')
  assert.doesNotMatch(extractFunction(unwired, 'async function refreshAccessToken('), /computeExpiryMs\(/)
})
